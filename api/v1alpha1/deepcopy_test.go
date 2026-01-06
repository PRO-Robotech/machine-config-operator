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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// TestMachineConfigDeepCopy verifies MachineConfig DeepCopy creates independent copies.
func TestMachineConfigDeepCopy(t *testing.T) {
	enabled := true
	original := &MachineConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-mc",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Spec: MachineConfigSpec{
			Priority: 50,
			Files: []FileSpec{
				{
					Path:    "/etc/test.conf",
					Content: "original content",
					Mode:    420,
					Owner:   "root:root",
					State:   "present",
				},
			},
			Systemd: SystemdSpec{
				Units: []UnitSpec{
					{
						Name:    "test.service",
						Enabled: &enabled,
						State:   "started",
					},
				},
			},
			Reboot: RebootRequirementSpec{
				Required: false,
				Reason:   "test",
			},
		},
	}

	// Create deep copy
	copied := original.DeepCopy()

	// Verify it's a different pointer
	if copied == original {
		t.Error("DeepCopy returned same pointer")
	}

	// Modify the copy
	copied.Name = "modified-mc"
	copied.Spec.Priority = 100
	copied.Spec.Files[0].Content = "modified content"
	*copied.Spec.Systemd.Units[0].Enabled = false

	// Verify original is unchanged
	if original.Name != "test-mc" {
		t.Errorf("original.Name was modified: got %q, want %q", original.Name, "test-mc")
	}
	if original.Spec.Priority != 50 {
		t.Errorf("original.Spec.Priority was modified: got %d, want %d", original.Spec.Priority, 50)
	}
	if original.Spec.Files[0].Content != "original content" {
		t.Errorf("original.Spec.Files[0].Content was modified: got %q", original.Spec.Files[0].Content)
	}
	if !*original.Spec.Systemd.Units[0].Enabled {
		t.Error("original.Spec.Systemd.Units[0].Enabled was modified")
	}
}

// TestMachineConfigPoolDeepCopy verifies MachineConfigPool DeepCopy creates independent copies.
func TestMachineConfigPoolDeepCopy(t *testing.T) {
	original := &MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pool",
		},
		Spec: MachineConfigPoolSpec{
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"role": "worker",
				},
			},
			MachineConfigSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"pool": "worker",
				},
			},
			Rollout: RolloutConfig{
				DebounceSeconds: 30,
			},
			Reboot: RebootPolicy{
				Strategy: "Never",
			},
		},
		Status: MachineConfigPoolStatus{
			MachineCount:      5,
			ReadyMachineCount: 3,
			Conditions: []metav1.Condition{
				{
					Type:   ConditionUpdated,
					Status: metav1.ConditionTrue,
				},
			},
		},
	}

	// Create deep copy
	copied := original.DeepCopy()

	// Verify it's a different pointer
	if copied == original {
		t.Error("DeepCopy returned same pointer")
	}

	// Modify the copy
	copied.Spec.NodeSelector.MatchLabels["role"] = "master"
	copied.Status.MachineCount = 10
	copied.Status.Conditions[0].Status = metav1.ConditionFalse

	// Verify original is unchanged
	if original.Spec.NodeSelector.MatchLabels["role"] != "worker" {
		t.Errorf("original.Spec.NodeSelector was modified")
	}
	if original.Status.MachineCount != 5 {
		t.Errorf("original.Status.MachineCount was modified: got %d", original.Status.MachineCount)
	}
	if original.Status.Conditions[0].Status != metav1.ConditionTrue {
		t.Error("original.Status.Conditions was modified")
	}
}

// TestRenderedMachineConfigDeepCopy verifies RenderedMachineConfig DeepCopy creates independent copies.
func TestRenderedMachineConfigDeepCopy(t *testing.T) {
	original := &RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "rendered-worker-abc123",
		},
		Spec: RenderedMachineConfigSpec{
			PoolName:   "worker",
			Revision:   "abc123",
			ConfigHash: "abc123def456789012345678901234567890123456789012345678901234abcd",
			Config: RenderedConfig{
				Files: []FileSpec{
					{
						Path:    "/etc/test.conf",
						Content: "original",
					},
				},
			},
			Sources: []ConfigSource{
				{Name: "base", Priority: 10},
				{Name: "custom", Priority: 50},
			},
			Reboot: RenderedRebootSpec{
				Required: false,
				Strategy: "Never",
			},
		},
	}

	// Create deep copy
	copied := original.DeepCopy()

	// Verify it's a different pointer
	if copied == original {
		t.Error("DeepCopy returned same pointer")
	}

	// Modify the copy
	copied.Spec.Config.Files[0].Content = "modified"
	copied.Spec.Sources[0].Priority = 100

	// Verify original is unchanged
	if original.Spec.Config.Files[0].Content != "original" {
		t.Errorf("original.Spec.Config.Files was modified")
	}
	if original.Spec.Sources[0].Priority != 10 {
		t.Errorf("original.Spec.Sources was modified")
	}
}

// TestDeepCopyNil verifies DeepCopy handles nil correctly.
func TestDeepCopyNil(t *testing.T) {
	var nilMC *MachineConfig
	if nilMC.DeepCopy() != nil {
		t.Error("DeepCopy of nil should return nil")
	}

	var nilMCP *MachineConfigPool
	if nilMCP.DeepCopy() != nil {
		t.Error("DeepCopy of nil should return nil")
	}

	var nilRMC *RenderedMachineConfig
	if nilRMC.DeepCopy() != nil {
		t.Error("DeepCopy of nil should return nil")
	}
}

// TestDeepCopyObject verifies types implement runtime.Object.
func TestDeepCopyObject(t *testing.T) {
	// Verify MachineConfig implements runtime.Object
	var _ runtime.Object = &MachineConfig{}
	var _ runtime.Object = &MachineConfigList{}

	// Verify MachineConfigPool implements runtime.Object
	var _ runtime.Object = &MachineConfigPool{}
	var _ runtime.Object = &MachineConfigPoolList{}

	// Verify RenderedMachineConfig implements runtime.Object
	var _ runtime.Object = &RenderedMachineConfig{}
	var _ runtime.Object = &RenderedMachineConfigList{}

	// Test DeepCopyObject returns correct type
	mc := &MachineConfig{ObjectMeta: metav1.ObjectMeta{Name: "test"}}
	mcObj := mc.DeepCopyObject()
	if _, ok := mcObj.(*MachineConfig); !ok {
		t.Error("DeepCopyObject should return *MachineConfig")
	}

	mcp := &MachineConfigPool{ObjectMeta: metav1.ObjectMeta{Name: "test"}}
	mcpObj := mcp.DeepCopyObject()
	if _, ok := mcpObj.(*MachineConfigPool); !ok {
		t.Error("DeepCopyObject should return *MachineConfigPool")
	}

	rmc := &RenderedMachineConfig{ObjectMeta: metav1.ObjectMeta{Name: "test"}}
	rmcObj := rmc.DeepCopyObject()
	if _, ok := rmcObj.(*RenderedMachineConfig); !ok {
		t.Error("DeepCopyObject should return *RenderedMachineConfig")
	}
}

// TestListDeepCopy verifies List types create independent copies.
func TestListDeepCopy(t *testing.T) {
	original := &MachineConfigList{
		Items: []MachineConfig{
			{ObjectMeta: metav1.ObjectMeta{Name: "mc1"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "mc2"}},
		},
	}

	copied := original.DeepCopy()

	// Modify the copy
	copied.Items[0].Name = "modified"

	// Verify original is unchanged
	if original.Items[0].Name != "mc1" {
		t.Errorf("original list was modified: got %q", original.Items[0].Name)
	}
}
