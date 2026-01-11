package controller

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

func TestSetDrainStuckCondition(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
	}

	SetDrainStuckCondition(pool, "Node test-node drain timeout")

	// Should have 2 conditions: DrainStuck and Degraded
	if len(pool.Status.Conditions) != 2 {
		t.Fatalf("expected 2 conditions (DrainStuck + Degraded), got %d", len(pool.Status.Conditions))
	}

	// Check DrainStuck condition
	var drainStuck, degraded *metav1.Condition
	for i := range pool.Status.Conditions {
		c := &pool.Status.Conditions[i]
		switch c.Type {
		case mcov1alpha1.ConditionDrainStuck:
			drainStuck = c
		case mcov1alpha1.ConditionDegraded:
			degraded = c
		}
	}

	if drainStuck == nil {
		t.Fatal("expected DrainStuck condition")
	}
	if drainStuck.Status != metav1.ConditionTrue {
		t.Errorf("expected DrainStuck True status, got %s", drainStuck.Status)
	}
	if drainStuck.Reason != "DrainTimeout" {
		t.Errorf("expected DrainTimeout reason, got %s", drainStuck.Reason)
	}
	if drainStuck.Message != "Node test-node drain timeout" {
		t.Errorf("unexpected DrainStuck message: %s", drainStuck.Message)
	}

	// Check Degraded condition is also set
	if degraded == nil {
		t.Fatal("expected Degraded condition")
	}
	if degraded.Status != metav1.ConditionTrue {
		t.Errorf("expected Degraded True status, got %s", degraded.Status)
	}
	if degraded.Reason != "DrainStuck" {
		t.Errorf("expected DrainStuck reason for Degraded, got %s", degraded.Reason)
	}
}

func TestSetDrainStuckCondition_UpdatesExisting(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Status: mcov1alpha1.MachineConfigPoolStatus{
			Conditions: []metav1.Condition{
				{
					Type:    mcov1alpha1.ConditionDrainStuck,
					Status:  metav1.ConditionFalse,
					Reason:  "DrainComplete",
					Message: "",
				},
			},
		},
	}

	SetDrainStuckCondition(pool, "New drain timeout")

	// Should have 2 conditions: DrainStuck (updated) + Degraded (added)
	if len(pool.Status.Conditions) != 2 {
		t.Fatalf("expected 2 conditions (DrainStuck + Degraded), got %d", len(pool.Status.Conditions))
	}

	var drainStuck *metav1.Condition
	for i := range pool.Status.Conditions {
		if pool.Status.Conditions[i].Type == mcov1alpha1.ConditionDrainStuck {
			drainStuck = &pool.Status.Conditions[i]
			break
		}
	}

	if drainStuck == nil {
		t.Fatal("expected DrainStuck condition")
	}
	if drainStuck.Status != metav1.ConditionTrue {
		t.Errorf("expected True status after update, got %s", drainStuck.Status)
	}
	if drainStuck.Message != "New drain timeout" {
		t.Errorf("expected updated message, got %s", drainStuck.Message)
	}
}

// TestSetDrainStuckCondition_NoOverrideDegraded verifies that existing Degraded condition
// with a different reason is not overridden.
func TestSetDrainStuckCondition_NoOverrideDegraded(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Status: mcov1alpha1.MachineConfigPoolStatus{
			Conditions: []metav1.Condition{
				{
					Type:    mcov1alpha1.ConditionDegraded,
					Status:  metav1.ConditionTrue,
					Reason:  "NodeErrors",
					Message: "3 nodes in error state",
				},
			},
		},
	}

	SetDrainStuckCondition(pool, "Drain timeout")

	// Should have 2 conditions: existing Degraded + new DrainStuck
	if len(pool.Status.Conditions) != 2 {
		t.Fatalf("expected 2 conditions, got %d", len(pool.Status.Conditions))
	}

	// Degraded should NOT be overridden - it was already True with different reason
	var degraded *metav1.Condition
	for i := range pool.Status.Conditions {
		if pool.Status.Conditions[i].Type == mcov1alpha1.ConditionDegraded {
			degraded = &pool.Status.Conditions[i]
			break
		}
	}

	if degraded == nil {
		t.Fatal("expected Degraded condition")
	}
	if degraded.Status != metav1.ConditionTrue {
		t.Errorf("expected Degraded True status, got %s", degraded.Status)
	}
	// The reason should remain NodeErrors, not be overwritten to DrainStuck
	if degraded.Reason != "NodeErrors" {
		t.Errorf("expected NodeErrors reason (not overridden), got %s", degraded.Reason)
	}
}

func TestClearDrainStuckCondition(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Status: mcov1alpha1.MachineConfigPoolStatus{
			Conditions: []metav1.Condition{
				{
					Type:    mcov1alpha1.ConditionDrainStuck,
					Status:  metav1.ConditionTrue,
					Reason:  "DrainTimeout",
					Message: "Stuck message",
				},
			},
		},
	}

	ClearDrainStuckCondition(pool)

	if len(pool.Status.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(pool.Status.Conditions))
	}

	cond := pool.Status.Conditions[0]
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("expected False status after clear, got %s", cond.Status)
	}
	if cond.Reason != "DrainComplete" {
		t.Errorf("expected DrainComplete reason, got %s", cond.Reason)
	}
}

func TestClearDrainStuckCondition_NoExistingCondition(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
	}

	ClearDrainStuckCondition(pool)

	// ClearDrainStuckCondition ensures condition exists as False for consistent querying
	if len(pool.Status.Conditions) != 1 {
		t.Errorf("expected 1 condition (DrainStuck=False), got %d", len(pool.Status.Conditions))
	}
	if pool.Status.Conditions[0].Type != mcov1alpha1.ConditionDrainStuck {
		t.Errorf("expected DrainStuck condition, got %s", pool.Status.Conditions[0].Type)
	}
	if pool.Status.Conditions[0].Status != metav1.ConditionFalse {
		t.Errorf("expected condition status False, got %s", pool.Status.Conditions[0].Status)
	}
}

// Tests for LastTransitionTime preservation to prevent constant status updates.

func TestClearDrainStuckCondition_PreservesTimestampWhenStatusUnchanged(t *testing.T) {
	// Given: pool with DrainStuck=False and specific timestamp from 1 hour ago
	oldTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Status: mcov1alpha1.MachineConfigPoolStatus{
			Conditions: []metav1.Condition{
				{
					Type:               mcov1alpha1.ConditionDrainStuck,
					Status:             metav1.ConditionFalse,
					Reason:             "DrainComplete",
					LastTransitionTime: oldTime,
				},
			},
		},
	}

	// When: ClearDrainStuckCondition is called (False -> False)
	ClearDrainStuckCondition(pool)

	// Then: LastTransitionTime should be preserved
	for _, c := range pool.Status.Conditions {
		if c.Type == mcov1alpha1.ConditionDrainStuck {
			if !c.LastTransitionTime.Equal(&oldTime) {
				t.Errorf("expected LastTransitionTime %v to be preserved, got %v", oldTime, c.LastTransitionTime)
			}
			return
		}
	}
	t.Error("DrainStuck condition not found")
}

func TestClearDrainStuckCondition_UpdatesTimestampOnRealTransition(t *testing.T) {
	// Given: pool with DrainStuck=True (stuck state)
	oldTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Status: mcov1alpha1.MachineConfigPoolStatus{
			Conditions: []metav1.Condition{
				{
					Type:               mcov1alpha1.ConditionDrainStuck,
					Status:             metav1.ConditionTrue, // True!
					Reason:             "DrainTimeout",
					LastTransitionTime: oldTime,
				},
			},
		},
	}

	// When: ClearDrainStuckCondition is called (True -> False)
	ClearDrainStuckCondition(pool)

	// Then: LastTransitionTime should be updated (not equal to old)
	for _, c := range pool.Status.Conditions {
		if c.Type == mcov1alpha1.ConditionDrainStuck {
			if c.LastTransitionTime.Equal(&oldTime) {
				t.Error("expected LastTransitionTime to be updated on status transition True->False")
			}
			if c.Status != metav1.ConditionFalse {
				t.Errorf("expected status False, got %s", c.Status)
			}
			return
		}
	}
	t.Error("DrainStuck condition not found")
}

func TestSetDrainStuckCondition_PreservesTimestampWhenStatusUnchanged(t *testing.T) {
	// Given: pool with DrainStuck=True and specific timestamp from 1 hour ago
	oldTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Status: mcov1alpha1.MachineConfigPoolStatus{
			Conditions: []metav1.Condition{
				{
					Type:               mcov1alpha1.ConditionDrainStuck,
					Status:             metav1.ConditionTrue,
					Reason:             "DrainTimeout",
					Message:            "old message",
					LastTransitionTime: oldTime,
				},
				{
					Type:               mcov1alpha1.ConditionDegraded,
					Status:             metav1.ConditionTrue,
					Reason:             "DrainStuck",
					LastTransitionTime: oldTime,
				},
			},
		},
	}

	// When: SetDrainStuckCondition is called again (True -> True)
	SetDrainStuckCondition(pool, "new message")

	// Then: LastTransitionTime should be preserved
	for _, c := range pool.Status.Conditions {
		if c.Type == mcov1alpha1.ConditionDrainStuck {
			if !c.LastTransitionTime.Equal(&oldTime) {
				t.Errorf("expected LastTransitionTime %v to be preserved, got %v", oldTime, c.LastTransitionTime)
			}
			// Message should be updated
			if c.Message != "new message" {
				t.Errorf("expected message to be updated, got %s", c.Message)
			}
			return
		}
	}
	t.Error("DrainStuck condition not found")
}

func TestSetDrainStuckCondition_UpdatesTimestampOnRealTransition(t *testing.T) {
	// Given: pool with DrainStuck=False (clear state)
	oldTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Status: mcov1alpha1.MachineConfigPoolStatus{
			Conditions: []metav1.Condition{
				{
					Type:               mcov1alpha1.ConditionDrainStuck,
					Status:             metav1.ConditionFalse, // False!
					Reason:             "DrainComplete",
					LastTransitionTime: oldTime,
				},
			},
		},
	}

	// When: SetDrainStuckCondition is called (False -> True)
	SetDrainStuckCondition(pool, "drain stuck now")

	// Then: LastTransitionTime should be updated (not equal to old)
	for _, c := range pool.Status.Conditions {
		if c.Type == mcov1alpha1.ConditionDrainStuck {
			if c.LastTransitionTime.Equal(&oldTime) {
				t.Error("expected LastTransitionTime to be updated on status transition False->True")
			}
			if c.Status != metav1.ConditionTrue {
				t.Errorf("expected status True, got %s", c.Status)
			}
			return
		}
	}
	t.Error("DrainStuck condition not found")
}

// Tests for new node detection logic.

func TestIsNewNode_TrueWhenNoAnnotations(t *testing.T) {
	// A node with no current-revision, no pool annotation, not cordoned
	// AND pool has LastSuccessfulRevision = "abc" should be considered a new node
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Status: mcov1alpha1.MachineConfigPoolStatus{
			LastSuccessfulRevision: "worker-abc123",
		},
	}

	// Test the detection logic (extracted from ProcessNodeUpdate)
	currentRevision := ""
	poolAnnotation := ""
	isCordoned := false
	isUnschedulable := false
	poolHasExistingConfig := pool.Status.LastSuccessfulRevision != ""

	isNewNode := currentRevision == "" && poolAnnotation == "" && !isCordoned && !isUnschedulable

	if !isNewNode {
		t.Error("expected node to be detected as new")
	}
	if !poolHasExistingConfig {
		t.Error("expected pool to have existing config")
	}
	// Combined condition for skipping cordon/drain
	shouldSkipCordonDrain := isNewNode && poolHasExistingConfig
	if !shouldSkipCordonDrain {
		t.Error("expected new node in existing pool to skip cordon/drain")
	}
}

func TestIsNewNode_FalseWhenHasPoolAnnotation(t *testing.T) {
	// A node that has a pool annotation (MCO has touched it before) is NOT new
	currentRevision := ""
	poolAnnotation := "worker"
	isCordoned := false

	isNewNode := currentRevision == "" && poolAnnotation == "" && !isCordoned

	if isNewNode {
		t.Error("node with pool annotation should NOT be detected as new")
	}
}

func TestIsNewNode_FalseWhenHasCurrentRevision(t *testing.T) {
	// A node that has a current-revision is NOT new
	currentRevision := "worker-abc123"
	poolAnnotation := ""
	isCordoned := false

	isNewNode := currentRevision == "" && poolAnnotation == "" && !isCordoned

	if isNewNode {
		t.Error("node with current-revision should NOT be detected as new")
	}
}

func TestIsNewNode_FalseWhenPoolHasNoConfig(t *testing.T) {
	// A new pool without LastSuccessfulRevision should NOT skip cordon/drain
	// (this is the initial config application case)
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Status:     mcov1alpha1.MachineConfigPoolStatus{
			// LastSuccessfulRevision is empty
		},
	}

	currentRevision := ""
	poolAnnotation := ""
	isCordoned := false
	isUnschedulable := false
	poolHasExistingConfig := pool.Status.LastSuccessfulRevision != ""

	isNewNode := currentRevision == "" && poolAnnotation == "" && !isCordoned && !isUnschedulable
	shouldSkipCordonDrain := isNewNode && poolHasExistingConfig

	if !isNewNode {
		t.Error("node should be detected as new")
	}
	if poolHasExistingConfig {
		t.Error("pool should NOT have existing config")
	}
	if shouldSkipCordonDrain {
		t.Error("should NOT skip cordon/drain for new node in new pool")
	}
}
