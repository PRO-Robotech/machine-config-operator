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
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/pkg/annotations"
)

// mockNodeWriter is a mock implementation of NodeAnnotationWriter.
type mockNodeWriter struct {
	state           string
	rebootPending   *bool
	forceCleared    bool
	currentRevision string
	setStateErr     error
	setPendingErr   error
	clearForceErr   error
	setRevisionErr  error
}

func (m *mockNodeWriter) SetState(ctx context.Context, state string) error {
	if m.setStateErr != nil {
		return m.setStateErr
	}
	m.state = state
	return nil
}

func (m *mockNodeWriter) SetRebootPending(ctx context.Context, pending bool) error {
	if m.setPendingErr != nil {
		return m.setPendingErr
	}
	m.rebootPending = &pending
	return nil
}

func (m *mockNodeWriter) ClearForceReboot(ctx context.Context) error {
	if m.clearForceErr != nil {
		return m.clearForceErr
	}
	m.forceCleared = true
	return nil
}

func (m *mockNodeWriter) SetCurrentRevision(ctx context.Context, revision string) error {
	if m.setRevisionErr != nil {
		return m.setRevisionErr
	}
	m.currentRevision = revision
	return nil
}

// mockExecutor is a mock reboot executor.
type mockExecutor struct {
	called bool
	err    error
}

func (m *mockExecutor) Execute(ctx context.Context) error {
	m.called = true
	return m.err
}

func TestNewHandler(t *testing.T) {
	writer := &mockNodeWriter{}
	executor := &mockExecutor{}

	handler := NewHandler("/host", writer, executor)

	if handler == nil {
		t.Fatal("NewHandler() returned nil")
	}
	if handler.hostRoot != "/host" {
		t.Errorf("hostRoot = %q, want %q", handler.hostRoot, "/host")
	}
	if handler.writer != writer {
		t.Error("writer not set correctly")
	}
	if handler.executor != executor {
		t.Error("executor not set correctly")
	}
}

func TestHandleReboot_NotRequired(t *testing.T) {
	writer := &mockNodeWriter{}
	executor := &mockExecutor{}
	handler := NewHandler(t.TempDir(), writer, executor)

	rmc := &mcov1alpha1.RenderedMachineConfig{
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Reboot: mcov1alpha1.RenderedRebootSpec{
				Required: false,
			},
		},
	}
	node := &corev1.Node{}

	err := handler.HandleReboot(context.Background(), rmc, node)

	if err != nil {
		t.Fatalf("HandleReboot() error = %v", err)
	}
	if executor.called {
		t.Error("executor was called when reboot not required")
	}
	if writer.rebootPending != nil {
		t.Error("reboot-pending was set when reboot not required")
	}
}

func TestHandleReboot_ForceReboot(t *testing.T) {
	writer := &mockNodeWriter{}
	executor := &mockExecutor{}
	handler := NewHandler(t.TempDir(), writer, executor)

	rmc := &mcov1alpha1.RenderedMachineConfig{
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Reboot: mcov1alpha1.RenderedRebootSpec{
				Required: true,
				Strategy: "Never", // Force-reboot should bypass this
			},
		},
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				annotations.ForceReboot: "true",
			},
		},
	}

	err := handler.HandleReboot(context.Background(), rmc, node)

	if err != nil {
		t.Fatalf("HandleReboot() error = %v", err)
	}
	if !executor.called {
		t.Error("executor was not called for force-reboot")
	}
	if !writer.forceCleared {
		t.Error("force-reboot annotation was not cleared")
	}
}

func TestHandleReboot_StrategyNever(t *testing.T) {
	writer := &mockNodeWriter{}
	executor := &mockExecutor{}
	handler := NewHandler(t.TempDir(), writer, executor)

	rmc := &mcov1alpha1.RenderedMachineConfig{
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Reboot: mcov1alpha1.RenderedRebootSpec{
				Required: true,
				Strategy: "Never",
			},
		},
	}
	node := &corev1.Node{}

	err := handler.HandleReboot(context.Background(), rmc, node)

	if err != nil {
		t.Fatalf("HandleReboot() error = %v", err)
	}
	if executor.called {
		t.Error("executor was called with Never strategy")
	}
	if writer.rebootPending == nil || !*writer.rebootPending {
		t.Error("reboot-pending was not set to true")
	}
}

func TestHandleReboot_StrategyEmpty(t *testing.T) {
	writer := &mockNodeWriter{}
	executor := &mockExecutor{}
	handler := NewHandler(t.TempDir(), writer, executor)

	rmc := &mcov1alpha1.RenderedMachineConfig{
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Reboot: mcov1alpha1.RenderedRebootSpec{
				Required: true,
				Strategy: "", // Should default to Never
			},
		},
	}
	node := &corev1.Node{}

	err := handler.HandleReboot(context.Background(), rmc, node)

	if err != nil {
		t.Fatalf("HandleReboot() error = %v", err)
	}
	if executor.called {
		t.Error("executor was called with empty strategy (should default to Never)")
	}
	if writer.rebootPending == nil || !*writer.rebootPending {
		t.Error("reboot-pending was not set to true")
	}
}

func TestHandleReboot_StrategyUnknown(t *testing.T) {
	writer := &mockNodeWriter{}
	executor := &mockExecutor{}
	handler := NewHandler(t.TempDir(), writer, executor)

	rmc := &mcov1alpha1.RenderedMachineConfig{
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Reboot: mcov1alpha1.RenderedRebootSpec{
				Required: true,
				Strategy: "Unknown",
			},
		},
	}
	node := &corev1.Node{}

	err := handler.HandleReboot(context.Background(), rmc, node)

	if err != nil {
		t.Fatalf("HandleReboot() error = %v", err)
	}
	if executor.called {
		t.Error("executor was called with unknown strategy")
	}
	if writer.rebootPending == nil || !*writer.rebootPending {
		t.Error("reboot-pending was not set to true for unknown strategy")
	}
}

func TestHandleReboot_IfRequired_NoLastReboot(t *testing.T) {
	writer := &mockNodeWriter{}
	executor := &mockExecutor{}
	// Use empty temp dir (no last-reboot file)
	handler := NewHandler(t.TempDir(), writer, executor)

	rmc := &mcov1alpha1.RenderedMachineConfig{
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Reboot: mcov1alpha1.RenderedRebootSpec{
				Required:           true,
				Strategy:           "IfRequired",
				MinIntervalSeconds: 1800,
			},
		},
	}
	node := &corev1.Node{}

	err := handler.HandleReboot(context.Background(), rmc, node)

	if err != nil {
		t.Fatalf("HandleReboot() error = %v", err)
	}
	if !executor.called {
		t.Error("executor was not called (first boot should reboot)")
	}
}

func TestHandleReboot_IfRequired_IntervalElapsed(t *testing.T) {
	writer := &mockNodeWriter{}
	executor := &mockExecutor{}
	hostRoot := t.TempDir()
	handler := NewHandler(hostRoot, writer, executor)

	// Write last reboot time 1 hour ago
	handler.state.WriteLastRebootTime(time.Now().Add(-1 * time.Hour))

	rmc := &mcov1alpha1.RenderedMachineConfig{
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Reboot: mcov1alpha1.RenderedRebootSpec{
				Required:           true,
				Strategy:           "IfRequired",
				MinIntervalSeconds: 1800, // 30 minutes
			},
		},
	}
	node := &corev1.Node{}

	err := handler.HandleReboot(context.Background(), rmc, node)

	if err != nil {
		t.Fatalf("HandleReboot() error = %v", err)
	}
	if !executor.called {
		t.Error("executor was not called (interval elapsed)")
	}
}

func TestHandleReboot_IfRequired_IntervalNotElapsed(t *testing.T) {
	writer := &mockNodeWriter{}
	executor := &mockExecutor{}
	hostRoot := t.TempDir()
	handler := NewHandler(hostRoot, writer, executor)

	// Write last reboot time 10 minutes ago
	handler.state.WriteLastRebootTime(time.Now().Add(-10 * time.Minute))

	rmc := &mcov1alpha1.RenderedMachineConfig{
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Reboot: mcov1alpha1.RenderedRebootSpec{
				Required:           true,
				Strategy:           "IfRequired",
				MinIntervalSeconds: 1800, // 30 minutes
			},
		},
	}
	node := &corev1.Node{}

	err := handler.HandleReboot(context.Background(), rmc, node)

	if err != nil {
		t.Fatalf("HandleReboot() error = %v", err)
	}
	if executor.called {
		t.Error("executor was called (interval not elapsed)")
	}
	if writer.rebootPending == nil || !*writer.rebootPending {
		t.Error("reboot-pending was not set to true")
	}
}

func TestHandleReboot_IfRequired_ZeroInterval(t *testing.T) {
	writer := &mockNodeWriter{}
	executor := &mockExecutor{}
	hostRoot := t.TempDir()
	handler := NewHandler(hostRoot, writer, executor)

	// Write last reboot time 1 second ago
	handler.state.WriteLastRebootTime(time.Now().Add(-1 * time.Second))

	rmc := &mcov1alpha1.RenderedMachineConfig{
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Reboot: mcov1alpha1.RenderedRebootSpec{
				Required:           true,
				Strategy:           "IfRequired",
				MinIntervalSeconds: 0, // No minimum
			},
		},
	}
	node := &corev1.Node{}

	err := handler.HandleReboot(context.Background(), rmc, node)

	if err != nil {
		t.Fatalf("HandleReboot() error = %v", err)
	}
	if !executor.called {
		t.Error("executor was not called (zero interval should always reboot)")
	}
}

func TestExecuteReboot_SetsState(t *testing.T) {
	writer := &mockNodeWriter{}
	executor := &mockExecutor{}
	handler := NewHandler(t.TempDir(), writer, executor)

	err := handler.executeReboot(context.Background())

	if err != nil {
		t.Fatalf("executeReboot() error = %v", err)
	}
	if writer.state != "rebooting" {
		t.Errorf("state = %q, want %q", writer.state, "rebooting")
	}
}

func TestExecuteReboot_ClearsAnnotations(t *testing.T) {
	writer := &mockNodeWriter{}
	executor := &mockExecutor{}
	handler := NewHandler(t.TempDir(), writer, executor)

	err := handler.executeReboot(context.Background())

	if err != nil {
		t.Fatalf("executeReboot() error = %v", err)
	}
	if !writer.forceCleared {
		t.Error("force-reboot was not cleared")
	}
	if writer.rebootPending == nil || *writer.rebootPending {
		t.Error("reboot-pending was not set to false")
	}
}

func TestExecuteReboot_WritesLastRebootTime(t *testing.T) {
	writer := &mockNodeWriter{}
	executor := &mockExecutor{}
	hostRoot := t.TempDir()
	handler := NewHandler(hostRoot, writer, executor)

	before := time.Now().Add(-1 * time.Second)
	err := handler.executeReboot(context.Background())
	after := time.Now().Add(1 * time.Second)

	if err != nil {
		t.Fatalf("executeReboot() error = %v", err)
	}

	lastReboot, err := handler.state.ReadLastRebootTime()
	if err != nil {
		t.Fatalf("ReadLastRebootTime() error = %v", err)
	}

	if lastReboot.Before(before) || lastReboot.After(after) {
		t.Errorf("last reboot time %v not between %v and %v", lastReboot, before, after)
	}
}

func TestCheckRebootPendingOnStartup_NotPending(t *testing.T) {
	writer := &mockNodeWriter{}
	executor := &mockExecutor{}
	handler := NewHandler(t.TempDir(), writer, executor)

	node := &corev1.Node{} // No reboot-pending annotation

	err := handler.CheckRebootPendingOnStartup(context.Background(), node)

	if err != nil {
		t.Fatalf("CheckRebootPendingOnStartup() error = %v", err)
	}
	if writer.rebootPending != nil {
		t.Error("reboot-pending was modified when not set")
	}
}

func TestCheckRebootPendingOnStartup_NoRecordedTime(t *testing.T) {
	writer := &mockNodeWriter{}
	executor := &mockExecutor{}
	handler := NewHandler(t.TempDir(), writer, executor)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				annotations.RebootPending: "true",
			},
		},
	}

	err := handler.CheckRebootPendingOnStartup(context.Background(), node)

	if err != nil {
		t.Fatalf("CheckRebootPendingOnStartup() error = %v", err)
	}
	// Should not modify annotation when no recorded time
	if writer.rebootPending != nil {
		t.Error("reboot-pending was modified when no recorded time exists")
	}
}

func TestCheckRebootPendingOnStartup_ClearsPending(t *testing.T) {
	writer := &mockNodeWriter{}
	executor := &mockExecutor{}
	hostRoot := t.TempDir()
	handler := NewHandler(hostRoot, writer, executor)

	// Write last reboot time (simulating a reboot request was made)
	handler.state.WriteLastRebootTime(time.Now())

	// Do NOT create boot marker (simulating system rebooted and /run was cleared)
	// The boot marker in /run/mco/boot-marker should not exist

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				annotations.RebootPending:   "true",
				annotations.DesiredRevision: "worker-abc123",
			},
		},
	}

	err := handler.CheckRebootPendingOnStartup(context.Background(), node)

	if err != nil {
		t.Fatalf("CheckRebootPendingOnStartup() error = %v", err)
	}
	// Should clear pending since boot marker doesn't exist (indicates reboot happened)
	if writer.rebootPending == nil || *writer.rebootPending {
		t.Error("reboot-pending was not cleared after detected reboot")
	}

	// Should update current-revision to desired-revision
	if writer.currentRevision != "worker-abc123" {
		t.Errorf("current-revision = %q, want %q", writer.currentRevision, "worker-abc123")
	}

	// Verify boot marker was created after check
	if !handler.state.BootMarkerExists() {
		t.Error("boot marker was not created after startup check")
	}
}

func TestCheckRebootPendingOnStartup_NoRebootDetected(t *testing.T) {
	writer := &mockNodeWriter{}
	executor := &mockExecutor{}
	hostRoot := t.TempDir()
	handler := NewHandler(hostRoot, writer, executor)

	// Write last reboot time (simulating a reboot request was made)
	handler.state.WriteLastRebootTime(time.Now())

	// Create boot marker (simulating agent restarted but system didn't reboot)
	handler.state.WriteBootMarker()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				annotations.RebootPending: "true",
			},
		},
	}

	err := handler.CheckRebootPendingOnStartup(context.Background(), node)

	if err != nil {
		t.Fatalf("CheckRebootPendingOnStartup() error = %v", err)
	}
	// Should NOT clear pending since boot marker exists (indicates no reboot)
	if writer.rebootPending != nil {
		t.Error("reboot-pending was modified when no reboot was detected")
	}
}

func TestExecuteReboot_ContinuesOnErrors(t *testing.T) {
	// Test that executeReboot continues even when writer methods fail
	writer := &mockNodeWriter{
		setStateErr:   fmt.Errorf("state error"),
		clearForceErr: fmt.Errorf("clear error"),
		setPendingErr: fmt.Errorf("pending error"),
	}
	executor := &mockExecutor{}
	handler := NewHandler(t.TempDir(), writer, executor)

	err := handler.executeReboot(context.Background())

	// Should still call executor despite writer errors
	if err != nil {
		t.Fatalf("executeReboot() error = %v", err)
	}
	if !executor.called {
		t.Error("executor was not called despite writer errors")
	}
}
