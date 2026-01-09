//go:build envtest || e2e

package fixtures

import (
	"context"
	"time"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

// Default timeouts for wait operations
// Note: CRD has default debounceSeconds=30, so we need >30s timeout
const (
	DefaultTimeout  = 60 * time.Second
	DefaultInterval = 250 * time.Millisecond
)

// WaitForRMC waits for a RenderedMachineConfig to be created for a pool.
func WaitForRMC(ctx context.Context, c client.Client, poolName string) *mcov1alpha1.RenderedMachineConfig {
	var rmc *mcov1alpha1.RenderedMachineConfig
	Eventually(func() bool {
		rmcList := &mcov1alpha1.RenderedMachineConfigList{}
		if err := c.List(ctx, rmcList, client.MatchingLabels{"mco.in-cloud.io/pool": poolName}); err != nil {
			return false
		}
		if len(rmcList.Items) > 0 {
			rmc = &rmcList.Items[0]
			return true
		}
		return false
	}, DefaultTimeout, DefaultInterval).Should(BeTrue(), "RMC should be created for pool %s", poolName)
	return rmc
}

// WaitForPoolStatus waits for pool status to match a condition.
func WaitForPoolStatus(ctx context.Context, c client.Client, poolName string, condition func(*mcov1alpha1.MachineConfigPoolStatus) bool) {
	Eventually(func() bool {
		pool, err := GetPool(ctx, c, poolName)
		if err != nil {
			return false
		}
		return condition(&pool.Status)
	}, DefaultTimeout, DefaultInterval).Should(BeTrue(), "pool status condition should be met for %s", poolName)
}

// WaitForNodeAnnotation waits for a node to have a specific annotation value.
func WaitForNodeAnnotation(ctx context.Context, c client.Client, nodeName, key, expectedValue string) {
	Eventually(func() string {
		return GetNodeAnnotation(ctx, c, nodeName, key)
	}, DefaultTimeout, DefaultInterval).Should(Equal(expectedValue),
		"node %s should have annotation %s=%s", nodeName, key, expectedValue)
}

// WaitForNodeCordoned waits for a node to be cordoned (unschedulable).
func WaitForNodeCordoned(ctx context.Context, c client.Client, nodeName string) {
	Eventually(func() bool {
		node, err := GetNode(ctx, c, nodeName)
		if err != nil {
			return false
		}
		return node.Spec.Unschedulable
	}, DefaultTimeout, DefaultInterval).Should(BeTrue(),
		"node %s should be cordoned", nodeName)
}

// WaitForNodeUncordoned waits for a node to be uncordoned (schedulable).
func WaitForNodeUncordoned(ctx context.Context, c client.Client, nodeName string) {
	Eventually(func() bool {
		node, err := GetNode(ctx, c, nodeName)
		if err != nil {
			return false
		}
		return !node.Spec.Unschedulable
	}, DefaultTimeout, DefaultInterval).Should(BeTrue(),
		"node %s should be uncordoned", nodeName)
}
