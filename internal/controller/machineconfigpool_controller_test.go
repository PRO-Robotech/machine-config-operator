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
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/internal/renderer"
	"in-cloud.io/machine-config/pkg/annotations"
)

func newReconciler(objs ...client.Object) *MachineConfigPoolReconciler {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = mcov1alpha1.AddToScheme(scheme)

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&mcov1alpha1.MachineConfigPool{}).
		// Add pod index for drain operations
		WithIndex(&corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string {
			pod := obj.(*corev1.Pod)
			return []string{pod.Spec.NodeName}
		}).
		Build()

	return NewMachineConfigPoolReconciler(c, scheme)
}

func TestNewMachineConfigPoolReconciler(t *testing.T) {
	r := newReconciler()

	if r == nil {
		t.Fatal("NewMachineConfigPoolReconciler() returned nil")
	}

	if r.debounce == nil {
		t.Error("debounce is nil")
	}
	if r.annotator == nil {
		t.Error("annotator is nil")
	}
	if r.cleaner == nil {
		t.Error("cleaner is nil")
	}
}

func TestReconcile_PoolNotFound(t *testing.T) {
	r := newReconciler()

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Name: "nonexistent"},
	})

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	if result.Requeue || result.RequeueAfter != 0 {
		t.Error("Reconcile() should not requeue for deleted pool")
	}
}

func TestReconcile_PoolPaused(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Paused: true,
		},
	}

	r := newReconciler(pool)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Name: "worker"},
	})

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	if result.Requeue || result.RequeueAfter != 0 {
		t.Error("Reconcile() should not requeue for paused pool")
	}
}

func TestReconcile_Debounce(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Rollout: mcov1alpha1.RolloutConfig{
				DebounceSeconds: 30,
			},
		},
	}

	mc := &mcov1alpha1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mc-1"},
		Spec: mcov1alpha1.MachineConfigSpec{
			Priority: 50,
		},
	}

	r := newReconciler(pool, mc)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Name: "worker"},
	})

	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if result.RequeueAfter == 0 {
		t.Error("First reconcile should requeue for debounce")
	}

	if result.RequeueAfter > 30*time.Second {
		t.Errorf("RequeueAfter = %v, should be <= 30s", result.RequeueAfter)
	}
}

func TestReconcile_CreatesRMC(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Rollout: mcov1alpha1.RolloutConfig{
				DebounceSeconds: 0, // No debounce for test
			},
		},
	}

	mc := &mcov1alpha1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mc-1"},
		Spec: mcov1alpha1.MachineConfigSpec{
			Priority: 50,
			Files: []mcov1alpha1.FileSpec{
				{Path: "/etc/test.conf", Content: "test"},
			},
		},
	}

	r := newReconciler(pool, mc)

	r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Name: "worker"},
	})

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Name: "worker"},
	})

	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if result.RequeueAfter != 0 {
		t.Errorf("Second reconcile should not requeue, got %v", result.RequeueAfter)
	}

	rmcList := &mcov1alpha1.RenderedMachineConfigList{}
	if err := r.List(context.Background(), rmcList); err != nil {
		t.Fatalf("Failed to list RMCs: %v", err)
	}

	if len(rmcList.Items) != 1 {
		t.Errorf("RMC count = %d, want 1", len(rmcList.Items))
	}
}

func TestReconcile_SetsNodeAnnotations(t *testing.T) {
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
		Spec: mcov1alpha1.MachineConfigSpec{
			Priority: 50,
		},
	}

	r := newReconciler(pool, node, mc)

	// First reconcile - debounce (since config changes)
	r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Name: "worker"},
	})

	// Second reconcile - node gets cordoned
	r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Name: "worker"},
	})

	// Check node is cordoned
	updatedNode := &corev1.Node{}
	if err := r.Get(context.Background(), client.ObjectKey{Name: "worker-1"}, updatedNode); err != nil {
		t.Fatalf("Failed to get node: %v", err)
	}

	if !updatedNode.Spec.Unschedulable {
		t.Error("node should be cordoned (unschedulable)")
	}

	cordoned := updatedNode.Annotations[annotations.Cordoned]
	if cordoned != "true" {
		t.Errorf("cordoned annotation = %q, want %q", cordoned, "true")
	}

	// Run additional reconciles - drain completes (no pods), desired-revision set
	// May need multiple reconciles as each step in ProcessNodeUpdate returns with requeue
	for i := 0; i < 5; i++ {
		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: client.ObjectKey{Name: "worker"},
		})
		if err != nil {
			t.Fatalf("Reconcile() error = %v at iteration %d", err, i)
		}
	}

	// Check node has desired-revision
	if err := r.Get(context.Background(), client.ObjectKey{Name: "worker-1"}, updatedNode); err != nil {
		t.Fatalf("Failed to get node: %v", err)
	}

	desired := updatedNode.Annotations[annotations.DesiredRevision]
	if desired == "" {
		t.Error("desired-revision annotation not set on node")
	}

	poolAnnotation := updatedNode.Annotations[annotations.Pool]
	if poolAnnotation != "worker" {
		t.Errorf("pool annotation = %q, want %q", poolAnnotation, "worker")
	}
}

func TestReconcile_UpdatesPoolStatus(t *testing.T) {
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

	nodes := []client.Object{
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "worker-1",
				Labels: map[string]string{"role": "worker"},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "worker-2",
				Labels: map[string]string{"role": "worker"},
			},
		},
	}

	mc := &mcov1alpha1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mc-1"},
		Spec:       mcov1alpha1.MachineConfigSpec{Priority: 50},
	}

	allObjs := append([]client.Object{pool, mc}, nodes...)
	r := newReconciler(allObjs...)

	r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Name: "worker"},
	})

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Name: "worker"},
	})

	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	updatedPool := &mcov1alpha1.MachineConfigPool{}
	if err := r.Get(context.Background(), client.ObjectKey{Name: "worker"}, updatedPool); err != nil {
		t.Fatalf("Failed to get pool: %v", err)
	}

	if updatedPool.Status.MachineCount != 2 {
		t.Errorf("MachineCount = %d, want 2", updatedPool.Status.MachineCount)
	}

	if updatedPool.Status.TargetRevision == "" {
		t.Error("TargetRevision should be set")
	}
}

func TestReconcile_ReusesExistingRMC(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Rollout: mcov1alpha1.RolloutConfig{
				DebounceSeconds: 0,
			},
		},
	}

	mc := &mcov1alpha1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mc-1"},
		Spec:       mcov1alpha1.MachineConfigSpec{Priority: 50},
	}

	r := newReconciler(pool, mc)

	r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Name: "worker"},
	})
	r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Name: "worker"},
	})

	rmcList := &mcov1alpha1.RenderedMachineConfigList{}
	r.List(context.Background(), rmcList)
	initialCount := len(rmcList.Items)

	r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Name: "worker"},
	})

	r.List(context.Background(), rmcList)
	if len(rmcList.Items) != initialCount {
		t.Errorf("RMC count changed from %d to %d, should reuse existing", initialCount, len(rmcList.Items))
	}
}

func TestMapMachineConfigToPool(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			MachineConfigSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"pool": "worker"},
			},
		},
	}

	mc := &mcov1alpha1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "mc-1",
			Labels: map[string]string{"pool": "worker"},
		},
	}

	r := newReconciler(pool, mc)

	requests := r.mapMachineConfigToPool(context.Background(), mc)

	if len(requests) != 1 {
		t.Errorf("mapMachineConfigToPool() returned %d requests, want 1", len(requests))
	}

	if len(requests) > 0 && requests[0].Name != "worker" {
		t.Errorf("Request name = %s, want worker", requests[0].Name)
	}
}

func TestMapNodeToPool(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"role": "worker"},
			},
		},
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-1",
			Labels: map[string]string{"role": "worker"},
		},
	}

	r := newReconciler(pool, node)

	requests := r.mapNodeToPool(context.Background(), node)

	if len(requests) != 1 {
		t.Errorf("mapNodeToPool() returned %d requests, want 1", len(requests))
	}

	if len(requests) > 0 && requests[0].Name != "worker" {
		t.Errorf("Request name = %s, want worker", requests[0].Name)
	}
}

func TestMapMachineConfigToPool_NonMatching(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			MachineConfigSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"pool": "worker"},
			},
		},
	}

	mc := &mcov1alpha1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "mc-master",
			Labels: map[string]string{"pool": "master"},
		},
	}

	r := newReconciler(pool, mc)

	requests := r.mapMachineConfigToPool(context.Background(), mc)

	if len(requests) != 0 {
		t.Errorf("mapMachineConfigToPool() returned %d requests, want 0", len(requests))
	}
}

func TestMapNodeToPool_WrongType(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
	}

	mc := &mcov1alpha1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mc-1"},
	}

	r := newReconciler(pool)

	requests := r.mapNodeToPool(context.Background(), mc)

	if len(requests) != 0 {
		t.Errorf("mapNodeToPool() with wrong type returned %d requests, want 0", len(requests))
	}
}

func TestListAllPoolNames(t *testing.T) {
	tests := []struct {
		name     string
		pools    []client.Object
		expected []string
	}{
		{
			name:     "no pools",
			pools:    nil,
			expected: []string{},
		},
		{
			name: "single pool",
			pools: []client.Object{
				&mcov1alpha1.MachineConfigPool{
					ObjectMeta: metav1.ObjectMeta{Name: "worker"},
				},
			},
			expected: []string{"worker"},
		},
		{
			name: "multiple pools",
			pools: []client.Object{
				&mcov1alpha1.MachineConfigPool{
					ObjectMeta: metav1.ObjectMeta{Name: "worker"},
				},
				&mcov1alpha1.MachineConfigPool{
					ObjectMeta: metav1.ObjectMeta{Name: "master"},
				},
				&mcov1alpha1.MachineConfigPool{
					ObjectMeta: metav1.ObjectMeta{Name: "infra"},
				},
			},
			expected: []string{"worker", "master", "infra"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newReconciler(tt.pools...)

			names, err := r.listAllPoolNames(context.Background())
			if err != nil {
				t.Fatalf("listAllPoolNames() error = %v", err)
			}

			if len(names) != len(tt.expected) {
				t.Errorf("listAllPoolNames() returned %d names, want %d", len(names), len(tt.expected))
				return
			}

			// Check all expected names are present (order may vary)
			nameSet := make(map[string]bool)
			for _, n := range names {
				nameSet[n] = true
			}
			for _, exp := range tt.expected {
				if !nameSet[exp] {
					t.Errorf("listAllPoolNames() missing expected pool %q", exp)
				}
			}
		})
	}
}

// TestEnsureRMC_HashCollision_UsesSuffix tests that hash collision triggers suffix retry loop.
func TestEnsureRMC_HashCollision_UsesSuffix(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			MachineConfigSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"mco.in-cloud.io/pool": "worker"},
			},
		},
	}

	// Create merged config - will generate a specific hash
	merged := &renderer.MergedConfig{
		Files: []mcov1alpha1.FileSpec{{Path: "/etc/new.conf", Content: "new content"}},
	}

	// Build expected RMC to get the name
	expectedRMC := renderer.BuildRMC(pool.Name, merged, pool)

	// Create existing RMC with DIFFERENT hash at the same name
	existingRMC := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: expectedRMC.Name, // Same name that would be generated
		},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			PoolName:   pool.Name,
			Revision:   "old1234567",
			ConfigHash: "0000000000000000000000000000000000000000000000000000000000000000", // Different hash (64 chars)
			Config: mcov1alpha1.RenderedConfig{
				Files: []mcov1alpha1.FileSpec{{Path: "/etc/old.conf", Content: "old"}},
			},
		},
	}

	r := newReconciler(pool, existingRMC)

	// Call ensureRMC - should trigger collision handling
	rmc, err := r.ensureRMC(context.Background(), pool, merged)
	if err != nil {
		t.Fatalf("ensureRMC() error = %v", err)
	}

	// Verify new RMC was created with suffix
	if rmc.Name == existingRMC.Name {
		t.Errorf("RMC name should have suffix, got %q (same as existing)", rmc.Name)
	}

	// Verify it has the correct suffix format
	if !strings.HasPrefix(rmc.Name, "worker-") {
		t.Errorf("RMC name should start with 'worker-', got %q", rmc.Name)
	}

	// Verify suffix was added
	if !strings.HasSuffix(rmc.Name, "-1") {
		t.Errorf("RMC name should end with '-1' suffix, got %q", rmc.Name)
	}
}

// TestEnsureRMC_HashCollision_ReusesMatchingSuffix tests that existing suffix with matching hash is reused
func TestEnsureRMC_HashCollision_ReusesMatchingSuffix(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			MachineConfigSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"mco.in-cloud.io/pool": "worker"},
			},
		},
	}

	// Merged config
	merged := &renderer.MergedConfig{
		Files: []mcov1alpha1.FileSpec{{Path: "/etc/test.conf", Content: "test content"}},
	}

	// Build RMC to get expected hash
	expectedRMC := renderer.BuildRMC(pool.Name, merged, pool)

	// Create existing RMC with OLD hash at base name
	existingBase := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: expectedRMC.Name,
		},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			PoolName:   pool.Name,
			Revision:   "old1234567",
			ConfigHash: "0000000000000000000000000000000000000000000000000000000000000000",
			Config: mcov1alpha1.RenderedConfig{
				Files: []mcov1alpha1.FileSpec{{Path: "/etc/old.conf", Content: "old"}},
			},
		},
	}

	// Create existing RMC with MATCHING hash at suffix-1
	existingSuffix := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: expectedRMC.Name + "-1",
		},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			PoolName:   pool.Name,
			Revision:   expectedRMC.Spec.Revision,
			ConfigHash: expectedRMC.Spec.ConfigHash, // Same hash!
			Config:     expectedRMC.Spec.Config,
		},
	}

	r := newReconciler(pool, existingBase, existingSuffix)

	rmc, err := r.ensureRMC(context.Background(), pool, merged)
	if err != nil {
		t.Fatalf("ensureRMC() error = %v", err)
	}

	// Verify existing suffix RMC was reused
	if rmc.Name != existingSuffix.Name {
		t.Errorf("RMC name = %q, want %q (should reuse existing with matching hash)", rmc.Name, existingSuffix.Name)
	}
}

// TestReconcile_EmptyMachineConfigList_NoRollout verifies that MCP without MachineConfigs
// does not trigger cordon/drain rollout.
func TestReconcile_EmptyMachineConfigList_NoRollout(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"role": "worker"},
			},
			Rollout: mcov1alpha1.RolloutConfig{
				DebounceSeconds: 0, // No debounce for test
			},
		},
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker-1",
			Labels: map[string]string{"role": "worker"},
		},
	}

	// No MachineConfigs - empty pool
	r := newReconciler(pool, node)

	// First reconcile
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Name: "worker"},
	})

	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Should not requeue (no work to do)
	if result.Requeue {
		t.Error("Reconcile() should not requeue for empty pool")
	}

	// Second reconcile to ensure stability
	result, err = r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Name: "worker"},
	})

	if err != nil {
		t.Fatalf("Second Reconcile() error = %v", err)
	}

	// Verify node is NOT cordoned
	updatedNode := &corev1.Node{}
	if err := r.Get(context.Background(), client.ObjectKey{Name: "worker-1"}, updatedNode); err != nil {
		t.Fatalf("Failed to get node: %v", err)
	}

	if updatedNode.Spec.Unschedulable {
		t.Error("node should NOT be cordoned for empty MachineConfig pool")
	}

	cordoned := updatedNode.Annotations[annotations.Cordoned]
	if cordoned == "true" {
		t.Error("node should NOT have cordoned annotation for empty pool")
	}

	// Verify no RMC was created
	rmcList := &mcov1alpha1.RenderedMachineConfigList{}
	if err := r.List(context.Background(), rmcList); err != nil {
		t.Fatalf("Failed to list RMCs: %v", err)
	}

	if len(rmcList.Items) != 0 {
		t.Errorf("RMC count = %d, want 0 for empty pool", len(rmcList.Items))
	}

	// Verify pool status is updated (AC-005)
	updatedPool := &mcov1alpha1.MachineConfigPool{}
	if err := r.Get(context.Background(), client.ObjectKey{Name: "worker"}, updatedPool); err != nil {
		t.Fatalf("Failed to get pool: %v", err)
	}

	if updatedPool.Status.MachineCount != 1 {
		t.Errorf("MachineCount = %d, want 1", updatedPool.Status.MachineCount)
	}

	if updatedPool.Status.ReadyMachineCount != 1 {
		t.Errorf("ReadyMachineCount = %d, want 1", updatedPool.Status.ReadyMachineCount)
	}

	if updatedPool.Status.TargetRevision != "" {
		t.Errorf("TargetRevision = %q, want empty for empty pool", updatedPool.Status.TargetRevision)
	}
}

// TestReconcile_EmptyToNonEmpty_TriggersRollout verifies that adding MachineConfig
// to previously empty pool triggers rollout correctly.
func TestReconcile_EmptyToNonEmpty_TriggersRollout(t *testing.T) {
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

	// Start with empty pool
	r := newReconciler(pool, node)

	// First reconcile - empty pool
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Name: "worker"},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Verify node is not cordoned (empty pool)
	updatedNode := &corev1.Node{}
	if err := r.Get(context.Background(), client.ObjectKey{Name: "worker-1"}, updatedNode); err != nil {
		t.Fatalf("Failed to get node: %v", err)
	}
	if updatedNode.Spec.Unschedulable {
		t.Error("node should NOT be cordoned for empty pool before adding MC")
	}

	// Now add a MachineConfig
	mc := &mcov1alpha1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mc-1"},
		Spec: mcov1alpha1.MachineConfigSpec{
			Priority: 50,
			Files: []mcov1alpha1.FileSpec{
				{Path: "/etc/test.conf", Content: "test"},
			},
		},
	}
	if err := r.Create(context.Background(), mc); err != nil {
		t.Fatalf("Failed to create MC: %v", err)
	}

	// Reconcile again - should now trigger rollout
	_, err = r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Name: "worker"},
	})
	if err != nil {
		t.Fatalf("Reconcile() after adding MC error = %v", err)
	}

	// Run a few more reconciles to process the rollout
	for i := 0; i < 3; i++ {
		r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: client.ObjectKey{Name: "worker"},
		})
	}

	// Verify RMC was created
	rmcList := &mcov1alpha1.RenderedMachineConfigList{}
	if err := r.List(context.Background(), rmcList); err != nil {
		t.Fatalf("Failed to list RMCs: %v", err)
	}

	if len(rmcList.Items) != 1 {
		t.Errorf("RMC count = %d, want 1 after adding MC", len(rmcList.Items))
	}

	// Verify node is now cordoned (rollout started)
	if err := r.Get(context.Background(), client.ObjectKey{Name: "worker-1"}, updatedNode); err != nil {
		t.Fatalf("Failed to get node: %v", err)
	}

	if !updatedNode.Spec.Unschedulable {
		t.Error("node should be cordoned after adding MC to trigger rollout")
	}

	// Verify pool has TargetRevision set
	updatedPool := &mcov1alpha1.MachineConfigPool{}
	if err := r.Get(context.Background(), client.ObjectKey{Name: "worker"}, updatedPool); err != nil {
		t.Fatalf("Failed to get pool: %v", err)
	}

	if updatedPool.Status.TargetRevision == "" {
		t.Error("TargetRevision should be set after adding MC")
	}
}

// TestEnsureRMC_NoCollision_ReusesExisting tests that matching hash reuses existing without suffix
func TestEnsureRMC_NoCollision_ReusesExisting(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			MachineConfigSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"mco.in-cloud.io/pool": "worker"},
			},
		},
	}

	merged := &renderer.MergedConfig{
		Files: []mcov1alpha1.FileSpec{{Path: "/etc/test.conf", Content: "test content"}},
	}

	// Build RMC to get expected hash
	expectedRMC := renderer.BuildRMC(pool.Name, merged, pool)

	// Create existing RMC with SAME hash
	existingRMC := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: expectedRMC.Name,
		},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			PoolName:   pool.Name,
			Revision:   expectedRMC.Spec.Revision,
			ConfigHash: expectedRMC.Spec.ConfigHash, // Same hash!
			Config:     expectedRMC.Spec.Config,
			Reboot:     expectedRMC.Spec.Reboot,
		},
	}

	r := newReconciler(pool, existingRMC)

	rmc, err := r.ensureRMC(context.Background(), pool, merged)
	if err != nil {
		t.Fatalf("ensureRMC() error = %v", err)
	}

	// Verify existing RMC was reused (no suffix)
	if rmc.Name != existingRMC.Name {
		t.Errorf("RMC name = %q, want %q (should reuse existing with matching hash)", rmc.Name, existingRMC.Name)
	}
}
