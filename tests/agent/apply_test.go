//go:build unit

package agent

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"
	"k8s.io/utils/ptr"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/internal/agent"
	"in-cloud.io/machine-config/tests/mocks"
)

// TestApplyFiles_Success tests successful file application.
func TestApplyFiles_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFiles := mocks.NewMockFileOperations(ctrl)
	mockSystemd := mocks.NewMockSystemdConnection(ctrl)

	// Expect file apply to succeed
	mockFiles.EXPECT().Apply(gomock.Any()).Return(agent.FileApplyResult{
		Path:    "/etc/test.conf",
		Applied: true,
		Error:   nil,
	})

	// Expect Close on applier.Close()
	mockSystemd.EXPECT().Close()

	applier := agent.NewApplierWithFileOps(mockFiles, mockSystemd)
	defer applier.Close()

	config := &mcov1alpha1.RenderedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/test.conf", Content: "test", Mode: 0644},
		},
	}

	result, err := applier.Apply(context.Background(), config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success=true")
	}
	if result.FilesApplied != 1 {
		t.Errorf("expected FilesApplied=1, got %d", result.FilesApplied)
	}
}

// TestApplyFiles_DeleteFile tests file deletion (state=absent).
func TestApplyFiles_DeleteFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFiles := mocks.NewMockFileOperations(ctrl)
	mockSystemd := mocks.NewMockSystemdConnection(ctrl)

	// Expect file apply with state=absent
	mockFiles.EXPECT().Apply(mcov1alpha1.FileSpec{
		Path:  "/etc/old.conf",
		State: "absent",
	}).Return(agent.FileApplyResult{
		Path:    "/etc/old.conf",
		Applied: true,
		Error:   nil,
	})

	mockSystemd.EXPECT().Close()

	applier := agent.NewApplierWithFileOps(mockFiles, mockSystemd)
	defer applier.Close()

	config := &mcov1alpha1.RenderedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/old.conf", State: "absent"},
		},
	}

	result, err := applier.Apply(context.Background(), config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success=true")
	}
	if result.FilesApplied != 1 {
		t.Errorf("expected FilesApplied=1, got %d", result.FilesApplied)
	}
}

// TestApplyFiles_Error tests file apply error handling.
func TestApplyFiles_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFiles := mocks.NewMockFileOperations(ctrl)
	mockSystemd := mocks.NewMockSystemdConnection(ctrl)

	expectedErr := errors.New("permission denied")
	mockFiles.EXPECT().Apply(gomock.Any()).Return(agent.FileApplyResult{
		Path:    "/etc/secure.conf",
		Applied: false,
		Error:   expectedErr,
	})

	mockSystemd.EXPECT().Close()

	applier := agent.NewApplierWithFileOps(mockFiles, mockSystemd)
	defer applier.Close()

	config := &mcov1alpha1.RenderedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/secure.conf", Content: "secret", Mode: 0600},
		},
	}

	result, err := applier.Apply(context.Background(), config)

	if err == nil {
		t.Fatal("expected error")
	}
	if result.Success {
		t.Error("expected success=false")
	}
}

// TestApplyFiles_SortedByPath tests that files are applied sorted by path.
func TestApplyFiles_SortedByPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFiles := mocks.NewMockFileOperations(ctrl)
	mockSystemd := mocks.NewMockSystemdConnection(ctrl)

	// Use gomock.InOrder to verify call order
	gomock.InOrder(
		mockFiles.EXPECT().Apply(mcov1alpha1.FileSpec{
			Path: "/etc/a.conf", Content: "a", Mode: 0644,
		}).Return(agent.FileApplyResult{Applied: true}),
		mockFiles.EXPECT().Apply(mcov1alpha1.FileSpec{
			Path: "/etc/b.conf", Content: "b", Mode: 0644,
		}).Return(agent.FileApplyResult{Applied: true}),
		mockFiles.EXPECT().Apply(mcov1alpha1.FileSpec{
			Path: "/etc/c.conf", Content: "c", Mode: 0644,
		}).Return(agent.FileApplyResult{Applied: true}),
	)

	mockSystemd.EXPECT().Close()

	applier := agent.NewApplierWithFileOps(mockFiles, mockSystemd)
	defer applier.Close()

	// Input is NOT sorted
	config := &mcov1alpha1.RenderedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/c.conf", Content: "c", Mode: 0644},
			{Path: "/etc/a.conf", Content: "a", Mode: 0644},
			{Path: "/etc/b.conf", Content: "b", Mode: 0644},
		},
	}

	result, err := applier.Apply(context.Background(), config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FilesApplied != 3 {
		t.Errorf("expected FilesApplied=3, got %d", result.FilesApplied)
	}
}

// TestApplyFiles_Idempotent tests idempotent file application (no change).
func TestApplyFiles_Idempotent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFiles := mocks.NewMockFileOperations(ctrl)
	mockSystemd := mocks.NewMockSystemdConnection(ctrl)

	// Return Applied=false (file unchanged)
	mockFiles.EXPECT().Apply(gomock.Any()).Return(agent.FileApplyResult{
		Path:    "/etc/unchanged.conf",
		Applied: false, // No change made
		Error:   nil,
	})

	mockSystemd.EXPECT().Close()

	applier := agent.NewApplierWithFileOps(mockFiles, mockSystemd)
	defer applier.Close()

	config := &mcov1alpha1.RenderedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/unchanged.conf", Content: "same", Mode: 0644},
		},
	}

	result, err := applier.Apply(context.Background(), config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success=true")
	}
	if result.FilesApplied != 0 {
		t.Errorf("expected FilesApplied=0, got %d", result.FilesApplied)
	}
	if result.FilesSkipped != 1 {
		t.Errorf("expected FilesSkipped=1, got %d", result.FilesSkipped)
	}
}

// TestApplySystemd_Success tests successful systemd unit application.
func TestApplySystemd_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFiles := mocks.NewMockFileOperations(ctrl)
	mockSystemd := mocks.NewMockSystemdConnection(ctrl)

	// The systemd applier checks mask state first, then enabled, then state
	// For enabled + started:
	//   1. applyMask checks UnitFileState (not masked)
	//   2. applyEnabled checks UnitFileState, enables
	//   3. applyState checks ActiveState, starts
	gomock.InOrder(
		mockSystemd.EXPECT().GetUnitProperty(gomock.Any(), "test.service", "UnitFileState").Return("disabled", nil), // mask check
		mockSystemd.EXPECT().GetUnitProperty(gomock.Any(), "test.service", "UnitFileState").Return("disabled", nil), // enabled check
		mockSystemd.EXPECT().EnableUnit(gomock.Any(), "test.service").Return(nil),
		mockSystemd.EXPECT().GetUnitProperty(gomock.Any(), "test.service", "ActiveState").Return("inactive", nil),
		mockSystemd.EXPECT().StartUnit(gomock.Any(), "test.service").Return(nil),
	)
	mockSystemd.EXPECT().Close()

	applier := agent.NewApplierWithFileOps(mockFiles, mockSystemd)
	defer applier.Close()

	config := &mcov1alpha1.RenderedConfig{
		Systemd: mcov1alpha1.SystemdSpec{
			Units: []mcov1alpha1.UnitSpec{
				{Name: "test.service", Enabled: ptr.To(true), State: "started"},
			},
		},
	}

	result, err := applier.Apply(context.Background(), config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success=true")
	}
	if result.UnitsApplied != 1 {
		t.Errorf("expected UnitsApplied=1, got %d", result.UnitsApplied)
	}
}

// TestApplySystemd_Restart tests systemd unit restart.
func TestApplySystemd_Restart(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFiles := mocks.NewMockFileOperations(ctrl)
	mockSystemd := mocks.NewMockSystemdConnection(ctrl)

	// For restart only: check mask state (returns not masked), then restart
	mockSystemd.EXPECT().GetUnitProperty(gomock.Any(), "nginx.service", "UnitFileState").Return("enabled", nil)
	mockSystemd.EXPECT().RestartUnit(gomock.Any(), "nginx.service").Return(nil)
	mockSystemd.EXPECT().Close()

	applier := agent.NewApplierWithFileOps(mockFiles, mockSystemd)
	defer applier.Close()

	config := &mcov1alpha1.RenderedConfig{
		Systemd: mcov1alpha1.SystemdSpec{
			Units: []mcov1alpha1.UnitSpec{
				{Name: "nginx.service", State: "restarted"},
			},
		},
	}

	result, err := applier.Apply(context.Background(), config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success=true")
	}
	if result.UnitsApplied != 1 {
		t.Errorf("expected UnitsApplied=1, got %d", result.UnitsApplied)
	}
}

// TestApplySystemd_Mask tests systemd unit masking.
func TestApplySystemd_Mask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFiles := mocks.NewMockFileOperations(ctrl)
	mockSystemd := mocks.NewMockSystemdConnection(ctrl)

	// For mask: check current state, then mask
	mockSystemd.EXPECT().GetUnitProperty(gomock.Any(), "dangerous.service", "UnitFileState").Return("disabled", nil)
	mockSystemd.EXPECT().MaskUnit(gomock.Any(), "dangerous.service").Return(nil)
	mockSystemd.EXPECT().Close()

	applier := agent.NewApplierWithFileOps(mockFiles, mockSystemd)
	defer applier.Close()

	config := &mcov1alpha1.RenderedConfig{
		Systemd: mcov1alpha1.SystemdSpec{
			Units: []mcov1alpha1.UnitSpec{
				{Name: "dangerous.service", Mask: true},
			},
		},
	}

	result, err := applier.Apply(context.Background(), config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success=true")
	}
}

// TestApplySystemd_Error tests systemd error handling.
func TestApplySystemd_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFiles := mocks.NewMockFileOperations(ctrl)
	mockSystemd := mocks.NewMockSystemdConnection(ctrl)

	expectedErr := errors.New("unit not found")
	// For restarted: check mask state first
	mockSystemd.EXPECT().GetUnitProperty(gomock.Any(), "missing.service", "UnitFileState").Return("", nil)
	mockSystemd.EXPECT().RestartUnit(gomock.Any(), "missing.service").Return(expectedErr)
	mockSystemd.EXPECT().Close()

	applier := agent.NewApplierWithFileOps(mockFiles, mockSystemd)
	defer applier.Close()

	config := &mcov1alpha1.RenderedConfig{
		Systemd: mcov1alpha1.SystemdSpec{
			Units: []mcov1alpha1.UnitSpec{
				{Name: "missing.service", State: "restarted"},
			},
		},
	}

	result, err := applier.Apply(context.Background(), config)

	if err == nil {
		t.Fatal("expected error")
	}
	if result.Success {
		t.Error("expected success=false")
	}
}

// TestApplySystemd_SortedByName tests that units are applied sorted by name.
func TestApplySystemd_SortedByName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFiles := mocks.NewMockFileOperations(ctrl)
	mockSystemd := mocks.NewMockSystemdConnection(ctrl)

	// Units should be applied in sorted order, each preceded by mask state check
	gomock.InOrder(
		mockSystemd.EXPECT().GetUnitProperty(gomock.Any(), "aaa.service", "UnitFileState").Return("enabled", nil),
		mockSystemd.EXPECT().RestartUnit(gomock.Any(), "aaa.service").Return(nil),
		mockSystemd.EXPECT().GetUnitProperty(gomock.Any(), "bbb.service", "UnitFileState").Return("enabled", nil),
		mockSystemd.EXPECT().RestartUnit(gomock.Any(), "bbb.service").Return(nil),
		mockSystemd.EXPECT().GetUnitProperty(gomock.Any(), "ccc.service", "UnitFileState").Return("enabled", nil),
		mockSystemd.EXPECT().RestartUnit(gomock.Any(), "ccc.service").Return(nil),
	)
	mockSystemd.EXPECT().Close()

	applier := agent.NewApplierWithFileOps(mockFiles, mockSystemd)
	defer applier.Close()

	// Input is NOT sorted
	config := &mcov1alpha1.RenderedConfig{
		Systemd: mcov1alpha1.SystemdSpec{
			Units: []mcov1alpha1.UnitSpec{
				{Name: "ccc.service", State: "restarted"},
				{Name: "aaa.service", State: "restarted"},
				{Name: "bbb.service", State: "restarted"},
			},
		},
	}

	result, err := applier.Apply(context.Background(), config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.UnitsApplied != 3 {
		t.Errorf("expected UnitsApplied=3, got %d", result.UnitsApplied)
	}
}

// TestApply_ContextCancellation tests that apply respects context cancellation.
func TestApply_ContextCancellation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFiles := mocks.NewMockFileOperations(ctrl)
	mockSystemd := mocks.NewMockSystemdConnection(ctrl)

	mockSystemd.EXPECT().Close()

	applier := agent.NewApplierWithFileOps(mockFiles, mockSystemd)
	defer applier.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	config := &mcov1alpha1.RenderedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/test.conf", Content: "test", Mode: 0644},
		},
	}

	result, err := applier.Apply(ctx, config)

	if err == nil {
		t.Fatal("expected context error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
	if result.Success {
		t.Error("expected success=false")
	}
}

// TestApply_FilesAndUnits tests applying both files and units.
func TestApply_FilesAndUnits(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFiles := mocks.NewMockFileOperations(ctrl)
	mockSystemd := mocks.NewMockSystemdConnection(ctrl)

	// Files first, then units (with mask check)
	gomock.InOrder(
		mockFiles.EXPECT().Apply(gomock.Any()).Return(agent.FileApplyResult{Applied: true}),
		mockSystemd.EXPECT().GetUnitProperty(gomock.Any(), "app.service", "UnitFileState").Return("enabled", nil),
		mockSystemd.EXPECT().RestartUnit(gomock.Any(), "app.service").Return(nil),
	)
	mockSystemd.EXPECT().Close()

	applier := agent.NewApplierWithFileOps(mockFiles, mockSystemd)
	defer applier.Close()

	config := &mcov1alpha1.RenderedConfig{
		Files: []mcov1alpha1.FileSpec{
			{Path: "/etc/app.conf", Content: "config", Mode: 0644},
		},
		Systemd: mcov1alpha1.SystemdSpec{
			Units: []mcov1alpha1.UnitSpec{
				{Name: "app.service", State: "restarted"},
			},
		},
	}

	result, err := applier.Apply(context.Background(), config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success=true")
	}
	if result.FilesApplied != 1 {
		t.Errorf("expected FilesApplied=1, got %d", result.FilesApplied)
	}
	if result.UnitsApplied != 1 {
		t.Errorf("expected UnitsApplied=1, got %d", result.UnitsApplied)
	}
}

// TestApply_EmptyConfig tests applying empty configuration.
func TestApply_EmptyConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFiles := mocks.NewMockFileOperations(ctrl)
	mockSystemd := mocks.NewMockSystemdConnection(ctrl)

	mockSystemd.EXPECT().Close()

	applier := agent.NewApplierWithFileOps(mockFiles, mockSystemd)
	defer applier.Close()

	config := &mcov1alpha1.RenderedConfig{}

	result, err := applier.Apply(context.Background(), config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success=true for empty config")
	}
	if result.FilesApplied != 0 || result.UnitsApplied != 0 {
		t.Error("expected no changes for empty config")
	}
}
