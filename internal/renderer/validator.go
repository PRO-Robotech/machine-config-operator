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

// Package renderer provides configuration rendering and validation.
package renderer

import (
	"fmt"
	"strings"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// ForbiddenPaths lists paths that cannot be managed by MachineConfig.
// These are critical system directories that should not be modified.
var ForbiddenPaths = []string{
	"/bin/",
	"/sbin/",
	"/usr/bin/",
	"/usr/sbin/",
	"/usr/lib/systemd/",
	"/lib/systemd/",
	"/etc/kubernetes/",
	"/etc/cni/",
	"/var/lib/kubelet/",
	"/var/lib/etcd/",
	"/proc/",
	"/sys/",
	"/dev/",
}

// ForbiddenUnits lists systemd units that cannot be managed.
// These are critical Kubernetes components.
var ForbiddenUnits = []string{
	"kubelet.service",
	"containerd.service",
	"docker.service",
	"cri-o.service",
	"etcd.service",
}

// ValidUnitSuffixes lists valid systemd unit type suffixes.
var ValidUnitSuffixes = []string{
	".service",
	".timer",
	".socket",
	".mount",
	".target",
}

// MaxFileContentSize is the maximum allowed file content size (1MB).
const MaxFileContentSize = 1024 * 1024

// IsPathForbidden checks if a path starts with any forbidden prefix.
func IsPathForbidden(path string) bool {
	for _, forbidden := range ForbiddenPaths {
		if strings.HasPrefix(path, forbidden) {
			return true
		}
	}
	return false
}

// IsUnitForbidden checks if a unit name is in the forbidden list.
func IsUnitForbidden(name string) bool {
	for _, forbidden := range ForbiddenUnits {
		if name == forbidden {
			return true
		}
	}
	return false
}

// HasValidUnitSuffix checks if a unit name has a valid suffix.
func HasValidUnitSuffix(name string) bool {
	for _, suffix := range ValidUnitSuffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

// ValidateFilePath validates a file path.
// Returns an error if the path is invalid or forbidden.
func ValidateFilePath(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("path must be absolute: %s", path)
	}

	if strings.Contains(path, "..") {
		return fmt.Errorf("path cannot contain '..': %s", path)
	}

	if strings.Contains(path, "//") {
		return fmt.Errorf("path cannot contain '//': %s", path)
	}

	if IsPathForbidden(path) {
		return fmt.Errorf("path is forbidden: %s", path)
	}

	return nil
}

// ValidateUnitName validates a systemd unit name.
// Returns an error if the name is invalid or forbidden.
func ValidateUnitName(name string) error {
	if name == "" {
		return fmt.Errorf("unit name cannot be empty")
	}

	if !HasValidUnitSuffix(name) {
		return fmt.Errorf("unit name must end with a valid suffix (%s): %s",
			strings.Join(ValidUnitSuffixes, ", "), name)
	}

	if IsUnitForbidden(name) {
		return fmt.Errorf("unit is forbidden: %s", name)
	}

	return nil
}

// ValidateFileSpec validates a FileSpec from a MachineConfig.
func ValidateFileSpec(f mcov1alpha1.FileSpec) error {
	if err := ValidateFilePath(f.Path); err != nil {
		return err
	}

	// Content is required when state is "present" (or empty, which defaults to "present")
	if (f.State == "" || f.State == "present") && f.Content == "" {
		return fmt.Errorf("content is required when state=present for path: %s", f.Path)
	}

	if len(f.Content) > MaxFileContentSize {
		return fmt.Errorf("content exceeds maximum size (%d bytes) for path: %s",
			MaxFileContentSize, f.Path)
	}

	return nil
}

// ValidateUnitSpec validates a UnitSpec from a MachineConfig.
func ValidateUnitSpec(u mcov1alpha1.UnitSpec) error {
	return ValidateUnitName(u.Name)
}

// ValidateMachineConfig validates an entire MachineConfig.
// Returns an error describing the first validation failure found.
func ValidateMachineConfig(mc *mcov1alpha1.MachineConfig) error {
	if mc == nil {
		return fmt.Errorf("MachineConfig cannot be nil")
	}

	for i, f := range mc.Spec.Files {
		if err := ValidateFileSpec(f); err != nil {
			return fmt.Errorf("files[%d]: %w", i, err)
		}
	}

	for i, u := range mc.Spec.Systemd.Units {
		if err := ValidateUnitSpec(u); err != nil {
			return fmt.Errorf("systemd.units[%d]: %w", i, err)
		}
	}

	return nil
}

// ValidateMachineConfigs validates a list of MachineConfigs.
// Returns an error if any config fails validation.
func ValidateMachineConfigs(configs []*mcov1alpha1.MachineConfig) error {
	for _, mc := range configs {
		if err := ValidateMachineConfig(mc); err != nil {
			return fmt.Errorf("MachineConfig %q: %w", mc.Name, err)
		}
	}
	return nil
}
