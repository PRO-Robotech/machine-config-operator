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

type NodeUpdateResult struct {
	Result        ctrl.Result
	DrainStuck    bool
	DrainStuckMsg string
}

func ProcessNodeUpdate(
	ctx context.Context,
	c client.Client,
	pool *mcov1alpha1.MachineConfigPool,
	node *corev1.Node,
	targetRevision string,
) NodeUpdateResult {
	logger := log.FromContext(ctx)

	if !IsNodeCordoned(node) {
		if err := CordonNode(ctx, c, node); err != nil {
			logger.Error(err, "failed to cordon node", "node", node.Name)
			return NodeUpdateResult{Result: ctrl.Result{RequeueAfter: 5 * time.Second}}
		}
		return NodeUpdateResult{Result: ctrl.Result{RequeueAfter: time.Second}}
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
			retry := HandleDrainRetry(ctx, c, node)

			result := NodeUpdateResult{
				Result:     ctrl.Result{RequeueAfter: retry.RequeueAfter},
				DrainStuck: retry.SetDrainStuck,
			}
			if retry.SetDrainStuck {
				result.DrainStuckMsg = fmt.Sprintf("Node %s drain timeout: %v", node.Name, err)
				RecordDrainStuck(pool.Name)
			}
			return result
		}
	}

	currentDesired := annotations.GetAnnotation(node.Annotations, annotations.DesiredRevision)
	if currentDesired != targetRevision {
		if err := SetNodeAnnotation(ctx, c, node, annotations.DesiredRevision, targetRevision); err != nil {
			logger.Error(err, "failed to set desired revision", "node", node.Name)
			return NodeUpdateResult{Result: ctrl.Result{RequeueAfter: 5 * time.Second}}
		}
		if err := SetNodeAnnotation(ctx, c, node, annotations.Pool, pool.Name); err != nil {
			logger.Error(err, "failed to set pool annotation", "node", node.Name)
		}
		return NodeUpdateResult{Result: ctrl.Result{RequeueAfter: time.Second}}
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
		return NodeUpdateResult{Result: ctrl.Result{}}
	}

	return NodeUpdateResult{Result: ctrl.Result{RequeueAfter: 10 * time.Second}}
}

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
			return
		}
	}
	pool.Status.Conditions = append(pool.Status.Conditions, condition)
}

func ClearDrainStuckCondition(pool *mcov1alpha1.MachineConfigPool) {
	for i, c := range pool.Status.Conditions {
		if c.Type == mcov1alpha1.ConditionDrainStuck {
			pool.Status.Conditions[i].Status = metav1.ConditionFalse
			pool.Status.Conditions[i].Reason = "DrainComplete"
			pool.Status.Conditions[i].Message = ""
			pool.Status.Conditions[i].LastTransitionTime = metav1.Now()
			return
		}
	}
}
