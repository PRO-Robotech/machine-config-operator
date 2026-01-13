//go:build envtest && !unit && !e2e

// Package envtest provides integration tests using controller-runtime's envtest.
package envtest

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/tests/envtest/fixtures"
)

var _ = Describe("Apply Timeout Handling", func() {

	Context("Apply timeout marks node as Degraded", func() {

		It("should count apply-timed-out nodes as Degraded, not Updating", func() {
			poolName := uniqueName("pool-apply-timeout")

			// Create MachineConfig
			mc := fixtures.NewMC("mc-"+poolName).
				ForPool(poolName).
				WithFile("/etc/timeout-test.conf", "content", 0644).
				Build()
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, mc)
			})

			// Create Pool with short apply timeout (60 seconds)
			pool := fixtures.NewPool(poolName).
				WithApplyTimeout(60).
				Build()
			Expect(k8sClient.Create(ctx, pool)).To(Succeed())
			DeferCleanup(func() {
				_ = fixtures.CleanupPool(ctx, k8sClient, poolName)
			})

			// Wait for RMC to exist so we can set desired-revision to the real target.
			rmc := fixtures.WaitForRMC(ctx, k8sClient, poolName)
			Expect(rmc).NotTo(BeNil())

			nodeName := poolName + "-node-stuck"
			// Create node with "stuck" applying state - timestamp older than timeout
			// Using 2 minutes ago, with pool timeout of 60 seconds
			stuckTimestamp := time.Now().Add(-2 * time.Minute).Format(time.RFC3339)
			node := fixtures.NewNode(nodeName).
				ForPool(poolName).
				WithAnnotations(map[string]string{
					"mco.in-cloud.io/agent-state":             "applying",
					"mco.in-cloud.io/current-revision":        "old-rev",
					"mco.in-cloud.io/desired-revision":        rmc.Name,
					"mco.in-cloud.io/desired-revision-set-at": stuckTimestamp,
				}).
				Build()
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, node)
			})

			By("waiting for RMC to be created and pool to reconcile")
			Eventually(func() bool {
				mcp := &mcov1alpha1.MachineConfigPool{}
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: poolName}, mcp); err != nil {
					return false
				}
				return mcp.Status.MachineCount == 1
			}, testTimeout, testInterval).Should(BeTrue())

			By("verifying degradedMachineCount == 1")
			Eventually(func() int {
				mcp := &mcov1alpha1.MachineConfigPool{}
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: poolName}, mcp); err != nil {
					return -1
				}
				return mcp.Status.DegradedMachineCount
			}, testTimeout, testInterval).Should(Equal(1),
				"timed-out node should be counted as degraded")

			By("verifying updatingMachineCount == 0")
			mcp := &mcov1alpha1.MachineConfigPool{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: poolName}, mcp)).To(Succeed())
			Expect(mcp.Status.UpdatingMachineCount).To(Equal(0),
				"timed-out node should NOT be counted as updating")

			By("verifying Degraded=True condition")
			Expect(meta.IsStatusConditionTrue(mcp.Status.Conditions, "Degraded")).To(BeTrue(),
				"pool should have Degraded=True due to timed-out apply")
		})

		It("should not mark actively applying node as degraded within timeout", func() {
			poolName := uniqueName("pool-apply-active")

			// Create MachineConfig
			mc := fixtures.NewMC("mc-"+poolName).
				ForPool(poolName).
				WithFile("/etc/active-test.conf", "content", 0644).
				Build()
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, mc)
			})

			// Create Pool with timeout of 300 seconds (5 min)
			pool := fixtures.NewPool(poolName).
				WithApplyTimeout(300).
				Build()
			Expect(k8sClient.Create(ctx, pool)).To(Succeed())
			DeferCleanup(func() {
				_ = fixtures.CleanupPool(ctx, k8sClient, poolName)
			})

			// Wait for RMC so we can set desired-revision to the real target.
			rmc := fixtures.WaitForRMC(ctx, k8sClient, poolName)
			Expect(rmc).NotTo(BeNil())

			nodeName := poolName + "-node-active"
			// Create node with recent applying state - within timeout
			recentTimestamp := time.Now().Add(-10 * time.Second).Format(time.RFC3339)
			node := fixtures.NewNode(nodeName).
				ForPool(poolName).
				WithAnnotations(map[string]string{
					"mco.in-cloud.io/agent-state":             "applying",
					"mco.in-cloud.io/current-revision":        "old-rev",
					"mco.in-cloud.io/desired-revision":        rmc.Name,
					"mco.in-cloud.io/desired-revision-set-at": recentTimestamp,
				}).
				Build()
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, node)
			})

			By("waiting for pool to reconcile")
			Eventually(func() bool {
				mcp := &mcov1alpha1.MachineConfigPool{}
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: poolName}, mcp); err != nil {
					return false
				}
				return mcp.Status.MachineCount == 1
			}, testTimeout, testInterval).Should(BeTrue())

			By("verifying node is counted as updating, not degraded")
			Eventually(func() int {
				mcp := &mcov1alpha1.MachineConfigPool{}
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: poolName}, mcp); err != nil {
					return -1
				}
				return mcp.Status.UpdatingMachineCount
			}, testTimeout, testInterval).Should(Equal(1),
				"actively applying node should be counted as updating")

			mcp := &mcov1alpha1.MachineConfigPool{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: poolName}, mcp)).To(Succeed())
			Expect(mcp.Status.DegradedMachineCount).To(Equal(0),
				"actively applying node should NOT be counted as degraded")
		})
	})
})
