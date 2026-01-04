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

// Package reboot implements reboot decision logic for the MCO agent.
//
// The reboot handler orchestrates reboot decisions based on:
//   - RMC reboot requirements (required field)
//   - Reboot strategy (Never, IfRequired)
//   - Minimum interval between reboots
//   - Force-reboot annotation
//
// Example usage:
//
//	handler := reboot.NewHandler(hostRoot, nodeWriter, executor)
//	err := handler.HandleReboot(ctx, rmc, node)
package reboot

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/pkg/annotations"
)

// NodeAnnotationWriter provides methods to update node annotations.
// This interface matches the methods from the agent's NodeWriter.
type NodeAnnotationWriter interface {
	SetState(ctx context.Context, state string) error
	SetRebootPending(ctx context.Context, pending bool) error
	SetCurrentRevision(ctx context.Context, revision string) error
	ClearForceReboot(ctx context.Context) error
}

// RebootExecutor executes the actual system reboot.
// This interface enables testing without triggering real reboots.
type RebootExecutor interface {
	Execute(ctx context.Context) error
}

// Handler handles reboot decision logic.
type Handler struct {
	hostRoot string
	writer   NodeAnnotationWriter
	executor RebootExecutor
	state    *StateManager
}

// NewHandler creates a new reboot handler.
func NewHandler(hostRoot string, writer NodeAnnotationWriter, executor RebootExecutor) *Handler {
	return &Handler{
		hostRoot: hostRoot,
		writer:   writer,
		executor: executor,
		state:    NewStateManager(hostRoot),
	}
}

// HandleReboot processes reboot requirements after a successful apply.
// It checks if reboot is required and handles it according to the strategy.
//
// Returns nil if:
//   - Reboot is not required
//   - Reboot is pending (strategy=Never or interval not elapsed)
//   - Reboot is triggered (executor returns nil)
//
// Returns error if annotation update or reboot execution fails.
func (h *Handler) HandleReboot(ctx context.Context, rmc *mcov1alpha1.RenderedMachineConfig, node *corev1.Node) error {
	logger := log.FromContext(ctx)

	// Check if reboot is required
	if !rmc.Spec.Reboot.Required {
		logger.V(1).Info("reboot not required")
		return nil
	}

	logger.Info("reboot required, checking policy")

	// Check force-reboot annotation (bypasses strategy and interval)
	if annotations.GetBoolAnnotation(node.Annotations, annotations.ForceReboot) {
		logger.Info("force-reboot annotation set, proceeding with reboot")
		return h.executeReboot(ctx)
	}

	// Get strategy (default to Never)
	strategy := rmc.Spec.Reboot.Strategy
	if strategy == "" {
		strategy = "Never"
	}

	switch strategy {
	case "Never":
		logger.Info("reboot strategy is Never, setting pending")
		return h.setPending(ctx)

	case "IfRequired":
		return h.handleIfRequired(ctx, rmc.Spec.Reboot.MinIntervalSeconds)

	default:
		logger.Info("unknown reboot strategy, treating as Never", "strategy", strategy)
		return h.setPending(ctx)
	}
}

// handleIfRequired handles the IfRequired strategy.
// It checks the minimum interval and either reboots or sets pending.
func (h *Handler) handleIfRequired(ctx context.Context, minIntervalSeconds int) error {
	logger := log.FromContext(ctx)

	// Read last reboot time
	lastReboot, err := h.state.ReadLastRebootTime()
	if err != nil {
		// No last reboot time - first boot, proceed with reboot
		logger.V(1).Info("no last reboot time found, proceeding with reboot")
		return h.executeReboot(ctx)
	}

	// Check if minInterval is 0 (disabled)
	if minIntervalSeconds <= 0 {
		logger.V(1).Info("minInterval is 0, proceeding with reboot")
		return h.executeReboot(ctx)
	}

	// Check interval
	elapsed := time.Since(lastReboot)
	required := time.Duration(minIntervalSeconds) * time.Second

	if elapsed < required {
		remaining := required - elapsed
		logger.Info("min interval not elapsed, setting pending",
			"elapsed", elapsed.Round(time.Second),
			"required", required,
			"remaining", remaining.Round(time.Second))
		return h.setPending(ctx)
	}

	logger.Info("min interval elapsed, proceeding with reboot",
		"elapsed", elapsed.Round(time.Second),
		"required", required)
	return h.executeReboot(ctx)
}

// setPending sets the reboot-pending annotation.
func (h *Handler) setPending(ctx context.Context) error {
	return h.writer.SetRebootPending(ctx, true)
}

// executeReboot executes the reboot sequence.
func (h *Handler) executeReboot(ctx context.Context) error {
	logger := log.FromContext(ctx)

	// Write last reboot time (before reboot, as we may not return)
	if err := h.state.WriteLastRebootTime(time.Now()); err != nil {
		logger.Error(err, "failed to write last reboot time, continuing anyway")
		// Continue with reboot despite this error
	}

	// Set state to rebooting
	if err := h.writer.SetState(ctx, "rebooting"); err != nil {
		logger.Error(err, "failed to set state to rebooting")
		// Continue with reboot despite this error
	}

	// Clear force-reboot annotation
	if err := h.writer.ClearForceReboot(ctx); err != nil {
		logger.Error(err, "failed to clear force-reboot annotation")
		// Continue with reboot despite this error
	}

	// Clear reboot-pending annotation
	if err := h.writer.SetRebootPending(ctx, false); err != nil {
		logger.Error(err, "failed to clear reboot-pending annotation")
		// Continue with reboot despite this error
	}

	// Execute the reboot
	logger.Info("executing reboot")
	return h.executor.Execute(ctx)
}

// CheckRebootPendingOnStartup clears reboot-pending if reboot actually occurred.
// This should be called once at agent startup.
//
// Detection logic uses a boot marker file in /run (tmpfs):
// - Before reboot: agent writes last-reboot timestamp and triggers reboot
// - After reboot: /run is cleared (tmpfs), boot marker is gone
// - On startup: if last-reboot exists but boot marker doesn't → reboot happened
//
// This works correctly in containers/VMs where /proc/uptime reflects host time.
func (h *Handler) CheckRebootPendingOnStartup(ctx context.Context, node *corev1.Node) error {
	logger := log.FromContext(ctx)

	// Always write boot marker at end (deferred)
	defer func() {
		if err := h.state.WriteBootMarker(); err != nil {
			logger.Error(err, "failed to write boot marker")
		}
	}()

	// If reboot-pending is not set, nothing to do
	if !annotations.GetBoolAnnotation(node.Annotations, annotations.RebootPending) {
		return nil
	}

	// Check if we have a recorded reboot request
	_, err := h.state.ReadLastRebootTime()
	if err != nil {
		// No recorded time, might be first boot - nothing to clear
		logger.V(1).Info("no last reboot time recorded, skipping startup check")
		return nil
	}

	// Check boot marker - if it doesn't exist, system rebooted
	if !h.state.BootMarkerExists() {
		logger.Info("detected reboot occurred (boot marker missing), clearing pending")
		// Clear reboot-pending since reboot completed
		if err := h.writer.SetRebootPending(ctx, false); err != nil {
			return err
		}
		// Also update current-revision to desired-revision to indicate reboot completed
		// This prevents the agent from re-evaluating the reboot policy for the same config
		desiredRevision := annotations.GetAnnotation(node.Annotations, annotations.DesiredRevision)
		if desiredRevision != "" {
			logger.Info("updating current-revision after reboot", "revision", desiredRevision)
			if err := h.writer.SetCurrentRevision(ctx, desiredRevision); err != nil {
				return err
			}
		}
		return nil
	}

	logger.V(1).Info("no reboot detected (boot marker exists)")
	return nil
}
