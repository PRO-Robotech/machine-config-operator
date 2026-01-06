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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// SelectNodes returns nodes matching the pool's nodeSelector.
// If nodeSelector is nil or empty, returns all nodes.
func SelectNodes(ctx context.Context, c client.Client, pool *mcov1alpha1.MachineConfigPool) ([]corev1.Node, error) {
	selector, err := selectorFromLabelSelector(pool.Spec.NodeSelector)
	if err != nil {
		return nil, fmt.Errorf("invalid nodeSelector: %w", err)
	}

	nodeList := &corev1.NodeList{}
	listOpts := &client.ListOptions{
		LabelSelector: selector,
	}

	if err := c.List(ctx, nodeList, listOpts); err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	return nodeList.Items, nil
}

// SelectMachineConfigs returns MachineConfigs matching the pool's machineConfigSelector.
// If machineConfigSelector is nil or empty, returns all MachineConfigs.
func SelectMachineConfigs(ctx context.Context, c client.Client, pool *mcov1alpha1.MachineConfigPool) ([]mcov1alpha1.MachineConfig, error) {
	selector, err := selectorFromLabelSelector(pool.Spec.MachineConfigSelector)
	if err != nil {
		return nil, fmt.Errorf("invalid machineConfigSelector: %w", err)
	}

	mcList := &mcov1alpha1.MachineConfigList{}
	listOpts := &client.ListOptions{
		LabelSelector: selector,
	}

	if err := c.List(ctx, mcList, listOpts); err != nil {
		return nil, fmt.Errorf("failed to list MachineConfigs: %w", err)
	}

	return mcList.Items, nil
}

func selectorFromLabelSelector(ls *metav1.LabelSelector) (labels.Selector, error) {
	if ls == nil {
		return labels.Everything(), nil
	}

	selector, err := metav1.LabelSelectorAsSelector(ls)
	if err != nil {
		return nil, err
	}

	return selector, nil
}

func NodesForPool(ctx context.Context, c client.Client, pool *mcov1alpha1.MachineConfigPool) ([]corev1.Node, error) {
	return SelectNodes(ctx, c, pool)
}

func MachineConfigsForPool(ctx context.Context, c client.Client, pool *mcov1alpha1.MachineConfigPool) ([]mcov1alpha1.MachineConfig, error) {
	return SelectMachineConfigs(ctx, c, pool)
}

// NodeMatchesPool checks if a single node matches the pool's nodeSelector.
func NodeMatchesPool(node *corev1.Node, pool *mcov1alpha1.MachineConfigPool) (bool, error) {
	selector, err := selectorFromLabelSelector(pool.Spec.NodeSelector)
	if err != nil {
		return false, fmt.Errorf("invalid nodeSelector: %w", err)
	}

	return selector.Matches(labels.Set(node.Labels)), nil
}

// MachineConfigMatchesPool checks if a single MachineConfig matches the pool's machineConfigSelector.
func MachineConfigMatchesPool(mc *mcov1alpha1.MachineConfig, pool *mcov1alpha1.MachineConfigPool) (bool, error) {
	selector, err := selectorFromLabelSelector(pool.Spec.MachineConfigSelector)
	if err != nil {
		return false, fmt.Errorf("invalid machineConfigSelector: %w", err)
	}

	return selector.Matches(labels.Set(mc.Labels)), nil
}
