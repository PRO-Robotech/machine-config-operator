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
	"testing"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// Helper to create bool pointer
func boolPtr(b bool) *bool {
	return &b
}

// TestDiffFiles_NoChanges verifies no changes when lists are identical.
func TestDiffFiles_NoChanges(t *testing.T) {
	currentList := []mcov1alpha1.FileSpec{
		{Path: "/etc/a.conf", Content: "a", Mode: 0644, Owner: "root:root", State: "present"},
		{Path: "/etc/b.conf", Content: "b", Mode: 0644, Owner: "root:root", State: "present"},
	}
	newList := []mcov1alpha1.FileSpec{
		{Path: "/etc/a.conf", Content: "a", Mode: 0644, Owner: "root:root", State: "present"},
		{Path: "/etc/b.conf", Content: "b", Mode: 0644, Owner: "root:root", State: "present"},
	}

	changes := DiffFiles(currentList, newList)

	if len(changes) != 0 {
		t.Errorf("Expected no changes, got %d: %+v", len(changes), changes)
	}
}

// TestDiffFiles_AddFile verifies detection of added files.
func TestDiffFiles_AddFile(t *testing.T) {
	currentList := []mcov1alpha1.FileSpec{
		{Path: "/etc/a.conf", Content: "a", Mode: 0644, Owner: "root:root", State: "present"},
	}
	newList := []mcov1alpha1.FileSpec{
		{Path: "/etc/a.conf", Content: "a", Mode: 0644, Owner: "root:root", State: "present"},
		{Path: "/etc/b.conf", Content: "b", Mode: 0644, Owner: "root:root", State: "present"},
	}

	changes := DiffFiles(currentList, newList)

	if len(changes) != 1 {
		t.Fatalf("Expected 1 change, got %d: %+v", len(changes), changes)
	}
	if changes[0].Path != "/etc/b.conf" || changes[0].ChangeType != ChangeTypeAdded {
		t.Errorf("Expected /etc/b.conf:added, got %s:%s", changes[0].Path, changes[0].ChangeType)
	}
}

// TestDiffFiles_RemoveFile verifies detection of removed files.
func TestDiffFiles_RemoveFile(t *testing.T) {
	currentList := []mcov1alpha1.FileSpec{
		{Path: "/etc/a.conf", Content: "a", Mode: 0644, Owner: "root:root", State: "present"},
		{Path: "/etc/b.conf", Content: "b", Mode: 0644, Owner: "root:root", State: "present"},
	}
	newList := []mcov1alpha1.FileSpec{
		{Path: "/etc/a.conf", Content: "a", Mode: 0644, Owner: "root:root", State: "present"},
	}

	changes := DiffFiles(currentList, newList)

	if len(changes) != 1 {
		t.Fatalf("Expected 1 change, got %d: %+v", len(changes), changes)
	}
	if changes[0].Path != "/etc/b.conf" || changes[0].ChangeType != ChangeTypeRemoved {
		t.Errorf("Expected /etc/b.conf:removed, got %s:%s", changes[0].Path, changes[0].ChangeType)
	}
}

// TestDiffFiles_ModifyContent verifies detection of content changes.
func TestDiffFiles_ModifyContent(t *testing.T) {
	currentList := []mcov1alpha1.FileSpec{
		{Path: "/etc/a.conf", Content: "v1", Mode: 0644, Owner: "root:root", State: "present"},
	}
	newList := []mcov1alpha1.FileSpec{
		{Path: "/etc/a.conf", Content: "v2", Mode: 0644, Owner: "root:root", State: "present"},
	}

	changes := DiffFiles(currentList, newList)

	if len(changes) != 1 {
		t.Fatalf("Expected 1 change, got %d: %+v", len(changes), changes)
	}
	if changes[0].Path != "/etc/a.conf" || changes[0].ChangeType != ChangeTypeModified {
		t.Errorf("Expected /etc/a.conf:modified, got %s:%s", changes[0].Path, changes[0].ChangeType)
	}
}

// TestDiffFiles_ModifyMode verifies detection of mode changes.
func TestDiffFiles_ModifyMode(t *testing.T) {
	currentList := []mcov1alpha1.FileSpec{
		{Path: "/etc/a.conf", Content: "a", Mode: 0644, Owner: "root:root", State: "present"},
	}
	newList := []mcov1alpha1.FileSpec{
		{Path: "/etc/a.conf", Content: "a", Mode: 0755, Owner: "root:root", State: "present"},
	}

	changes := DiffFiles(currentList, newList)

	if len(changes) != 1 {
		t.Fatalf("Expected 1 change, got %d: %+v", len(changes), changes)
	}
	if changes[0].ChangeType != ChangeTypeModified {
		t.Errorf("Expected modified, got %s", changes[0].ChangeType)
	}
}

// TestDiffFiles_ModifyOwner verifies detection of owner changes.
func TestDiffFiles_ModifyOwner(t *testing.T) {
	currentList := []mcov1alpha1.FileSpec{
		{Path: "/etc/a.conf", Content: "a", Mode: 0644, Owner: "root:root", State: "present"},
	}
	newList := []mcov1alpha1.FileSpec{
		{Path: "/etc/a.conf", Content: "a", Mode: 0644, Owner: "user:group", State: "present"},
	}

	changes := DiffFiles(currentList, newList)

	if len(changes) != 1 {
		t.Fatalf("Expected 1 change, got %d: %+v", len(changes), changes)
	}
	if changes[0].ChangeType != ChangeTypeModified {
		t.Errorf("Expected modified, got %s", changes[0].ChangeType)
	}
}

// TestDiffFiles_ModifyState verifies detection of state changes.
func TestDiffFiles_ModifyState(t *testing.T) {
	currentList := []mcov1alpha1.FileSpec{
		{Path: "/etc/a.conf", Content: "a", Mode: 0644, Owner: "root:root", State: "present"},
	}
	newList := []mcov1alpha1.FileSpec{
		{Path: "/etc/a.conf", Content: "a", Mode: 0644, Owner: "root:root", State: "absent"},
	}

	changes := DiffFiles(currentList, newList)

	if len(changes) != 1 {
		t.Fatalf("Expected 1 change, got %d: %+v", len(changes), changes)
	}
	if changes[0].ChangeType != ChangeTypeModified {
		t.Errorf("Expected modified, got %s", changes[0].ChangeType)
	}
}

// TestDiffFiles_MultipleChanges verifies handling of multiple changes.
func TestDiffFiles_MultipleChanges(t *testing.T) {
	currentList := []mcov1alpha1.FileSpec{
		{Path: "/etc/a.conf", Content: "a", Mode: 0644, Owner: "root:root", State: "present"},
		{Path: "/etc/b.conf", Content: "b", Mode: 0644, Owner: "root:root", State: "present"},
	}
	newList := []mcov1alpha1.FileSpec{
		{Path: "/etc/b.conf", Content: "b-modified", Mode: 0644, Owner: "root:root", State: "present"},
		{Path: "/etc/c.conf", Content: "c", Mode: 0644, Owner: "root:root", State: "present"},
	}

	changes := DiffFiles(currentList, newList)

	if len(changes) != 3 {
		t.Fatalf("Expected 3 changes, got %d: %+v", len(changes), changes)
	}

	// Should be sorted by path: a (removed), b (modified), c (added)
	expected := []struct {
		path       string
		changeType ChangeType
	}{
		{"/etc/a.conf", ChangeTypeRemoved},
		{"/etc/b.conf", ChangeTypeModified},
		{"/etc/c.conf", ChangeTypeAdded},
	}

	for i, exp := range expected {
		if changes[i].Path != exp.path || changes[i].ChangeType != exp.changeType {
			t.Errorf("Change %d: expected %s:%s, got %s:%s",
				i, exp.path, exp.changeType, changes[i].Path, changes[i].ChangeType)
		}
	}
}

// TestDiffFiles_EmptyCurrent verifies handling of empty current list.
func TestDiffFiles_EmptyCurrent(t *testing.T) {
	currentList := []mcov1alpha1.FileSpec{}
	newList := []mcov1alpha1.FileSpec{
		{Path: "/etc/a.conf", Content: "a", Mode: 0644, Owner: "root:root", State: "present"},
	}

	changes := DiffFiles(currentList, newList)

	if len(changes) != 1 {
		t.Fatalf("Expected 1 change, got %d: %+v", len(changes), changes)
	}
	if changes[0].ChangeType != ChangeTypeAdded {
		t.Errorf("Expected added, got %s", changes[0].ChangeType)
	}
}

// TestDiffFiles_EmptyNew verifies handling of empty new list.
func TestDiffFiles_EmptyNew(t *testing.T) {
	currentList := []mcov1alpha1.FileSpec{
		{Path: "/etc/a.conf", Content: "a", Mode: 0644, Owner: "root:root", State: "present"},
	}
	newList := []mcov1alpha1.FileSpec{}

	changes := DiffFiles(currentList, newList)

	if len(changes) != 1 {
		t.Fatalf("Expected 1 change, got %d: %+v", len(changes), changes)
	}
	if changes[0].ChangeType != ChangeTypeRemoved {
		t.Errorf("Expected removed, got %s", changes[0].ChangeType)
	}
}

// TestDiffFiles_BothEmpty verifies handling of two empty lists.
func TestDiffFiles_BothEmpty(t *testing.T) {
	currentList := []mcov1alpha1.FileSpec{}
	newList := []mcov1alpha1.FileSpec{}

	changes := DiffFiles(currentList, newList)

	if len(changes) != 0 {
		t.Errorf("Expected no changes, got %d: %+v", len(changes), changes)
	}
}

// TestDiffFiles_NilSlices verifies handling of nil slices.
func TestDiffFiles_NilSlices(t *testing.T) {
	changes := DiffFiles(nil, nil)

	if len(changes) != 0 {
		t.Errorf("Expected no changes, got %d: %+v", len(changes), changes)
	}
}

// TestDiffUnits_NoChanges verifies no changes when lists are identical.
func TestDiffUnits_NoChanges(t *testing.T) {
	currentList := []mcov1alpha1.UnitSpec{
		{Name: "a.service", Enabled: boolPtr(true), State: "started", Mask: false},
		{Name: "b.service", Enabled: boolPtr(false), State: "stopped", Mask: false},
	}
	newList := []mcov1alpha1.UnitSpec{
		{Name: "a.service", Enabled: boolPtr(true), State: "started", Mask: false},
		{Name: "b.service", Enabled: boolPtr(false), State: "stopped", Mask: false},
	}

	changes := DiffUnits(currentList, newList)

	if len(changes) != 0 {
		t.Errorf("Expected no changes, got %d: %+v", len(changes), changes)
	}
}

// TestDiffUnits_AddUnit verifies detection of added units.
func TestDiffUnits_AddUnit(t *testing.T) {
	currentList := []mcov1alpha1.UnitSpec{
		{Name: "a.service", Enabled: boolPtr(true), State: "started"},
	}
	newList := []mcov1alpha1.UnitSpec{
		{Name: "a.service", Enabled: boolPtr(true), State: "started"},
		{Name: "b.service", Enabled: boolPtr(true), State: "started"},
	}

	changes := DiffUnits(currentList, newList)

	if len(changes) != 1 {
		t.Fatalf("Expected 1 change, got %d: %+v", len(changes), changes)
	}
	if changes[0].Name != "b.service" || changes[0].ChangeType != ChangeTypeAdded {
		t.Errorf("Expected b.service:added, got %s:%s", changes[0].Name, changes[0].ChangeType)
	}
}

// TestDiffUnits_RemoveUnit verifies detection of removed units.
func TestDiffUnits_RemoveUnit(t *testing.T) {
	currentList := []mcov1alpha1.UnitSpec{
		{Name: "a.service", Enabled: boolPtr(true), State: "started"},
		{Name: "b.service", Enabled: boolPtr(true), State: "started"},
	}
	newList := []mcov1alpha1.UnitSpec{
		{Name: "a.service", Enabled: boolPtr(true), State: "started"},
	}

	changes := DiffUnits(currentList, newList)

	if len(changes) != 1 {
		t.Fatalf("Expected 1 change, got %d: %+v", len(changes), changes)
	}
	if changes[0].Name != "b.service" || changes[0].ChangeType != ChangeTypeRemoved {
		t.Errorf("Expected b.service:removed, got %s:%s", changes[0].Name, changes[0].ChangeType)
	}
}

// TestDiffUnits_ModifyEnabled verifies detection of enabled changes.
func TestDiffUnits_ModifyEnabled(t *testing.T) {
	currentList := []mcov1alpha1.UnitSpec{
		{Name: "a.service", Enabled: boolPtr(true), State: "started"},
	}
	newList := []mcov1alpha1.UnitSpec{
		{Name: "a.service", Enabled: boolPtr(false), State: "started"},
	}

	changes := DiffUnits(currentList, newList)

	if len(changes) != 1 {
		t.Fatalf("Expected 1 change, got %d: %+v", len(changes), changes)
	}
	if changes[0].ChangeType != ChangeTypeModified {
		t.Errorf("Expected modified, got %s", changes[0].ChangeType)
	}
}

// TestDiffUnits_ModifyState verifies detection of state changes.
func TestDiffUnits_ModifyState(t *testing.T) {
	currentList := []mcov1alpha1.UnitSpec{
		{Name: "a.service", Enabled: boolPtr(true), State: "started"},
	}
	newList := []mcov1alpha1.UnitSpec{
		{Name: "a.service", Enabled: boolPtr(true), State: "stopped"},
	}

	changes := DiffUnits(currentList, newList)

	if len(changes) != 1 {
		t.Fatalf("Expected 1 change, got %d: %+v", len(changes), changes)
	}
	if changes[0].ChangeType != ChangeTypeModified {
		t.Errorf("Expected modified, got %s", changes[0].ChangeType)
	}
}

// TestDiffUnits_ModifyMask verifies detection of mask changes.
func TestDiffUnits_ModifyMask(t *testing.T) {
	currentList := []mcov1alpha1.UnitSpec{
		{Name: "a.service", Enabled: boolPtr(true), State: "started", Mask: false},
	}
	newList := []mcov1alpha1.UnitSpec{
		{Name: "a.service", Enabled: boolPtr(true), State: "started", Mask: true},
	}

	changes := DiffUnits(currentList, newList)

	if len(changes) != 1 {
		t.Fatalf("Expected 1 change, got %d: %+v", len(changes), changes)
	}
	if changes[0].ChangeType != ChangeTypeModified {
		t.Errorf("Expected modified, got %s", changes[0].ChangeType)
	}
}

// TestDiffUnits_EnabledNilVsTrue verifies nil vs true Enabled comparison.
func TestDiffUnits_EnabledNilVsTrue(t *testing.T) {
	currentList := []mcov1alpha1.UnitSpec{
		{Name: "a.service", Enabled: nil, State: "started"},
	}
	newList := []mcov1alpha1.UnitSpec{
		{Name: "a.service", Enabled: boolPtr(true), State: "started"},
	}

	changes := DiffUnits(currentList, newList)

	if len(changes) != 1 {
		t.Fatalf("Expected 1 change, got %d: %+v", len(changes), changes)
	}
	if changes[0].ChangeType != ChangeTypeModified {
		t.Errorf("Expected modified, got %s", changes[0].ChangeType)
	}
}

// TestDiffUnits_EnabledNilVsFalse verifies nil vs false Enabled comparison.
func TestDiffUnits_EnabledNilVsFalse(t *testing.T) {
	currentList := []mcov1alpha1.UnitSpec{
		{Name: "a.service", Enabled: nil, State: "started"},
	}
	newList := []mcov1alpha1.UnitSpec{
		{Name: "a.service", Enabled: boolPtr(false), State: "started"},
	}

	changes := DiffUnits(currentList, newList)

	if len(changes) != 1 {
		t.Fatalf("Expected 1 change, got %d: %+v", len(changes), changes)
	}
	if changes[0].ChangeType != ChangeTypeModified {
		t.Errorf("Expected modified, got %s", changes[0].ChangeType)
	}
}

// TestDiffUnits_EnabledNilVsNil verifies nil vs nil Enabled is equal.
func TestDiffUnits_EnabledNilVsNil(t *testing.T) {
	currentList := []mcov1alpha1.UnitSpec{
		{Name: "a.service", Enabled: nil, State: "started"},
	}
	newList := []mcov1alpha1.UnitSpec{
		{Name: "a.service", Enabled: nil, State: "started"},
	}

	changes := DiffUnits(currentList, newList)

	if len(changes) != 0 {
		t.Errorf("Expected no changes for nil vs nil Enabled, got %d: %+v", len(changes), changes)
	}
}

// TestDiffUnits_MultipleChanges verifies handling of multiple unit changes.
func TestDiffUnits_MultipleChanges(t *testing.T) {
	currentList := []mcov1alpha1.UnitSpec{
		{Name: "a.service", Enabled: boolPtr(true), State: "started"},
		{Name: "b.service", Enabled: boolPtr(true), State: "started"},
	}
	newList := []mcov1alpha1.UnitSpec{
		{Name: "b.service", Enabled: boolPtr(false), State: "stopped"},
		{Name: "c.service", Enabled: boolPtr(true), State: "started"},
	}

	changes := DiffUnits(currentList, newList)

	if len(changes) != 3 {
		t.Fatalf("Expected 3 changes, got %d: %+v", len(changes), changes)
	}

	// Should be sorted by name: a (removed), b (modified), c (added)
	expected := []struct {
		name       string
		changeType ChangeType
	}{
		{"a.service", ChangeTypeRemoved},
		{"b.service", ChangeTypeModified},
		{"c.service", ChangeTypeAdded},
	}

	for i, exp := range expected {
		if changes[i].Name != exp.name || changes[i].ChangeType != exp.changeType {
			t.Errorf("Change %d: expected %s:%s, got %s:%s",
				i, exp.name, exp.changeType, changes[i].Name, changes[i].ChangeType)
		}
	}
}

// TestDiffUnits_EmptyCurrent verifies handling of empty current list.
func TestDiffUnits_EmptyCurrent(t *testing.T) {
	currentList := []mcov1alpha1.UnitSpec{}
	newList := []mcov1alpha1.UnitSpec{
		{Name: "a.service", Enabled: boolPtr(true), State: "started"},
	}

	changes := DiffUnits(currentList, newList)

	if len(changes) != 1 {
		t.Fatalf("Expected 1 change, got %d: %+v", len(changes), changes)
	}
	if changes[0].ChangeType != ChangeTypeAdded {
		t.Errorf("Expected added, got %s", changes[0].ChangeType)
	}
}

// TestDiffUnits_EmptyNew verifies handling of empty new list.
func TestDiffUnits_EmptyNew(t *testing.T) {
	currentList := []mcov1alpha1.UnitSpec{
		{Name: "a.service", Enabled: boolPtr(true), State: "started"},
	}
	newList := []mcov1alpha1.UnitSpec{}

	changes := DiffUnits(currentList, newList)

	if len(changes) != 1 {
		t.Fatalf("Expected 1 change, got %d: %+v", len(changes), changes)
	}
	if changes[0].ChangeType != ChangeTypeRemoved {
		t.Errorf("Expected removed, got %s", changes[0].ChangeType)
	}
}

// TestDiffUnits_BothEmpty verifies handling of two empty lists.
func TestDiffUnits_BothEmpty(t *testing.T) {
	currentList := []mcov1alpha1.UnitSpec{}
	newList := []mcov1alpha1.UnitSpec{}

	changes := DiffUnits(currentList, newList)

	if len(changes) != 0 {
		t.Errorf("Expected no changes, got %d: %+v", len(changes), changes)
	}
}

// TestDiffUnits_NilSlices verifies handling of nil slices.
func TestDiffUnits_NilSlices(t *testing.T) {
	changes := DiffUnits(nil, nil)

	if len(changes) != 0 {
		t.Errorf("Expected no changes, got %d: %+v", len(changes), changes)
	}
}

// TestFilesEqual verifies the filesEqual helper function.
func TestFilesEqual(t *testing.T) {
	tests := []struct {
		name     string
		a, b     mcov1alpha1.FileSpec
		expected bool
	}{
		{
			name:     "identical files",
			a:        mcov1alpha1.FileSpec{Path: "/a", Content: "x", Mode: 0644, Owner: "root:root", State: "present"},
			b:        mcov1alpha1.FileSpec{Path: "/a", Content: "x", Mode: 0644, Owner: "root:root", State: "present"},
			expected: true,
		},
		{
			name:     "different content",
			a:        mcov1alpha1.FileSpec{Path: "/a", Content: "x", Mode: 0644, Owner: "root:root", State: "present"},
			b:        mcov1alpha1.FileSpec{Path: "/a", Content: "y", Mode: 0644, Owner: "root:root", State: "present"},
			expected: false,
		},
		{
			name:     "different mode",
			a:        mcov1alpha1.FileSpec{Path: "/a", Content: "x", Mode: 0644, Owner: "root:root", State: "present"},
			b:        mcov1alpha1.FileSpec{Path: "/a", Content: "x", Mode: 0755, Owner: "root:root", State: "present"},
			expected: false,
		},
		{
			name:     "different owner",
			a:        mcov1alpha1.FileSpec{Path: "/a", Content: "x", Mode: 0644, Owner: "root:root", State: "present"},
			b:        mcov1alpha1.FileSpec{Path: "/a", Content: "x", Mode: 0644, Owner: "user:user", State: "present"},
			expected: false,
		},
		{
			name:     "different state",
			a:        mcov1alpha1.FileSpec{Path: "/a", Content: "x", Mode: 0644, Owner: "root:root", State: "present"},
			b:        mcov1alpha1.FileSpec{Path: "/a", Content: "x", Mode: 0644, Owner: "root:root", State: "absent"},
			expected: false,
		},
		{
			name:     "path difference ignored",
			a:        mcov1alpha1.FileSpec{Path: "/a", Content: "x", Mode: 0644, Owner: "root:root", State: "present"},
			b:        mcov1alpha1.FileSpec{Path: "/b", Content: "x", Mode: 0644, Owner: "root:root", State: "present"},
			expected: true, // Path is the key, not compared in filesEqual
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filesEqual(tt.a, tt.b)
			if got != tt.expected {
				t.Errorf("filesEqual() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestUnitsEqual verifies the unitsEqual helper function.
func TestUnitsEqual(t *testing.T) {
	tests := []struct {
		name     string
		a, b     mcov1alpha1.UnitSpec
		expected bool
	}{
		{
			name:     "identical units",
			a:        mcov1alpha1.UnitSpec{Name: "a.service", Enabled: boolPtr(true), State: "started", Mask: false},
			b:        mcov1alpha1.UnitSpec{Name: "a.service", Enabled: boolPtr(true), State: "started", Mask: false},
			expected: true,
		},
		{
			name:     "different enabled",
			a:        mcov1alpha1.UnitSpec{Name: "a.service", Enabled: boolPtr(true), State: "started", Mask: false},
			b:        mcov1alpha1.UnitSpec{Name: "a.service", Enabled: boolPtr(false), State: "started", Mask: false},
			expected: false,
		},
		{
			name:     "different state",
			a:        mcov1alpha1.UnitSpec{Name: "a.service", Enabled: boolPtr(true), State: "started", Mask: false},
			b:        mcov1alpha1.UnitSpec{Name: "a.service", Enabled: boolPtr(true), State: "stopped", Mask: false},
			expected: false,
		},
		{
			name:     "different mask",
			a:        mcov1alpha1.UnitSpec{Name: "a.service", Enabled: boolPtr(true), State: "started", Mask: false},
			b:        mcov1alpha1.UnitSpec{Name: "a.service", Enabled: boolPtr(true), State: "started", Mask: true},
			expected: false,
		},
		{
			name:     "name difference ignored",
			a:        mcov1alpha1.UnitSpec{Name: "a.service", Enabled: boolPtr(true), State: "started", Mask: false},
			b:        mcov1alpha1.UnitSpec{Name: "b.service", Enabled: boolPtr(true), State: "started", Mask: false},
			expected: true, // Name is the key, not compared in unitsEqual
		},
		{
			name:     "nil enabled both",
			a:        mcov1alpha1.UnitSpec{Name: "a.service", Enabled: nil, State: "started", Mask: false},
			b:        mcov1alpha1.UnitSpec{Name: "a.service", Enabled: nil, State: "started", Mask: false},
			expected: true,
		},
		{
			name:     "nil vs true enabled",
			a:        mcov1alpha1.UnitSpec{Name: "a.service", Enabled: nil, State: "started", Mask: false},
			b:        mcov1alpha1.UnitSpec{Name: "a.service", Enabled: boolPtr(true), State: "started", Mask: false},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unitsEqual(tt.a, tt.b)
			if got != tt.expected {
				t.Errorf("unitsEqual() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestBoolPtrEqual verifies the boolPtrEqual helper function.
func TestBoolPtrEqual(t *testing.T) {
	tests := []struct {
		name     string
		a, b     *bool
		expected bool
	}{
		{"nil vs nil", nil, nil, true},
		{"nil vs true", nil, boolPtr(true), false},
		{"nil vs false", nil, boolPtr(false), false},
		{"true vs nil", boolPtr(true), nil, false},
		{"false vs nil", boolPtr(false), nil, false},
		{"true vs true", boolPtr(true), boolPtr(true), true},
		{"false vs false", boolPtr(false), boolPtr(false), true},
		{"true vs false", boolPtr(true), boolPtr(false), false},
		{"false vs true", boolPtr(false), boolPtr(true), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := boolPtrEqual(tt.a, tt.b)
			if got != tt.expected {
				t.Errorf("boolPtrEqual() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestDiffFiles_SortOrder verifies that results are sorted by path.
func TestDiffFiles_SortOrder(t *testing.T) {
	currentList := []mcov1alpha1.FileSpec{}
	newList := []mcov1alpha1.FileSpec{
		{Path: "/etc/z.conf", Content: "z", Mode: 0644, Owner: "root:root", State: "present"},
		{Path: "/etc/a.conf", Content: "a", Mode: 0644, Owner: "root:root", State: "present"},
		{Path: "/etc/m.conf", Content: "m", Mode: 0644, Owner: "root:root", State: "present"},
	}

	changes := DiffFiles(currentList, newList)

	if len(changes) != 3 {
		t.Fatalf("Expected 3 changes, got %d", len(changes))
	}

	// Should be sorted alphabetically
	expectedOrder := []string{"/etc/a.conf", "/etc/m.conf", "/etc/z.conf"}
	for i, exp := range expectedOrder {
		if changes[i].Path != exp {
			t.Errorf("Position %d: expected %s, got %s", i, exp, changes[i].Path)
		}
	}
}

// TestDiffUnits_SortOrder verifies that results are sorted by name.
func TestDiffUnits_SortOrder(t *testing.T) {
	currentList := []mcov1alpha1.UnitSpec{}
	newList := []mcov1alpha1.UnitSpec{
		{Name: "z.service", Enabled: boolPtr(true), State: "started"},
		{Name: "a.service", Enabled: boolPtr(true), State: "started"},
		{Name: "m.service", Enabled: boolPtr(true), State: "started"},
	}

	changes := DiffUnits(currentList, newList)

	if len(changes) != 3 {
		t.Fatalf("Expected 3 changes, got %d", len(changes))
	}

	// Should be sorted alphabetically
	expectedOrder := []string{"a.service", "m.service", "z.service"}
	for i, exp := range expectedOrder {
		if changes[i].Name != exp {
			t.Errorf("Position %d: expected %s, got %s", i, exp, changes[i].Name)
		}
	}
}
