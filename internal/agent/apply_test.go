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
	"os"
	"path/filepath"
	"testing"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

func TestNewApplier(t *testing.T) {
	mock := NewMockConnection()
	a := NewApplier("/host", mock)
	if a == nil {
		t.Fatal("NewApplier returned nil")
	}
	if a.files == nil {
		t.Error("files applier is nil")
	}
	if a.systemd == nil {
		t.Error("systemd applier is nil")
	}
}

func TestApplier_Close(t *testing.T) {
	mock := NewMockConnection()
	a := NewApplier("/host", mock)
	a.Close()
	if !mock.Closed {
		t.Error("Close() did not close systemd connection")
	}
}

func TestApply_FilesOnly(t *testing.T) {
	dir := t.TempDir()
	mock := NewMockConnection()
	a := NewApplierWithOptions(dir, mock, true)

	config := &mcov1alpha1.RenderedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/file1.conf", Content: "content1", State: "present"},
			{Path: "/etc/file2.conf", Content: "content2", State: "present"},
		},
	}

	result, err := a.Apply(context.Background(), config)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !result.Success {
		t.Error("Apply() should succeed")
	}
	if result.FilesApplied != 2 {
		t.Errorf("FilesApplied = %d, want 2", result.FilesApplied)
	}

	for _, f := range config.Files {
		path := filepath.Join(dir, f.Path)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("file %s not created: %v", f.Path, err)
		}
	}
}

func TestApply_UnitsOnly(t *testing.T) {
	mock := NewMockConnection()
	a := NewApplier("", mock)
	ctx := context.Background()

	config := &mcov1alpha1.RenderedConfig{
		Systemd: mcov1alpha1.SystemdSpec{
			Units: []mcov1alpha1.UnitSpec{
				{Name: "unit1.service", State: "restarted"},
				{Name: "unit2.service", State: "restarted"},
			},
		},
	}

	result, err := a.Apply(ctx, config)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !result.Success {
		t.Error("Apply() should succeed")
	}
	if result.UnitsApplied != 2 {
		t.Errorf("UnitsApplied = %d, want 2", result.UnitsApplied)
	}
	if len(mock.RestartCalls) != 2 {
		t.Errorf("RestartCalls = %d, want 2", len(mock.RestartCalls))
	}
}

func TestApply_FilesAndUnits(t *testing.T) {
	dir := t.TempDir()
	mock := NewMockConnection()
	a := NewApplierWithOptions(dir, mock, true)
	ctx := context.Background()

	config := &mcov1alpha1.RenderedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/test.conf", Content: "content", State: "present"},
		},
		Systemd: mcov1alpha1.SystemdSpec{
			Units: []mcov1alpha1.UnitSpec{
				{Name: "test.service", State: "restarted"},
			},
		},
	}

	result, err := a.Apply(ctx, config)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !result.Success {
		t.Error("Apply() should succeed")
	}
	if result.FilesApplied != 1 {
		t.Errorf("FilesApplied = %d, want 1", result.FilesApplied)
	}
	if result.UnitsApplied != 1 {
		t.Errorf("UnitsApplied = %d, want 1", result.UnitsApplied)
	}
}

func TestApply_FilesSortedByPath(t *testing.T) {
	dir := t.TempDir()
	mock := NewMockConnection()
	a := NewApplierWithOptions(dir, mock, true)
	ctx := context.Background()

	// Files in random order
	config := &mcov1alpha1.RenderedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/z.conf", Content: "z", State: "present"},
			{Path: "/etc/a.conf", Content: "a", State: "present"},
			{Path: "/etc/m.conf", Content: "m", State: "present"},
		},
	}

	result, err := a.Apply(ctx, config)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.FilesApplied != 3 {
		t.Errorf("FilesApplied = %d, want 3", result.FilesApplied)
	}

	for _, f := range config.Files {
		path := filepath.Join(dir, f.Path)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("file %s not created", f.Path)
		}
	}
}

func TestApply_UnitsSortedByName(t *testing.T) {
	mock := NewMockConnection()
	a := NewApplier("", mock)
	ctx := context.Background()

	// Units in random order
	config := &mcov1alpha1.RenderedConfig{
		Systemd: mcov1alpha1.SystemdSpec{
			Units: []mcov1alpha1.UnitSpec{
				{Name: "z.service", State: "restarted"},
				{Name: "a.service", State: "restarted"},
				{Name: "m.service", State: "restarted"},
			},
		},
	}

	result, err := a.Apply(ctx, config)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.UnitsApplied != 3 {
		t.Errorf("UnitsApplied = %d, want 3", result.UnitsApplied)
	}

	// Check order of calls
	expected := []string{"a.service", "m.service", "z.service"}
	for i, name := range expected {
		if mock.RestartCalls[i] != name {
			t.Errorf("RestartCalls[%d] = %s, want %s", i, mock.RestartCalls[i], name)
		}
	}
}

func TestApply_FileErrorStops(t *testing.T) {
	dir := t.TempDir()
	mock := NewMockConnection()
	a := NewApplierWithOptions(dir, mock, true)
	ctx := context.Background()

	// Files sorted by path: /aaa (good), /zzz (good), relative (bad - fails)
	config := &mcov1alpha1.RenderedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/zzz/good.conf", Content: "good", State: "present"},
			{Path: "relative/bad.conf", Content: "bad", State: "present"}, // will fail - relative path
			{Path: "/aaa/first.conf", Content: "first", State: "present"},
		},
		Systemd: mcov1alpha1.SystemdSpec{
			Units: []mcov1alpha1.UnitSpec{
				{Name: "test.service", State: "restarted"},
			},
		},
	}

	result, err := a.Apply(ctx, config)
	if err == nil {
		t.Fatal("Apply() should error")
	}
	if result.Success {
		t.Error("Apply() should not succeed")
	}

	// After sorting: /aaa (applied), /zzz (applied), relative (error)
	// 2 files applied before the relative path error
	if result.FilesApplied != 2 {
		t.Errorf("FilesApplied = %d, want 2", result.FilesApplied)
	}
	if result.UnitsApplied != 0 {
		t.Errorf("UnitsApplied = %d, want 0", result.UnitsApplied)
	}
}

func TestApply_UnitErrorStops(t *testing.T) {
	dir := t.TempDir()
	mock := NewMockConnection()
	a := NewApplierWithOptions(dir, mock, true)
	ctx := context.Background()

	// Units sorted by name: aaa (good), mmm (bad), zzz (never)
	config := &mcov1alpha1.RenderedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/good.conf", Content: "good", State: "present"},
		},
		Systemd: mcov1alpha1.SystemdSpec{
			Units: []mcov1alpha1.UnitSpec{
				{Name: "zzz.service", State: "restarted"},
				{Name: "mmm.service", State: "invalid"}, // will fail
				{Name: "aaa.service", State: "restarted"},
			},
		},
	}

	result, err := a.Apply(ctx, config)
	if err == nil {
		t.Fatal("Apply() should error")
	}
	if result.Success {
		t.Error("Apply() should not succeed")
	}

	// File applied, units sorted: aaa (applied), mmm (failed), zzz (never)
	if result.FilesApplied != 1 {
		t.Errorf("FilesApplied = %d, want 1", result.FilesApplied)
	}
	if result.UnitsApplied != 1 {
		t.Errorf("UnitsApplied = %d, want 1", result.UnitsApplied)
	}
}

func TestApplier_Idempotent(t *testing.T) {
	dir := t.TempDir()
	mock := NewMockConnection()
	a := NewApplierWithOptions(dir, mock, true)
	ctx := context.Background()

	// Pre-create file with same content
	path := filepath.Join(dir, "/etc/test.conf")
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte("content"), 0644)

	// Pre-set unit as active
	mock.SetProperty("test.service", "ActiveState", "active")

	config := &mcov1alpha1.RenderedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/test.conf", Content: "content", State: "present"},
		},
		Systemd: mcov1alpha1.SystemdSpec{
			Units: []mcov1alpha1.UnitSpec{
				{Name: "test.service", State: "started"},
			},
		},
	}

	result, err := a.Apply(ctx, config)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !result.Success {
		t.Error("Apply() should succeed")
	}
	if result.FilesApplied != 0 {
		t.Errorf("FilesApplied = %d, want 0 (idempotent)", result.FilesApplied)
	}
	if result.FilesSkipped != 1 {
		t.Errorf("FilesSkipped = %d, want 1", result.FilesSkipped)
	}
	if result.UnitsApplied != 0 {
		t.Errorf("UnitsApplied = %d, want 0 (idempotent)", result.UnitsApplied)
	}
	if result.UnitsSkipped != 1 {
		t.Errorf("UnitsSkipped = %d, want 1", result.UnitsSkipped)
	}
}

func TestApply_EmptyConfig(t *testing.T) {
	mock := NewMockConnection()
	a := NewApplier("", mock)
	ctx := context.Background()

	config := &mcov1alpha1.RenderedConfig{}

	result, err := a.Apply(ctx, config)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !result.Success {
		t.Error("Apply() should succeed with empty config")
	}
	if result.FilesApplied != 0 || result.UnitsApplied != 0 {
		t.Error("Empty config should have no applied items")
	}
}

func TestApply_ContextCanceled(t *testing.T) {
	dir := t.TempDir()
	mock := NewMockConnection()
	a := NewApplierWithOptions(dir, mock, true)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	config := &mcov1alpha1.RenderedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/test.conf", Content: "content", State: "present"},
		},
	}

	result, err := a.Apply(ctx, config)
	if err == nil {
		t.Fatal("Apply() should error on canceled context")
	}
	if result.Success {
		t.Error("Apply() should not succeed on canceled context")
	}
}

func TestApplySpec(t *testing.T) {
	dir := t.TempDir()
	mock := NewMockConnection()
	a := NewApplierWithOptions(dir, mock, true)
	ctx := context.Background()

	spec := &mcov1alpha1.RenderedMachineConfigSpec{
		Config: mcov1alpha1.RenderedConfig{
			Files: []mcov1alpha1.FileSpec{
				{Path: "/etc/test.conf", Content: "content", State: "present"},
			},
		},
	}

	result, err := a.ApplySpec(ctx, spec)
	if err != nil {
		t.Fatalf("ApplySpec() error = %v", err)
	}
	if !result.Success {
		t.Error("ApplySpec() should succeed")
	}
	if result.FilesApplied != 1 {
		t.Errorf("FilesApplied = %d, want 1", result.FilesApplied)
	}
}

func TestDryRun(t *testing.T) {
	dir := t.TempDir()
	mock := NewMockConnection()
	a := NewApplierWithOptions(dir, mock, true)
	ctx := context.Background()

	// Pre-create one file
	path := filepath.Join(dir, "/etc/existing.conf")
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte("existing"), 0644)

	config := &mcov1alpha1.RenderedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/existing.conf", Content: "existing", State: "present"}, // no change
			{Path: "/etc/new.conf", Content: "new", State: "present"},           // needs creation
		},
		Systemd: mcov1alpha1.SystemdSpec{
			Units: []mcov1alpha1.UnitSpec{
				{Name: "test.service", State: "restarted"},
			},
		},
	}

	result, err := a.DryRun(ctx, config)
	if err != nil {
		t.Fatalf("DryRun() error = %v", err)
	}

	// Only new.conf needs change
	if len(result.FilesToChange) != 1 {
		t.Errorf("FilesToChange = %v, want 1 item", result.FilesToChange)
	}
	if len(result.FilesToChange) > 0 && result.FilesToChange[0] != "/etc/new.conf" {
		t.Errorf("FilesToChange[0] = %s, want /etc/new.conf", result.FilesToChange[0])
	}

	if len(result.UnitsToChange) != 1 {
		t.Errorf("UnitsToChange = %v, want 1 item", result.UnitsToChange)
	}

	if !result.HasChanges() {
		t.Error("HasChanges() should return true")
	}
}

func TestDryRun_NoChanges(t *testing.T) {
	dir := t.TempDir()
	mock := NewMockConnection()
	a := NewApplierWithOptions(dir, mock, true)
	ctx := context.Background()

	// Pre-create file with same content
	path := filepath.Join(dir, "/etc/test.conf")
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte("content"), 0644)

	config := &mcov1alpha1.RenderedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/test.conf", Content: "content", State: "present"},
		},
	}

	result, err := a.DryRun(ctx, config)
	if err != nil {
		t.Fatalf("DryRun() error = %v", err)
	}

	if len(result.FilesToChange) != 0 {
		t.Errorf("FilesToChange = %v, want empty", result.FilesToChange)
	}
	if result.HasChanges() {
		t.Error("HasChanges() should return false")
	}
}

func TestSortFilesByPath(t *testing.T) {
	files := []mcov1alpha1.FileSpec{
		{Path: "/etc/z.conf"},
		{Path: "/etc/a.conf"},
		{Path: "/etc/m.conf"},
	}

	sorted := sortFilesByPath(files)

	expected := []string{"/etc/a.conf", "/etc/m.conf", "/etc/z.conf"}
	for i, e := range expected {
		if sorted[i].Path != e {
			t.Errorf("sorted[%d].Path = %s, want %s", i, sorted[i].Path, e)
		}
	}

	// Original should be unchanged
	if files[0].Path != "/etc/z.conf" {
		t.Error("Original slice was modified")
	}
}

func TestSortUnitsByName(t *testing.T) {
	units := []mcov1alpha1.UnitSpec{
		{Name: "z.service"},
		{Name: "a.service"},
		{Name: "m.service"},
	}

	sorted := sortUnitsByName(units)

	expected := []string{"a.service", "m.service", "z.service"}
	for i, e := range expected {
		if sorted[i].Name != e {
			t.Errorf("sorted[%d].Name = %s, want %s", i, sorted[i].Name, e)
		}
	}

	// Original should be unchanged
	if units[0].Name != "z.service" {
		t.Error("Original slice was modified")
	}
}
