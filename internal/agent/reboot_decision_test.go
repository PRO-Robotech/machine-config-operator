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
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// mockRMCFetcher implements RMCFetcher for testing.
type mockRMCFetcher struct {
	rmcs map[string]*mcov1alpha1.RenderedMachineConfig
	err  error
}

func (m *mockRMCFetcher) FetchRMC(ctx context.Context, name string) (*mcov1alpha1.RenderedMachineConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	rmc, ok := m.rmcs[name]
	if !ok {
		return nil, errors.New("RMC not found")
	}
	return rmc, nil
}

// Helper to create a simple RMC for testing.
func makeRMC(name string, files []mcov1alpha1.FileSpec, units []mcov1alpha1.UnitSpec, legacyReboot bool, fileReqs map[string]bool, unitReqs map[string]bool) *mcov1alpha1.RenderedMachineConfig {
	return &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Config: mcov1alpha1.RenderedConfig{
				Files: files,
				Systemd: mcov1alpha1.SystemdSpec{
					Units: units,
				},
			},
			Reboot: mcov1alpha1.RenderedRebootSpec{
				Required: legacyReboot,
			},
			RebootRequirements: mcov1alpha1.RebootRequirements{
				Files: fileReqs,
				Units: unitReqs,
			},
		},
	}
}

// TestDetermineReboot_FirstApply verifies first apply uses legacy logic.
func TestDetermineReboot_FirstApply(t *testing.T) {
	tests := []struct {
		name           string
		legacyReboot   bool
		expectRequired bool
	}{
		{"legacy reboot true", true, true},
		{"legacy reboot false", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetcher := &mockRMCFetcher{}
			determiner := NewRebootDeterminer(fetcher)

			newRMC := makeRMC("workers-abc123", nil, nil, tt.legacyReboot, nil, nil)

			decision := determiner.DetermineReboot(context.Background(), "", newRMC)

			if decision.Required != tt.expectRequired {
				t.Errorf("Expected Required=%v, got %v", tt.expectRequired, decision.Required)
			}
			if decision.Method != MethodLegacyFirstApply {
				t.Errorf("Expected Method=%s, got %s", MethodLegacyFirstApply, decision.Method)
			}
			if len(decision.Reasons) != 1 || decision.Reasons[0] != "first apply" {
				t.Errorf("Expected Reasons=[first apply], got %v", decision.Reasons)
			}
		})
	}
}

// TestDetermineReboot_SameRevision verifies no reboot when revision unchanged.
func TestDetermineReboot_SameRevision(t *testing.T) {
	fetcher := &mockRMCFetcher{}
	determiner := NewRebootDeterminer(fetcher)

	newRMC := makeRMC("workers-abc123", nil, nil, true, nil, nil)

	decision := determiner.DetermineReboot(context.Background(), "workers-abc123", newRMC)

	if decision.Required {
		t.Error("Expected Required=false for same revision")
	}
	if decision.Method != MethodSameRevision {
		t.Errorf("Expected Method=%s, got %s", MethodSameRevision, decision.Method)
	}
}

// TestDetermineReboot_CurrentRMCNotFound verifies fallback when current RMC missing.
func TestDetermineReboot_CurrentRMCNotFound(t *testing.T) {
	fetcher := &mockRMCFetcher{
		err: errors.New("not found"),
	}
	determiner := NewRebootDeterminer(fetcher)

	newRMC := makeRMC("workers-new123", nil, nil, true, nil, nil)

	decision := determiner.DetermineReboot(context.Background(), "workers-old123", newRMC)

	if !decision.Required {
		t.Error("Expected Required=true (fallback to legacy)")
	}
	if decision.Method != MethodLegacyFallback {
		t.Errorf("Expected Method=%s, got %s", MethodLegacyFallback, decision.Method)
	}
	if len(decision.Reasons) != 1 {
		t.Errorf("Expected 1 reason, got %d", len(decision.Reasons))
	}
}

// TestDetermineReboot_MissingRebootRequirements verifies fallback when RebootRequirements empty.
func TestDetermineReboot_MissingRebootRequirements(t *testing.T) {
	currentRMC := makeRMC("workers-old123",
		[]mcov1alpha1.FileSpec{{Path: "/etc/a.conf", Content: "a"}},
		nil, true, nil, nil)

	newRMC := makeRMC("workers-new123",
		[]mcov1alpha1.FileSpec{{Path: "/etc/a.conf", Content: "b"}},
		nil, true, nil, nil) // No RebootRequirements

	fetcher := &mockRMCFetcher{
		rmcs: map[string]*mcov1alpha1.RenderedMachineConfig{
			"workers-old123": currentRMC,
		},
	}
	determiner := NewRebootDeterminer(fetcher)

	decision := determiner.DetermineReboot(context.Background(), "workers-old123", newRMC)

	if !decision.Required {
		t.Error("Expected Required=true (fallback to legacy)")
	}
	if decision.Method != MethodLegacyFallback {
		t.Errorf("Expected Method=%s, got %s", MethodLegacyFallback, decision.Method)
	}
}

// TestDetermineReboot_AddNonRebootFile verifies no reboot when adding non-reboot file.
func TestDetermineReboot_AddNonRebootFile(t *testing.T) {
	currentRMC := makeRMC("workers-old123",
		[]mcov1alpha1.FileSpec{
			{Path: "/etc/a.conf", Content: "a", Mode: 0644, Owner: "root:root", State: "present"},
		},
		nil, true,
		map[string]bool{"/etc/a.conf": true},
		nil)

	newRMC := makeRMC("workers-new123",
		[]mcov1alpha1.FileSpec{
			{Path: "/etc/a.conf", Content: "a", Mode: 0644, Owner: "root:root", State: "present"},
			{Path: "/etc/b.conf", Content: "b", Mode: 0644, Owner: "root:root", State: "present"},
		},
		nil, true,
		map[string]bool{"/etc/a.conf": true, "/etc/b.conf": false},
		nil)

	fetcher := &mockRMCFetcher{
		rmcs: map[string]*mcov1alpha1.RenderedMachineConfig{
			"workers-old123": currentRMC,
		},
	}
	determiner := NewRebootDeterminer(fetcher)

	decision := determiner.DetermineReboot(context.Background(), "workers-old123", newRMC)

	if decision.Required {
		t.Errorf("Expected Required=false, got true with reasons: %v", decision.Reasons)
	}
	if decision.Method != MethodDiffBased {
		t.Errorf("Expected Method=%s, got %s", MethodDiffBased, decision.Method)
	}
}

// TestDetermineReboot_ModifyRebootFile verifies reboot when modifying reboot-requiring file.
func TestDetermineReboot_ModifyRebootFile(t *testing.T) {
	currentRMC := makeRMC("workers-old123",
		[]mcov1alpha1.FileSpec{
			{Path: "/etc/sysctl.conf", Content: "old", Mode: 0644, Owner: "root:root", State: "present"},
		},
		nil, true,
		map[string]bool{"/etc/sysctl.conf": true},
		nil)

	newRMC := makeRMC("workers-new123",
		[]mcov1alpha1.FileSpec{
			{Path: "/etc/sysctl.conf", Content: "new", Mode: 0644, Owner: "root:root", State: "present"},
		},
		nil, true,
		map[string]bool{"/etc/sysctl.conf": true},
		nil)

	fetcher := &mockRMCFetcher{
		rmcs: map[string]*mcov1alpha1.RenderedMachineConfig{
			"workers-old123": currentRMC,
		},
	}
	determiner := NewRebootDeterminer(fetcher)

	decision := determiner.DetermineReboot(context.Background(), "workers-old123", newRMC)

	if !decision.Required {
		t.Error("Expected Required=true")
	}
	if decision.Method != MethodDiffBased {
		t.Errorf("Expected Method=%s, got %s", MethodDiffBased, decision.Method)
	}
	if len(decision.Reasons) != 1 {
		t.Errorf("Expected 1 reason, got %d: %v", len(decision.Reasons), decision.Reasons)
	}
}

// TestDetermineReboot_RemoveRebootFile verifies reboot when removing reboot-requiring file.
func TestDetermineReboot_RemoveRebootFile(t *testing.T) {
	currentRMC := makeRMC("workers-old123",
		[]mcov1alpha1.FileSpec{
			{Path: "/etc/old.conf", Content: "old", Mode: 0644, Owner: "root:root", State: "present"},
		},
		nil, true,
		map[string]bool{"/etc/old.conf": true},
		nil)

	newRMC := makeRMC("workers-new123",
		[]mcov1alpha1.FileSpec{}, // File removed
		nil, true,
		map[string]bool{}, // Empty - file doesn't exist in new
		nil)

	fetcher := &mockRMCFetcher{
		rmcs: map[string]*mcov1alpha1.RenderedMachineConfig{
			"workers-old123": currentRMC,
		},
	}
	determiner := NewRebootDeterminer(fetcher)

	decision := determiner.DetermineReboot(context.Background(), "workers-old123", newRMC)

	if !decision.Required {
		t.Error("Expected Required=true for removed reboot-requiring file")
	}
	if decision.Method != MethodDiffBased {
		t.Errorf("Expected Method=%s, got %s", MethodDiffBased, decision.Method)
	}
	// Should mention the removed file
	found := false
	for _, r := range decision.Reasons {
		if r == "file /etc/old.conf (removed) requires reboot" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected reason about removed file, got %v", decision.Reasons)
	}
}

// TestDetermineReboot_ModifyNonRebootFile verifies no reboot when modifying non-reboot file.
func TestDetermineReboot_ModifyNonRebootFile(t *testing.T) {
	currentRMC := makeRMC("workers-old123",
		[]mcov1alpha1.FileSpec{
			{Path: "/etc/app.conf", Content: "v1", Mode: 0644, Owner: "root:root", State: "present"},
		},
		nil, true,
		map[string]bool{"/etc/app.conf": false},
		nil)

	newRMC := makeRMC("workers-new123",
		[]mcov1alpha1.FileSpec{
			{Path: "/etc/app.conf", Content: "v2", Mode: 0644, Owner: "root:root", State: "present"},
		},
		nil, true,
		map[string]bool{"/etc/app.conf": false},
		nil)

	fetcher := &mockRMCFetcher{
		rmcs: map[string]*mcov1alpha1.RenderedMachineConfig{
			"workers-old123": currentRMC,
		},
	}
	determiner := NewRebootDeterminer(fetcher)

	decision := determiner.DetermineReboot(context.Background(), "workers-old123", newRMC)

	if decision.Required {
		t.Errorf("Expected Required=false, got true with reasons: %v", decision.Reasons)
	}
	if decision.Method != MethodDiffBased {
		t.Errorf("Expected Method=%s, got %s", MethodDiffBased, decision.Method)
	}
}

// TestDetermineReboot_AddRebootUnit verifies reboot when adding reboot-requiring unit.
func TestDetermineReboot_AddRebootUnit(t *testing.T) {
	currentRMC := makeRMC("workers-old123",
		nil,
		[]mcov1alpha1.UnitSpec{},
		true,
		nil,
		map[string]bool{})

	newRMC := makeRMC("workers-new123",
		nil,
		[]mcov1alpha1.UnitSpec{
			{Name: "kernel-tuning.service", Enabled: boolPtr(true), State: "started"},
		},
		true,
		nil,
		map[string]bool{"kernel-tuning.service": true})

	fetcher := &mockRMCFetcher{
		rmcs: map[string]*mcov1alpha1.RenderedMachineConfig{
			"workers-old123": currentRMC,
		},
	}
	determiner := NewRebootDeterminer(fetcher)

	decision := determiner.DetermineReboot(context.Background(), "workers-old123", newRMC)

	if !decision.Required {
		t.Error("Expected Required=true for added reboot-requiring unit")
	}
	if decision.Method != MethodDiffBased {
		t.Errorf("Expected Method=%s, got %s", MethodDiffBased, decision.Method)
	}
}

// TestDetermineReboot_ModifyRebootUnit verifies reboot when modifying reboot-requiring unit.
func TestDetermineReboot_ModifyRebootUnit(t *testing.T) {
	currentRMC := makeRMC("workers-old123",
		nil,
		[]mcov1alpha1.UnitSpec{
			{Name: "kernel.service", Enabled: boolPtr(true), State: "started"},
		},
		true,
		nil,
		map[string]bool{"kernel.service": true})

	newRMC := makeRMC("workers-new123",
		nil,
		[]mcov1alpha1.UnitSpec{
			{Name: "kernel.service", Enabled: boolPtr(false), State: "stopped"}, // Changed
		},
		true,
		nil,
		map[string]bool{"kernel.service": true})

	fetcher := &mockRMCFetcher{
		rmcs: map[string]*mcov1alpha1.RenderedMachineConfig{
			"workers-old123": currentRMC,
		},
	}
	determiner := NewRebootDeterminer(fetcher)

	decision := determiner.DetermineReboot(context.Background(), "workers-old123", newRMC)

	if !decision.Required {
		t.Error("Expected Required=true for modified reboot-requiring unit")
	}
	if decision.Method != MethodDiffBased {
		t.Errorf("Expected Method=%s, got %s", MethodDiffBased, decision.Method)
	}
}

// TestDetermineReboot_RemoveRebootUnit verifies reboot when removing reboot-requiring unit.
func TestDetermineReboot_RemoveRebootUnit(t *testing.T) {
	currentRMC := makeRMC("workers-old123",
		nil,
		[]mcov1alpha1.UnitSpec{
			{Name: "old.service", Enabled: boolPtr(true), State: "started"},
		},
		true,
		nil,
		map[string]bool{"old.service": true})

	newRMC := makeRMC("workers-new123",
		nil,
		[]mcov1alpha1.UnitSpec{}, // Unit removed
		true,
		nil,
		map[string]bool{})

	fetcher := &mockRMCFetcher{
		rmcs: map[string]*mcov1alpha1.RenderedMachineConfig{
			"workers-old123": currentRMC,
		},
	}
	determiner := NewRebootDeterminer(fetcher)

	decision := determiner.DetermineReboot(context.Background(), "workers-old123", newRMC)

	if !decision.Required {
		t.Error("Expected Required=true for removed reboot-requiring unit")
	}
	if decision.Method != MethodDiffBased {
		t.Errorf("Expected Method=%s, got %s", MethodDiffBased, decision.Method)
	}
}

// TestDetermineReboot_MixedChanges verifies multiple changes with mixed reboot requirements.
func TestDetermineReboot_MixedChanges(t *testing.T) {
	currentRMC := makeRMC("workers-old123",
		[]mcov1alpha1.FileSpec{
			{Path: "/etc/a.conf", Content: "a", Mode: 0644, Owner: "root:root", State: "present"},
			{Path: "/etc/b.conf", Content: "b", Mode: 0644, Owner: "root:root", State: "present"},
		},
		[]mcov1alpha1.UnitSpec{
			{Name: "x.service", Enabled: boolPtr(true), State: "started"},
		},
		true,
		map[string]bool{"/etc/a.conf": false, "/etc/b.conf": true},
		map[string]bool{"x.service": false})

	newRMC := makeRMC("workers-new123",
		[]mcov1alpha1.FileSpec{
			{Path: "/etc/a.conf", Content: "a-modified", Mode: 0644, Owner: "root:root", State: "present"}, // Modified, no reboot
			{Path: "/etc/c.conf", Content: "c", Mode: 0644, Owner: "root:root", State: "present"},          // Added, no reboot
		},
		[]mcov1alpha1.UnitSpec{
			{Name: "y.service", Enabled: boolPtr(true), State: "started"}, // Added, requires reboot
		},
		true,
		map[string]bool{"/etc/a.conf": false, "/etc/c.conf": false},
		map[string]bool{"y.service": true})
	// b.conf removed (requires reboot from current), x.service removed (no reboot)

	fetcher := &mockRMCFetcher{
		rmcs: map[string]*mcov1alpha1.RenderedMachineConfig{
			"workers-old123": currentRMC,
		},
	}
	determiner := NewRebootDeterminer(fetcher)

	decision := determiner.DetermineReboot(context.Background(), "workers-old123", newRMC)

	if !decision.Required {
		t.Error("Expected Required=true for mixed changes")
	}
	if decision.Method != MethodDiffBased {
		t.Errorf("Expected Method=%s, got %s", MethodDiffBased, decision.Method)
	}
	// Should have 2 reasons: b.conf removed, y.service added
	if len(decision.Reasons) != 2 {
		t.Errorf("Expected 2 reasons, got %d: %v", len(decision.Reasons), decision.Reasons)
	}
}

// TestDetermineReboot_NoChanges verifies no reboot when configs are identical.
func TestDetermineReboot_NoChanges(t *testing.T) {
	// Same config, different names (shouldn't happen in practice but tests the diff logic)
	files := []mcov1alpha1.FileSpec{
		{Path: "/etc/a.conf", Content: "a", Mode: 0644, Owner: "root:root", State: "present"},
	}
	units := []mcov1alpha1.UnitSpec{
		{Name: "x.service", Enabled: boolPtr(true), State: "started"},
	}
	fileReqs := map[string]bool{"/etc/a.conf": true}
	unitReqs := map[string]bool{"x.service": true}

	currentRMC := makeRMC("workers-old123", files, units, true, fileReqs, unitReqs)
	newRMC := makeRMC("workers-new123", files, units, true, fileReqs, unitReqs)

	fetcher := &mockRMCFetcher{
		rmcs: map[string]*mcov1alpha1.RenderedMachineConfig{
			"workers-old123": currentRMC,
		},
	}
	determiner := NewRebootDeterminer(fetcher)

	decision := determiner.DetermineReboot(context.Background(), "workers-old123", newRMC)

	if decision.Required {
		t.Errorf("Expected Required=false for identical configs, got true with reasons: %v", decision.Reasons)
	}
	if decision.Method != MethodDiffBased {
		t.Errorf("Expected Method=%s, got %s", MethodDiffBased, decision.Method)
	}
}

// TestHasRebootRequirements verifies the helper function.
func TestHasRebootRequirements(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]bool
		units    map[string]bool
		expected bool
	}{
		{"both nil", nil, nil, false},
		{"both empty", map[string]bool{}, map[string]bool{}, false},
		{"files only", map[string]bool{"/etc/a": true}, nil, true},
		{"units only", nil, map[string]bool{"a.service": true}, true},
		{"both populated", map[string]bool{"/etc/a": true}, map[string]bool{"a.service": true}, true},
		{"files empty units populated", map[string]bool{}, map[string]bool{"a.service": false}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rmc := &mcov1alpha1.RenderedMachineConfig{
				Spec: mcov1alpha1.RenderedMachineConfigSpec{
					RebootRequirements: mcov1alpha1.RebootRequirements{
						Files: tt.files,
						Units: tt.units,
					},
				},
			}
			got := hasRebootRequirements(rmc)
			if got != tt.expected {
				t.Errorf("hasRebootRequirements() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestDiffBasedReboot_EmptyConfigs verifies diff with empty configs.
func TestDiffBasedReboot_EmptyConfigs(t *testing.T) {
	currentRMC := makeRMC("workers-old123", nil, nil, false, map[string]bool{}, map[string]bool{})
	newRMC := makeRMC("workers-new123", nil, nil, false, map[string]bool{}, map[string]bool{})

	decision := diffBasedReboot(currentRMC, newRMC)

	if decision.Required {
		t.Error("Expected Required=false for empty configs")
	}
	if decision.Method != MethodDiffBased {
		t.Errorf("Expected Method=%s, got %s", MethodDiffBased, decision.Method)
	}
	if len(decision.Reasons) != 0 {
		t.Errorf("Expected no reasons, got %v", decision.Reasons)
	}
}

// TestDetermineReboot_LegacyFallbackWithNoReboot verifies fallback with legacy=false.
func TestDetermineReboot_LegacyFallbackWithNoReboot(t *testing.T) {
	fetcher := &mockRMCFetcher{
		err: errors.New("not found"),
	}
	determiner := NewRebootDeterminer(fetcher)

	newRMC := makeRMC("workers-new123", nil, nil, false, nil, nil)

	decision := determiner.DetermineReboot(context.Background(), "workers-old123", newRMC)

	if decision.Required {
		t.Error("Expected Required=false when legacy=false")
	}
	if decision.Method != MethodLegacyFallback {
		t.Errorf("Expected Method=%s, got %s", MethodLegacyFallback, decision.Method)
	}
}
