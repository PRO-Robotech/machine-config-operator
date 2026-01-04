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

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// SystemdConnection abstracts the systemd D-Bus connection for testing.
type SystemdConnection interface {
	// Close closes the connection.
	Close()

	// GetUnitProperty gets a property of a unit.
	GetUnitProperty(ctx context.Context, unit, property string) (interface{}, error)

	// MaskUnit masks a unit (prevents it from starting).
	MaskUnit(ctx context.Context, name string) error

	// UnmaskUnit unmasks a unit.
	UnmaskUnit(ctx context.Context, name string) error

	// EnableUnit enables a unit to start at boot.
	EnableUnit(ctx context.Context, name string) error

	// DisableUnit disables a unit from starting at boot.
	DisableUnit(ctx context.Context, name string) error

	// StartUnit starts a unit.
	StartUnit(ctx context.Context, name string) error

	// StopUnit stops a unit.
	StopUnit(ctx context.Context, name string) error

	// RestartUnit restarts a unit.
	RestartUnit(ctx context.Context, name string) error

	// ReloadUnit reloads a unit's configuration.
	ReloadUnit(ctx context.Context, name string) error
}

// UnitApplyResult contains the result of a unit apply operation.
type UnitApplyResult struct {
	Name    string
	Applied bool // true if any change was made
	Error   error
}

// SystemdApplier manages systemd units.
type SystemdApplier struct {
	conn SystemdConnection
}

// NewSystemdApplier creates a new systemd applier with the given connection.
func NewSystemdApplier(conn SystemdConnection) *SystemdApplier {
	return &SystemdApplier{conn: conn}
}

// Close closes the underlying connection.
func (a *SystemdApplier) Close() {
	if a.conn != nil {
		a.conn.Close()
	}
}

// Apply applies a single unit spec.
// Operations are applied in order: mask/unmask, enable/disable, state change.
func (a *SystemdApplier) Apply(ctx context.Context, u mcov1alpha1.UnitSpec) UnitApplyResult {
	result := UnitApplyResult{Name: u.Name}
	var anyApplied bool

	applied, err := a.applyMask(ctx, u.Name, u.Mask)
	if err != nil {
		result.Error = fmt.Errorf("mask: %w", err)
		return result
	}
	anyApplied = anyApplied || applied

	if u.Enabled != nil {
		applied, err = a.applyEnabled(ctx, u.Name, *u.Enabled)
		if err != nil {
			result.Error = fmt.Errorf("enabled: %w", err)
			return result
		}
		anyApplied = anyApplied || applied
	}

	if u.State != "" {
		applied, err = a.applyState(ctx, u.Name, u.State)
		if err != nil {
			result.Error = fmt.Errorf("state: %w", err)
			return result
		}
		anyApplied = anyApplied || applied
	}

	result.Applied = anyApplied
	return result
}

// ApplyAll applies multiple unit specs in order.
// Stops on first error and returns all results so far.
func (a *SystemdApplier) ApplyAll(ctx context.Context, units []mcov1alpha1.UnitSpec) ([]UnitApplyResult, error) {
	results := make([]UnitApplyResult, 0, len(units))

	for _, u := range units {
		result := a.Apply(ctx, u)
		results = append(results, result)
		if result.Error != nil {
			return results, fmt.Errorf("unit %s: %w", u.Name, result.Error)
		}
	}

	return results, nil
}

func (a *SystemdApplier) applyMask(ctx context.Context, name string, mask bool) (bool, error) {
	if mask {
		state, _ := a.getUnitFileState(ctx, name)
		if state == "masked" {
			return false, nil
		}
		return true, a.conn.MaskUnit(ctx, name)
	}

	state, _ := a.getUnitFileState(ctx, name)
	if state != "masked" {
		return false, nil
	}
	return true, a.conn.UnmaskUnit(ctx, name)
}

func (a *SystemdApplier) applyEnabled(ctx context.Context, name string, enabled bool) (bool, error) {
	state, err := a.getUnitFileState(ctx, name)
	if err != nil {
		// Unit might not exist yet, proceed anyway
		state = ""
	}

	if enabled {
		if state == "enabled" || state == "enabled-runtime" {
			return false, nil
		}
		return true, a.conn.EnableUnit(ctx, name)
	}

	// Disable
	if state == "disabled" || state == "" {
		return false, nil // already disabled or doesn't exist
	}
	return true, a.conn.DisableUnit(ctx, name)
}

func (a *SystemdApplier) applyState(ctx context.Context, name, state string) (bool, error) {
	switch state {
	case "started":
		return a.start(ctx, name)
	case "stopped":
		return a.stop(ctx, name)
	case "restarted":
		// restart is not idempotent - always executes
		return true, a.conn.RestartUnit(ctx, name)
	case "reloaded":
		// reload is not idempotent - always executes
		return true, a.conn.ReloadUnit(ctx, name)
	default:
		return false, fmt.Errorf("unknown state: %s", state)
	}
}

func (a *SystemdApplier) start(ctx context.Context, name string) (bool, error) {
	active, _ := a.getActiveState(ctx, name)
	if active == "active" {
		return false, nil // already running
	}
	return true, a.conn.StartUnit(ctx, name)
}

func (a *SystemdApplier) stop(ctx context.Context, name string) (bool, error) {
	active, _ := a.getActiveState(ctx, name)
	if active == "inactive" || active == "failed" || active == "" {
		return false, nil // already stopped
	}
	return true, a.conn.StopUnit(ctx, name)
}

func (a *SystemdApplier) getUnitFileState(ctx context.Context, name string) (string, error) {
	val, err := a.conn.GetUnitProperty(ctx, name, "UnitFileState")
	if err != nil {
		return "", err
	}
	if s, ok := val.(string); ok {
		return s, nil
	}
	return "", nil
}

func (a *SystemdApplier) getActiveState(ctx context.Context, name string) (string, error) {
	val, err := a.conn.GetUnitProperty(ctx, name, "ActiveState")
	if err != nil {
		return "", err
	}
	if s, ok := val.(string); ok {
		return s, nil
	}
	return "", nil
}
