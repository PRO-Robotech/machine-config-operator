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
)

// RenderedConfig contains the merged configuration from all source MachineConfigs.
type RenderedConfig struct {
	// Files is the merged list of files to manage on the host.
	// Files are sorted by path and deduplicated (higher priority wins).
	// +optional
	Files []FileSpec `json:"files,omitempty"`

	// Systemd is the merged systemd configuration.
	// Units are deduplicated by name (higher priority wins).
	// +optional
	Systemd SystemdSpec `json:"systemd,omitempty"`
}

// ConfigSource identifies a MachineConfig that contributed to this render.
type ConfigSource struct {
	// Name is the MachineConfig name.
	Name string `json:"name"`

	// Priority is the priority value from the MachineConfig.
	Priority int `json:"priority"`
}

// RenderedRebootSpec contains reboot configuration for this rendered config.
type RenderedRebootSpec struct {
	// Required indicates whether a reboot is needed after applying this config.
	// This is the OR of all source MachineConfigs' reboot.required fields.
	Required bool `json:"required"`

	// Strategy is the reboot strategy from the MachineConfigPool.
	// +kubebuilder:validation:Enum=Never;IfRequired
	Strategy string `json:"strategy"`

	// MinIntervalSeconds is the minimum time between reboots from the pool.
	MinIntervalSeconds int `json:"minIntervalSeconds"`
}

// RebootRequirements contains per-component reboot requirements.
// This enables diff-based reboot determination: when transitioning between
// RMC versions, the agent can check which specific files/units changed
// and whether those changes require a reboot.
type RebootRequirements struct {
	// Files maps file paths to their reboot requirements.
	// Key: absolute file path (e.g., "/etc/sysctl.d/99-custom.conf")
	// Value: true if reboot is required when this file changes
	// +optional
	Files map[string]bool `json:"files,omitempty"`

	// Units maps systemd unit names to their reboot requirements.
	// Key: unit name (e.g., "containerd.service")
	// Value: true if reboot is required when this unit changes
	// +optional
	Units map[string]bool `json:"units,omitempty"`
}

// RenderedMachineConfigSpec defines the desired state of RenderedMachineConfig.
// This spec is immutable once created.
type RenderedMachineConfigSpec struct {
	// PoolName is the name of the MachineConfigPool this RMC belongs to.
	PoolName string `json:"poolName"`

	// Revision is a short identifier (first 10 characters of the config hash).
	// Used for display and quick identification.
	// +kubebuilder:validation:MaxLength=10
	Revision string `json:"revision"`

	// ConfigHash is the full SHA256 hash of the canonical configuration.
	// Used for collision detection and exact matching.
	// +kubebuilder:validation:Pattern=`^[a-f0-9]{64}$`
	ConfigHash string `json:"configHash"`

	// Config contains the merged configuration to be applied.
	Config RenderedConfig `json:"config"`

	// Sources lists the MachineConfigs that were merged to create this RMC.
	// Ordered by priority (lowest first).
	// +optional
	Sources []ConfigSource `json:"sources,omitempty"`

	// RebootRequirements contains per-file and per-unit reboot requirements.
	// Used by the Agent for diff-based reboot determination when transitioning
	// from one RMC to another. This field is populated by the Renderer based
	// on each source MachineConfig's reboot.required setting.
	// +optional
	RebootRequirements RebootRequirements `json:"rebootRequirements,omitempty"`

	// Reboot contains the legacy reboot configuration for this rendered config.
	// This is the OR of all source MachineConfigs' reboot.required fields.
	// Used for first apply and fallback scenarios.
	Reboot RenderedRebootSpec `json:"reboot"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=rmc
// +kubebuilder:printcolumn:name="Pool",type=string,JSONPath=`.spec.poolName`
// +kubebuilder:printcolumn:name="Revision",type=string,JSONPath=`.spec.revision`
// +kubebuilder:printcolumn:name="Reboot",type=boolean,JSONPath=`.spec.reboot.required`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// RenderedMachineConfig is an immutable snapshot of merged MachineConfigs.
// It represents the complete configuration that should be applied to nodes
// in a MachineConfigPool. Once created, it is never modified - new configurations
// result in new RenderedMachineConfig objects.
type RenderedMachineConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec RenderedMachineConfigSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// RenderedMachineConfigList contains a list of RenderedMachineConfig.
type RenderedMachineConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RenderedMachineConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RenderedMachineConfig{}, &RenderedMachineConfigList{})
}
