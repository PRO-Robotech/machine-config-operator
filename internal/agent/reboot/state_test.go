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
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewStateManager(t *testing.T) {
	sm := NewStateManager("/host")

	if sm == nil {
		t.Fatal("NewStateManager() returned nil")
	}
	if sm.hostRoot != "/host" {
		t.Errorf("hostRoot = %q, want %q", sm.hostRoot, "/host")
	}
}

func TestReadLastRebootTime_Exists(t *testing.T) {
	hostRoot := t.TempDir()
	sm := NewStateManager(hostRoot)

	// Create state directory and file
	stateDir := filepath.Join(hostRoot, "var", "lib", "mco")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatalf("failed to create state dir: %v", err)
	}

	expectedTime := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	if err := os.WriteFile(
		filepath.Join(stateDir, "last-reboot"),
		[]byte(expectedTime.Format(time.RFC3339)),
		0644,
	); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Read it back
	result, err := sm.ReadLastRebootTime()

	if err != nil {
		t.Fatalf("ReadLastRebootTime() error = %v", err)
	}
	if !result.Equal(expectedTime) {
		t.Errorf("ReadLastRebootTime() = %v, want %v", result, expectedTime)
	}
}

func TestReadLastRebootTime_NotExists(t *testing.T) {
	sm := NewStateManager(t.TempDir())

	result, err := sm.ReadLastRebootTime()

	if err == nil {
		t.Fatal("ReadLastRebootTime() expected error for missing file")
	}
	if !result.IsZero() {
		t.Errorf("ReadLastRebootTime() = %v, want zero time", result)
	}
}

func TestReadLastRebootTime_Corrupted(t *testing.T) {
	hostRoot := t.TempDir()
	sm := NewStateManager(hostRoot)

	// Create state directory and file with invalid content
	stateDir := filepath.Join(hostRoot, "var", "lib", "mco")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatalf("failed to create state dir: %v", err)
	}

	if err := os.WriteFile(
		filepath.Join(stateDir, "last-reboot"),
		[]byte("not-a-timestamp"),
		0644,
	); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Read it back
	result, err := sm.ReadLastRebootTime()

	if err == nil {
		t.Fatal("ReadLastRebootTime() expected error for corrupted file")
	}
	if !result.IsZero() {
		t.Errorf("ReadLastRebootTime() = %v, want zero time", result)
	}
}

func TestWriteLastRebootTime_CreatesDir(t *testing.T) {
	hostRoot := t.TempDir()
	sm := NewStateManager(hostRoot)

	now := time.Now().Truncate(time.Second).UTC()
	err := sm.WriteLastRebootTime(now)

	if err != nil {
		t.Fatalf("WriteLastRebootTime() error = %v", err)
	}

	// Verify directory was created
	stateDir := filepath.Join(hostRoot, "var", "lib", "mco")
	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		t.Error("state directory was not created")
	}

	// Verify file was created
	filePath := filepath.Join(stateDir, "last-reboot")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("last-reboot file was not created")
	}
}

func TestWriteLastRebootTime_Overwrites(t *testing.T) {
	hostRoot := t.TempDir()
	sm := NewStateManager(hostRoot)

	// Write first time
	first := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := sm.WriteLastRebootTime(first); err != nil {
		t.Fatalf("WriteLastRebootTime() error = %v", err)
	}

	// Write second time
	second := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	if err := sm.WriteLastRebootTime(second); err != nil {
		t.Fatalf("WriteLastRebootTime() error = %v", err)
	}

	// Read it back
	result, err := sm.ReadLastRebootTime()
	if err != nil {
		t.Fatalf("ReadLastRebootTime() error = %v", err)
	}

	if !result.Equal(second) {
		t.Errorf("ReadLastRebootTime() = %v, want %v", result, second)
	}
}

func TestRoundtrip(t *testing.T) {
	hostRoot := t.TempDir()
	sm := NewStateManager(hostRoot)

	// Use a fixed time (truncated to seconds since RFC3339 doesn't preserve nanoseconds)
	original := time.Now().Truncate(time.Second).UTC()

	// Write
	if err := sm.WriteLastRebootTime(original); err != nil {
		t.Fatalf("WriteLastRebootTime() error = %v", err)
	}

	// Read
	result, err := sm.ReadLastRebootTime()
	if err != nil {
		t.Fatalf("ReadLastRebootTime() error = %v", err)
	}

	if !result.Equal(original) {
		t.Errorf("Roundtrip failed: wrote %v, read %v", original, result)
	}
}

func TestGetSystemBootTime(t *testing.T) {
	hostRoot := t.TempDir()
	sm := NewStateManager(hostRoot)

	// Create mock /proc/uptime
	procDir := filepath.Join(hostRoot, "proc")
	if err := os.MkdirAll(procDir, 0755); err != nil {
		t.Fatalf("failed to create proc dir: %v", err)
	}

	// uptime format: "uptime_seconds idle_seconds"
	// 3600 seconds = 1 hour uptime
	uptimeContent := "3600.50 7200.25\n"
	if err := os.WriteFile(filepath.Join(procDir, "uptime"), []byte(uptimeContent), 0644); err != nil {
		t.Fatalf("failed to write /proc/uptime: %v", err)
	}

	before := time.Now()
	result, err := sm.GetSystemBootTime()
	after := time.Now()

	if err != nil {
		t.Fatalf("GetSystemBootTime() error = %v", err)
	}

	// Boot time should be approximately 1 hour ago
	expectedBoot := before.Add(-3600 * time.Second)

	// Allow 2 second tolerance for test execution time
	if result.Before(expectedBoot.Add(-2*time.Second)) || result.After(after.Add(-3600*time.Second).Add(2*time.Second)) {
		t.Errorf("GetSystemBootTime() = %v, expected around %v (1 hour ago)", result, expectedBoot)
	}
}

func TestGetSystemBootTime_InvalidFormat(t *testing.T) {
	hostRoot := t.TempDir()
	sm := NewStateManager(hostRoot)

	// Create mock /proc/uptime with invalid format
	procDir := filepath.Join(hostRoot, "proc")
	if err := os.MkdirAll(procDir, 0755); err != nil {
		t.Fatalf("failed to create proc dir: %v", err)
	}

	// Invalid uptime content (non-numeric)
	uptimeContent := "not-a-number idle\n"
	if err := os.WriteFile(filepath.Join(procDir, "uptime"), []byte(uptimeContent), 0644); err != nil {
		t.Fatalf("failed to write /proc/uptime: %v", err)
	}

	_, err := sm.GetSystemBootTime()

	if err == nil {
		t.Fatal("GetSystemBootTime() expected error for invalid uptime format")
	}
}

func TestGetSystemBootTime_MissingFile(t *testing.T) {
	sm := NewStateManager(t.TempDir())

	_, err := sm.GetSystemBootTime()

	if err == nil {
		t.Fatal("GetSystemBootTime() expected error for missing /proc/uptime")
	}
}

func TestStatePath(t *testing.T) {
	sm := NewStateManager("/host")

	tests := []struct {
		filename string
		want     string
	}{
		{"", "/host/var/lib/mco"},
		{"last-reboot", "/host/var/lib/mco/last-reboot"},
		{"other-file", "/host/var/lib/mco/other-file"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := sm.statePath(tt.filename)
			if result != tt.want {
				t.Errorf("statePath(%q) = %q, want %q", tt.filename, result, tt.want)
			}
		})
	}
}
