//go:build envtest && !unit && !e2e

// Package envtest provides integration tests using controller-runtime's envtest.
// These tests run against a real API server (etcd) without requiring a full cluster.
package envtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/internal/controller"
)

// Test configuration
const (
	// Timeouts for test assertions
	// Note: CRD has default debounceSeconds=30, so we need >30s timeout
	testTimeout  = 60 * time.Second
	testInterval = 250 * time.Millisecond

	// Longer timeout for operations like drain
	longTimeout = 2 * time.Minute
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
)

func TestEnvTest(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "EnvTest Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.Background())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// Register MCO API types
	err = mcov1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// Create client for test assertions
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Create manager for controller
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // Disable metrics server in tests
		},
		// LeaderElection disabled for tests
		LeaderElection: false,
	})
	Expect(err).NotTo(HaveOccurred())

	// CRITICAL: Register Pod indexer for drain functionality
	// This allows listing pods by spec.nodeName
	err = mgr.GetFieldIndexer().IndexField(
		ctx,
		&corev1.Pod{},
		"spec.nodeName",
		func(obj client.Object) []string {
			pod, ok := obj.(*corev1.Pod)
			if !ok || pod.Spec.NodeName == "" {
				return nil
			}
			return []string{pod.Spec.NodeName}
		},
	)
	Expect(err).NotTo(HaveOccurred())

	// Setup MachineConfigPool reconciler
	reconciler := controller.NewMachineConfigPoolReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
	)
	err = reconciler.SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	// Start manager in background
	go func() {
		defer GinkgoRecover()
		err = mgr.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
	}()

	// ANTI-FLAKE: Wait for manager cache to sync
	By("waiting for manager cache to sync")
	Eventually(func() bool {
		// Check cache has synced by trying to list resources
		nodeList := &corev1.NodeList{}
		if err := k8sClient.List(ctx, nodeList); err != nil {
			return false
		}
		// Additional check: try to list MCPs
		mcpList := &mcov1alpha1.MachineConfigPoolList{}
		if err := k8sClient.List(ctx, mcpList); err != nil {
			return false
		}
		return true
	}, testTimeout, testInterval).Should(BeTrue(), "Manager cache did not sync in time")
})

var _ = AfterSuite(func() {
	By("cancelling context")
	cancel()

	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// uniqueName generates a unique name for test resources.
// Uses timestamp + random suffix to avoid collisions in parallel tests.
func uniqueName(prefix string) string {
	// Generate 4 random bytes for uniqueness
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Fallback to time-only if crypto/rand fails
		return prefix + "-" + time.Now().Format("150405")
	}
	return prefix + "-" + time.Now().Format("150405") + "-" + hex.EncodeToString(b)
}
