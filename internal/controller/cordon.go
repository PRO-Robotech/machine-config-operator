package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"in-cloud.io/machine-config/pkg/annotations"
)

func CordonNode(ctx context.Context, c client.Client, node *corev1.Node) error {
	logger := log.FromContext(ctx)

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		current := &corev1.Node{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(node), current); err != nil {
			return err
		}

		if current.Spec.Unschedulable && annotations.GetBoolAnnotation(current.Annotations, annotations.Cordoned) {
			return nil
		}

		current.Spec.Unschedulable = true
		if current.Annotations == nil {
			current.Annotations = make(map[string]string)
		}
		current.Annotations[annotations.Cordoned] = annotations.ValueTrue

		if err := c.Update(ctx, current); err != nil {
			return err
		}

		logger.Info("node cordoned", "node", current.Name)
		return nil
	})
}

func UncordonNode(ctx context.Context, c client.Client, node *corev1.Node) error {
	logger := log.FromContext(ctx)

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		current := &corev1.Node{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(node), current); err != nil {
			return err
		}

		if !current.Spec.Unschedulable && !annotations.GetBoolAnnotation(current.Annotations, annotations.Cordoned) {
			return nil
		}

		current.Spec.Unschedulable = false
		if current.Annotations != nil {
			delete(current.Annotations, annotations.Cordoned)
			delete(current.Annotations, annotations.DrainStartedAt)
			delete(current.Annotations, annotations.DrainRetryCount)
		}

		if err := c.Update(ctx, current); err != nil {
			return err
		}

		logger.Info("node uncordoned", "node", current.Name)
		return nil
	})
}

func IsNodeCordoned(node *corev1.Node) bool {
	return annotations.GetBoolAnnotation(node.Annotations, annotations.Cordoned)
}

// ShouldUncordon returns true if node should be uncordoned.
//
// Condition: Node is cordoned AND current revision matches target.
// Agent state is NOT checked because when revisions already match,
// agent may stay in "idle" state (nothing to apply). The key indicator
// that the node is ready is the revision match, not the agent state.
func ShouldUncordon(node *corev1.Node, targetRevision string) bool {
	if !IsNodeCordoned(node) {
		return false
	}

	current := annotations.GetAnnotation(node.Annotations, annotations.CurrentRevision)

	// Only check revision match - agent state is not relevant
	// once the node has reached the target revision
	return current == targetRevision
}

func SetNodeAnnotation(ctx context.Context, c client.Client, node *corev1.Node, key, value string) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		current := &corev1.Node{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(node), current); err != nil {
			return err
		}

		if current.Annotations == nil {
			current.Annotations = make(map[string]string)
		}

		if current.Annotations[key] == value {
			return nil
		}

		current.Annotations[key] = value
		return c.Update(ctx, current)
	})
}

func RemoveNodeAnnotation(ctx context.Context, c client.Client, node *corev1.Node, key string) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		current := &corev1.Node{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(node), current); err != nil {
			return err
		}

		if current.Annotations == nil || current.Annotations[key] == "" {
			return nil
		}

		delete(current.Annotations, key)
		return c.Update(ctx, current)
	})
}

func GetIntAnnotation(node *corev1.Node, key string) int {
	val := annotations.GetAnnotation(node.Annotations, key)
	if val == "" {
		return 0
	}
	var result int
	_, _ = fmt.Sscanf(val, "%d", &result)
	return result
}
