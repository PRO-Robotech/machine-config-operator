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
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		if _, ok := pod.Annotations["kubernetes.io/config.mirror"]; ok {
			continue
		}

		if config.IgnoreDS && IsDaemonSetPod(&pod) {
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

func HandleDrainRetry(ctx context.Context, c client.Client, node *corev1.Node) DrainRetryResult {
	drainStartStr := annotations.GetAnnotation(node.Annotations, annotations.DrainStartedAt)
	if drainStartStr == "" {
		return DrainRetryResult{RequeueAfter: time.Minute, SetDrainStuck: false}
	}

	drainStart, err := time.Parse(time.RFC3339, drainStartStr)
	if err != nil {
		return DrainRetryResult{RequeueAfter: time.Minute, SetDrainStuck: false}
	}

	elapsed := time.Since(drainStart)

	retryCount := GetIntAnnotation(node, annotations.DrainRetryCount) + 1
	_ = SetNodeAnnotation(ctx, c, node, annotations.DrainRetryCount, strconv.Itoa(retryCount))

	switch {
	case elapsed < 10*time.Minute:
		return DrainRetryResult{RequeueAfter: time.Minute, SetDrainStuck: false}
	case elapsed < 60*time.Minute:
		return DrainRetryResult{RequeueAfter: 5 * time.Minute, SetDrainStuck: false}
	default:
		return DrainRetryResult{RequeueAfter: 5 * time.Minute, SetDrainStuck: true}
	}
}

func ClearDrainAnnotations(ctx context.Context, c client.Client, node *corev1.Node) error {
	if err := RemoveNodeAnnotation(ctx, c, node, annotations.DrainStartedAt); err != nil {
		return err
	}
	return RemoveNodeAnnotation(ctx, c, node, annotations.DrainRetryCount)
}
