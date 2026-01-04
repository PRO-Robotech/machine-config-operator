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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"in-cloud.io/machine-config/pkg/annotations"
)

// NodeAnnotator writes controller-owned annotations to nodes.
// It handles the desired-revision and pool annotations.
type NodeAnnotator struct {
	client client.Client
}

// NewNodeAnnotator creates a new NodeAnnotator.
func NewNodeAnnotator(c client.Client) *NodeAnnotator {
	return &NodeAnnotator{client: c}
}

// SetDesiredRevision sets the desired-revision and pool annotations on a node.
// Uses JSON merge patch for atomic update with retry on conflict.
func (a *NodeAnnotator) SetDesiredRevision(ctx context.Context, nodeName, revision, pool string) error {
	patch := fmt.Sprintf(
		`{"metadata":{"annotations":{%q:%q,%q:%q}}}`,
		annotations.DesiredRevision, revision,
		annotations.Pool, pool,
	)
	return a.patch(ctx, nodeName, patch)
}

// RemoveDesiredRevision removes the desired-revision and pool annotations from a node.
// This is called when a node leaves a pool.
func (a *NodeAnnotator) RemoveDesiredRevision(ctx context.Context, nodeName string) error {
	patch := fmt.Sprintf(
		`{"metadata":{"annotations":{%q:null,%q:null}}}`,
		annotations.DesiredRevision,
		annotations.Pool,
	)
	return a.patch(ctx, nodeName, patch)
}

// SetDesiredRevisionForNodes sets the desired-revision on multiple nodes.
// Skips paused nodes. Returns the count of nodes updated and any error.
func (a *NodeAnnotator) SetDesiredRevisionForNodes(ctx context.Context, nodes []corev1.Node, revision, pool string) (int, error) {
	updated := 0
	for _, node := range nodes {
		// Skip paused nodes
		if annotations.IsNodePaused(node.Annotations) {
			continue
		}

		if annotations.GetAnnotation(node.Annotations, annotations.DesiredRevision) == revision {
			continue
		}

		if err := a.SetDesiredRevision(ctx, node.Name, revision, pool); err != nil {
			return updated, fmt.Errorf("failed to set desired revision on node %s: %w", node.Name, err)
		}
		updated++
	}
	return updated, nil
}

func (a *NodeAnnotator) patch(ctx context.Context, nodeName, patch string) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		node := &corev1.Node{}
		if err := a.client.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
			return err
		}

		return a.client.Patch(
			ctx,
			node,
			client.RawPatch(types.MergePatchType, []byte(patch)),
		)
	})
}
