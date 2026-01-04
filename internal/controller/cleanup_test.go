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

package controller

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/pkg/annotations"
)

func makeRMC(name, pool string, createdBefore time.Duration) *mcov1alpha1.RenderedMachineConfig {
	return &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-createdBefore)),
			Labels: map[string]string{
				"mco.in-cloud.io/pool": pool,
			},
		},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			PoolName: pool,
		},
	}
}

func TestNewRMCCleaner(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(newTestScheme()).Build()
	cleaner := NewRMCCleaner(c)

	if cleaner == nil {
		t.Fatal("NewRMCCleaner() returned nil")
	}
}

func TestCleanupOldRMCs_UnlimitedRetention(t *testing.T) {
	scheme := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	cleaner := NewRMCCleaner(c)

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			RevisionHistory: mcov1alpha1.RevisionHistoryConfig{
				Limit: 0,
			},
		},
	}

	deleted, err := cleaner.CleanupOldRMCs(context.Background(), pool, []corev1.Node{})
	if err != nil {
		t.Fatalf("CleanupOldRMCs() error = %v", err)
	}

	if deleted != 0 {
		t.Errorf("deleted = %d, want 0 for unlimited retention", deleted)
	}
}

func TestCleanupOldRMCs_WithinLimit(t *testing.T) {
	scheme := newTestScheme()

	rmcs := []client.Object{
		makeRMC("worker-abc", "worker", 3*time.Hour),
		makeRMC("worker-def", "worker", 2*time.Hour),
		makeRMC("worker-ghi", "worker", 1*time.Hour),
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rmcs...).Build()
	cleaner := NewRMCCleaner(c)

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			RevisionHistory: mcov1alpha1.RevisionHistoryConfig{
				Limit: 5,
			},
		},
	}

	deleted, err := cleaner.CleanupOldRMCs(context.Background(), pool, []corev1.Node{})
	if err != nil {
		t.Fatalf("CleanupOldRMCs() error = %v", err)
	}

	if deleted != 0 {
		t.Errorf("deleted = %d, want 0 (within limit)", deleted)
	}
}

func TestCleanupOldRMCs_ExceedsLimit(t *testing.T) {
	scheme := newTestScheme()

	rmcs := []client.Object{
		makeRMC("worker-oldest", "worker", 5*time.Hour),
		makeRMC("worker-old", "worker", 4*time.Hour),
		makeRMC("worker-mid", "worker", 3*time.Hour),
		makeRMC("worker-new", "worker", 2*time.Hour),
		makeRMC("worker-newest", "worker", 1*time.Hour),
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rmcs...).Build()
	cleaner := NewRMCCleaner(c)

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			RevisionHistory: mcov1alpha1.RevisionHistoryConfig{
				Limit: 3,
			},
		},
	}

	deleted, err := cleaner.CleanupOldRMCs(context.Background(), pool, []corev1.Node{})
	if err != nil {
		t.Fatalf("CleanupOldRMCs() error = %v", err)
	}

	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}

	rmcList := &mcov1alpha1.RenderedMachineConfigList{}
	if err := c.List(context.Background(), rmcList); err != nil {
		t.Fatalf("Failed to list RMCs: %v", err)
	}

	if len(rmcList.Items) != 3 {
		t.Errorf("Remaining RMCs = %d, want 3", len(rmcList.Items))
	}
}

func TestCleanupOldRMCs_PreservesInUse(t *testing.T) {
	scheme := newTestScheme()

	rmcs := []client.Object{
		makeRMC("worker-oldest", "worker", 5*time.Hour), // oldest but in use
		makeRMC("worker-old", "worker", 4*time.Hour),
		makeRMC("worker-mid", "worker", 3*time.Hour),
		makeRMC("worker-new", "worker", 2*time.Hour),
		makeRMC("worker-newest", "worker", 1*time.Hour),
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rmcs...).Build()
	cleaner := NewRMCCleaner(c)

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			RevisionHistory: mcov1alpha1.RevisionHistoryConfig{
				Limit: 3,
			},
		},
	}

	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
				Annotations: map[string]string{
					annotations.CurrentRevision: "worker-oldest",
				},
			},
		},
	}

	deleted, err := cleaner.CleanupOldRMCs(context.Background(), pool, nodes)
	if err != nil {
		t.Fatalf("CleanupOldRMCs() error = %v", err)
	}

	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}

	rmcList := &mcov1alpha1.RenderedMachineConfigList{}
	if err := c.List(context.Background(), rmcList); err != nil {
		t.Fatalf("Failed to list RMCs: %v", err)
	}

	if len(rmcList.Items) != 3 {
		t.Errorf("Remaining RMCs = %d, want 3", len(rmcList.Items))
	}

	foundOldest := false
	for _, rmc := range rmcList.Items {
		if rmc.Name == "worker-oldest" {
			foundOldest = true
			break
		}
	}
	if !foundOldest {
		t.Error("worker-oldest should be preserved (in use)")
	}
}

func TestCleanupOldRMCs_PreservesTarget(t *testing.T) {
	scheme := newTestScheme()

	rmcs := []client.Object{
		makeRMC("worker-target", "worker", 5*time.Hour), // oldest but is target
		makeRMC("worker-old", "worker", 4*time.Hour),
		makeRMC("worker-new", "worker", 1*time.Hour),
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rmcs...).Build()
	cleaner := NewRMCCleaner(c)

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			RevisionHistory: mcov1alpha1.RevisionHistoryConfig{
				Limit: 2,
			},
		},
		Status: mcov1alpha1.MachineConfigPoolStatus{
			TargetRevision: "worker-target",
		},
	}

	deleted, err := cleaner.CleanupOldRMCs(context.Background(), pool, []corev1.Node{})
	if err != nil {
		t.Fatalf("CleanupOldRMCs() error = %v", err)
	}

	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}
}

func TestCleanupOldRMCs_PreservesDesiredRevision(t *testing.T) {
	scheme := newTestScheme()

	rmcs := []client.Object{
		makeRMC("worker-oldest", "worker", 5*time.Hour),
		makeRMC("worker-desired", "worker", 4*time.Hour), // desired by node
		makeRMC("worker-new", "worker", 1*time.Hour),
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rmcs...).Build()
	cleaner := NewRMCCleaner(c)

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: mcov1alpha1.MachineConfigPoolSpec{
			RevisionHistory: mcov1alpha1.RevisionHistoryConfig{
				Limit: 2,
			},
		},
	}

	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
				Annotations: map[string]string{
					annotations.DesiredRevision: "worker-desired",
				},
			},
		},
	}

	deleted, err := cleaner.CleanupOldRMCs(context.Background(), pool, nodes)
	if err != nil {
		t.Fatalf("CleanupOldRMCs() error = %v", err)
	}

	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}

	rmcList := &mcov1alpha1.RenderedMachineConfigList{}
	c.List(context.Background(), rmcList)

	foundDesired := false
	for _, rmc := range rmcList.Items {
		if rmc.Name == "worker-desired" {
			foundDesired = true
			break
		}
	}
	if !foundDesired {
		t.Error("worker-desired should be preserved")
	}
}

func TestGetRevisionsInUse(t *testing.T) {
	cleaner := &RMCCleaner{}

	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					annotations.CurrentRevision: "rev-current",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					annotations.DesiredRevision: "rev-desired",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{}, // no annotations
		},
	}

	inUse := cleaner.getRevisionsInUse(nodes, "rev-target")

	expected := map[string]bool{
		"rev-target":  true,
		"rev-current": true,
		"rev-desired": true,
	}

	for rev, want := range expected {
		if inUse[rev] != want {
			t.Errorf("inUse[%s] = %v, want %v", rev, inUse[rev], want)
		}
	}
}

func TestGetRevisionsInUse_EmptyTarget(t *testing.T) {
	cleaner := &RMCCleaner{}

	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					annotations.CurrentRevision: "rev-current",
				},
			},
		},
	}

	inUse := cleaner.getRevisionsInUse(nodes, "")

	if inUse[""] {
		t.Error("Empty target should not be in use set")
	}
	if !inUse["rev-current"] {
		t.Error("rev-current should be in use set")
	}
}

func TestCleanupOrphanedRMCs(t *testing.T) {
	scheme := newTestScheme()

	pools := []client.Object{
		&mcov1alpha1.MachineConfigPool{
			ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		},
	}

	rmcs := []client.Object{
		makeRMC("worker-abc", "worker", 1*time.Hour),
		makeRMC("deleted-pool-xyz", "deleted", 1*time.Hour),
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(append(pools, rmcs...)...).Build()
	cleaner := NewRMCCleaner(c)

	deleted, err := cleaner.CleanupOrphanedRMCs(context.Background())
	if err != nil {
		t.Fatalf("CleanupOrphanedRMCs() error = %v", err)
	}

	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}

	rmcList := &mcov1alpha1.RenderedMachineConfigList{}
	c.List(context.Background(), rmcList)

	if len(rmcList.Items) != 1 {
		t.Errorf("Remaining RMCs = %d, want 1", len(rmcList.Items))
	}

	if rmcList.Items[0].Name != "worker-abc" {
		t.Errorf("Remaining RMC = %s, want worker-abc", rmcList.Items[0].Name)
	}
}

func TestCleanupOrphanedRMCs_NoOrphans(t *testing.T) {
	scheme := newTestScheme()

	pools := []client.Object{
		&mcov1alpha1.MachineConfigPool{
			ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		},
	}

	rmcs := []client.Object{
		makeRMC("worker-abc", "worker", 1*time.Hour),
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(append(pools, rmcs...)...).Build()
	cleaner := NewRMCCleaner(c)

	deleted, err := cleaner.CleanupOrphanedRMCs(context.Background())
	if err != nil {
		t.Fatalf("CleanupOrphanedRMCs() error = %v", err)
	}

	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}
}
