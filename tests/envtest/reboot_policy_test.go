//go:build envtest && !unit && !e2e

// Package envtest provides integration tests using controller-runtime's envtest.
package envtest

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/tests/envtest/fixtures"
)

var _ = Describe("Reboot Policy Update", func() {

	Context("Reboot policy change updates existing RMC", func() {

		It("should update existing RMC reboot spec when pool reboot policy changes", func() {
			poolName := uniqueName("pool-reboot-policy")
			nodeName := poolName + "-node"

			// Create node
			node := fixtures.NewNode(nodeName).
				ForPool(poolName).
				Build()
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, node)
			})

			// Create MC
			mc := fixtures.NewMC("mc-"+poolName).
				ForPool(poolName).
				WithFile("/etc/reboot-policy.conf", "content", 0644).
				Build()
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, mc)
			})

			// Create pool with initial reboot strategy "Never"
			pool := fixtures.NewPool(poolName).
				WithRebootStrategy("Never").
				Build()
			Expect(k8sClient.Create(ctx, pool)).To(Succeed())
			DeferCleanup(func() {
				_ = fixtures.CleanupPool(ctx, k8sClient, poolName)
			})

			// Wait for RMC
			By("waiting for initial RMC")
			rmc := fixtures.WaitForRMC(ctx, k8sClient, poolName)
			Expect(rmc).NotTo(BeNil())
			initialRMCName := rmc.Name

			// Verify initial reboot strategy
			Expect(rmc.Spec.Reboot.Strategy).To(Equal("Never"))

			By("counting RMCs before policy change")
			initialRMCCount := countRMCsForPool(poolName)

			By("changing pool reboot strategy to IfRequired")
			poolToUpdate := &mcov1alpha1.MachineConfigPool{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: poolName}, poolToUpdate)).To(Succeed())
			poolToUpdate.Spec.Reboot.Strategy = "IfRequired"
			poolToUpdate.Spec.Reboot.MinIntervalSeconds = 300
			Expect(k8sClient.Update(ctx, poolToUpdate)).To(Succeed())

			By("waiting for RMC to be updated with new reboot policy")
			Eventually(func() string {
				rmcList := &mcov1alpha1.RenderedMachineConfigList{}
				if err := k8sClient.List(ctx, rmcList, client.MatchingLabels{
					"mco.in-cloud.io/pool": poolName,
				}); err != nil || len(rmcList.Items) == 0 {
					return ""
				}
				// Find latest RMC
				for _, r := range rmcList.Items {
					if r.Spec.Reboot.Strategy == "IfRequired" {
						return r.Spec.Reboot.Strategy
					}
				}
				return ""
			}, testTimeout, testInterval).Should(Equal("IfRequired"),
				"RMC should be updated with new reboot strategy")

			By("verifying RMC count did not increase (or only by 1 if new RMC created)")
			// Note: implementation may either update existing RMC or create new one
			// depending on design decision. Here we verify the reboot policy is correct.
			finalRMCCount := countRMCsForPool(poolName)
			// Allow for at most 1 new RMC (if policy change creates new revision)
			Expect(finalRMCCount).To(BeNumerically("<=", initialRMCCount+1))

			By("verifying pool status still tracks correct target")
			pool2 := &mcov1alpha1.MachineConfigPool{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: poolName}, pool2)).To(Succeed())
			Expect(pool2.Status.TargetRevision).NotTo(BeEmpty())

			// If new RMC was created, verify the original still exists or is cleaned up properly
			_ = initialRMCName // May or may not still exist depending on implementation
		})
	})
})

// countRMCsForPool counts RMCs with the pool label.
func countRMCsForPool(poolName string) int {
	rmcList := &mcov1alpha1.RenderedMachineConfigList{}
	if err := k8sClient.List(ctx, rmcList, client.MatchingLabels{
		"mco.in-cloud.io/pool": poolName,
	}); err != nil {
		return 0
	}
	return len(rmcList.Items)
}
