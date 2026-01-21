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

package drain

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestLoadDrainConfig_Found(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mco-drain-config",
			Namespace: "mco-system",
			Labels: map[string]string{
				DrainConfigLabel: "true",
			},
		},
		Data: map[string]string{
			DrainConfigDataKey: `
defaults:
  skipToleratAllPods: true
rules:
  - namespaces:
      - kube-system
  - podNamePatterns:
      - "debug-*"
`,
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	ctx := context.Background()

	result, err := LoadDrainConfig(ctx, c, "mco-system")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have no parse warning for valid YAML
	if result.ParseWarning != nil {
		t.Errorf("unexpected parse warning: %v", result.ParseWarning)
	}

	config := result.Config
	if config == nil {
		t.Fatal("expected non-nil config")
	}

	if !config.Defaults.SkipToleratAllPods {
		t.Error("expected SkipToleratAllPods to be true")
	}

	if len(config.Rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(config.Rules))
	}

	if len(config.Rules[0].Namespaces) != 1 || config.Rules[0].Namespaces[0] != "kube-system" {
		t.Errorf("expected first rule to have namespace kube-system")
	}

	if len(config.Rules[1].PodNamePatterns) != 1 || config.Rules[1].PodNamePatterns[0] != "debug-*" {
		t.Errorf("expected second rule to have pattern debug-*")
	}
}

func TestLoadDrainConfig_NotFound_ReturnsDefaults(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	ctx := context.Background()

	result, err := LoadDrainConfig(ctx, c, "mco-system")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No ConfigMap is expected behavior - no warning
	if result.ParseWarning != nil {
		t.Errorf("unexpected parse warning: %v", result.ParseWarning)
	}

	config := result.Config
	if config == nil {
		t.Fatal("expected non-nil config")
	}

	if config.Defaults.SkipToleratAllPods != DefaultSkipToleratAllPods {
		t.Errorf("expected default SkipToleratAllPods=%v", DefaultSkipToleratAllPods)
	}

	if config.Rules != nil {
		t.Errorf("expected nil rules for default config")
	}
}

func TestLoadDrainConfig_InvalidYAML(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mco-drain-config",
			Namespace: "mco-system",
			Labels: map[string]string{
				DrainConfigLabel: "true",
			},
		},
		Data: map[string]string{
			DrainConfigDataKey: `
this is not valid yaml: [
  broken
`,
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	ctx := context.Background()

	result, err := LoadDrainConfig(ctx, c, "mco-system")
	// API error should be nil - invalid YAML is a soft failure
	if err != nil {
		t.Fatalf("unexpected API error: %v", err)
	}

	// Should have defaults (not nil)
	if result.Config == nil {
		t.Fatal("expected default config, got nil")
	}

	// Should have parse warning
	if result.ParseWarning == nil {
		t.Fatal("expected parse warning for invalid YAML")
	}

	// Should have ConfigMapRef
	if result.ConfigMapRef == "" {
		t.Error("expected ConfigMapRef to be set")
	}
	expectedRef := "mco-system/mco-drain-config"
	if result.ConfigMapRef != expectedRef {
		t.Errorf("expected ConfigMapRef=%q, got %q", expectedRef, result.ConfigMapRef)
	}

	// Verify defaults are used
	if result.Config.Defaults.SkipToleratAllPods != DefaultSkipToleratAllPods {
		t.Errorf("expected default SkipToleratAllPods=%v", DefaultSkipToleratAllPods)
	}
}

func TestLoadDrainConfig_EmptyData(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mco-drain-config",
			Namespace: "mco-system",
			Labels: map[string]string{
				DrainConfigLabel: "true",
			},
		},
		Data: map[string]string{
			// Empty data or missing key
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	ctx := context.Background()

	result, err := LoadDrainConfig(ctx, c, "mco-system")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Empty data is not an error - no warning
	if result.ParseWarning != nil {
		t.Errorf("unexpected parse warning: %v", result.ParseWarning)
	}

	config := result.Config
	if config == nil {
		t.Fatal("expected non-nil config")
	}

	// Should return defaults
	if config.Defaults.SkipToleratAllPods != DefaultSkipToleratAllPods {
		t.Errorf("expected default SkipToleratAllPods=%v", DefaultSkipToleratAllPods)
	}
}

func TestLoadDrainConfig_ExplicitFalseSkipToleratAll(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mco-drain-config",
			Namespace: "mco-system",
			Labels: map[string]string{
				DrainConfigLabel: "true",
			},
		},
		Data: map[string]string{
			DrainConfigDataKey: `
defaults:
  skipToleratAllPods: false
`,
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	ctx := context.Background()

	result, err := LoadDrainConfig(ctx, c, "mco-system")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have no parse warning for valid YAML
	if result.ParseWarning != nil {
		t.Errorf("unexpected parse warning: %v", result.ParseWarning)
	}

	config := result.Config
	if config == nil {
		t.Fatal("expected non-nil config")
	}

	// Explicit false should be preserved
	if config.Defaults.SkipToleratAllPods {
		t.Error("expected SkipToleratAllPods=false")
	}
}

func TestDefaultDrainConfig(t *testing.T) {
	config := DefaultDrainConfig()

	// Default is false for backward compatibility: without ConfigMap, drain evicts all pods
	if config.Defaults.SkipToleratAllPods {
		t.Error("expected default SkipToleratAllPods=false for backward compatibility")
	}

	if config.Rules != nil {
		t.Error("expected nil rules")
	}
}
