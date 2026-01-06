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

package v1alpha1

import (
	"encoding/json"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestRebootRequirements_Creation verifies struct creation and field access.
func TestRebootRequirements_Creation(t *testing.T) {
	rr := RebootRequirements{
		Files: map[string]bool{
			"/etc/sysctl.conf": true,
			"/etc/motd":        false,
		},
		Units: map[string]bool{
			"sshd.service": false,
		},
	}

	// Assert fields are accessible
	if rr.Files["/etc/sysctl.conf"] != true {
		t.Errorf("Files['/etc/sysctl.conf'] = %v, want true", rr.Files["/etc/sysctl.conf"])
	}
	if rr.Files["/etc/motd"] != false {
		t.Errorf("Files['/etc/motd'] = %v, want false", rr.Files["/etc/motd"])
	}
	if rr.Units["sshd.service"] != false {
		t.Errorf("Units['sshd.service'] = %v, want false", rr.Units["sshd.service"])
	}
}

// TestRebootRequirements_MarshalEmpty verifies empty struct marshals correctly.
func TestRebootRequirements_MarshalEmpty(t *testing.T) {
	rr := RebootRequirements{}

	data, err := json.Marshal(rr)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	// Empty struct should marshal to {} due to omitempty
	expected := "{}"
	if string(data) != expected {
		t.Errorf("Marshal empty = %s, want %s", string(data), expected)
	}
}

// TestRebootRequirements_MarshalPopulated verifies populated struct marshals correctly.
func TestRebootRequirements_MarshalPopulated(t *testing.T) {
	rr := RebootRequirements{
		Files: map[string]bool{"/etc/foo": true},
		Units: map[string]bool{"bar.service": false},
	}

	data, err := json.Marshal(rr)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	// Verify JSON contains expected fields
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if _, ok := result["files"]; !ok {
		t.Error("JSON missing 'files' key")
	}
	if _, ok := result["units"]; !ok {
		t.Error("JSON missing 'units' key")
	}
}

// TestRebootRequirements_UnmarshalRoundTrip verifies JSON round-trip.
func TestRebootRequirements_UnmarshalRoundTrip(t *testing.T) {
	original := RebootRequirements{
		Files: map[string]bool{
			"/etc/sysctl.d/99-custom.conf": true,
			"/etc/chrony.conf":             false,
		},
		Units: map[string]bool{
			"containerd.service": true,
			"myapp.service":      false,
		},
	}

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Unmarshal
	var restored RebootRequirements
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify
	if len(restored.Files) != len(original.Files) {
		t.Errorf("Files count mismatch: got %d, want %d", len(restored.Files), len(original.Files))
	}
	for path, expected := range original.Files {
		if restored.Files[path] != expected {
			t.Errorf("Files[%q] = %v, want %v", path, restored.Files[path], expected)
		}
	}
	if len(restored.Units) != len(original.Units) {
		t.Errorf("Units count mismatch: got %d, want %d", len(restored.Units), len(original.Units))
	}
	for name, expected := range original.Units {
		if restored.Units[name] != expected {
			t.Errorf("Units[%q] = %v, want %v", name, restored.Units[name], expected)
		}
	}
}

// TestRMC_UnmarshalWithoutRebootRequirements verifies backward compatibility.
// Old RMC YAML without rebootRequirements field should unmarshal successfully.
func TestRMC_UnmarshalWithoutRebootRequirements(t *testing.T) {
	// Old format without rebootRequirements
	oldJSON := `{
		"apiVersion": "mco.in-cloud.io/v1alpha1",
		"kind": "RenderedMachineConfig",
		"metadata": {"name": "test-rmc"},
		"spec": {
			"poolName": "workers",
			"revision": "abc123",
			"configHash": "abc123def456789012345678901234567890123456789012345678901234abcd",
			"config": {},
			"reboot": {
				"required": true,
				"strategy": "Never",
				"minIntervalSeconds": 1800
			}
		}
	}`

	var rmc RenderedMachineConfig
	if err := json.Unmarshal([]byte(oldJSON), &rmc); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify RebootRequirements is empty (nil maps)
	if rmc.Spec.RebootRequirements.Files != nil {
		t.Errorf("RebootRequirements.Files should be nil, got %v", rmc.Spec.RebootRequirements.Files)
	}
	if rmc.Spec.RebootRequirements.Units != nil {
		t.Errorf("RebootRequirements.Units should be nil, got %v", rmc.Spec.RebootRequirements.Units)
	}

	// Verify legacy field still works
	if !rmc.Spec.Reboot.Required {
		t.Error("Reboot.Required should be true")
	}
}

// TestRebootRequirements_DeepCopy verifies DeepCopy creates independent copies.
func TestRebootRequirements_DeepCopy(t *testing.T) {
	original := &RebootRequirements{
		Files: map[string]bool{
			"/etc/foo": true,
			"/etc/bar": false,
		},
		Units: map[string]bool{
			"foo.service": true,
		},
	}

	copied := original.DeepCopy()

	// Verify it's a different pointer
	if copied == original {
		t.Error("DeepCopy returned same pointer")
	}

	// Modify copied
	copied.Files["/etc/foo"] = false
	copied.Files["/etc/new"] = true
	copied.Units["foo.service"] = false

	// Verify original is unchanged
	if original.Files["/etc/foo"] != true {
		t.Errorf("original.Files['/etc/foo'] was modified: got %v", original.Files["/etc/foo"])
	}
	if _, exists := original.Files["/etc/new"]; exists {
		t.Error("original.Files should not have '/etc/new'")
	}
	if original.Units["foo.service"] != true {
		t.Errorf("original.Units['foo.service'] was modified: got %v", original.Units["foo.service"])
	}
}

// TestRebootRequirements_DeepCopyInto_NilMaps verifies nil map safety.
func TestRebootRequirements_DeepCopyInto_NilMaps(t *testing.T) {
	original := &RebootRequirements{} // nil maps
	copied := &RebootRequirements{}

	// Should not panic
	original.DeepCopyInto(copied)

	if copied.Files != nil {
		t.Error("copied.Files should be nil")
	}
	if copied.Units != nil {
		t.Error("copied.Units should be nil")
	}
}

// TestRenderedMachineConfigWithRebootRequirements verifies RMC with new field.
func TestRenderedMachineConfigWithRebootRequirements(t *testing.T) {
	rmc := &RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "workers-abc123",
		},
		Spec: RenderedMachineConfigSpec{
			PoolName:   "workers",
			Revision:   "abc123",
			ConfigHash: "abc123def456789012345678901234567890123456789012345678901234abcd",
			Config: RenderedConfig{
				Files: []FileSpec{
					{Path: "/etc/sysctl.conf", Content: "vm.swappiness=10"},
					{Path: "/etc/motd", Content: "Welcome"},
				},
				Systemd: SystemdSpec{
					Units: []UnitSpec{
						{Name: "custom.service"},
					},
				},
			},
			RebootRequirements: RebootRequirements{
				Files: map[string]bool{
					"/etc/sysctl.conf": true,
					"/etc/motd":        false,
				},
				Units: map[string]bool{
					"custom.service": false,
				},
			},
			Reboot: RenderedRebootSpec{
				Required: true,
				Strategy: "IfRequired",
			},
		},
	}

	// Test DeepCopy preserves RebootRequirements
	copied := rmc.DeepCopy()

	// Modify copied
	copied.Spec.RebootRequirements.Files["/etc/sysctl.conf"] = false

	// Verify original is unchanged
	if rmc.Spec.RebootRequirements.Files["/etc/sysctl.conf"] != true {
		t.Errorf("original RebootRequirements was modified")
	}
}

// TestRebootRequirements_EmptyMapsVsNil tests behavior with empty vs nil maps.
func TestRebootRequirements_EmptyMapsVsNil(t *testing.T) {
	tests := []struct {
		name string
		rr   RebootRequirements
	}{
		{
			name: "nil maps",
			rr:   RebootRequirements{},
		},
		{
			name: "empty maps",
			rr: RebootRequirements{
				Files: map[string]bool{},
				Units: map[string]bool{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal should work for both
			data, err := json.Marshal(tt.rr)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			// Unmarshal should work
			var restored RebootRequirements
			if err := json.Unmarshal(data, &restored); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			// Lookup of non-existent key should return false
			if restored.Files["/nonexistent"] != false {
				t.Error("Lookup of non-existent file should return false")
			}
			if restored.Units["nonexistent.service"] != false {
				t.Error("Lookup of non-existent unit should return false")
			}
		})
	}
}
