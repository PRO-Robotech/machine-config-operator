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
	"testing"
)

func TestNewSystemdExecutor(t *testing.T) {
	executor := NewSystemdExecutor("/host")

	if executor == nil {
		t.Fatal("NewSystemdExecutor() returned nil")
	}
	if executor.hostRoot != "/host" {
		t.Errorf("hostRoot = %q, want %q", executor.hostRoot, "/host")
	}
}

func TestNoOpExecutor(t *testing.T) {
	executor := &NoOpExecutor{}

	if executor.Called {
		t.Error("Called should be false initially")
	}

	err := executor.Execute(context.Background())

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !executor.Called {
		t.Error("Called should be true after Execute()")
	}
}

// Note: We don't test SystemdExecutor.Execute() because it would actually
// attempt to reboot the system. The NoOpExecutor is used for testing.
