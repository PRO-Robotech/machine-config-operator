package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"in-cloud.io/machine-config/pkg/annotations"
)

func TestCordonNode(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	ctx := context.Background()

	err := CordonNode(ctx, c, node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := &corev1.Node{}
	if err := c.Get(ctx, client.ObjectKey{Name: "test-node"}, updated); err != nil {
		t.Fatalf("failed to get node: %v", err)
	}

	if !updated.Spec.Unschedulable {
		t.Error("expected node to be unschedulable")
	}

	if updated.Annotations[annotations.Cordoned] != "true" {
		t.Error("expected cordoned annotation to be true")
	}
}

func TestCordonNode_AlreadyCordoned(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-node",
			Annotations: map[string]string{annotations.Cordoned: "true"},
		},
		Spec: corev1.NodeSpec{Unschedulable: true},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	ctx := context.Background()

	err := CordonNode(ctx, c, node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUncordonNode(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				annotations.Cordoned:        "true",
				annotations.DrainStartedAt:  "2025-01-07T00:00:00Z",
				annotations.DrainRetryCount: "5",
			},
		},
		Spec: corev1.NodeSpec{Unschedulable: true},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	ctx := context.Background()

	err := UncordonNode(ctx, c, node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := &corev1.Node{}
	if err := c.Get(ctx, client.ObjectKey{Name: "test-node"}, updated); err != nil {
		t.Fatalf("failed to get node: %v", err)
	}

	if updated.Spec.Unschedulable {
		t.Error("expected node to be schedulable")
	}

	if _, ok := updated.Annotations[annotations.Cordoned]; ok {
		t.Error("expected cordoned annotation to be removed")
	}

	if _, ok := updated.Annotations[annotations.DrainStartedAt]; ok {
		t.Error("expected drain-started-at annotation to be removed")
	}

	if _, ok := updated.Annotations[annotations.DrainRetryCount]; ok {
		t.Error("expected drain-retry-count annotation to be removed")
	}
}

func TestUncordonNode_AlreadyUncordoned(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	ctx := context.Background()

	err := UncordonNode(ctx, c, node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIsNodeCordoned(t *testing.T) {
	tests := []struct {
		name     string
		node     *corev1.Node
		expected bool
	}{
		{
			name:     "nil annotations",
			node:     &corev1.Node{},
			expected: false,
		},
		{
			name: "cordoned true",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{annotations.Cordoned: "true"},
				},
			},
			expected: true,
		},
		{
			name: "cordoned false",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{annotations.Cordoned: "false"},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNodeCordoned(tt.node)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestShouldUncordon(t *testing.T) {
	tests := []struct {
		name           string
		node           *corev1.Node
		targetRevision string
		expected       bool
	}{
		{
			name:           "not cordoned",
			node:           &corev1.Node{},
			targetRevision: "rev-1",
			expected:       false,
		},
		{
			name: "cordoned but revision mismatch",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						annotations.Cordoned:        "true",
						annotations.CurrentRevision: "rev-0",
						annotations.AgentState:      annotations.StateDone,
					},
				},
			},
			targetRevision: "rev-1",
			expected:       false,
		},
		{
			name: "cordoned but not done",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						annotations.Cordoned:        "true",
						annotations.CurrentRevision: "rev-1",
						annotations.AgentState:      annotations.StateApplying,
					},
				},
			},
			targetRevision: "rev-1",
			expected:       false,
		},
		{
			name: "should uncordon",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						annotations.Cordoned:        "true",
						annotations.CurrentRevision: "rev-1",
						annotations.AgentState:      annotations.StateDone,
					},
				},
			},
			targetRevision: "rev-1",
			expected:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldUncordon(tt.node, tt.targetRevision)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetIntAnnotation(t *testing.T) {
	tests := []struct {
		name     string
		node     *corev1.Node
		key      string
		expected int
	}{
		{
			name:     "nil annotations",
			node:     &corev1.Node{},
			key:      "test-key",
			expected: 0,
		},
		{
			name: "missing key",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"other": "value"},
				},
			},
			key:      "test-key",
			expected: 0,
		},
		{
			name: "valid integer",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"test-key": "42"},
				},
			},
			key:      "test-key",
			expected: 42,
		},
		{
			name: "invalid integer",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"test-key": "not-a-number"},
				},
			},
			key:      "test-key",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetIntAnnotation(tt.node, tt.key)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}
