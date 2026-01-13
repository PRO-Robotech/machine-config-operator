package controller

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/pkg/annotations"
)

func TestCalculateMaxUnavailable_Nil(t *testing.T) {
	result := CalculateMaxUnavailable(nil, 10)
	if result != 1 {
		t.Errorf("expected 1, got %d", result)
	}
}

func TestCalculateMaxUnavailable_Integer(t *testing.T) {
	tests := []struct {
		name      string
		value     int
		nodeCount int
		expected  int
	}{
		{"integer 1", 1, 10, 1},
		{"integer 2", 2, 10, 2},
		{"integer 5", 5, 10, 5},
		{"integer larger than nodes", 15, 10, 15},
		{"integer 0 returns min 1", 0, 10, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val := intstr.FromInt(tt.value)
			result := CalculateMaxUnavailable(&val, tt.nodeCount)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestCalculateMaxUnavailable_Percentage(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		nodeCount int
		expected  int
	}{
		{"10% of 10 nodes", "10%", 10, 1},
		{"25% of 10 nodes", "25%", 10, 3},
		{"50% of 10 nodes", "50%", 10, 5},
		{"100% of 10 nodes", "100%", 10, 10},
		{"5% of 10 nodes (ceil)", "5%", 10, 1},
		{"1% of 10 nodes (min 1)", "1%", 10, 1},
		{"0% returns min 1", "0%", 10, 1},
		{"33% of 10 nodes (ceil)", "33%", 10, 4},
		{"20% of 100 nodes", "20%", 100, 20},
		{"15% of 7 nodes (ceil)", "15%", 7, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val := intstr.FromString(tt.value)
			result := CalculateMaxUnavailable(&val, tt.nodeCount)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestCalculateMaxUnavailable_InvalidPercentage(t *testing.T) {
	val := intstr.FromString("invalid")
	result := CalculateMaxUnavailable(&val, 10)
	if result != 1 {
		t.Errorf("expected 1 for invalid percentage, got %d", result)
	}
}

func TestSortNodesForUpdate_ByZone(t *testing.T) {
	now := time.Now()
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-c", Labels: map[string]string{zoneLabel: "zone-c"}, CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-a", Labels: map[string]string{zoneLabel: "zone-a"}, CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-b", Labels: map[string]string{zoneLabel: "zone-b"}, CreationTimestamp: metav1.Time{Time: now}}},
	}

	SortNodesForUpdate(nodes)

	expected := []string{"node-a", "node-b", "node-c"}
	for i, name := range expected {
		if nodes[i].Name != name {
			t.Errorf("position %d: expected %s, got %s", i, name, nodes[i].Name)
		}
	}
}

func TestSortNodesForUpdate_ByAge(t *testing.T) {
	now := time.Now()
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-new", Labels: map[string]string{zoneLabel: "zone-a"}, CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-old", Labels: map[string]string{zoneLabel: "zone-a"}, CreationTimestamp: metav1.Time{Time: now.Add(-time.Hour)}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-mid", Labels: map[string]string{zoneLabel: "zone-a"}, CreationTimestamp: metav1.Time{Time: now.Add(-30 * time.Minute)}}},
	}

	SortNodesForUpdate(nodes)

	expected := []string{"node-old", "node-mid", "node-new"}
	for i, name := range expected {
		if nodes[i].Name != name {
			t.Errorf("position %d: expected %s, got %s", i, name, nodes[i].Name)
		}
	}
}

func TestSortNodesForUpdate_NoZoneLast(t *testing.T) {
	now := time.Now()
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-no-zone", CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-with-zone", Labels: map[string]string{zoneLabel: "zone-a"}, CreationTimestamp: metav1.Time{Time: now}}},
	}

	SortNodesForUpdate(nodes)

	if nodes[0].Name != "node-with-zone" {
		t.Errorf("expected node-with-zone first, got %s", nodes[0].Name)
	}
	if nodes[1].Name != "node-no-zone" {
		t.Errorf("expected node-no-zone last, got %s", nodes[1].Name)
	}
}

func TestSortNodesForUpdate_TieBreaker(t *testing.T) {
	now := time.Now()
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-c", Labels: map[string]string{zoneLabel: "zone-a"}, CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-a", Labels: map[string]string{zoneLabel: "zone-a"}, CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-b", Labels: map[string]string{zoneLabel: "zone-a"}, CreationTimestamp: metav1.Time{Time: now}}},
	}

	SortNodesForUpdate(nodes)

	expected := []string{"node-a", "node-b", "node-c"}
	for i, name := range expected {
		if nodes[i].Name != name {
			t.Errorf("position %d: expected %s, got %s", i, name, nodes[i].Name)
		}
	}
}

func TestSortNodesForUpdate_Deterministic(t *testing.T) {
	now := time.Now()
	makeNodes := func() []corev1.Node {
		return []corev1.Node{
			{ObjectMeta: metav1.ObjectMeta{Name: "node-3", Labels: map[string]string{zoneLabel: "zone-b"}, CreationTimestamp: metav1.Time{Time: now}}},
			{ObjectMeta: metav1.ObjectMeta{Name: "node-1", Labels: map[string]string{zoneLabel: "zone-a"}, CreationTimestamp: metav1.Time{Time: now.Add(-time.Hour)}}},
			{ObjectMeta: metav1.ObjectMeta{Name: "node-2", Labels: map[string]string{zoneLabel: "zone-a"}, CreationTimestamp: metav1.Time{Time: now}}},
			{ObjectMeta: metav1.ObjectMeta{Name: "node-4", CreationTimestamp: metav1.Time{Time: now}}},
		}
	}

	nodes1 := makeNodes()
	nodes2 := makeNodes()

	SortNodesForUpdate(nodes1)
	SortNodesForUpdate(nodes2)

	for i := range nodes1 {
		if nodes1[i].Name != nodes2[i].Name {
			t.Errorf("sort not deterministic at position %d: %s vs %s", i, nodes1[i].Name, nodes2[i].Name)
		}
	}
}

func TestIsNodeUnavailable(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    bool
	}{
		{"nil annotations", nil, false},
		{"empty annotations", map[string]string{}, false},
		{"cordoned", map[string]string{annotations.Cordoned: "true"}, true},
		{"draining", map[string]string{annotations.DrainStartedAt: "2024-01-01T00:00:00Z"}, true},
		{"applying", map[string]string{annotations.AgentState: "applying"}, true},
		{"rebooting", map[string]string{annotations.AgentState: "rebooting"}, true},
		{"idle", map[string]string{annotations.AgentState: "idle"}, false},
		{"done", map[string]string{annotations.AgentState: "done"}, false},
		{"current != desired", map[string]string{
			annotations.CurrentRevision: "rev-1",
			annotations.DesiredRevision: "rev-2",
		}, true},
		{"current == desired", map[string]string{
			annotations.CurrentRevision: "rev-1",
			annotations.DesiredRevision: "rev-1",
		}, false},
		{"desired set, current empty", map[string]string{
			annotations.DesiredRevision: "rev-1",
		}, true},
		{"desired empty", map[string]string{
			annotations.CurrentRevision: "rev-1",
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Annotations: tt.annotations},
			}
			result := IsNodeUnavailable(node)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestSelectNodesForUpdate_NoNodesNeedUpdate(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{}
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-1", Annotations: map[string]string{annotations.CurrentRevision: "rev-1"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-2", Annotations: map[string]string{annotations.CurrentRevision: "rev-1"}}},
	}

	result := SelectNodesForUpdate(pool, nodes, "rev-1")
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestSelectNodesForUpdate_AllNodesNeedUpdate(t *testing.T) {
	pool := &mcov1alpha1.MachineConfigPool{}
	now := time.Now()
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-1", CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-2", CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-3", CreationTimestamp: metav1.Time{Time: now}}},
	}

	result := SelectNodesForUpdate(pool, nodes, "rev-1")
	if len(result) != 1 {
		t.Errorf("expected 1 node (default maxUnavailable), got %d", len(result))
	}
}

func TestSelectNodesForUpdate_RespectsMaxUnavailable(t *testing.T) {
	maxUnavailable := intstr.FromInt(2)
	pool := &mcov1alpha1.MachineConfigPool{
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Rollout: mcov1alpha1.RolloutConfig{
				MaxUnavailable: &maxUnavailable,
			},
		},
	}
	now := time.Now()
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-1", CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-2", CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-3", CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-4", CreationTimestamp: metav1.Time{Time: now}}},
	}

	result := SelectNodesForUpdate(pool, nodes, "rev-1")
	if len(result) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(result))
	}
}

func TestSelectNodesForUpdate_AlreadyUnavailable(t *testing.T) {
	maxUnavailable := intstr.FromInt(2)
	pool := &mcov1alpha1.MachineConfigPool{
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Rollout: mcov1alpha1.RolloutConfig{
				MaxUnavailable: &maxUnavailable,
			},
		},
	}
	now := time.Now()
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-1", CreationTimestamp: metav1.Time{Time: now}, Annotations: map[string]string{annotations.AgentState: "applying"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-2", CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-3", CreationTimestamp: metav1.Time{Time: now}}},
	}

	result := SelectNodesForUpdate(pool, nodes, "rev-1")
	if len(result) != 1 {
		t.Errorf("expected 1 node (2 max - 1 applying), got %d", len(result))
	}
}

func TestSelectNodesForUpdate_MaxReached(t *testing.T) {
	maxUnavailable := intstr.FromInt(1)
	pool := &mcov1alpha1.MachineConfigPool{
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Rollout: mcov1alpha1.RolloutConfig{
				MaxUnavailable: &maxUnavailable,
			},
		},
	}
	now := time.Now()
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-1", CreationTimestamp: metav1.Time{Time: now}, Annotations: map[string]string{annotations.AgentState: "applying"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-2", CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-3", CreationTimestamp: metav1.Time{Time: now}}},
	}

	result := SelectNodesForUpdate(pool, nodes, "rev-1")
	if result != nil {
		t.Errorf("expected nil (max reached), got %v", result)
	}
}

func TestSelectNodesForUpdate_Percentage(t *testing.T) {
	maxUnavailable := intstr.FromString("50%")
	pool := &mcov1alpha1.MachineConfigPool{
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Rollout: mcov1alpha1.RolloutConfig{
				MaxUnavailable: &maxUnavailable,
			},
		},
	}
	now := time.Now()
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-1", CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-2", CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-3", CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-4", CreationTimestamp: metav1.Time{Time: now}}},
	}

	result := SelectNodesForUpdate(pool, nodes, "rev-1")
	if len(result) != 2 {
		t.Errorf("expected 2 nodes (50%% of 4), got %d", len(result))
	}
}

func TestSelectNodesForUpdate_SortOrder(t *testing.T) {
	maxUnavailable := intstr.FromInt(3)
	pool := &mcov1alpha1.MachineConfigPool{
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Rollout: mcov1alpha1.RolloutConfig{
				MaxUnavailable: &maxUnavailable,
			},
		},
	}
	now := time.Now()
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-zone-b", Labels: map[string]string{zoneLabel: "zone-b"}, CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-zone-a-new", Labels: map[string]string{zoneLabel: "zone-a"}, CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-zone-a-old", Labels: map[string]string{zoneLabel: "zone-a"}, CreationTimestamp: metav1.Time{Time: now.Add(-time.Hour)}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-no-zone", CreationTimestamp: metav1.Time{Time: now}}},
	}

	result := SelectNodesForUpdate(pool, nodes, "rev-1")
	if len(result) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(result))
	}

	expected := []string{"node-zone-a-old", "node-zone-a-new", "node-zone-b"}
	for i, name := range expected {
		if result[i].Name != name {
			t.Errorf("position %d: expected %s, got %s", i, name, result[i].Name)
		}
	}
}

func TestSelectNodesForUpdate_PartialUpdate(t *testing.T) {
	maxUnavailable := intstr.FromInt(2)
	pool := &mcov1alpha1.MachineConfigPool{
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Rollout: mcov1alpha1.RolloutConfig{
				MaxUnavailable: &maxUnavailable,
			},
		},
	}
	now := time.Now()
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-1", CreationTimestamp: metav1.Time{Time: now}, Annotations: map[string]string{annotations.CurrentRevision: "rev-1"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-2", CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-3", CreationTimestamp: metav1.Time{Time: now}}},
	}

	result := SelectNodesForUpdate(pool, nodes, "rev-1")
	if len(result) != 2 {
		t.Errorf("expected 2 nodes (node-1 already updated), got %d", len(result))
	}
}

func TestSelectNodesForUpdate_ExcludesInProgressNodes(t *testing.T) {
	maxUnavailable := intstr.FromInt(3)
	pool := &mcov1alpha1.MachineConfigPool{
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Rollout: mcov1alpha1.RolloutConfig{
				MaxUnavailable: &maxUnavailable,
			},
		},
	}
	now := time.Now()
	nodes := []corev1.Node{
		// Node already cordoned (in-progress) - should NOT be in SelectNodesForUpdate result
		{ObjectMeta: metav1.ObjectMeta{Name: "node-cordoned", CreationTimestamp: metav1.Time{Time: now}, Annotations: map[string]string{annotations.Cordoned: "true"}}},
		// Normal nodes that need update
		{ObjectMeta: metav1.ObjectMeta{Name: "node-2", CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-3", CreationTimestamp: metav1.Time{Time: now}}},
	}

	result := SelectNodesForUpdate(pool, nodes, "rev-1")

	// Should only include node-2 and node-3 (not the cordoned one)
	if len(result) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(result))
	}

	// Verify cordoned node is not included
	for _, n := range result {
		if n.Name == "node-cordoned" {
			t.Error("cordoned node should not be in SelectNodesForUpdate result")
		}
	}
}

func TestCollectNodesInProgress_Empty(t *testing.T) {
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-2"}},
	}

	result := collectNodesInProgress(nodes, "rev-1")
	if len(result) != 0 {
		t.Errorf("expected 0 in-progress nodes, got %d", len(result))
	}
}

func TestCollectNodesInProgress_Cordoned(t *testing.T) {
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-cordoned", Annotations: map[string]string{annotations.Cordoned: "true"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-normal"}},
	}

	result := collectNodesInProgress(nodes, "rev-1")
	if len(result) != 1 {
		t.Errorf("expected 1 in-progress node, got %d", len(result))
	}
	if result[0].Name != "node-cordoned" {
		t.Errorf("expected node-cordoned, got %s", result[0].Name)
	}
}

func TestCollectNodesInProgress_Draining(t *testing.T) {
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-draining", Annotations: map[string]string{annotations.DrainStartedAt: "2024-01-01T00:00:00Z"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-normal"}},
	}

	result := collectNodesInProgress(nodes, "rev-1")
	if len(result) != 1 {
		t.Errorf("expected 1 in-progress node, got %d", len(result))
	}
	if result[0].Name != "node-draining" {
		t.Errorf("expected node-draining, got %s", result[0].Name)
	}
}

func TestCollectNodesInProgress_Applying(t *testing.T) {
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-applying", Annotations: map[string]string{annotations.AgentState: "applying"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-normal"}},
	}

	result := collectNodesInProgress(nodes, "rev-1")
	if len(result) != 1 {
		t.Errorf("expected 1 in-progress node, got %d", len(result))
	}
	if result[0].Name != "node-applying" {
		t.Errorf("expected node-applying, got %s", result[0].Name)
	}
}

func TestCollectNodesInProgress_AlreadyAtTarget(t *testing.T) {
	// Node is cordoned but already at target revision - should NOT be in progress
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-done", Annotations: map[string]string{
			annotations.Cordoned:        "true",
			annotations.CurrentRevision: "rev-1",
		}}},
	}

	result := collectNodesInProgress(nodes, "rev-1")
	if len(result) != 0 {
		t.Errorf("expected 0 in-progress nodes (already at target), got %d", len(result))
	}
}

func TestCollectNodesInProgress_Multiple(t *testing.T) {
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-cordoned", Annotations: map[string]string{annotations.Cordoned: "true"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-applying", Annotations: map[string]string{annotations.AgentState: "applying"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-normal"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-done", Annotations: map[string]string{annotations.CurrentRevision: "rev-1"}}},
	}

	result := collectNodesInProgress(nodes, "rev-1")
	if len(result) != 2 {
		t.Errorf("expected 2 in-progress nodes, got %d", len(result))
	}
}

func TestIsNodeUnavailable_PausedNotUnavailable(t *testing.T) {
	// Paused nodes should NOT be counted as unavailable.
	// This prevents paused nodes from consuming maxUnavailable slots.
	tests := []struct {
		name        string
		annotations map[string]string
		expected    bool
	}{
		{
			name: "paused node is not unavailable",
			annotations: map[string]string{
				annotations.Paused: "true",
			},
			expected: false,
		},
		{
			name: "paused + cordoned is not unavailable (pause takes precedence)",
			annotations: map[string]string{
				annotations.Paused:   "true",
				annotations.Cordoned: "true",
			},
			expected: false,
		},
		{
			name: "paused + draining is not unavailable",
			annotations: map[string]string{
				annotations.Paused:         "true",
				annotations.DrainStartedAt: "2024-01-01T00:00:00Z",
			},
			expected: false,
		},
		{
			name: "paused + applying is not unavailable",
			annotations: map[string]string{
				annotations.Paused:     "true",
				annotations.AgentState: "applying",
			},
			expected: false,
		},
		{
			name: "paused + revision mismatch is not unavailable",
			annotations: map[string]string{
				annotations.Paused:          "true",
				annotations.CurrentRevision: "rev-1",
				annotations.DesiredRevision: "rev-2",
			},
			expected: false,
		},
		{
			name: "not paused, cordoned is unavailable",
			annotations: map[string]string{
				annotations.Cordoned: "true",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Annotations: tt.annotations},
			}
			result := IsNodeUnavailable(node)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestSelectNodesForUpdate_SkipsPausedNodes(t *testing.T) {
	// Paused nodes should be completely skipped from update selection.
	// They should not be selected for update regardless of their revision.
	maxUnavailable := intstr.FromInt(3)
	pool := &mcov1alpha1.MachineConfigPool{
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Rollout: mcov1alpha1.RolloutConfig{
				MaxUnavailable: &maxUnavailable,
			},
		},
	}
	now := time.Now()
	nodes := []corev1.Node{
		// Paused node that needs update - should be skipped
		{ObjectMeta: metav1.ObjectMeta{
			Name:              "node-paused",
			CreationTimestamp: metav1.Time{Time: now},
			Annotations:       map[string]string{annotations.Paused: "true"},
		}},
		// Normal nodes that need update
		{ObjectMeta: metav1.ObjectMeta{Name: "node-2", CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-3", CreationTimestamp: metav1.Time{Time: now}}},
	}

	result := SelectNodesForUpdate(pool, nodes, "rev-1")

	// Should only include node-2 and node-3 (not the paused one)
	if len(result) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(result))
	}

	// Verify paused node is not included
	for _, n := range result {
		if n.Name == "node-paused" {
			t.Error("paused node should not be selected for update")
		}
	}
}

func TestSelectNodesForUpdate_PausedDoesNotConsumeMaxUnavailable(t *testing.T) {
	// Paused nodes should NOT count against maxUnavailable limit.
	// This allows rollout to continue for other nodes even when some are paused.
	maxUnavailable := intstr.FromInt(1)
	pool := &mcov1alpha1.MachineConfigPool{
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Rollout: mcov1alpha1.RolloutConfig{
				MaxUnavailable: &maxUnavailable,
			},
		},
	}
	now := time.Now()
	nodes := []corev1.Node{
		// Paused node - should NOT count as unavailable
		{ObjectMeta: metav1.ObjectMeta{
			Name:              "node-paused",
			CreationTimestamp: metav1.Time{Time: now},
			Annotations:       map[string]string{annotations.Paused: "true"},
		}},
		// Normal node that needs update
		{ObjectMeta: metav1.ObjectMeta{Name: "node-2", CreationTimestamp: metav1.Time{Time: now}}},
		// Another normal node
		{ObjectMeta: metav1.ObjectMeta{Name: "node-3", CreationTimestamp: metav1.Time{Time: now}}},
	}

	result := SelectNodesForUpdate(pool, nodes, "rev-1")

	// With maxUnavailable=1 and no unavailable nodes (paused doesn't count),
	// we should get 1 node selected for update
	if len(result) != 1 {
		t.Errorf("expected 1 node (maxUnavailable=1), got %d", len(result))
	}
}

func TestCollectNodesInProgress_SkipsPausedNodes(t *testing.T) {
	// Paused nodes should not be collected as in-progress,
	// even if they have cordoned/draining annotations.
	nodes := []corev1.Node{
		// Paused + cordoned - should NOT be collected
		{ObjectMeta: metav1.ObjectMeta{
			Name: "node-paused-cordoned",
			Annotations: map[string]string{
				annotations.Paused:   "true",
				annotations.Cordoned: "true",
			},
		}},
		// Normal cordoned - should be collected
		{ObjectMeta: metav1.ObjectMeta{
			Name:        "node-cordoned",
			Annotations: map[string]string{annotations.Cordoned: "true"},
		}},
		// Normal node
		{ObjectMeta: metav1.ObjectMeta{Name: "node-normal"}},
	}

	result := collectNodesInProgress(nodes, "rev-1")

	if len(result) != 1 {
		t.Errorf("expected 1 in-progress node, got %d", len(result))
	}
	if len(result) > 0 && result[0].Name != "node-cordoned" {
		t.Errorf("expected node-cordoned, got %s", result[0].Name)
	}
}

func TestRollingUpdate_ContinuesPastPausedNodes(t *testing.T) {
	// Integration test: rolling update should continue even when
	// some nodes are paused. Paused nodes are completely excluded
	// from the rollout calculation.
	maxUnavailable := intstr.FromInt(2)
	pool := &mcov1alpha1.MachineConfigPool{
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Rollout: mcov1alpha1.RolloutConfig{
				MaxUnavailable: &maxUnavailable,
			},
		},
	}
	now := time.Now()
	nodes := []corev1.Node{
		// 2 paused nodes - should be completely ignored
		{ObjectMeta: metav1.ObjectMeta{
			Name:              "node-paused-1",
			CreationTimestamp: metav1.Time{Time: now},
			Annotations:       map[string]string{annotations.Paused: "true"},
		}},
		{ObjectMeta: metav1.ObjectMeta{
			Name:              "node-paused-2",
			CreationTimestamp: metav1.Time{Time: now},
			Annotations:       map[string]string{annotations.Paused: "true"},
		}},
		// 1 node already applying - counts as unavailable
		{ObjectMeta: metav1.ObjectMeta{
			Name:              "node-applying",
			CreationTimestamp: metav1.Time{Time: now},
			Annotations:       map[string]string{annotations.AgentState: "applying"},
		}},
		// 3 nodes ready for update
		{ObjectMeta: metav1.ObjectMeta{Name: "node-ready-1", CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-ready-2", CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-ready-3", CreationTimestamp: metav1.Time{Time: now}}},
	}

	result := SelectNodesForUpdate(pool, nodes, "rev-1")

	// With maxUnavailable=2, and 1 applying node:
	// - Paused nodes don't count as unavailable
	// - 1 slot is used by applying node
	// - 1 slot available for new updates
	// So we should select 1 node
	if len(result) != 1 {
		t.Errorf("expected 1 node (2 max - 1 applying, paused ignored), got %d", len(result))
	}

	// Verify no paused nodes in result
	for _, n := range result {
		if n.Name == "node-paused-1" || n.Name == "node-paused-2" {
			t.Errorf("paused node %s should not be in result", n.Name)
		}
	}
}

func TestIsNodeUnavailable_ManualCordon(t *testing.T) {
	// Node with Spec.Unschedulable=true (kubectl cordon) should be unavailable
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
		Spec: corev1.NodeSpec{
			Unschedulable: true,
		},
	}

	if !IsNodeUnavailable(node) {
		t.Error("manually cordoned node (Unschedulable=true) should be unavailable")
	}
}

func TestIsNodeUnavailable_ManualCordonNoAnnotations(t *testing.T) {
	// Node with Spec.Unschedulable=true and no annotations
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-1",
			// No annotations at all
		},
		Spec: corev1.NodeSpec{
			Unschedulable: true,
		},
	}

	if !IsNodeUnavailable(node) {
		t.Error("manually cordoned node without annotations should be unavailable")
	}
}

func TestIsNodeUnavailable_NotCordoned(t *testing.T) {
	// Normal node - not cordoned
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-1",
			Annotations: map[string]string{
				annotations.AgentState: annotations.StateIdle,
			},
		},
		Spec: corev1.NodeSpec{
			Unschedulable: false,
		},
	}

	if IsNodeUnavailable(node) {
		t.Error("normal node should not be unavailable")
	}
}

// TestIsNodeUnavailable_PausedAndUnschedulable verifies that paused nodes are NOT
// counted as unavailable even when Spec.Unschedulable=true.
func TestIsNodeUnavailable_PausedAndUnschedulable(t *testing.T) {
	// A paused node that was cordoned (Unschedulable=true) should NOT be unavailable
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "paused-cordoned-worker",
			Annotations: map[string]string{
				annotations.Paused: "true",
			},
		},
		Spec: corev1.NodeSpec{
			Unschedulable: true, // Cordoned
		},
	}

	if IsNodeUnavailable(node) {
		t.Error("paused node with Unschedulable=true should NOT be unavailable")
	}
}

// TestIsNodeUnavailable_PausedAndMCOCordoned verifies paused nodes with MCO cordon annotation.
func TestIsNodeUnavailable_PausedAndMCOCordoned(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "paused-mco-cordoned",
			Annotations: map[string]string{
				annotations.Paused:   "true",
				annotations.Cordoned: "true",
			},
		},
		Spec: corev1.NodeSpec{
			Unschedulable: true,
		},
	}

	if IsNodeUnavailable(node) {
		t.Error("paused node with MCO cordoned annotation should NOT be unavailable")
	}
}

// TestIsNodeUnavailable_PausedAndDraining verifies paused nodes during drain.
func TestIsNodeUnavailable_PausedAndDraining(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "paused-draining",
			Annotations: map[string]string{
				annotations.Paused:         "true",
				annotations.DrainStartedAt: "2026-01-08T12:00:00Z",
			},
		},
		Spec: corev1.NodeSpec{
			Unschedulable: true,
		},
	}

	if IsNodeUnavailable(node) {
		t.Error("paused node with drain-started-at annotation should NOT be unavailable")
	}
}

// TestIsNodeUnavailable_PausedAndApplying verifies paused nodes with agent applying.
func TestIsNodeUnavailable_PausedAndApplying(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "paused-applying",
			Annotations: map[string]string{
				annotations.Paused:     "true",
				annotations.AgentState: "applying",
			},
		},
	}

	if IsNodeUnavailable(node) {
		t.Error("paused node with agent-state=applying should NOT be unavailable")
	}
}

func TestSelectNodesForUpdate_RespectsManualCordon(t *testing.T) {
	// Manual cordon should count against maxUnavailable
	maxUnavailable := intstr.FromInt(2)
	pool := &mcov1alpha1.MachineConfigPool{
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Rollout: mcov1alpha1.RolloutConfig{
				MaxUnavailable: &maxUnavailable,
			},
		},
	}
	now := time.Now()
	nodes := []corev1.Node{
		// Manually cordoned node - counts as unavailable
		{
			ObjectMeta: metav1.ObjectMeta{Name: "node-manual-cordon", CreationTimestamp: metav1.Time{Time: now}},
			Spec:       corev1.NodeSpec{Unschedulable: true},
		},
		// MCO cordoned node - also counts as unavailable
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "node-mco-cordon",
				CreationTimestamp: metav1.Time{Time: now},
				Annotations:       map[string]string{annotations.Cordoned: "true"},
			},
		},
		// Normal nodes needing update
		{ObjectMeta: metav1.ObjectMeta{Name: "node-ready-1", CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-ready-2", CreationTimestamp: metav1.Time{Time: now}}},
	}

	result := SelectNodesForUpdate(pool, nodes, "rev-1")

	// maxUnavailable=2, 2 are already unavailable (manual + MCO cordon)
	// So no new nodes should be selected
	if len(result) != 0 {
		t.Errorf("expected nil or empty (maxUnavailable reached), got %d nodes", len(result))
	}
}

func TestSelectNodesForUpdate_ManualCordonPartialCapacity(t *testing.T) {
	// One manual cordon leaves room for one more update
	maxUnavailable := intstr.FromInt(2)
	pool := &mcov1alpha1.MachineConfigPool{
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Rollout: mcov1alpha1.RolloutConfig{
				MaxUnavailable: &maxUnavailable,
			},
		},
	}
	now := time.Now()
	nodes := []corev1.Node{
		// Manually cordoned node - counts as unavailable
		{
			ObjectMeta: metav1.ObjectMeta{Name: "node-manual-cordon", CreationTimestamp: metav1.Time{Time: now}},
			Spec:       corev1.NodeSpec{Unschedulable: true},
		},
		// Normal nodes needing update
		{ObjectMeta: metav1.ObjectMeta{Name: "node-ready-1", CreationTimestamp: metav1.Time{Time: now}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-ready-2", CreationTimestamp: metav1.Time{Time: now}}},
	}

	result := SelectNodesForUpdate(pool, nodes, "rev-1")

	// maxUnavailable=2, 1 manual cordon leaves room for 1 update
	if len(result) != 1 {
		t.Errorf("expected 1 node (maxUnavailable=2, 1 manual cordon), got %d", len(result))
	}
}
