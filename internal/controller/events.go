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
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// Event reasons for rolling update lifecycle.
const (
	// ReasonPoolOverlap indicates nodes match multiple pools.
	ReasonPoolOverlap = "PoolOverlap"

	// ReasonPoolOverlapResolved indicates overlap has been resolved.
	ReasonPoolOverlapResolved = "PoolOverlapResolved"

	// ReasonNodeCordon indicates a node was cordoned for update.
	ReasonNodeCordon = "NodeCordon"

	// ReasonNodeDrain indicates drain was started on a node.
	ReasonNodeDrain = "NodeDrain"

	// ReasonDrainStuck indicates drain has exceeded timeout.
	ReasonDrainStuck = "DrainStuck"

	// ReasonApplyTimeout indicates node apply has exceeded timeout.
	ReasonApplyTimeout = "ApplyTimeout"

	// ReasonDrainComplete indicates drain completed successfully.
	ReasonDrainComplete = "DrainComplete"

	// ReasonNodeUncordon indicates a node was uncordoned after update.
	ReasonNodeUncordon = "NodeUncordon"

	// ReasonRolloutBatch indicates a new batch of nodes started updating.
	ReasonRolloutBatch = "RolloutBatch"

	// ReasonRolloutComplete indicates all nodes have been updated.
	ReasonRolloutComplete = "RolloutComplete"

	// ReasonDrainFailed indicates a drain attempt failed (will retry).
	ReasonDrainFailed = "DrainFailed"
)

// EventRecorder provides methods to emit Kubernetes events for rolling update lifecycle.
type EventRecorder struct {
	recorder record.EventRecorder
}

// NewEventRecorder creates a new EventRecorder.
func NewEventRecorder(recorder record.EventRecorder) *EventRecorder {
	return &EventRecorder{recorder: recorder}
}

// PoolOverlapDetected emits a warning event when pool overlap is detected.
func (e *EventRecorder) PoolOverlapDetected(pool *mcov1alpha1.MachineConfigPool, nodes []string) {
	if e.recorder == nil {
		return
	}
	e.recorder.Eventf(pool, corev1.EventTypeWarning, ReasonPoolOverlap,
		"Pool overlap detected for nodes: %s", strings.Join(nodes, ", "))
}

// PoolOverlapResolved emits a normal event when pool overlap is resolved.
func (e *EventRecorder) PoolOverlapResolved(pool *mcov1alpha1.MachineConfigPool) {
	if e.recorder == nil {
		return
	}
	e.recorder.Event(pool, corev1.EventTypeNormal, ReasonPoolOverlapResolved,
		"Pool overlap resolved")
}

// NodeCordonStarted emits a WARNING event when a node is cordoned for update.
// Warning because cordon is a destructive action - node becomes unschedulable.
func (e *EventRecorder) NodeCordonStarted(pool *mcov1alpha1.MachineConfigPool, nodeName string) {
	if e.recorder == nil {
		return
	}
	e.recorder.Eventf(pool, corev1.EventTypeWarning, ReasonNodeCordon,
		"Node %s cordoned for update (unschedulable)", nodeName)
}

// NodeDrainStarted emits a WARNING event when drain is started on a node.
// Warning because drain is a destructive action - pods are being evicted.
func (e *EventRecorder) NodeDrainStarted(pool *mcov1alpha1.MachineConfigPool, nodeName string) {
	if e.recorder == nil {
		return
	}
	e.recorder.Eventf(pool, corev1.EventTypeWarning, ReasonNodeDrain,
		"Drain started on node %s (evicting pods)", nodeName)
}

// DrainStuck emits a warning event when drain exceeds timeout.
func (e *EventRecorder) DrainStuck(pool *mcov1alpha1.MachineConfigPool, nodeName string) {
	if e.recorder == nil {
		return
	}
	e.recorder.Eventf(pool, corev1.EventTypeWarning, ReasonDrainStuck,
		"Drain stuck on node %s, timeout exceeded", nodeName)
}

// ApplyTimeout emits a warning event when node apply exceeds timeout.
func (e *EventRecorder) ApplyTimeout(pool *mcov1alpha1.MachineConfigPool, nodeName string, timeoutSeconds int) {
	if e.recorder == nil {
		return
	}
	e.recorder.Eventf(pool, corev1.EventTypeWarning, ReasonApplyTimeout,
		"Node %s apply timeout exceeded (%ds)", nodeName, timeoutSeconds)
}

// DrainComplete emits a normal event when drain completes successfully.
func (e *EventRecorder) DrainComplete(pool *mcov1alpha1.MachineConfigPool, nodeName string) {
	if e.recorder == nil {
		return
	}
	e.recorder.Eventf(pool, corev1.EventTypeNormal, ReasonDrainComplete,
		"Drain completed on node %s", nodeName)
}

// NodeUncordoned emits a normal event when a node is uncordoned after update.
func (e *EventRecorder) NodeUncordoned(pool *mcov1alpha1.MachineConfigPool, nodeName string) {
	if e.recorder == nil {
		return
	}
	e.recorder.Eventf(pool, corev1.EventTypeNormal, ReasonNodeUncordon,
		"Node %s uncordoned after successful update", nodeName)
}

// RolloutBatchStarted emits a normal event when a new batch of nodes starts updating.
func (e *EventRecorder) RolloutBatchStarted(pool *mcov1alpha1.MachineConfigPool, nodeCount int, nodeNames []string) {
	if e.recorder == nil {
		return
	}
	if len(nodeNames) <= 3 {
		e.recorder.Eventf(pool, corev1.EventTypeNormal, ReasonRolloutBatch,
			"Starting update on %d nodes: %s", nodeCount, strings.Join(nodeNames, ", "))
	} else {
		e.recorder.Eventf(pool, corev1.EventTypeNormal, ReasonRolloutBatch,
			"Starting update on %d nodes: %s, ... and %d more",
			nodeCount, strings.Join(nodeNames[:3], ", "), nodeCount-3)
	}
}

// RolloutComplete emits a normal event when all nodes have been updated.
func (e *EventRecorder) RolloutComplete(pool *mcov1alpha1.MachineConfigPool) {
	if e.recorder == nil {
		return
	}
	e.recorder.Event(pool, corev1.EventTypeNormal, ReasonRolloutComplete,
		"All nodes updated to target revision")
}

// DrainFailed emits a warning event when a drain attempt fails (will be retried).
func (e *EventRecorder) DrainFailed(pool *mcov1alpha1.MachineConfigPool, nodeName, reason string) {
	if e.recorder == nil {
		return
	}
	e.recorder.Eventf(pool, corev1.EventTypeWarning, ReasonDrainFailed,
		"Drain failed on node %s: %s (will retry)", nodeName, reason)
}

// CreateEventRecorder creates an EventRecorder from a manager's scheme.
// This is a helper for setting up the recorder during manager initialization.
func CreateEventRecorder(mgr interface {
	GetEventRecorderFor(name string) record.EventRecorder
	GetScheme() *runtime.Scheme
}) *EventRecorder {
	return NewEventRecorder(mgr.GetEventRecorderFor("machineconfigpool-controller"))
}
