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
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/renameio/v2"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// File state constants.
const (
	FileStatePresent = "present"
	FileStateAbsent  = "absent"
)

// FileApplyResult contains the result of a file apply operation.
type FileApplyResult struct {
	Path    string
	Applied bool // true if file was modified
	Error   error
}

// FileOperations defines the interface for file operations.
type FileOperations interface {
	// Apply applies a single file spec.
	Apply(f mcov1alpha1.FileSpec) FileApplyResult

	// ApplyAll applies multiple file specs in order.
	ApplyAll(files []mcov1alpha1.FileSpec) ([]FileApplyResult, error)

	// NeedsUpdate checks if file needs update without applying.
	NeedsUpdate(f mcov1alpha1.FileSpec) (bool, error)
}

var _ FileOperations = (*FileApplier)(nil)

// FileApplier applies file configurations to the host filesystem.
// It supports atomic writes, idempotent deletes, and content comparison.
type FileApplier struct {
	hostRoot      string // e.g., "/host" for container, "" for direct
	skipOwnership bool   // skip chown (for testing as non-root)
}

// NewFileApplier creates a new file applier.
// hostRoot is the path prefix for all file operations (e.g., "/host" when
// running as a container with the host filesystem mounted).
func NewFileApplier(hostRoot string) *FileApplier {
	return &FileApplier{hostRoot: hostRoot}
}

// NewFileApplierWithOptions creates a file applier with options.
func NewFileApplierWithOptions(hostRoot string, skipOwnership bool) *FileApplier {
	return &FileApplier{
		hostRoot:      hostRoot,
		skipOwnership: skipOwnership,
	}
}

// Apply applies a single file spec.
// For state=absent, the file is deleted if it exists.
// For state=present (default), the file is written atomically.
// Returns a result indicating whether the file was modified.
func (a *FileApplier) Apply(f mcov1alpha1.FileSpec) FileApplyResult {
	result := FileApplyResult{Path: f.Path}

	if !filepath.IsAbs(f.Path) {
		result.Error = fmt.Errorf("path must be absolute: %s", f.Path)
		return result
	}

	path := filepath.Join(a.hostRoot, f.Path)

	state := f.State
	if state == "" {
		state = FileStatePresent
	}

	switch state {
	case FileStateAbsent:
		applied, err := a.deleteFile(path)
		result.Applied = applied
		result.Error = err
	case FileStatePresent:
		applied, err := a.writeFile(path, f)
		result.Applied = applied
		result.Error = err
	default:
		result.Error = fmt.Errorf("unknown state: %s", state)
	}

	return result
}

// ApplyAll applies multiple file specs in order.
func (a *FileApplier) ApplyAll(files []mcov1alpha1.FileSpec) ([]FileApplyResult, error) {
	results := make([]FileApplyResult, 0, len(files))

	for _, f := range files {
		result := a.Apply(f)
		results = append(results, result)
		if result.Error != nil {
			return results, fmt.Errorf("file %s: %w", f.Path, result.Error)
		}
	}

	return results, nil
}

func (a *FileApplier) deleteFile(path string) (bool, error) {
	err := os.Remove(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("delete file: %w", err)
}

func (a *FileApplier) writeFile(path string, f mcov1alpha1.FileSpec) (bool, error) {
	content := []byte(f.Content)

	if !a.needsUpdate(path, content) {
		return false, nil
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return false, fmt.Errorf("create directory %s: %w", dir, err)
	}

	mode := os.FileMode(f.Mode)
	if mode == 0 {
		mode = 0644
	}

	t, err := renameio.TempFile(dir, path)
	if err != nil {
		return false, fmt.Errorf("create temp file: %w", err)
	}
	defer func() { _ = t.Cleanup() }()

	if _, err := t.Write(content); err != nil {
		return false, fmt.Errorf("write content: %w", err)
	}

	if err := t.Chmod(mode); err != nil {
		return false, fmt.Errorf("set mode: %w", err)
	}

	if err := t.CloseAtomicallyReplace(); err != nil {
		return false, fmt.Errorf("atomic replace: %w", err)
	}

	if !a.skipOwnership {
		if err := a.setOwnership(path, f.Owner); err != nil {
			return true, fmt.Errorf("set ownership: %w", err)
		}
	}

	return true, nil
}

func (a *FileApplier) needsUpdate(path string, content []byte) bool {
	existing, err := os.ReadFile(path)
	if err != nil {
		return true // file doesn't exist or error reading
	}
	return !bytes.Equal(existing, content)
}

func (a *FileApplier) setOwnership(path, owner string) error {
	if owner == "" {
		owner = "root:root"
	}

	parts := strings.Split(owner, ":")
	if len(parts) != 2 {
		return fmt.Errorf("invalid owner format (expected user:group): %s", owner)
	}

	uid, err := a.lookupUID(parts[0])
	if err != nil {
		return fmt.Errorf("lookup user %s: %w", parts[0], err)
	}
	gid, err := a.lookupGID(parts[1])
	if err != nil {
		return fmt.Errorf("lookup group %s: %w", parts[1], err)
	}

	return os.Chown(path, uid, gid)
}

func (a *FileApplier) lookupUID(s string) (int, error) {
	if uid, err := strconv.Atoi(s); err == nil {
		return uid, nil
	}

	u, err := user.Lookup(s)
	if err != nil {
		if s == "root" {
			return 0, nil
		}
		return 0, err
	}

	return strconv.Atoi(u.Uid)
}

func (a *FileApplier) lookupGID(s string) (int, error) {
	if gid, err := strconv.Atoi(s); err == nil {
		return gid, nil
	}

	g, err := user.LookupGroup(s)
	if err != nil {
		if s == "root" {
			return 0, nil
		}
		return 0, err
	}

	return strconv.Atoi(g.Gid)
}

func (a *FileApplier) NeedsUpdate(f mcov1alpha1.FileSpec) (bool, error) {
	if !filepath.IsAbs(f.Path) {
		return false, fmt.Errorf("path must be absolute: %s", f.Path)
	}

	path := filepath.Join(a.hostRoot, f.Path)
	state := f.State
	if state == "" {
		state = "present"
	}

	switch state {
	case "absent":
		_, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return true, nil
	case "present":
		return a.needsUpdate(path, []byte(f.Content)), nil
	default:
		return false, fmt.Errorf("unknown state: %s", state)
	}
}
