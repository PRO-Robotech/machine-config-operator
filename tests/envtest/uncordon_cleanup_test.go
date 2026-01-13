//go:build envtest && !unit && !e2e

// Package envtest provides integration tests using controller-runtime's envtest.
package envtest

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"in-cloud.io/machine-config/tests/envtest/fixtures"
)

var _ = Describe("Uncordon Cleanup", func() {

	Context("Uncordon clears drain annotations", func() {

		It("should clear cordon+drain annotations when node is uncordoned", func() {
			poolName := uniqueName("pool-uncordon")
			nodeName := poolName + "-node"

			// First, create MC and pool to get an RMC
			mc := fixtures.NewMC("mc-"+poolName).
				ForPool(poolName).
				WithFile("/etc/uncordon-test.conf", "content", 0644).
				Build()
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, mc)
			})

			pool := fixtures.NewPool(poolName).Build()
			Expect(k8sClient.Create(ctx, pool)).To(Succeed())
			DeferCleanup(func() {
				_ = fixtures.CleanupPool(ctx, k8sClient, poolName)
			})

			// Wait for RMC to be created
			By("waiting for RMC to be created")
			rmc := fixtures.WaitForRMC(ctx, k8sClient, poolName)
			Expect(rmc).NotTo(BeNil())

			rmcName := rmc.Name

			// Create node with successful apply state but still cordoned
			// (simulating state after apply is done but before uncordon)
			node := fixtures.NewNode(nodeName).
				ForPool(poolName).
				WithAnnotations(map[string]string{
					"mco.in-cloud.io/current-revision": rmcName,
					"mco.in-cloud.io/desired-revision": rmcName,
					"mco.in-cloud.io/agent-state":      "done",
					"mco.in-cloud.io/pool":             poolName,
					// Drain annotations (should be cleared after uncordon)
					"mco.in-cloud.io/cordoned":          "true",
					"mco.in-cloud.io/drain-started-at":  time.Now().Add(-5 * time.Minute).Format(time.RFC3339),
					"mco.in-cloud.io/drain-retry-count": "3",
				}).
				WithUnschedulable(true).
				Build()
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, node)
			})

			By("waiting for node to be uncordoned")
			Eventually(func() bool {
				n := &corev1.Node{}
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: nodeName}, n); err != nil {
					return false
				}
				return !n.Spec.Unschedulable
			}, testTimeout, testInterval).Should(BeTrue(),
				"node should be uncordoned after successful apply")

			By("verifying all drain annotations are cleared")
			n := &corev1.Node{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: nodeName}, n)).To(Succeed())

			// Verify node is schedulable
			Expect(n.Spec.Unschedulable).To(BeFalse(),
				"node should be schedulable after uncordon")

			// Verify drain annotations are cleared
			Expect(n.Annotations).NotTo(HaveKey("mco.in-cloud.io/cordoned"),
				"cordoned annotation should be cleared")
			Expect(n.Annotations).NotTo(HaveKey("mco.in-cloud.io/drain-started-at"),
				"drain-started-at annotation should be cleared")
			Expect(n.Annotations).NotTo(HaveKey("mco.in-cloud.io/drain-retry-count"),
				"drain-retry-count annotation should be cleared")

			// Verify MCO annotations are preserved
			Expect(n.Annotations).To(HaveKeyWithValue("mco.in-cloud.io/current-revision", rmcName),
				"current-revision should be preserved")
			Expect(n.Annotations).To(HaveKeyWithValue("mco.in-cloud.io/agent-state", "done"),
				"agent-state should still be done")
		})

		It("should NOT uncordon node that is still applying", func() {
			poolName := uniqueName("pool-no-uncordon")
			nodeName := poolName + "-node"

			// Create MC and pool
			mc := fixtures.NewMC("mc-"+poolName).
				ForPool(poolName).
				WithFile("/etc/no-uncordon-test.conf", "content", 0644).
				Build()
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, mc)
			})

			pool := fixtures.NewPool(poolName).Build()
			Expect(k8sClient.Create(ctx, pool)).To(Succeed())
			DeferCleanup(func() {
				_ = fixtures.CleanupPool(ctx, k8sClient, poolName)
			})

			// Wait for RMC
			rmc := fixtures.WaitForRMC(ctx, k8sClient, poolName)
			Expect(rmc).NotTo(BeNil())
			rmcName := rmc.Name

			// Create node that is still applying (current != desired)
			node := fixtures.NewNode(nodeName).
				ForPool(poolName).
				WithAnnotations(map[string]string{
					"mco.in-cloud.io/current-revision": "old-rmc",
					"mco.in-cloud.io/desired-revision": rmcName,
					"mco.in-cloud.io/agent-state":      "applying",
					"mco.in-cloud.io/pool":             poolName,
					"mco.in-cloud.io/cordoned":         "true",
				}).
				WithUnschedulable(true).
				Build()
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, node)
			})

			By("verifying node remains cordoned while applying")
			Consistently(func() bool {
				n := &corev1.Node{}
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: nodeName}, n); err != nil {
					return true
				}
				return n.Spec.Unschedulable
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"node should remain cordoned while still applying")
		})
	})
})
