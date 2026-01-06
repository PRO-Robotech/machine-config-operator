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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// RolloutConfig defines rollout behavior for the pool.
type RolloutConfig struct {
	// DebounceSeconds is the time to wait after a config change before
	// rendering a new RenderedMachineConfig. This prevents rapid re-renders
	// when multiple MachineConfigs are updated in sequence.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=3600
	// +kubebuilder:default=30
	// +optional
	DebounceSeconds int `json:"debounceSeconds,omitempty"`

	// ApplyTimeoutSeconds is the maximum time to wait for a node to apply
	// a configuration before marking it as degraded.
	// +kubebuilder:validation:Minimum=60
	// +kubebuilder:validation:Maximum=3600
	// +kubebuilder:default=600
	// +optional
	ApplyTimeoutSeconds int `json:"applyTimeoutSeconds,omitempty"`

	// MaxUnavailable is the maximum number of nodes that can be unavailable
	// during an update. Value can be an absolute number (ex: 5) or a percentage
	// of total nodes (ex: "10%"). Defaults to 1.
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
}

// RebootPolicy defines the reboot behavior for nodes in the pool.
type RebootPolicy struct {
	// Strategy determines when nodes are allowed to reboot.
	// - Never: Nodes never reboot automatically (manual intervention required)
	// - IfRequired: Nodes reboot when a MachineConfig requires it
	// +kubebuilder:validation:Enum=Never;IfRequired
	// +kubebuilder:default="Never"
	// +optional
	Strategy string `json:"strategy,omitempty"`

	// MinIntervalSeconds is the minimum time between reboots for a single node.
	// This prevents reboot storms when multiple configs requiring reboot are applied.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1800
	// +optional
	MinIntervalSeconds int `json:"minIntervalSeconds,omitempty"`
}

// RevisionHistoryConfig defines how many old RenderedMachineConfigs to keep.
type RevisionHistoryConfig struct {
	// Limit is the maximum number of old RenderedMachineConfigs to retain.
	// Set to 0 for unlimited retention.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=5
	// +optional
	Limit int `json:"limit,omitempty"`
}

// MachineConfigPoolSpec defines the desired state of MachineConfigPool.
type MachineConfigPoolSpec struct {
	// NodeSelector selects nodes that belong to this pool.
	// +optional
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`

	// MachineConfigSelector selects MachineConfigs that apply to this pool.
	// +optional
	MachineConfigSelector *metav1.LabelSelector `json:"machineConfigSelector,omitempty"`

	// Rollout defines how configuration changes are rolled out to nodes.
	// +optional
	Rollout RolloutConfig `json:"rollout,omitempty"`

	// Reboot defines the reboot policy for nodes in this pool.
	// +optional
	Reboot RebootPolicy `json:"reboot,omitempty"`

	// RevisionHistory defines how many old RenderedMachineConfigs to keep.
	// +optional
	RevisionHistory RevisionHistoryConfig `json:"revisionHistory,omitempty"`

	// Paused stops all reconciliation for this pool when set to true.
	// No new RenderedMachineConfigs will be created and no nodes will be updated.
	// +kubebuilder:default=false
	// +optional
	Paused bool `json:"paused,omitempty"`
}

// MachineConfigPoolStatus defines the observed state of MachineConfigPool.
type MachineConfigPoolStatus struct {
	// TargetRevision is the name of the RenderedMachineConfig that nodes
	// should be converging to.
	// +optional
	TargetRevision string `json:"targetRevision,omitempty"`

	// CurrentRevision is the most common revision among nodes in the pool.
	// This represents the "current" state of the pool.
	// +optional
	CurrentRevision string `json:"currentRevision,omitempty"`

	// LastSuccessfulRevision is the last revision that was successfully
	// applied to all nodes in the pool.
	// +optional
	LastSuccessfulRevision string `json:"lastSuccessfulRevision,omitempty"`

	// MachineCount is the total number of nodes in this pool.
	MachineCount int `json:"machineCount"`

	// ReadyMachineCount is the number of nodes with current == target AND state == done.
	ReadyMachineCount int `json:"readyMachineCount"`

	// UpdatedMachineCount is the number of nodes with current == target.
	UpdatedMachineCount int `json:"updatedMachineCount"`

	// UpdatingMachineCount is the number of nodes with state == applying.
	UpdatingMachineCount int `json:"updatingMachineCount"`

	// DegradedMachineCount is the number of nodes with state == error.
	DegradedMachineCount int `json:"degradedMachineCount"`

	// UnavailableMachineCount is the number of nodes that are not ready.
	UnavailableMachineCount int `json:"unavailableMachineCount"`

	// PendingRebootCount is the number of nodes waiting for a reboot.
	PendingRebootCount int `json:"pendingRebootCount"`

	// Conditions represent the latest available observations of the pool's state.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// Condition types for MachineConfigPool.
const (
	// ConditionUpdated indicates all nodes have the target revision.
	ConditionUpdated string = "Updated"

	// ConditionUpdating indicates some nodes are still applying the target revision.
	ConditionUpdating string = "Updating"

	// ConditionDegraded indicates one or more nodes are in error state.
	ConditionDegraded string = "Degraded"

	// ConditionRenderDegraded indicates the renderer failed to create a RenderedMachineConfig.
	ConditionRenderDegraded string = "RenderDegraded"

	// ConditionPoolOverlap indicates nodes in this pool also match other pools.
	ConditionPoolOverlap string = "PoolOverlap"

	// ConditionDrainStuck indicates drain has been stuck for longer than timeout.
	ConditionDrainStuck string = "DrainStuck"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=mcp
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.status.targetRevision`
// +kubebuilder:printcolumn:name="Current",type=string,JSONPath=`.status.currentRevision`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyMachineCount`
// +kubebuilder:printcolumn:name="Updated",type=integer,JSONPath=`.status.updatedMachineCount`
// +kubebuilder:printcolumn:name="Degraded",type=integer,JSONPath=`.status.degradedMachineCount`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// MachineConfigPool groups nodes and MachineConfigs together.
// It defines which MachineConfigs apply to which nodes, and controls
// the rollout behavior and reboot policy.
type MachineConfigPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MachineConfigPoolSpec   `json:"spec,omitempty"`
	Status MachineConfigPoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MachineConfigPoolList contains a list of MachineConfigPool.
type MachineConfigPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MachineConfigPool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MachineConfigPool{}, &MachineConfigPoolList{})
}
