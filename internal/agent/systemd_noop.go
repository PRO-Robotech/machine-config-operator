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

	ctrl "sigs.k8s.io/controller-runtime"
)

var noopLog = ctrl.Log.WithName("systemd-noop")

// NoOpSystemdConnection is a no-op implementation of SystemdConnection
// for environments without systemd (e.g., minikube, Docker Desktop).
// All operations are logged but do nothing.
type NoOpSystemdConnection struct{}

// NewNoOpSystemdConnection creates a new no-op systemd connection.
func NewNoOpSystemdConnection() *NoOpSystemdConnection {
	noopLog.Info("using no-op systemd connection (systemd operations will be skipped)")
	return &NoOpSystemdConnection{}
}

func (n *NoOpSystemdConnection) Close() {}

func (n *NoOpSystemdConnection) GetUnitProperty(_ context.Context, unit, property string) (interface{}, error) {
	noopLog.V(1).Info("no-op: GetUnitProperty", "unit", unit, "property", property)
	// Return "inactive" for ActiveState to avoid triggering actions
	if property == "ActiveState" {
		return "inactive", nil
	}
	return "", nil
}

func (n *NoOpSystemdConnection) MaskUnit(_ context.Context, name string) error {
	noopLog.Info("no-op: MaskUnit", "unit", name)
	return nil
}

func (n *NoOpSystemdConnection) UnmaskUnit(_ context.Context, name string) error {
	noopLog.Info("no-op: UnmaskUnit", "unit", name)
	return nil
}

func (n *NoOpSystemdConnection) EnableUnit(_ context.Context, name string) error {
	noopLog.Info("no-op: EnableUnit", "unit", name)
	return nil
}

func (n *NoOpSystemdConnection) DisableUnit(_ context.Context, name string) error {
	noopLog.Info("no-op: DisableUnit", "unit", name)
	return nil
}

func (n *NoOpSystemdConnection) StartUnit(_ context.Context, name string) error {
	noopLog.Info("no-op: StartUnit", "unit", name)
	return nil
}

func (n *NoOpSystemdConnection) StopUnit(_ context.Context, name string) error {
	noopLog.Info("no-op: StopUnit", "unit", name)
	return nil
}

func (n *NoOpSystemdConnection) RestartUnit(_ context.Context, name string) error {
	noopLog.Info("no-op: RestartUnit", "unit", name)
	return nil
}

func (n *NoOpSystemdConnection) ReloadUnit(_ context.Context, name string) error {
	noopLog.Info("no-op: ReloadUnit", "unit", name)
	return nil
}
