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
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

type OverlapResult struct {
	ConflictingNodes map[string][]string
}

func NewOverlapResult() *OverlapResult {
	return &OverlapResult{
		ConflictingNodes: make(map[string][]string),
	}
}

func (r *OverlapResult) HasConflicts() bool {
	return len(r.ConflictingNodes) > 0
}

func (r *OverlapResult) ConflictCount() int {
	return len(r.ConflictingNodes)
}

func (r *OverlapResult) GetConflictsForPool(poolName string) []string {
	var nodes []string
	for nodeName, pools := range r.ConflictingNodes {
		for _, p := range pools {
			if p == poolName {
				nodes = append(nodes, nodeName)
				break
			}
		}
	}
	sort.Strings(nodes)
	return nodes
}

func (r *OverlapResult) GetPoolsForNode(nodeName string) []string {
	pools, exists := r.ConflictingNodes[nodeName]
	if !exists {
		return nil
	}
	result := make([]string, len(pools))
	copy(result, pools)
	return result
}

func (r *OverlapResult) IsNodeConflicting(nodeName string) bool {
	_, exists := r.ConflictingNodes[nodeName]
	return exists
}

func (r *OverlapResult) GetAllConflictingPools() []string {
	poolSet := make(map[string]struct{})
	for _, pools := range r.ConflictingNodes {
		for _, pool := range pools {
			poolSet[pool] = struct{}{}
		}
	}

	result := make([]string, 0, len(poolSet))
	for pool := range poolSet {
		result = append(result, pool)
	}
	sort.Strings(result)
	return result
}

func DetectPoolOverlap(ctx context.Context, c client.Client) (*OverlapResult, error) {
	pools := &mcov1alpha1.MachineConfigPoolList{}
	if err := c.List(ctx, pools); err != nil {
		return nil, err
	}

	nodes := &corev1.NodeList{}
	if err := c.List(ctx, nodes); err != nil {
		return nil, err
	}

	return DetectPoolOverlapFromLists(pools.Items, nodes.Items)
}

func DetectPoolOverlapFromLists(pools []mcov1alpha1.MachineConfigPool, nodes []corev1.Node) (*OverlapResult, error) {
	nodeToPool := make(map[string][]string)

	for i := range nodes {
		node := &nodes[i]
		for j := range pools {
			pool := &pools[j]

			matches, err := nodeMatchesPoolSelector(node, pool)
			if err != nil {
				continue
			}
			if matches {
				nodeToPool[node.Name] = append(nodeToPool[node.Name], pool.Name)
			}
		}
	}

	result := NewOverlapResult()

	for nodeName, poolNames := range nodeToPool {
		if len(poolNames) > 1 {
			sort.Strings(poolNames)
			result.ConflictingNodes[nodeName] = poolNames
		}
	}

	return result, nil
}

func nodeMatchesPoolSelector(node *corev1.Node, pool *mcov1alpha1.MachineConfigPool) (bool, error) {
	if pool.Spec.NodeSelector == nil {
		return true, nil
	}

	selector, err := metav1.LabelSelectorAsSelector(pool.Spec.NodeSelector)
	if err != nil {
		return false, err
	}

	return selector.Matches(labels.Set(node.Labels)), nil
}

func FilterNonConflictingNodes(nodes []corev1.Node, overlap *OverlapResult) []corev1.Node {
	if overlap == nil || !overlap.HasConflicts() {
		return nodes
	}

	result := make([]corev1.Node, 0, len(nodes))
	for _, node := range nodes {
		if !overlap.IsNodeConflicting(node.Name) {
			result = append(result, node)
		}
	}
	return result
}
