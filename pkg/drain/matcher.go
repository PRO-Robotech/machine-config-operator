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
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// Matches returns true if pod matches all specified conditions (AND logic).
func (r *Rule) Matches(pod *corev1.Pod) bool {
	if len(r.Namespaces) > 0 && !slices.Contains(r.Namespaces, pod.Namespace) {
		return false
	}

	if len(r.NamespacePrefixes) > 0 {
		matched := false
		for _, prefix := range r.NamespacePrefixes {
			if strings.HasPrefix(pod.Namespace, prefix) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	if len(r.PodNamePatterns) > 0 {
		matched := false
		for _, pattern := range r.PodNamePatterns {
			if m, _ := filepath.Match(pattern, pod.Name); m {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	if r.PodSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(r.PodSelector)
		if err != nil {
			return false
		}
		if !selector.Matches(labels.Set(pod.Labels)) {
			return false
		}
	}

	return true
}

func (c *DrainConfig) ShouldSkipPod(pod *corev1.Pod) (skip bool, reason string) {
	if c.Defaults.SkipToleratAllPods && HasToleratAllToleration(pod) {
		return true, "tolerate-all-pod"
	}

	for i, rule := range c.Rules {
		if rule.Matches(pod) {
			return true, fmt.Sprintf("rule[%d]", i)
		}
	}

	return false, ""
}

// HasToleratAllToleration returns true if pod tolerates all taints ({operator: Exists}).
// Such pods can stay on cordoned nodes, causing infinite drain loops.
func HasToleratAllToleration(pod *corev1.Pod) bool {
	for _, t := range pod.Spec.Tolerations {
		if t.Operator == corev1.TolerationOpExists && t.Key == "" {
			return true
		}
	}
	return false
}

func ToleratesUnschedulableTaint(pod *corev1.Pod) bool {
	for _, t := range pod.Spec.Tolerations {
		if t.Key == "node.kubernetes.io/unschedulable" {
			return true
		}
		if t.Operator == corev1.TolerationOpExists && t.Key == "" {
			return true
		}
	}
	return false
}
