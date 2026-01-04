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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// HashResult contains both short and full hash.
type HashResult struct {
	// Short is the first 10 characters of the hash, used in RMC names.
	Short string
	// Full is the complete SHA256 hash with "sha256:" prefix.
	Full string
}

// Canonical types for deterministic JSON marshaling.
// Fields are alphabetically ordered to ensure consistent JSON output.
// Only fields that affect the actual configuration are included.

// canonicalFile represents a file for hashing (alphabetical field order).
type canonicalFile struct {
	Content string `json:"content"`
	Mode    int    `json:"mode"`
	Owner   string `json:"owner"`
	Path    string `json:"path"`
	State   string `json:"state"`
}

// canonicalUnit represents a systemd unit for hashing (alphabetical field order).
type canonicalUnit struct {
	Enabled *bool  `json:"enabled,omitempty"`
	Mask    bool   `json:"mask"`
	Name    string `json:"name"`
	State   string `json:"state,omitempty"`
}

// canonicalReboot represents reboot config for hashing.
type canonicalReboot struct {
	Required bool `json:"required"`
}

// canonicalSystemd represents systemd config for hashing.
type canonicalSystemd struct {
	Units []canonicalUnit `json:"units"`
}

type canonicalConfig struct {
	Files   []canonicalFile  `json:"files"`
	Reboot  canonicalReboot  `json:"reboot"`
	Systemd canonicalSystemd `json:"systemd"`
}

// ComputeHash computes a deterministic SHA256 hash of the merged configuration.
func ComputeHash(merged *MergedConfig) HashResult {
	if merged == nil {
		merged = &MergedConfig{
			Files: []mcov1alpha1.FileSpec{},
			Units: []mcov1alpha1.UnitSpec{},
		}
	}

	files := make([]canonicalFile, len(merged.Files))
	for i, f := range merged.Files {
		files[i] = canonicalFile{
			Content: f.Content,
			Mode:    f.Mode,
			Owner:   f.Owner,
			Path:    f.Path,
			State:   f.State,
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	units := make([]canonicalUnit, len(merged.Units))
	for i, u := range merged.Units {
		units[i] = canonicalUnit{
			Enabled: u.Enabled,
			Mask:    u.Mask,
			Name:    u.Name,
			State:   u.State,
		}
	}
	sort.Slice(units, func(i, j int) bool {
		return units[i].Name < units[j].Name
	})

	canonical := canonicalConfig{
		Files: files,
		Reboot: canonicalReboot{
			Required: merged.RebootRequired,
		},
		Systemd: canonicalSystemd{
			Units: units,
		},
	}

	jsonBytes, err := json.Marshal(canonical)
	if err != nil {
		return HashResult{Short: "", Full: ""}
	}

	hash := sha256.Sum256(jsonBytes)
	fullHex := hex.EncodeToString(hash[:])

	return HashResult{
		Short: fullHex[:10],
		Full:  "sha256:" + fullHex,
	}
}

// RMCName generates a RenderedMachineConfig name from pool name and hash.
// Format: {poolName}-{shortHash}
// Example: "worker-a1b2c3d4e5"
func RMCName(poolName string, hash HashResult) string {
	return poolName + "-" + hash.Short
}

// ToCanonicalJSON returns the canonical JSON representation used for hashing.
func ToCanonicalJSON(merged *MergedConfig) ([]byte, error) {
	if merged == nil {
		merged = &MergedConfig{
			Files: []mcov1alpha1.FileSpec{},
			Units: []mcov1alpha1.UnitSpec{},
		}
	}

	files := make([]canonicalFile, len(merged.Files))
	for i, f := range merged.Files {
		files[i] = canonicalFile{
			Content: f.Content,
			Mode:    f.Mode,
			Owner:   f.Owner,
			Path:    f.Path,
			State:   f.State,
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	units := make([]canonicalUnit, len(merged.Units))
	for i, u := range merged.Units {
		units[i] = canonicalUnit{
			Enabled: u.Enabled,
			Mask:    u.Mask,
			Name:    u.Name,
			State:   u.State,
		}
	}
	sort.Slice(units, func(i, j int) bool {
		return units[i].Name < units[j].Name
	})

	canonical := canonicalConfig{
		Files: files,
		Reboot: canonicalReboot{
			Required: merged.RebootRequired,
		},
		Systemd: canonicalSystemd{
			Units: units,
		},
	}

	return json.Marshal(canonical)
}
