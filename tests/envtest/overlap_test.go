//go:build envtest && !unit && !e2e

// Package envtest provides integration tests using controller-runtime's envtest.
package envtest

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/tests/envtest/fixtures"
)

var _ = Describe("Pool Overlap Detection", func() {

	Context("PoolOverlap condition", func() {

		It("should set PoolOverlap=True and Degraded=True when node matches multiple pools", func() {
			poolName1 := uniqueName("pool-overlap-1")
			poolName2 := uniqueName("pool-overlap-2")
			nodeName := uniqueName("node-conflict")
			overlapLabelKey := uniqueName("overlap-zone")
			overlapLabelValue := "us-east"

			// Create node that matches both pools
			node := fixtures.NewNode(nodeName).
				WithLabels(map[string]string{
					overlapLabelKey: overlapLabelValue,
				}).
				Build()
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, node)
			})

			// Create MachineConfig for pool1
			mc1 := fixtures.NewMC("mc-"+poolName1).
				ForPool(poolName1).
				WithFile("/etc/pool1.conf", "content1", 0644).
				Build()
			Expect(k8sClient.Create(ctx, mc1)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, mc1)
			})

			// Create MachineConfig for pool2
			mc2 := fixtures.NewMC("mc-"+poolName2).
				ForPool(poolName2).
				WithFile("/etc/pool2.conf", "content2", 0644).
				Build()
			Expect(k8sClient.Create(ctx, mc2)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, mc2)
			})

			// Create pool1 with same nodeSelector
			pool1 := fixtures.NewPool(poolName1).
				WithNodeSelector(map[string]string{overlapLabelKey: overlapLabelValue}).
				Build()
			Expect(k8sClient.Create(ctx, pool1)).To(Succeed())
			DeferCleanup(func() {
				_ = fixtures.CleanupPool(ctx, k8sClient, poolName1)
			})

			// Create pool2 with same nodeSelector (overlap!)
			pool2 := fixtures.NewPool(poolName2).
				WithNodeSelector(map[string]string{overlapLabelKey: overlapLabelValue}).
				Build()
			Expect(k8sClient.Create(ctx, pool2)).To(Succeed())
			DeferCleanup(func() {
				_ = fixtures.CleanupPool(ctx, k8sClient, poolName2)
			})

			By("waiting for PoolOverlap condition to be set on pool1")
			Eventually(func() bool {
				mcp := &mcov1alpha1.MachineConfigPool{}
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: poolName1}, mcp); err != nil {
					return false
				}
				return meta.IsStatusConditionTrue(mcp.Status.Conditions, "PoolOverlap")
			}, testTimeout, testInterval).Should(BeTrue(), "pool1 should have PoolOverlap=True")

			By("verifying PoolOverlap condition on pool2")
			mcp2 := &mcov1alpha1.MachineConfigPool{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: poolName2}, mcp2)).To(Succeed())
			Expect(meta.IsStatusConditionTrue(mcp2.Status.Conditions, "PoolOverlap")).To(BeTrue(),
				"pool2 should have PoolOverlap=True")

			By("verifying Degraded condition is set")
			mcp1 := &mcov1alpha1.MachineConfigPool{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: poolName1}, mcp1)).To(Succeed())
			Expect(meta.IsStatusConditionTrue(mcp1.Status.Conditions, "Degraded")).To(BeTrue(),
				"pool1 should have Degraded=True due to overlap")

			By("verifying PoolOverlap reason")
			cond := meta.FindStatusCondition(mcp1.Status.Conditions, "PoolOverlap")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Reason).To(Equal("NodesInMultiplePools"),
				"PoolOverlap reason should be NodesInMultiplePools")

			By("verifying conflicting node has no desired-revision")
			n := &corev1.Node{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: nodeName}, n)).To(Succeed())
			_, hasDesiredRevision := n.Annotations["mco.in-cloud.io/desired-revision"]
			Expect(hasDesiredRevision).To(BeFalse(),
				"conflicting node should NOT have desired-revision annotation")

			By("verifying MCP is not Ready while overlap exists")
			Expect(meta.IsStatusConditionTrue(mcp1.Status.Conditions, mcov1alpha1.ConditionReady)).To(BeFalse(),
				"pool1 should NOT have Ready=True while overlap exists")
		})
	})

	Context("PoolOverlap resolved", func() {

		It("should clear PoolOverlap after conflict resolved and proceed with node update", func() {
			poolName1 := uniqueName("pool-resolve-1")
			poolName2 := uniqueName("pool-resolve-2")
			nodeName := uniqueName("node-resolve")
			overlapLabelKey := uniqueName("resolve-zone")
			overlapLabelValue := "us-west"

			// Create node that matches both pools
			node := fixtures.NewNode(nodeName).
				WithLabels(map[string]string{
					overlapLabelKey: overlapLabelValue,
				}).
				Build()
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, node)
			})

			// Create MC for pool1
			mc1 := fixtures.NewMC("mc-"+poolName1).
				ForPool(poolName1).
				WithFile("/etc/resolve1.conf", "content1", 0644).
				Build()
			Expect(k8sClient.Create(ctx, mc1)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, mc1)
			})

			// Create MC for pool2
			mc2 := fixtures.NewMC("mc-"+poolName2).
				ForPool(poolName2).
				WithFile("/etc/resolve2.conf", "content2", 0644).
				Build()
			Expect(k8sClient.Create(ctx, mc2)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, mc2)
			})

			// Create both pools with same selector (overlap)
			pool1 := fixtures.NewPool(poolName1).
				WithNodeSelector(map[string]string{overlapLabelKey: overlapLabelValue}).
				Build()
			Expect(k8sClient.Create(ctx, pool1)).To(Succeed())
			DeferCleanup(func() {
				_ = fixtures.CleanupPool(ctx, k8sClient, poolName1)
			})

			pool2 := fixtures.NewPool(poolName2).
				WithNodeSelector(map[string]string{overlapLabelKey: overlapLabelValue}).
				Build()
			Expect(k8sClient.Create(ctx, pool2)).To(Succeed())
			// We'll delete pool2 to resolve overlap

			By("waiting for PoolOverlap to be detected")
			Eventually(func() bool {
				mcp := &mcov1alpha1.MachineConfigPool{}
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: poolName1}, mcp); err != nil {
					return false
				}
				return meta.IsStatusConditionTrue(mcp.Status.Conditions, "PoolOverlap")
			}, testTimeout, testInterval).Should(BeTrue())

			By("resolving overlap by deleting pool2")
			Expect(fixtures.CleanupPool(ctx, k8sClient, poolName2)).To(Succeed())

			By("waiting for PoolOverlap to be cleared")
			Eventually(func() bool {
				mcp := &mcov1alpha1.MachineConfigPool{}
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: poolName1}, mcp); err != nil {
					return false
				}
				return meta.IsStatusConditionFalse(mcp.Status.Conditions, "PoolOverlap")
			}, testTimeout, testInterval).Should(BeTrue(), "PoolOverlap should be cleared after removing pool2")

			By("waiting for node to receive desired-revision")
			Eventually(func() bool {
				n := &corev1.Node{}
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: nodeName}, n); err != nil {
					return false
				}
				_, hasDesiredRevision := n.Annotations["mco.in-cloud.io/desired-revision"]
				return hasDesiredRevision
			}, testTimeout, testInterval).Should(BeTrue(), "node should receive desired-revision after overlap resolved")

			By("verifying node gets cordoned (rollout started)")
			Eventually(func() bool {
				n := &corev1.Node{}
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: nodeName}, n); err != nil {
					return false
				}
				// Either unschedulable or cordoned annotation
				return n.Spec.Unschedulable || n.Annotations["mco.in-cloud.io/cordoned"] == "true"
			}, testTimeout, testInterval).Should(BeTrue(), "node should be cordoned after overlap resolved")
		})
	})
})
