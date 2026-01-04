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

package annotations

import "testing"

func TestGetAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		key         string
		want        string
	}{
		{
			name:        "nil map",
			annotations: nil,
			key:         DesiredRevision,
			want:        "",
		},
		{
			name:        "empty map",
			annotations: map[string]string{},
			key:         DesiredRevision,
			want:        "",
		},
		{
			name:        "key exists",
			annotations: map[string]string{DesiredRevision: "worker-abc123"},
			key:         DesiredRevision,
			want:        "worker-abc123",
		},
		{
			name:        "key not exists",
			annotations: map[string]string{Pool: "worker"},
			key:         DesiredRevision,
			want:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetAnnotation(tt.annotations, tt.key)
			if got != tt.want {
				t.Errorf("GetAnnotation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetBoolAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		key         string
		want        bool
	}{
		{
			name:        "nil map",
			annotations: nil,
			key:         Paused,
			want:        false,
		},
		{
			name:        "true value",
			annotations: map[string]string{Paused: "true"},
			key:         Paused,
			want:        true,
		},
		{
			name:        "false value",
			annotations: map[string]string{Paused: "false"},
			key:         Paused,
			want:        false,
		},
		{
			name:        "other value",
			annotations: map[string]string{Paused: "yes"},
			key:         Paused,
			want:        false,
		},
		{
			name:        "empty value",
			annotations: map[string]string{Paused: ""},
			key:         Paused,
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetBoolAnnotation(tt.annotations, tt.key)
			if got != tt.want {
				t.Errorf("GetBoolAnnotation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		key         string
		value       string
		wantKey     string
		wantValue   string
	}{
		{
			name:        "nil map",
			annotations: nil,
			key:         DesiredRevision,
			value:       "worker-abc123",
			wantKey:     DesiredRevision,
			wantValue:   "worker-abc123",
		},
		{
			name:        "empty map",
			annotations: map[string]string{},
			key:         DesiredRevision,
			value:       "worker-abc123",
			wantKey:     DesiredRevision,
			wantValue:   "worker-abc123",
		},
		{
			name:        "existing map",
			annotations: map[string]string{Pool: "worker"},
			key:         DesiredRevision,
			value:       "worker-abc123",
			wantKey:     DesiredRevision,
			wantValue:   "worker-abc123",
		},
		{
			name:        "overwrite existing",
			annotations: map[string]string{DesiredRevision: "old"},
			key:         DesiredRevision,
			value:       "new",
			wantKey:     DesiredRevision,
			wantValue:   "new",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SetAnnotation(tt.annotations, tt.key, tt.value)
			if got == nil {
				t.Fatal("SetAnnotation() returned nil")
			}
			if got[tt.wantKey] != tt.wantValue {
				t.Errorf("SetAnnotation()[%s] = %v, want %v", tt.wantKey, got[tt.wantKey], tt.wantValue)
			}
		})
	}
}

func TestRemoveAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		key         string
		wantExists  bool
	}{
		{
			name:        "nil map",
			annotations: nil,
			key:         DesiredRevision,
			wantExists:  false,
		},
		{
			name:        "key exists",
			annotations: map[string]string{DesiredRevision: "worker-abc123"},
			key:         DesiredRevision,
			wantExists:  false,
		},
		{
			name:        "key not exists",
			annotations: map[string]string{Pool: "worker"},
			key:         DesiredRevision,
			wantExists:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RemoveAnnotation(tt.annotations, tt.key)
			if got != nil {
				_, exists := got[tt.key]
				if exists != tt.wantExists {
					t.Errorf("RemoveAnnotation() key exists = %v, want %v", exists, tt.wantExists)
				}
			}
		})
	}
}

func TestIsNodePaused(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{
			name:        "nil annotations",
			annotations: nil,
			want:        false,
		},
		{
			name:        "paused true",
			annotations: map[string]string{Paused: "true"},
			want:        true,
		},
		{
			name:        "paused false",
			annotations: map[string]string{Paused: "false"},
			want:        false,
		},
		{
			name:        "no paused annotation",
			annotations: map[string]string{Pool: "worker"},
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsNodePaused(tt.annotations)
			if got != tt.want {
				t.Errorf("IsNodePaused() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNeedsUpdate(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{
			name:        "no desired revision",
			annotations: map[string]string{},
			want:        false,
		},
		{
			name:        "desired != current",
			annotations: map[string]string{DesiredRevision: "new", CurrentRevision: "old"},
			want:        true,
		},
		{
			name:        "desired == current",
			annotations: map[string]string{DesiredRevision: "same", CurrentRevision: "same"},
			want:        false,
		},
		{
			name:        "desired set, no current",
			annotations: map[string]string{DesiredRevision: "new"},
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NeedsUpdate(tt.annotations)
			if got != tt.want {
				t.Errorf("NeedsUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsUpToDate(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{
			name:        "no desired revision",
			annotations: map[string]string{},
			want:        true,
		},
		{
			name:        "desired != current",
			annotations: map[string]string{DesiredRevision: "new", CurrentRevision: "old"},
			want:        false,
		},
		{
			name:        "desired == current",
			annotations: map[string]string{DesiredRevision: "same", CurrentRevision: "same"},
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsUpToDate(tt.annotations)
			if got != tt.want {
				t.Errorf("IsUpToDate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsReady(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{
			name:        "up to date and done",
			annotations: map[string]string{DesiredRevision: "rev", CurrentRevision: "rev", AgentState: StateDone},
			want:        true,
		},
		{
			name:        "up to date but applying",
			annotations: map[string]string{DesiredRevision: "rev", CurrentRevision: "rev", AgentState: StateApplying},
			want:        false,
		},
		{
			name:        "not up to date",
			annotations: map[string]string{DesiredRevision: "new", CurrentRevision: "old", AgentState: StateDone},
			want:        false,
		},
		{
			name:        "no desired revision",
			annotations: map[string]string{AgentState: StateDone},
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsReady(tt.annotations)
			if got != tt.want {
				t.Errorf("IsReady() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConstants(t *testing.T) {
	// Verify annotation keys have correct prefix
	if Prefix != "mco.in-cloud.io/" {
		t.Errorf("Prefix = %q, want mco.in-cloud.io/", Prefix)
	}

	expectedAnnotations := map[string]string{
		"DesiredRevision": DesiredRevision,
		"Pool":            Pool,
		"CurrentRevision": CurrentRevision,
		"AgentState":      AgentState,
		"LastError":       LastError,
		"RebootPending":   RebootPending,
		"Paused":          Paused,
		"ForceReboot":     ForceReboot,
	}

	for name, key := range expectedAnnotations {
		if len(key) <= len(Prefix) {
			t.Errorf("%s annotation key too short: %q", name, key)
		}
		if key[:len(Prefix)] != Prefix {
			t.Errorf("%s annotation key doesn't start with prefix: %q", name, key)
		}
	}

	// Verify state constants
	states := []string{StateIdle, StateApplying, StateDone, StateError}
	for _, s := range states {
		if s == "" {
			t.Error("State constant is empty")
		}
	}
}
