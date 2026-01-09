//go:build envtest && !unit && !e2e

package envtest

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/pkg/annotations"
	"in-cloud.io/machine-config/tests/envtest/fixtures"
)

var _ = Describe("MachineConfigPool Controller", func() {

	Context("Basic Reconciliation", func() {

		It("should create RMC when pool is created", func() {
			poolName := uniqueName("pool-basic")

			// Create Node
			node := fixtures.NewNode(poolName + "-node-0").
				ForPool(poolName).
				Build()
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, node)
			})

			// Create MachineConfig
			mc := fixtures.NewMC("mc-"+poolName).
				ForPool(poolName).
				WithPriority(50).
				WithFile("/etc/test-basic.conf", "key=value", 0644).
				Build()
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, mc)
			})

			// Create MachineConfigPool
			pool := fixtures.NewPool(poolName).Build()
			Expect(k8sClient.Create(ctx, pool)).To(Succeed())
			DeferCleanup(func() {
				_ = fixtures.CleanupPool(ctx, k8sClient, poolName)
			})

			// Verify RMC is created
			By("waiting for RMC to be created")
			rmc := fixtures.WaitForRMC(ctx, k8sClient, poolName)
			Expect(rmc).NotTo(BeNil())
			Expect(rmc.Labels["mco.in-cloud.io/pool"]).To(Equal(poolName))
			Expect(rmc.Spec.PoolName).To(Equal(poolName))

			// Verify RMC contains merged config
			Expect(len(rmc.Spec.Config.Files)).To(Equal(1))
			Expect(rmc.Spec.Config.Files[0].Path).To(Equal("/etc/test-basic.conf"))
			Expect(rmc.Spec.Config.Files[0].Content).To(Equal("key=value"))

			// Verify pool status updated
			By("waiting for pool status to be updated")
			fixtures.WaitForPoolStatus(ctx, k8sClient, poolName, func(status *mcov1alpha1.MachineConfigPoolStatus) bool {
				return status.MachineCount == 1 && status.TargetRevision != ""
			})
		})

		It("should set desired-revision annotation on nodes", func() {
			poolName := uniqueName("pool-desired")

			// Create Node
			node := fixtures.NewNode(poolName + "-node-0").
				ForPool(poolName).
				Build()
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, node)
			})

			// Create MachineConfig
			mc := fixtures.NewMC("mc-"+poolName).
				ForPool(poolName).
				WithFile("/etc/test-desired.conf", "test=true", 0644).
				Build()
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, mc)
			})

			// Create Pool
			pool := fixtures.NewPool(poolName).Build()
			Expect(k8sClient.Create(ctx, pool)).To(Succeed())
			DeferCleanup(func() {
				_ = fixtures.CleanupPool(ctx, k8sClient, poolName)
			})

			// Wait for RMC
			rmc := fixtures.WaitForRMC(ctx, k8sClient, poolName)

			// Verify node has desired-revision annotation
			By("waiting for node to have desired-revision annotation")
			fixtures.WaitForNodeAnnotation(ctx, k8sClient, node.Name,
				annotations.DesiredRevision, rmc.Name)

			// Verify node has pool annotation
			fixtures.WaitForNodeAnnotation(ctx, k8sClient, node.Name,
				annotations.Pool, poolName)
		})

		It("should cordon node before update", func() {
			poolName := uniqueName("pool-cordon")

			// Create Node with existing revision (simulating existing state)
			node := fixtures.NewNode(poolName + "-node-0").
				ForPool(poolName).
				WithCurrentRevision("old-revision").
				WithDesiredRevision("old-revision").
				WithAgentState("done").
				Build()
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, node)
			})

			// Create MachineConfig (new config that triggers update)
			mc := fixtures.NewMC("mc-"+poolName).
				ForPool(poolName).
				WithFile("/etc/test-cordon.conf", "new=config", 0644).
				Build()
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, mc)
			})

			// Create Pool
			pool := fixtures.NewPool(poolName).
				WithMaxUnavailable(1).
				Build()
			Expect(k8sClient.Create(ctx, pool)).To(Succeed())
			DeferCleanup(func() {
				_ = fixtures.CleanupPool(ctx, k8sClient, poolName)
			})

			// Wait for node to be cordoned
			By("waiting for node to be cordoned")
			fixtures.WaitForNodeCordoned(ctx, k8sClient, node.Name)

			// Verify cordon annotation is set
			fixtures.WaitForNodeAnnotation(ctx, k8sClient, node.Name,
				annotations.Cordoned, "true")
		})

	})

	Context("MaxUnavailable Enforcement", func() {

		It("should respect maxUnavailable=1 with multiple nodes", func() {
			poolName := uniqueName("pool-maxunavail")

			// Create 3 nodes with old revision
			nodes := make([]*corev1.Node, 3)
			for i := 0; i < 3; i++ {
				nodes[i] = fixtures.NewNode(fmt.Sprintf("%s-node-%d", poolName, i)).
					ForPool(poolName).
					WithCurrentRevision("old-revision").
					WithDesiredRevision("old-revision").
					WithAgentState("done").
					Build()
				Expect(k8sClient.Create(ctx, nodes[i])).To(Succeed())
				n := nodes[i] // capture for closure
				DeferCleanup(func() {
					_ = k8sClient.Delete(ctx, n)
				})
			}

			// Create MachineConfig
			mc := fixtures.NewMC("mc-"+poolName).
				ForPool(poolName).
				WithFile("/etc/test-maxunavail.conf", "updated", 0644).
				Build()
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, mc)
			})

			// Create Pool with maxUnavailable=1
			pool := fixtures.NewPool(poolName).
				WithMaxUnavailable(1).
				Build()
			Expect(k8sClient.Create(ctx, pool)).To(Succeed())
			DeferCleanup(func() {
				_ = fixtures.CleanupPool(ctx, k8sClient, poolName)
			})

			// Wait for some processing
			time.Sleep(1 * time.Second)

			// Verify only 1 node is cordoned at a time
			By("verifying only 1 node is cordoned")
			Consistently(func() int {
				return fixtures.CountCordonedNodes(ctx, k8sClient, poolName)
			}, 5*time.Second, 500*time.Millisecond).Should(BeNumerically("<=", 1),
				"at most 1 node should be cordoned at a time")
		})

	})

	Context("Status Aggregation", func() {

		It("should correctly count machines in pool", func() {
			poolName := uniqueName("pool-count")

			// Create 3 nodes
			for i := 0; i < 3; i++ {
				node := fixtures.NewNode(fmt.Sprintf("%s-node-%d", poolName, i)).
					ForPool(poolName).
					Build()
				Expect(k8sClient.Create(ctx, node)).To(Succeed())
				n := node // capture
				DeferCleanup(func() {
					_ = k8sClient.Delete(ctx, n)
				})
			}

			// Create MC and Pool
			mc := fixtures.NewMC("mc-"+poolName).
				ForPool(poolName).
				WithFile("/etc/test-count.conf", "x", 0644).
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

			// Verify machine count
			By("verifying machineCount = 3")
			fixtures.WaitForPoolStatus(ctx, k8sClient, poolName, func(status *mcov1alpha1.MachineConfigPoolStatus) bool {
				return status.MachineCount == 3
			})
		})

		It("should track updated nodes correctly", func() {
			poolName := uniqueName("pool-updated")

			// Create node
			node := fixtures.NewNode(poolName + "-node-0").
				ForPool(poolName).
				Build()
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, node)
			})

			// Create MC and Pool
			mc := fixtures.NewMC("mc-"+poolName).
				ForPool(poolName).
				WithFile("/etc/test-updated.conf", "y", 0644).
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

			// Simulate agent completing apply
			By("simulating agent apply completion")
			err := fixtures.SimulateAgentApply(ctx, k8sClient, node.Name, rmc.Name)
			Expect(err).NotTo(HaveOccurred())

			// Verify updated count
			By("verifying updatedMachineCount = 1")
			fixtures.WaitForPoolStatus(ctx, k8sClient, poolName, func(status *mcov1alpha1.MachineConfigPoolStatus) bool {
				return status.UpdatedMachineCount == 1 && status.ReadyMachineCount == 1
			})
		})

	})

	Context("Pause Functionality", func() {

		It("should skip paused pool reconciliation", func() {
			poolName := uniqueName("pool-paused")

			// Create Node
			node := fixtures.NewNode(poolName + "-node-0").
				ForPool(poolName).
				Build()
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, node)
			})

			// Create MC
			mc := fixtures.NewMC("mc-"+poolName).
				ForPool(poolName).
				WithFile("/etc/test-paused.conf", "z", 0644).
				Build()
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, mc)
			})

			// Create PAUSED Pool
			pool := fixtures.NewPool(poolName).
				WithPaused(true).
				Build()
			Expect(k8sClient.Create(ctx, pool)).To(Succeed())
			DeferCleanup(func() {
				_ = fixtures.CleanupPool(ctx, k8sClient, poolName)
			})

			// Verify no RMC is created (pool is paused)
			By("verifying no RMC is created for paused pool")
			Consistently(func() int {
				rmcList := &mcov1alpha1.RenderedMachineConfigList{}
				err := k8sClient.List(ctx, rmcList, client.MatchingLabels{
					"mco.in-cloud.io/pool": poolName,
				})
				if err != nil {
					return -1
				}
				return len(rmcList.Items)
			}, 3*time.Second, 500*time.Millisecond).Should(Equal(0),
				"no RMC should be created for paused pool")

			// Verify node has no desired-revision
			Consistently(func() string {
				return fixtures.GetNodeAnnotation(ctx, k8sClient, node.Name, annotations.DesiredRevision)
			}, 3*time.Second, 500*time.Millisecond).Should(BeEmpty(),
				"paused pool should not set desired-revision")
		})

	})

	Context("Empty Pool Handling", func() {

		It("should handle pool with no matching nodes", func() {
			poolName := uniqueName("pool-empty")

			// Create MC
			mc := fixtures.NewMC("mc-"+poolName).
				ForPool(poolName).
				WithFile("/etc/test-empty.conf", "empty", 0644).
				Build()
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, mc)
			})

			// Create Pool (no nodes match)
			pool := fixtures.NewPool(poolName).Build()
			Expect(k8sClient.Create(ctx, pool)).To(Succeed())
			DeferCleanup(func() {
				_ = fixtures.CleanupPool(ctx, k8sClient, poolName)
			})

			// RMC should still be created
			By("verifying RMC is created even for empty pool")
			rmc := fixtures.WaitForRMC(ctx, k8sClient, poolName)
			Expect(rmc).NotTo(BeNil())

			// Status should show 0 machines
			By("verifying pool status shows 0 machines")
			fixtures.WaitForPoolStatus(ctx, k8sClient, poolName, func(status *mcov1alpha1.MachineConfigPoolStatus) bool {
				return status.MachineCount == 0 &&
					status.TargetRevision == rmc.Name
			})
		})

	})

})
