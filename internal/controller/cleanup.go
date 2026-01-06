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
	"sort"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/pkg/annotations"
)

// RMCCleaner handles cleanup of old RenderedMachineConfigs.
type RMCCleaner struct {
	client client.Client
}

// NewRMCCleaner creates a new RMC cleaner.
func NewRMCCleaner(c client.Client) *RMCCleaner {
	return &RMCCleaner{client: c}
}

// CleanupOldRMCs removes old RenderedMachineConfigs for a pool, keeping only
// the most recent ones up to the history limit. It never deletes RMCs that
// are still referenced by nodes.
//
// Returns the number of RMCs deleted and any error.
func (c *RMCCleaner) CleanupOldRMCs(ctx context.Context, pool *mcov1alpha1.MachineConfigPool, nodes []corev1.Node) (int, error) {
	limit := pool.Spec.RevisionHistory.Limit
	if limit == 0 {
		// 0 means unlimited retention
		return 0, nil
	}

	rmcs, err := c.listRMCsForPool(ctx, pool.Name)
	if err != nil {
		return 0, fmt.Errorf("failed to list RMCs: %w", err)
	}

	if len(rmcs) <= limit {
		// Nothing to clean up
		return 0, nil
	}

	inUse := c.getRevisionsInUse(nodes, pool.Status.TargetRevision)
	sort.Slice(rmcs, func(i, j int) bool {
		return rmcs[i].CreationTimestamp.Before(&rmcs[j].CreationTimestamp)
	})

	toDelete := len(rmcs) - limit
	deleted := 0
	for i := 0; i < len(rmcs) && deleted < toDelete; i++ {
		rmc := &rmcs[i]

		if inUse[rmc.Name] {
			continue
		}

		if err := c.client.Delete(ctx, rmc); err != nil {
			return deleted, fmt.Errorf("failed to delete RMC %s: %w", rmc.Name, err)
		}
		deleted++
	}

	return deleted, nil
}

// listRMCsForPool returns all RMCs belonging to a pool.
func (c *RMCCleaner) listRMCsForPool(ctx context.Context, poolName string) ([]mcov1alpha1.RenderedMachineConfig, error) {
	rmcList := &mcov1alpha1.RenderedMachineConfigList{}
	if err := c.client.List(ctx, rmcList, client.MatchingLabels{
		"mco.in-cloud.io/pool": poolName,
	}); err != nil {
		return nil, err
	}
	return rmcList.Items, nil
}

func (c *RMCCleaner) getRevisionsInUse(nodes []corev1.Node, target string) map[string]bool {
	inUse := make(map[string]bool)

	if target != "" {
		inUse[target] = true
	}

	for _, node := range nodes {
		current := annotations.GetAnnotation(node.Annotations, annotations.CurrentRevision)
		if current != "" {
			inUse[current] = true
		}

		desired := annotations.GetAnnotation(node.Annotations, annotations.DesiredRevision)
		if desired != "" {
			inUse[desired] = true
		}
	}

	return inUse
}

// CleanupOrphanedRMCs removes RMCs that belong to pools that no longer exist.
func (c *RMCCleaner) CleanupOrphanedRMCs(ctx context.Context) (int, error) {
	pools := &mcov1alpha1.MachineConfigPoolList{}
	if err := c.client.List(ctx, pools); err != nil {
		return 0, fmt.Errorf("failed to list pools: %w", err)
	}

	poolNames := make(map[string]bool)
	for _, p := range pools.Items {
		poolNames[p.Name] = true
	}

	rmcs := &mcov1alpha1.RenderedMachineConfigList{}
	if err := c.client.List(ctx, rmcs); err != nil {
		return 0, fmt.Errorf("failed to list RMCs: %w", err)
	}

	deleted := 0
	for i := range rmcs.Items {
		rmc := &rmcs.Items[i]
		poolName := rmc.Spec.PoolName
		if !poolNames[poolName] {
			if err := c.client.Delete(ctx, rmc); err != nil {
				return deleted, fmt.Errorf("failed to delete orphaned RMC %s: %w", rmc.Name, err)
			}
			deleted++
		}
	}

	return deleted, nil
}
