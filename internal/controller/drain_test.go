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
)

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

	result := FilterEvictablePods(pods, DrainConfig{DeleteOrphans: true})

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

	result := FilterEvictablePods(pods, DrainConfig{DeleteOrphans: true})

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

	result := FilterEvictablePods(pods, DrainConfig{IgnoreDS: true, DeleteOrphans: true})

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

	result := FilterEvictablePods(pods, DrainConfig{IgnoreDS: false, DeleteOrphans: true})

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

	result := FilterEvictablePods(pods, DrainConfig{DeleteOrphans: false})

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

	result := FilterEvictablePods(pods, DrainConfig{DeleteOrphans: true})

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

	result := HandleDrainRetry(ctx, c, node, DefaultDrainTimeoutSeconds)

	if result.RequeueAfter != time.Minute {
		t.Errorf("expected 1 minute requeue, got %v", result.RequeueAfter)
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

	result := HandleDrainRetry(ctx, c, node, DefaultDrainTimeoutSeconds)

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

	result := HandleDrainRetry(ctx, c, node, DefaultDrainTimeoutSeconds)

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

	result := HandleDrainRetry(ctx, c, node, DefaultDrainTimeoutSeconds)

	if result.RequeueAfter != time.Minute {
		t.Errorf("expected 1 minute requeue for first attempt, got %v", result.RequeueAfter)
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
	result := HandleDrainRetry(ctx, c, node, DefaultDrainTimeoutSeconds)
	if result.SetDrainStuck {
		t.Error("expected SetDrainStuck to be false with default timeout")
	}

	// With 15 minute timeout, should be stuck
	result = HandleDrainRetry(ctx, c, node, 900) // 15 minutes
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

	// Pass 0 to use default
	result := HandleDrainRetry(ctx, c, node, 0)
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

	complete, err := IsDrainComplete(ctx, c, node, DrainConfig{IgnoreDS: true, DeleteOrphans: true})
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

	complete, err := IsDrainComplete(ctx, c, node, DrainConfig{IgnoreDS: true, DeleteOrphans: true})
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

	complete, err := IsDrainComplete(ctx, c, node, DrainConfig{IgnoreDS: true, DeleteOrphans: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !complete {
		t.Error("expected drain to be complete when only DS pods remain")
	}
}
