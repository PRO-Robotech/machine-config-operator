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

package reboot

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// stateDir is the directory for MCO state files.
	stateDir = "/var/lib/mco"
	// lastRebootFile is the file containing the last reboot timestamp.
	lastRebootFile = "last-reboot"
	// bootMarkerDir is where we write a boot marker (tmpfs, cleared on reboot).
	bootMarkerDir = "/run/mco"
	// bootMarkerFile indicates the agent has started since last reboot.
	bootMarkerFile = "boot-marker"
)

// StateManager manages local state persistence for reboot handling.
type StateManager struct {
	hostRoot string
}

// NewStateManager creates a new state manager.
func NewStateManager(hostRoot string) *StateManager {
	return &StateManager{hostRoot: hostRoot}
}

// ReadLastRebootTime reads the last reboot timestamp from disk.
// Returns the zero time and an error if the file doesn't exist or is corrupted.
func (s *StateManager) ReadLastRebootTime() (time.Time, error) {
	path := s.statePath(lastRebootFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, err
	}

	// Parse RFC3339 format
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse last reboot time: %w", err)
	}

	return t, nil
}

// WriteLastRebootTime writes the reboot timestamp to disk.
// Creates the state directory if it doesn't exist.
func (s *StateManager) WriteLastRebootTime(t time.Time) error {
	dir := s.statePath("")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	path := s.statePath(lastRebootFile)
	data := t.Format(time.RFC3339)

	// Write atomically using temp file and rename
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(data), 0644); err != nil {
		return fmt.Errorf("failed to write last reboot time: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath) // Clean up on error
		return fmt.Errorf("failed to rename last reboot file: %w", err)
	}

	return nil
}

// GetSystemBootTime calculates the system boot time using /proc/uptime.
// This method works correctly in containers and VMs where /proc/stat btime
// may reflect the host's boot time rather than the container/VM boot time.
// Boot time is calculated as: now - uptime.
func (s *StateManager) GetSystemBootTime() (time.Time, error) {
	path := filepath.Join(s.hostRoot, "proc", "uptime")
	data, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to read /proc/uptime: %w", err)
	}

	// /proc/uptime format: "uptime_seconds idle_seconds"
	// e.g., "12345.67 98765.43"
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return time.Time{}, fmt.Errorf("invalid /proc/uptime format")
	}

	uptimeSeconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse uptime: %w", err)
	}

	// Calculate boot time: now - uptime
	now := time.Now()
	bootTime := now.Add(-time.Duration(uptimeSeconds * float64(time.Second)))

	return bootTime, nil
}

// statePath returns the full path for a state file.
func (s *StateManager) statePath(filename string) string {
	if filename == "" {
		return filepath.Join(s.hostRoot, stateDir)
	}
	return filepath.Join(s.hostRoot, stateDir, filename)
}

// BootMarkerExists checks if the boot marker file exists.
// The boot marker is stored in /run (tmpfs), which is cleared on system reboot.
// If the marker doesn't exist, it indicates the system has rebooted since
// the agent last ran.
func (s *StateManager) BootMarkerExists() bool {
	path := filepath.Join(s.hostRoot, bootMarkerDir, bootMarkerFile)
	_, err := os.Stat(path)
	return err == nil
}

// WriteBootMarker creates the boot marker file.
// This should be called after successful startup to indicate the agent
// has started since the last reboot.
func (s *StateManager) WriteBootMarker() error {
	dir := filepath.Join(s.hostRoot, bootMarkerDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create boot marker directory: %w", err)
	}

	path := filepath.Join(dir, bootMarkerFile)
	content := time.Now().Format(time.RFC3339)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write boot marker: %w", err)
	}

	return nil
}
