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

	if len(pool.Status.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(pool.Status.Conditions))
	}

	cond := pool.Status.Conditions[0]
	if cond.Type != mcov1alpha1.ConditionDrainStuck {
		t.Errorf("expected DrainStuck condition, got %s", cond.Type)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("expected True status, got %s", cond.Status)
	}
	if cond.Reason != "DrainTimeout" {
		t.Errorf("expected DrainTimeout reason, got %s", cond.Reason)
	}
	if cond.Message != "Node test-node drain timeout" {
		t.Errorf("unexpected message: %s", cond.Message)
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

	if len(pool.Status.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(pool.Status.Conditions))
	}

	cond := pool.Status.Conditions[0]
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("expected True status after update, got %s", cond.Status)
	}
	if cond.Message != "New drain timeout" {
		t.Errorf("expected updated message, got %s", cond.Message)
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
