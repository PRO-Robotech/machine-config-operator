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

package client

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// Compile-time interface compliance checks.
var (
	_ MCOClient = (*RuntimeClient)(nil)
	_ RMCGetter = (*runtimeRMCGetter)(nil)
)

// RuntimeClient implements MCOClient using controller-runtime's client.
type RuntimeClient struct {
	client client.Client
}

// NewRuntimeClient creates a new RuntimeClient wrapping the provided
// controller-runtime client.
func NewRuntimeClient(c client.Client) *RuntimeClient {
	return &RuntimeClient{client: c}
}

// RenderedMachineConfigs returns an RMCGetter for accessing RenderedMachineConfig resources.
func (r *RuntimeClient) RenderedMachineConfigs() RMCGetter {
	return &runtimeRMCGetter{client: r.client}
}

// runtimeRMCGetter implements RMCGetter using controller-runtime's client.
type runtimeRMCGetter struct {
	client client.Client
}

// Get retrieves a RenderedMachineConfig by name.
func (g *runtimeRMCGetter) Get(ctx context.Context, name string, _ metav1.GetOptions) (*mcov1alpha1.RenderedMachineConfig, error) {
	rmc := &mcov1alpha1.RenderedMachineConfig{}
	// RenderedMachineConfig is cluster-scoped, so Namespace is empty
	err := g.client.Get(ctx, types.NamespacedName{Name: name}, rmc)
	if err != nil {
		return nil, err
	}
	return rmc, nil
}
