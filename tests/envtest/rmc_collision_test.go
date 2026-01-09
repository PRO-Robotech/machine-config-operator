//go:build envtest && !unit && !e2e

// Package envtest provides integration tests using controller-runtime's envtest.
package envtest

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/internal/renderer"
	"in-cloud.io/machine-config/tests/envtest/fixtures"
)

var _ = Describe("RMC Name Collision", func() {

	Context("RMC name collision resolution", func() {

		It("should resolve RMC name collision by creating a suffixed RMC", func() {
			poolName := uniqueName("pool-collision")
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
				WithFile("/etc/collision.conf", "content-v1", 0644).
				Build()
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, mc)
			})

			// Pre-create a "conflicting" RMC with the exact name the renderer would
			// generate for this pool+MC. This forces the controller collision path
			// (name already taken but hash differs), which should result in a suffixed RMC.
			pool := fixtures.NewPool(poolName).Build()
			merged := renderer.Merge([]*mcov1alpha1.MachineConfig{mc})
			expected := renderer.BuildRMC(poolName, merged, pool)
			conflictRMC := &mcov1alpha1.RenderedMachineConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: expected.Name,
					Labels: map[string]string{
						"mco.in-cloud.io/pool": poolName,
					},
				},
				Spec: mcov1alpha1.RenderedMachineConfigSpec{
					PoolName: poolName,
					// Must be valid 64-hex to pass CRD validation, but DIFFERENT from the real hash.
					Revision:   "0000000000",
					ConfigHash: "0000000000000000000000000000000000000000000000000000000000000000",
					Config:     expected.Spec.Config,
					Reboot:     expected.Spec.Reboot,
				},
			}
			Expect(k8sClient.Create(ctx, conflictRMC)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, conflictRMC)
			})

			// Create pool - should detect collision and create suffixed RMC
			Expect(k8sClient.Create(ctx, pool)).To(Succeed())
			DeferCleanup(func() {
				_ = fixtures.CleanupPool(ctx, k8sClient, poolName)
			})

			By("waiting for RMC to be created (may be suffixed)")
			Eventually(func() int {
				rmcList := &mcov1alpha1.RenderedMachineConfigList{}
				if err := k8sClient.List(ctx, rmcList, client.MatchingLabels{
					"mco.in-cloud.io/pool": poolName,
				}); err != nil {
					return 0
				}
				return len(rmcList.Items)
			}, testTimeout, testInterval).Should(BeNumerically(">=", 2),
				"should have at least 2 RMCs (original conflict + new)")

			By("verifying pool status has targetRevision set")
			pool2 := &mcov1alpha1.MachineConfigPool{}
			Eventually(func() string {
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: poolName}, pool2); err != nil {
					return ""
				}
				return pool2.Status.TargetRevision
			}, testTimeout, testInterval).ShouldNot(BeEmpty(),
				"pool should have targetRevision set")

			By("verifying targetRevision is not the conflict RMC")
			Expect(pool2.Status.TargetRevision).NotTo(Equal(conflictRMC.Name),
				"targetRevision should not point to conflict RMC with different hash")
		})
	})
})
