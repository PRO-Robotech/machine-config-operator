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
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// TestInterfaceCompliance verifies compile-time interface compliance.
// This is a compile-time check - if it compiles, the interfaces are implemented.
func TestInterfaceCompliance(t *testing.T) {
	// These lines will fail to compile if interfaces are not implemented
	var _ MCOClient = (*RuntimeClient)(nil)
	var _ RMCGetter = (*runtimeRMCGetter)(nil)
}

func TestNewRuntimeClient(t *testing.T) {
	// Arrange
	scheme := runtime.NewScheme()
	_ = mcov1alpha1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Act
	rc := NewRuntimeClient(fakeClient)

	// Assert
	if rc == nil {
		t.Fatal("NewRuntimeClient() returned nil")
	}
	if rc.client != fakeClient {
		t.Error("NewRuntimeClient() did not store the provided client")
	}
}

func TestRuntimeClient_RenderedMachineConfigs(t *testing.T) {
	// Arrange
	scheme := runtime.NewScheme()
	_ = mcov1alpha1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	rc := NewRuntimeClient(fakeClient)

	// Act
	getter := rc.RenderedMachineConfigs()

	// Assert
	if getter == nil {
		t.Fatal("RenderedMachineConfigs() returned nil")
	}
}

func TestRMCGetter_Get_Success(t *testing.T) {
	// Arrange
	scheme := runtime.NewScheme()
	_ = mcov1alpha1.AddToScheme(scheme)

	existingRMC := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-rmc",
		},
		Spec: mcov1alpha1.RenderedMachineConfigSpec{
			PoolName:   "worker",
			ConfigHash: "abc123def456abc123def456abc123def456abc123def456abc123def456abc123de",
			Revision:   "abc123def4",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existingRMC).
		Build()

	rc := NewRuntimeClient(fakeClient)
	getter := rc.RenderedMachineConfigs()
	ctx := context.Background()

	// Act
	rmc, err := getter.Get(ctx, "test-rmc", metav1.GetOptions{})

	// Assert
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if rmc == nil {
		t.Fatal("Get() returned nil RMC")
	}
	if rmc.Name != "test-rmc" {
		t.Errorf("Get() rmc.Name = %q, want %q", rmc.Name, "test-rmc")
	}
	if rmc.Spec.PoolName != "worker" {
		t.Errorf("Get() rmc.Spec.PoolName = %q, want %q", rmc.Spec.PoolName, "worker")
	}
	if rmc.Spec.ConfigHash != "abc123def456abc123def456abc123def456abc123def456abc123def456abc123de" {
		t.Errorf("Get() rmc.Spec.ConfigHash = %q, want expected hash", rmc.Spec.ConfigHash)
	}
}

func TestRMCGetter_Get_NotFound(t *testing.T) {
	// Arrange
	scheme := runtime.NewScheme()
	_ = mcov1alpha1.AddToScheme(scheme)

	// Create fake client without any RMC
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	rc := NewRuntimeClient(fakeClient)
	getter := rc.RenderedMachineConfigs()
	ctx := context.Background()

	// Act
	rmc, err := getter.Get(ctx, "nonexistent", metav1.GetOptions{})

	// Assert
	if err == nil {
		t.Fatal("Get() error = nil, want NotFound error")
	}
	if !apierrors.IsNotFound(err) {
		t.Errorf("Get() error = %v, want NotFound error", err)
	}
	if rmc != nil {
		t.Errorf("Get() returned non-nil RMC on error: %v", rmc)
	}
}

func TestRMCGetter_Get_OtherError(t *testing.T) {
	// Arrange
	scheme := runtime.NewScheme()
	_ = mcov1alpha1.AddToScheme(scheme)

	expectedErr := errors.New("network error")

	// Create a fake client with interceptor that returns an error
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, client client.WithWatch, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
				return expectedErr
			},
		}).
		Build()

	rc := NewRuntimeClient(fakeClient)
	getter := rc.RenderedMachineConfigs()
	ctx := context.Background()

	// Act
	rmc, err := getter.Get(ctx, "any", metav1.GetOptions{})

	// Assert
	if err == nil {
		t.Fatal("Get() error = nil, want error")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("Get() error = %v, want %v", err, expectedErr)
	}
	if rmc != nil {
		t.Errorf("Get() returned non-nil RMC on error: %v", rmc)
	}
}

func TestRMCGetter_Get_ClusterScoped(t *testing.T) {
	// This test verifies that Get works without namespace (cluster-scoped resource)
	// Arrange
	scheme := runtime.NewScheme()
	_ = mcov1alpha1.AddToScheme(scheme)

	// Track what NamespacedName was used
	var capturedKey types.NamespacedName

	existingRMC := &mcov1alpha1.RenderedMachineConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-rmc",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existingRMC).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
				capturedKey = key
				return c.Get(ctx, key, obj, opts...)
			},
		}).
		Build()

	rc := NewRuntimeClient(fakeClient)
	getter := rc.RenderedMachineConfigs()
	ctx := context.Background()

	// Act
	_, err := getter.Get(ctx, "cluster-rmc", metav1.GetOptions{})

	// Assert
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if capturedKey.Namespace != "" {
		t.Errorf("Get() used Namespace = %q, want empty (cluster-scoped)", capturedKey.Namespace)
	}
	if capturedKey.Name != "cluster-rmc" {
		t.Errorf("Get() used Name = %q, want %q", capturedKey.Name, "cluster-rmc")
	}
}
