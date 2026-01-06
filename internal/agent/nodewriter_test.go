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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"in-cloud.io/machine-config/pkg/annotations"
)

func TestNodeWriter_SetState(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
	}
	client := fake.NewSimpleClientset(node)
	writer := NewNodeWriter(client, "test-node")

	err := writer.SetState(context.Background(), annotations.StateApplying)
	if err != nil {
		t.Fatalf("SetState() error = %v", err)
	}

	updated, err := client.CoreV1().Nodes().Get(context.Background(), "test-node", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get node error = %v", err)
	}

	got := updated.Annotations[annotations.AgentState]
	if got != annotations.StateApplying {
		t.Errorf("AgentState = %q, want %q", got, annotations.StateApplying)
	}
}

func TestNodeWriter_SetCurrentRevision(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
	}
	client := fake.NewSimpleClientset(node)
	writer := NewNodeWriter(client, "test-node")

	err := writer.SetCurrentRevision(context.Background(), "worker-abc123")
	if err != nil {
		t.Fatalf("SetCurrentRevision() error = %v", err)
	}

	updated, err := client.CoreV1().Nodes().Get(context.Background(), "test-node", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get node error = %v", err)
	}

	got := updated.Annotations[annotations.CurrentRevision]
	if got != "worker-abc123" {
		t.Errorf("CurrentRevision = %q, want %q", got, "worker-abc123")
	}
}

func TestNodeWriter_SetLastError(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
	}
	client := fake.NewSimpleClientset(node)
	writer := NewNodeWriter(client, "test-node")

	err := writer.SetLastError(context.Background(), "something went wrong")
	if err != nil {
		t.Fatalf("SetLastError() error = %v", err)
	}

	updated, err := client.CoreV1().Nodes().Get(context.Background(), "test-node", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get node error = %v", err)
	}

	got := updated.Annotations[annotations.LastError]
	if got != "something went wrong" {
		t.Errorf("LastError = %q, want %q", got, "something went wrong")
	}
}

func TestNodeWriter_ClearLastError(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-node",
			Annotations: map[string]string{annotations.LastError: "old error"},
		},
	}
	client := fake.NewSimpleClientset(node)
	writer := NewNodeWriter(client, "test-node")

	err := writer.ClearLastError(context.Background())
	if err != nil {
		t.Fatalf("ClearLastError() error = %v", err)
	}

	updated, err := client.CoreV1().Nodes().Get(context.Background(), "test-node", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get node error = %v", err)
	}

	_, exists := updated.Annotations[annotations.LastError]
	if exists {
		t.Error("LastError annotation should be removed")
	}
}

func TestNodeWriter_SetRebootPending(t *testing.T) {
	tests := []struct {
		name       string
		pending    bool
		wantExists bool
		wantValue  string
	}{
		{
			name:       "set pending true",
			pending:    true,
			wantExists: true,
			wantValue:  "true",
		},
		{
			name:       "set pending false",
			pending:    false,
			wantExists: false,
			wantValue:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-node",
					Annotations: map[string]string{annotations.RebootPending: "true"},
				},
			}
			client := fake.NewSimpleClientset(node)
			writer := NewNodeWriter(client, "test-node")

			err := writer.SetRebootPending(context.Background(), tt.pending)
			if err != nil {
				t.Fatalf("SetRebootPending() error = %v", err)
			}

			updated, err := client.CoreV1().Nodes().Get(context.Background(), "test-node", metav1.GetOptions{})
			if err != nil {
				t.Fatalf("Get node error = %v", err)
			}

			value, exists := updated.Annotations[annotations.RebootPending]
			if exists != tt.wantExists {
				t.Errorf("RebootPending exists = %v, want %v", exists, tt.wantExists)
			}
			if exists && value != tt.wantValue {
				t.Errorf("RebootPending = %q, want %q", value, tt.wantValue)
			}
		})
	}
}

func TestNodeWriter_SetStateWithError(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
	}
	client := fake.NewSimpleClientset(node)
	writer := NewNodeWriter(client, "test-node")

	err := writer.SetStateWithError(context.Background(), annotations.StateError, "apply failed")
	if err != nil {
		t.Fatalf("SetStateWithError() error = %v", err)
	}

	updated, err := client.CoreV1().Nodes().Get(context.Background(), "test-node", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get node error = %v", err)
	}

	if got := updated.Annotations[annotations.AgentState]; got != annotations.StateError {
		t.Errorf("AgentState = %q, want %q", got, annotations.StateError)
	}
	if got := updated.Annotations[annotations.LastError]; got != "apply failed" {
		t.Errorf("LastError = %q, want %q", got, "apply failed")
	}
}

func TestNodeWriter_SetDone(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				annotations.AgentState:      annotations.StateApplying,
				annotations.CurrentRevision: "old-rev",
			},
		},
	}
	client := fake.NewSimpleClientset(node)
	writer := NewNodeWriter(client, "test-node")

	err := writer.SetDone(context.Background(), "new-rev")
	if err != nil {
		t.Fatalf("SetDone() error = %v", err)
	}

	updated, err := client.CoreV1().Nodes().Get(context.Background(), "test-node", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get node error = %v", err)
	}

	if got := updated.Annotations[annotations.AgentState]; got != annotations.StateDone {
		t.Errorf("AgentState = %q, want %q", got, annotations.StateDone)
	}
	if got := updated.Annotations[annotations.CurrentRevision]; got != "new-rev" {
		t.Errorf("CurrentRevision = %q, want %q", got, "new-rev")
	}
}

func TestNodeWriter_ClearForceReboot(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-node",
			Annotations: map[string]string{annotations.ForceReboot: "true"},
		},
	}
	client := fake.NewSimpleClientset(node)
	writer := NewNodeWriter(client, "test-node")

	err := writer.ClearForceReboot(context.Background())
	if err != nil {
		t.Fatalf("ClearForceReboot() error = %v", err)
	}

	updated, err := client.CoreV1().Nodes().Get(context.Background(), "test-node", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get node error = %v", err)
	}

	_, exists := updated.Annotations[annotations.ForceReboot]
	if exists {
		t.Error("ForceReboot annotation should be removed")
	}
}

func TestNodeWriter_ExistingAnnotationsPreserved(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				"other-annotation": "preserved-value",
				annotations.Pool:   "worker",
			},
		},
	}
	client := fake.NewSimpleClientset(node)
	writer := NewNodeWriter(client, "test-node")

	err := writer.SetState(context.Background(), annotations.StateApplying)
	if err != nil {
		t.Fatalf("SetState() error = %v", err)
	}

	updated, err := client.CoreV1().Nodes().Get(context.Background(), "test-node", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get node error = %v", err)
	}

	// Verify other annotations are preserved
	if got := updated.Annotations["other-annotation"]; got != "preserved-value" {
		t.Errorf("other-annotation = %q, want %q", got, "preserved-value")
	}
	if got := updated.Annotations[annotations.Pool]; got != "worker" {
		t.Errorf("Pool = %q, want %q", got, "worker")
	}
	// Verify new annotation is set
	if got := updated.Annotations[annotations.AgentState]; got != annotations.StateApplying {
		t.Errorf("AgentState = %q, want %q", got, annotations.StateApplying)
	}
}
