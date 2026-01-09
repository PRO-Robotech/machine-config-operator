//go:build envtest && !unit && !e2e

// Package envtest provides integration tests using controller-runtime's envtest.
package envtest

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/tests/envtest/fixtures"
)

var _ = Describe("RMC RevisionHistory Cleanup", func() {

	Context("RevisionHistory cleanup keeps in-use revisions", func() {

		It("should cleanup old RMCs but never delete revisions referenced by nodes", func() {
			poolName := uniqueName("pool-history")
			nodeName := poolName + "-node"

			// Create node
			node := fixtures.NewNode(nodeName).
				ForPool(poolName).
				Build()
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, node)
			})

			// Create pool with revisionHistory.limit=2
			pool := fixtures.NewPool(poolName).Build()
			// Set revision history limit to 2
			pool.Spec.RevisionHistory.Limit = 2
			Expect(k8sClient.Create(ctx, pool)).To(Succeed())
			DeferCleanup(func() {
				_ = fixtures.CleanupPool(ctx, k8sClient, poolName)
			})

			By("creating first MC and waiting for RMC")
			mc1 := fixtures.NewMC("mc1-"+poolName).
				ForPool(poolName).
				WithFile("/etc/history-v1.conf", "v1", 0644).
				Build()
			Expect(k8sClient.Create(ctx, mc1)).To(Succeed())

			rmc1 := fixtures.WaitForRMC(ctx, k8sClient, poolName)
			Expect(rmc1).NotTo(BeNil())
			rmc1Name := rmc1.Name

			By("simulating node with current-revision pointing to rmc1")
			Expect(retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				nodeToUpdate := &corev1.Node{}
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: nodeName}, nodeToUpdate); err != nil {
					return err
				}
				if nodeToUpdate.Annotations == nil {
					nodeToUpdate.Annotations = make(map[string]string)
				}
				nodeToUpdate.Annotations["mco.in-cloud.io/current-revision"] = rmc1Name
				nodeToUpdate.Annotations["mco.in-cloud.io/desired-revision"] = rmc1Name
				nodeToUpdate.Annotations["mco.in-cloud.io/agent-state"] = "done"
				return k8sClient.Update(ctx, nodeToUpdate)
			})).To(Succeed())

			By("creating multiple MC updates to generate more RMCs")
			// Wait for each MC change to be reflected as a new pool targetRevision
			// before creating the next MC. This avoids relying on RMC list counts,
			// which can shrink due to cleanup while we are generating history.
			prevTarget := rmc1Name
			for i := 2; i <= 5; i++ {
				mc := fixtures.NewMC(fmt.Sprintf("mc%d-%s", i, poolName)).
					ForPool(poolName).
					WithFile(fmt.Sprintf("/etc/history-v%d.conf", i), fmt.Sprintf("v%d", i), 0644).
					Build()
				Expect(k8sClient.Create(ctx, mc)).To(Succeed())
				DeferCleanup(func(m *mcov1alpha1.MachineConfig) func() {
					return func() { _ = k8sClient.Delete(ctx, m) }
				}(mc))

				// Wait for pool targetRevision to change (new RMC selected).
				var newTarget string
				Eventually(func() string {
					p := &mcov1alpha1.MachineConfigPool{}
					if err := k8sClient.Get(ctx, client.ObjectKey{Name: poolName}, p); err != nil {
						return ""
					}
					return p.Status.TargetRevision
				}, testTimeout, testInterval).ShouldNot(Equal(prevTarget))

				// Capture updated target for the next loop iteration.
				p := &mcov1alpha1.MachineConfigPool{}
				Expect(k8sClient.Get(ctx, client.ObjectKey{Name: poolName}, p)).To(Succeed())
				newTarget = p.Status.TargetRevision
				Expect(newTarget).NotTo(BeEmpty())
				prevTarget = newTarget
			}

			By("waiting for cleanup to run")
			Eventually(func() int {
				return countRMCsForPool(poolName)
			}, testTimeout, testInterval).Should(BeNumerically("<=", 4),
				"RMC count should be limited by revisionHistory")

			By("verifying in-use RMC is NOT deleted")
			rmcList := &mcov1alpha1.RenderedMachineConfigList{}
			Expect(k8sClient.List(ctx, rmcList, client.MatchingLabels{
				"mco.in-cloud.io/pool": poolName,
			})).To(Succeed())

			rmcNames := make([]string, 0, len(rmcList.Items))
			for _, r := range rmcList.Items {
				rmcNames = append(rmcNames, r.Name)
			}

			Expect(rmcNames).To(ContainElement(rmc1Name),
				"RMC referenced by node's current-revision should NOT be deleted")

			By("verifying pool targetRevision RMC exists")
			pool2 := &mcov1alpha1.MachineConfigPool{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: poolName}, pool2)).To(Succeed())
			Expect(rmcNames).To(ContainElement(pool2.Status.TargetRevision),
				"RMC referenced by pool's targetRevision should NOT be deleted")

			// Cleanup MC1
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, mc1)
			})
		})
	})
})
