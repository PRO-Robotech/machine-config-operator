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
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/internal/agent/reboot"
	"in-cloud.io/machine-config/pkg/annotations"
	mcoclient "in-cloud.io/machine-config/pkg/client"
)

// mockRMCGetter implements mcoclient.RMCGetter for testing.
type mockRMCGetter struct {
	rmcs      map[string]*mcov1alpha1.RenderedMachineConfig
	getError  error
	callCount int
}

func (m *mockRMCGetter) Get(ctx context.Context, name string, opts metav1.GetOptions) (*mcov1alpha1.RenderedMachineConfig, error) {
	m.callCount++
	if m.getError != nil {
		return nil, m.getError
	}
	rmc, ok := m.rmcs[name]
	if !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: "mco.in-cloud.io", Resource: "renderedmachineconfigs"}, name)
	}
	return rmc, nil
}

// mockMCOClient implements mcoclient.MCOClient for testing.
type mockMCOClient struct {
	rmcGetter *mockRMCGetter
}

func (m *mockMCOClient) RenderedMachineConfigs() mcoclient.RMCGetter {
	return m.rmcGetter
}

func newMockMCOClient() *mockMCOClient {
	return &mockMCOClient{
		rmcGetter: &mockRMCGetter{
			rmcs: make(map[string]*mcov1alpha1.RenderedMachineConfig),
		},
	}
}

func (m *mockMCOClient) addRMC(rmc *mcov1alpha1.RenderedMachineConfig) {
	m.rmcGetter.rmcs[rmc.Name] = rmc
}

func (m *mockMCOClient) setGetError(err error) {
	m.rmcGetter.getError = err
}

// newTestAgent creates an Agent with all required fields for testing.
// Uses NoOpExecutor for reboot handler to avoid actual reboots.
func newTestAgent(nodeName string, k8sClient kubernetes.Interface, mcoClient mcoclient.MCOClient) *Agent {
	writer := NewNodeWriter(k8sClient, nodeName)
	executor := &reboot.NoOpExecutor{}
	rebootHandler := reboot.NewHandler("", writer, executor)
	rmcCache := NewRMCCache(DefaultRMCCacheTTL)

	agent := &Agent{
		nodeName:      nodeName,
		k8sClient:     k8sClient,
		mcoClient:     mcoClient,
		writer:        writer,
		applier:       NewApplierWithOptions("", NewMockConnection(), true),
		rebootHandler: rebootHandler,
		rmcCache:      rmcCache,
	}
	// Initialize rebootDeterminer with agent as the RMCFetcher
	agent.rebootDeterminer = NewRebootDeterminer(agent)
	return agent
}

func TestNew_Validation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing node name",
			cfg:     Config{},
			wantErr: true,
			errMsg:  "node name is required",
		},
		{
			name: "missing k8s client",
			cfg: Config{
				NodeName: "test-node",
			},
			wantErr: true,
			errMsg:  "kubernetes client is required",
		},
		{
			name: "missing mco client",
			cfg: Config{
				NodeName:  "test-node",
				K8sClient: fake.NewSimpleClientset(),
			},
			wantErr: true,
			errMsg:  "MCO client is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("New() error = %v, wantErr %v", err, tt.errMsg)
			}
		})
	}
}

func TestAgent_HandleNodeUpdate_Paused(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				annotations.Paused:          "true",
				annotations.DesiredRevision: "new-rev",
				annotations.CurrentRevision: "old-rev",
			},
		},
	}
	k8sClient := fake.NewSimpleClientset(node)
	mcoClient := newMockMCOClient()

	// Add RMC that should NOT be fetched (because node is paused)
	mcoClient.addRMC(&mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "new-rev"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Config: mcov1alpha1.RenderedConfig{
				Files: []mcov1alpha1.FileSpec{{Path: "/etc/test.conf", Content: "test"}},
			},
		},
	})

	agent := newTestAgent("test-node", k8sClient, mcoClient)

	err := agent.handleNodeUpdate(context.Background(), node)
	if err != nil {
		t.Fatalf("handleNodeUpdate() error = %v", err)
	}

	// Verify RMC was NOT fetched (paused node should skip)
	if mcoClient.rmcGetter.callCount != 0 {
		t.Errorf("RMC Get called %d times, want 0 (paused)", mcoClient.rmcGetter.callCount)
	}
}

func TestAgent_HandleNodeUpdate_SameRevision(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				annotations.DesiredRevision: "same-rev",
				annotations.CurrentRevision: "same-rev",
			},
		},
	}
	k8sClient := fake.NewSimpleClientset(node)
	mcoClient := newMockMCOClient()

	agent := newTestAgent("test-node", k8sClient, mcoClient)

	err := agent.handleNodeUpdate(context.Background(), node)
	if err != nil {
		t.Fatalf("handleNodeUpdate() error = %v", err)
	}

	// Verify RMC was NOT fetched (already at same revision)
	if mcoClient.rmcGetter.callCount != 0 {
		t.Errorf("RMC Get called %d times, want 0 (same revision)", mcoClient.rmcGetter.callCount)
	}
}

func TestAgent_HandleNodeUpdate_NoDesiredRevision(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				annotations.CurrentRevision: "old-rev",
			},
		},
	}
	k8sClient := fake.NewSimpleClientset(node)
	mcoClient := newMockMCOClient()

	agent := newTestAgent("test-node", k8sClient, mcoClient)

	err := agent.handleNodeUpdate(context.Background(), node)
	if err != nil {
		t.Fatalf("handleNodeUpdate() error = %v", err)
	}

	// Verify RMC was NOT fetched (no desired revision)
	if mcoClient.rmcGetter.callCount != 0 {
		t.Errorf("RMC Get called %d times, want 0 (no desired)", mcoClient.rmcGetter.callCount)
	}
}

func TestAgent_HandleNodeUpdate_RebootPending(t *testing.T) {
	// When reboot-pending=true, agent should NOT re-apply config
	// but SHOULD fetch RMC to check if minInterval has elapsed
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				annotations.DesiredRevision: "new-rev",
				annotations.CurrentRevision: "old-rev",
				annotations.RebootPending:   "true", // Reboot is pending
			},
		},
	}
	k8sClient := fake.NewSimpleClientset(node)
	mcoClient := newMockMCOClient()

	// Add RMC so fetch succeeds
	mcoClient.addRMC(&mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "new-rev"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Reboot: mcov1alpha1.RenderedRebootSpec{Required: true},
		},
	})

	agent := newTestAgent("test-node", k8sClient, mcoClient)

	err := agent.handleNodeUpdate(context.Background(), node)
	if err != nil {
		t.Fatalf("handleNodeUpdate() error = %v", err)
	}

	// Verify RMC WAS fetched (to check reboot interval)
	if mcoClient.rmcGetter.callCount != 1 {
		t.Errorf("RMC Get called %d times, want 1 (reboot pending check)", mcoClient.rmcGetter.callCount)
	}
}

func TestAgent_HandleNodeUpdate_PendingRebootRevision(t *testing.T) {
	// When agent has pendingRebootRevision set, it should fetch RMC
	// to check if minInterval has elapsed, but NOT re-apply config
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				annotations.DesiredRevision: "new-rev",
				annotations.CurrentRevision: "old-rev",
				// Note: reboot-pending is NOT set in this stale event
			},
		},
	}
	k8sClient := fake.NewSimpleClientset(node)
	mcoClient := newMockMCOClient()

	// Add RMC so fetch succeeds
	mcoClient.addRMC(&mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "new-rev"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Reboot: mcov1alpha1.RenderedRebootSpec{Required: true},
		},
	})

	agent := newTestAgent("test-node", k8sClient, mcoClient)
	agent.pendingRebootRevision = "new-rev" // Already applied, waiting for reboot

	err := agent.handleNodeUpdate(context.Background(), node)
	if err != nil {
		t.Fatalf("handleNodeUpdate() error = %v", err)
	}

	// Verify RMC WAS fetched (to check reboot interval)
	if mcoClient.rmcGetter.callCount != 1 {
		t.Errorf("RMC Get called %d times, want 1 (pending reboot revision check)", mcoClient.rmcGetter.callCount)
	}
}

func TestAgent_HandleNodeUpdate_NewRevision(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				annotations.DesiredRevision: "new-rev",
				annotations.CurrentRevision: "old-rev",
			},
		},
	}
	k8sClient := fake.NewSimpleClientset(node)
	mcoClient := newMockMCOClient()

	// Add RMC that will be applied
	mcoClient.addRMC(&mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "new-rev"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Config: mcov1alpha1.RenderedConfig{
				Files:   []mcov1alpha1.FileSpec{},
				Systemd: mcov1alpha1.SystemdSpec{Units: []mcov1alpha1.UnitSpec{}},
			},
		},
	})

	agent := newTestAgent("test-node", k8sClient, mcoClient)

	err := agent.handleNodeUpdate(context.Background(), node)
	if err != nil {
		t.Fatalf("handleNodeUpdate() error = %v", err)
	}

	// Verify RMC was fetched (at least once for apply, possibly twice for diff)
	if mcoClient.rmcGetter.callCount < 1 {
		t.Errorf("RMC Get called %d times, want at least 1", mcoClient.rmcGetter.callCount)
	}

	// Verify state transitions
	updated, _ := k8sClient.CoreV1().Nodes().Get(context.Background(), "test-node", metav1.GetOptions{})
	if got := updated.Annotations[annotations.AgentState]; got != annotations.StateDone {
		t.Errorf("AgentState = %q, want %q", got, annotations.StateDone)
	}
	if got := updated.Annotations[annotations.CurrentRevision]; got != "new-rev" {
		t.Errorf("CurrentRevision = %q, want %q", got, "new-rev")
	}
}

func TestAgent_HandleNodeUpdate_RMCNotFound(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				annotations.DesiredRevision: "missing-rev",
				annotations.CurrentRevision: "old-rev",
			},
		},
	}
	k8sClient := fake.NewSimpleClientset(node)
	mcoClient := newMockMCOClient()
	// Don't add RMC - it will be "not found"

	agent := newTestAgent("test-node", k8sClient, mcoClient)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := agent.handleNodeUpdate(ctx, node)
	if err == nil {
		t.Fatal("handleNodeUpdate() expected error for missing RMC")
	}

	// Verify state is error
	updated, _ := k8sClient.CoreV1().Nodes().Get(context.Background(), "test-node", metav1.GetOptions{})
	if got := updated.Annotations[annotations.AgentState]; got != annotations.StateError {
		t.Errorf("AgentState = %q, want %q", got, annotations.StateError)
	}
}

func TestAgent_HandleNodeUpdate_PermanentError(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				annotations.DesiredRevision: "some-rev",
				annotations.CurrentRevision: "old-rev",
			},
		},
	}
	k8sClient := fake.NewSimpleClientset(node)
	mcoClient := newMockMCOClient()
	mcoClient.setGetError(errors.New("permanent error"))

	agent := newTestAgent("test-node", k8sClient, mcoClient)

	err := agent.handleNodeUpdate(context.Background(), node)
	if err == nil {
		t.Fatal("handleNodeUpdate() expected error for permanent error")
	}

	// Verify state is error
	updated, _ := k8sClient.CoreV1().Nodes().Get(context.Background(), "test-node", metav1.GetOptions{})
	if got := updated.Annotations[annotations.AgentState]; got != annotations.StateError {
		t.Errorf("AgentState = %q, want %q", got, annotations.StateError)
	}
}

func TestAgent_HandleNodeUpdate_RebootRequired(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				annotations.DesiredRevision: "new-rev",
				annotations.CurrentRevision: "old-rev",
			},
		},
	}
	k8sClient := fake.NewSimpleClientset(node)
	mcoClient := newMockMCOClient()

	// Add RMC that requires reboot
	mcoClient.addRMC(&mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "new-rev"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			Config: mcov1alpha1.RenderedConfig{
				Files:   []mcov1alpha1.FileSpec{},
				Systemd: mcov1alpha1.SystemdSpec{Units: []mcov1alpha1.UnitSpec{}},
			},
			Reboot: mcov1alpha1.RenderedRebootSpec{
				Required: true,
			},
		},
	})

	// Use newTestAgent which includes rebootHandler
	agent := newTestAgent("test-node", k8sClient, mcoClient)

	err := agent.handleNodeUpdate(context.Background(), node)
	if err != nil {
		t.Fatalf("handleNodeUpdate() error = %v", err)
	}

	// Verify reboot-pending is set (strategy defaults to Never)
	updated, _ := k8sClient.CoreV1().Nodes().Get(context.Background(), "test-node", metav1.GetOptions{})
	if got := updated.Annotations[annotations.RebootPending]; got != "true" {
		t.Errorf("RebootPending = %q, want %q", got, "true")
	}

	// Current revision should NOT be updated yet (waiting for reboot)
	if got := updated.Annotations[annotations.CurrentRevision]; got == "new-rev" {
		t.Error("CurrentRevision should not be updated before reboot")
	}
}

func TestAgent_GetNodeName(t *testing.T) {
	agent := &Agent{nodeName: "my-node"}
	if got := agent.GetNodeName(); got != "my-node" {
		t.Errorf("GetNodeName() = %q, want %q", got, "my-node")
	}
}

// TestAgent_FetchRMC tests the RMCFetcher interface implementation.
// STORY-064: Tests for diff-based reboot RMC fetching.
func TestAgent_FetchRMC_CacheHit(t *testing.T) {
	k8sClient := fake.NewSimpleClientset()
	mcoClient := newMockMCOClient()

	agent := newTestAgent("test-node", k8sClient, mcoClient)

	// Pre-populate cache
	rmc := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "cached-rev"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			PoolName: "workers",
		},
	}
	agent.rmcCache.Set("cached-rev", rmc)

	// Fetch should hit cache
	got, err := agent.FetchRMC(context.Background(), "cached-rev")
	if err != nil {
		t.Fatalf("FetchRMC() error = %v", err)
	}
	if got.Name != "cached-rev" {
		t.Errorf("FetchRMC() name = %q, want %q", got.Name, "cached-rev")
	}

	// Verify API was NOT called
	if mcoClient.rmcGetter.callCount != 0 {
		t.Errorf("API called %d times, want 0 (cache hit)", mcoClient.rmcGetter.callCount)
	}
}

func TestAgent_FetchRMC_CacheMiss(t *testing.T) {
	k8sClient := fake.NewSimpleClientset()
	mcoClient := newMockMCOClient()

	// Add RMC to API
	rmc := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "new-rev"},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			PoolName: "workers",
		},
	}
	mcoClient.addRMC(rmc)

	agent := newTestAgent("test-node", k8sClient, mcoClient)

	// Fetch should miss cache, call API
	got, err := agent.FetchRMC(context.Background(), "new-rev")
	if err != nil {
		t.Fatalf("FetchRMC() error = %v", err)
	}
	if got.Name != "new-rev" {
		t.Errorf("FetchRMC() name = %q, want %q", got.Name, "new-rev")
	}

	// Verify API was called
	if mcoClient.rmcGetter.callCount != 1 {
		t.Errorf("API called %d times, want 1", mcoClient.rmcGetter.callCount)
	}

	// Verify item was cached
	cached := agent.rmcCache.Get("new-rev")
	if cached == nil {
		t.Error("Expected RMC to be cached after fetch")
	}
}

func TestAgent_FetchRMC_NotFound(t *testing.T) {
	k8sClient := fake.NewSimpleClientset()
	mcoClient := newMockMCOClient()
	// Don't add RMC - it will be not found

	agent := newTestAgent("test-node", k8sClient, mcoClient)

	_, err := agent.FetchRMC(context.Background(), "missing-rev")
	if err == nil {
		t.Fatal("FetchRMC() expected error for missing RMC")
	}

	// Error should indicate not found
	if !errors.Is(err, nil) && err.Error() == "" {
		t.Errorf("Expected non-empty error message")
	}
}

func TestAgent_FetchRMC_APIError(t *testing.T) {
	k8sClient := fake.NewSimpleClientset()
	mcoClient := newMockMCOClient()
	mcoClient.setGetError(errors.New("API server unavailable"))

	agent := newTestAgent("test-node", k8sClient, mcoClient)

	_, err := agent.FetchRMC(context.Background(), "some-rev")
	if err == nil {
		t.Fatal("FetchRMC() expected error for API failure")
	}

	// Verify item was NOT cached on error
	cached := agent.rmcCache.Get("some-rev")
	if cached != nil {
		t.Error("Expected RMC NOT to be cached after fetch error")
	}
}
