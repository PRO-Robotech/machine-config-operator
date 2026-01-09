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
	"context"
	"os/exec"
	"syscall"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

// SystemdExecutor executes reboot via systemctl.
type SystemdExecutor struct {
	hostRoot string
}

// NewSystemdExecutor creates a new systemd-based reboot executor.
func NewSystemdExecutor(hostRoot string) *SystemdExecutor {
	return &SystemdExecutor{hostRoot: hostRoot}
}

// Execute performs the system reboot.
// It syncs filesystems and then calls systemctl reboot via chroot.
func (e *SystemdExecutor) Execute(ctx context.Context) error {
	logger := log.FromContext(ctx)

	// Sync filesystems before reboot
	logger.Info("syncing filesystems")
	_ = syscall.Sync()

	// Execute reboot via chroot
	logger.Info("executing systemctl reboot")
	cmd := exec.CommandContext(ctx, "chroot", e.hostRoot, "systemctl", "reboot")
	return cmd.Run()
}

// NoOpExecutor is a reboot executor that does nothing.
// Useful for testing and dry-run scenarios.
type NoOpExecutor struct {
	Called bool
}

// Execute records that it was called but doesn't reboot.
func (e *NoOpExecutor) Execute(ctx context.Context) error {
	e.Called = true
	log.FromContext(ctx).Info("NoOpExecutor: reboot would be executed here")
	return nil
}
