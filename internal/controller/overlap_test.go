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
	"reflect"
	"sort"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// TestOverlapResult_NewOverlapResult verifies constructor.
func TestOverlapResult_NewOverlapResult(t *testing.T) {
	result := NewOverlapResult()

	if result == nil {
		t.Fatal("NewOverlapResult() returned nil")
	}
	if result.ConflictingNodes == nil {
		t.Error("NewOverlapResult() should initialize ConflictingNodes map")
	}
	if result.HasConflicts() {
		t.Error("NewOverlapResult() should have no conflicts initially")
	}
}

// TestOverlapResult_HasConflicts verifies HasConflicts method.
func TestOverlapResult_HasConflicts(t *testing.T) {
	tests := []struct {
		name     string
		nodes    map[string][]string
		expected bool
	}{
		{
			name:     "empty map",
			nodes:    map[string][]string{},
			expected: false,
		},
		{
			name:     "one conflicting node",
			nodes:    map[string][]string{"node1": {"pool1", "pool2"}},
			expected: true,
		},
		{
			name: "multiple conflicting nodes",
			nodes: map[string][]string{
				"node1": {"pool1", "pool2"},
				"node2": {"pool2", "pool3"},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &OverlapResult{ConflictingNodes: tt.nodes}
			if got := result.HasConflicts(); got != tt.expected {
				t.Errorf("HasConflicts() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestOverlapResult_ConflictCount verifies ConflictCount method.
func TestOverlapResult_ConflictCount(t *testing.T) {
	tests := []struct {
		name     string
		nodes    map[string][]string
		expected int
	}{
		{
			name:     "empty",
			nodes:    map[string][]string{},
			expected: 0,
		},
		{
			name:     "one node",
			nodes:    map[string][]string{"node1": {"pool1", "pool2"}},
			expected: 1,
		},
		{
			name: "three nodes",
			nodes: map[string][]string{
				"node1": {"pool1", "pool2"},
				"node2": {"pool2", "pool3"},
				"node3": {"pool1", "pool3"},
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &OverlapResult{ConflictingNodes: tt.nodes}
			if got := result.ConflictCount(); got != tt.expected {
				t.Errorf("ConflictCount() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestOverlapResult_GetConflictsForPool verifies GetConflictsForPool method.
func TestOverlapResult_GetConflictsForPool(t *testing.T) {
	result := &OverlapResult{
		ConflictingNodes: map[string][]string{
			"node1": {"pool1", "pool2"},
			"node2": {"pool2", "pool3"},
			"node3": {"pool1", "pool3"},
		},
	}

	tests := []struct {
		pool     string
		expected []string
	}{
		{pool: "pool1", expected: []string{"node1", "node3"}},
		{pool: "pool2", expected: []string{"node1", "node2"}},
		{pool: "pool3", expected: []string{"node2", "node3"}},
		{pool: "pool4", expected: nil}, // Pool not in any conflict
	}

	for _, tt := range tests {
		t.Run(tt.pool, func(t *testing.T) {
			got := result.GetConflictsForPool(tt.pool)
			// Sort for comparison (GetConflictsForPool returns sorted)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("GetConflictsForPool(%s) = %v, want %v", tt.pool, got, tt.expected)
			}
		})
	}
}

// TestOverlapResult_GetPoolsForNode verifies GetPoolsForNode method.
func TestOverlapResult_GetPoolsForNode(t *testing.T) {
	result := &OverlapResult{
		ConflictingNodes: map[string][]string{
			"node1": {"pool1", "pool2"},
			"node2": {"pool2", "pool3"},
		},
	}

	tests := []struct {
		node     string
		expected []string
	}{
		{node: "node1", expected: []string{"pool1", "pool2"}},
		{node: "node2", expected: []string{"pool2", "pool3"}},
		{node: "node3", expected: nil}, // Node not conflicting
	}

	for _, tt := range tests {
		t.Run(tt.node, func(t *testing.T) {
			got := result.GetPoolsForNode(tt.node)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("GetPoolsForNode(%s) = %v, want %v", tt.node, got, tt.expected)
			}
		})
	}
}

// TestOverlapResult_IsNodeConflicting verifies IsNodeConflicting method.
func TestOverlapResult_IsNodeConflicting(t *testing.T) {
	result := &OverlapResult{
		ConflictingNodes: map[string][]string{
			"node1": {"pool1", "pool2"},
		},
	}

	if !result.IsNodeConflicting("node1") {
		t.Error("IsNodeConflicting(node1) should return true")
	}
	if result.IsNodeConflicting("node2") {
		t.Error("IsNodeConflicting(node2) should return false")
	}
}

// TestOverlapResult_GetAllConflictingPools verifies GetAllConflictingPools method.
func TestOverlapResult_GetAllConflictingPools(t *testing.T) {
	result := &OverlapResult{
		ConflictingNodes: map[string][]string{
			"node1": {"pool1", "pool2"},
			"node2": {"pool2", "pool3"},
		},
	}

	expected := []string{"pool1", "pool2", "pool3"}
	got := result.GetAllConflictingPools()

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("GetAllConflictingPools() = %v, want %v", got, expected)
	}
}

// TestDetectPoolOverlap_NoConflict verifies detection when no overlap exists.
func TestDetectPoolOverlap_NoConflict(t *testing.T) {
	scheme := newTestScheme()

	// Create nodes with different labels
	nodes := []client.Object{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker1", Labels: map[string]string{"role": "worker"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker2", Labels: map[string]string{"role": "worker"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "master1", Labels: map[string]string{"role": "master"}}},
	}

	// Create pools with non-overlapping selectors
	pools := []client.Object{
		&mcov1alpha1.MachineConfigPool{
			ObjectMeta: metav1.ObjectMeta{Name: "workers"},
			Spec: mcov1alpha1.MachineConfigPoolSpec{
				NodeSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"role": "worker"},
				},
			},
		},
		&mcov1alpha1.MachineConfigPool{
			ObjectMeta: metav1.ObjectMeta{Name: "masters"},
			Spec: mcov1alpha1.MachineConfigPoolSpec{
				NodeSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"role": "master"},
				},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(append(nodes, pools...)...).Build()

	result, err := DetectPoolOverlap(context.Background(), c)
	if err != nil {
		t.Fatalf("DetectPoolOverlap() error = %v", err)
	}

	if result.HasConflicts() {
		t.Errorf("DetectPoolOverlap() found conflicts where none exist: %v", result.ConflictingNodes)
	}
}

// TestDetectPoolOverlap_SingleNodeConflict verifies detection of single node overlap.
func TestDetectPoolOverlap_SingleNodeConflict(t *testing.T) {
	scheme := newTestScheme()

	// Node with labels that match both pools
	nodes := []client.Object{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{
			Name:   "node1",
			Labels: map[string]string{"role": "worker", "env": "prod"},
		}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{
			Name:   "node2",
			Labels: map[string]string{"role": "worker", "env": "dev"},
		}},
	}

	// Two pools with overlapping selectors for node1
	pools := []client.Object{
		&mcov1alpha1.MachineConfigPool{
			ObjectMeta: metav1.ObjectMeta{Name: "workers"},
			Spec: mcov1alpha1.MachineConfigPoolSpec{
				NodeSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"role": "worker"},
				},
			},
		},
		&mcov1alpha1.MachineConfigPool{
			ObjectMeta: metav1.ObjectMeta{Name: "prod"},
			Spec: mcov1alpha1.MachineConfigPoolSpec{
				NodeSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"env": "prod"},
				},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(append(nodes, pools...)...).Build()

	result, err := DetectPoolOverlap(context.Background(), c)
	if err != nil {
		t.Fatalf("DetectPoolOverlap() error = %v", err)
	}

	if !result.HasConflicts() {
		t.Fatal("DetectPoolOverlap() should detect conflict")
	}

	if result.ConflictCount() != 1 {
		t.Errorf("DetectPoolOverlap() conflict count = %d, want 1", result.ConflictCount())
	}

	if !result.IsNodeConflicting("node1") {
		t.Error("node1 should be conflicting")
	}

	if result.IsNodeConflicting("node2") {
		t.Error("node2 should not be conflicting")
	}

	matchingPools := result.GetPoolsForNode("node1")
	sort.Strings(matchingPools)
	expected := []string{"prod", "workers"}
	if !reflect.DeepEqual(matchingPools, expected) {
		t.Errorf("GetPoolsForNode(node1) = %v, want %v", matchingPools, expected)
	}
}

// TestDetectPoolOverlap_MultipleNodesConflict verifies detection of multiple overlapping nodes.
func TestDetectPoolOverlap_MultipleNodesConflict(t *testing.T) {
	scheme := newTestScheme()

	nodes := []client.Object{
		// Both nodes match "workers" and "infra" pools
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{
			Name:   "node1",
			Labels: map[string]string{"role": "worker", "type": "infra"},
		}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{
			Name:   "node2",
			Labels: map[string]string{"role": "worker", "type": "infra"},
		}},
		// node3 only matches workers
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{
			Name:   "node3",
			Labels: map[string]string{"role": "worker"},
		}},
	}

	pools := []client.Object{
		&mcov1alpha1.MachineConfigPool{
			ObjectMeta: metav1.ObjectMeta{Name: "workers"},
			Spec: mcov1alpha1.MachineConfigPoolSpec{
				NodeSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"role": "worker"},
				},
			},
		},
		&mcov1alpha1.MachineConfigPool{
			ObjectMeta: metav1.ObjectMeta{Name: "infra"},
			Spec: mcov1alpha1.MachineConfigPoolSpec{
				NodeSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"type": "infra"},
				},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(append(nodes, pools...)...).Build()

	result, err := DetectPoolOverlap(context.Background(), c)
	if err != nil {
		t.Fatalf("DetectPoolOverlap() error = %v", err)
	}

	if result.ConflictCount() != 2 {
		t.Errorf("DetectPoolOverlap() conflict count = %d, want 2", result.ConflictCount())
	}

	if !result.IsNodeConflicting("node1") || !result.IsNodeConflicting("node2") {
		t.Error("node1 and node2 should both be conflicting")
	}

	if result.IsNodeConflicting("node3") {
		t.Error("node3 should not be conflicting")
	}
}

// TestDetectPoolOverlap_NilSelector verifies pool with nil selector matches all.
func TestDetectPoolOverlap_NilSelector(t *testing.T) {
	scheme := newTestScheme()

	nodes := []client.Object{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1", Labels: map[string]string{"role": "worker"}}},
	}

	pools := []client.Object{
		// Pool with nil selector matches all nodes
		&mcov1alpha1.MachineConfigPool{
			ObjectMeta: metav1.ObjectMeta{Name: "all-nodes"},
			Spec:       mcov1alpha1.MachineConfigPoolSpec{NodeSelector: nil},
		},
		&mcov1alpha1.MachineConfigPool{
			ObjectMeta: metav1.ObjectMeta{Name: "workers"},
			Spec: mcov1alpha1.MachineConfigPoolSpec{
				NodeSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"role": "worker"},
				},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(append(nodes, pools...)...).Build()

	result, err := DetectPoolOverlap(context.Background(), c)
	if err != nil {
		t.Fatalf("DetectPoolOverlap() error = %v", err)
	}

	// node1 matches both pools (nil selector and "role=worker")
	if !result.HasConflicts() {
		t.Error("Should detect conflict when one pool has nil selector")
	}

	if !result.IsNodeConflicting("node1") {
		t.Error("node1 should be conflicting due to nil selector pool")
	}
}

// TestDetectPoolOverlap_NoNodes verifies behavior with no nodes.
func TestDetectPoolOverlap_NoNodes(t *testing.T) {
	scheme := newTestScheme()

	pools := []client.Object{
		&mcov1alpha1.MachineConfigPool{
			ObjectMeta: metav1.ObjectMeta{Name: "workers"},
			Spec: mcov1alpha1.MachineConfigPoolSpec{
				NodeSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"role": "worker"},
				},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pools...).Build()

	result, err := DetectPoolOverlap(context.Background(), c)
	if err != nil {
		t.Fatalf("DetectPoolOverlap() error = %v", err)
	}

	if result.HasConflicts() {
		t.Error("Should not detect conflicts with no nodes")
	}
}

// TestDetectPoolOverlap_NoPools verifies behavior with no pools.
func TestDetectPoolOverlap_NoPools(t *testing.T) {
	scheme := newTestScheme()

	nodes := []client.Object{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodes...).Build()

	result, err := DetectPoolOverlap(context.Background(), c)
	if err != nil {
		t.Fatalf("DetectPoolOverlap() error = %v", err)
	}

	if result.HasConflicts() {
		t.Error("Should not detect conflicts with no pools")
	}
}

// TestDetectPoolOverlap_ThreePools verifies node matching three pools.
func TestDetectPoolOverlap_ThreePools(t *testing.T) {
	scheme := newTestScheme()

	nodes := []client.Object{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{
			Name:   "node1",
			Labels: map[string]string{"role": "worker", "env": "prod", "zone": "us-east"},
		}},
	}

	pools := []client.Object{
		&mcov1alpha1.MachineConfigPool{
			ObjectMeta: metav1.ObjectMeta{Name: "workers"},
			Spec: mcov1alpha1.MachineConfigPoolSpec{
				NodeSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"role": "worker"},
				},
			},
		},
		&mcov1alpha1.MachineConfigPool{
			ObjectMeta: metav1.ObjectMeta{Name: "prod"},
			Spec: mcov1alpha1.MachineConfigPoolSpec{
				NodeSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"env": "prod"},
				},
			},
		},
		&mcov1alpha1.MachineConfigPool{
			ObjectMeta: metav1.ObjectMeta{Name: "us-east"},
			Spec: mcov1alpha1.MachineConfigPoolSpec{
				NodeSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"zone": "us-east"},
				},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(append(nodes, pools...)...).Build()

	result, err := DetectPoolOverlap(context.Background(), c)
	if err != nil {
		t.Fatalf("DetectPoolOverlap() error = %v", err)
	}

	if result.ConflictCount() != 1 {
		t.Errorf("Conflict count = %d, want 1", result.ConflictCount())
	}

	poolsForNode := result.GetPoolsForNode("node1")
	if len(poolsForNode) != 3 {
		t.Errorf("node1 should match 3 pools, got %d: %v", len(poolsForNode), poolsForNode)
	}
}

// TestDetectPoolOverlapFromLists verifies the list-based detection.
func TestDetectPoolOverlapFromLists(t *testing.T) {
	pools := []mcov1alpha1.MachineConfigPool{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pool1"},
			Spec: mcov1alpha1.MachineConfigPoolSpec{
				NodeSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"role": "worker"},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pool2"},
			Spec: mcov1alpha1.MachineConfigPoolSpec{
				NodeSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"env": "prod"},
				},
			},
		},
	}

	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "conflicting", Labels: map[string]string{"role": "worker", "env": "prod"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "ok", Labels: map[string]string{"role": "worker"}}},
	}

	result, err := DetectPoolOverlapFromLists(pools, nodes)
	if err != nil {
		t.Fatalf("DetectPoolOverlapFromLists() error = %v", err)
	}

	if !result.IsNodeConflicting("conflicting") {
		t.Error("Node 'conflicting' should be in conflict")
	}
	if result.IsNodeConflicting("ok") {
		t.Error("Node 'ok' should not be in conflict")
	}
}

// TestFilterNonConflictingNodes verifies node filtering.
func TestFilterNonConflictingNodes(t *testing.T) {
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node2"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node3"}},
	}

	overlap := &OverlapResult{
		ConflictingNodes: map[string][]string{
			"node1": {"pool1", "pool2"},
			"node3": {"pool2", "pool3"},
		},
	}

	result := FilterNonConflictingNodes(nodes, overlap)

	if len(result) != 1 {
		t.Errorf("FilterNonConflictingNodes() returned %d nodes, want 1", len(result))
	}

	if result[0].Name != "node2" {
		t.Errorf("FilterNonConflictingNodes() should return node2, got %s", result[0].Name)
	}
}

// TestFilterNonConflictingNodes_NilOverlap verifies nil overlap handling.
func TestFilterNonConflictingNodes_NilOverlap(t *testing.T) {
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node2"}},
	}

	result := FilterNonConflictingNodes(nodes, nil)

	if len(result) != 2 {
		t.Errorf("FilterNonConflictingNodes(nil) should return all nodes, got %d", len(result))
	}
}

// TestFilterNonConflictingNodes_NoConflicts verifies behavior when no conflicts.
func TestFilterNonConflictingNodes_NoConflicts(t *testing.T) {
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node2"}},
	}

	overlap := NewOverlapResult()

	result := FilterNonConflictingNodes(nodes, overlap)

	if len(result) != 2 {
		t.Errorf("FilterNonConflictingNodes with no conflicts should return all nodes, got %d", len(result))
	}
}

// TestFilterNonConflictingNodes_AllConflicting verifies all nodes conflicting.
func TestFilterNonConflictingNodes_AllConflicting(t *testing.T) {
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node2"}},
	}

	overlap := &OverlapResult{
		ConflictingNodes: map[string][]string{
			"node1": {"pool1", "pool2"},
			"node2": {"pool1", "pool3"},
		},
	}

	result := FilterNonConflictingNodes(nodes, overlap)

	if len(result) != 0 {
		t.Errorf("FilterNonConflictingNodes should return empty when all nodes conflict, got %d", len(result))
	}
}

// TestDetectPoolOverlap_MatchExpressions verifies matchExpressions selector.
func TestDetectPoolOverlap_MatchExpressions(t *testing.T) {
	scheme := newTestScheme()

	nodes := []client.Object{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{
			Name:   "node1",
			Labels: map[string]string{"zone": "us-east"},
		}},
	}

	pools := []client.Object{
		&mcov1alpha1.MachineConfigPool{
			ObjectMeta: metav1.ObjectMeta{Name: "east-zones"},
			Spec: mcov1alpha1.MachineConfigPoolSpec{
				NodeSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "zone",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"us-east", "us-west"},
						},
					},
				},
			},
		},
		&mcov1alpha1.MachineConfigPool{
			ObjectMeta: metav1.ObjectMeta{Name: "all-zones"},
			Spec: mcov1alpha1.MachineConfigPoolSpec{
				NodeSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "zone",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(append(nodes, pools...)...).Build()

	result, err := DetectPoolOverlap(context.Background(), c)
	if err != nil {
		t.Fatalf("DetectPoolOverlap() error = %v", err)
	}

	if !result.IsNodeConflicting("node1") {
		t.Error("node1 should conflict (matches both MatchExpressions pools)")
	}
}
