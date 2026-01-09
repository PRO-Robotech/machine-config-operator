package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/pkg/annotations"
)

// NodeUpdateResult contains the result of processing a node update.
type NodeUpdateResult struct {
	Result        ctrl.Result
	DrainStuck    bool
	DrainStuckMsg string

	// Event flags for centralized event emission
	Cordoned      bool // Node was just cordoned in this reconcile
	DrainStarted  bool // Drain was just started in this reconcile
	DrainComplete bool // Drain just completed in this reconcile
	Uncordoned    bool // Node was just uncordoned in this reconcile
}

// ProcessNodeUpdate handles the node update lifecycle: cordon -> drain -> set revision -> uncordon.
// drainTimeoutSeconds specifies the maximum time before marking drain as stuck.
// drainRetrySeconds specifies the interval between drain retry attempts.
// If drainTimeoutSeconds is 0, DefaultDrainTimeoutSeconds (3600) is used.
// If drainRetrySeconds is 0, it is calculated as max(30, drainTimeoutSeconds/12).
func ProcessNodeUpdate(
	ctx context.Context,
	c client.Client,
	pool *mcov1alpha1.MachineConfigPool,
	node *corev1.Node,
	targetRevision string,
	drainTimeoutSeconds int,
	drainRetrySeconds int,
) NodeUpdateResult {
	logger := log.FromContext(ctx)

	// Check if drain was already started (for DrainStarted event)
	drainWasStarted := annotations.GetAnnotation(node.Annotations, annotations.DrainStartedAt) != ""

	if !IsNodeCordoned(node) {
		if err := CordonNode(ctx, c, node); err != nil {
			logger.Error(err, "failed to cordon node", "node", node.Name)
			return NodeUpdateResult{Result: ctrl.Result{RequeueAfter: 5 * time.Second}}
		}
		// Node was just cordoned
		return NodeUpdateResult{
			Result:   ctrl.Result{RequeueAfter: time.Second},
			Cordoned: true,
		}
	}

	drainConfig := DrainConfig{
		GracePeriod:   -1,
		IgnoreDS:      true,
		DeleteOrphans: true,
	}

	complete, err := IsDrainComplete(ctx, c, node, drainConfig)
	if err != nil {
		logger.Error(err, "failed to check drain status", "node", node.Name)
		return NodeUpdateResult{Result: ctrl.Result{RequeueAfter: 5 * time.Second}}
	}

	if !complete {
		if err := DrainNode(ctx, c, node, drainConfig); err != nil {
			logger.Info("drain incomplete, scheduling retry", "node", node.Name, "error", err)
			retry := HandleDrainRetry(ctx, c, node, drainTimeoutSeconds, drainRetrySeconds)

			result := NodeUpdateResult{
				Result:     ctrl.Result{RequeueAfter: retry.RequeueAfter},
				DrainStuck: retry.SetDrainStuck,
			}
			if retry.SetDrainStuck {
				result.DrainStuckMsg = fmt.Sprintf("Node %s drain timeout: %v", node.Name, err)
				RecordDrainStuck(pool.Name)
			}
			// DrainStarted event: first time drain started (annotation was just set)
			if !drainWasStarted {
				result.DrainStarted = true
			}
			return result
		}
		// Drain making progress but not complete - always return and requeue
		// This prevents falling through to set desired-revision while pods are still draining
		return NodeUpdateResult{
			Result:       ctrl.Result{RequeueAfter: 5 * time.Second},
			DrainStarted: !drainWasStarted, // true only on first call
		}
	}

	// Drain is complete - set flag if we just transitioned
	drainJustCompleted := drainWasStarted && complete

	currentDesired := annotations.GetAnnotation(node.Annotations, annotations.DesiredRevision)
	if currentDesired != targetRevision {
		if err := SetNodeAnnotation(ctx, c, node, annotations.DesiredRevision, targetRevision); err != nil {
			logger.Error(err, "failed to set desired revision", "node", node.Name)
			return NodeUpdateResult{Result: ctrl.Result{RequeueAfter: 5 * time.Second}}
		}
		// Record when we set the desired revision for apply timeout detection
		if err := SetNodeAnnotation(ctx, c, node, annotations.DesiredRevisionSetAt, time.Now().UTC().Format(time.RFC3339)); err != nil {
			logger.Error(err, "failed to set desired-revision-set-at", "node", node.Name)
		}
		if err := SetNodeAnnotation(ctx, c, node, annotations.Pool, pool.Name); err != nil {
			logger.Error(err, "failed to set pool annotation", "node", node.Name)
		}
		return NodeUpdateResult{
			Result:        ctrl.Result{RequeueAfter: time.Second},
			DrainComplete: drainJustCompleted,
		}
	}

	if ShouldUncordon(node, targetRevision) {
		drainStarted := annotations.GetAnnotation(node.Annotations, annotations.DrainStartedAt)
		if drainStarted != "" {
			if startTime, err := time.Parse(time.RFC3339, drainStarted); err == nil {
				duration := time.Since(startTime).Seconds()
				RecordDrainDuration(pool.Name, node.Name, duration)
			}
		}
		if err := UncordonNode(ctx, c, node); err != nil {
			logger.Error(err, "failed to uncordon node", "node", node.Name)
			return NodeUpdateResult{Result: ctrl.Result{RequeueAfter: 5 * time.Second}}
		}
		logger.Info("node update complete", "node", node.Name, "revision", targetRevision)
		// Node was just uncordoned - update complete
		return NodeUpdateResult{
			Result:     ctrl.Result{},
			Uncordoned: true,
		}
	}

	return NodeUpdateResult{Result: ctrl.Result{RequeueAfter: 10 * time.Second}}
}

// SetDrainStuckCondition sets DrainStuck=True and also Degraded=True.
func SetDrainStuckCondition(pool *mcov1alpha1.MachineConfigPool, message string) {
	condition := metav1.Condition{
		Type:               mcov1alpha1.ConditionDrainStuck,
		Status:             metav1.ConditionTrue,
		Reason:             "DrainTimeout",
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}

	for i, c := range pool.Status.Conditions {
		if c.Type == mcov1alpha1.ConditionDrainStuck {
			pool.Status.Conditions[i] = condition
			setDegradedForDrainStuck(pool)
			return
		}
	}
	pool.Status.Conditions = append(pool.Status.Conditions, condition)
	setDegradedForDrainStuck(pool)
}

// setDegradedForDrainStuck sets Degraded=True due to drain timeout.
// This doesn't override other degraded reasons - it only adds DrainStuck reason if not already degraded.
func setDegradedForDrainStuck(pool *mcov1alpha1.MachineConfigPool) {
	now := metav1.Now()

	for i, c := range pool.Status.Conditions {
		if c.Type == mcov1alpha1.ConditionDegraded {
			// Only update if not already degraded for another reason
			if c.Status != metav1.ConditionTrue {
				pool.Status.Conditions[i] = metav1.Condition{
					Type:               mcov1alpha1.ConditionDegraded,
					Status:             metav1.ConditionTrue,
					Reason:             "DrainStuck",
					Message:            "One or more nodes have drain stuck",
					LastTransitionTime: now,
				}
			}
			return
		}
	}

	// No existing Degraded condition, add one
	pool.Status.Conditions = append(pool.Status.Conditions, metav1.Condition{
		Type:               mcov1alpha1.ConditionDegraded,
		Status:             metav1.ConditionTrue,
		Reason:             "DrainStuck",
		Message:            "One or more nodes have drain stuck",
		LastTransitionTime: now,
	})
}

func ClearDrainStuckCondition(pool *mcov1alpha1.MachineConfigPool) {
	condition := metav1.Condition{
		Type:               mcov1alpha1.ConditionDrainStuck,
		Status:             metav1.ConditionFalse,
		Reason:             "DrainComplete",
		Message:            "",
		LastTransitionTime: metav1.Now(),
	}

	for i, c := range pool.Status.Conditions {
		if c.Type == mcov1alpha1.ConditionDrainStuck {
			pool.Status.Conditions[i] = condition
			return
		}
	}

	// Ensure the condition exists
	pool.Status.Conditions = append(pool.Status.Conditions, condition)
}
