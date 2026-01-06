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

// Package client provides interfaces for accessing MCO resources.
package client

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// RMCGetter provides access to RenderedMachineConfig resources.
type RMCGetter interface {
	// Get retrieves a RenderedMachineConfig by name.
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*mcov1alpha1.RenderedMachineConfig, error)
}

// MCOClient provides access to MCO resources.
type MCOClient interface {
	// RenderedMachineConfigs returns the RMCGetter interface.
	RenderedMachineConfigs() RMCGetter
}
