package controller

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"in-cloud.io/machine-config/pkg/annotations"
	"in-cloud.io/machine-config/pkg/drain"
)

// testMCONamespace is the MCO namespace used in tests.
const testMCONamespace = "machine-config-system"

func TestFilterEvictablePods_SkipsCompletedPods(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "running"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "succeeded"},
			Status:     corev1.PodStatus{Phase: corev1.PodSucceeded},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "failed"},
			Status:     corev1.PodStatus{Phase: corev1.PodFailed},
		},
	}

	result := FilterEvictablePods(pods, DrainOptions{DeleteOrphans: true}, testMCONamespace)

	if len(result) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(result))
	}
	if result[0].Name != "running" {
		t.Errorf("expected running pod, got %s", result[0].Name)
	}
}

func TestFilterEvictablePods_SkipsMirrorPods(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "regular"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "mirror",
				Annotations: map[string]string{"kubernetes.io/config.mirror": "abc123"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	result := FilterEvictablePods(pods, DrainOptions{DeleteOrphans: true}, testMCONamespace)

	if len(result) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(result))
	}
	if result[0].Name != "regular" {
		t.Errorf("expected regular pod, got %s", result[0].Name)
	}
}

func TestFilterEvictablePods_SkipsDaemonSetPods(t *testing.T) {
	controller := true
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "deployment-pod",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "ReplicaSet", Controller: &controller},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "daemonset-pod",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "DaemonSet", Controller: &controller},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	result := FilterEvictablePods(pods, DrainOptions{IgnoreDS: true, DeleteOrphans: true}, testMCONamespace)

	if len(result) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(result))
	}
	if result[0].Name != "deployment-pod" {
		t.Errorf("expected deployment-pod, got %s", result[0].Name)
	}
}

func TestFilterEvictablePods_IncludesDaemonSetPodsWhenNotIgnored(t *testing.T) {
	controller := true
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "daemonset-pod",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "DaemonSet", Controller: &controller},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	result := FilterEvictablePods(pods, DrainOptions{IgnoreDS: false, DeleteOrphans: true}, testMCONamespace)

	if len(result) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(result))
	}
}

func TestFilterEvictablePods_SkipsOrphanPods(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "orphan"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	result := FilterEvictablePods(pods, DrainOptions{DeleteOrphans: false}, testMCONamespace)

	if len(result) != 0 {
		t.Fatalf("expected 0 pods, got %d", len(result))
	}
}

func TestFilterEvictablePods_IncludesOrphanPodsWhenAllowed(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "orphan"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	result := FilterEvictablePods(pods, DrainOptions{DeleteOrphans: true}, testMCONamespace)

	if len(result) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(result))
	}
}

func TestFilterEvictablePodsWithExclusions_SkipsByRule(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "nginx", Namespace: "default"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "netshoot-abc", Namespace: "default"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	exclusions := &drain.DrainConfig{
		Rules: []drain.Rule{
			{Namespaces: []string{"kube-system"}},
			{PodNamePatterns: []string{"netshoot-*"}},
		},
	}

	result := FilterEvictablePodsWithExclusions(pods, DrainOptions{DeleteOrphans: true}, exclusions, testMCONamespace)

	if len(result) != 1 {
		t.Fatalf("expected 1 pod (nginx), got %d", len(result))
	}
	if result[0].Name != "nginx" {
		t.Errorf("expected nginx, got %s", result[0].Name)
	}
}

func TestFilterEvictablePodsWithExclusions_SkipsToleratAll(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "nginx", Namespace: "default"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "netshoot", Namespace: "default"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
			Spec: corev1.PodSpec{
				Tolerations: []corev1.Toleration{
					{Operator: corev1.TolerationOpExists},
				},
			},
		},
	}

	exclusions := &drain.DrainConfig{
		Defaults: drain.Defaults{
			SkipToleratAllPods: true,
		},
	}

	result := FilterEvictablePodsWithExclusions(pods, DrainOptions{DeleteOrphans: true}, exclusions, testMCONamespace)

	if len(result) != 1 {
		t.Fatalf("expected 1 pod (nginx), got %d", len(result))
	}
	if result[0].Name != "nginx" {
		t.Errorf("expected nginx, got %s", result[0].Name)
	}
}

func TestFilterEvictablePodsWithExclusions_NilExclusions(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "nginx", Namespace: "default"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	// nil exclusions should work (no exclusions applied)
	result := FilterEvictablePodsWithExclusions(pods, DrainOptions{DeleteOrphans: true}, nil, testMCONamespace)

	if len(result) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(result))
	}
}

func TestIsDaemonSetPod(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected bool
	}{
		{
			name:     "no owner references",
			pod:      &corev1.Pod{},
			expected: false,
		},
		{
			name: "replicaset owner",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet"}},
				},
			},
			expected: false,
		},
		{
			name: "daemonset owner",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{{Kind: "DaemonSet"}},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsDaemonSetPod(tt.pod)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestHasController(t *testing.T) {
	controller := true
	notController := false

	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected bool
	}{
		{
			name:     "no owner references",
			pod:      &corev1.Pod{},
			expected: false,
		},
		{
			name: "has controller",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "ReplicaSet", Controller: &controller},
					},
				},
			},
			expected: true,
		},
		{
			name: "no controller flag",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "ReplicaSet", Controller: &notController},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasController(tt.pod)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestHandleDrainRetry_Phase1(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	drainStart := time.Now().Add(-5 * time.Minute)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				annotations.DrainStartedAt: drainStart.Format(time.RFC3339),
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	ctx := context.Background()

	// With default timeout (3600s) and auto-calculated retry (300s = 5min)
	result := HandleDrainRetry(ctx, c, node, DefaultDrainTimeoutSeconds, 0)

	// Auto-calculated retry interval: max(30, 3600/12) = 300s = 5min
	if result.RequeueAfter != 5*time.Minute {
		t.Errorf("expected 5 minute requeue (auto-calculated), got %v", result.RequeueAfter)
	}
	if result.SetDrainStuck {
		t.Error("expected SetDrainStuck to be false")
	}
}

func TestHandleDrainRetry_Phase2(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	drainStart := time.Now().Add(-30 * time.Minute)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				annotations.DrainStartedAt: drainStart.Format(time.RFC3339),
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	ctx := context.Background()

	// Auto-calculated retry: max(30, 3600/12) = 300s = 5min
	result := HandleDrainRetry(ctx, c, node, DefaultDrainTimeoutSeconds, 0)

	// Remaining = 3600-1800 = 1800s = 30min, so requeue = min(300s, 1800s) = 300s = 5min
	if result.RequeueAfter != 5*time.Minute {
		t.Errorf("expected 5 minute requeue, got %v", result.RequeueAfter)
	}
	if result.SetDrainStuck {
		t.Error("expected SetDrainStuck to be false")
	}
}

func TestHandleDrainRetry_Phase3_DrainStuck(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	drainStart := time.Now().Add(-65 * time.Minute)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				annotations.DrainStartedAt: drainStart.Format(time.RFC3339),
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	ctx := context.Background()

	// 65 min > 60 min timeout → drain stuck
	result := HandleDrainRetry(ctx, c, node, DefaultDrainTimeoutSeconds, 0)

	// Auto-calculated retry interval: max(30, 3600/12) = 300s = 5min
	if result.RequeueAfter != 5*time.Minute {
		t.Errorf("expected 5 minute requeue, got %v", result.RequeueAfter)
	}
	if !result.SetDrainStuck {
		t.Error("expected SetDrainStuck to be true")
	}
}

func TestHandleDrainRetry_NoDrainStarted(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	ctx := context.Background()

	// No drain started yet → first attempt uses auto-calculated interval
	result := HandleDrainRetry(ctx, c, node, DefaultDrainTimeoutSeconds, 0)

	// Auto-calculated retry: max(30, 3600/12) = 300s = 5min
	if result.RequeueAfter != 5*time.Minute {
		t.Errorf("expected 5 minute requeue for first attempt (auto-calculated), got %v", result.RequeueAfter)
	}
	if result.SetDrainStuck {
		t.Error("expected SetDrainStuck to be false")
	}
}

func TestHandleDrainRetry_CustomTimeout(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Drain started 20 minutes ago
	drainStart := time.Now().Add(-20 * time.Minute)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				annotations.DrainStartedAt: drainStart.Format(time.RFC3339),
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	ctx := context.Background()

	// With default timeout (3600s), should not be stuck
	result := HandleDrainRetry(ctx, c, node, DefaultDrainTimeoutSeconds, 0)
	if result.SetDrainStuck {
		t.Error("expected SetDrainStuck to be false with default timeout")
	}

	// With 15 minute timeout, should be stuck (20min > 15min)
	result = HandleDrainRetry(ctx, c, node, 900, 0) // 15 minutes
	if !result.SetDrainStuck {
		t.Error("expected SetDrainStuck to be true with 15 minute timeout")
	}
}

func TestHandleDrainRetry_ZeroTimeoutUsesDefault(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// 30 minutes into drain - should not be stuck with default timeout
	drainStart := time.Now().Add(-30 * time.Minute)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				annotations.DrainStartedAt: drainStart.Format(time.RFC3339),
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	ctx := context.Background()

	// Pass 0, 0 to use defaults for both timeout and retry
	result := HandleDrainRetry(ctx, c, node, 0, 0)
	if result.SetDrainStuck {
		t.Error("expected SetDrainStuck to be false when using default timeout")
	}
}

func TestPDBBlockedError(t *testing.T) {
	err := &PDBBlockedError{Pod: "test-pod", Err: nil}
	msg := err.Error()

	if msg != "PDB blocked eviction of pod test-pod: <nil>" {
		t.Errorf("unexpected error message: %s", msg)
	}
}

// TestHandleDrainRetry_ShortTimeout_60Seconds verifies that drainTimeoutSeconds=60
// is respected (regression test for hardcoded 10-minute check).
func TestHandleDrainRetry_ShortTimeout_60Seconds(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Drain started 70 seconds ago (> 60s timeout)
	drainStart := time.Now().Add(-70 * time.Second)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				annotations.DrainStartedAt: drainStart.Format(time.RFC3339),
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	ctx := context.Background()

	// With 60s timeout, should be stuck after 70s
	result := HandleDrainRetry(ctx, c, node, 60, 0)
	if !result.SetDrainStuck {
		t.Error("expected SetDrainStuck=true after 70s with 60s timeout")
	}

	// But at 50s, should NOT be stuck
	drainStart = time.Now().Add(-50 * time.Second)
	node.Annotations[annotations.DrainStartedAt] = drainStart.Format(time.RFC3339)
	_ = c.Update(ctx, node)

	result = HandleDrainRetry(ctx, c, node, 60, 0)
	if result.SetDrainStuck {
		t.Error("expected SetDrainStuck=false at 50s with 60s timeout")
	}
}

// TestHandleDrainRetry_ShortTimeout_120Seconds verifies drainTimeoutSeconds=120 works.
func TestHandleDrainRetry_ShortTimeout_120Seconds(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Drain started 130 seconds ago (> 120s timeout)
	drainStart := time.Now().Add(-130 * time.Second)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				annotations.DrainStartedAt: drainStart.Format(time.RFC3339),
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	ctx := context.Background()

	// With 120s timeout, should be stuck after 130s
	result := HandleDrainRetry(ctx, c, node, 120, 0)
	if !result.SetDrainStuck {
		t.Error("expected SetDrainStuck=true after 130s with 120s timeout")
	}

	// But at 110s, should NOT be stuck
	drainStart = time.Now().Add(-110 * time.Second)
	node.Annotations[annotations.DrainStartedAt] = drainStart.Format(time.RFC3339)
	_ = c.Update(ctx, node)

	result = HandleDrainRetry(ctx, c, node, 120, 0)
	if result.SetDrainStuck {
		t.Error("expected SetDrainStuck=false at 110s with 120s timeout")
	}
}

// TestHandleDrainRetry_ShortTimeout_300Seconds verifies drainTimeoutSeconds=300 (5min) works.
func TestHandleDrainRetry_ShortTimeout_300Seconds(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Drain started 310 seconds ago (> 300s timeout)
	drainStart := time.Now().Add(-310 * time.Second)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				annotations.DrainStartedAt: drainStart.Format(time.RFC3339),
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	ctx := context.Background()

	// With 300s (5min) timeout, should be stuck after 310s
	result := HandleDrainRetry(ctx, c, node, 300, 0)
	if !result.SetDrainStuck {
		t.Error("expected SetDrainStuck=true after 310s with 300s timeout")
	}

	// But at 290s, should NOT be stuck
	drainStart = time.Now().Add(-290 * time.Second)
	node.Annotations[annotations.DrainStartedAt] = drainStart.Format(time.RFC3339)
	_ = c.Update(ctx, node)

	result = HandleDrainRetry(ctx, c, node, 300, 0)
	if result.SetDrainStuck {
		t.Error("expected SetDrainStuck=false at 290s with 300s timeout")
	}
}

// TestHandleDrainRetry_TimeoutBoundary tests exact boundary conditions.
func TestHandleDrainRetry_TimeoutBoundary(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	tests := []struct {
		name           string
		elapsed        time.Duration
		timeoutSeconds int
		wantStuck      bool
	}{
		{"exactly at timeout", 300 * time.Second, 300, true},
		{"1 second before timeout", 299 * time.Second, 300, false},
		{"1 second after timeout", 301 * time.Second, 300, true},
		{"60s timeout exact", 60 * time.Second, 60, true},
		{"60s timeout minus 1", 59 * time.Second, 60, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drainStart := time.Now().Add(-tt.elapsed)
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
					Annotations: map[string]string{
						annotations.DrainStartedAt: drainStart.Format(time.RFC3339),
					},
				},
			}

			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
			result := HandleDrainRetry(context.Background(), c, node, tt.timeoutSeconds, 0)

			if result.SetDrainStuck != tt.wantStuck {
				t.Errorf("SetDrainStuck = %v, want %v", result.SetDrainStuck, tt.wantStuck)
			}
		})
	}
}

// TestHandleDrainRetry_ConfigurableRequeueInterval verifies configurable retry intervals.
func TestHandleDrainRetry_ConfigurableRequeueInterval(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	tests := []struct {
		name           string
		elapsed        time.Duration
		timeoutSeconds int
		retrySeconds   int
		wantRequeue    time.Duration
		wantStuck      bool
	}{
		// With 60s timeout and auto-calculated retry: max(30, 60/12) = 30s
		// remaining = 60-30 = 30s, so requeue = 30s
		{"60s timeout, 30s elapsed, auto retry", 30 * time.Second, 60, 0, 30 * time.Second, false},

		// With 60s timeout and custom 10s retry
		// remaining = 60-30 = 30s, requeue = min(10s, 30s) = 10s
		{"60s timeout, 30s elapsed, 10s retry", 30 * time.Second, 60, 10, 10 * time.Second, false},

		// With 300s timeout and auto-calculated retry: max(30, 300/12) = 30s
		// remaining = 300-30 = 270s, so requeue = 30s
		{"300s timeout, 30s elapsed, auto retry", 30 * time.Second, 300, 0, 30 * time.Second, false},

		// Near end: remaining < retryInterval, so cap to remaining (min 10s)
		// 300s timeout, 295s elapsed → remaining = 5s → cap to 10s (minimum)
		{"300s timeout, 295s elapsed (near end)", 295 * time.Second, 300, 0, 10 * time.Second, false},

		// Default timeout (3600s), auto retry: max(30, 3600/12) = 300s = 5min
		{"default timeout, 5min elapsed", 5 * time.Minute, DefaultDrainTimeoutSeconds, 0, 5 * time.Minute, false},

		// Default timeout, 30 min elapsed
		// remaining = 3600-1800 = 1800s, requeue = min(300s, 1800s) = 300s = 5min
		{"default timeout, 30min elapsed", 30 * time.Minute, DefaultDrainTimeoutSeconds, 0, 5 * time.Minute, false},

		// Custom retry of 60s with default timeout
		{"default timeout, 5min elapsed, 60s retry", 5 * time.Minute, DefaultDrainTimeoutSeconds, 60, 60 * time.Second, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drainStart := time.Now().Add(-tt.elapsed)
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
					Annotations: map[string]string{
						annotations.DrainStartedAt: drainStart.Format(time.RFC3339),
					},
				},
			}

			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
			result := HandleDrainRetry(context.Background(), c, node, tt.timeoutSeconds, tt.retrySeconds)

			if result.SetDrainStuck != tt.wantStuck {
				t.Errorf("SetDrainStuck = %v, want %v", result.SetDrainStuck, tt.wantStuck)
			}
			// Allow 5 second tolerance for timing variations
			diff := result.RequeueAfter - tt.wantRequeue
			if diff < 0 {
				diff = -diff
			}
			if diff > 5*time.Second {
				t.Errorf("RequeueAfter = %v, want %v (±5s)", result.RequeueAfter, tt.wantRequeue)
			}
		})
	}
}

// TestHandleDrainRetry_CustomRetryInterval verifies explicit drainRetrySeconds works.
func TestHandleDrainRetry_CustomRetryInterval(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	drainStart := time.Now().Add(-2 * time.Minute)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				annotations.DrainStartedAt: drainStart.Format(time.RFC3339),
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	ctx := context.Background()

	// With custom 20s retry interval
	result := HandleDrainRetry(ctx, c, node, 300, 20)

	// Should use the specified 20s interval
	if result.RequeueAfter != 20*time.Second {
		t.Errorf("expected 20s requeue with custom retry, got %v", result.RequeueAfter)
	}
	if result.SetDrainStuck {
		t.Error("expected SetDrainStuck to be false")
	}
}

func TestIsDrainComplete_NoPods(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(node).
		WithIndex(&corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string {
			pod := obj.(*corev1.Pod)
			return []string{pod.Spec.NodeName}
		}).
		Build()
	ctx := context.Background()

	complete, err := IsDrainComplete(ctx, c, node, DrainOptions{IgnoreDS: true, DeleteOrphans: true}, testMCONamespace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !complete {
		t.Error("expected drain to be complete with no pods")
	}
}

func TestIsDrainComplete_WithEvictablePods(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "test-node"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(node, pod).
		WithIndex(&corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string {
			p := obj.(*corev1.Pod)
			return []string{p.Spec.NodeName}
		}).
		Build()
	ctx := context.Background()

	complete, err := IsDrainComplete(ctx, c, node, DrainOptions{IgnoreDS: true, DeleteOrphans: true}, testMCONamespace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if complete {
		t.Error("expected drain to be incomplete with evictable pods")
	}
}

func TestIsDrainComplete_OnlyDaemonSetPods(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
	}

	controller := true
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "DaemonSet", Controller: &controller},
			},
		},
		Spec:   corev1.PodSpec{NodeName: "test-node"},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(node, pod).
		WithIndex(&corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string {
			p := obj.(*corev1.Pod)
			return []string{p.Spec.NodeName}
		}).
		Build()
	ctx := context.Background()

	complete, err := IsDrainComplete(ctx, c, node, DrainOptions{IgnoreDS: true, DeleteOrphans: true}, testMCONamespace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !complete {
		t.Error("expected drain to be complete when only DS pods remain")
	}
}

// TestIsMCOPod_ByNamespace verifies that pods in MCO namespace are identified.
func TestIsMCOPod_ByNamespace(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "some-pod",
			Namespace: testMCONamespace,
		},
	}
	if !isMCOPod(pod, testMCONamespace) {
		t.Error("pod in MCO namespace should be identified as MCO pod")
	}
}

// TestIsMCOPod_ByLabels verifies label fallback when namespace differs.
func TestIsMCOPod_ByLabels(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "controller-manager-xxx",
			Namespace: "other-namespace",
			Labels: map[string]string{
				LabelAppName:      MCOAppName,
				LabelControlPlane: MCOControllerManager,
			},
		},
	}
	if !isMCOPod(pod, testMCONamespace) {
		t.Error("pod with MCO labels should be identified via fallback")
	}
}

// TestIsMCOPod_OtherPod verifies regular pods are not identified as MCO pods.
func TestIsMCOPod_OtherPod(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-pod",
			Namespace: "default",
			Labels: map[string]string{
				"app": "my-app",
			},
		},
	}
	if isMCOPod(pod, testMCONamespace) {
		t.Error("regular pod should not be identified as MCO pod")
	}
}

// TestIsMCOPod_PartialLabels verifies that both labels are required for fallback.
func TestIsMCOPod_PartialLabels(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   bool
	}{
		{
			name:   "only app.kubernetes.io/name",
			labels: map[string]string{LabelAppName: MCOAppName},
			want:   false,
		},
		{
			name:   "only control-plane",
			labels: map[string]string{LabelControlPlane: MCOControllerManager},
			want:   false,
		},
		{
			name:   "wrong app name",
			labels: map[string]string{LabelAppName: "other-app", LabelControlPlane: MCOControllerManager},
			want:   false,
		},
		{
			name:   "wrong control-plane",
			labels: map[string]string{LabelAppName: MCOAppName, LabelControlPlane: "other"},
			want:   false,
		},
		{
			name:   "both correct",
			labels: map[string]string{LabelAppName: MCOAppName, LabelControlPlane: MCOControllerManager},
			want:   true,
		},
		{
			name:   "nil labels",
			labels: nil,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "other-namespace", // Not MCO namespace
					Labels:    tt.labels,
				},
			}
			if got := isMCOPod(pod, testMCONamespace); got != tt.want {
				t.Errorf("isMCOPod() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFilterEvictablePods_ExcludesMCOByNamespace verifies MCO pods are excluded by namespace.
func TestFilterEvictablePods_ExcludesMCOByNamespace(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "controller-manager-abc",
				Namespace: testMCONamespace,
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-pod",
				Namespace: "default",
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	result := FilterEvictablePods(pods, DrainOptions{DeleteOrphans: true}, testMCONamespace)

	if len(result) != 1 {
		t.Errorf("expected 1 evictable pod, got %d", len(result))
	}
	if len(result) > 0 && result[0].Name != "app-pod" {
		t.Errorf("expected app-pod to be evictable, got %s", result[0].Name)
	}
}

// TestFilterEvictablePods_ExcludesMCOByLabel verifies MCO pods are excluded by label fallback.
func TestFilterEvictablePods_ExcludesMCOByLabel(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "controller-manager-xyz",
				Namespace: "custom-mco-ns",
				Labels: map[string]string{
					LabelAppName:      MCOAppName,
					LabelControlPlane: MCOControllerManager,
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "user-app",
				Namespace: "custom-mco-ns",
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	result := FilterEvictablePods(pods, DrainOptions{DeleteOrphans: true}, testMCONamespace)

	if len(result) != 1 {
		t.Errorf("expected 1 evictable pod, got %d", len(result))
	}
	if len(result) > 0 && result[0].Name != "user-app" {
		t.Errorf("expected user-app to be evictable, got %s", result[0].Name)
	}
}

// TestFilterEvictablePods_ExcludesMCOMixed verifies mixed MCO and regular pods.
func TestFilterEvictablePods_ExcludesMCOMixed(t *testing.T) {
	pods := []corev1.Pod{
		// MCO controller in MCO namespace
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "controller-manager-abc",
				Namespace: testMCONamespace,
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		// MCO agent in MCO namespace (also excluded)
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agent-xyz",
				Namespace: testMCONamespace,
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		// Regular app
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-1",
				Namespace: "default",
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		// Another regular app
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-2",
				Namespace: "kube-system",
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	result := FilterEvictablePods(pods, DrainOptions{DeleteOrphans: true}, testMCONamespace)

	if len(result) != 2 {
		t.Errorf("expected 2 evictable pods, got %d", len(result))
	}

	// Verify the result contains only non-MCO pods
	names := make(map[string]bool)
	for _, p := range result {
		names[p.Name] = true
	}

	if !names["app-1"] || !names["app-2"] {
		t.Errorf("expected app-1 and app-2 to be evictable, got %v", names)
	}
	if names["controller-manager-abc"] || names["agent-xyz"] {
		t.Error("MCO pods should not be in evictable list")
	}
}
