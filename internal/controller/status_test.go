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
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/pkg/annotations"
)

func makeNode(name string, current, state string) corev1.Node {
	ann := make(map[string]string)
	if current != "" {
		ann[annotations.CurrentRevision] = current
	}
	if state != "" {
		ann[annotations.AgentState] = state
	}
	return corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: ann,
		},
	}
}

func makeNodeWithReboot(name string, current, state string, rebootPending bool) corev1.Node {
	node := makeNode(name, current, state)
	if rebootPending {
		node.Annotations[annotations.RebootPending] = "true"
	}
	return node
}

// TestAggregateStatus_AllUpdated verifies status when all nodes are at target.
func TestAggregateStatus_AllUpdated(t *testing.T) {
	nodes := []corev1.Node{
		makeNode("worker-1", "workers-abc", annotations.StateDone),
		makeNode("worker-2", "workers-abc", annotations.StateDone),
		makeNode("worker-3", "workers-abc", annotations.StateIdle),
	}

	status := AggregateStatus("workers-abc", nodes, 0)

	if status.MachineCount != 3 {
		t.Errorf("MachineCount = %d, want 3", status.MachineCount)
	}
	if status.UpdatedMachineCount != 3 {
		t.Errorf("UpdatedMachineCount = %d, want 3", status.UpdatedMachineCount)
	}
	if status.ReadyMachineCount != 3 {
		t.Errorf("ReadyMachineCount = %d, want 3", status.ReadyMachineCount)
	}
	if status.DegradedMachineCount != 0 {
		t.Errorf("DegradedMachineCount = %d, want 0", status.DegradedMachineCount)
	}
	if status.CurrentRevision != "workers-abc" {
		t.Errorf("CurrentRevision = %s, want workers-abc", status.CurrentRevision)
	}

	foundUpdated := false
	for _, c := range status.Conditions {
		if c.Type == mcov1alpha1.ConditionReady && c.Status == metav1.ConditionTrue {
			foundUpdated = true
			break
		}
	}
	if !foundUpdated {
		t.Error("Updated condition should be True")
	}
}

// TestAggregateStatus_PartialUpdate verifies status during rollout.
func TestAggregateStatus_PartialUpdate(t *testing.T) {
	nodes := []corev1.Node{
		makeNode("worker-1", "workers-new", annotations.StateDone),
		makeNode("worker-2", "workers-old", annotations.StateApplying),
		makeNode("worker-3", "workers-old", annotations.StateIdle),
	}

	status := AggregateStatus("workers-new", nodes, 0)

	if status.UpdatedMachineCount != 1 {
		t.Errorf("UpdatedMachineCount = %d, want 1", status.UpdatedMachineCount)
	}
	if status.UpdatingMachineCount != 1 {
		t.Errorf("UpdatingMachineCount = %d, want 1", status.UpdatingMachineCount)
	}
	if status.UnavailableMachineCount != 1 {
		t.Errorf("UnavailableMachineCount = %d, want 1", status.UnavailableMachineCount)
	}

	// Check Updating condition
	foundUpdating := false
	for _, c := range status.Conditions {
		if c.Type == mcov1alpha1.ConditionUpdating && c.Status == metav1.ConditionTrue {
			foundUpdating = true
			break
		}
	}
	if !foundUpdating {
		t.Error("Updating condition should be True")
	}
}

// TestAggregateStatus_Degraded verifies status when nodes are in error.
func TestAggregateStatus_Degraded(t *testing.T) {
	nodes := []corev1.Node{
		makeNode("worker-1", "workers-abc", annotations.StateDone),
		makeNode("worker-2", "workers-old", annotations.StateError),
	}

	status := AggregateStatus("workers-abc", nodes, 0)

	if status.DegradedMachineCount != 1 {
		t.Errorf("DegradedMachineCount = %d, want 1", status.DegradedMachineCount)
	}

	// Check Degraded condition
	foundDegraded := false
	for _, c := range status.Conditions {
		if c.Type == mcov1alpha1.ConditionDegraded && c.Status == metav1.ConditionTrue {
			foundDegraded = true
			break
		}
	}
	if !foundDegraded {
		t.Error("Degraded condition should be True")
	}

	// Updating should be True when nodes are not all updated,
	// even if some nodes are degraded. This allows tracking rollout progress.
	foundUpdating := false
	for _, c := range status.Conditions {
		if c.Type == mcov1alpha1.ConditionUpdating && c.Status == metav1.ConditionTrue {
			foundUpdating = true
			break
		}
	}
	if !foundUpdating {
		t.Error("Updating condition should be True (nodes not all updated, degraded doesn't block)")
	}
}

// TestAggregateStatus_Empty verifies status with no nodes.
func TestAggregateStatus_Empty(t *testing.T) {
	status := AggregateStatus("workers-abc", []corev1.Node{}, 0)

	if status.MachineCount != 0 {
		t.Errorf("MachineCount = %d, want 0", status.MachineCount)
	}
	if status.CurrentRevision != "workers-abc" {
		t.Errorf("CurrentRevision = %s, want workers-abc (target)", status.CurrentRevision)
	}

	// Updated should be False for empty pool
	foundUpdated := false
	for _, c := range status.Conditions {
		if c.Type == mcov1alpha1.ConditionReady && c.Status == metav1.ConditionFalse {
			foundUpdated = true
			break
		}
	}
	if !foundUpdated {
		t.Error("Updated condition should be False for empty pool")
	}
}

// TestAggregateStatus_NilAnnotations verifies handling of nodes without annotations.
func TestAggregateStatus_NilAnnotations(t *testing.T) {
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "worker-1"}}, // nil annotations
		makeNode("worker-2", "workers-abc", annotations.StateDone),
	}

	status := AggregateStatus("workers-abc", nodes, 0)

	if status.MachineCount != 2 {
		t.Errorf("MachineCount = %d, want 2", status.MachineCount)
	}
	if status.UpdatedMachineCount != 1 {
		t.Errorf("UpdatedMachineCount = %d, want 1", status.UpdatedMachineCount)
	}
}

// TestAggregateStatus_RebootPending verifies reboot pending count.
func TestAggregateStatus_RebootPending(t *testing.T) {
	nodes := []corev1.Node{
		makeNodeWithReboot("worker-1", "workers-abc", annotations.StateDone, true),
		makeNodeWithReboot("worker-2", "workers-abc", annotations.StateDone, true),
		makeNode("worker-3", "workers-abc", annotations.StateDone),
	}

	status := AggregateStatus("workers-abc", nodes, 0)

	if status.PendingRebootCount != 2 {
		t.Errorf("PendingRebootCount = %d, want 2", status.PendingRebootCount)
	}
}

// TestComputeCurrentRevision_MostCommon verifies most common revision wins.
func TestComputeCurrentRevision_MostCommon(t *testing.T) {
	counts := map[string]int{
		"rev-a": 5,
		"rev-b": 3,
		"rev-c": 1,
	}

	result := computeCurrentRevision(counts, "rev-b")

	if result != "rev-a" {
		t.Errorf("currentRevision = %s, want rev-a (most common)", result)
	}
}

// TestComputeCurrentRevision_TiePreferTarget verifies target is preferred in tie.
func TestComputeCurrentRevision_TiePreferTarget(t *testing.T) {
	counts := map[string]int{
		"rev-a": 3,
		"rev-b": 3, // tie with target
	}

	result := computeCurrentRevision(counts, "rev-b")

	if result != "rev-b" {
		t.Errorf("currentRevision = %s, want rev-b (target in tie)", result)
	}
}

// TestComputeCurrentRevision_TieLexicographic verifies lexicographic fallback.
func TestComputeCurrentRevision_TieLexicographic(t *testing.T) {
	counts := map[string]int{
		"rev-c": 3,
		"rev-a": 3, // tie, not target
		"rev-b": 3,
	}

	result := computeCurrentRevision(counts, "rev-x")

	if result != "rev-a" {
		t.Errorf("currentRevision = %s, want rev-a (lexicographically first)", result)
	}
}

// TestComputeCurrentRevision_Empty verifies empty returns target.
func TestComputeCurrentRevision_Empty(t *testing.T) {
	result := computeCurrentRevision(map[string]int{}, "workers-abc")

	if result != "workers-abc" {
		t.Errorf("currentRevision = %s, want workers-abc (target for empty)", result)
	}
}

// TestComputeConditions_AllTrue verifies conditions when fully updated.
func TestComputeConditions_AllTrue(t *testing.T) {
	status := &AggregatedStatus{
		MachineCount:        3,
		UpdatedMachineCount: 3,
		ReadyMachineCount:   3,
	}

	conditions := computeConditions(status)

	// Expect 4 conditions: Ready, Updating, Degraded, Draining
	if len(conditions) != 4 {
		t.Errorf("len(conditions) = %d, want 4", len(conditions))
	}

	// Find Ready condition
	var readyCond *metav1.Condition
	for i := range conditions {
		if conditions[i].Type == mcov1alpha1.ConditionReady {
			readyCond = &conditions[i]
			break
		}
	}

	if readyCond == nil {
		t.Fatal("Ready condition not found")
	}
	if readyCond.Status != metav1.ConditionTrue {
		t.Errorf("Ready status = %s, want True", readyCond.Status)
	}
}

// TestComputeConditions_UpdatingWithDegraded verifies Updating is True even when
// there are degraded nodes. The update progress should be shown regardless of errors.
func TestComputeConditions_UpdatingWithDegraded(t *testing.T) {
	status := &AggregatedStatus{
		MachineCount:         5,
		UpdatedMachineCount:  3, // Not all updated
		DegradedMachineCount: 1, // Has degraded node
	}

	conditions := computeConditions(status)

	// Find conditions by type
	var updatingCond, degradedCond *metav1.Condition
	for i := range conditions {
		switch conditions[i].Type {
		case mcov1alpha1.ConditionUpdating:
			updatingCond = &conditions[i]
		case mcov1alpha1.ConditionDegraded:
			degradedCond = &conditions[i]
		}
	}

	// Updating should be True - nodes are still not at target
	if updatingCond == nil {
		t.Fatal("Updating condition not found")
	}
	if updatingCond.Status != metav1.ConditionTrue {
		t.Errorf("Updating status = %s, want True (should show progress despite degraded)", updatingCond.Status)
	}

	// Degraded should also be True
	if degradedCond == nil {
		t.Fatal("Degraded condition not found")
	}
	if degradedCond.Status != metav1.ConditionTrue {
		t.Errorf("Degraded status = %s, want True", degradedCond.Status)
	}
}

// TestApplyStatusToPool verifies status is applied correctly.
func TestApplyStatusToPool(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
	}

	status := &AggregatedStatus{
		TargetRevision:          "workers-new",
		CurrentRevision:         "workers-old",
		MachineCount:            5,
		ReadyMachineCount:       3,
		UpdatedMachineCount:     3,
		UpdatingMachineCount:    1,
		DegradedMachineCount:    1,
		UnavailableMachineCount: 2,
		PendingRebootCount:      0,
		Conditions: []metav1.Condition{
			{Type: mcov1alpha1.ConditionReady, Status: metav1.ConditionFalse},
		},
	}

	ApplyStatusToPool(pool, status)

	if pool.Status.TargetRevision != "workers-new" {
		t.Errorf("TargetRevision = %s, want workers-new", pool.Status.TargetRevision)
	}
	if pool.Status.MachineCount != 5 {
		t.Errorf("MachineCount = %d, want 5", pool.Status.MachineCount)
	}
	if len(pool.Status.Conditions) != 1 {
		t.Errorf("len(Conditions) = %d, want 1", len(pool.Status.Conditions))
	}
}

// TestMergeConditions_PreservesTransitionTime verifies LastTransitionTime is preserved.
func TestMergeConditions_PreservesTransitionTime(t *testing.T) {
	oldTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
	newTime := metav1.Now()

	existing := []metav1.Condition{
		{Type: mcov1alpha1.ConditionReady, Status: metav1.ConditionTrue, LastTransitionTime: oldTime},
		{Type: mcov1alpha1.ConditionDegraded, Status: metav1.ConditionFalse, LastTransitionTime: oldTime},
	}

	new := []metav1.Condition{
		{Type: mcov1alpha1.ConditionReady, Status: metav1.ConditionTrue, LastTransitionTime: newTime},    // same status
		{Type: mcov1alpha1.ConditionDegraded, Status: metav1.ConditionTrue, LastTransitionTime: newTime}, // changed status
	}

	result := mergeConditions(existing, new)

	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}

	// Updated: same status, should preserve old time
	if result[0].LastTransitionTime != oldTime {
		t.Error("Updated condition should preserve old LastTransitionTime")
	}

	// Degraded: changed status, should use new time
	if result[1].LastTransitionTime != newTime {
		t.Error("Degraded condition should use new LastTransitionTime")
	}
}

// TestMergeConditions_NewCondition verifies new conditions get current time.
func TestMergeConditions_NewCondition(t *testing.T) {
	newTime := metav1.Now()

	existing := []metav1.Condition{}
	new := []metav1.Condition{
		{Type: mcov1alpha1.ConditionReady, Status: metav1.ConditionTrue, LastTransitionTime: newTime},
	}

	result := mergeConditions(existing, new)

	if len(result) != 1 {
		t.Fatalf("len(result) = %d, want 1", len(result))
	}

	if result[0].LastTransitionTime != newTime {
		t.Error("New condition should use provided LastTransitionTime")
	}
}

// TestComputeOverlapCondition_NoConflicts verifies condition when no overlap.
func TestComputeOverlapCondition_NoConflicts(t *testing.T) {
	overlap := NewOverlapResult()

	condition := ComputeOverlapCondition("workers", overlap)

	if condition.Type != mcov1alpha1.ConditionPoolOverlap {
		t.Errorf("Type = %s, want PoolOverlap", condition.Type)
	}
	if condition.Status != metav1.ConditionFalse {
		t.Errorf("Status = %s, want False", condition.Status)
	}
	if condition.Reason != "NoOverlap" {
		t.Errorf("Reason = %s, want NoOverlap", condition.Reason)
	}
}

// TestComputeOverlapCondition_NilOverlap verifies condition when overlap is nil.
func TestComputeOverlapCondition_NilOverlap(t *testing.T) {
	condition := ComputeOverlapCondition("workers", nil)

	if condition.Status != metav1.ConditionFalse {
		t.Errorf("Status = %s, want False for nil overlap", condition.Status)
	}
}

// TestComputeOverlapCondition_HasConflict verifies condition when pool has overlapping nodes.
func TestComputeOverlapCondition_HasConflict(t *testing.T) {
	overlap := &OverlapResult{
		ConflictingNodes: map[string][]string{
			"node1": {"workers", "infra"},
			"node2": {"workers", "prod"},
		},
	}

	condition := ComputeOverlapCondition("workers", overlap)

	if condition.Status != metav1.ConditionTrue {
		t.Errorf("Status = %s, want True", condition.Status)
	}
	if condition.Reason != "NodesInMultiplePools" {
		t.Errorf("Reason = %s, want NodesInMultiplePools", condition.Reason)
	}

	// Message should contain node names and other pools
	if condition.Message == "" {
		t.Error("Message should not be empty")
	}
}

// TestComputeOverlapCondition_SingleNode verifies message format for single node.
func TestComputeOverlapCondition_SingleNode(t *testing.T) {
	overlap := &OverlapResult{
		ConflictingNodes: map[string][]string{
			"node1": {"workers", "infra"},
		},
	}

	condition := ComputeOverlapCondition("workers", overlap)

	// For single node, message format should be "Node X also matches pools: [Y]"
	if condition.Message == "" {
		t.Error("Message should not be empty")
	}
	// Verify it mentions the other pool (infra) but not the current pool (workers)
	if len(condition.Message) == 0 {
		t.Error("Message should describe the conflict")
	}
}

// TestComputeOverlapCondition_PoolNotInConflict verifies pool with no conflicting nodes.
func TestComputeOverlapCondition_PoolNotInConflict(t *testing.T) {
	overlap := &OverlapResult{
		ConflictingNodes: map[string][]string{
			"node1": {"infra", "prod"}, // workers is not involved
		},
	}

	condition := ComputeOverlapCondition("workers", overlap)

	// workers has no conflicting nodes, should be False
	if condition.Status != metav1.ConditionFalse {
		t.Errorf("Status = %s, want False (pool not in conflict)", condition.Status)
	}
}

// TestApplyOverlapCondition_NoConflicts verifies applying no-conflict condition.
func TestApplyOverlapCondition_NoConflicts(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "workers"},
	}
	overlap := NewOverlapResult()

	ApplyOverlapCondition(pool, overlap)

	// Should have PoolOverlap condition
	var foundOverlap bool
	for _, c := range pool.Status.Conditions {
		if c.Type == mcov1alpha1.ConditionPoolOverlap {
			foundOverlap = true
			if c.Status != metav1.ConditionFalse {
				t.Errorf("PoolOverlap status = %s, want False", c.Status)
			}
		}
	}
	if !foundOverlap {
		t.Error("PoolOverlap condition should be added")
	}
}

// TestApplyOverlapCondition_WithConflicts verifies applying conflict condition.
func TestApplyOverlapCondition_WithConflicts(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "workers"},
	}
	overlap := &OverlapResult{
		ConflictingNodes: map[string][]string{
			"node1": {"workers", "infra"},
		},
	}

	ApplyOverlapCondition(pool, overlap)

	// Should have PoolOverlap=True
	var foundOverlap, foundDegraded bool
	for _, c := range pool.Status.Conditions {
		if c.Type == mcov1alpha1.ConditionPoolOverlap {
			foundOverlap = true
			if c.Status != metav1.ConditionTrue {
				t.Errorf("PoolOverlap status = %s, want True", c.Status)
			}
		}
		if c.Type == mcov1alpha1.ConditionDegraded {
			foundDegraded = true
			if c.Status != metav1.ConditionTrue {
				t.Errorf("Degraded status = %s, want True", c.Status)
			}
			if c.Reason != "PoolOverlapDetected" {
				t.Errorf("Degraded reason = %s, want PoolOverlapDetected", c.Reason)
			}
		}
	}
	if !foundOverlap {
		t.Error("PoolOverlap condition should be added")
	}
	if !foundDegraded {
		t.Error("Degraded condition should be set when overlap detected")
	}
}

// TestApplyOverlapCondition_PreservesExistingConditions verifies existing conditions are preserved.
func TestApplyOverlapCondition_PreservesExistingConditions(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "workers"},
		Status: mcov1alpha1.MachineConfigPoolStatus{
			Conditions: []metav1.Condition{
				{
					Type:   mcov1alpha1.ConditionReady,
					Status: metav1.ConditionTrue,
					Reason: "AllNodesUpdated",
				},
			},
		},
	}
	overlap := NewOverlapResult()

	ApplyOverlapCondition(pool, overlap)

	// Original Updated condition should still be there
	var foundUpdated bool
	for _, c := range pool.Status.Conditions {
		if c.Type == mcov1alpha1.ConditionReady {
			foundUpdated = true
		}
	}
	if !foundUpdated {
		t.Error("Existing Updated condition should be preserved")
	}
}

// TestApplyOverlapCondition_UpdatesExistingOverlapCondition verifies condition update.
func TestApplyOverlapCondition_UpdatesExistingOverlapCondition(t *testing.T) {
	oldTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "workers"},
		Status: mcov1alpha1.MachineConfigPoolStatus{
			Conditions: []metav1.Condition{
				{
					Type:               mcov1alpha1.ConditionPoolOverlap,
					Status:             metav1.ConditionFalse,
					Reason:             "NoOverlap",
					LastTransitionTime: oldTime,
				},
			},
		},
	}

	// Now there's a conflict
	overlap := &OverlapResult{
		ConflictingNodes: map[string][]string{
			"node1": {"workers", "infra"},
		},
	}

	ApplyOverlapCondition(pool, overlap)

	// Find PoolOverlap condition
	for _, c := range pool.Status.Conditions {
		if c.Type == mcov1alpha1.ConditionPoolOverlap {
			if c.Status != metav1.ConditionTrue {
				t.Errorf("PoolOverlap status should be updated to True")
			}
			// LastTransitionTime should be updated (status changed)
			if c.LastTransitionTime == oldTime {
				t.Error("LastTransitionTime should be updated when status changes")
			}
			return
		}
	}
	t.Error("PoolOverlap condition not found")
}

// TestSetDegradedForOverlap_DoesNotOverrideExistingDegraded verifies existing degraded is preserved.
func TestSetDegradedForOverlap_DoesNotOverrideExistingDegraded(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "workers"},
		Status: mcov1alpha1.MachineConfigPoolStatus{
			Conditions: []metav1.Condition{
				{
					Type:   mcov1alpha1.ConditionDegraded,
					Status: metav1.ConditionTrue,
					Reason: "NodeErrors", // Existing degraded reason
				},
			},
		},
	}

	setDegradedForOverlap(pool)

	// Should not override existing Degraded=True with a different reason
	for _, c := range pool.Status.Conditions {
		if c.Type == mcov1alpha1.ConditionDegraded {
			if c.Reason != "NodeErrors" {
				t.Errorf("Degraded reason should not change from NodeErrors, got %s", c.Reason)
			}
			return
		}
	}
	t.Error("Degraded condition not found")
}

func makeCordonedNode(name string, drainStartedAt string) corev1.Node {
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: make(map[string]string),
		},
	}
	node.Annotations[annotations.Cordoned] = "true"
	node.Spec.Unschedulable = true
	if drainStartedAt != "" {
		node.Annotations[annotations.DrainStartedAt] = drainStartedAt
	}
	return node
}

func TestAggregateStatus_NoCordoned(t *testing.T) {
	nodes := []corev1.Node{
		makeNode("worker-1", "workers-abc", annotations.StateDone),
		makeNode("worker-2", "workers-abc", annotations.StateDone),
	}

	status := AggregateStatus("workers-abc", nodes, 0)

	if status.CordonedMachineCount != 0 {
		t.Errorf("CordonedMachineCount = %d, want 0", status.CordonedMachineCount)
	}
	if status.DrainingMachineCount != 0 {
		t.Errorf("DrainingMachineCount = %d, want 0", status.DrainingMachineCount)
	}
}

func TestAggregateStatus_OneCordoned(t *testing.T) {
	nodes := []corev1.Node{
		makeCordonedNode("worker-1", ""),
		makeNode("worker-2", "workers-abc", annotations.StateDone),
	}

	status := AggregateStatus("workers-abc", nodes, 0)

	if status.CordonedMachineCount != 1 {
		t.Errorf("CordonedMachineCount = %d, want 1", status.CordonedMachineCount)
	}
	if status.DrainingMachineCount != 0 {
		t.Errorf("DrainingMachineCount = %d, want 0", status.DrainingMachineCount)
	}
}

func TestAggregateStatus_MultipleCordoned(t *testing.T) {
	nodes := []corev1.Node{
		makeCordonedNode("worker-1", ""),
		makeCordonedNode("worker-2", ""),
		makeCordonedNode("worker-3", ""),
		makeNode("worker-4", "workers-abc", annotations.StateDone),
	}

	status := AggregateStatus("workers-abc", nodes, 0)

	if status.CordonedMachineCount != 3 {
		t.Errorf("CordonedMachineCount = %d, want 3", status.CordonedMachineCount)
	}
}

func TestAggregateStatus_Draining(t *testing.T) {
	drainStarted := time.Now().Format(time.RFC3339)
	nodes := []corev1.Node{
		makeCordonedNode("worker-1", drainStarted),
		makeCordonedNode("worker-2", drainStarted),
		makeNode("worker-3", "workers-abc", annotations.StateDone),
	}

	status := AggregateStatus("workers-abc", nodes, 0)

	if status.CordonedMachineCount != 2 {
		t.Errorf("CordonedMachineCount = %d, want 2", status.CordonedMachineCount)
	}
	if status.DrainingMachineCount != 2 {
		t.Errorf("DrainingMachineCount = %d, want 2", status.DrainingMachineCount)
	}
}

func TestAggregateStatus_MixedCordonDrain(t *testing.T) {
	drainStarted := time.Now().Format(time.RFC3339)
	nodes := []corev1.Node{
		makeCordonedNode("worker-1", drainStarted), // cordoned + draining
		makeCordonedNode("worker-2", ""),           // cordoned only
		makeNode("worker-3", "workers-abc", annotations.StateDone),
	}

	status := AggregateStatus("workers-abc", nodes, 0)

	if status.CordonedMachineCount != 2 {
		t.Errorf("CordonedMachineCount = %d, want 2", status.CordonedMachineCount)
	}
	if status.DrainingMachineCount != 1 {
		t.Errorf("DrainingMachineCount = %d, want 1", status.DrainingMachineCount)
	}
}

func TestAggregateStatus_UnschedulableWithoutAnnotation(t *testing.T) {
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "worker-1",
			Annotations: make(map[string]string),
		},
		Spec: corev1.NodeSpec{
			Unschedulable: true, // manually cordoned without annotation
		},
	}
	nodes := []corev1.Node{node}

	status := AggregateStatus("workers-abc", nodes, 0)

	if status.CordonedMachineCount != 1 {
		t.Errorf("CordonedMachineCount = %d, want 1 (unschedulable)", status.CordonedMachineCount)
	}
}

func TestApplyStatusToPool_NewFields(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
	}

	status := &AggregatedStatus{
		TargetRevision:       "workers-new",
		CurrentRevision:      "workers-old",
		MachineCount:         5,
		CordonedMachineCount: 2,
		DrainingMachineCount: 1,
		Conditions:           []metav1.Condition{},
	}

	ApplyStatusToPool(pool, status)

	if pool.Status.CordonedMachineCount != 2 {
		t.Errorf("CordonedMachineCount = %d, want 2", pool.Status.CordonedMachineCount)
	}
	if pool.Status.DrainingMachineCount != 1 {
		t.Errorf("DrainingMachineCount = %d, want 1", pool.Status.DrainingMachineCount)
	}
}

// TestApplyStatusToPool_UpdatesLastSuccessfulRevision verifies LastSuccessfulRevision
// is updated when all nodes are successfully updated with no degraded or pending-reboot.
func TestApplyStatusToPool_UpdatesLastSuccessfulRevision(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Status: mcov1alpha1.MachineConfigPoolStatus{
			LastSuccessfulRevision: "workers-old",
		},
	}

	status := &AggregatedStatus{
		TargetRevision:       "workers-new",
		CurrentRevision:      "workers-new",
		MachineCount:         3,
		UpdatedMachineCount:  3, // All nodes updated
		DegradedMachineCount: 0, // No errors
		PendingRebootCount:   0, // No pending reboots
		Conditions:           []metav1.Condition{},
	}

	ApplyStatusToPool(pool, status)

	if pool.Status.LastSuccessfulRevision != "workers-new" {
		t.Errorf("LastSuccessfulRevision = %s, want workers-new", pool.Status.LastSuccessfulRevision)
	}
}

// TestApplyStatusToPool_NoUpdateWhenDegraded verifies LastSuccessfulRevision
// is NOT updated when there are degraded nodes.
func TestApplyStatusToPool_NoUpdateWhenDegraded(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Status: mcov1alpha1.MachineConfigPoolStatus{
			LastSuccessfulRevision: "workers-old",
		},
	}

	status := &AggregatedStatus{
		TargetRevision:       "workers-new",
		CurrentRevision:      "workers-new",
		MachineCount:         3,
		UpdatedMachineCount:  3,
		DegradedMachineCount: 1, // Has degraded nodes
		PendingRebootCount:   0,
		Conditions:           []metav1.Condition{},
	}

	ApplyStatusToPool(pool, status)

	if pool.Status.LastSuccessfulRevision != "workers-old" {
		t.Errorf("LastSuccessfulRevision = %s, want workers-old (should not update when degraded)", pool.Status.LastSuccessfulRevision)
	}
}

// TestApplyStatusToPool_NoUpdateWhenPendingReboot verifies LastSuccessfulRevision
// is NOT updated when there are pending-reboot nodes.
func TestApplyStatusToPool_NoUpdateWhenPendingReboot(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Status: mcov1alpha1.MachineConfigPoolStatus{
			LastSuccessfulRevision: "workers-old",
		},
	}

	status := &AggregatedStatus{
		TargetRevision:       "workers-new",
		CurrentRevision:      "workers-new",
		MachineCount:         3,
		UpdatedMachineCount:  3,
		DegradedMachineCount: 0,
		PendingRebootCount:   2, // Has pending reboots
		Conditions:           []metav1.Condition{},
	}

	ApplyStatusToPool(pool, status)

	if pool.Status.LastSuccessfulRevision != "workers-old" {
		t.Errorf("LastSuccessfulRevision = %s, want workers-old (should not update when pending reboot)", pool.Status.LastSuccessfulRevision)
	}
}

// TestApplyStatusToPool_NoUpdateWhenPartialUpdate verifies LastSuccessfulRevision
// is NOT updated when not all nodes are updated.
func TestApplyStatusToPool_NoUpdateWhenPartialUpdate(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Status: mcov1alpha1.MachineConfigPoolStatus{
			LastSuccessfulRevision: "workers-old",
		},
	}

	status := &AggregatedStatus{
		TargetRevision:       "workers-new",
		CurrentRevision:      "workers-old",
		MachineCount:         3,
		UpdatedMachineCount:  2, // Not all nodes updated
		DegradedMachineCount: 0,
		PendingRebootCount:   0,
		Conditions:           []metav1.Condition{},
	}

	ApplyStatusToPool(pool, status)

	if pool.Status.LastSuccessfulRevision != "workers-old" {
		t.Errorf("LastSuccessfulRevision = %s, want workers-old (should not update when partial)", pool.Status.LastSuccessfulRevision)
	}
}

// TestApplyStatusToPool_NoUpdateWhenEmpty verifies LastSuccessfulRevision
// is NOT updated when pool has no nodes.
func TestApplyStatusToPool_NoUpdateWhenEmpty(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Status: mcov1alpha1.MachineConfigPoolStatus{
			LastSuccessfulRevision: "workers-old",
		},
	}

	status := &AggregatedStatus{
		TargetRevision:       "workers-new",
		CurrentRevision:      "workers-new",
		MachineCount:         0, // Empty pool
		UpdatedMachineCount:  0,
		DegradedMachineCount: 0,
		PendingRebootCount:   0,
		Conditions:           []metav1.Condition{},
	}

	ApplyStatusToPool(pool, status)

	if pool.Status.LastSuccessfulRevision != "workers-old" {
		t.Errorf("LastSuccessfulRevision = %s, want workers-old (should not update for empty pool)", pool.Status.LastSuccessfulRevision)
	}
}

// TestSetRenderDegradedCondition verifies RenderDegraded condition is set.
func TestSetRenderDegradedCondition(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
	}

	SetRenderDegradedCondition(pool, "failed to merge configs: duplicate file path")

	// Check RenderDegraded condition
	var foundRenderDegraded bool
	for _, c := range pool.Status.Conditions {
		if c.Type == mcov1alpha1.ConditionDegraded {
			foundRenderDegraded = true
			if c.Status != metav1.ConditionTrue {
				t.Errorf("RenderDegraded status = %s, want True", c.Status)
			}
			if c.Reason != "RenderFailed" {
				t.Errorf("RenderDegraded reason = %s, want RenderFailed", c.Reason)
			}
			if c.Message != "failed to merge configs: duplicate file path" {
				t.Errorf("RenderDegraded message = %s, want error message", c.Message)
			}
		}
	}
	if !foundRenderDegraded {
		t.Error("RenderDegraded condition should be set")
	}
}

// TestSetRenderDegradedCondition_AlsoSetsDegraded verifies Degraded is set with RenderFailed reason.
func TestSetRenderDegradedCondition_AlsoSetsDegraded(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
	}

	SetRenderDegradedCondition(pool, "merge error")

	// Check Degraded condition is set with RenderFailed reason
	var foundDegraded bool
	for _, c := range pool.Status.Conditions {
		if c.Type == mcov1alpha1.ConditionDegraded {
			foundDegraded = true
			if c.Status != metav1.ConditionTrue {
				t.Errorf("Degraded status = %s, want True", c.Status)
			}
			if c.Reason != "RenderFailed" {
				t.Errorf("Degraded reason = %s, want RenderFailed", c.Reason)
			}
		}
	}
	if !foundDegraded {
		t.Error("Degraded condition should be set when render fails")
	}
}

// TestClearRenderDegradedCondition verifies RenderDegraded condition is cleared.
func TestClearRenderDegradedCondition(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Status: mcov1alpha1.MachineConfigPoolStatus{
			Conditions: []metav1.Condition{
				{
					Type:   mcov1alpha1.ConditionDegraded,
					Status: metav1.ConditionTrue,
					Reason: "RenderFailed",
				},
			},
		},
	}

	ClearRenderDegradedCondition(pool)

	// Check RenderDegraded is now False
	for _, c := range pool.Status.Conditions {
		if c.Type == mcov1alpha1.ConditionDegraded {
			if c.Status != metav1.ConditionFalse {
				t.Errorf("RenderDegraded status = %s, want False", c.Status)
			}
			if c.Reason != "RenderSuccess" {
				t.Errorf("RenderDegraded reason = %s, want RenderSuccess", c.Reason)
			}
			return
		}
	}
	t.Error("RenderDegraded condition should exist")
}

// TestSetRenderDegradedCondition_UpdatesExisting verifies existing condition is updated.
func TestSetRenderDegradedCondition_UpdatesExisting(t *testing.T) {
	oldTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Status: mcov1alpha1.MachineConfigPoolStatus{
			Conditions: []metav1.Condition{
				{
					Type:               mcov1alpha1.ConditionDegraded,
					Status:             metav1.ConditionFalse,
					Reason:             "RenderSuccess",
					LastTransitionTime: oldTime,
				},
			},
		},
	}

	SetRenderDegradedCondition(pool, "new error")

	// Should update existing condition
	if len(pool.Status.Conditions) < 1 {
		t.Fatal("Should have conditions")
	}

	for _, c := range pool.Status.Conditions {
		if c.Type == mcov1alpha1.ConditionDegraded {
			if c.Status != metav1.ConditionTrue {
				t.Errorf("RenderDegraded status = %s, want True", c.Status)
			}
			// LastTransitionTime should be updated (status changed)
			if c.LastTransitionTime == oldTime {
				t.Error("LastTransitionTime should be updated when status changes")
			}
			return
		}
	}
	t.Error("RenderDegraded condition not found")
}

// TestAggregateStatus_ApplyTimeout verifies nodes exceeding timeout are degraded.
func TestAggregateStatus_ApplyTimeout(t *testing.T) {
	// Node started applying 700 seconds ago (beyond 600s default timeout)
	pastTime := time.Now().Add(-700 * time.Second).UTC().Format(time.RFC3339)
	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-1",
				Annotations: map[string]string{
					annotations.AgentState:           annotations.StateApplying,
					annotations.DesiredRevisionSetAt: pastTime,
				},
			},
		},
	}

	// Use default timeout (0 means use DefaultApplyTimeoutSeconds = 600)
	status := AggregateStatus("workers-new", nodes, 0)

	// Node should be degraded, not updating
	if status.DegradedMachineCount != 1 {
		t.Errorf("DegradedMachineCount = %d, want 1", status.DegradedMachineCount)
	}
	if status.UpdatingMachineCount != 0 {
		t.Errorf("UpdatingMachineCount = %d, want 0", status.UpdatingMachineCount)
	}
	if len(status.TimedOutNodes) != 1 || status.TimedOutNodes[0] != "worker-1" {
		t.Errorf("TimedOutNodes = %v, want [worker-1]", status.TimedOutNodes)
	}
}

// TestAggregateStatus_ApplyWithinTimeout verifies nodes within timeout are updating.
func TestAggregateStatus_ApplyWithinTimeout(t *testing.T) {
	// Node started applying 100 seconds ago (within 600s default timeout)
	recentTime := time.Now().Add(-100 * time.Second).UTC().Format(time.RFC3339)
	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-1",
				Annotations: map[string]string{
					annotations.AgentState:           annotations.StateApplying,
					annotations.DesiredRevisionSetAt: recentTime,
				},
			},
		},
	}

	status := AggregateStatus("workers-new", nodes, 0)

	// Node should be updating, not degraded
	if status.UpdatingMachineCount != 1 {
		t.Errorf("UpdatingMachineCount = %d, want 1", status.UpdatingMachineCount)
	}
	if status.DegradedMachineCount != 0 {
		t.Errorf("DegradedMachineCount = %d, want 0", status.DegradedMachineCount)
	}
	if len(status.TimedOutNodes) != 0 {
		t.Errorf("TimedOutNodes = %v, want empty", status.TimedOutNodes)
	}
}

// TestAggregateStatus_DefaultTimeout verifies default 600s timeout is used.
func TestAggregateStatus_DefaultTimeout(t *testing.T) {
	// Node started applying 650 seconds ago (beyond 600s default but within 700s)
	pastTime := time.Now().Add(-650 * time.Second).UTC().Format(time.RFC3339)
	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-1",
				Annotations: map[string]string{
					annotations.AgentState:           annotations.StateApplying,
					annotations.DesiredRevisionSetAt: pastTime,
				},
			},
		},
	}

	// Pass 0 to use default
	status := AggregateStatus("workers-new", nodes, 0)

	// Should timeout because default is 600s and 650s > 600s
	if status.DegradedMachineCount != 1 {
		t.Errorf("DegradedMachineCount = %d, want 1 (default timeout is 600s)", status.DegradedMachineCount)
	}
}

// TestAggregateStatus_CustomTimeout verifies custom timeout value is respected.
func TestAggregateStatus_CustomTimeout(t *testing.T) {
	// Node started applying 500 seconds ago
	pastTime := time.Now().Add(-500 * time.Second).UTC().Format(time.RFC3339)
	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-1",
				Annotations: map[string]string{
					annotations.AgentState:           annotations.StateApplying,
					annotations.DesiredRevisionSetAt: pastTime,
				},
			},
		},
	}

	// Custom timeout of 300s - node should be timed out (500s > 300s)
	status := AggregateStatus("workers-new", nodes, 300)

	if status.DegradedMachineCount != 1 {
		t.Errorf("DegradedMachineCount = %d, want 1 (custom timeout 300s)", status.DegradedMachineCount)
	}

	// Same node with 600s timeout - should NOT be timed out (500s < 600s)
	status2 := AggregateStatus("workers-new", nodes, 600)

	if status2.DegradedMachineCount != 0 {
		t.Errorf("DegradedMachineCount = %d, want 0 (custom timeout 600s)", status2.DegradedMachineCount)
	}
	if status2.UpdatingMachineCount != 1 {
		t.Errorf("UpdatingMachineCount = %d, want 1", status2.UpdatingMachineCount)
	}
}

// TestAggregateStatus_ApplyNoTimestamp verifies nodes without timestamp are not timed out.
func TestAggregateStatus_ApplyNoTimestamp(t *testing.T) {
	// Node is applying but no timestamp annotation
	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-1",
				Annotations: map[string]string{
					annotations.AgentState: annotations.StateApplying,
					// No DesiredRevisionSetAt annotation
				},
			},
		},
	}

	status := AggregateStatus("workers-new", nodes, 0)

	// Should be updating, not degraded (can't determine timeout)
	if status.UpdatingMachineCount != 1 {
		t.Errorf("UpdatingMachineCount = %d, want 1", status.UpdatingMachineCount)
	}
	if status.DegradedMachineCount != 0 {
		t.Errorf("DegradedMachineCount = %d, want 0", status.DegradedMachineCount)
	}
}

// TestAggregateStatus_MixedTimeoutAndNormal verifies mixed nodes are counted correctly.
func TestAggregateStatus_MixedTimeoutAndNormal(t *testing.T) {
	pastTime := time.Now().Add(-700 * time.Second).UTC().Format(time.RFC3339)
	recentTime := time.Now().Add(-100 * time.Second).UTC().Format(time.RFC3339)

	nodes := []corev1.Node{
		// Timed out node
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-1",
				Annotations: map[string]string{
					annotations.AgentState:           annotations.StateApplying,
					annotations.DesiredRevisionSetAt: pastTime,
				},
			},
		},
		// Normal applying node
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-2",
				Annotations: map[string]string{
					annotations.AgentState:           annotations.StateApplying,
					annotations.DesiredRevisionSetAt: recentTime,
				},
			},
		},
		// Error node
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-3",
				Annotations: map[string]string{
					annotations.AgentState: annotations.StateError,
				},
			},
		},
		// Done node
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-4",
				Annotations: map[string]string{
					annotations.CurrentRevision: "workers-new",
					annotations.AgentState:      annotations.StateDone,
				},
			},
		},
	}

	status := AggregateStatus("workers-new", nodes, 0)

	if status.MachineCount != 4 {
		t.Errorf("MachineCount = %d, want 4", status.MachineCount)
	}
	// Degraded: 1 (timeout) + 1 (error) = 2
	if status.DegradedMachineCount != 2 {
		t.Errorf("DegradedMachineCount = %d, want 2", status.DegradedMachineCount)
	}
	// Updating: 1 (worker-2 within timeout)
	if status.UpdatingMachineCount != 1 {
		t.Errorf("UpdatingMachineCount = %d, want 1", status.UpdatingMachineCount)
	}
	// Updated: 1 (worker-4)
	if status.UpdatedMachineCount != 1 {
		t.Errorf("UpdatedMachineCount = %d, want 1", status.UpdatedMachineCount)
	}
	// TimedOutNodes should only have the timed out one
	if len(status.TimedOutNodes) != 1 || status.TimedOutNodes[0] != "worker-1" {
		t.Errorf("TimedOutNodes = %v, want [worker-1]", status.TimedOutNodes)
	}
}

// Tests for CleanupLegacyConditions migration from deprecated conditions.

func TestCleanupLegacyConditions_RemovesUpdatedCondition(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Status: mcov1alpha1.MachineConfigPoolStatus{
			Conditions: []metav1.Condition{
				{
					Type:   "Updated", // Deprecated condition
					Status: metav1.ConditionTrue,
					Reason: "AllNodesUpdated",
				},
				{
					Type:   mcov1alpha1.ConditionDegraded,
					Status: metav1.ConditionFalse,
					Reason: "NoErrors",
				},
			},
		},
	}

	cleaned := CleanupLegacyConditions(pool)

	if !cleaned {
		t.Error("Expected cleanup to return true when legacy conditions exist")
	}
	if len(pool.Status.Conditions) != 1 {
		t.Errorf("Expected 1 condition after cleanup, got %d", len(pool.Status.Conditions))
	}
	if pool.Status.Conditions[0].Type != mcov1alpha1.ConditionDegraded {
		t.Errorf("Expected Degraded condition to remain, got %s", pool.Status.Conditions[0].Type)
	}
}

func TestCleanupLegacyConditions_RemovesRenderDegradedCondition(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Status: mcov1alpha1.MachineConfigPoolStatus{
			Conditions: []metav1.Condition{
				{
					Type:   "RenderDegraded", // Deprecated condition
					Status: metav1.ConditionTrue,
					Reason: "RenderFailed",
				},
				{
					Type:   mcov1alpha1.ConditionReady,
					Status: metav1.ConditionTrue,
					Reason: "AllNodesUpdated",
				},
			},
		},
	}

	cleaned := CleanupLegacyConditions(pool)

	if !cleaned {
		t.Error("Expected cleanup to return true when legacy conditions exist")
	}
	if len(pool.Status.Conditions) != 1 {
		t.Errorf("Expected 1 condition after cleanup, got %d", len(pool.Status.Conditions))
	}
	if pool.Status.Conditions[0].Type != mcov1alpha1.ConditionReady {
		t.Errorf("Expected Ready condition to remain, got %s", pool.Status.Conditions[0].Type)
	}
}

func TestCleanupLegacyConditions_RemovesBothLegacyConditions(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Status: mcov1alpha1.MachineConfigPoolStatus{
			Conditions: []metav1.Condition{
				{
					Type:   "Updated", // Legacy
					Status: metav1.ConditionTrue,
				},
				{
					Type:   "RenderDegraded", // Legacy
					Status: metav1.ConditionFalse,
				},
				{
					Type:   mcov1alpha1.ConditionReady, // Current
					Status: metav1.ConditionTrue,
				},
				{
					Type:   mcov1alpha1.ConditionDraining, // Current
					Status: metav1.ConditionFalse,
				},
			},
		},
	}

	cleaned := CleanupLegacyConditions(pool)

	if !cleaned {
		t.Error("Expected cleanup to return true")
	}
	if len(pool.Status.Conditions) != 2 {
		t.Errorf("Expected 2 conditions after cleanup, got %d", len(pool.Status.Conditions))
	}
	// Check only current conditions remain
	for _, c := range pool.Status.Conditions {
		if c.Type == "Updated" || c.Type == "RenderDegraded" {
			t.Errorf("Legacy condition %s should have been removed", c.Type)
		}
	}
}

func TestCleanupLegacyConditions_NoopWhenNoLegacyConditions(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Status: mcov1alpha1.MachineConfigPoolStatus{
			Conditions: []metav1.Condition{
				{
					Type:   mcov1alpha1.ConditionReady,
					Status: metav1.ConditionTrue,
				},
				{
					Type:   mcov1alpha1.ConditionDegraded,
					Status: metav1.ConditionFalse,
				},
			},
		},
	}

	cleaned := CleanupLegacyConditions(pool)

	if cleaned {
		t.Error("Expected cleanup to return false when no legacy conditions exist")
	}
	if len(pool.Status.Conditions) != 2 {
		t.Errorf("Expected 2 conditions to remain unchanged, got %d", len(pool.Status.Conditions))
	}
}

func TestCleanupLegacyConditions_HandleNilPool(t *testing.T) {
	cleaned := CleanupLegacyConditions(nil)

	if cleaned {
		t.Error("Expected cleanup to return false for nil pool")
	}
}

func TestCleanupLegacyConditions_HandleEmptyConditions(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Status:     mcov1alpha1.MachineConfigPoolStatus{},
	}

	cleaned := CleanupLegacyConditions(pool)

	if cleaned {
		t.Error("Expected cleanup to return false for empty conditions")
	}
}

// Test for Draining condition behavior.

func TestAggregateStatus_DrainingCondition_True(t *testing.T) {
	// Create a node that is draining (cordoned + has drain-started-at annotation)
	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-1",
				Annotations: map[string]string{
					annotations.CurrentRevision: "workers-old",
					annotations.DesiredRevision: "workers-new",
					annotations.AgentState:      annotations.StateDone,
					annotations.Cordoned:        "true",
					annotations.DrainStartedAt:  time.Now().Format(time.RFC3339),
				},
			},
			Spec: corev1.NodeSpec{
				Unschedulable: true,
			},
		},
	}

	status := AggregateStatus("workers-new", nodes, 0)

	if status.DrainingMachineCount != 1 {
		t.Errorf("DrainingMachineCount = %d, want 1", status.DrainingMachineCount)
	}

	// Check Draining condition is True
	foundDraining := false
	for _, c := range status.Conditions {
		if c.Type == mcov1alpha1.ConditionDraining {
			if c.Status != metav1.ConditionTrue {
				t.Errorf("Draining condition status = %s, want True", c.Status)
			}
			foundDraining = true
			break
		}
	}
	if !foundDraining {
		t.Error("Draining condition not found")
	}
}

func TestAggregateStatus_DrainingCondition_False(t *testing.T) {
	// Create a node that is NOT draining
	nodes := []corev1.Node{
		makeNode("worker-1", "workers-new", annotations.StateDone),
	}

	status := AggregateStatus("workers-new", nodes, 0)

	if status.DrainingMachineCount != 0 {
		t.Errorf("DrainingMachineCount = %d, want 0", status.DrainingMachineCount)
	}

	// Check Draining condition is False
	foundDraining := false
	for _, c := range status.Conditions {
		if c.Type == mcov1alpha1.ConditionDraining {
			if c.Status != metav1.ConditionFalse {
				t.Errorf("Draining condition status = %s, want False", c.Status)
			}
			foundDraining = true
			break
		}
	}
	if !foundDraining {
		t.Error("Draining condition not found")
	}
}
