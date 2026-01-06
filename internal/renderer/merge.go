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

package renderer

import (
	"sort"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// ConfigSource tracks which MachineConfig contributed to the merged result.
// This is used for auditing and debugging.
type ConfigSource struct {
	// Name is the MachineConfig name.
	Name string `json:"name"`
	// Priority is the MachineConfig priority at merge time.
	Priority int `json:"priority"`
}

// MergedConfig is the result of merging multiple MachineConfigs.
// Files and Units are deduplicated by path/name, with higher priority winning.
type MergedConfig struct {
	// Files is the merged list of files, sorted by path.
	Files []mcov1alpha1.FileSpec `json:"files,omitempty"`
	// Units is the merged list of systemd units, sorted by name.
	Units []mcov1alpha1.UnitSpec `json:"units,omitempty"`
	// RebootRequired is true if ANY source config requires reboot.
	// This is the legacy OR-based logic used for first apply and fallback.
	RebootRequired bool `json:"rebootRequired"`
	// Sources lists all MachineConfigs that were merged, in priority order.
	Sources []ConfigSource `json:"sources"`

	// FileRebootRequirements maps file paths to their reboot requirements.
	// For each file, this indicates whether the winning MachineConfig
	// (the one that contributed this file) has reboot.required=true.
	// Used for diff-based reboot determination.
	FileRebootRequirements map[string]bool `json:"fileRebootRequirements,omitempty"`

	// UnitRebootRequirements maps unit names to their reboot requirements.
	// For each unit, this indicates whether the winning MachineConfig
	// (the one that contributed this unit) has reboot.required=true.
	// Used for diff-based reboot determination.
	UnitRebootRequirements map[string]bool `json:"unitRebootRequirements,omitempty"`
}

// Merge combines multiple MachineConfigs into a single MergedConfig.
// Returns an empty MergedConfig if configs is empty (not an error).
func Merge(configs []*mcov1alpha1.MachineConfig) *MergedConfig {
	if len(configs) == 0 {
		return &MergedConfig{
			Files:                  []mcov1alpha1.FileSpec{},
			Units:                  []mcov1alpha1.UnitSpec{},
			Sources:                []ConfigSource{},
			FileRebootRequirements: map[string]bool{},
			UnitRebootRequirements: map[string]bool{},
		}
	}

	sorted := sortByPriority(configs)

	filesByPath := make(map[string]mcov1alpha1.FileSpec)
	unitsByName := make(map[string]mcov1alpha1.UnitSpec)

	fileSourceReboot := make(map[string]bool)
	unitSourceReboot := make(map[string]bool)

	rebootRequired := false
	sources := make([]ConfigSource, 0, len(sorted))

	for _, mc := range sorted {
		for _, f := range mc.Spec.Files {
			filesByPath[f.Path] = f
			fileSourceReboot[f.Path] = mc.Spec.Reboot.Required
		}

		for _, u := range mc.Spec.Systemd.Units {
			unitsByName[u.Name] = u
			unitSourceReboot[u.Name] = mc.Spec.Reboot.Required
		}

		if mc.Spec.Reboot.Required {
			rebootRequired = true
		}

		sources = append(sources, ConfigSource{
			Name:     mc.Name,
			Priority: mc.Spec.Priority,
		})
	}

	files := filesToSortedSlice(filesByPath)
	units := unitsToSortedSlice(unitsByName)

	return &MergedConfig{
		Files:                  files,
		Units:                  units,
		RebootRequired:         rebootRequired,
		Sources:                sources,
		FileRebootRequirements: fileSourceReboot,
		UnitRebootRequirements: unitSourceReboot,
	}
}

// sortByPriority returns a new slice sorted by priority ASC, then name ASC.
// This ensures lower priority configs are applied first (and overwritten by higher).
func sortByPriority(configs []*mcov1alpha1.MachineConfig) []*mcov1alpha1.MachineConfig {
	sorted := make([]*mcov1alpha1.MachineConfig, len(configs))
	copy(sorted, configs)

	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Spec.Priority != sorted[j].Spec.Priority {
			return sorted[i].Spec.Priority < sorted[j].Spec.Priority
		}
		return sorted[i].Name < sorted[j].Name
	})

	return sorted
}

// filesToSortedSlice converts a map of files to a sorted slice.
// Files are sorted by path for deterministic output.
func filesToSortedSlice(files map[string]mcov1alpha1.FileSpec) []mcov1alpha1.FileSpec {
	if len(files) == 0 {
		return []mcov1alpha1.FileSpec{}
	}

	result := make([]mcov1alpha1.FileSpec, 0, len(files))
	for _, f := range files {
		result = append(result, f)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Path < result[j].Path
	})

	return result
}

// unitsToSortedSlice converts a map of units to a sorted slice.
// Units are sorted by name for deterministic output.
func unitsToSortedSlice(units map[string]mcov1alpha1.UnitSpec) []mcov1alpha1.UnitSpec {
	if len(units) == 0 {
		return []mcov1alpha1.UnitSpec{}
	}

	result := make([]mcov1alpha1.UnitSpec, 0, len(units))
	for _, u := range units {
		result = append(result, u)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result
}
