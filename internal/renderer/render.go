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
	"context"
	"errors"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// ErrHashCollision is returned when two different configs produce the same short hash.
var ErrHashCollision = errors.New("hash collision detected")

// RenderResult contains the output of a render operation.
type RenderResult struct {
	// RMC is the RenderedMachineConfig object to create or use.
	RMC *mcov1alpha1.RenderedMachineConfig

	// Collision indicates whether a hash collision was detected.
	// If true, the RMC name has been modified with a suffix.
	Collision bool

	// Existing indicates whether an identical RMC already exists.
	// If true, no creation is needed.
	Existing bool
}

// BuildRMC creates a RenderedMachineConfig from merged configuration.
// This is a pure function that builds the RMC object without any API calls.
//
// Parameters:
//   - poolName: Name of the MachineConfigPool
//   - merged: Result from Merge() function
//   - pool: MachineConfigPool for reboot settings (can be nil for defaults)
//
// Returns a fully populated RenderedMachineConfig ready for creation.
func BuildRMC(poolName string, merged *MergedConfig, pool *mcov1alpha1.MachineConfigPool) *mcov1alpha1.RenderedMachineConfig {
	hash := ComputeHash(merged)

	config := mcov1alpha1.RenderedConfig{
		Files: merged.Files,
		Systemd: mcov1alpha1.SystemdSpec{
			Units: merged.Units,
		},
	}

	sources := make([]mcov1alpha1.ConfigSource, len(merged.Sources))
	for i, s := range merged.Sources {
		sources[i] = mcov1alpha1.ConfigSource{
			Name:     s.Name,
			Priority: s.Priority,
		}
	}

	var rebootSpec mcov1alpha1.RenderedRebootSpec
	if pool != nil {
		rebootSpec = mcov1alpha1.RenderedRebootSpec{
			Required:           merged.RebootRequired,
			Strategy:           pool.Spec.Reboot.Strategy,
			MinIntervalSeconds: pool.Spec.Reboot.MinIntervalSeconds,
		}
	} else {
		rebootSpec = mcov1alpha1.RenderedRebootSpec{
			Required:           merged.RebootRequired,
			Strategy:           "Never",
			MinIntervalSeconds: 1800,
		}
	}

	configHash := strings.TrimPrefix(hash.Full, "sha256:")

	rebootRequirements := mcov1alpha1.RebootRequirements{
		Files: merged.FileRebootRequirements,
		Units: merged.UnitRebootRequirements,
	}
	// Ensure maps are not nil (use empty maps for consistency)
	if rebootRequirements.Files == nil {
		rebootRequirements.Files = map[string]bool{}
	}
	if rebootRequirements.Units == nil {
		rebootRequirements.Units = map[string]bool{}
	}

	return &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: RMCName(poolName, hash),
			Labels: map[string]string{
				"mco.in-cloud.io/pool": poolName,
			},
		},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			PoolName:           poolName,
			Revision:           hash.Short,
			ConfigHash:         configHash,
			Config:             config,
			Sources:            sources,
			RebootRequirements: rebootRequirements,
			Reboot:             rebootSpec,
		},
	}
}

// CheckExistingRMC checks if an RMC with the same name already exists.
// Returns:
//   - existing RMC if found with matching hash (no action needed)
//   - nil if not found (should create)
//   - error if found with different hash (collision)
func CheckExistingRMC(ctx context.Context, c client.Client, newRMC *mcov1alpha1.RenderedMachineConfig) (*mcov1alpha1.RenderedMachineConfig, error) {
	existing := &mcov1alpha1.RenderedMachineConfig{}
	err := c.Get(ctx, client.ObjectKey{Name: newRMC.Name}, existing)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return nil, fmt.Errorf("checking existing RMC: %w", err)
		}
		return nil, nil
	}

	if existing.Spec.ConfigHash == newRMC.Spec.ConfigHash {
		return existing, nil
	}

	return existing, fmt.Errorf("%w: name=%s existing_hash=%s new_hash=%s",
		ErrHashCollision,
		newRMC.Name,
		existing.Spec.ConfigHash,
		newRMC.Spec.ConfigHash,
	)
}

// HandleCollision modifies the RMC name to resolve a collision.
// It appends a numeric suffix to the name.
func HandleCollision(rmc *mcov1alpha1.RenderedMachineConfig, suffix int) *mcov1alpha1.RenderedMachineConfig {
	rmc.Name = fmt.Sprintf("%s-%d", rmc.Name, suffix)
	return rmc
}

func Render(ctx context.Context, c client.Client, poolName string, configs []*mcov1alpha1.MachineConfig, pool *mcov1alpha1.MachineConfigPool) (*RenderResult, error) {
	if poolName == "" {
		return nil, errors.New("poolName is required")
	}

	merged := Merge(configs)
	if err := validateMerged(merged); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}
	rmc := BuildRMC(poolName, merged, pool)
	existing, err := CheckExistingRMC(ctx, c, rmc)
	if err != nil {
		if errors.Is(err, ErrHashCollision) {
			rmc = HandleCollision(rmc, 1)
			return &RenderResult{
				RMC:       rmc,
				Collision: true,
				Existing:  false,
			}, nil
		}
		return nil, err
	}

	if existing != nil {
		return &RenderResult{
			RMC:      existing,
			Existing: true,
		}, nil
	}

	return &RenderResult{
		RMC:      rmc,
		Existing: false,
	}, nil
}

func validateMerged(merged *MergedConfig) error {
	if merged == nil {
		return errors.New("merged config is nil")
	}

	for i, f := range merged.Files {
		if err := ValidateFileSpec(f); err != nil {
			return fmt.Errorf("file[%d]: %w", i, err)
		}
	}

	for i, u := range merged.Units {
		if err := ValidateUnitSpec(u); err != nil {
			return fmt.Errorf("unit[%d]: %w", i, err)
		}
	}

	return nil
}
