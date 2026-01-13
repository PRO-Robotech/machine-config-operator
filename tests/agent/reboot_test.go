//go:build unit

package agent

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/internal/agent"
	"in-cloud.io/machine-config/tests/mocks"
)

// TestRebootDecision_NotRequired tests when reboot is not required.
func TestRebootDecision_NotRequired(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFetcher := mocks.NewMockRMCFetcher(ctrl)

	// Current RMC with no reboot requirements
	currentRMC := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "rmc-current"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Config: mcov1alpha1.RenderedConfig{
				Files: []mcov1alpha1.FileSpec{
					{Path: "/etc/old.conf", Content: "old"},
				},
			},
			RebootRequirements: mcov1alpha1.RebootRequirements{
				Files: map[string]bool{},
				Units: map[string]bool{},
			},
		},
	}

	// New RMC with changed file, but no reboot requirement
	newRMC := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "rmc-new"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Config: mcov1alpha1.RenderedConfig{
				Files: []mcov1alpha1.FileSpec{
					{Path: "/etc/old.conf", Content: "new"}, // Changed but doesn't require reboot
				},
			},
			RebootRequirements: mcov1alpha1.RebootRequirements{
				Files: map[string]bool{"/etc/old.conf": false}, // Explicitly not required
				Units: map[string]bool{},
			},
		},
	}

	mockFetcher.EXPECT().FetchRMC(gomock.Any(), "rmc-current").Return(currentRMC, nil)

	determiner := agent.NewRebootDeterminer(mockFetcher)
	decision := determiner.DetermineReboot(context.Background(), "rmc-current", newRMC)

	if decision.Required {
		t.Errorf("expected Required=false, reasons: %v", decision.Reasons)
	}
	if decision.Method != agent.MethodDiffBased {
		t.Errorf("expected Method=%s, got %s", agent.MethodDiffBased, decision.Method)
	}
}

// TestRebootDecision_RequiredWithFileChange tests reboot required by config
// but controlled by pool strategy (handled at controller level, not here).
func TestRebootDecision_RequiredWithFileChange(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFetcher := mocks.NewMockRMCFetcher(ctrl)

	// Current RMC
	currentRMC := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "rmc-current"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Config: mcov1alpha1.RenderedConfig{
				Files: []mcov1alpha1.FileSpec{
					{Path: "/etc/kernel.conf", Content: "old"},
				},
			},
			RebootRequirements: mcov1alpha1.RebootRequirements{
				Files: map[string]bool{"/etc/kernel.conf": true},
				Units: map[string]bool{},
			},
		},
	}

	// New RMC with changed kernel config (requires reboot)
	newRMC := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "rmc-new"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Config: mcov1alpha1.RenderedConfig{
				Files: []mcov1alpha1.FileSpec{
					{Path: "/etc/kernel.conf", Content: "new"},
				},
			},
			RebootRequirements: mcov1alpha1.RebootRequirements{
				Files: map[string]bool{"/etc/kernel.conf": true},
				Units: map[string]bool{},
			},
		},
	}

	mockFetcher.EXPECT().FetchRMC(gomock.Any(), "rmc-current").Return(currentRMC, nil)

	determiner := agent.NewRebootDeterminer(mockFetcher)
	decision := determiner.DetermineReboot(context.Background(), "rmc-current", newRMC)

	if !decision.Required {
		t.Error("expected Required=true")
	}
	if decision.Method != agent.MethodDiffBased {
		t.Errorf("expected Method=%s, got %s", agent.MethodDiffBased, decision.Method)
	}
	if len(decision.Reasons) == 0 {
		t.Error("expected at least one reason")
	}
}

// TestRebootDecision_FirstApply tests first apply (no current revision).
func TestRebootDecision_FirstApply(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFetcher := mocks.NewMockRMCFetcher(ctrl)
	// No FetchRMC call expected for first apply

	newRMC := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "rmc-initial"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Reboot: mcov1alpha1.RenderedRebootSpec{
				Required: true,
			},
		},
	}

	determiner := agent.NewRebootDeterminer(mockFetcher)
	decision := determiner.DetermineReboot(context.Background(), "", newRMC) // Empty current

	if !decision.Required {
		t.Error("expected Required=true for first apply with reboot required")
	}
	if decision.Method != agent.MethodLegacyFirstApply {
		t.Errorf("expected Method=%s, got %s", agent.MethodLegacyFirstApply, decision.Method)
	}
}

// TestRebootDecision_SameRevision tests same revision (no change).
func TestRebootDecision_SameRevision(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFetcher := mocks.NewMockRMCFetcher(ctrl)
	// No FetchRMC call expected for same revision

	rmc := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "rmc-same"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Reboot: mcov1alpha1.RenderedRebootSpec{
				Required: true, // Even with Required=true, same revision means no reboot
			},
		},
	}

	determiner := agent.NewRebootDeterminer(mockFetcher)
	decision := determiner.DetermineReboot(context.Background(), "rmc-same", rmc)

	if decision.Required {
		t.Error("expected Required=false for same revision")
	}
	if decision.Method != agent.MethodSameRevision {
		t.Errorf("expected Method=%s, got %s", agent.MethodSameRevision, decision.Method)
	}
}

// TestRebootDecision_UnitChange tests reboot required due to unit change.
func TestRebootDecision_UnitChange(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFetcher := mocks.NewMockRMCFetcher(ctrl)

	// Current RMC
	currentRMC := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "rmc-current"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Config: mcov1alpha1.RenderedConfig{
				Systemd: mcov1alpha1.SystemdSpec{},
			},
			RebootRequirements: mcov1alpha1.RebootRequirements{
				Files: map[string]bool{},
				Units: map[string]bool{},
			},
		},
	}

	// New RMC with new unit that requires reboot
	newRMC := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "rmc-new"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Config: mcov1alpha1.RenderedConfig{
				Systemd: mcov1alpha1.SystemdSpec{
					Units: []mcov1alpha1.UnitSpec{
						{Name: "critical.service", State: "started"},
					},
				},
			},
			RebootRequirements: mcov1alpha1.RebootRequirements{
				Files: map[string]bool{},
				Units: map[string]bool{"critical.service": true},
			},
		},
	}

	mockFetcher.EXPECT().FetchRMC(gomock.Any(), "rmc-current").Return(currentRMC, nil)

	determiner := agent.NewRebootDeterminer(mockFetcher)
	decision := determiner.DetermineReboot(context.Background(), "rmc-current", newRMC)

	if !decision.Required {
		t.Error("expected Required=true for unit requiring reboot")
	}
	if decision.Method != agent.MethodDiffBased {
		t.Errorf("expected Method=%s, got %s", agent.MethodDiffBased, decision.Method)
	}
}

// TestRebootDecision_RemovedFile tests reboot required when removing a file
// that originally required reboot.
func TestRebootDecision_RemovedFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFetcher := mocks.NewMockRMCFetcher(ctrl)

	// Current RMC has file that requires reboot
	currentRMC := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "rmc-current"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Config: mcov1alpha1.RenderedConfig{
				Files: []mcov1alpha1.FileSpec{
					{Path: "/etc/kernel-module.conf", Content: "module"},
				},
			},
			RebootRequirements: mcov1alpha1.RebootRequirements{
				Files: map[string]bool{"/etc/kernel-module.conf": true}, // Removing this requires reboot
				Units: map[string]bool{},
			},
		},
	}

	// New RMC - file removed
	newRMC := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "rmc-new"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Config: mcov1alpha1.RenderedConfig{
				Files: []mcov1alpha1.FileSpec{}, // File removed
			},
			RebootRequirements: mcov1alpha1.RebootRequirements{
				Files: map[string]bool{},
				Units: map[string]bool{},
			},
		},
	}

	mockFetcher.EXPECT().FetchRMC(gomock.Any(), "rmc-current").Return(currentRMC, nil)

	determiner := agent.NewRebootDeterminer(mockFetcher)
	decision := determiner.DetermineReboot(context.Background(), "rmc-current", newRMC)

	if !decision.Required {
		t.Error("expected Required=true when removing file that required reboot")
	}
}

// TestRebootDecision_RMCFetchError tests fallback when current RMC cannot be fetched.
func TestRebootDecision_RMCFetchError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFetcher := mocks.NewMockRMCFetcher(ctrl)

	expectedErr := errors.New("RMC not found")
	mockFetcher.EXPECT().FetchRMC(gomock.Any(), "rmc-missing").Return(nil, expectedErr)

	newRMC := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "rmc-new"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Reboot: mcov1alpha1.RenderedRebootSpec{
				Required: true,
			},
		},
	}

	determiner := agent.NewRebootDeterminer(mockFetcher)
	decision := determiner.DetermineReboot(context.Background(), "rmc-missing", newRMC)

	if !decision.Required {
		t.Error("expected Required=true (fallback to legacy)")
	}
	if decision.Method != agent.MethodLegacyFallback {
		t.Errorf("expected Method=%s, got %s", agent.MethodLegacyFallback, decision.Method)
	}
}

// TestRebootDecision_NoRebootRequirements tests fallback when neither RMC
// has RebootRequirements populated.
func TestRebootDecision_NoRebootRequirements(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFetcher := mocks.NewMockRMCFetcher(ctrl)

	// Current RMC without RebootRequirements
	currentRMC := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "rmc-current"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Config: mcov1alpha1.RenderedConfig{
				Files: []mcov1alpha1.FileSpec{
					{Path: "/etc/test.conf", Content: "old"},
				},
			},
			// RebootRequirements empty (not populated)
		},
	}

	// New RMC without RebootRequirements
	newRMC := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "rmc-new"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Config: mcov1alpha1.RenderedConfig{
				Files: []mcov1alpha1.FileSpec{
					{Path: "/etc/test.conf", Content: "new"},
				},
			},
			Reboot: mcov1alpha1.RenderedRebootSpec{
				Required: false, // Legacy OR-based result
			},
		},
	}

	mockFetcher.EXPECT().FetchRMC(gomock.Any(), "rmc-current").Return(currentRMC, nil)

	determiner := agent.NewRebootDeterminer(mockFetcher)
	decision := determiner.DetermineReboot(context.Background(), "rmc-current", newRMC)

	if decision.Method != agent.MethodLegacyFallback {
		t.Errorf("expected Method=%s, got %s", agent.MethodLegacyFallback, decision.Method)
	}
}

// TestRebootDecision_MultipleChanges tests multiple file and unit changes.
func TestRebootDecision_MultipleChanges(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFetcher := mocks.NewMockRMCFetcher(ctrl)

	currentRMC := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "rmc-current"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Config: mcov1alpha1.RenderedConfig{
				Files: []mcov1alpha1.FileSpec{
					{Path: "/etc/a.conf", Content: "a"},
					{Path: "/etc/b.conf", Content: "b"},
				},
				Systemd: mcov1alpha1.SystemdSpec{
					Units: []mcov1alpha1.UnitSpec{
						{Name: "svc-a.service"},
					},
				},
			},
			RebootRequirements: mcov1alpha1.RebootRequirements{
				Files: map[string]bool{"/etc/a.conf": true, "/etc/b.conf": false},
				Units: map[string]bool{"svc-a.service": true},
			},
		},
	}

	newRMC := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "rmc-new"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Config: mcov1alpha1.RenderedConfig{
				Files: []mcov1alpha1.FileSpec{
					{Path: "/etc/a.conf", Content: "a-new"}, // Changed, requires reboot
					{Path: "/etc/b.conf", Content: "b-new"}, // Changed, no reboot
					{Path: "/etc/c.conf", Content: "c"},     // Added
				},
				Systemd: mcov1alpha1.SystemdSpec{
					Units: []mcov1alpha1.UnitSpec{
						{Name: "svc-b.service"}, // svc-a removed, svc-b added
					},
				},
			},
			RebootRequirements: mcov1alpha1.RebootRequirements{
				Files: map[string]bool{
					"/etc/a.conf": true,
					"/etc/b.conf": false,
					"/etc/c.conf": false,
				},
				Units: map[string]bool{"svc-b.service": false},
			},
		},
	}

	mockFetcher.EXPECT().FetchRMC(gomock.Any(), "rmc-current").Return(currentRMC, nil)

	determiner := agent.NewRebootDeterminer(mockFetcher)
	decision := determiner.DetermineReboot(context.Background(), "rmc-current", newRMC)

	if !decision.Required {
		t.Error("expected Required=true")
	}
	// Should have reasons for both /etc/a.conf change and svc-a.service removal
	if len(decision.Reasons) < 2 {
		t.Errorf("expected at least 2 reasons, got %d: %v", len(decision.Reasons), decision.Reasons)
	}
}
