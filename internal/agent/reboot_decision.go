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

// RebootDecision contains the result of reboot determination.
type RebootDecision struct {
	// Required is true if reboot is needed.
	Required bool

	// Reasons lists why reboot is required (empty if not required).
	Reasons []string

	// Method describes how the decision was made.
	// Values: "diff-based", "legacy-first-apply", "legacy-fallback", "same-revision"
	Method string
}

// Reboot determination methods.
const (
	MethodDiffBased        = "diff-based"
	MethodLegacyFirstApply = "legacy-first-apply"
	MethodLegacyFallback   = "legacy-fallback"
	MethodSameRevision     = "same-revision"
)

// RMCFetcher is an interface for fetching RenderedMachineConfigs.
// This allows for easy testing without requiring a full client.
type RMCFetcher interface {
	FetchRMC(ctx context.Context, name string) (*mcov1alpha1.RenderedMachineConfig, error)
}

// RebootDeterminer determines if reboot is required when transitioning
// between configurations. It uses diff-based logic when possible,
// falling back to legacy OR-based logic when necessary.
type RebootDeterminer struct {
	fetcher RMCFetcher
}

// NewRebootDeterminer creates a new RebootDeterminer with the given fetcher.
func NewRebootDeterminer(fetcher RMCFetcher) *RebootDeterminer {
	return &RebootDeterminer{
		fetcher: fetcher,
	}
}

// DetermineReboot determines if reboot is needed when transitioning
// from currentRevision to newRMC.
//
// Decision logic:
//  1. First apply (currentRevision == ""): Use legacy (OR of all MCs)
//  2. Same revision: No reboot needed
//  3. Current RMC not available: Fallback to legacy
//  4. RebootRequirements not populated: Fallback to legacy
//  5. Normal transition: Use diff-based logic
func (d *RebootDeterminer) DetermineReboot(ctx context.Context, currentRevision string, newRMC *mcov1alpha1.RenderedMachineConfig) RebootDecision {
	if currentRevision == "" {
		return RebootDecision{
			Required: newRMC.Spec.Reboot.Required,
			Reasons:  []string{"first apply"},
			Method:   MethodLegacyFirstApply,
		}
	}

	if currentRevision == newRMC.Name {
		return RebootDecision{
			Required: false,
			Method:   MethodSameRevision,
		}
	}

	currentRMC, err := d.fetcher.FetchRMC(ctx, currentRevision)
	if err != nil {
		agentLog.Info("cannot fetch current RMC, using legacy reboot check",
			"currentRevision", currentRevision,
			"error", err)
		return RebootDecision{
			Required: newRMC.Spec.Reboot.Required,
			Reasons:  []string{"fallback: current RMC not available"},
			Method:   MethodLegacyFallback,
		}
	}

	if !hasRebootRequirements(newRMC) && !hasRebootRequirements(currentRMC) {
		agentLog.Info("neither RMC has RebootRequirements populated, using legacy check")
		return RebootDecision{
			Required: newRMC.Spec.Reboot.Required,
			Reasons:  []string{"fallback: RebootRequirements not populated"},
			Method:   MethodLegacyFallback,
		}
	}

	return diffBasedReboot(currentRMC, newRMC)
}

func hasRebootRequirements(rmc *mcov1alpha1.RenderedMachineConfig) bool {
	return len(rmc.Spec.RebootRequirements.Files) > 0 ||
		len(rmc.Spec.RebootRequirements.Units) > 0
}

func diffBasedReboot(current, new *mcov1alpha1.RenderedMachineConfig) RebootDecision {
	var reasons []string

	fileChanges := DiffFiles(current.Spec.Config.Files, new.Spec.Config.Files)
	for _, change := range fileChanges {
		var requiresReboot bool

		switch change.ChangeType {
		case ChangeTypeAdded, ChangeTypeModified:
			// For added/modified: check new RMC's requirements
			requiresReboot = new.Spec.RebootRequirements.Files[change.Path]
		case ChangeTypeRemoved:
			// For removed: check current RMC's requirements
			// (removing a reboot-requiring file also requires reboot)
			requiresReboot = current.Spec.RebootRequirements.Files[change.Path]
		}

		if requiresReboot {
			reasons = append(reasons, fmt.Sprintf(
				"file %s (%s) requires reboot", change.Path, change.ChangeType))
		}
	}

	unitChanges := DiffUnits(
		current.Spec.Config.Systemd.Units,
		new.Spec.Config.Systemd.Units,
	)
	for _, change := range unitChanges {
		var requiresReboot bool

		switch change.ChangeType {
		case ChangeTypeAdded, ChangeTypeModified:
			// For added/modified: check new RMC's requirements
			requiresReboot = new.Spec.RebootRequirements.Units[change.Name]
		case ChangeTypeRemoved:
			// For removed: check current RMC's requirements
			requiresReboot = current.Spec.RebootRequirements.Units[change.Name]
		}

		if requiresReboot {
			reasons = append(reasons, fmt.Sprintf(
				"unit %s (%s) requires reboot", change.Name, change.ChangeType))
		}
	}

	return RebootDecision{
		Required: len(reasons) > 0,
		Reasons:  reasons,
		Method:   MethodDiffBased,
	}
}
