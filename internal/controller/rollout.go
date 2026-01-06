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
	if ann == nil {
		return false
	}

	if ann[annotations.Cordoned] == "true" {
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
		current := ""
		if node.Annotations != nil {
			current = node.Annotations[annotations.CurrentRevision]
		}
		if current != targetRevision {
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
