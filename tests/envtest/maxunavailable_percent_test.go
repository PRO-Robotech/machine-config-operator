//go:build envtest && !unit && !e2e

// Package envtest provides integration tests using controller-runtime's envtest.
package envtest

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"in-cloud.io/machine-config/tests/envtest/fixtures"
)

var _ = Describe("maxUnavailable Percentage", func() {

	Context("maxUnavailable percentage with ceil()", func() {

		It("should respect maxUnavailable=50% using ceil (4 nodes → max 2 cordoned)", func() {
			poolName := uniqueName("pool-maxu-50pct")

			// Create 4 nodes
			nodeNames := []string{
				poolName + "-node-0",
				poolName + "-node-1",
				poolName + "-node-2",
				poolName + "-node-3",
			}

			for _, nodeName := range nodeNames {
				node := fixtures.NewNode(nodeName).
					ForPool(poolName).
					Build()
				Expect(k8sClient.Create(ctx, node)).To(Succeed())
				DeferCleanup(func(name string) func() {
					return func() { _ = k8sClient.Delete(ctx, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}}) }
				}(nodeName))
			}

			// Create MachineConfig
			mc := fixtures.NewMC("mc-"+poolName).
				ForPool(poolName).
				WithFile("/etc/maxu50.conf", "content", 0644).
				Build()
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, mc)
			})

			// Create Pool with maxUnavailable=50%
			pool := fixtures.NewPool(poolName).
				WithMaxUnavailablePercent("50%").
				Build()
			Expect(k8sClient.Create(ctx, pool)).To(Succeed())
			DeferCleanup(func() {
				_ = fixtures.CleanupPool(ctx, k8sClient, poolName)
			})

			By("waiting for rollout to start (some nodes should be cordoned)")
			Eventually(func() int {
				return countCordonedNodes(nodeNames)
			}, testTimeout, testInterval).Should(BeNumerically(">=", 1), "at least one node should be cordoned")

			By("verifying never more than 2 nodes cordoned (ceil(4*0.5)=2)")
			// Check immediately and consistently over time
			Consistently(func() int {
				return countCordonedNodes(nodeNames)
			}, 10*time.Second, 500*time.Millisecond).Should(BeNumerically("<=", 2),
				"at most 2 nodes should be cordoned at any time with maxUnavailable=50%")
		})

		It("should respect maxUnavailable=25% using ceil (4 nodes → max 1 cordoned)", func() {
			poolName := uniqueName("pool-maxu-25pct")

			// Create 4 nodes
			nodeNames := []string{
				poolName + "-node-0",
				poolName + "-node-1",
				poolName + "-node-2",
				poolName + "-node-3",
			}

			for _, nodeName := range nodeNames {
				node := fixtures.NewNode(nodeName).
					ForPool(poolName).
					Build()
				Expect(k8sClient.Create(ctx, node)).To(Succeed())
				DeferCleanup(func(name string) func() {
					return func() { _ = k8sClient.Delete(ctx, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}}) }
				}(nodeName))
			}

			// Create MachineConfig
			mc := fixtures.NewMC("mc-"+poolName).
				ForPool(poolName).
				WithFile("/etc/maxu25.conf", "content", 0644).
				Build()
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, mc)
			})

			// Create Pool with maxUnavailable=25%
			pool := fixtures.NewPool(poolName).
				WithMaxUnavailablePercent("25%").
				Build()
			Expect(k8sClient.Create(ctx, pool)).To(Succeed())
			DeferCleanup(func() {
				_ = fixtures.CleanupPool(ctx, k8sClient, poolName)
			})

			By("waiting for rollout to start (some nodes should be cordoned)")
			Eventually(func() int {
				return countCordonedNodes(nodeNames)
			}, testTimeout, testInterval).Should(BeNumerically(">=", 1), "at least one node should be cordoned")

			By("verifying never more than 1 node cordoned (ceil(4*0.25)=1)")
			Consistently(func() int {
				return countCordonedNodes(nodeNames)
			}, 10*time.Second, 500*time.Millisecond).Should(BeNumerically("<=", 1),
				"at most 1 node should be cordoned at any time with maxUnavailable=25%")
		})

		It("should respect maxUnavailable=10% using ceil (10 nodes → max 1 cordoned)", func() {
			poolName := uniqueName("pool-maxu-10pct")

			// Create 10 nodes
			var nodeNames []string
			for i := 0; i < 10; i++ {
				nodeName := poolName + "-node-" + string(rune('a'+i))
				nodeNames = append(nodeNames, nodeName)
				node := fixtures.NewNode(nodeName).
					ForPool(poolName).
					Build()
				Expect(k8sClient.Create(ctx, node)).To(Succeed())
				DeferCleanup(func(name string) func() {
					return func() { _ = k8sClient.Delete(ctx, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}}) }
				}(nodeName))
			}

			// Create MachineConfig
			mc := fixtures.NewMC("mc-"+poolName).
				ForPool(poolName).
				WithFile("/etc/maxu10.conf", "content", 0644).
				Build()
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, mc)
			})

			// Create Pool with maxUnavailable=10%
			pool := fixtures.NewPool(poolName).
				WithMaxUnavailablePercent("10%").
				Build()
			Expect(k8sClient.Create(ctx, pool)).To(Succeed())
			DeferCleanup(func() {
				_ = fixtures.CleanupPool(ctx, k8sClient, poolName)
			})

			By("waiting for rollout to start")
			Eventually(func() int {
				return countCordonedNodes(nodeNames)
			}, testTimeout, testInterval).Should(BeNumerically(">=", 1))

			By("verifying never more than 1 node cordoned (ceil(10*0.1)=1)")
			Consistently(func() int {
				return countCordonedNodes(nodeNames)
			}, 10*time.Second, 500*time.Millisecond).Should(BeNumerically("<=", 1),
				"at most 1 node should be cordoned at any time with maxUnavailable=10%")
		})
	})
})

// countCordonedNodes counts how many nodes are currently cordoned (unschedulable).
func countCordonedNodes(nodeNames []string) int {
	count := 0
	for _, name := range nodeNames {
		node := &corev1.Node{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: name}, node); err != nil {
			continue
		}
		if node.Spec.Unschedulable {
			count++
		}
	}
	return count
}
