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
	"os"
	"path/filepath"
	"testing"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

func TestNewFileApplier(t *testing.T) {
	a := NewFileApplier("/host")
	if a == nil {
		t.Fatal("NewFileApplier returned nil")
	}
	if a.hostRoot != "/host" {
		t.Errorf("hostRoot = %s, want /host", a.hostRoot)
	}
}

func TestApply_CreateFile(t *testing.T) {
	dir := t.TempDir()
	a := NewFileApplierWithOptions(dir, true) // skip ownership for non-root tests

	f := mcov1alpha1.FileSpec{
		Path:    "/etc/test.conf",
		Content: "hello world",
		Mode:    0644,
		State:   "present",
	}

	result := a.Apply(f)
	if result.Error != nil {
		t.Fatalf("Apply() error = %v", result.Error)
	}
	if !result.Applied {
		t.Error("Apply() should report file as applied")
	}

	// Verify file exists
	path := filepath.Join(dir, f.Path)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != f.Content {
		t.Errorf("content = %q, want %q", string(content), f.Content)
	}
}

func TestApply_CreateFileCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	a := NewFileApplierWithOptions(dir, true)

	f := mcov1alpha1.FileSpec{
		Path:    "/etc/nested/deep/test.conf",
		Content: "nested content",
		State:   "present",
	}

	result := a.Apply(f)
	if result.Error != nil {
		t.Fatalf("Apply() error = %v", result.Error)
	}

	// Verify directory was created
	path := filepath.Join(dir, f.Path)
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestApply_Idempotent(t *testing.T) {
	dir := t.TempDir()
	a := NewFileApplierWithOptions(dir, true)

	f := mcov1alpha1.FileSpec{
		Path:    "/etc/test.conf",
		Content: "hello world",
		Mode:    0644,
		State:   "present",
	}

	// First apply
	result1 := a.Apply(f)
	if result1.Error != nil {
		t.Fatalf("First Apply() error = %v", result1.Error)
	}
	if !result1.Applied {
		t.Error("First Apply() should report file as applied")
	}

	// Second apply - same content
	result2 := a.Apply(f)
	if result2.Error != nil {
		t.Fatalf("Second Apply() error = %v", result2.Error)
	}
	if result2.Applied {
		t.Error("Second Apply() should not report file as applied (idempotent)")
	}
}

func TestApply_UpdateFile(t *testing.T) {
	dir := t.TempDir()
	a := NewFileApplierWithOptions(dir, true)

	// Create initial file
	f1 := mcov1alpha1.FileSpec{
		Path:    "/etc/test.conf",
		Content: "initial content",
		State:   "present",
	}
	a.Apply(f1)

	// Update file
	f2 := mcov1alpha1.FileSpec{
		Path:    "/etc/test.conf",
		Content: "updated content",
		State:   "present",
	}
	result := a.Apply(f2)
	if result.Error != nil {
		t.Fatalf("Apply() error = %v", result.Error)
	}
	if !result.Applied {
		t.Error("Apply() should report file as applied for update")
	}

	// Verify updated content
	path := filepath.Join(dir, f2.Path)
	content, _ := os.ReadFile(path)
	if string(content) != f2.Content {
		t.Errorf("content = %q, want %q", string(content), f2.Content)
	}
}

func TestApply_DeleteFile(t *testing.T) {
	dir := t.TempDir()
	a := NewFileApplier(dir)

	// Create file first
	path := filepath.Join(dir, "/etc/test.conf")
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte("content"), 0644)

	f := mcov1alpha1.FileSpec{
		Path:  "/etc/test.conf",
		State: "absent",
	}

	result := a.Apply(f)
	if result.Error != nil {
		t.Fatalf("Apply() error = %v", result.Error)
	}
	if !result.Applied {
		t.Error("Apply() should report file as applied (deleted)")
	}

	// Verify file is gone
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}
}

func TestApply_DeleteFileIdempotent(t *testing.T) {
	dir := t.TempDir()
	a := NewFileApplier(dir)

	// Delete non-existent file
	f := mcov1alpha1.FileSpec{
		Path:  "/etc/nonexistent.conf",
		State: "absent",
	}

	result := a.Apply(f)
	if result.Error != nil {
		t.Fatalf("Apply() error = %v", result.Error)
	}
	if result.Applied {
		t.Error("Apply() should not report as applied for non-existent file")
	}
}

func TestApply_DefaultState(t *testing.T) {
	dir := t.TempDir()
	a := NewFileApplierWithOptions(dir, true)

	// No state specified - should default to present
	f := mcov1alpha1.FileSpec{
		Path:    "/etc/test.conf",
		Content: "default state",
	}

	result := a.Apply(f)
	if result.Error != nil {
		t.Fatalf("Apply() error = %v", result.Error)
	}

	// Verify file was created
	path := filepath.Join(dir, f.Path)
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file should exist: %v", err)
	}
}

func TestApply_DefaultMode(t *testing.T) {
	dir := t.TempDir()
	a := NewFileApplierWithOptions(dir, true)

	// No mode specified - should default to 0644
	f := mcov1alpha1.FileSpec{
		Path:    "/etc/test.conf",
		Content: "default mode",
		State:   "present",
	}

	result := a.Apply(f)
	if result.Error != nil {
		t.Fatalf("Apply() error = %v", result.Error)
	}

	// Verify mode
	path := filepath.Join(dir, f.Path)
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0644 {
		t.Errorf("mode = %o, want 0644", info.Mode().Perm())
	}
}

func TestApply_CustomMode(t *testing.T) {
	dir := t.TempDir()
	a := NewFileApplierWithOptions(dir, true)

	f := mcov1alpha1.FileSpec{
		Path:    "/etc/test.conf",
		Content: "custom mode",
		Mode:    0600,
		State:   "present",
	}

	result := a.Apply(f)
	if result.Error != nil {
		t.Fatalf("Apply() error = %v", result.Error)
	}

	// Verify mode
	path := filepath.Join(dir, f.Path)
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Errorf("mode = %o, want 0600", info.Mode().Perm())
	}
}

func TestApply_RelativePathError(t *testing.T) {
	a := NewFileApplier("")

	f := mcov1alpha1.FileSpec{
		Path:    "relative/path.conf",
		Content: "content",
		State:   "present",
	}

	result := a.Apply(f)
	if result.Error == nil {
		t.Error("Apply() should error on relative path")
	}
}

func TestApply_InvalidState(t *testing.T) {
	dir := t.TempDir()
	a := NewFileApplier(dir)

	f := mcov1alpha1.FileSpec{
		Path:  "/etc/test.conf",
		State: "invalid",
	}

	result := a.Apply(f)
	if result.Error == nil {
		t.Error("Apply() should error on invalid state")
	}
}

func TestApplyAll_Success(t *testing.T) {
	dir := t.TempDir()
	a := NewFileApplierWithOptions(dir, true)

	files := []mcov1alpha1.FileSpec{
		{Path: "/etc/file1.conf", Content: "content1", State: "present"},
		{Path: "/etc/file2.conf", Content: "content2", State: "present"},
		{Path: "/etc/file3.conf", Content: "content3", State: "present"},
	}

	results, err := a.ApplyAll(files)
	if err != nil {
		t.Fatalf("ApplyAll() error = %v", err)
	}
	if len(results) != 3 {
		t.Errorf("results count = %d, want 3", len(results))
	}

	// Verify all files created
	for _, f := range files {
		path := filepath.Join(dir, f.Path)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("file %s not created: %v", f.Path, err)
		}
	}
}

func TestApplyAll_StopsOnError(t *testing.T) {
	dir := t.TempDir()
	a := NewFileApplierWithOptions(dir, true)

	files := []mcov1alpha1.FileSpec{
		{Path: "/etc/file1.conf", Content: "content1", State: "present"},
		{Path: "relative/path.conf", Content: "bad", State: "present"}, // will fail
		{Path: "/etc/file3.conf", Content: "content3", State: "present"},
	}

	results, err := a.ApplyAll(files)
	if err == nil {
		t.Fatal("ApplyAll() should error")
	}

	// Should have 2 results (stopped after error)
	if len(results) != 2 {
		t.Errorf("results count = %d, want 2", len(results))
	}

	// First file should be created
	path := filepath.Join(dir, files[0].Path)
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file1 should be created: %v", err)
	}

	// Third file should NOT be created
	path = filepath.Join(dir, files[2].Path)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file3 should NOT be created (stopped on error)")
	}
}

func TestNeedsUpdate_FileNotExists(t *testing.T) {
	dir := t.TempDir()
	a := NewFileApplier(dir)

	f := mcov1alpha1.FileSpec{
		Path:    "/etc/nonexistent.conf",
		Content: "content",
		State:   "present",
	}

	needs, err := a.NeedsUpdate(f)
	if err != nil {
		t.Fatalf("NeedsUpdate() error = %v", err)
	}
	if !needs {
		t.Error("NeedsUpdate() should return true for non-existent file")
	}
}

func TestNeedsUpdate_SameContent(t *testing.T) {
	dir := t.TempDir()
	a := NewFileApplier(dir)

	// Create file
	path := filepath.Join(dir, "/etc/test.conf")
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte("content"), 0644)

	f := mcov1alpha1.FileSpec{
		Path:    "/etc/test.conf",
		Content: "content",
		State:   "present",
	}

	needs, err := a.NeedsUpdate(f)
	if err != nil {
		t.Fatalf("NeedsUpdate() error = %v", err)
	}
	if needs {
		t.Error("NeedsUpdate() should return false for same content")
	}
}

func TestNeedsUpdate_DifferentContent(t *testing.T) {
	dir := t.TempDir()
	a := NewFileApplier(dir)

	// Create file
	path := filepath.Join(dir, "/etc/test.conf")
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte("old content"), 0644)

	f := mcov1alpha1.FileSpec{
		Path:    "/etc/test.conf",
		Content: "new content",
		State:   "present",
	}

	needs, err := a.NeedsUpdate(f)
	if err != nil {
		t.Fatalf("NeedsUpdate() error = %v", err)
	}
	if !needs {
		t.Error("NeedsUpdate() should return true for different content")
	}
}

func TestNeedsUpdate_AbsentExisting(t *testing.T) {
	dir := t.TempDir()
	a := NewFileApplier(dir)

	// Create file
	path := filepath.Join(dir, "/etc/test.conf")
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte("content"), 0644)

	f := mcov1alpha1.FileSpec{
		Path:  "/etc/test.conf",
		State: "absent",
	}

	needs, err := a.NeedsUpdate(f)
	if err != nil {
		t.Fatalf("NeedsUpdate() error = %v", err)
	}
	if !needs {
		t.Error("NeedsUpdate() should return true for existing file with absent state")
	}
}

func TestNeedsUpdate_AbsentNonexistent(t *testing.T) {
	dir := t.TempDir()
	a := NewFileApplier(dir)

	f := mcov1alpha1.FileSpec{
		Path:  "/etc/nonexistent.conf",
		State: "absent",
	}

	needs, err := a.NeedsUpdate(f)
	if err != nil {
		t.Fatalf("NeedsUpdate() error = %v", err)
	}
	if needs {
		t.Error("NeedsUpdate() should return false for non-existent file with absent state")
	}
}

func TestLookupUID_Numeric(t *testing.T) {
	a := NewFileApplier("")

	uid, err := a.lookupUID("1000")
	if err != nil {
		t.Fatalf("lookupUID() error = %v", err)
	}
	if uid != 1000 {
		t.Errorf("uid = %d, want 1000", uid)
	}
}

func TestLookupUID_Root(t *testing.T) {
	a := NewFileApplier("")

	uid, err := a.lookupUID("root")
	if err != nil {
		t.Fatalf("lookupUID() error = %v", err)
	}
	if uid != 0 {
		t.Errorf("uid = %d, want 0", uid)
	}
}

func TestLookupGID_Numeric(t *testing.T) {
	a := NewFileApplier("")

	gid, err := a.lookupGID("1000")
	if err != nil {
		t.Fatalf("lookupGID() error = %v", err)
	}
	if gid != 1000 {
		t.Errorf("gid = %d, want 1000", gid)
	}
}

func TestLookupGID_Root(t *testing.T) {
	a := NewFileApplier("")

	gid, err := a.lookupGID("root")
	if err != nil {
		t.Fatalf("lookupGID() error = %v", err)
	}
	if gid != 0 {
		t.Errorf("gid = %d, want 0", gid)
	}
}

func TestApply_OwnerFormat(t *testing.T) {
	dir := t.TempDir()
	a := NewFileApplier(dir)

	// Invalid owner format
	f := mcov1alpha1.FileSpec{
		Path:    "/etc/test.conf",
		Content: "content",
		Owner:   "invalid",
		State:   "present",
	}

	result := a.Apply(f)
	if result.Error == nil {
		t.Error("Apply() should error on invalid owner format")
	}
}
