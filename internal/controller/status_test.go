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

	status := AggregateStatus("workers-abc", nodes)

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
		if c.Type == mcov1alpha1.ConditionUpdated && c.Status == metav1.ConditionTrue {
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

	status := AggregateStatus("workers-new", nodes)

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

	status := AggregateStatus("workers-abc", nodes)

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

	// Updating should be false when degraded (even if not all updated)
	foundUpdating := false
	for _, c := range status.Conditions {
		if c.Type == mcov1alpha1.ConditionUpdating && c.Status == metav1.ConditionTrue {
			foundUpdating = true
			break
		}
	}
	if foundUpdating {
		t.Error("Updating condition should be False when degraded")
	}
}

// TestAggregateStatus_Empty verifies status with no nodes.
func TestAggregateStatus_Empty(t *testing.T) {
	status := AggregateStatus("workers-abc", []corev1.Node{})

	if status.MachineCount != 0 {
		t.Errorf("MachineCount = %d, want 0", status.MachineCount)
	}
	if status.CurrentRevision != "workers-abc" {
		t.Errorf("CurrentRevision = %s, want workers-abc (target)", status.CurrentRevision)
	}

	// Updated should be False for empty pool
	foundUpdated := false
	for _, c := range status.Conditions {
		if c.Type == mcov1alpha1.ConditionUpdated && c.Status == metav1.ConditionFalse {
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

	status := AggregateStatus("workers-abc", nodes)

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

	status := AggregateStatus("workers-abc", nodes)

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

	if len(conditions) != 3 {
		t.Errorf("len(conditions) = %d, want 3", len(conditions))
	}

	// Find Updated condition
	var updatedCond *metav1.Condition
	for i := range conditions {
		if conditions[i].Type == mcov1alpha1.ConditionUpdated {
			updatedCond = &conditions[i]
			break
		}
	}

	if updatedCond == nil {
		t.Fatal("Updated condition not found")
	}
	if updatedCond.Status != metav1.ConditionTrue {
		t.Errorf("Updated status = %s, want True", updatedCond.Status)
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
			{Type: mcov1alpha1.ConditionUpdated, Status: metav1.ConditionFalse},
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
		{Type: mcov1alpha1.ConditionUpdated, Status: metav1.ConditionTrue, LastTransitionTime: oldTime},
		{Type: mcov1alpha1.ConditionDegraded, Status: metav1.ConditionFalse, LastTransitionTime: oldTime},
	}

	new := []metav1.Condition{
		{Type: mcov1alpha1.ConditionUpdated, Status: metav1.ConditionTrue, LastTransitionTime: newTime},  // same status
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
		{Type: mcov1alpha1.ConditionUpdated, Status: metav1.ConditionTrue, LastTransitionTime: newTime},
	}

	result := mergeConditions(existing, new)

	if len(result) != 1 {
		t.Fatalf("len(result) = %d, want 1", len(result))
	}

	if result[0].LastTransitionTime != newTime {
		t.Error("New condition should use provided LastTransitionTime")
	}
}
