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

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// MockSystemdConnection is a mock implementation for testing.
type MockSystemdConnection struct {
	Properties   map[string]map[string]interface{} // unit -> property -> value
	MaskCalls    []string
	UnmaskCalls  []string
	EnableCalls  []string
	DisableCalls []string
	StartCalls   []string
	StopCalls    []string
	RestartCalls []string
	ReloadCalls  []string
	Closed       bool
	Error        error // Error to return for all operations
}

func NewMockConnection() *MockSystemdConnection {
	return &MockSystemdConnection{
		Properties: make(map[string]map[string]interface{}),
	}
}

func (m *MockSystemdConnection) SetProperty(unit, property string, value interface{}) {
	if m.Properties[unit] == nil {
		m.Properties[unit] = make(map[string]interface{})
	}
	m.Properties[unit][property] = value
}

func (m *MockSystemdConnection) Close() {
	m.Closed = true
}

func (m *MockSystemdConnection) GetUnitProperty(ctx context.Context, unit, property string) (interface{}, error) {
	if m.Error != nil {
		return nil, m.Error
	}
	if props, ok := m.Properties[unit]; ok {
		return props[property], nil
	}
	return nil, nil
}

func (m *MockSystemdConnection) MaskUnit(ctx context.Context, name string) error {
	m.MaskCalls = append(m.MaskCalls, name)
	if m.Error != nil {
		return m.Error
	}
	m.SetProperty(name, "UnitFileState", "masked")
	return nil
}

func (m *MockSystemdConnection) UnmaskUnit(ctx context.Context, name string) error {
	m.UnmaskCalls = append(m.UnmaskCalls, name)
	if m.Error != nil {
		return m.Error
	}
	m.SetProperty(name, "UnitFileState", "disabled")
	return nil
}

func (m *MockSystemdConnection) EnableUnit(ctx context.Context, name string) error {
	m.EnableCalls = append(m.EnableCalls, name)
	if m.Error != nil {
		return m.Error
	}
	m.SetProperty(name, "UnitFileState", "enabled")
	return nil
}

func (m *MockSystemdConnection) DisableUnit(ctx context.Context, name string) error {
	m.DisableCalls = append(m.DisableCalls, name)
	if m.Error != nil {
		return m.Error
	}
	m.SetProperty(name, "UnitFileState", "disabled")
	return nil
}

func (m *MockSystemdConnection) StartUnit(ctx context.Context, name string) error {
	m.StartCalls = append(m.StartCalls, name)
	if m.Error != nil {
		return m.Error
	}
	m.SetProperty(name, "ActiveState", "active")
	return nil
}

func (m *MockSystemdConnection) StopUnit(ctx context.Context, name string) error {
	m.StopCalls = append(m.StopCalls, name)
	if m.Error != nil {
		return m.Error
	}
	m.SetProperty(name, "ActiveState", "inactive")
	return nil
}

func (m *MockSystemdConnection) RestartUnit(ctx context.Context, name string) error {
	m.RestartCalls = append(m.RestartCalls, name)
	if m.Error != nil {
		return m.Error
	}
	m.SetProperty(name, "ActiveState", "active")
	return nil
}

func (m *MockSystemdConnection) ReloadUnit(ctx context.Context, name string) error {
	m.ReloadCalls = append(m.ReloadCalls, name)
	return m.Error
}

func TestNewSystemdApplier(t *testing.T) {
	mock := NewMockConnection()
	a := NewSystemdApplier(mock)
	if a == nil {
		t.Fatal("NewSystemdApplier returned nil")
	}
}

func TestSystemdApplier_Close(t *testing.T) {
	mock := NewMockConnection()
	a := NewSystemdApplier(mock)
	a.Close()
	if !mock.Closed {
		t.Error("Close() did not close the connection")
	}
}

func TestApply_MaskUnit(t *testing.T) {
	mock := NewMockConnection()
	a := NewSystemdApplier(mock)
	ctx := context.Background()

	u := mcov1alpha1.UnitSpec{
		Name: "test.service",
		Mask: true,
	}

	result := a.Apply(ctx, u)
	if result.Error != nil {
		t.Fatalf("Apply() error = %v", result.Error)
	}
	if !result.Applied {
		t.Error("Apply() should report applied for mask")
	}
	if len(mock.MaskCalls) != 1 || mock.MaskCalls[0] != "test.service" {
		t.Errorf("MaskCalls = %v, want [test.service]", mock.MaskCalls)
	}
}

func TestApply_MaskIdempotent(t *testing.T) {
	mock := NewMockConnection()
	mock.SetProperty("test.service", "UnitFileState", "masked")
	a := NewSystemdApplier(mock)
	ctx := context.Background()

	u := mcov1alpha1.UnitSpec{
		Name: "test.service",
		Mask: true,
	}

	result := a.Apply(ctx, u)
	if result.Error != nil {
		t.Fatalf("Apply() error = %v", result.Error)
	}
	if result.Applied {
		t.Error("Apply() should not report applied for already masked")
	}
	if len(mock.MaskCalls) != 0 {
		t.Errorf("MaskCalls = %v, want empty", mock.MaskCalls)
	}
}

func TestApply_UnmaskUnit(t *testing.T) {
	mock := NewMockConnection()
	mock.SetProperty("test.service", "UnitFileState", "masked")
	a := NewSystemdApplier(mock)
	ctx := context.Background()

	u := mcov1alpha1.UnitSpec{
		Name: "test.service",
		Mask: false,
	}

	result := a.Apply(ctx, u)
	if result.Error != nil {
		t.Fatalf("Apply() error = %v", result.Error)
	}
	if !result.Applied {
		t.Error("Apply() should report applied for unmask")
	}
	if len(mock.UnmaskCalls) != 1 {
		t.Errorf("UnmaskCalls = %v, want [test.service]", mock.UnmaskCalls)
	}
}

func TestApply_EnableUnit(t *testing.T) {
	mock := NewMockConnection()
	mock.SetProperty("test.service", "UnitFileState", "disabled")
	a := NewSystemdApplier(mock)
	ctx := context.Background()

	enabled := true
	u := mcov1alpha1.UnitSpec{
		Name:    "test.service",
		Enabled: &enabled,
	}

	result := a.Apply(ctx, u)
	if result.Error != nil {
		t.Fatalf("Apply() error = %v", result.Error)
	}
	if !result.Applied {
		t.Error("Apply() should report applied for enable")
	}
	if len(mock.EnableCalls) != 1 {
		t.Errorf("EnableCalls = %v, want [test.service]", mock.EnableCalls)
	}
}

func TestApply_EnableIdempotent(t *testing.T) {
	mock := NewMockConnection()
	mock.SetProperty("test.service", "UnitFileState", "enabled")
	a := NewSystemdApplier(mock)
	ctx := context.Background()

	enabled := true
	u := mcov1alpha1.UnitSpec{
		Name:    "test.service",
		Enabled: &enabled,
	}

	result := a.Apply(ctx, u)
	if result.Error != nil {
		t.Fatalf("Apply() error = %v", result.Error)
	}
	if result.Applied {
		t.Error("Apply() should not report applied for already enabled")
	}
	if len(mock.EnableCalls) != 0 {
		t.Errorf("EnableCalls = %v, want empty", mock.EnableCalls)
	}
}

func TestApply_DisableUnit(t *testing.T) {
	mock := NewMockConnection()
	mock.SetProperty("test.service", "UnitFileState", "enabled")
	a := NewSystemdApplier(mock)
	ctx := context.Background()

	enabled := false
	u := mcov1alpha1.UnitSpec{
		Name:    "test.service",
		Enabled: &enabled,
	}

	result := a.Apply(ctx, u)
	if result.Error != nil {
		t.Fatalf("Apply() error = %v", result.Error)
	}
	if !result.Applied {
		t.Error("Apply() should report applied for disable")
	}
	if len(mock.DisableCalls) != 1 {
		t.Errorf("DisableCalls = %v, want [test.service]", mock.DisableCalls)
	}
}

func TestApply_StartUnit(t *testing.T) {
	mock := NewMockConnection()
	mock.SetProperty("test.service", "ActiveState", "inactive")
	a := NewSystemdApplier(mock)
	ctx := context.Background()

	u := mcov1alpha1.UnitSpec{
		Name:  "test.service",
		State: "started",
	}

	result := a.Apply(ctx, u)
	if result.Error != nil {
		t.Fatalf("Apply() error = %v", result.Error)
	}
	if !result.Applied {
		t.Error("Apply() should report applied for start")
	}
	if len(mock.StartCalls) != 1 {
		t.Errorf("StartCalls = %v, want [test.service]", mock.StartCalls)
	}
}

func TestApply_StartIdempotent(t *testing.T) {
	mock := NewMockConnection()
	mock.SetProperty("test.service", "ActiveState", "active")
	a := NewSystemdApplier(mock)
	ctx := context.Background()

	u := mcov1alpha1.UnitSpec{
		Name:  "test.service",
		State: "started",
	}

	result := a.Apply(ctx, u)
	if result.Error != nil {
		t.Fatalf("Apply() error = %v", result.Error)
	}
	if result.Applied {
		t.Error("Apply() should not report applied for already started")
	}
	if len(mock.StartCalls) != 0 {
		t.Errorf("StartCalls = %v, want empty", mock.StartCalls)
	}
}

func TestApply_StopUnit(t *testing.T) {
	mock := NewMockConnection()
	mock.SetProperty("test.service", "ActiveState", "active")
	a := NewSystemdApplier(mock)
	ctx := context.Background()

	u := mcov1alpha1.UnitSpec{
		Name:  "test.service",
		State: "stopped",
	}

	result := a.Apply(ctx, u)
	if result.Error != nil {
		t.Fatalf("Apply() error = %v", result.Error)
	}
	if !result.Applied {
		t.Error("Apply() should report applied for stop")
	}
	if len(mock.StopCalls) != 1 {
		t.Errorf("StopCalls = %v, want [test.service]", mock.StopCalls)
	}
}

func TestApply_StopIdempotent(t *testing.T) {
	mock := NewMockConnection()
	mock.SetProperty("test.service", "ActiveState", "inactive")
	a := NewSystemdApplier(mock)
	ctx := context.Background()

	u := mcov1alpha1.UnitSpec{
		Name:  "test.service",
		State: "stopped",
	}

	result := a.Apply(ctx, u)
	if result.Error != nil {
		t.Fatalf("Apply() error = %v", result.Error)
	}
	if result.Applied {
		t.Error("Apply() should not report applied for already stopped")
	}
}

func TestApply_RestartUnit(t *testing.T) {
	mock := NewMockConnection()
	a := NewSystemdApplier(mock)
	ctx := context.Background()

	u := mcov1alpha1.UnitSpec{
		Name:  "test.service",
		State: "restarted",
	}

	result := a.Apply(ctx, u)
	if result.Error != nil {
		t.Fatalf("Apply() error = %v", result.Error)
	}
	if !result.Applied {
		t.Error("Apply() should always report applied for restart")
	}
	if len(mock.RestartCalls) != 1 {
		t.Errorf("RestartCalls = %v, want [test.service]", mock.RestartCalls)
	}
}

func TestApply_ReloadUnit(t *testing.T) {
	mock := NewMockConnection()
	a := NewSystemdApplier(mock)
	ctx := context.Background()

	u := mcov1alpha1.UnitSpec{
		Name:  "test.service",
		State: "reloaded",
	}

	result := a.Apply(ctx, u)
	if result.Error != nil {
		t.Fatalf("Apply() error = %v", result.Error)
	}
	if !result.Applied {
		t.Error("Apply() should always report applied for reload")
	}
	if len(mock.ReloadCalls) != 1 {
		t.Errorf("ReloadCalls = %v, want [test.service]", mock.ReloadCalls)
	}
}

func TestSystemdApply_InvalidState(t *testing.T) {
	mock := NewMockConnection()
	a := NewSystemdApplier(mock)
	ctx := context.Background()

	u := mcov1alpha1.UnitSpec{
		Name:  "test.service",
		State: "invalid",
	}

	result := a.Apply(ctx, u)
	if result.Error == nil {
		t.Error("Apply() should error for invalid state")
	}
}

func TestApply_MaskError(t *testing.T) {
	mock := NewMockConnection()
	mock.Error = errors.New("mask failed")
	a := NewSystemdApplier(mock)
	ctx := context.Background()

	u := mcov1alpha1.UnitSpec{
		Name: "test.service",
		Mask: true,
	}

	result := a.Apply(ctx, u)
	if result.Error == nil {
		t.Error("Apply() should propagate mask error")
	}
}

func TestApply_EnableError(t *testing.T) {
	mock := NewMockConnection()
	a := NewSystemdApplier(mock)
	ctx := context.Background()

	// Set error after initial mask check
	enabled := true
	u := mcov1alpha1.UnitSpec{
		Name:    "test.service",
		Enabled: &enabled,
	}

	// Set disabled so enable will be attempted
	mock.SetProperty("test.service", "UnitFileState", "disabled")
	mock.Error = errors.New("enable failed")

	result := a.Apply(ctx, u)
	if result.Error == nil {
		t.Error("Apply() should propagate enable error")
	}
}

func TestApply_CombinedOperations(t *testing.T) {
	mock := NewMockConnection()
	mock.SetProperty("test.service", "UnitFileState", "disabled")
	mock.SetProperty("test.service", "ActiveState", "inactive")
	a := NewSystemdApplier(mock)
	ctx := context.Background()

	enabled := true
	u := mcov1alpha1.UnitSpec{
		Name:    "test.service",
		Mask:    false,
		Enabled: &enabled,
		State:   "started",
	}

	result := a.Apply(ctx, u)
	if result.Error != nil {
		t.Fatalf("Apply() error = %v", result.Error)
	}
	if !result.Applied {
		t.Error("Apply() should report applied")
	}
	if len(mock.EnableCalls) != 1 {
		t.Errorf("EnableCalls = %v, want 1", len(mock.EnableCalls))
	}
	if len(mock.StartCalls) != 1 {
		t.Errorf("StartCalls = %v, want 1", len(mock.StartCalls))
	}
}

func TestSystemdApplyAll_Success(t *testing.T) {
	mock := NewMockConnection()
	a := NewSystemdApplier(mock)
	ctx := context.Background()

	units := []mcov1alpha1.UnitSpec{
		{Name: "unit1.service", State: "restarted"},
		{Name: "unit2.service", State: "restarted"},
		{Name: "unit3.service", State: "restarted"},
	}

	results, err := a.ApplyAll(ctx, units)
	if err != nil {
		t.Fatalf("ApplyAll() error = %v", err)
	}
	if len(results) != 3 {
		t.Errorf("results count = %d, want 3", len(results))
	}
	if len(mock.RestartCalls) != 3 {
		t.Errorf("RestartCalls = %d, want 3", len(mock.RestartCalls))
	}
}

func TestSystemdApplyAll_StopsOnError(t *testing.T) {
	mock := NewMockConnection()
	a := NewSystemdApplier(mock)
	ctx := context.Background()

	units := []mcov1alpha1.UnitSpec{
		{Name: "unit1.service", State: "restarted"},
		{Name: "unit2.service", State: "invalid"}, // will fail
		{Name: "unit3.service", State: "restarted"},
	}

	results, err := a.ApplyAll(ctx, units)
	if err == nil {
		t.Fatal("ApplyAll() should error")
	}
	if len(results) != 2 {
		t.Errorf("results count = %d, want 2", len(results))
	}
	if len(mock.RestartCalls) != 1 {
		t.Errorf("RestartCalls = %d, want 1", len(mock.RestartCalls))
	}
}

func TestApply_NoEnabledField(t *testing.T) {
	mock := NewMockConnection()
	a := NewSystemdApplier(mock)
	ctx := context.Background()

	// Enabled is nil - should not call enable or disable
	u := mcov1alpha1.UnitSpec{
		Name:  "test.service",
		State: "started",
	}

	mock.SetProperty("test.service", "ActiveState", "inactive")

	result := a.Apply(ctx, u)
	if result.Error != nil {
		t.Fatalf("Apply() error = %v", result.Error)
	}
	if len(mock.EnableCalls) != 0 {
		t.Errorf("EnableCalls = %v, want empty", mock.EnableCalls)
	}
	if len(mock.DisableCalls) != 0 {
		t.Errorf("DisableCalls = %v, want empty", mock.DisableCalls)
	}
}

func TestApply_StopFailedState(t *testing.T) {
	mock := NewMockConnection()
	mock.SetProperty("test.service", "ActiveState", "failed")
	a := NewSystemdApplier(mock)
	ctx := context.Background()

	u := mcov1alpha1.UnitSpec{
		Name:  "test.service",
		State: "stopped",
	}

	result := a.Apply(ctx, u)
	if result.Error != nil {
		t.Fatalf("Apply() error = %v", result.Error)
	}
	// Already in failed state - no stop needed
	if result.Applied {
		t.Error("Apply() should not report applied for failed->stopped")
	}
}

func TestApply_EnabledRuntime(t *testing.T) {
	mock := NewMockConnection()
	mock.SetProperty("test.service", "UnitFileState", "enabled-runtime")
	a := NewSystemdApplier(mock)
	ctx := context.Background()

	enabled := true
	u := mcov1alpha1.UnitSpec{
		Name:    "test.service",
		Enabled: &enabled,
	}

	result := a.Apply(ctx, u)
	if result.Error != nil {
		t.Fatalf("Apply() error = %v", result.Error)
	}
	// enabled-runtime counts as enabled
	if result.Applied {
		t.Error("Apply() should not report applied for enabled-runtime")
	}
}
