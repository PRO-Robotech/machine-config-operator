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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRule_Matches_Namespace(t *testing.T) {
	rule := Rule{
		Namespaces: []string{"kube-system", "kube-public"},
	}

	tests := []struct {
		name      string
		namespace string
		expected  bool
	}{
		{"exact match kube-system", "kube-system", true},
		{"exact match kube-public", "kube-public", true},
		{"no match default", "default", false},
		{"no match monitoring", "monitoring", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: tt.namespace,
				},
			}
			if result := rule.Matches(pod); result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestRule_Matches_NamespacePrefix(t *testing.T) {
	rule := Rule{
		NamespacePrefixes: []string{"kube-", "in-cloud-"},
	}

	tests := []struct {
		name      string
		namespace string
		expected  bool
	}{
		{"prefix kube-system", "kube-system", true},
		{"prefix kube-public", "kube-public", true},
		{"prefix in-cloud-system", "in-cloud-system", true},
		{"no match default", "default", false},
		{"no match monitoring", "monitoring", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: tt.namespace,
				},
			}
			if result := rule.Matches(pod); result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestRule_Matches_PodNamePattern(t *testing.T) {
	rule := Rule{
		PodNamePatterns: []string{"netshoot-*", "debug-*", "busybox-?"},
	}

	tests := []struct {
		name     string
		podName  string
		expected bool
	}{
		{"match netshoot-abc", "netshoot-abc", true},
		{"match netshoot-123", "netshoot-123", true},
		{"match debug-pod", "debug-pod", true},
		{"match busybox-1", "busybox-1", true},
		{"no match busybox-12", "busybox-12", false}, // ? matches single char
		{"no match nginx", "nginx", false},
		{"no match netshoot", "netshoot", false}, // no dash
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.podName,
					Namespace: "default",
				},
			}
			if result := rule.Matches(pod); result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestRule_Matches_PodSelector(t *testing.T) {
	rule := Rule{
		PodSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app": "netshoot",
			},
		},
	}

	tests := []struct {
		name     string
		labels   map[string]string
		expected bool
	}{
		{"match app=netshoot", map[string]string{"app": "netshoot"}, true},
		{"match with extra labels", map[string]string{"app": "netshoot", "env": "test"}, true},
		{"no match app=nginx", map[string]string{"app": "nginx"}, false},
		{"no match empty labels", map[string]string{}, false},
		{"no match nil labels", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Labels:    tt.labels,
				},
			}
			if result := rule.Matches(pod); result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestRule_Matches_Combo(t *testing.T) {
	// Rule with multiple conditions (AND logic)
	rule := Rule{
		Namespaces:      []string{"monitoring"},
		PodNamePatterns: []string{"netshoot-*"},
	}

	tests := []struct {
		name      string
		namespace string
		podName   string
		expected  bool
	}{
		{"both match", "monitoring", "netshoot-abc", true},
		{"namespace only", "monitoring", "prometheus", false},
		{"pattern only", "default", "netshoot-abc", false},
		{"neither match", "default", "nginx", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.podName,
					Namespace: tt.namespace,
				},
			}
			if result := rule.Matches(pod); result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestRule_Matches_EmptyRule(t *testing.T) {
	// Empty rule should match all pods
	rule := Rule{}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "any-pod",
			Namespace: "any-namespace",
		},
	}

	if !rule.Matches(pod) {
		t.Error("empty rule should match all pods")
	}
}

func TestRule_Matches_InvalidSelector(t *testing.T) {
	// Invalid selector should not match
	rule := Rule{
		PodSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "app",
					Operator: "InvalidOperator", // Invalid operator
					Values:   []string{"test"},
				},
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
	}

	if rule.Matches(pod) {
		t.Error("invalid selector should not match")
	}
}

func TestDrainConfig_ShouldSkipPod_RulesOR(t *testing.T) {
	config := &DrainConfig{
		Defaults: Defaults{
			SkipToleratAllPods: false,
		},
		Rules: []Rule{
			{Namespaces: []string{"kube-system"}},
			{PodNamePatterns: []string{"debug-*"}},
		},
	}

	tests := []struct {
		name      string
		namespace string
		podName   string
		skip      bool
		reason    string
	}{
		{"match first rule", "kube-system", "coredns", true, "rule[0]"},
		{"match second rule", "default", "debug-pod", true, "rule[1]"},
		{"no match", "default", "nginx", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.podName,
					Namespace: tt.namespace,
				},
			}
			skip, reason := config.ShouldSkipPod(pod)
			if skip != tt.skip {
				t.Errorf("expected skip=%v, got %v", tt.skip, skip)
			}
			if reason != tt.reason {
				t.Errorf("expected reason=%q, got %q", tt.reason, reason)
			}
		})
	}
}

func TestDrainConfig_ShouldSkipPod_NoMatch(t *testing.T) {
	config := &DrainConfig{
		Defaults: Defaults{
			SkipToleratAllPods: false,
		},
		Rules: []Rule{
			{Namespaces: []string{"kube-system"}},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: "default",
		},
	}

	skip, reason := config.ShouldSkipPod(pod)
	if skip {
		t.Error("expected skip=false for non-matching pod")
	}
	if reason != "" {
		t.Errorf("expected empty reason, got %q", reason)
	}
}

func TestDrainConfig_ShouldSkipPod_ToleratAll(t *testing.T) {
	config := &DrainConfig{
		Defaults: Defaults{
			SkipToleratAllPods: true,
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "netshoot",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Tolerations: []corev1.Toleration{
				{Operator: corev1.TolerationOpExists},
			},
		},
	}

	skip, reason := config.ShouldSkipPod(pod)
	if !skip {
		t.Error("expected skip=true for tolerate-all pod")
	}
	if reason != "tolerate-all-pod" {
		t.Errorf("expected reason='tolerate-all-pod', got %q", reason)
	}
}

func TestDrainConfig_ShouldSkipPod_ToleratAll_Disabled(t *testing.T) {
	config := &DrainConfig{
		Defaults: Defaults{
			SkipToleratAllPods: false,
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "netshoot",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Tolerations: []corev1.Toleration{
				{Operator: corev1.TolerationOpExists},
			},
		},
	}

	skip, _ := config.ShouldSkipPod(pod)
	if skip {
		t.Error("expected skip=false when SkipToleratAllPods is disabled")
	}
}

func TestHasToleratAllToleration(t *testing.T) {
	tests := []struct {
		name        string
		tolerations []corev1.Toleration
		expected    bool
	}{
		{
			name:        "nil tolerations",
			tolerations: nil,
			expected:    false,
		},
		{
			name:        "empty tolerations",
			tolerations: []corev1.Toleration{},
			expected:    false,
		},
		{
			name: "tolerate-all operator=Exists",
			tolerations: []corev1.Toleration{
				{Operator: corev1.TolerationOpExists},
			},
			expected: true,
		},
		{
			name: "explicit key with Exists",
			tolerations: []corev1.Toleration{
				{Key: "node.kubernetes.io/not-ready", Operator: corev1.TolerationOpExists},
			},
			expected: false,
		},
		{
			name: "normal toleration",
			tolerations: []corev1.Toleration{
				{Key: "dedicated", Operator: corev1.TolerationOpEqual, Value: "special-user"},
			},
			expected: false,
		},
		{
			name: "mixed with tolerate-all",
			tolerations: []corev1.Toleration{
				{Key: "dedicated", Operator: corev1.TolerationOpEqual, Value: "special-user"},
				{Operator: corev1.TolerationOpExists}, // tolerate-all
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				Spec: corev1.PodSpec{
					Tolerations: tt.tolerations,
				},
			}
			if result := HasToleratAllToleration(pod); result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestToleratesUnschedulableTaint(t *testing.T) {
	tests := []struct {
		name        string
		tolerations []corev1.Toleration
		expected    bool
	}{
		{
			name:        "nil tolerations",
			tolerations: nil,
			expected:    false,
		},
		{
			name: "explicit unschedulable toleration",
			tolerations: []corev1.Toleration{
				{Key: "node.kubernetes.io/unschedulable", Operator: corev1.TolerationOpExists},
			},
			expected: true,
		},
		{
			name: "tolerate-all",
			tolerations: []corev1.Toleration{
				{Operator: corev1.TolerationOpExists},
			},
			expected: true,
		},
		{
			name: "different key",
			tolerations: []corev1.Toleration{
				{Key: "node.kubernetes.io/not-ready", Operator: corev1.TolerationOpExists},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				Spec: corev1.PodSpec{
					Tolerations: tt.tolerations,
				},
			}
			if result := ToleratesUnschedulableTaint(pod); result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
