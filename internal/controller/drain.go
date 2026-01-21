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
	"in-cloud.io/machine-config/pkg/drain"
)

type DrainOptions struct {
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

func DrainNode(ctx context.Context, c client.Client, node *corev1.Node, config DrainOptions, mcoNamespace string) error {
	return DrainNodeWithExclusions(ctx, c, node, config, nil, mcoNamespace)
}

func DrainNodeWithExclusions(ctx context.Context, c client.Client, node *corev1.Node, config DrainOptions, exclusions *drain.DrainConfig, mcoNamespace string) error {
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

	evictable := FilterEvictablePodsWithExclusions(podList.Items, config, exclusions, mcoNamespace)
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

func FilterEvictablePods(pods []corev1.Pod, config DrainOptions, mcoNamespace string) []corev1.Pod {
	return FilterEvictablePodsWithExclusions(pods, config, nil, mcoNamespace)
}

func FilterEvictablePodsWithExclusions(pods []corev1.Pod, config DrainOptions, exclusions *drain.DrainConfig, mcoNamespace string) []corev1.Pod {
	result := make([]corev1.Pod, 0, len(pods))

	for i := range pods {
		pod := &pods[i]

		// Skip terminating pods
		if pod.DeletionTimestamp != nil {
			continue
		}

		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		if _, ok := pod.Annotations["kubernetes.io/config.mirror"]; ok {
			continue
		}

		if config.IgnoreDS && IsDaemonSetPod(pod) {
			continue
		}

		if isMCOPod(pod, mcoNamespace) {
			continue
		}

		if !config.DeleteOrphans && !HasController(pod) {
			continue
		}

		if exclusions != nil {
			if skip, _ := exclusions.ShouldSkipPod(pod); skip {
				continue
			}
		}

		result = append(result, *pod)
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

func IsDrainComplete(ctx context.Context, c client.Client, node *corev1.Node, config DrainOptions, mcoNamespace string) (bool, error) {
	return IsDrainCompleteWithExclusions(ctx, c, node, config, nil, mcoNamespace)
}

func IsDrainCompleteWithExclusions(ctx context.Context, c client.Client, node *corev1.Node, config DrainOptions, exclusions *drain.DrainConfig, mcoNamespace string) (bool, error) {
	podList := &corev1.PodList{}
	if err := c.List(ctx, podList, client.MatchingFields{"spec.nodeName": node.Name}); err != nil {
		return false, err
	}

	evictable := FilterEvictablePodsWithExclusions(podList.Items, config, exclusions, mcoNamespace)
	return len(evictable) == 0, nil
}

type DrainRetryResult struct {
	RequeueAfter  time.Duration
	SetDrainStuck bool
}

const (
	DefaultDrainTimeoutSeconds = 3600
	DefaultDrainRetrySeconds   = 30
)

const (
	LabelAppName         = "app.kubernetes.io/name"
	LabelControlPlane    = "control-plane"
	MCOAppName           = "machine-config"
	MCOControllerManager = "controller-manager"
)

func isMCOPod(pod *corev1.Pod, mcoNamespace string) bool {
	if pod.Namespace == mcoNamespace {
		return true
	}

	// Fallback by labels if namespace differs
	if pod.Labels != nil &&
		pod.Labels[LabelAppName] == MCOAppName &&
		pod.Labels[LabelControlPlane] == MCOControllerManager {
		return true
	}

	return false
}

func HandleDrainRetry(ctx context.Context, c client.Client, node *corev1.Node, drainTimeoutSeconds, drainRetrySeconds int) DrainRetryResult {
	drainStartStr := annotations.GetAnnotation(node.Annotations, annotations.DrainStartedAt)
	if drainStartStr == "" {
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

	if drainTimeoutSeconds <= 0 {
		drainTimeoutSeconds = DefaultDrainTimeoutSeconds
	}
	drainTimeout := time.Duration(drainTimeoutSeconds) * time.Second

	retryInterval := calculateRetryInterval(drainTimeoutSeconds, drainRetrySeconds)

	if elapsed >= drainTimeout {
		return DrainRetryResult{RequeueAfter: retryInterval, SetDrainStuck: true}
	}

	remaining := drainTimeout - elapsed
	requeue := min(retryInterval, remaining)
	requeue = max(requeue, 10*time.Second)

	return DrainRetryResult{RequeueAfter: requeue, SetDrainStuck: false}
}

func calculateRetryInterval(drainTimeoutSeconds, drainRetrySeconds int) time.Duration {
	if drainRetrySeconds > 0 {
		return time.Duration(drainRetrySeconds) * time.Second
	}

	if drainTimeoutSeconds <= 0 {
		drainTimeoutSeconds = DefaultDrainTimeoutSeconds
	}
	calculated := max(drainTimeoutSeconds/12, DefaultDrainRetrySeconds)
	return time.Duration(calculated) * time.Second
}

func ClearDrainAnnotations(ctx context.Context, c client.Client, node *corev1.Node) error {
	if err := RemoveNodeAnnotation(ctx, c, node, annotations.DrainStartedAt); err != nil {
		return err
	}
	return RemoveNodeAnnotation(ctx, c, node, annotations.DrainRetryCount)
}
