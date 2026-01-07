package controller

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

func TestSetDrainStuckCondition(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
	}

	SetDrainStuckCondition(pool, "Node test-node drain timeout")

	// Should have 2 conditions: DrainStuck and Degraded (per FR-009-AC6)
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

	// Check Degraded condition is also set (FR-009-AC6)
	if degraded == nil {
		t.Fatal("expected Degraded condition (FR-009-AC6)")
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
// with a different reason is not overridden (AC-3).
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

	if len(pool.Status.Conditions) != 0 {
		t.Errorf("expected 0 conditions when clearing non-existent, got %d", len(pool.Status.Conditions))
	}
}
