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
	"fmt"
	"sort"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// ApplyResult contains the result of a full config apply operation.
type ApplyResult struct {
	Success      bool
	Error        error
	FilesApplied int
	FilesSkipped int
	UnitsApplied int
	UnitsSkipped int
}

// Applier orchestrates the application of rendered configurations.
// It applies files first (sorted by path), then systemd units (sorted by name).
// On any error, it stops immediately and returns the partial result.
type Applier struct {
	files   *FileApplier
	systemd *SystemdApplier
}

// NewApplier creates a new configuration applier.
// hostRoot is the path prefix for file operations (e.g., "/host").
// conn is the systemd connection to use.
func NewApplier(hostRoot string, conn SystemdConnection) *Applier {
	return &Applier{
		files:   NewFileApplier(hostRoot),
		systemd: NewSystemdApplier(conn),
	}
}

// NewApplierWithOptions creates an applier with configurable options.
func NewApplierWithOptions(hostRoot string, conn SystemdConnection, skipOwnership bool) *Applier {
	return &Applier{
		files:   NewFileApplierWithOptions(hostRoot, skipOwnership),
		systemd: NewSystemdApplier(conn),
	}
}

// Close closes any resources held by the applier.
func (a *Applier) Close() {
	if a.systemd != nil {
		a.systemd.Close()
	}
}

// Apply applies the rendered configuration to the host.
// Files are applied first (sorted by path), then systemd units (sorted by name).
// Stops on first error.
func (a *Applier) Apply(ctx context.Context, config *mcov1alpha1.RenderedConfig) (*ApplyResult, error) {
	result := &ApplyResult{}

	files := sortFilesByPath(config.Files)
	for _, f := range files {
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			return result, result.Error
		default:
		}

		fileResult := a.files.Apply(f)
		if fileResult.Error != nil {
			result.Error = fmt.Errorf("file %s: %w", f.Path, fileResult.Error)
			return result, result.Error
		}
		if fileResult.Applied {
			result.FilesApplied++
		} else {
			result.FilesSkipped++
		}
	}

	units := sortUnitsByName(config.Systemd.Units)
	for _, u := range units {
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			return result, result.Error
		default:
		}

		unitResult := a.systemd.Apply(ctx, u)
		if unitResult.Error != nil {
			result.Error = fmt.Errorf("unit %s: %w", u.Name, unitResult.Error)
			return result, result.Error
		}
		if unitResult.Applied {
			result.UnitsApplied++
		} else {
			result.UnitsSkipped++
		}
	}

	result.Success = true
	return result, nil
}

// ApplySpec applies a RenderedMachineConfig spec.
func (a *Applier) ApplySpec(ctx context.Context, spec *mcov1alpha1.RenderedMachineConfigSpec) (*ApplyResult, error) {
	return a.Apply(ctx, &spec.Config)
}

func sortFilesByPath(files []mcov1alpha1.FileSpec) []mcov1alpha1.FileSpec {
	sorted := make([]mcov1alpha1.FileSpec, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Path < sorted[j].Path
	})
	return sorted
}

func sortUnitsByName(units []mcov1alpha1.UnitSpec) []mcov1alpha1.UnitSpec {
	sorted := make([]mcov1alpha1.UnitSpec, len(units))
	copy(sorted, units)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})
	return sorted
}

// DryRun checks what changes would be made without applying them.
// Returns the files and units that need changes.
func (a *Applier) DryRun(ctx context.Context, config *mcov1alpha1.RenderedConfig) (*DryRunResult, error) {
	result := &DryRunResult{
		FilesToChange: make([]string, 0),
		UnitsToChange: make([]string, 0),
	}

	for _, f := range config.Files {
		needs, err := a.files.NeedsUpdate(f)
		if err != nil {
			return nil, fmt.Errorf("check file %s: %w", f.Path, err)
		}
		if needs {
			result.FilesToChange = append(result.FilesToChange, f.Path)
		}
	}

	for _, u := range config.Systemd.Units {
		result.UnitsToChange = append(result.UnitsToChange, u.Name)
	}

	return result, nil
}

// DryRunResult contains the result of a dry run.
type DryRunResult struct {
	FilesToChange []string
	UnitsToChange []string
}

// HasChanges returns true if any changes would be made.
func (r *DryRunResult) HasChanges() bool {
	return len(r.FilesToChange) > 0 || len(r.UnitsToChange) > 0
}
