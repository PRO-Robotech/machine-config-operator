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
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// Helper to create a MachineConfig with common defaults
func newMachineConfig(name string, priority int) *mcov1alpha1.MachineConfig {
	return &mcov1alpha1.MachineConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: mcov1alpha1.MachineConfigSpec{
			Priority: priority,
		},
	}
}

// TestMerge_EmptyConfigs verifies empty input returns empty result.
func TestMerge_EmptyConfigs(t *testing.T) {
	result := Merge(nil)

	if result == nil {
		t.Fatal("Merge(nil) returned nil, expected empty MergedConfig")
	}
	if len(result.Files) != 0 {
		t.Errorf("Files = %v, want empty", result.Files)
	}
	if len(result.Units) != 0 {
		t.Errorf("Units = %v, want empty", result.Units)
	}
	if result.RebootRequired {
		t.Error("RebootRequired = true, want false")
	}
	if len(result.Sources) != 0 {
		t.Errorf("Sources = %v, want empty", result.Sources)
	}

	// Also test with empty slice
	result2 := Merge([]*mcov1alpha1.MachineConfig{})
	if result2 == nil {
		t.Fatal("Merge([]) returned nil")
	}
}

// TestMerge_SingleConfig verifies single config passes through correctly.
func TestMerge_SingleConfig(t *testing.T) {
	mc := newMachineConfig("single", 50)
	mc.Spec.Files = []mcov1alpha1.FileSpec{
		{Path: "/etc/test.conf", Content: "test content", Mode: 420},
	}
	mc.Spec.Systemd.Units = []mcov1alpha1.UnitSpec{
		{Name: "test.service", State: "started"},
	}
	mc.Spec.Reboot.Required = true

	result := Merge([]*mcov1alpha1.MachineConfig{mc})

	if len(result.Files) != 1 {
		t.Fatalf("Files count = %d, want 1", len(result.Files))
	}
	if result.Files[0].Path != "/etc/test.conf" {
		t.Errorf("Files[0].Path = %q, want /etc/test.conf", result.Files[0].Path)
	}
	if len(result.Units) != 1 {
		t.Fatalf("Units count = %d, want 1", len(result.Units))
	}
	if result.Units[0].Name != "test.service" {
		t.Errorf("Units[0].Name = %q, want test.service", result.Units[0].Name)
	}
	if !result.RebootRequired {
		t.Error("RebootRequired = false, want true")
	}
	if len(result.Sources) != 1 || result.Sources[0].Name != "single" {
		t.Errorf("Sources = %v, want [{single 50}]", result.Sources)
	}
}

// TestMerge_PriorityOrdering verifies configs are sorted by priority ASC.
// Higher priority configs should win on conflicts.
func TestMerge_PriorityOrdering(t *testing.T) {
	// Create configs with different priorities
	low := newMachineConfig("config-low", 10)
	low.Spec.Files = []mcov1alpha1.FileSpec{
		{Path: "/etc/shared.conf", Content: "low priority content"},
	}

	high := newMachineConfig("config-high", 90)
	high.Spec.Files = []mcov1alpha1.FileSpec{
		{Path: "/etc/shared.conf", Content: "high priority content"},
	}

	// Pass in reverse order to verify sorting works
	result := Merge([]*mcov1alpha1.MachineConfig{high, low})

	if len(result.Files) != 1 {
		t.Fatalf("Files count = %d, want 1 (deduplicated)", len(result.Files))
	}
	if result.Files[0].Content != "high priority content" {
		t.Errorf("File content = %q, want 'high priority content' (higher priority wins)",
			result.Files[0].Content)
	}

	if len(result.Sources) != 2 {
		t.Fatalf("Sources count = %d, want 2", len(result.Sources))
	}
	if result.Sources[0].Name != "config-low" {
		t.Errorf("Sources[0].Name = %q, want config-low (sorted by priority)", result.Sources[0].Name)
	}
	if result.Sources[1].Name != "config-high" {
		t.Errorf("Sources[1].Name = %q, want config-high", result.Sources[1].Name)
	}
}

// TestMerge_NameTieBreaker verifies alphabetical name ordering when priorities match.
func TestMerge_NameTieBreaker(t *testing.T) {
	configA := newMachineConfig("aaa-config", 50)
	configA.Spec.Files = []mcov1alpha1.FileSpec{
		{Path: "/etc/shared.conf", Content: "from aaa"},
	}

	configZ := newMachineConfig("zzz-config", 50)
	configZ.Spec.Files = []mcov1alpha1.FileSpec{
		{Path: "/etc/shared.conf", Content: "from zzz"},
	}

	result := Merge([]*mcov1alpha1.MachineConfig{configZ, configA})

	if result.Files[0].Content != "from zzz" {
		t.Errorf("File content = %q, want 'from zzz' (later name wins)",
			result.Files[0].Content)
	}

	if result.Sources[0].Name != "aaa-config" {
		t.Errorf("Sources[0].Name = %q, want aaa-config", result.Sources[0].Name)
	}
	if result.Sources[1].Name != "zzz-config" {
		t.Errorf("Sources[1].Name = %q, want zzz-config", result.Sources[1].Name)
	}
}

// TestMerge_FileConflicts verifies last-writer-wins for file paths.
func TestMerge_FileConflicts(t *testing.T) {
	// Three configs with overlapping files
	mc1 := newMachineConfig("mc1", 10)
	mc1.Spec.Files = []mcov1alpha1.FileSpec{
		{Path: "/etc/file-a.conf", Content: "a from mc1"},
		{Path: "/etc/file-b.conf", Content: "b from mc1"},
	}

	mc2 := newMachineConfig("mc2", 20)
	mc2.Spec.Files = []mcov1alpha1.FileSpec{
		{Path: "/etc/file-b.conf", Content: "b from mc2"},
		{Path: "/etc/file-c.conf", Content: "c from mc2"},
	}

	mc3 := newMachineConfig("mc3", 30)
	mc3.Spec.Files = []mcov1alpha1.FileSpec{
		{Path: "/etc/file-a.conf", Content: "a from mc3"},
	}

	result := Merge([]*mcov1alpha1.MachineConfig{mc1, mc2, mc3})

	// Should have 3 files (deduplicated)
	if len(result.Files) != 3 {
		t.Fatalf("Files count = %d, want 3", len(result.Files))
	}

	fileMap := make(map[string]string)
	for _, f := range result.Files {
		fileMap[f.Path] = f.Content
	}

	// file-a: mc3 wins (priority 30)
	if fileMap["/etc/file-a.conf"] != "a from mc3" {
		t.Errorf("/etc/file-a.conf = %q, want 'a from mc3'", fileMap["/etc/file-a.conf"])
	}
	// file-b: mc2 wins (priority 20, mc3 doesn't define it)
	if fileMap["/etc/file-b.conf"] != "b from mc2" {
		t.Errorf("/etc/file-b.conf = %q, want 'b from mc2'", fileMap["/etc/file-b.conf"])
	}
	// file-c: only mc2 defines it
	if fileMap["/etc/file-c.conf"] != "c from mc2" {
		t.Errorf("/etc/file-c.conf = %q, want 'c from mc2'", fileMap["/etc/file-c.conf"])
	}
}

// TestMerge_UnitConflicts verifies last-writer-wins for unit names.
func TestMerge_UnitConflicts(t *testing.T) {
	mc1 := newMachineConfig("mc1", 10)
	mc1.Spec.Systemd.Units = []mcov1alpha1.UnitSpec{
		{Name: "test.service", State: "stopped"},
	}

	mc2 := newMachineConfig("mc2", 20)
	mc2.Spec.Systemd.Units = []mcov1alpha1.UnitSpec{
		{Name: "test.service", State: "started"},
	}

	result := Merge([]*mcov1alpha1.MachineConfig{mc1, mc2})

	if len(result.Units) != 1 {
		t.Fatalf("Units count = %d, want 1", len(result.Units))
	}
	// mc2 wins (higher priority)
	if result.Units[0].State != "started" {
		t.Errorf("Unit state = %q, want 'started' (mc2 wins)", result.Units[0].State)
	}
}

// TestMerge_RebootOR verifies reboot.required is ORed across all configs.
func TestMerge_RebootOR(t *testing.T) {
	tests := []struct {
		name     string
		reboots  []bool
		expected bool
	}{
		{"all false", []bool{false, false, false}, false},
		{"first true", []bool{true, false, false}, true},
		{"middle true", []bool{false, true, false}, true},
		{"last true", []bool{false, false, true}, true},
		{"all true", []bool{true, true, true}, true},
		{"single false", []bool{false}, false},
		{"single true", []bool{true}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configs := make([]*mcov1alpha1.MachineConfig, len(tt.reboots))
			for i, reboot := range tt.reboots {
				configs[i] = newMachineConfig("mc"+string(rune('0'+i)), i*10)
				configs[i].Spec.Reboot.Required = reboot
			}

			result := Merge(configs)

			if result.RebootRequired != tt.expected {
				t.Errorf("RebootRequired = %v, want %v", result.RebootRequired, tt.expected)
			}
		})
	}
}

// TestMerge_DeterministicOutput verifies files and units are sorted.
func TestMerge_DeterministicOutput(t *testing.T) {
	mc := newMachineConfig("mc", 50)
	mc.Spec.Files = []mcov1alpha1.FileSpec{
		{Path: "/etc/z-last.conf", Content: "z"},
		{Path: "/etc/a-first.conf", Content: "a"},
		{Path: "/etc/m-middle.conf", Content: "m"},
	}
	mc.Spec.Systemd.Units = []mcov1alpha1.UnitSpec{
		{Name: "zzz.service"},
		{Name: "aaa.service"},
		{Name: "mmm.service"},
	}

	result := Merge([]*mcov1alpha1.MachineConfig{mc})

	// Files should be sorted by path
	expectedFilePaths := []string{"/etc/a-first.conf", "/etc/m-middle.conf", "/etc/z-last.conf"}
	for i, expected := range expectedFilePaths {
		if result.Files[i].Path != expected {
			t.Errorf("Files[%d].Path = %q, want %q", i, result.Files[i].Path, expected)
		}
	}

	// Units should be sorted by name
	expectedUnitNames := []string{"aaa.service", "mmm.service", "zzz.service"}
	for i, expected := range expectedUnitNames {
		if result.Units[i].Name != expected {
			t.Errorf("Units[%d].Name = %q, want %q", i, result.Units[i].Name, expected)
		}
	}
}

// TestMerge_SourcesTracking verifies all sources are recorded in order.
func TestMerge_SourcesTracking(t *testing.T) {
	configs := []*mcov1alpha1.MachineConfig{
		newMachineConfig("base-config", 10),
		newMachineConfig("override-config", 50),
		newMachineConfig("final-config", 90),
	}

	result := Merge(configs)

	expected := []ConfigSource{
		{Name: "base-config", Priority: 10},
		{Name: "override-config", Priority: 50},
		{Name: "final-config", Priority: 90},
	}

	if !reflect.DeepEqual(result.Sources, expected) {
		t.Errorf("Sources = %v, want %v", result.Sources, expected)
	}
}

// TestMerge_DoesNotMutateInput verifies the input slice is not modified.
func TestMerge_DoesNotMutateInput(t *testing.T) {
	mc1 := newMachineConfig("mc1", 90)
	mc2 := newMachineConfig("mc2", 10)
	original := []*mcov1alpha1.MachineConfig{mc1, mc2}

	// Keep references to check order
	firstBefore := original[0].Name
	secondBefore := original[1].Name

	_ = Merge(original)

	// Original slice should be unchanged
	if original[0].Name != firstBefore || original[1].Name != secondBefore {
		t.Error("Merge mutated the input slice order")
	}
}

// TestSortByPriority verifies the sorting function directly.
func TestSortByPriority(t *testing.T) {
	configs := []*mcov1alpha1.MachineConfig{
		newMachineConfig("z-high", 90),
		newMachineConfig("a-low", 10),
		newMachineConfig("m-med", 50),
		newMachineConfig("a-med", 50), // Same priority as m-med, earlier name
	}

	sorted := sortByPriority(configs)

	expected := []string{"a-low", "a-med", "m-med", "z-high"}
	for i, name := range expected {
		if sorted[i].Name != name {
			t.Errorf("sorted[%d].Name = %q, want %q", i, sorted[i].Name, name)
		}
	}

	// Verify original is not mutated
	if configs[0].Name != "z-high" {
		t.Error("sortByPriority mutated the input slice")
	}
}

// TestFilesToSortedSlice verifies file slice conversion.
func TestFilesToSortedSlice(t *testing.T) {
	files := map[string]mcov1alpha1.FileSpec{
		"/etc/z.conf": {Path: "/etc/z.conf"},
		"/etc/a.conf": {Path: "/etc/a.conf"},
	}

	result := filesToSortedSlice(files)

	if len(result) != 2 {
		t.Fatalf("result length = %d, want 2", len(result))
	}
	if result[0].Path != "/etc/a.conf" {
		t.Errorf("result[0].Path = %q, want /etc/a.conf", result[0].Path)
	}
	if result[1].Path != "/etc/z.conf" {
		t.Errorf("result[1].Path = %q, want /etc/z.conf", result[1].Path)
	}

	// Test empty map
	empty := filesToSortedSlice(map[string]mcov1alpha1.FileSpec{})
	if len(empty) != 0 {
		t.Errorf("empty map result = %v, want []", empty)
	}
}

// TestUnitsToSortedSlice verifies unit slice conversion.
func TestUnitsToSortedSlice(t *testing.T) {
	units := map[string]mcov1alpha1.UnitSpec{
		"z.service": {Name: "z.service"},
		"a.service": {Name: "a.service"},
	}

	result := unitsToSortedSlice(units)

	if len(result) != 2 {
		t.Fatalf("result length = %d, want 2", len(result))
	}
	if result[0].Name != "a.service" {
		t.Errorf("result[0].Name = %q, want a.service", result[0].Name)
	}
	if result[1].Name != "z.service" {
		t.Errorf("result[1].Name = %q, want z.service", result[1].Name)
	}

	// Test empty map
	empty := unitsToSortedSlice(map[string]mcov1alpha1.UnitSpec{})
	if len(empty) != 0 {
		t.Errorf("empty map result = %v, want []", empty)
	}
}

// TestMerge_PreservesAllFileFields verifies all FileSpec fields are preserved.
func TestMerge_PreservesAllFileFields(t *testing.T) {
	mc := newMachineConfig("mc", 50)
	mc.Spec.Files = []mcov1alpha1.FileSpec{
		{
			Path:    "/etc/test.conf",
			Content: "content here",
			Mode:    0600,
			Owner:   "nobody:nogroup",
			State:   "present",
		},
	}

	result := Merge([]*mcov1alpha1.MachineConfig{mc})

	f := result.Files[0]
	if f.Path != "/etc/test.conf" {
		t.Errorf("Path = %q, want /etc/test.conf", f.Path)
	}
	if f.Content != "content here" {
		t.Errorf("Content = %q, want 'content here'", f.Content)
	}
	if f.Mode != 0600 {
		t.Errorf("Mode = %o, want 0600", f.Mode)
	}
	if f.Owner != "nobody:nogroup" {
		t.Errorf("Owner = %q, want nobody:nogroup", f.Owner)
	}
	if f.State != "present" {
		t.Errorf("State = %q, want present", f.State)
	}
}

// TestMerge_PreservesAllUnitFields verifies all UnitSpec fields are preserved.
func TestMerge_PreservesAllUnitFields(t *testing.T) {
	enabled := true
	mc := newMachineConfig("mc", 50)
	mc.Spec.Systemd.Units = []mcov1alpha1.UnitSpec{
		{
			Name:    "test.service",
			Enabled: &enabled,
			State:   "started",
			Mask:    true,
		},
	}

	result := Merge([]*mcov1alpha1.MachineConfig{mc})

	u := result.Units[0]
	if u.Name != "test.service" {
		t.Errorf("Name = %q, want test.service", u.Name)
	}
	if u.Enabled == nil || !*u.Enabled {
		t.Errorf("Enabled = %v, want true", u.Enabled)
	}
	if u.State != "started" {
		t.Errorf("State = %q, want started", u.State)
	}
	if !u.Mask {
		t.Error("Mask = false, want true")
	}
}

// TestMerge_EmptyFilesAndUnits verifies configs with no files/units work correctly.
func TestMerge_EmptyFilesAndUnits(t *testing.T) {
	mc := newMachineConfig("empty", 50)

	result := Merge([]*mcov1alpha1.MachineConfig{mc})

	if len(result.Files) != 0 {
		t.Errorf("Files = %v, want empty", result.Files)
	}
	if len(result.Units) != 0 {
		t.Errorf("Units = %v, want empty", result.Units)
	}
	if len(result.Sources) != 1 {
		t.Errorf("Sources count = %d, want 1", len(result.Sources))
	}
}

// TestMerge_FileRebootRequirements verifies per-file reboot requirements are tracked.
func TestMerge_FileRebootRequirements(t *testing.T) {
	// MC with reboot required
	kernel := newMachineConfig("10-kernel", 10)
	kernel.Spec.Reboot.Required = true
	kernel.Spec.Files = []mcov1alpha1.FileSpec{
		{Path: "/etc/sysctl.conf", Content: "vm.swappiness=10"},
	}

	// MC without reboot required
	ntp := newMachineConfig("50-ntp", 50)
	ntp.Spec.Reboot.Required = false
	ntp.Spec.Files = []mcov1alpha1.FileSpec{
		{Path: "/etc/chrony.conf", Content: "server ntp.example.com"},
	}

	result := Merge([]*mcov1alpha1.MachineConfig{kernel, ntp})

	// Verify FileRebootRequirements
	if result.FileRebootRequirements == nil {
		t.Fatal("FileRebootRequirements is nil")
	}
	if len(result.FileRebootRequirements) != 2 {
		t.Fatalf("FileRebootRequirements count = %d, want 2", len(result.FileRebootRequirements))
	}

	// sysctl.conf comes from kernel MC (reboot: true)
	if !result.FileRebootRequirements["/etc/sysctl.conf"] {
		t.Error("FileRebootRequirements['/etc/sysctl.conf'] = false, want true")
	}
	// chrony.conf comes from ntp MC (reboot: false)
	if result.FileRebootRequirements["/etc/chrony.conf"] {
		t.Error("FileRebootRequirements['/etc/chrony.conf'] = true, want false")
	}
}

// TestMerge_FileRebootRequirements_Override verifies winning MC's reboot setting is used.
func TestMerge_FileRebootRequirements_Override(t *testing.T) {
	// Base MC with reboot required
	base := newMachineConfig("10-base", 10)
	base.Spec.Reboot.Required = true
	base.Spec.Files = []mcov1alpha1.FileSpec{
		{Path: "/etc/foo.conf", Content: "base content"},
	}

	// Override MC without reboot required (higher priority)
	override := newMachineConfig("50-override", 50)
	override.Spec.Reboot.Required = false
	override.Spec.Files = []mcov1alpha1.FileSpec{
		{Path: "/etc/foo.conf", Content: "override content"},
	}

	result := Merge([]*mcov1alpha1.MachineConfig{base, override})

	// Verify file content from override (higher priority wins)
	if result.Files[0].Content != "override content" {
		t.Errorf("File content = %q, want 'override content'", result.Files[0].Content)
	}

	// Verify reboot requirement from override MC (higher priority wins)
	if result.FileRebootRequirements["/etc/foo.conf"] {
		t.Error("FileRebootRequirements['/etc/foo.conf'] = true, want false (override wins)")
	}

	if !result.RebootRequired {
		t.Error("RebootRequired = false, want true (OR of all MCs)")
	}
}

// TestMerge_FileRebootRequirements_NameTiebreaker verifies name tiebreaker for reboot requirements.
func TestMerge_FileRebootRequirements_NameTiebreaker(t *testing.T) {
	alpha := newMachineConfig("50-alpha", 50)
	alpha.Spec.Reboot.Required = false
	alpha.Spec.Files = []mcov1alpha1.FileSpec{
		{Path: "/etc/foo.conf", Content: "from alpha"},
	}

	beta := newMachineConfig("50-beta", 50)
	beta.Spec.Reboot.Required = true
	beta.Spec.Files = []mcov1alpha1.FileSpec{
		{Path: "/etc/foo.conf", Content: "from beta"},
	}

	result := Merge([]*mcov1alpha1.MachineConfig{beta, alpha})

	// "beta" comes after "alpha" alphabetically, so "beta" wins
	if result.Files[0].Content != "from beta" {
		t.Errorf("File content = %q, want 'from beta'", result.Files[0].Content)
	}

	// Reboot requirement from beta (the winner)
	if !result.FileRebootRequirements["/etc/foo.conf"] {
		t.Error("FileRebootRequirements['/etc/foo.conf'] = false, want true (beta wins)")
	}
}

// TestMerge_UnitRebootRequirements verifies per-unit reboot requirements are tracked.
func TestMerge_UnitRebootRequirements(t *testing.T) {
	enabled := true

	// MC with reboot required
	kernel := newMachineConfig("10-kernel", 10)
	kernel.Spec.Reboot.Required = true
	kernel.Spec.Systemd.Units = []mcov1alpha1.UnitSpec{
		{Name: "containerd.service", Enabled: &enabled, State: "started"},
	}

	// MC without reboot required
	app := newMachineConfig("50-app", 50)
	app.Spec.Reboot.Required = false
	app.Spec.Systemd.Units = []mcov1alpha1.UnitSpec{
		{Name: "myapp.service", Enabled: &enabled, State: "started"},
	}

	result := Merge([]*mcov1alpha1.MachineConfig{kernel, app})

	// Verify UnitRebootRequirements
	if result.UnitRebootRequirements == nil {
		t.Fatal("UnitRebootRequirements is nil")
	}
	if len(result.UnitRebootRequirements) != 2 {
		t.Fatalf("UnitRebootRequirements count = %d, want 2", len(result.UnitRebootRequirements))
	}

	// containerd.service from kernel MC (reboot: true)
	if !result.UnitRebootRequirements["containerd.service"] {
		t.Error("UnitRebootRequirements['containerd.service'] = false, want true")
	}
	// myapp.service from app MC (reboot: false)
	if result.UnitRebootRequirements["myapp.service"] {
		t.Error("UnitRebootRequirements['myapp.service'] = true, want false")
	}
}

// TestMerge_EmptyRebootRequirements verifies empty configs have empty maps.
func TestMerge_EmptyRebootRequirements(t *testing.T) {
	// Empty config list
	result := Merge(nil)
	if result.FileRebootRequirements == nil {
		t.Error("FileRebootRequirements should not be nil for empty config")
	}
	if len(result.FileRebootRequirements) != 0 {
		t.Errorf("FileRebootRequirements = %v, want empty", result.FileRebootRequirements)
	}
	if result.UnitRebootRequirements == nil {
		t.Error("UnitRebootRequirements should not be nil for empty config")
	}
	if len(result.UnitRebootRequirements) != 0 {
		t.Errorf("UnitRebootRequirements = %v, want empty", result.UnitRebootRequirements)
	}

	// Config with reboot but no files/units
	mc := newMachineConfig("empty", 50)
	mc.Spec.Reboot.Required = true

	result2 := Merge([]*mcov1alpha1.MachineConfig{mc})
	if len(result2.FileRebootRequirements) != 0 {
		t.Errorf("FileRebootRequirements = %v, want empty", result2.FileRebootRequirements)
	}
	if len(result2.UnitRebootRequirements) != 0 {
		t.Errorf("UnitRebootRequirements = %v, want empty", result2.UnitRebootRequirements)
	}
}

// TestMerge_ComplexScenario tests a realistic multi-config merge.
func TestMerge_ComplexScenario(t *testing.T) {
	// Base config: lowest priority, provides defaults
	base := newMachineConfig("00-base", 10)
	base.Spec.Files = []mcov1alpha1.FileSpec{
		{Path: "/etc/ntp.conf", Content: "server pool.ntp.org", Mode: 420},
		{Path: "/etc/sysctl.d/99-base.conf", Content: "vm.swappiness=10"},
	}
	base.Spec.Systemd.Units = []mcov1alpha1.UnitSpec{
		{Name: "ntpd.service", State: "started"},
	}

	// Role config: override NTP for this role
	role := newMachineConfig("50-worker", 50)
	role.Spec.Files = []mcov1alpha1.FileSpec{
		{Path: "/etc/ntp.conf", Content: "server internal-ntp.local"},
	}
	role.Spec.Reboot.Required = true

	// Custom config: highest priority
	custom := newMachineConfig("99-emergency", 99)
	custom.Spec.Systemd.Units = []mcov1alpha1.UnitSpec{
		{Name: "ntpd.service", State: "stopped"},
	}

	result := Merge([]*mcov1alpha1.MachineConfig{custom, base, role})

	// Verify file merge
	if len(result.Files) != 2 {
		t.Fatalf("Files count = %d, want 2", len(result.Files))
	}

	fileMap := make(map[string]mcov1alpha1.FileSpec)
	for _, f := range result.Files {
		fileMap[f.Path] = f
	}

	// NTP: role config wins (priority 50 > base 10)
	if fileMap["/etc/ntp.conf"].Content != "server internal-ntp.local" {
		t.Errorf("NTP content wrong, role should override base")
	}
	// sysctl: only base defines it
	if fileMap["/etc/sysctl.d/99-base.conf"].Content != "vm.swappiness=10" {
		t.Errorf("sysctl content wrong")
	}

	// Verify unit merge
	if len(result.Units) != 1 {
		t.Fatalf("Units count = %d, want 1", len(result.Units))
	}
	// Emergency config wins (priority 99)
	if result.Units[0].State != "stopped" {
		t.Errorf("Unit state = %q, want 'stopped' (emergency override)", result.Units[0].State)
	}

	// Verify reboot OR
	if !result.RebootRequired {
		t.Error("RebootRequired = false, want true (role requires it)")
	}

	// Verify sources in priority order
	if len(result.Sources) != 3 {
		t.Fatalf("Sources count = %d, want 3", len(result.Sources))
	}
	expectedOrder := []string{"00-base", "50-worker", "99-emergency"}
	for i, name := range expectedOrder {
		if result.Sources[i].Name != name {
			t.Errorf("Sources[%d].Name = %q, want %q", i, result.Sources[i].Name, name)
		}
	}
}

// TestMerge_TieBreaker_LargerNameWins documents OpenShift-compatible behavior:
// At equal priority, larger name wins (later in sorted order overwrites).
// This is consistent with OpenShift MCO where alphanumerically larger names
// take precedence (e.g., 99-custom > 00-base, 50-beta > 50-alpha).
func TestMerge_TieBreaker_LargerNameWins(t *testing.T) {
	// Create two configs with SAME priority but different names
	mcAlpha := newMachineConfig("50-alpha", 50)
	mcAlpha.Spec.Files = []mcov1alpha1.FileSpec{
		{Path: "/etc/test.conf", Content: "alpha content"},
	}

	mcBeta := newMachineConfig("50-beta", 50)
	mcBeta.Spec.Files = []mcov1alpha1.FileSpec{
		{Path: "/etc/test.conf", Content: "beta content"},
	}

	// Pass in arbitrary order - merge should sort and apply consistently
	result := Merge([]*mcov1alpha1.MachineConfig{mcBeta, mcAlpha})

	// Verify 50-beta wins (larger name)
	if len(result.Files) != 1 {
		t.Fatalf("Files count = %d, want 1", len(result.Files))
	}
	if result.Files[0].Content != "beta content" {
		t.Errorf("File content = %q, want 'beta content' (larger name 50-beta should win)", result.Files[0].Content)
	}

	// Also verify the order in sources: alpha before beta (sorted by name ASC)
	if len(result.Sources) != 2 {
		t.Fatalf("Sources count = %d, want 2", len(result.Sources))
	}
	if result.Sources[0].Name != "50-alpha" || result.Sources[1].Name != "50-beta" {
		t.Errorf("Sources order = %v, want [50-alpha, 50-beta]", result.Sources)
	}
}

// TestMerge_TieBreaker_NamingConvention verifies the common naming convention
// where configs are named <priority>-<name> and higher numbers override lower.
func TestMerge_TieBreaker_NamingConvention(t *testing.T) {
	// Simulate common naming: 00-base, 50-role, 99-override
	// All have explicit priority=50 but names imply precedence
	mcBase := newMachineConfig("00-base", 50)
	mcBase.Spec.Files = []mcov1alpha1.FileSpec{
		{Path: "/etc/config", Content: "base"},
	}

	mcOverride := newMachineConfig("99-override", 50)
	mcOverride.Spec.Files = []mcov1alpha1.FileSpec{
		{Path: "/etc/config", Content: "override"},
	}

	result := Merge([]*mcov1alpha1.MachineConfig{mcOverride, mcBase})

	// 99-override should win (larger name)
	if len(result.Files) != 1 {
		t.Fatalf("Files count = %d, want 1", len(result.Files))
	}
	if result.Files[0].Content != "override" {
		t.Errorf("File content = %q, want 'override' (99-override should win over 00-base)", result.Files[0].Content)
	}
}
