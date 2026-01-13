package controller

import (
	"context"
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"in-cloud.io/machine-config/pkg/annotations"
)

type DrainConfig struct {
	GracePeriod   int64
	IgnoreDS      bool
	DeleteOrphans bool
}

type PDBBlockedError struct {
	Pod string
	Err error
}

func (e *PDBBlockedError) Error() string {
	return fmt.Sprintf("PDB blocked eviction of pod %s: %v", e.Pod, e.Err)
}

func DrainNode(ctx context.Context, c client.Client, node *corev1.Node, config DrainConfig) error {
	logger := log.FromContext(ctx)

	if annotations.GetAnnotation(node.Annotations, annotations.DrainStartedAt) == "" {
		if err := SetNodeAnnotation(ctx, c, node, annotations.DrainStartedAt,
			time.Now().Format(time.RFC3339)); err != nil {
			return err
		}
	}

	podList := &corev1.PodList{}
	if err := c.List(ctx, podList, client.MatchingFields{"spec.nodeName": node.Name}); err != nil {
		return fmt.Errorf("failed to list pods on node %s: %w", node.Name, err)
	}

	evictable := FilterEvictablePods(podList.Items, config)
	if len(evictable) == 0 {
		logger.Info("drain complete, no pods to evict", "node", node.Name)
		return nil
	}

	var errs []error
	for i := range evictable {
		pod := &evictable[i]
		if err := EvictPod(ctx, c, pod, config.GracePeriod); err != nil {
			errs = append(errs, fmt.Errorf("pod %s/%s: %w", pod.Namespace, pod.Name, err))
		} else {
			logger.Info("evicted pod", "pod", pod.Namespace+"/"+pod.Name, "node", node.Name)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("drain incomplete: %d/%d pods failed: %v", len(errs), len(evictable), errs[0])
	}

	logger.Info("drain complete", "node", node.Name, "evicted", len(evictable))
	return nil
}

func FilterEvictablePods(pods []corev1.Pod, config DrainConfig) []corev1.Pod {
	result := make([]corev1.Pod, 0, len(pods))

	for _, pod := range pods {
		// If a pod is already terminating, treat it as effectively drained.
		// In envtest (no kubelet), pods may remain with a DeletionTimestamp for a while,
		// and repeatedly trying to evict them would prevent progress.
		if pod.DeletionTimestamp != nil {
			continue
		}

		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		if _, ok := pod.Annotations["kubernetes.io/config.mirror"]; ok {
			continue
		}

		if config.IgnoreDS && IsDaemonSetPod(&pod) {
			continue
		}

		// Skip MCO's own pods to prevent self-eviction
		if isMCOPod(&pod) {
			continue
		}

		if !config.DeleteOrphans && !HasController(&pod) {
			continue
		}

		result = append(result, pod)
	}

	return result
}

func IsDaemonSetPod(pod *corev1.Pod) bool {
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}

func HasController(pod *corev1.Pod) bool {
	for _, ref := range pod.OwnerReferences {
		if ref.Controller != nil && *ref.Controller {
			return true
		}
	}
	return false
}

func EvictPod(ctx context.Context, c client.Client, pod *corev1.Pod, gracePeriod int64) error {
	eviction := &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
	}

	if gracePeriod >= 0 {
		eviction.DeleteOptions = &metav1.DeleteOptions{
			GracePeriodSeconds: &gracePeriod,
		}
	}

	err := c.SubResource("eviction").Create(ctx, pod, eviction)
	if err != nil {
		if apierrors.IsTooManyRequests(err) {
			return &PDBBlockedError{Pod: pod.Name, Err: err}
		}
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return nil
}

func IsDrainComplete(ctx context.Context, c client.Client, node *corev1.Node, config DrainConfig) (bool, error) {
	podList := &corev1.PodList{}
	if err := c.List(ctx, podList, client.MatchingFields{"spec.nodeName": node.Name}); err != nil {
		return false, err
	}

	evictable := FilterEvictablePods(podList.Items, config)
	return len(evictable) == 0, nil
}

type DrainRetryResult struct {
	RequeueAfter  time.Duration
	SetDrainStuck bool
}

// DefaultDrainTimeoutSeconds is the default drain timeout (1 hour).
const DefaultDrainTimeoutSeconds = 3600

// DefaultDrainRetrySeconds is the minimum drain retry interval (30 seconds).
const DefaultDrainRetrySeconds = 30

// MCONamespace is the namespace where MCO components run.
// Pods in this namespace are excluded from eviction to prevent self-disruption.
const MCONamespace = "machine-config-system"

// MCO labels used for fallback identification when namespace differs.
const (
	LabelAppName      = "app.kubernetes.io/name"
	LabelControlPlane = "control-plane"

	// Label values for MCO controller
	MCOAppName           = "machine-config"
	MCOControllerManager = "controller-manager"
)

// isMCOPod checks if a pod belongs to MCO and should be excluded from eviction.
// Uses namespace as primary check, labels as fallback for robustness.
func isMCOPod(pod *corev1.Pod) bool {
	if pod.Namespace == MCONamespace {
		return true
	}

	// Fallback: check by labels (in case namespace differs or pod is misconfigured)
	// Controller has: app.kubernetes.io/name=machine-config, control-plane=controller-manager
	if pod.Labels != nil &&
		pod.Labels[LabelAppName] == MCOAppName &&
		pod.Labels[LabelControlPlane] == MCOControllerManager {
		return true
	}

	return false
}

// HandleDrainRetry manages drain retry logic and determines if drain is stuck.
// drainTimeoutSeconds specifies the maximum time before marking drain as stuck.
// drainRetrySeconds specifies the interval between retry attempts.
// If drainTimeoutSeconds is 0, DefaultDrainTimeoutSeconds (3600) is used.
// If drainRetrySeconds is 0, it is calculated as max(30, drainTimeoutSeconds/12).
func HandleDrainRetry(ctx context.Context, c client.Client, node *corev1.Node, drainTimeoutSeconds, drainRetrySeconds int) DrainRetryResult {
	drainStartStr := annotations.GetAnnotation(node.Annotations, annotations.DrainStartedAt)
	if drainStartStr == "" {
		// First retry - use configured interval or default
		retryInterval := calculateRetryInterval(drainTimeoutSeconds, drainRetrySeconds)
		return DrainRetryResult{RequeueAfter: retryInterval, SetDrainStuck: false}
	}

	drainStart, err := time.Parse(time.RFC3339, drainStartStr)
	if err != nil {
		retryInterval := calculateRetryInterval(drainTimeoutSeconds, drainRetrySeconds)
		return DrainRetryResult{RequeueAfter: retryInterval, SetDrainStuck: false}
	}

	elapsed := time.Since(drainStart)

	retryCount := GetIntAnnotation(node, annotations.DrainRetryCount) + 1
	_ = SetNodeAnnotation(ctx, c, node, annotations.DrainRetryCount, strconv.Itoa(retryCount))

	// Use default timeout if not specified
	if drainTimeoutSeconds <= 0 {
		drainTimeoutSeconds = DefaultDrainTimeoutSeconds
	}
	drainTimeout := time.Duration(drainTimeoutSeconds) * time.Second

	// Calculate retry interval (configurable or auto-calculated)
	retryInterval := calculateRetryInterval(drainTimeoutSeconds, drainRetrySeconds)

	if elapsed >= drainTimeout {
		return DrainRetryResult{RequeueAfter: retryInterval, SetDrainStuck: true}
	}

	// Cap requeue to remaining time to avoid overshooting timeout
	remaining := drainTimeout - elapsed
	requeue := retryInterval
	if remaining < requeue {
		requeue = remaining
	}
	if requeue < 10*time.Second {
		requeue = 10 * time.Second // Minimum 10s to avoid busy-looping
	}

	return DrainRetryResult{RequeueAfter: requeue, SetDrainStuck: false}
}

// calculateRetryInterval returns the drain retry interval.
// If drainRetrySeconds is specified (> 0), it is used directly.
// Otherwise, it is calculated as max(30, drainTimeoutSeconds/12).
func calculateRetryInterval(drainTimeoutSeconds, drainRetrySeconds int) time.Duration {
	if drainRetrySeconds > 0 {
		return time.Duration(drainRetrySeconds) * time.Second
	}

	// Auto-calculate: ~12 retries before timeout
	if drainTimeoutSeconds <= 0 {
		drainTimeoutSeconds = DefaultDrainTimeoutSeconds
	}
	calculated := drainTimeoutSeconds / 12
	if calculated < DefaultDrainRetrySeconds {
		calculated = DefaultDrainRetrySeconds
	}
	return time.Duration(calculated) * time.Second
}

func ClearDrainAnnotations(ctx context.Context, c client.Client, node *corev1.Node) error {
	if err := RemoveNodeAnnotation(ctx, c, node, annotations.DrainStartedAt); err != nil {
		return err
	}
	return RemoveNodeAnnotation(ctx, c, node, annotations.DrainRetryCount)
}
