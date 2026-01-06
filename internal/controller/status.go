/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/pkg/annotations"
)

// AggregatedStatus contains the computed pool status derived from node states.
type AggregatedStatus struct {
	TargetRevision          string
	CurrentRevision         string
	MachineCount            int
	ReadyMachineCount       int
	UpdatedMachineCount     int
	UpdatingMachineCount    int
	DegradedMachineCount    int
	UnavailableMachineCount int
	PendingRebootCount      int
	CordonedMachineCount    int
	DrainingMachineCount    int
	Conditions              []metav1.Condition
}

// AggregateStatus computes pool status from node states.
func AggregateStatus(target string, nodes []corev1.Node) *AggregatedStatus {
	status := &AggregatedStatus{
		TargetRevision: target,
		MachineCount:   len(nodes),
	}

	revisionCounts := make(map[string]int)

	for _, node := range nodes {
		nodeAnnotations := node.Annotations
		if nodeAnnotations == nil {
			nodeAnnotations = make(map[string]string)
		}

		current := annotations.GetAnnotation(nodeAnnotations, annotations.CurrentRevision)
		state := annotations.GetAnnotation(nodeAnnotations, annotations.AgentState)
		rebootPending := annotations.GetBoolAnnotation(nodeAnnotations, annotations.RebootPending)

		if current != "" {
			revisionCounts[current]++
		}

		if current == target {
			status.UpdatedMachineCount++
			if state == annotations.StateDone || state == annotations.StateIdle {
				status.ReadyMachineCount++
			}
		}

		if state == annotations.StateApplying {
			status.UpdatingMachineCount++
		}

		if state == annotations.StateError {
			status.DegradedMachineCount++
		}

		if state != annotations.StateDone && state != annotations.StateIdle {
			status.UnavailableMachineCount++
		}

		if rebootPending {
			status.PendingRebootCount++
		}

		cordoned := annotations.GetBoolAnnotation(nodeAnnotations, annotations.Cordoned)
		if cordoned || node.Spec.Unschedulable {
			status.CordonedMachineCount++
		}

		drainStarted := annotations.GetAnnotation(nodeAnnotations, annotations.DrainStartedAt)
		if drainStarted != "" {
			status.DrainingMachineCount++
		}
	}

	status.CurrentRevision = computeCurrentRevision(revisionCounts, target)

	status.Conditions = computeConditions(status)

	return status
}

func computeCurrentRevision(counts map[string]int, target string) string {
	if len(counts) == 0 {
		return target
	}

	maxCount := 0
	for _, count := range counts {
		if count > maxCount {
			maxCount = count
		}
	}

	candidates := make([]string, 0)
	for rev, count := range counts {
		if count == maxCount {
			candidates = append(candidates, rev)
		}
	}

	for _, c := range candidates {
		if c == target {
			return target
		}
	}

	sort.Strings(candidates)
	return candidates[0]
}

func computeConditions(status *AggregatedStatus) []metav1.Condition {
	now := metav1.Now()
	conditions := make([]metav1.Condition, 0, 3)

	if status.MachineCount > 0 && status.UpdatedMachineCount == status.MachineCount {
		conditions = append(conditions, metav1.Condition{
			Type:               mcov1alpha1.ConditionUpdated,
			Status:             metav1.ConditionTrue,
			Reason:             "AllNodesUpdated",
			Message:            fmt.Sprintf("All %d nodes are at target revision", status.MachineCount),
			LastTransitionTime: now,
		})
	} else {
		conditions = append(conditions, metav1.Condition{
			Type:               mcov1alpha1.ConditionUpdated,
			Status:             metav1.ConditionFalse,
			Reason:             "NodesUpdating",
			Message:            fmt.Sprintf("%d of %d nodes updated", status.UpdatedMachineCount, status.MachineCount),
			LastTransitionTime: now,
		})
	}

	if status.UpdatedMachineCount < status.MachineCount && status.DegradedMachineCount == 0 {
		conditions = append(conditions, metav1.Condition{
			Type:               mcov1alpha1.ConditionUpdating,
			Status:             metav1.ConditionTrue,
			Reason:             "RolloutInProgress",
			Message:            fmt.Sprintf("Updating %d nodes", status.MachineCount-status.UpdatedMachineCount),
			LastTransitionTime: now,
		})
	} else {
		conditions = append(conditions, metav1.Condition{
			Type:               mcov1alpha1.ConditionUpdating,
			Status:             metav1.ConditionFalse,
			Reason:             "RolloutComplete",
			Message:            "No rollout in progress",
			LastTransitionTime: now,
		})
	}

	if status.DegradedMachineCount > 0 {
		conditions = append(conditions, metav1.Condition{
			Type:               mcov1alpha1.ConditionDegraded,
			Status:             metav1.ConditionTrue,
			Reason:             "NodeErrors",
			Message:            fmt.Sprintf("%d nodes in error state", status.DegradedMachineCount),
			LastTransitionTime: now,
		})
	} else {
		conditions = append(conditions, metav1.Condition{
			Type:               mcov1alpha1.ConditionDegraded,
			Status:             metav1.ConditionFalse,
			Reason:             "NoErrors",
			Message:            "No nodes in error state",
			LastTransitionTime: now,
		})
	}

	return conditions
}

// ApplyStatusToPool updates the pool status with aggregated values.
func ApplyStatusToPool(pool *mcov1alpha1.MachineConfigPool, status *AggregatedStatus) {
	pool.Status.TargetRevision = status.TargetRevision
	pool.Status.CurrentRevision = status.CurrentRevision
	pool.Status.MachineCount = status.MachineCount
	pool.Status.ReadyMachineCount = status.ReadyMachineCount
	pool.Status.UpdatedMachineCount = status.UpdatedMachineCount
	pool.Status.UpdatingMachineCount = status.UpdatingMachineCount
	pool.Status.DegradedMachineCount = status.DegradedMachineCount
	pool.Status.UnavailableMachineCount = status.UnavailableMachineCount
	pool.Status.PendingRebootCount = status.PendingRebootCount
	pool.Status.CordonedMachineCount = status.CordonedMachineCount
	pool.Status.DrainingMachineCount = status.DrainingMachineCount

	pool.Status.Conditions = mergeConditions(pool.Status.Conditions, status.Conditions)
}

func mergeConditions(existing, new []metav1.Condition) []metav1.Condition {
	existingMap := make(map[string]metav1.Condition)
	for _, c := range existing {
		existingMap[c.Type] = c
	}

	result := make([]metav1.Condition, 0, len(new))
	for _, newCondition := range new {
		if existingCondition, ok := existingMap[newCondition.Type]; ok {
			if existingCondition.Status == newCondition.Status {
				newCondition.LastTransitionTime = existingCondition.LastTransitionTime
			}
		}
		result = append(result, newCondition)
	}

	return result
}

// ComputeOverlapCondition creates the PoolOverlap condition based on overlap detection result.
func ComputeOverlapCondition(poolName string, overlap *OverlapResult) metav1.Condition {
	now := metav1.Now()

	if overlap == nil || !overlap.HasConflicts() {
		return metav1.Condition{
			Type:               mcov1alpha1.ConditionPoolOverlap,
			Status:             metav1.ConditionFalse,
			Reason:             "NoOverlap",
			Message:            "No overlapping nodes detected",
			LastTransitionTime: now,
		}
	}

	conflictingNodes := overlap.GetConflictsForPool(poolName)
	if len(conflictingNodes) == 0 {
		return metav1.Condition{
			Type:               mcov1alpha1.ConditionPoolOverlap,
			Status:             metav1.ConditionFalse,
			Reason:             "NoOverlap",
			Message:            "No overlapping nodes detected",
			LastTransitionTime: now,
		}
	}

	// Build a message with node and pool information
	poolsInvolved := make(map[string]struct{})
	for _, nodeName := range conflictingNodes {
		pools := overlap.GetPoolsForNode(nodeName)
		for _, p := range pools {
			poolsInvolved[p] = struct{}{}
		}
	}

	// Remove current pool from the list for clearer message
	delete(poolsInvolved, poolName)
	otherPools := make([]string, 0, len(poolsInvolved))
	for p := range poolsInvolved {
		otherPools = append(otherPools, p)
	}
	sort.Strings(otherPools)

	var message string
	if len(conflictingNodes) == 1 {
		message = fmt.Sprintf("Node %s also matches pools: %v", conflictingNodes[0], otherPools)
	} else {
		message = fmt.Sprintf("Nodes %v also match pools: %v", conflictingNodes, otherPools)
	}

	return metav1.Condition{
		Type:               mcov1alpha1.ConditionPoolOverlap,
		Status:             metav1.ConditionTrue,
		Reason:             "NodesInMultiplePools",
		Message:            message,
		LastTransitionTime: now,
	}
}

// ApplyOverlapCondition updates the pool's conditions with overlap status.
// It also sets Degraded=True when there's an overlap conflict.
func ApplyOverlapCondition(pool *mcov1alpha1.MachineConfigPool, overlap *OverlapResult) {
	overlapCondition := ComputeOverlapCondition(pool.Name, overlap)

	// Find and update or append the PoolOverlap condition
	found := false
	for i, c := range pool.Status.Conditions {
		if c.Type == mcov1alpha1.ConditionPoolOverlap {
			if c.Status == overlapCondition.Status {
				overlapCondition.LastTransitionTime = c.LastTransitionTime
			}
			pool.Status.Conditions[i] = overlapCondition
			found = true
			break
		}
	}
	if !found {
		pool.Status.Conditions = append(pool.Status.Conditions, overlapCondition)
	}

	// If there's overlap, ensure Degraded is set
	if overlapCondition.Status == metav1.ConditionTrue {
		setDegradedForOverlap(pool)
	}
}

// setDegradedForOverlap sets Degraded=True due to pool overlap.
// This doesn't override other degraded reasons - it only adds overlap reason if not already degraded.
func setDegradedForOverlap(pool *mcov1alpha1.MachineConfigPool) {
	now := metav1.Now()

	for i, c := range pool.Status.Conditions {
		if c.Type == mcov1alpha1.ConditionDegraded {
			// Only update if not already degraded for another reason
			if c.Status != metav1.ConditionTrue {
				pool.Status.Conditions[i] = metav1.Condition{
					Type:               mcov1alpha1.ConditionDegraded,
					Status:             metav1.ConditionTrue,
					Reason:             "PoolOverlapDetected",
					Message:            "Pool has nodes that match other pools",
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
		Reason:             "PoolOverlapDetected",
		Message:            "Pool has nodes that match other pools",
		LastTransitionTime: now,
	})
}
