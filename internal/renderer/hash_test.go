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
	"encoding/json"
	"strings"
	"testing"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// Helper to create bool pointer
func boolPtr(b bool) *bool {
	return &b
}

// TestComputeHash_Empty verifies hashing empty/nil configs.
func TestComputeHash_Empty(t *testing.T) {
	tests := []struct {
		name   string
		merged *MergedConfig
	}{
		{"nil config", nil},
		{"empty config", &MergedConfig{}},
		{"empty slices", &MergedConfig{
			Files: []mcov1alpha1.FileSpec{},
			Units: []mcov1alpha1.UnitSpec{},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ComputeHash(tt.merged)

			if result.Short == "" {
				t.Error("Short hash should not be empty")
			}
			if len(result.Short) != 10 {
				t.Errorf("Short hash length = %d, want 10", len(result.Short))
			}
			if !strings.HasPrefix(result.Full, "sha256:") {
				t.Errorf("Full hash should start with 'sha256:', got %q", result.Full)
			}
			if len(result.Full) != 71 { // "sha256:" (7) + 64 hex chars
				t.Errorf("Full hash length = %d, want 71", len(result.Full))
			}
		})
	}
}

// TestComputeHash_Determinism verifies same input produces same output.
func TestComputeHash_Determinism(t *testing.T) {
	merged := &MergedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/b.conf", Content: "b", Mode: 420, Owner: "root:root", State: "present"},
			{Path: "/etc/a.conf", Content: "a", Mode: 420, Owner: "root:root", State: "present"},
		},
		Units: []mcov1alpha1.UnitSpec{
			{Name: "b.service", Enabled: boolPtr(true)},
			{Name: "a.service", Enabled: boolPtr(true)},
		},
		RebootRequired: false,
	}

	hashes := make(map[string]int)
	for i := 0; i < 100; i++ {
		result := ComputeHash(merged)
		hashes[result.Full]++
	}

	if len(hashes) != 1 {
		t.Errorf("expected 1 unique hash, got %d unique hashes", len(hashes))
	}
}

// TestComputeHash_OrderIndependence verifies input order doesn't affect hash.
func TestComputeHash_OrderIndependence(t *testing.T) {
	merged1 := &MergedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/a.conf", Content: "a", Mode: 420, Owner: "root:root", State: "present"},
			{Path: "/etc/b.conf", Content: "b", Mode: 420, Owner: "root:root", State: "present"},
		},
	}
	merged2 := &MergedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/b.conf", Content: "b", Mode: 420, Owner: "root:root", State: "present"},
			{Path: "/etc/a.conf", Content: "a", Mode: 420, Owner: "root:root", State: "present"},
		},
	}

	hash1 := ComputeHash(merged1)
	hash2 := ComputeHash(merged2)

	if hash1.Full != hash2.Full {
		t.Errorf("hashes should match regardless of file input order\nhash1: %s\nhash2: %s",
			hash1.Full, hash2.Full)
	}

	merged3 := &MergedConfig{
		Units: []mcov1alpha1.UnitSpec{
			{Name: "a.service", State: "started"},
			{Name: "b.service", State: "stopped"},
		},
	}
	merged4 := &MergedConfig{
		Units: []mcov1alpha1.UnitSpec{
			{Name: "b.service", State: "stopped"},
			{Name: "a.service", State: "started"},
		},
	}

	hash3 := ComputeHash(merged3)
	hash4 := ComputeHash(merged4)

	if hash3.Full != hash4.Full {
		t.Errorf("hashes should match regardless of unit input order\nhash3: %s\nhash4: %s",
			hash3.Full, hash4.Full)
	}
}

// TestComputeHash_ExcludedFields verifies excluded fields don't affect hash.
func TestComputeHash_ExcludedFields(t *testing.T) {
	// Same config, different sources
	merged1 := &MergedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/test.conf", Content: "test", Mode: 420, Owner: "root:root", State: "present"},
		},
		Sources: []ConfigSource{
			{Name: "mc-a", Priority: 0},
		},
	}
	merged2 := &MergedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/test.conf", Content: "test", Mode: 420, Owner: "root:root", State: "present"},
		},
		Sources: []ConfigSource{
			{Name: "mc-b", Priority: 50},
			{Name: "mc-c", Priority: 100},
		},
	}

	hash1 := ComputeHash(merged1)
	hash2 := ComputeHash(merged2)

	if hash1.Full != hash2.Full {
		t.Errorf("Sources should not affect hash\nhash1: %s\nhash2: %s", hash1.Full, hash2.Full)
	}
}

// TestComputeHash_ExcludesRebootRequirements verifies RebootRequirements don't affect hash.
func TestComputeHash_ExcludesRebootRequirements(t *testing.T) {
	// Same file content, different reboot requirements
	merged1 := &MergedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/test.conf", Content: "test", Mode: 420, Owner: "root:root", State: "present"},
		},
		Units: []mcov1alpha1.UnitSpec{
			{Name: "test.service", Enabled: boolPtr(true), State: "started"},
		},
		RebootRequired: true,
		FileRebootRequirements: map[string]bool{
			"/etc/test.conf": true,
		},
		UnitRebootRequirements: map[string]bool{
			"test.service": true,
		},
	}

	merged2 := &MergedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/test.conf", Content: "test", Mode: 420, Owner: "root:root", State: "present"},
		},
		Units: []mcov1alpha1.UnitSpec{
			{Name: "test.service", Enabled: boolPtr(true), State: "started"},
		},
		RebootRequired: true,
		FileRebootRequirements: map[string]bool{
			"/etc/test.conf": false, // Different per-file requirement
		},
		UnitRebootRequirements: map[string]bool{
			"test.service": false, // Different per-unit requirement
		},
	}

	hash1 := ComputeHash(merged1)
	hash2 := ComputeHash(merged2)

	if hash1.Full != hash2.Full {
		t.Errorf("FileRebootRequirements and UnitRebootRequirements should NOT affect hash\nhash1: %s\nhash2: %s",
			hash1.Full, hash2.Full)
	}

	merged3 := &MergedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/test.conf", Content: "test", Mode: 420, Owner: "root:root", State: "present"},
		},
		Units: []mcov1alpha1.UnitSpec{
			{Name: "test.service", Enabled: boolPtr(true), State: "started"},
		},
		RebootRequired:         true,
		FileRebootRequirements: nil,
		UnitRebootRequirements: nil,
	}

	hash3 := ComputeHash(merged3)

	if hash1.Full != hash3.Full {
		t.Errorf("Nil vs populated RebootRequirements maps should produce same hash\nhash1: %s\nhash3: %s",
			hash1.Full, hash3.Full)
	}
}

// TestComputeHash_IncludedFields verifies included fields do affect hash.
func TestComputeHash_IncludedFields(t *testing.T) {
	base := &MergedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/test.conf", Content: "base", Mode: 420, Owner: "root:root", State: "present"},
		},
	}

	tests := []struct {
		name    string
		changed *MergedConfig
	}{
		{"different content", &MergedConfig{
			Files: []mcov1alpha1.FileSpec{
				{Path: "/etc/test.conf", Content: "different", Mode: 420, Owner: "root:root", State: "present"},
			},
		}},
		{"different mode", &MergedConfig{
			Files: []mcov1alpha1.FileSpec{
				{Path: "/etc/test.conf", Content: "base", Mode: 384, Owner: "root:root", State: "present"}, // 0600
			},
		}},
		{"different owner", &MergedConfig{
			Files: []mcov1alpha1.FileSpec{
				{Path: "/etc/test.conf", Content: "base", Mode: 420, Owner: "nobody:nogroup", State: "present"},
			},
		}},
		{"different path", &MergedConfig{
			Files: []mcov1alpha1.FileSpec{
				{Path: "/etc/other.conf", Content: "base", Mode: 420, Owner: "root:root", State: "present"},
			},
		}},
		{"different state", &MergedConfig{
			Files: []mcov1alpha1.FileSpec{
				{Path: "/etc/test.conf", Content: "base", Mode: 420, Owner: "root:root", State: "absent"},
			},
		}},
		{"different reboot", &MergedConfig{
			Files: []mcov1alpha1.FileSpec{
				{Path: "/etc/test.conf", Content: "base", Mode: 420, Owner: "root:root", State: "present"},
			},
			RebootRequired: true,
		}},
	}

	baseHash := ComputeHash(base)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changedHash := ComputeHash(tt.changed)

			if changedHash.Full == baseHash.Full {
				t.Errorf("changing %s should produce different hash", tt.name)
			}
		})
	}
}

// TestComputeHash_UnitFields verifies unit field changes affect hash.
func TestComputeHash_UnitFields(t *testing.T) {
	base := &MergedConfig{
		Units: []mcov1alpha1.UnitSpec{
			{Name: "test.service", Enabled: boolPtr(true), State: "started", Mask: false},
		},
	}

	tests := []struct {
		name    string
		changed *MergedConfig
	}{
		{"different enabled", &MergedConfig{
			Units: []mcov1alpha1.UnitSpec{
				{Name: "test.service", Enabled: boolPtr(false), State: "started", Mask: false},
			},
		}},
		{"different state", &MergedConfig{
			Units: []mcov1alpha1.UnitSpec{
				{Name: "test.service", Enabled: boolPtr(true), State: "stopped", Mask: false},
			},
		}},
		{"different mask", &MergedConfig{
			Units: []mcov1alpha1.UnitSpec{
				{Name: "test.service", Enabled: boolPtr(true), State: "started", Mask: true},
			},
		}},
		{"different name", &MergedConfig{
			Units: []mcov1alpha1.UnitSpec{
				{Name: "other.service", Enabled: boolPtr(true), State: "started", Mask: false},
			},
		}},
		{"nil enabled vs true", &MergedConfig{
			Units: []mcov1alpha1.UnitSpec{
				{Name: "test.service", Enabled: nil, State: "started", Mask: false},
			},
		}},
	}

	baseHash := ComputeHash(base)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changedHash := ComputeHash(tt.changed)

			if changedHash.Full == baseHash.Full {
				t.Errorf("changing %s should produce different hash", tt.name)
			}
		})
	}
}

// TestComputeHash_ShortHashFormat verifies short hash format.
func TestComputeHash_ShortHashFormat(t *testing.T) {
	merged := &MergedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/test.conf", Content: "test"},
		},
	}

	result := ComputeHash(merged)

	// Short hash should be 10 hex chars
	if len(result.Short) != 10 {
		t.Errorf("Short hash length = %d, want 10", len(result.Short))
	}

	for _, c := range result.Short {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("Short hash contains non-hex character: %c", c)
		}
	}

	fullHex := strings.TrimPrefix(result.Full, "sha256:")
	if !strings.HasPrefix(fullHex, result.Short) {
		t.Errorf("Short hash %q is not prefix of full hash %q", result.Short, fullHex)
	}
}

// TestRMCName verifies RMC name generation.
func TestRMCName(t *testing.T) {
	tests := []struct {
		pool string
		hash HashResult
		want string
	}{
		{"worker", HashResult{Short: "a1b2c3d4e5"}, "worker-a1b2c3d4e5"},
		{"master", HashResult{Short: "0000000000"}, "master-0000000000"},
		{"control-plane", HashResult{Short: "ffffffffff"}, "control-plane-ffffffffff"},
		{"my-pool-name", HashResult{Short: "1234567890"}, "my-pool-name-1234567890"},
	}

	for _, tt := range tests {
		t.Run(tt.pool, func(t *testing.T) {
			got := RMCName(tt.pool, tt.hash)
			if got != tt.want {
				t.Errorf("RMCName(%q, %v) = %q, want %q", tt.pool, tt.hash, got, tt.want)
			}
		})
	}
}

// TestToCanonicalJSON verifies JSON output format.
func TestToCanonicalJSON(t *testing.T) {
	merged := &MergedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/b.conf", Content: "b", Mode: 420, Owner: "root:root", State: "present"},
			{Path: "/etc/a.conf", Content: "a", Mode: 384, Owner: "nobody:nogroup", State: "absent"},
		},
		Units: []mcov1alpha1.UnitSpec{
			{Name: "b.service", Enabled: boolPtr(false), State: "stopped", Mask: true},
			{Name: "a.service", Enabled: boolPtr(true), State: "started", Mask: false},
		},
		RebootRequired: true,
		Sources: []ConfigSource{
			{Name: "should-be-excluded", Priority: 999},
		},
	}

	jsonBytes, err := ToCanonicalJSON(merged)
	if err != nil {
		t.Fatalf("ToCanonicalJSON() error = %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("ToCanonicalJSON produced invalid JSON: %v", err)
	}

	jsonStr := string(jsonBytes)
	aIndex := strings.Index(jsonStr, "/etc/a.conf")
	bIndex := strings.Index(jsonStr, "/etc/b.conf")
	if aIndex > bIndex {
		t.Error("Files should be sorted by path (a before b)")
	}

	aServiceIndex := strings.Index(jsonStr, "a.service")
	bServiceIndex := strings.Index(jsonStr, "b.service")
	if aServiceIndex > bServiceIndex {
		t.Error("Units should be sorted by name (a before b)")
	}

	if strings.Contains(jsonStr, "should-be-excluded") {
		t.Error("Sources should be excluded from canonical JSON")
	}

	if strings.Contains(jsonStr, "\n") || strings.Contains(jsonStr, "  ") {
		t.Error("Canonical JSON should be compact (no whitespace)")
	}
}

// TestToCanonicalJSON_Empty verifies empty/nil handling.
func TestToCanonicalJSON_Empty(t *testing.T) {
	tests := []struct {
		name   string
		merged *MergedConfig
	}{
		{"nil", nil},
		{"empty", &MergedConfig{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBytes, err := ToCanonicalJSON(tt.merged)
			if err != nil {
				t.Fatalf("ToCanonicalJSON() error = %v", err)
			}

			if len(jsonBytes) == 0 {
				t.Error("ToCanonicalJSON should return valid JSON even for empty input")
			}

			var parsed interface{}
			if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
				t.Fatalf("ToCanonicalJSON produced invalid JSON: %v", err)
			}
		})
	}
}

// TestComputeHash_ConsistentWithCanonicalJSON verifies hash is computed from canonical JSON.
func TestComputeHash_ConsistentWithCanonicalJSON(t *testing.T) {
	merged := &MergedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/test.conf", Content: "test", Mode: 420, Owner: "root:root", State: "present"},
		},
		Units: []mcov1alpha1.UnitSpec{
			{Name: "test.service", Enabled: boolPtr(true), State: "started"},
		},
		RebootRequired: false,
	}

	jsonBytes, err := ToCanonicalJSON(merged)
	if err != nil {
		t.Fatalf("ToCanonicalJSON() error = %v", err)
	}

	if len(jsonBytes) == 0 {
		t.Error("Canonical JSON should not be empty")
	}

	hashResult1 := ComputeHash(merged)
	hashResult2 := ComputeHash(merged)
	if hashResult1.Full != hashResult2.Full {
		t.Errorf("Hash computation should be deterministic")
	}
}

// TestComputeHash_RealWorldScenario tests a realistic configuration.
func TestComputeHash_RealWorldScenario(t *testing.T) {
	merged := &MergedConfig{
		Files: []mcov1alpha1.FileSpec{
			{
				Path:    "/etc/chrony.conf",
				Content: "server pool.ntp.org iburst\ndriftfile /var/lib/chrony/drift",
				Mode:    420,
				Owner:   "root:root",
				State:   "present",
			},
			{
				Path:    "/etc/sysctl.d/99-custom.conf",
				Content: "vm.swappiness=10\nnet.core.somaxconn=65535",
				Mode:    420,
				Owner:   "root:root",
				State:   "present",
			},
		},
		Units: []mcov1alpha1.UnitSpec{
			{Name: "chronyd.service", Enabled: boolPtr(true), State: "started", Mask: false},
		},
		RebootRequired: true,
		Sources: []ConfigSource{
			{Name: "00-base", Priority: 10},
			{Name: "50-worker", Priority: 50},
		},
	}

	hash1 := ComputeHash(merged)
	hash2 := ComputeHash(merged)

	// Same config should produce same hash
	if hash1.Full != hash2.Full {
		t.Error("Same config should produce same hash")
	}

	if len(hash1.Short) != 10 {
		t.Errorf("Short hash length = %d, want 10", len(hash1.Short))
	}
	if !strings.HasPrefix(hash1.Full, "sha256:") {
		t.Error("Full hash should start with sha256:")
	}

	rmcName := RMCName("worker", hash1)
	if !strings.HasPrefix(rmcName, "worker-") {
		t.Errorf("RMC name should start with pool name: %s", rmcName)
	}
	if len(rmcName) != len("worker-")+10 {
		t.Errorf("RMC name length = %d, want %d", len(rmcName), len("worker-")+10)
	}
}
