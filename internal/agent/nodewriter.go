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

package agent

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"

	"in-cloud.io/machine-config/pkg/annotations"
)

// NodeWriter writes agent annotations to the Node object.
// It uses JSON merge patch with retry on conflict.
type NodeWriter struct {
	client   kubernetes.Interface
	nodeName string
}

// NewNodeWriter creates a new NodeWriter for the specified node.
func NewNodeWriter(client kubernetes.Interface, nodeName string) *NodeWriter {
	return &NodeWriter{
		client:   client,
		nodeName: nodeName,
	}
}

// SetState sets the agent-state annotation.
func (w *NodeWriter) SetState(ctx context.Context, state string) error {
	return w.patchAnnotation(ctx, annotations.AgentState, state)
}

// SetCurrentRevision sets the current-revision annotation.
func (w *NodeWriter) SetCurrentRevision(ctx context.Context, revision string) error {
	return w.patchAnnotation(ctx, annotations.CurrentRevision, revision)
}

// SetLastError sets the last-error annotation.
func (w *NodeWriter) SetLastError(ctx context.Context, errMsg string) error {
	return w.patchAnnotation(ctx, annotations.LastError, errMsg)
}

// ClearLastError removes the last-error annotation.
func (w *NodeWriter) ClearLastError(ctx context.Context) error {
	return w.removeAnnotation(ctx, annotations.LastError)
}

// SetRebootPending sets or clears the reboot-pending annotation.
func (w *NodeWriter) SetRebootPending(ctx context.Context, pending bool) error {
	if pending {
		return w.patchAnnotation(ctx, annotations.RebootPending, "true")
	}
	return w.removeAnnotation(ctx, annotations.RebootPending)
}

// ClearForceReboot removes the force-reboot annotation.
func (w *NodeWriter) ClearForceReboot(ctx context.Context) error {
	return w.removeAnnotation(ctx, annotations.ForceReboot)
}

// SetStateWithError sets both state and last-error in a single patch.
func (w *NodeWriter) SetStateWithError(ctx context.Context, state, errMsg string) error {
	patch := fmt.Sprintf(
		`{"metadata":{"annotations":{%q:%q,%q:%q}}}`,
		annotations.AgentState, state,
		annotations.LastError, errMsg,
	)
	return w.patch(ctx, patch)
}

// SetDone sets state to done and updates current-revision in a single patch.
func (w *NodeWriter) SetDone(ctx context.Context, revision string) error {
	patch := fmt.Sprintf(
		`{"metadata":{"annotations":{%q:%q,%q:%q}}}`,
		annotations.AgentState, annotations.StateDone,
		annotations.CurrentRevision, revision,
	)
	return w.patch(ctx, patch)
}

func (w *NodeWriter) patchAnnotation(ctx context.Context, key, value string) error {
	patch := fmt.Sprintf(`{"metadata":{"annotations":{%q:%q}}}`, key, value)
	return w.patch(ctx, patch)
}

func (w *NodeWriter) removeAnnotation(ctx context.Context, key string) error {
	patch := fmt.Sprintf(`{"metadata":{"annotations":{%q:null}}}`, key)
	return w.patch(ctx, patch)
}

func (w *NodeWriter) patch(ctx context.Context, patchData string) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		_, err := w.client.CoreV1().Nodes().Patch(
			ctx,
			w.nodeName,
			types.MergePatchType,
			[]byte(patchData),
			metav1.PatchOptions{},
		)
		return err
	})
}
