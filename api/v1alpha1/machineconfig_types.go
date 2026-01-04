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

// FileSpec defines a file to be managed on the host.
type FileSpec struct {
	// Path is the absolute path to the file on the host.
	// +kubebuilder:validation:Pattern=`^/.*`
	Path string `json:"path"`

	// Content is the file content. Required when state=present.
	// +kubebuilder:validation:MaxLength=1048576
	// +optional
	Content string `json:"content,omitempty"`

	// Mode is the Unix file permissions (e.g., 0644).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=4095
	// +kubebuilder:default=420
	// +optional
	Mode int `json:"mode,omitempty"`

	// Owner is the file owner in format "user:group" or "uid:gid".
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9_-]+:[a-zA-Z0-9_-]+$|^[0-9]+:[0-9]+$`
	// +kubebuilder:default="root:root"
	// +optional
	Owner string `json:"owner,omitempty"`

	// State is the desired state of the file: present or absent.
	// +kubebuilder:validation:Enum=present;absent
	// +kubebuilder:default="present"
	// +optional
	State string `json:"state,omitempty"`
}

// UnitSpec defines a systemd unit to be managed.
type UnitSpec struct {
	// Name is the unit name (e.g., "nginx.service").
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9@._-]+\.(service|socket|timer|mount|target)$`
	Name string `json:"name"`

	// Enabled sets whether the unit should start at boot.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// State is the desired runtime state: started, stopped, restarted, reloaded.
	// +kubebuilder:validation:Enum=started;stopped;restarted;reloaded
	// +optional
	State string `json:"state,omitempty"`

	// Mask completely disables the unit (cannot be started).
	// +kubebuilder:default=false
	// +optional
	Mask bool `json:"mask,omitempty"`
}

// SystemdSpec defines systemd configuration.
type SystemdSpec struct {
	// Units is the list of systemd units to manage.
	// +optional
	Units []UnitSpec `json:"units,omitempty"`
}

// RebootRequirementSpec defines reboot requirements for this configuration.
type RebootRequirementSpec struct {
	// Required indicates whether a reboot is needed after applying this config.
	// +kubebuilder:default=false
	// +optional
	Required bool `json:"required,omitempty"`

	// Reason is a human-readable explanation for why reboot is required.
	// This field is informational only and not included in hash computation.
	// +optional
	Reason string `json:"reason,omitempty"`
}

// MachineConfigSpec defines the desired state of MachineConfig.
type MachineConfigSpec struct {
	// Priority determines the merge order. Lower values are applied first,
	// higher values win on conflicts. Range: 0-99999.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=99999
	// +kubebuilder:default=50
	// +optional
	Priority int `json:"priority,omitempty"`

	// Files is the list of files to manage on the host.
	// +optional
	Files []FileSpec `json:"files,omitempty"`

	// Systemd defines systemd units to manage.
	// +optional
	Systemd SystemdSpec `json:"systemd,omitempty"`

	// Reboot defines reboot requirements for this configuration.
	// +optional
	Reboot RebootRequirementSpec `json:"reboot,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=mc
// +kubebuilder:printcolumn:name="Priority",type=integer,JSONPath=`.spec.priority`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// MachineConfig defines a fragment of host configuration.
// Multiple MachineConfigs are merged by the controller based on priority
// to produce a RenderedMachineConfig.
type MachineConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec MachineConfigSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// MachineConfigList contains a list of MachineConfig.
type MachineConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MachineConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MachineConfig{}, &MachineConfigList{})
}
