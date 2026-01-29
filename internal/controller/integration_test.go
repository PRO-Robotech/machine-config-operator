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
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/pkg/annotations"
)

// newIntegrationReconciler creates a reconciler with pod index for drain operations
func newIntegrationReconciler(objs ...client.Object) *MachineConfigPoolReconciler {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = mcov1alpha1.AddToScheme(scheme)

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&mcov1alpha1.MachineConfigPool{}).
		WithIndex(&corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string {
			pod := obj.(*corev1.Pod)
			return []string{pod.Spec.NodeName}
		}).
		Build()

	return NewMachineConfigPoolReconciler(c, scheme)
}

func reconcileN(r *MachineConfigPoolReconciler, name string, n int) error {
	ctx := context.Background()
	for i := 0; i < n; i++ {
		_, err := r.Reconcile(ctx, ctrl.Request{
			NamespacedName: client.ObjectKey{Name: name},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func getNode(t *testing.T, r *MachineConfigPoolReconciler, name string) *corev1.Node {
	t.Helper()
	node := &corev1.Node{}
	if err := r.Get(context.Background(), client.ObjectKey{Name: name}, node); err != nil {
		t.Fatalf("Failed to get node %s: %v", name, err)
	}
	return node
}

func getPool(t *testing.T, r *MachineConfigPoolReconciler, name string) *mcov1alpha1.MachineConfigPool {
	t.Helper()
	pool := &mcov1alpha1.MachineConfigPool{}
	if err := r.Get(context.Background(), client.ObjectKey{Name: name}, pool); err != nil {
		t.Fatalf("Failed to get pool %s: %v", name, err)
	}
	return pool
}

func countCordonedNodesIntegration(t *testing.T, r *MachineConfigPoolReconciler) int {
	t.Helper()
	nodeList := &corev1.NodeList{}
	if err := r.List(context.Background(), nodeList); err != nil {
		t.Fatalf("Failed to list nodes: %v", err)
	}
	count := 0
	for _, node := range nodeList.Items {
		if node.Spec.Unschedulable {
			count++
		}
	}
	return count
}

// TestRollingUpdate_MaxUnavailable1 tests sequential update with maxUnavailable=1
func TestRollingUpdate_MaxUnavailable1(t *testing.T) {
	maxUnavailable := intstr.FromInt(1)
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"role": "worker"},
			},
			Rollout: mcov1alpha1.RolloutConfig{
				DebounceSeconds: 0,
				MaxUnavailable:  &maxUnavailable,
			},
		},
	}

	nodes := []client.Object{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1", Labels: map[string]string{"role": "worker"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-2", Labels: map[string]string{"role": "worker"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-3", Labels: map[string]string{"role": "worker"}}},
	}

	mc := &mcov1alpha1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mc-1"},
		Spec:       mcov1alpha1.MachineConfigSpec{Priority: 50},
	}

	allObjs := append([]client.Object{pool, mc}, nodes...)
	r := newIntegrationReconciler(allObjs...)

	// First reconcile triggers debounce check
	if err := reconcileN(r, "worker", 1); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Second reconcile starts processing
	if err := reconcileN(r, "worker", 1); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Check that exactly 1 node is cordoned (maxUnavailable=1)
	cordoned := countCordonedNodesIntegration(t, r)
	if cordoned != 1 {
		t.Errorf("expected 1 cordoned node, got %d", cordoned)
	}
}

// TestRollingUpdate_MaxUnavailable2 tests parallel update with maxUnavailable=2
func TestRollingUpdate_MaxUnavailable2(t *testing.T) {
	maxUnavailable := intstr.FromInt(2)
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"role": "worker"},
			},
			Rollout: mcov1alpha1.RolloutConfig{
				DebounceSeconds: 0,
				MaxUnavailable:  &maxUnavailable,
			},
		},
	}

	nodes := []client.Object{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1", Labels: map[string]string{"role": "worker"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-2", Labels: map[string]string{"role": "worker"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-3", Labels: map[string]string{"role": "worker"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-4", Labels: map[string]string{"role": "worker"}}},
	}

	mc := &mcov1alpha1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mc-1"},
		Spec:       mcov1alpha1.MachineConfigSpec{Priority: 50},
	}

	allObjs := append([]client.Object{pool, mc}, nodes...)
	r := newIntegrationReconciler(allObjs...)

	if err := reconcileN(r, "worker", 2); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Check that exactly 2 nodes are cordoned (maxUnavailable=2)
	cordoned := countCordonedNodesIntegration(t, r)
	if cordoned != 2 {
		t.Errorf("expected 2 cordoned nodes, got %d", cordoned)
	}
}

// TestRollingUpdate_Percentage tests percentage-based maxUnavailable
func TestRollingUpdate_Percentage(t *testing.T) {
	maxUnavailable := intstr.FromString("25%")
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"role": "worker"},
			},
			Rollout: mcov1alpha1.RolloutConfig{
				DebounceSeconds: 0,
				MaxUnavailable:  &maxUnavailable,
			},
		},
	}

	// 8 nodes, 25% = 2 nodes
	nodes := make([]client.Object, 8)
	for i := 0; i < 8; i++ {
		nodes[i] = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "node-" + string(rune('1'+i)),
				Labels: map[string]string{"role": "worker"},
			},
		}
	}

	mc := &mcov1alpha1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mc-1"},
		Spec:       mcov1alpha1.MachineConfigSpec{Priority: 50},
	}

	allObjs := append([]client.Object{pool, mc}, nodes...)
	r := newIntegrationReconciler(allObjs...)

	if err := reconcileN(r, "worker", 2); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// 25% of 8 = 2 nodes
	cordoned := countCordonedNodesIntegration(t, r)
	if cordoned != 2 {
		t.Errorf("expected 2 cordoned nodes (25%% of 8), got %d", cordoned)
	}
}

// TestCordonDrain_Flow tests the cordon -> drain -> set desired-revision flow
func TestCordonDrain_Flow(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"role": "worker"},
			},
			Rollout: mcov1alpha1.RolloutConfig{
				DebounceSeconds: 0,
			},
		},
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-1",
			Labels: map[string]string{"role": "worker"},
		},
	}

	mc := &mcov1alpha1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mc-1"},
		Spec:       mcov1alpha1.MachineConfigSpec{Priority: 50},
	}

	r := newIntegrationReconciler(pool, node, mc)

	// First reconcile - debounce
	if err := reconcileN(r, "worker", 1); err != nil {
		t.Fatalf("Reconcile 1 failed: %v", err)
	}

	// Second reconcile - cordon node
	if err := reconcileN(r, "worker", 1); err != nil {
		t.Fatalf("Reconcile 2 failed: %v", err)
	}

	// Verify node is cordoned
	updatedNode := getNode(t, r, "worker-1")
	if !updatedNode.Spec.Unschedulable {
		t.Error("node should be cordoned (unschedulable)")
	}
	if updatedNode.Annotations[annotations.Cordoned] != "true" {
		t.Error("cordoned annotation should be set")
	}

	// Continue reconciles - drain completes (no pods), desired-revision set
	// Need more reconciles as each step in ProcessNodeUpdate returns with requeue
	if err := reconcileN(r, "worker", 10); err != nil {
		t.Fatalf("Reconcile 3-12 failed: %v", err)
	}

	updatedNode = getNode(t, r, "worker-1")
	if updatedNode.Annotations[annotations.DesiredRevision] == "" {
		t.Error("desired-revision should be set after drain")
	}

	// Verify the full flow: node should still be cordoned until agent completes
	if !updatedNode.Spec.Unschedulable {
		t.Error("node should remain cordoned until agent completes")
	}
}

// TestUncordon_AfterAgentDone tests that ShouldUncordon returns true when agent is done
func TestUncordon_AfterAgentDone(t *testing.T) {
	// This is a unit test for the ShouldUncordon function
	// The integration flow is tested in TestCordonDrain_Flow

	targetRevision := "worker-abc123"
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-1",
			Annotations: map[string]string{
				annotations.Cordoned:        "true",
				annotations.DesiredRevision: targetRevision,
				annotations.CurrentRevision: targetRevision,
				annotations.AgentState:      "done",
			},
		},
		Spec: corev1.NodeSpec{
			Unschedulable: true,
		},
	}

	if !ShouldUncordon(node, targetRevision) {
		t.Error("ShouldUncordon should return true when current==desired and state==done")
	}

	// Test negative case - different revision
	if ShouldUncordon(node, "different-revision") {
		t.Error("ShouldUncordon should return false when target differs")
	}

	// Test that agent state is NOT checked for uncordon decision.
	// When revision matches, node should uncordon regardless of agent state (idle/applying/done).
	node.Annotations[annotations.AgentState] = "applying"
	if !ShouldUncordon(node, targetRevision) {
		t.Error("ShouldUncordon should return true when revision matches, regardless of state")
	}

	node.Annotations[annotations.AgentState] = "idle"
	if !ShouldUncordon(node, targetRevision) {
		t.Error("ShouldUncordon should return true when revision matches, regardless of state")
	}
}

// TestDrainStuck_Condition tests HandleDrainRetry returns stuck=true on timeout
func TestDrainStuck_Condition(t *testing.T) {
	// Unit test for HandleDrainRetry drain stuck detection
	// Full integration is complex because RMC name is computed dynamically

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = mcov1alpha1.AddToScheme(scheme)

	// Node with drain started 2 hours ago
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-1",
			Annotations: map[string]string{
				annotations.DrainStartedAt: time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
				annotations.Cordoned:       "true",
			},
		},
		Spec: corev1.NodeSpec{
			Unschedulable: true,
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(node).
		Build()

	ctx := context.Background()
	drainTimeoutSeconds := 60 // 1 minute

	result := HandleDrainRetry(ctx, c, node, drainTimeoutSeconds, 0)

	if !result.SetDrainStuck {
		t.Error("HandleDrainRetry should return SetDrainStuck=true when timeout exceeded")
	}

	// Verify DrainRetryCount was incremented
	updatedNode := &corev1.Node{}
	if err := c.Get(ctx, client.ObjectKey{Name: "worker-1"}, updatedNode); err != nil {
		t.Fatalf("Failed to get node: %v", err)
	}

	retryCount := GetIntAnnotation(updatedNode, annotations.DrainRetryCount)
	if retryCount != 1 {
		t.Errorf("expected DrainRetryCount=1, got %d", retryCount)
	}
}

// TestSetDrainStuckCondition_Integration tests SetDrainStuckCondition on pool.
// Sets both DrainStuck=True and Degraded=True conditions.
func TestSetDrainStuckCondition_Integration(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
	}

	SetDrainStuckCondition(pool, "Node worker-1 drain timeout")

	// Should have 2 conditions: DrainStuck + Degraded
	if len(pool.Status.Conditions) != 2 {
		t.Fatalf("expected 2 conditions (DrainStuck + Degraded), got %d", len(pool.Status.Conditions))
	}

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

// TestPoolOverlap_BlocksUpdate tests that nodes matching multiple pools are not updated
func TestPoolOverlap_BlocksUpdate(t *testing.T) {
	// Create two pools that both select the same node via different labels
	pool1 := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "workers"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"role": "worker"},
			},
			Rollout: mcov1alpha1.RolloutConfig{
				DebounceSeconds: 0,
			},
		},
	}

	pool2 := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "infra"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"infra": "true"},
			},
			Rollout: mcov1alpha1.RolloutConfig{
				DebounceSeconds: 0,
			},
		},
	}

	// Node with both labels - matches BOTH pools (overlap)
	overlappingNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "overlap-node",
			Labels: map[string]string{"role": "worker", "infra": "true"},
		},
	}

	// Node with only worker label - non-overlapping
	workerNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-node",
			Labels: map[string]string{"role": "worker"},
		},
	}

	mc := &mcov1alpha1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mc-1"},
		Spec:       mcov1alpha1.MachineConfigSpec{Priority: 50},
	}

	r := newIntegrationReconciler(pool1, pool2, overlappingNode, workerNode, mc)

	if err := reconcileN(r, "workers", 3); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Worker-only node should be cordoned (non-overlapping)
	workerUpdated := getNode(t, r, "worker-node")
	if !workerUpdated.Spec.Unschedulable {
		t.Error("worker-node should be cordoned")
	}

	// Overlapping node should NOT be cordoned (conflict detected)
	overlapUpdated := getNode(t, r, "overlap-node")
	if overlapUpdated.Spec.Unschedulable {
		t.Error("overlap-node should NOT be cordoned due to pool conflict")
	}

	// Overlap detection should set condition on pool
	updatedPool := getPool(t, r, "workers")
	var overlapCondition *metav1.Condition
	for i := range updatedPool.Status.Conditions {
		if updatedPool.Status.Conditions[i].Type == mcov1alpha1.ConditionPoolOverlap {
			overlapCondition = &updatedPool.Status.Conditions[i]
			break
		}
	}

	if overlapCondition == nil {
		t.Error("PoolOverlap condition should be present")
	} else if overlapCondition.Status != metav1.ConditionTrue {
		t.Errorf("PoolOverlap condition should be True, got %s", overlapCondition.Status)
	}
}

// TestStatus_UpdatedCorrectly tests that pool status reflects node states
func TestStatus_UpdatedCorrectly(t *testing.T) {
	maxUnavailable := intstr.FromInt(1)
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"role": "worker"},
			},
			Rollout: mcov1alpha1.RolloutConfig{
				DebounceSeconds: 0,
				MaxUnavailable:  &maxUnavailable,
			},
		},
	}

	nodes := []client.Object{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1", Labels: map[string]string{"role": "worker"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-2", Labels: map[string]string{"role": "worker"}}},
	}

	mc := &mcov1alpha1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mc-1"},
		Spec:       mcov1alpha1.MachineConfigSpec{Priority: 50},
	}

	allObjs := append([]client.Object{pool, mc}, nodes...)
	r := newIntegrationReconciler(allObjs...)

	// Need 3 reconciles: debounce, then cordon, then status update
	if err := reconcileN(r, "worker", 3); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	updatedPool := getPool(t, r, "worker")

	// Check MachineCount
	if updatedPool.Status.MachineCount != 2 {
		t.Errorf("expected MachineCount=2, got %d", updatedPool.Status.MachineCount)
	}

	// Check TargetRevision is set
	if updatedPool.Status.TargetRevision == "" {
		t.Error("TargetRevision should be set")
	}

	// Check CordonedMachineCount (1 node should be cordoned with maxUnavailable=1)
	if updatedPool.Status.CordonedMachineCount != 1 {
		t.Errorf("expected CordonedMachineCount=1, got %d", updatedPool.Status.CordonedMachineCount)
	}
}

// TestRequeueAfter_Aggregation tests that minimum RequeueAfter is returned
func TestRequeueAfter_Aggregation(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"role": "worker"},
			},
			Rollout: mcov1alpha1.RolloutConfig{
				DebounceSeconds: 0,
			},
		},
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-1",
			Labels: map[string]string{"role": "worker"},
		},
	}

	mc := &mcov1alpha1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mc-1"},
		Spec:       mcov1alpha1.MachineConfigSpec{Priority: 50},
	}

	r := newIntegrationReconciler(pool, node, mc)

	// Skip debounce
	r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Name: "worker"},
	})

	// Second reconcile should have requeue for node update lifecycle
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Name: "worker"},
	})

	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Should requeue for node update progress
	if result.RequeueAfter == 0 {
		t.Error("expected non-zero RequeueAfter for in-progress updates")
	}
}

// TestDrainBeforeApply_NoDesiredRevisionWhileDraining verifies that desired-revision
// is NOT set while pods are still being drained from the node.
func TestDrainBeforeApply_NoDesiredRevisionWhileDraining(t *testing.T) {
	maxUnavailable := intstr.FromInt(1)
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker",
		},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"node-role.kubernetes.io/worker": ""},
			},
			MachineConfigSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"mco.in-cloud.io/pool": "worker"},
			},
			Rollout: mcov1alpha1.RolloutConfig{
				DebounceSeconds: 0,
				MaxUnavailable:  &maxUnavailable,
			},
		},
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-1",
			Labels: map[string]string{
				"node-role.kubernetes.io/worker": "",
			},
		},
	}

	mc := &mcov1alpha1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "mc-drain-test",
			Labels: map[string]string{"mco.in-cloud.io/pool": "worker"},
		},
		Spec: mcov1alpha1.MachineConfigSpec{Priority: 50},
	}

	// Create a pod on the node that will prevent drain from completing
	// This pod is owned by a ReplicaSet so it's not a DaemonSet/static pod
	controllerTrue := true
	blockingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blocking-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       "test-rs",
					UID:        "test-uid",
					Controller: &controllerTrue, // Required for HasController() to return true
				},
			},
		},
		Spec: corev1.PodSpec{
			NodeName: "worker-1",
			Containers: []corev1.Container{
				{Name: "test", Image: "busybox"},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	r := newIntegrationReconciler(pool, node, mc, blockingPod)

	// First reconcile - debounce/RMC creation
	if err := reconcileN(r, "worker", 1); err != nil {
		t.Fatalf("Reconcile 1 failed: %v", err)
	}

	// Second reconcile - cordon node
	if err := reconcileN(r, "worker", 1); err != nil {
		t.Fatalf("Reconcile 2 failed: %v", err)
	}

	// Verify node is cordoned
	updatedNode := getNode(t, r, "worker-1")
	if !updatedNode.Spec.Unschedulable {
		t.Error("node should be cordoned (unschedulable)")
	}

	// Multiple reconciles - drain should be attempted but pod blocks completion
	// The key assertion: desired-revision should NOT be set while pod is still present
	//
	// NOTE: The fake client immediately deletes pods on eviction, so we need to
	// recreate the pod after each reconcile to simulate a pod that is being
	// evicted but hasn't terminated yet (e.g., slow graceful shutdown, PDB blocked).
	for i := 0; i < 3; i++ {
		if err := reconcileN(r, "worker", 1); err != nil {
			t.Fatalf("Reconcile %d failed: %v", i+3, err)
		}

		// Recreate the pod to simulate it still being present (graceful termination)
		newPod := blockingPod.DeepCopy()
		newPod.Name = fmt.Sprintf("blocking-pod-%d", i)
		newPod.ResourceVersion = ""
		if err := r.Create(context.Background(), newPod); err != nil {
			t.Fatalf("Failed to recreate blocking pod: %v", err)
		}

		// After each reconcile, verify desired-revision is NOT set
		// (because drain is not complete due to blocking pod being recreated)
		updatedNode = getNode(t, r, "worker-1")
		if updatedNode.Annotations[annotations.DesiredRevision] != "" {
			t.Fatalf("REGRESSION: desired-revision was set while drain is incomplete (reconcile %d)", i+3)
		}
	}

	// Delete all pods to allow drain to complete
	// (the original was already evicted, delete the recreated ones)
	podList := &corev1.PodList{}
	if err := r.List(context.Background(), podList); err != nil {
		t.Fatalf("Failed to list pods: %v", err)
	}
	for _, pod := range podList.Items {
		_ = r.Delete(context.Background(), &pod) // Ignore errors for already-deleted pods
	}

	// More reconciles - now drain should complete and desired-revision should be set
	if err := reconcileN(r, "worker", 5); err != nil {
		t.Fatalf("Reconcile after pod deletion failed: %v", err)
	}

	updatedNode = getNode(t, r, "worker-1")
	if updatedNode.Annotations[annotations.DesiredRevision] == "" {
		t.Error("desired-revision should be set after drain completes")
	}

	t.Log("Drain-before-apply semantics verified: desired-revision only set after drain complete")
}
