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
	"sort"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// ChangeType describes the type of change between two configs.
type ChangeType string

const (
	// ChangeTypeAdded indicates a new file or unit was added.
	ChangeTypeAdded ChangeType = "added"
	// ChangeTypeModified indicates an existing file or unit was modified.
	ChangeTypeModified ChangeType = "modified"
	// ChangeTypeRemoved indicates a file or unit was removed.
	ChangeTypeRemoved ChangeType = "removed"
)

// FileChange describes a change to a file.
type FileChange struct {
	// Path is the file path that changed.
	Path string
	// ChangeType is the type of change (added, modified, removed).
	ChangeType ChangeType
}

// UnitChange describes a change to a systemd unit.
type UnitChange struct {
	// Name is the unit name that changed.
	Name string
	// ChangeType is the type of change (added, modified, removed).
	ChangeType ChangeType
}

// DiffFiles computes the difference between two file lists.
// It returns all changes: added, modified, and removed files.
// The result is sorted by path for deterministic output.
//
// Parameters:
//   - current: the current file list (from old RMC or empty)
//   - new: the new file list (from new RMC)
//
// Returns a slice of FileChange describing all differences.
func DiffFiles(current, new []mcov1alpha1.FileSpec) []FileChange {
	// Build maps for O(1) lookup
	currentMap := make(map[string]mcov1alpha1.FileSpec, len(current))
	for _, f := range current {
		currentMap[f.Path] = f
	}

	newMap := make(map[string]mcov1alpha1.FileSpec, len(new))
	for _, f := range new {
		newMap[f.Path] = f
	}

	var changes []FileChange

	for _, f := range new {
		if curr, exists := currentMap[f.Path]; !exists {
			changes = append(changes, FileChange{
				Path:       f.Path,
				ChangeType: ChangeTypeAdded,
			})
		} else if !filesEqual(curr, f) {
			changes = append(changes, FileChange{
				Path:       f.Path,
				ChangeType: ChangeTypeModified,
			})
		}
	}

	for _, f := range current {
		if _, exists := newMap[f.Path]; !exists {
			changes = append(changes, FileChange{
				Path:       f.Path,
				ChangeType: ChangeTypeRemoved,
			})
		}
	}

	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Path < changes[j].Path
	})

	return changes
}

// DiffUnits computes the difference between two unit lists.
// It returns all changes: added, modified, and removed units.
// The result is sorted by name for deterministic output.
//
// Parameters:
//   - current: the current unit list (from old RMC or empty)
//   - new: the new unit list (from new RMC)
//
// Returns a slice of UnitChange describing all differences.
func DiffUnits(current, new []mcov1alpha1.UnitSpec) []UnitChange {
	// Build maps for O(1) lookup
	currentMap := make(map[string]mcov1alpha1.UnitSpec, len(current))
	for _, u := range current {
		currentMap[u.Name] = u
	}

	newMap := make(map[string]mcov1alpha1.UnitSpec, len(new))
	for _, u := range new {
		newMap[u.Name] = u
	}

	var changes []UnitChange

	for _, u := range new {
		if curr, exists := currentMap[u.Name]; !exists {
			changes = append(changes, UnitChange{
				Name:       u.Name,
				ChangeType: ChangeTypeAdded,
			})
		} else if !unitsEqual(curr, u) {
			changes = append(changes, UnitChange{
				Name:       u.Name,
				ChangeType: ChangeTypeModified,
			})
		}
	}

	for _, u := range current {
		if _, exists := newMap[u.Name]; !exists {
			changes = append(changes, UnitChange{
				Name:       u.Name,
				ChangeType: ChangeTypeRemoved,
			})
		}
	}

	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Name < changes[j].Name
	})

	return changes
}

func filesEqual(a, b mcov1alpha1.FileSpec) bool {
	return a.Content == b.Content &&
		a.Mode == b.Mode &&
		a.Owner == b.Owner &&
		a.State == b.State
}

func unitsEqual(a, b mcov1alpha1.UnitSpec) bool {
	if !boolPtrEqual(a.Enabled, b.Enabled) {
		return false
	}
	return a.State == b.State && a.Mask == b.Mask
}

func boolPtrEqual(a, b *bool) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
