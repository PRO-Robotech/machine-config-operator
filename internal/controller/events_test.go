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
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

func TestNewEventRecorder(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	er := NewEventRecorder(recorder)

	if er == nil {
		t.Fatal("NewEventRecorder returned nil")
	}
	if er.recorder == nil {
		t.Error("recorder is nil")
	}
}

func TestEventRecorder_PoolOverlapDetected(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	er := NewEventRecorder(recorder)

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
	}

	er.PoolOverlapDetected(pool, []string{"node-1", "node-2"})

	select {
	case event := <-recorder.Events:
		if !strings.Contains(event, ReasonPoolOverlap) {
			t.Errorf("expected reason %s, got %s", ReasonPoolOverlap, event)
		}
		if !strings.Contains(event, "node-1") || !strings.Contains(event, "node-2") {
			t.Errorf("expected node names in event, got %s", event)
		}
		if !strings.Contains(event, "Warning") {
			t.Errorf("expected Warning type, got %s", event)
		}
	default:
		t.Error("expected event to be recorded")
	}
}

func TestEventRecorder_NodeCordonStarted(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	er := NewEventRecorder(recorder)

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
	}

	er.NodeCordonStarted(pool, "node-1")

	select {
	case event := <-recorder.Events:
		if !strings.Contains(event, ReasonNodeCordon) {
			t.Errorf("expected reason %s, got %s", ReasonNodeCordon, event)
		}
		if !strings.Contains(event, "node-1") {
			t.Errorf("expected node name in event, got %s", event)
		}
		if !strings.Contains(event, "Normal") {
			t.Errorf("expected Normal type, got %s", event)
		}
	default:
		t.Error("expected event to be recorded")
	}
}

func TestEventRecorder_DrainStuck(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	er := NewEventRecorder(recorder)

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
	}

	er.DrainStuck(pool, "node-1")

	select {
	case event := <-recorder.Events:
		if !strings.Contains(event, ReasonDrainStuck) {
			t.Errorf("expected reason %s, got %s", ReasonDrainStuck, event)
		}
		if !strings.Contains(event, "node-1") {
			t.Errorf("expected node name in event, got %s", event)
		}
		if !strings.Contains(event, "Warning") {
			t.Errorf("expected Warning type, got %s", event)
		}
	default:
		t.Error("expected event to be recorded")
	}
}

func TestEventRecorder_RolloutBatchStarted(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	er := NewEventRecorder(recorder)

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
	}

	er.RolloutBatchStarted(pool, 2, []string{"node-1", "node-2"})

	select {
	case event := <-recorder.Events:
		if !strings.Contains(event, ReasonRolloutBatch) {
			t.Errorf("expected reason %s, got %s", ReasonRolloutBatch, event)
		}
		if !strings.Contains(event, "node-1") || !strings.Contains(event, "node-2") {
			t.Errorf("expected node names in event, got %s", event)
		}
	default:
		t.Error("expected event to be recorded")
	}
}

func TestEventRecorder_RolloutBatchStarted_ManyNodes(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	er := NewEventRecorder(recorder)

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
	}

	// More than 3 nodes - should truncate
	er.RolloutBatchStarted(pool, 5, []string{"node-1", "node-2", "node-3", "node-4", "node-5"})

	select {
	case event := <-recorder.Events:
		if !strings.Contains(event, "and 2 more") {
			t.Errorf("expected truncation message, got %s", event)
		}
	default:
		t.Error("expected event to be recorded")
	}
}

func TestEventRecorder_NilRecorder(t *testing.T) {
	// EventRecorder with nil recorder should not panic
	er := &EventRecorder{recorder: nil}

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
	}

	// These should not panic
	er.PoolOverlapDetected(pool, []string{"node-1"})
	er.NodeCordonStarted(pool, "node-1")
	er.DrainStuck(pool, "node-1")
	er.RolloutBatchStarted(pool, 1, []string{"node-1"})
	er.RolloutComplete(pool)
}

func TestEventRecorder_NodeUncordoned(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	er := NewEventRecorder(recorder)

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
	}

	er.NodeUncordoned(pool, "node-1")

	select {
	case event := <-recorder.Events:
		if !strings.Contains(event, ReasonNodeUncordon) {
			t.Errorf("expected reason %s, got %s", ReasonNodeUncordon, event)
		}
		if !strings.Contains(event, "node-1") {
			t.Errorf("expected node name in event, got %s", event)
		}
	default:
		t.Error("expected event to be recorded")
	}
}

func TestEventRecorder_DrainComplete(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	er := NewEventRecorder(recorder)

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
	}

	er.DrainComplete(pool, "node-1")

	select {
	case event := <-recorder.Events:
		if !strings.Contains(event, ReasonDrainComplete) {
			t.Errorf("expected reason %s, got %s", ReasonDrainComplete, event)
		}
	default:
		t.Error("expected event to be recorded")
	}
}

func TestEventRecorder_RolloutComplete(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	er := NewEventRecorder(recorder)

	pool := &mcov1alpha1.MachineConfigPool{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
	}

	er.RolloutComplete(pool)

	select {
	case event := <-recorder.Events:
		if !strings.Contains(event, ReasonRolloutComplete) {
			t.Errorf("expected reason %s, got %s", ReasonRolloutComplete, event)
		}
	default:
		t.Error("expected event to be recorded")
	}
}
