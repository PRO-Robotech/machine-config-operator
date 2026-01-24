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

// Package drain provides drain exclusion configuration via ConfigMap.
package drain

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	DrainConfigLabel      = "mco.in-cloud.io/drain-config"
	DrainConfigLabelValue = "true"
	DrainConfigDataKey    = "config.yaml"
)

const (
	// False by default for backward compatibility - drain all pods without ConfigMap.
	DefaultSkipToleratAllPods = false
)

// LoadDrainConfigResult allows returning config with parse warnings (soft failure).
type LoadDrainConfigResult struct {
	Config       *DrainConfig
	ParseWarning error
	ConfigMapRef string
}

type DrainConfig struct {
	Defaults Defaults `json:"defaults,omitempty"`
	Rules    []Rule   `json:"rules,omitempty"` // OR logic between rules
}

type Defaults struct {
	SkipToleratAllPods bool `json:"skipToleratAllPods,omitempty"`
}

// Rule defines exclusion criteria. AND logic within rule, OR between rules.
type Rule struct {
	Namespaces        []string              `json:"namespaces,omitempty"`
	NamespacePrefixes []string              `json:"namespacePrefixes,omitempty"`
	PodNamePatterns   []string              `json:"podNamePatterns,omitempty"` // glob patterns
	PodSelector       *metav1.LabelSelector `json:"podSelector,omitempty"`
}

// LoadDrainConfig loads config from ConfigMap. Returns defaults on missing/invalid config.
func LoadDrainConfig(ctx context.Context, c client.Client, namespace string) (LoadDrainConfigResult, error) {
	cmList := &corev1.ConfigMapList{}
	if err := c.List(ctx, cmList,
		client.InNamespace(namespace),
		client.MatchingLabels{DrainConfigLabel: DrainConfigLabelValue},
	); err != nil {
		return LoadDrainConfigResult{}, fmt.Errorf("failed to list drain config ConfigMaps: %w", err)
	}

	if len(cmList.Items) == 0 {
		return LoadDrainConfigResult{Config: DefaultDrainConfig()}, nil
	}

	cm := &cmList.Items[0]
	configMapRef := fmt.Sprintf("%s/%s", cm.Namespace, cm.Name)

	data, ok := cm.Data[DrainConfigDataKey]
	if !ok || data == "" {
		return LoadDrainConfigResult{Config: DefaultDrainConfig()}, nil
	}

	var config DrainConfig
	if err := yaml.Unmarshal([]byte(data), &config); err != nil {
		return LoadDrainConfigResult{
			Config:       DefaultDrainConfig(),
			ParseWarning: fmt.Errorf("invalid YAML: %w", err),
			ConfigMapRef: configMapRef,
		}, nil
	}

	return LoadDrainConfigResult{Config: &config}, nil
}

func DefaultDrainConfig() *DrainConfig {
	return &DrainConfig{
		Defaults: Defaults{SkipToleratAllPods: DefaultSkipToleratAllPods},
	}
}
