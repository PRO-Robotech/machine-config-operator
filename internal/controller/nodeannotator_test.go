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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"in-cloud.io/machine-config/pkg/annotations"
)

func TestNewNodeAnnotator(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(newTestScheme()).Build()
	annotator := NewNodeAnnotator(c)

	if annotator == nil {
		t.Fatal("NewNodeAnnotator() returned nil")
	}

	if annotator.client == nil {
		t.Error("NodeAnnotator.client is nil")
	}
}

func TestNodeAnnotator_SetDesiredRevision(t *testing.T) {
	scheme := newTestScheme()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	annotator := NewNodeAnnotator(c)

	err := annotator.SetDesiredRevision(context.Background(), "worker-1", "worker-abc123", "worker")
	if err != nil {
		t.Fatalf("SetDesiredRevision() error = %v", err)
	}

	updatedNode := &corev1.Node{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: "worker-1"}, updatedNode); err != nil {
		t.Fatalf("Failed to get node: %v", err)
	}

	if updatedNode.Annotations[annotations.DesiredRevision] != "worker-abc123" {
		t.Errorf("DesiredRevision = %q, want %q", updatedNode.Annotations[annotations.DesiredRevision], "worker-abc123")
	}

	if updatedNode.Annotations[annotations.Pool] != "worker" {
		t.Errorf("Pool = %q, want %q", updatedNode.Annotations[annotations.Pool], "worker")
	}
}

func TestNodeAnnotator_SetDesiredRevision_NodeNotFound(t *testing.T) {
	scheme := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	annotator := NewNodeAnnotator(c)

	err := annotator.SetDesiredRevision(context.Background(), "nonexistent", "worker-abc123", "worker")
	if err == nil {
		t.Error("SetDesiredRevision() should return error for nonexistent node")
	}
}

func TestNodeAnnotator_RemoveDesiredRevision(t *testing.T) {
	scheme := newTestScheme()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-1",
			Annotations: map[string]string{
				annotations.DesiredRevision: "worker-abc123",
				annotations.Pool:            "worker",
				"other":                     "keep-me",
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	annotator := NewNodeAnnotator(c)

	err := annotator.RemoveDesiredRevision(context.Background(), "worker-1")
	if err != nil {
		t.Fatalf("RemoveDesiredRevision() error = %v", err)
	}

	updatedNode := &corev1.Node{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: "worker-1"}, updatedNode); err != nil {
		t.Fatalf("Failed to get node: %v", err)
	}

	if _, exists := updatedNode.Annotations[annotations.DesiredRevision]; exists {
		t.Error("DesiredRevision annotation should be removed")
	}

	if _, exists := updatedNode.Annotations[annotations.Pool]; exists {
		t.Error("Pool annotation should be removed")
	}

	if updatedNode.Annotations["other"] != "keep-me" {
		t.Error("Other annotations should not be affected")
	}
}

func TestNodeAnnotator_SetDesiredRevisionForNodes(t *testing.T) {
	scheme := newTestScheme()

	nodes := []client.Object{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-1"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-2"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-3", Annotations: map[string]string{annotations.Paused: "true"}}},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodes...).Build()
	annotator := NewNodeAnnotator(c)

	nodeList := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "worker-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "worker-2"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "worker-3", Annotations: map[string]string{annotations.Paused: "true"}}},
	}

	updated, err := annotator.SetDesiredRevisionForNodes(context.Background(), nodeList, "worker-abc123", "worker")
	if err != nil {
		t.Fatalf("SetDesiredRevisionForNodes() error = %v", err)
	}

	if updated != 2 {
		t.Errorf("Updated count = %d, want 2", updated)
	}

	node1 := &corev1.Node{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: "worker-1"}, node1); err != nil {
		t.Fatalf("Failed to get worker-1: %v", err)
	}
	if node1.Annotations[annotations.DesiredRevision] != "worker-abc123" {
		t.Error("worker-1 should have desired revision")
	}

	node3 := &corev1.Node{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: "worker-3"}, node3); err != nil {
		t.Fatalf("Failed to get worker-3: %v", err)
	}
	if node3.Annotations[annotations.DesiredRevision] == "worker-abc123" {
		t.Error("worker-3 should NOT have desired revision (paused)")
	}
}

func TestNodeAnnotator_SetDesiredRevisionForNodes_SkipsUpToDate(t *testing.T) {
	scheme := newTestScheme()

	nodes := []client.Object{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-1"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-2", Annotations: map[string]string{annotations.DesiredRevision: "worker-abc123"}}},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodes...).Build()
	annotator := NewNodeAnnotator(c)

	nodeList := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "worker-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "worker-2", Annotations: map[string]string{annotations.DesiredRevision: "worker-abc123"}}},
	}

	updated, err := annotator.SetDesiredRevisionForNodes(context.Background(), nodeList, "worker-abc123", "worker")
	if err != nil {
		t.Fatalf("SetDesiredRevisionForNodes() error = %v", err)
	}

	if updated != 1 {
		t.Errorf("Updated count = %d, want 1", updated)
	}
}

func TestNodeAnnotator_SetDesiredRevisionForNodes_EmptyList(t *testing.T) {
	scheme := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	annotator := NewNodeAnnotator(c)

	updated, err := annotator.SetDesiredRevisionForNodes(context.Background(), []corev1.Node{}, "worker-abc123", "worker")
	if err != nil {
		t.Fatalf("SetDesiredRevisionForNodes() error = %v", err)
	}

	if updated != 0 {
		t.Errorf("Updated count = %d, want 0", updated)
	}
}

func TestNodeAnnotator_SetDesiredRevisionForNodes_NodeNotFound(t *testing.T) {
	scheme := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	annotator := NewNodeAnnotator(c)

	nodeList := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "nonexistent"}},
	}

	_, err := annotator.SetDesiredRevisionForNodes(context.Background(), nodeList, "worker-abc123", "worker")
	if err == nil {
		t.Error("SetDesiredRevisionForNodes() should return error for nonexistent node")
	}
}
