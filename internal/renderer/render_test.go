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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// setupFakeClient creates a fake client with the scheme registered.
func setupFakeClient(objects ...client.Object) client.Client {
	scheme := runtime.NewScheme()
	_ = mcov1alpha1.AddToScheme(scheme)

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		Build()
}

// TestBuildRMC_Basic verifies basic RMC creation.
func TestBuildRMC_Basic(t *testing.T) {
	merged := &MergedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/test.conf", Content: "test", Mode: 420, Owner: "root:root", State: "present"},
		},
		Units: []mcov1alpha1.UnitSpec{
			{Name: "test.service", Enabled: boolPtr(true), State: "started"},
		},
		RebootRequired: true,
		Sources: []ConfigSource{
			{Name: "mc-base", Priority: 10},
			{Name: "mc-override", Priority: 50},
		},
	}

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Reboot: mcov1alpha1.RebootPolicy{
				Strategy:           "IfRequired",
				MinIntervalSeconds: 3600,
			},
		},
	}

	rmc := BuildRMC("worker", merged, pool)

	if len(rmc.Name) == 0 {
		t.Error("RMC name should not be empty")
	}
	if rmc.Name[:7] != "worker-" {
		t.Errorf("RMC name should start with 'worker-', got %q", rmc.Name)
	}

	if rmc.Labels["mco.in-cloud.io/pool"] != "worker" {
		t.Errorf("Pool label = %q, want 'worker'", rmc.Labels["mco.in-cloud.io/pool"])
	}

	if rmc.Spec.PoolName != "worker" {
		t.Errorf("PoolName = %q, want 'worker'", rmc.Spec.PoolName)
	}
	if len(rmc.Spec.Revision) != 10 {
		t.Errorf("Revision length = %d, want 10", len(rmc.Spec.Revision))
	}
	if len(rmc.Spec.ConfigHash) != 64 { // SHA256 hex without prefix
		t.Errorf("ConfigHash length = %d, want 64", len(rmc.Spec.ConfigHash))
	}

	if len(rmc.Spec.Config.Files) != 1 {
		t.Errorf("Config.Files count = %d, want 1", len(rmc.Spec.Config.Files))
	}
	if len(rmc.Spec.Config.Systemd.Units) != 1 {
		t.Errorf("Config.Systemd.Units count = %d, want 1", len(rmc.Spec.Config.Systemd.Units))
	}

	if len(rmc.Spec.Sources) != 2 {
		t.Errorf("Sources count = %d, want 2", len(rmc.Spec.Sources))
	}

	if !rmc.Spec.Reboot.Required {
		t.Error("Reboot.Required should be true")
	}
	if rmc.Spec.Reboot.Strategy != "IfRequired" {
		t.Errorf("Reboot.Strategy = %q, want 'IfRequired'", rmc.Spec.Reboot.Strategy)
	}
	if rmc.Spec.Reboot.MinIntervalSeconds != 3600 {
		t.Errorf("Reboot.MinIntervalSeconds = %d, want 3600", rmc.Spec.Reboot.MinIntervalSeconds)
	}
}

// TestBuildRMC_NilPool verifies defaults when pool is nil.
func TestBuildRMC_NilPool(t *testing.T) {
	merged := &MergedConfig{
		RebootRequired: false,
	}

	rmc := BuildRMC("master", merged, nil)

	if rmc.Spec.Reboot.Strategy != "Never" {
		t.Errorf("Default strategy = %q, want 'Never'", rmc.Spec.Reboot.Strategy)
	}
	if rmc.Spec.Reboot.MinIntervalSeconds != 1800 {
		t.Errorf("Default MinIntervalSeconds = %d, want 1800", rmc.Spec.Reboot.MinIntervalSeconds)
	}
}

// TestBuildRMC_EmptyMerged verifies handling of empty merged config.
func TestBuildRMC_EmptyMerged(t *testing.T) {
	merged := &MergedConfig{}

	rmc := BuildRMC("empty-pool", merged, nil)

	if rmc.Name == "" {
		t.Error("RMC name should not be empty even with empty config")
	}
	if rmc.Spec.PoolName != "empty-pool" {
		t.Errorf("PoolName = %q, want 'empty-pool'", rmc.Spec.PoolName)
	}
}

// TestBuildRMC_Determinism verifies same input produces same output.
func TestBuildRMC_Determinism(t *testing.T) {
	merged := &MergedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/a.conf", Content: "a"},
			{Path: "/etc/b.conf", Content: "b"},
		},
	}

	rmc1 := BuildRMC("test", merged, nil)
	rmc2 := BuildRMC("test", merged, nil)

	if rmc1.Name != rmc2.Name {
		t.Errorf("RMC names should be identical: %q vs %q", rmc1.Name, rmc2.Name)
	}
	if rmc1.Spec.ConfigHash != rmc2.Spec.ConfigHash {
		t.Errorf("ConfigHashes should be identical")
	}
}

// TestCheckExistingRMC_NotFound verifies behavior when RMC doesn't exist.
func TestCheckExistingRMC_NotFound(t *testing.T) {
	c := setupFakeClient()
	ctx := context.Background()

	newRMC := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-abcdef1234"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			ConfigHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		},
	}

	existing, err := CheckExistingRMC(ctx, c, newRMC)

	if err != nil {
		t.Fatalf("CheckExistingRMC() error = %v", err)
	}
	if existing != nil {
		t.Error("Expected nil when RMC doesn't exist")
	}
}

// TestCheckExistingRMC_SameHash verifies behavior when same config exists.
func TestCheckExistingRMC_SameHash(t *testing.T) {
	existingRMC := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-abcdef1234"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			ConfigHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		},
	}
	c := setupFakeClient(existingRMC)
	ctx := context.Background()

	newRMC := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-abcdef1234"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			ConfigHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", // Same hash
		},
	}

	existing, err := CheckExistingRMC(ctx, c, newRMC)

	if err != nil {
		t.Fatalf("CheckExistingRMC() error = %v", err)
	}
	if existing == nil {
		t.Error("Expected existing RMC when hash matches")
	}
	if existing.Name != existingRMC.Name {
		t.Errorf("Returned wrong RMC: got %q, want %q", existing.Name, existingRMC.Name)
	}
}

// TestCheckExistingRMC_Collision verifies collision detection.
func TestCheckExistingRMC_Collision(t *testing.T) {
	existingRMC := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-abcdef1234"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			ConfigHash: "1111111111111111111111111111111111111111111111111111111111111111",
		},
	}
	c := setupFakeClient(existingRMC)
	ctx := context.Background()

	newRMC := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-abcdef1234"}, // Same name
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			ConfigHash: "2222222222222222222222222222222222222222222222222222222222222222", // Different hash!
		},
	}

	existing, err := CheckExistingRMC(ctx, c, newRMC)

	if err == nil {
		t.Error("Expected error for collision")
	}
	if !errors.Is(err, ErrHashCollision) {
		t.Errorf("Expected ErrHashCollision, got %v", err)
	}
	if existing == nil {
		t.Error("Expected existing RMC to be returned on collision")
	}
}

// TestHandleCollision verifies collision handling.
func TestHandleCollision(t *testing.T) {
	rmc := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-abcdef1234"},
	}

	result := HandleCollision(rmc, 1)

	if result.Name != "worker-abcdef1234-1" {
		t.Errorf("Name = %q, want 'worker-abcdef1234-1'", result.Name)
	}

	// Multiple collisions
	result = HandleCollision(result, 2)
	if result.Name != "worker-abcdef1234-1-2" {
		t.Errorf("Name = %q, want 'worker-abcdef1234-1-2'", result.Name)
	}
}

// TestRender_NewRMC verifies render creates new RMC when none exists.
func TestRender_NewRMC(t *testing.T) {
	c := setupFakeClient()
	ctx := context.Background()

	configs := []*mcov1alpha1.MachineConfig{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "mc-test"},
			Spec: mcov1alpha1.MachineConfigSpec{
				Priority: 50,
				Files: []mcov1alpha1.FileSpec{
					{Path: "/etc/test.conf", Content: "test", Mode: 420, Owner: "root:root", State: "present"},
				},
			},
		},
	}

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			Reboot: mcov1alpha1.RebootPolicy{Strategy: "Never"},
		},
	}

	result, err := Render(ctx, c, "worker", configs, pool)

	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if result.Existing {
		t.Error("Expected new RMC, not existing")
	}
	if result.Collision {
		t.Error("Expected no collision")
	}
	if result.RMC == nil {
		t.Fatal("RMC should not be nil")
	}
	if result.RMC.Spec.PoolName != "worker" {
		t.Errorf("PoolName = %q, want 'worker'", result.RMC.Spec.PoolName)
	}
}

// TestRender_Existing verifies render detects existing RMC.
func TestRender_Existing(t *testing.T) {
	configs := []*mcov1alpha1.MachineConfig{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "mc-test"},
			Spec: mcov1alpha1.MachineConfigSpec{
				Priority: 50,
				Files: []mcov1alpha1.FileSpec{
					{Path: "/etc/test.conf", Content: "test", Mode: 420, Owner: "root:root", State: "present"},
				},
			},
		},
	}

	merged := Merge(configs)
	expectedRMC := BuildRMC("worker", merged, nil)

	c := setupFakeClient(expectedRMC)
	ctx := context.Background()

	result, err := Render(ctx, c, "worker", configs, nil)

	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !result.Existing {
		t.Error("Expected existing RMC")
	}
	if result.Collision {
		t.Error("Expected no collision")
	}
}

// TestRender_Collision verifies render handles collision.
func TestRender_Collision(t *testing.T) {
	configs := []*mcov1alpha1.MachineConfig{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "mc-new"},
			Spec: mcov1alpha1.MachineConfigSpec{
				Files: []mcov1alpha1.FileSpec{
					{Path: "/etc/new.conf", Content: "new content", Mode: 420, Owner: "root:root", State: "present"},
				},
			},
		},
	}

	merged := Merge(configs)
	expectedRMC := BuildRMC("worker", merged, nil)

	// Create a conflicting RMC with same name but different hash
	conflictingRMC := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: expectedRMC.Name},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			ConfigHash: "0000000000000000000000000000000000000000000000000000000000000000", // Different!
		},
	}

	c := setupFakeClient(conflictingRMC)
	ctx := context.Background()

	result, err := Render(ctx, c, "worker", configs, nil)

	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !result.Collision {
		t.Error("Expected collision to be detected")
	}
	// Name should have suffix
	if result.RMC.Name == expectedRMC.Name {
		t.Error("RMC name should have suffix after collision")
	}
}

// TestRender_EmptyPoolName verifies error on empty pool name.
func TestRender_EmptyPoolName(t *testing.T) {
	c := setupFakeClient()
	ctx := context.Background()

	_, err := Render(ctx, c, "", nil, nil)

	if err == nil {
		t.Error("Expected error for empty pool name")
	}
}

// TestRender_EmptyConfigs verifies handling of empty config list.
func TestRender_EmptyConfigs(t *testing.T) {
	c := setupFakeClient()
	ctx := context.Background()

	result, err := Render(ctx, c, "worker", nil, nil)

	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if result.RMC == nil {
		t.Error("RMC should not be nil even with empty configs")
	}
}

// TestValidateMerged verifies merged config validation.
func TestValidateMerged(t *testing.T) {
	tests := []struct {
		name    string
		merged  *MergedConfig
		wantErr bool
	}{
		{
			name:    "nil merged",
			merged:  nil,
			wantErr: true,
		},
		{
			name:    "valid empty",
			merged:  &MergedConfig{},
			wantErr: false,
		},
		{
			name: "valid with files",
			merged: &MergedConfig{
				Files: []mcov1alpha1.FileSpec{
					{Path: "/etc/test.conf", Content: "test", Mode: 420, Owner: "root:root", State: "present"},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid file path",
			merged: &MergedConfig{
				Files: []mcov1alpha1.FileSpec{
					{Path: "/bin/bad", Content: "test"},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid unit name",
			merged: &MergedConfig{
				Units: []mcov1alpha1.UnitSpec{
					{Name: "kubelet.service"}, // Forbidden
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMerged(tt.merged)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateMerged() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
