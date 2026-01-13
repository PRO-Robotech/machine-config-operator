package controller

import (
	"math"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/pkg/annotations"
)

const (
	zoneLabel = "topology.kubernetes.io/zone"
)

func CalculateMaxUnavailable(maxUnavailable *intstr.IntOrString, nodeCount int) int {
	if maxUnavailable == nil {
		return 1
	}

	var effective int

	if maxUnavailable.Type == intstr.Int {
		effective = maxUnavailable.IntValue()
	} else {
		pctStr := maxUnavailable.StrVal
		pctStr = strings.TrimSuffix(pctStr, "%")
		pct, err := strconv.Atoi(pctStr)
		if err != nil {
			return 1
		}
		effective = int(math.Ceil(float64(nodeCount) * float64(pct) / 100.0))
	}

	if effective < 1 {
		effective = 1
	}

	return effective
}

func SortNodesForUpdate(nodes []corev1.Node) {
	sort.Slice(nodes, func(i, j int) bool {
		zoneI := nodes[i].Labels[zoneLabel]
		zoneJ := nodes[j].Labels[zoneLabel]

		if zoneI == "" && zoneJ != "" {
			return false
		}
		if zoneI != "" && zoneJ == "" {
			return true
		}
		if zoneI != zoneJ {
			return zoneI < zoneJ
		}

		timeI := nodes[i].CreationTimestamp.Time
		timeJ := nodes[j].CreationTimestamp.Time
		if !timeI.Equal(timeJ) {
			return timeI.Before(timeJ)
		}

		return nodes[i].Name < nodes[j].Name
	})
}

func IsNodeUnavailable(node *corev1.Node) bool {
	ann := node.Annotations

	// Paused nodes are NOT counted as unavailable.
	// They are intentionally paused for maintenance, not failing.
	// This prevents paused nodes from consuming maxUnavailable slots.
	// This check MUST be first, before any cordon/drain checks.
	if ann != nil && annotations.IsNodePaused(ann) {
		return false
	}

	// Check for manual cordon (kubectl cordon) - spec.unschedulable
	// This must be checked BEFORE MCO annotations to respect manual admin action.
	if node.Spec.Unschedulable {
		return true
	}

	if ann == nil {
		return false
	}

	if ann[annotations.Cordoned] == annotations.ValueTrue {
		return true
	}

	if ann[annotations.DrainStartedAt] != "" {
		return true
	}

	state := ann[annotations.AgentState]
	if state == "applying" || state == "rebooting" {
		return true
	}

	current := ann[annotations.CurrentRevision]
	desired := ann[annotations.DesiredRevision]
	if desired != "" && current != desired {
		return true
	}

	return false
}

func SelectNodesForUpdate(
	pool *mcov1alpha1.MachineConfigPool,
	allNodes []corev1.Node,
	targetRevision string,
) []corev1.Node {
	var needsUpdate []corev1.Node
	for _, node := range allNodes {
		ann := node.Annotations
		if ann == nil {
			ann = make(map[string]string)
		}

		// Skip paused nodes entirely - they should not be selected for update
		if annotations.IsNodePaused(ann) {
			continue
		}

		current := ann[annotations.CurrentRevision]
		// Only include nodes that are NOT already in progress (cordoned/draining)
		// Those are handled separately by collectNodesInProgress
		if current != targetRevision && !IsNodeUnavailable(&node) {
			needsUpdate = append(needsUpdate, node)
		}
	}

	if len(needsUpdate) == 0 {
		return nil
	}

	SortNodesForUpdate(needsUpdate)

	unavailableCount := 0
	for i := range allNodes {
		if IsNodeUnavailable(&allNodes[i]) {
			unavailableCount++
		}
	}

	maxUnavailable := CalculateMaxUnavailable(pool.Spec.Rollout.MaxUnavailable, len(allNodes))
	canUpdateCount := maxUnavailable - unavailableCount

	if canUpdateCount <= 0 {
		return nil
	}

	if canUpdateCount > len(needsUpdate) {
		canUpdateCount = len(needsUpdate)
	}

	return needsUpdate[:canUpdateCount]
}

// collectNodesInProgress returns nodes that are already in the update process
// (cordoned or draining) and haven't completed their update yet.
func collectNodesInProgress(allNodes []corev1.Node, targetRevision string) []corev1.Node {
	var inProgress []corev1.Node
	for _, node := range allNodes {
		ann := node.Annotations
		if ann == nil {
			ann = make(map[string]string)
		}

		// Skip paused nodes - they are intentionally paused
		if annotations.IsNodePaused(ann) {
			continue
		}

		// Node is in-progress if it's cordoned/draining but not yet at target revision
		current := ann[annotations.CurrentRevision]

		// Include nodes that are unavailable (cordoned/draining/applying).
		// Also include nodes that already reached the target revision but still
		// need lifecycle cleanup (uncordon + drain annotation cleanup).
		if IsNodeUnavailable(&node) && (current != targetRevision || ShouldUncordon(&node, targetRevision)) {
			inProgress = append(inProgress, node)
		}
	}
	return inProgress
}
