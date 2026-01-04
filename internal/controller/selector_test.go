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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = mcov1alpha1.AddToScheme(scheme)
	return scheme
}

// TestSelectNodes_NilSelector verifies that nil nodeSelector matches all nodes.
func TestSelectNodes_NilSelector(t *testing.T) {
	scheme := newTestScheme()

	nodes := []client.Object{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1", Labels: map[string]string{"role": "worker"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node2", Labels: map[string]string{"role": "master"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node3"}},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodes...).Build()

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			NodeSelector: nil,
		},
	}

	result, err := SelectNodes(context.Background(), c, pool)
	if err != nil {
		t.Fatalf("SelectNodes() error = %v", err)
	}

	if len(result) != 3 {
		t.Errorf("SelectNodes() returned %d nodes, want 3", len(result))
	}
}

// TestSelectNodes_MatchLabels verifies nodeSelector with matchLabels.
func TestSelectNodes_MatchLabels(t *testing.T) {
	scheme := newTestScheme()

	nodes := []client.Object{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker1", Labels: map[string]string{"role": "worker"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker2", Labels: map[string]string{"role": "worker"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "master1", Labels: map[string]string{"role": "master"}}},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodes...).Build()

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-pool"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"role": "worker"},
			},
		},
	}

	result, err := SelectNodes(context.Background(), c, pool)
	if err != nil {
		t.Fatalf("SelectNodes() error = %v", err)
	}

	if len(result) != 2 {
		t.Errorf("SelectNodes() returned %d nodes, want 2", len(result))
	}

	names := make(map[string]bool)
	for _, n := range result {
		names[n.Name] = true
	}

	if !names["worker1"] || !names["worker2"] {
		t.Errorf("SelectNodes() should return worker1 and worker2, got %v", names)
	}
}

// TestSelectNodes_MatchExpressions verifies nodeSelector with matchExpressions.
func TestSelectNodes_MatchExpressions(t *testing.T) {
	scheme := newTestScheme()

	nodes := []client.Object{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "zone-a", Labels: map[string]string{"zone": "a"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "zone-b", Labels: map[string]string{"zone": "b"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "zone-c", Labels: map[string]string{"zone": "c"}}},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodes...).Build()

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			NodeSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "zone",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"a", "b"},
					},
				},
			},
		},
	}

	result, err := SelectNodes(context.Background(), c, pool)
	if err != nil {
		t.Fatalf("SelectNodes() error = %v", err)
	}

	if len(result) != 2 {
		t.Errorf("SelectNodes() returned %d nodes, want 2", len(result))
	}
}

// TestSelectNodes_NoMatches verifies empty result when no nodes match.
func TestSelectNodes_NoMatches(t *testing.T) {
	scheme := newTestScheme()

	nodes := []client.Object{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1", Labels: map[string]string{"role": "worker"}}},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodes...).Build()

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"role": "nonexistent"},
			},
		},
	}

	result, err := SelectNodes(context.Background(), c, pool)
	if err != nil {
		t.Fatalf("SelectNodes() error = %v", err)
	}

	if len(result) != 0 {
		t.Errorf("SelectNodes() returned %d nodes, want 0", len(result))
	}
}

// TestSelectMachineConfigs_NilSelector verifies nil selector matches all MCs.
func TestSelectMachineConfigs_NilSelector(t *testing.T) {
	scheme := newTestScheme()

	mcs := []client.Object{
		&mcov1alpha1.MachineConfig{ObjectMeta: metav1.ObjectMeta{Name: "mc1", Labels: map[string]string{"pool": "worker"}}},
		&mcov1alpha1.MachineConfig{ObjectMeta: metav1.ObjectMeta{Name: "mc2", Labels: map[string]string{"pool": "master"}}},
		&mcov1alpha1.MachineConfig{ObjectMeta: metav1.ObjectMeta{Name: "mc3"}},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mcs...).Build()

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			MachineConfigSelector: nil,
		},
	}

	result, err := SelectMachineConfigs(context.Background(), c, pool)
	if err != nil {
		t.Fatalf("SelectMachineConfigs() error = %v", err)
	}

	if len(result) != 3 {
		t.Errorf("SelectMachineConfigs() returned %d MCs, want 3", len(result))
	}
}

// TestSelectMachineConfigs_MatchLabels verifies selector with matchLabels.
func TestSelectMachineConfigs_MatchLabels(t *testing.T) {
	scheme := newTestScheme()

	mcs := []client.Object{
		&mcov1alpha1.MachineConfig{ObjectMeta: metav1.ObjectMeta{Name: "mc-worker-1", Labels: map[string]string{"pool": "worker"}}},
		&mcov1alpha1.MachineConfig{ObjectMeta: metav1.ObjectMeta{Name: "mc-worker-2", Labels: map[string]string{"pool": "worker"}}},
		&mcov1alpha1.MachineConfig{ObjectMeta: metav1.ObjectMeta{Name: "mc-master", Labels: map[string]string{"pool": "master"}}},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mcs...).Build()

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-pool"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			MachineConfigSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"pool": "worker"},
			},
		},
	}

	result, err := SelectMachineConfigs(context.Background(), c, pool)
	if err != nil {
		t.Fatalf("SelectMachineConfigs() error = %v", err)
	}

	if len(result) != 2 {
		t.Errorf("SelectMachineConfigs() returned %d MCs, want 2", len(result))
	}
}

// TestNodeMatchesPool_Matches verifies node matching.
func TestNodeMatchesPool_Matches(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker1",
			Labels: map[string]string{"role": "worker", "zone": "a"},
		},
	}

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-pool"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"role": "worker"},
			},
		},
	}

	matches, err := NodeMatchesPool(node, pool)
	if err != nil {
		t.Fatalf("NodeMatchesPool() error = %v", err)
	}

	if !matches {
		t.Error("NodeMatchesPool() should return true for matching node")
	}
}

// TestNodeMatchesPool_NoMatch verifies node not matching.
func TestNodeMatchesPool_NoMatch(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "master1",
			Labels: map[string]string{"role": "master"},
		},
	}

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-pool"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"role": "worker"},
			},
		},
	}

	matches, err := NodeMatchesPool(node, pool)
	if err != nil {
		t.Fatalf("NodeMatchesPool() error = %v", err)
	}

	if matches {
		t.Error("NodeMatchesPool() should return false for non-matching node")
	}
}

// TestNodeMatchesPool_NilSelector verifies nil selector matches all.
func TestNodeMatchesPool_NilSelector(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "any-node"},
	}

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			NodeSelector: nil,
		},
	}

	matches, err := NodeMatchesPool(node, pool)
	if err != nil {
		t.Fatalf("NodeMatchesPool() error = %v", err)
	}

	if !matches {
		t.Error("NodeMatchesPool() with nil selector should match any node")
	}
}

// TestMachineConfigMatchesPool_Matches verifies MC matching.
func TestMachineConfigMatchesPool_Matches(t *testing.T) {
	mc := &mcov1alpha1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "mc-worker",
			Labels: map[string]string{"pool": "worker"},
		},
	}

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-pool"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			MachineConfigSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"pool": "worker"},
			},
		},
	}

	matches, err := MachineConfigMatchesPool(mc, pool)
	if err != nil {
		t.Fatalf("MachineConfigMatchesPool() error = %v", err)
	}

	if !matches {
		t.Error("MachineConfigMatchesPool() should return true for matching MC")
	}
}

// TestMachineConfigMatchesPool_NoMatch verifies MC not matching.
func TestMachineConfigMatchesPool_NoMatch(t *testing.T) {
	mc := &mcov1alpha1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "mc-master",
			Labels: map[string]string{"pool": "master"},
		},
	}

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-pool"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			MachineConfigSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"pool": "worker"},
			},
		},
	}

	matches, err := MachineConfigMatchesPool(mc, pool)
	if err != nil {
		t.Fatalf("MachineConfigMatchesPool() error = %v", err)
	}

	if matches {
		t.Error("MachineConfigMatchesPool() should return false for non-matching MC")
	}
}

// TestSelectorFromLabelSelector_EmptySelector verifies empty LabelSelector.
func TestSelectorFromLabelSelector_EmptySelector(t *testing.T) {
	selector, err := selectorFromLabelSelector(&metav1.LabelSelector{})
	if err != nil {
		t.Fatalf("selectorFromLabelSelector() error = %v", err)
	}

	// Empty selector matches everything
	if selector.String() != "" {
		t.Errorf("Empty LabelSelector should produce empty selector string, got %q", selector.String())
	}
}

// TestNodesForPool_Wrapper verifies convenience wrapper.
func TestNodesForPool_Wrapper(t *testing.T) {
	scheme := newTestScheme()

	nodes := []client.Object{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker1", Labels: map[string]string{"role": "worker"}}},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodes...).Build()

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-pool"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"role": "worker"},
			},
		},
	}

	// NodesForPool should behave same as SelectNodes
	result, err := NodesForPool(context.Background(), c, pool)
	if err != nil {
		t.Fatalf("NodesForPool() error = %v", err)
	}

	if len(result) != 1 {
		t.Errorf("NodesForPool() returned %d nodes, want 1", len(result))
	}
}

// TestMachineConfigsForPool_Wrapper verifies convenience wrapper.
func TestMachineConfigsForPool_Wrapper(t *testing.T) {
	scheme := newTestScheme()

	mcs := []client.Object{
		&mcov1alpha1.MachineConfig{ObjectMeta: metav1.ObjectMeta{Name: "mc1", Labels: map[string]string{"pool": "worker"}}},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mcs...).Build()

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-pool"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			MachineConfigSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"pool": "worker"},
			},
		},
	}

	result, err := MachineConfigsForPool(context.Background(), c, pool)
	if err != nil {
		t.Fatalf("MachineConfigsForPool() error = %v", err)
	}

	if len(result) != 1 {
		t.Errorf("MachineConfigsForPool() returned %d MCs, want 1", len(result))
	}
}

// TestSelectNodes_InvalidSelector verifies error on invalid selector.
func TestSelectNodes_InvalidSelector(t *testing.T) {
	scheme := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			NodeSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "key",
						Operator: "InvalidOperator", // Invalid operator
						Values:   []string{"value"},
					},
				},
			},
		},
	}

	_, err := SelectNodes(context.Background(), c, pool)
	if err == nil {
		t.Error("SelectNodes() should return error for invalid selector")
	}
}

// TestSelectMachineConfigs_InvalidSelector verifies error on invalid selector.
func TestSelectMachineConfigs_InvalidSelector(t *testing.T) {
	scheme := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			MachineConfigSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "key",
						Operator: "InvalidOperator",
						Values:   []string{"value"},
					},
				},
			},
		},
	}

	_, err := SelectMachineConfigs(context.Background(), c, pool)
	if err == nil {
		t.Error("SelectMachineConfigs() should return error for invalid selector")
	}
}

// TestNodeMatchesPool_InvalidSelector verifies error on invalid selector.
func TestNodeMatchesPool_InvalidSelector(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node1"},
	}

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			NodeSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "key",
						Operator: "InvalidOperator",
						Values:   []string{"value"},
					},
				},
			},
		},
	}

	_, err := NodeMatchesPool(node, pool)
	if err == nil {
		t.Error("NodeMatchesPool() should return error for invalid selector")
	}
}

// TestMachineConfigMatchesPool_InvalidSelector verifies error on invalid selector.
func TestMachineConfigMatchesPool_InvalidSelector(t *testing.T) {
	mc := &mcov1alpha1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mc1"},
	}

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			MachineConfigSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "key",
						Operator: "InvalidOperator",
						Values:   []string{"value"},
					},
				},
			},
		},
	}

	_, err := MachineConfigMatchesPool(mc, pool)
	if err == nil {
		t.Error("MachineConfigMatchesPool() should return error for invalid selector")
	}
}
