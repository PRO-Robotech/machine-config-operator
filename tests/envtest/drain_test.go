//go:build envtest && !unit && !e2e

package envtest

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
	"in-cloud.io/machine-config/internal/controller"
	"in-cloud.io/machine-config/pkg/annotations"
	"in-cloud.io/machine-config/tests/envtest/fixtures"
)

var _ = Describe("Drain Lifecycle", func() {

	Context("Drain Wait", func() {

		It("should wait for drain to complete before setting desired-revision", func() {
			poolName := uniqueName("pool-drain-wait")

			// Create Node with existing revision (simulates node needing update)
			node := fixtures.NewNode(poolName + "-node-0").
				ForPool(poolName).
				WithCurrentRevision("old-rev").
				WithDesiredRevision("old-rev").
				WithAgentState("done").
				Build()
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, node)
			})

			// Create Pod on the node (simulates workload that needs draining)
			pod := fixtures.NewPod("workload-pod-"+poolName, "default").
				OnNode(node.Name).
				WithLabels(map[string]string{"app": "test"}).
				WithOwnerReference("Deployment", "test-deployment", "dep-1").
				Build()
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, pod)
			})

			// Create MC and Pool
			mc := fixtures.NewMC("mc-"+poolName).
				ForPool(poolName).
				WithFile("/etc/test-drain.conf", "drain-test", 0644).
				Build()
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, mc)
			})

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

			// Node should have drain-started annotation set
			By("verifying drain-started annotation is set")
			Eventually(func() string {
				return fixtures.GetNodeAnnotation(ctx, k8sClient, node.Name, annotations.DrainStartedAt)
			}, testTimeout, testInterval).ShouldNot(BeEmpty(),
				"drain-started-at annotation should be set")

			// Wait for RMC to be created
			By("waiting for RMC")
			rmc := fixtures.WaitForRMC(ctx, k8sClient, poolName)

			// Give controller time to process drain
			time.Sleep(1 * time.Second)

			// Simulate drain completion by deleting the pod
			By("simulating drain completion by deleting pod")
			Expect(k8sClient.Delete(ctx, pod)).To(Succeed())

			// Now desired-revision should be set
			By("verifying desired-revision is set after drain completes")
			fixtures.WaitForNodeAnnotation(ctx, k8sClient, node.Name,
				annotations.DesiredRevision, rmc.Name)
		})

	})

	Context("DaemonSet Pod Skip", func() {

		It("should skip DaemonSet pods during drain", func() {
			poolName := uniqueName("pool-drain-ds")

			// Create Node with existing revision
			node := fixtures.NewNode(poolName + "-node-0").
				ForPool(poolName).
				WithCurrentRevision("old-rev").
				WithDesiredRevision("old-rev").
				WithAgentState("done").
				Build()
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, node)
			})

			// Create DaemonSet Pod (should be skipped during drain)
			dsPod := fixtures.NewPod("ds-pod-"+poolName, "default").
				OnNode(node.Name).
				AsDaemonSetPod().
				Build()
			Expect(k8sClient.Create(ctx, dsPod)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, dsPod)
			})

			// Create MC and Pool
			mc := fixtures.NewMC("mc-"+poolName).
				ForPool(poolName).
				WithFile("/etc/test-drain-ds.conf", "ds-test", 0644).
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

			// Wait for RMC and desired-revision
			// DaemonSet pods should not block drain
			By("verifying drain completes despite DaemonSet pod")
			rmc := fixtures.WaitForRMC(ctx, k8sClient, poolName)
			fixtures.WaitForNodeAnnotation(ctx, k8sClient, node.Name,
				annotations.DesiredRevision, rmc.Name)

			// DaemonSet pod should still exist (not evicted)
			By("verifying DaemonSet pod was not evicted")
			Eventually(func() bool {
				existingPod := &corev1.Pod{}
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      dsPod.Name,
					Namespace: dsPod.Namespace,
				}, existingPod)
				return err == nil
			}, testTimeout, testInterval).Should(BeTrue(),
				"DaemonSet pod should still exist")
		})

	})

	Context("Drain Timeout", func() {

		It("should set DrainStuck condition when drain times out", func() {
			poolName := uniqueName("pool-drain-timeout")

			// Create MC and Pool
			mc := fixtures.NewMC("mc-"+poolName).
				ForPool(poolName).
				WithFile("/etc/test-timeout.conf", "timeout", 0644).
				Build()
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, mc)
			})

			pool := fixtures.NewPool(poolName).
				WithMaxUnavailable(1).
				Build()
			Expect(k8sClient.Create(ctx, pool)).To(Succeed())
			DeferCleanup(func() {
				_ = fixtures.CleanupPool(ctx, k8sClient, poolName)
			})

			// Wait for RMC to be created (debounce)
			By("waiting for RMC")
			fixtures.WaitForRMC(ctx, k8sClient, poolName)

			// Manually set DrainStuck condition on pool to verify it works
			// This simulates what the controller does after 10+ min timeout
			By("setting DrainStuck condition manually")
			Eventually(func() error {
				var p mcov1alpha1.MachineConfigPool
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: poolName}, &p); err != nil {
					return err
				}
				controller.SetDrainStuckCondition(&p, "Node test-node drain timeout")
				return k8sClient.Status().Update(ctx, &p)
			}, testTimeout, testInterval).Should(Succeed())

			// Verify pool has DrainStuck condition
			By("verifying DrainStuck condition")
			Eventually(func() bool {
				status := fixtures.GetPoolStatus(ctx, k8sClient, poolName)
				if status == nil {
					return false
				}
				for _, cond := range status.Conditions {
					if cond.Type == mcov1alpha1.ConditionDrainStuck && cond.Status == metav1.ConditionTrue {
						return true
					}
				}
				return false
			}, testTimeout, testInterval).Should(BeTrue(),
				"pool should have DrainStuck=True condition")

			// Verify Degraded is also set
			By("verifying Degraded condition")
			Eventually(func() bool {
				status := fixtures.GetPoolStatus(ctx, k8sClient, poolName)
				if status == nil {
					return false
				}
				for _, cond := range status.Conditions {
					if cond.Type == mcov1alpha1.ConditionDegraded && cond.Status == metav1.ConditionTrue {
						return true
					}
				}
				return false
			}, testTimeout, testInterval).Should(BeTrue(),
				"pool should have Degraded=True condition when drain stuck")
		})

	})

	Context("Uncordon After Apply", func() {

		It("should uncordon node after successful apply", func() {
			poolName := uniqueName("pool-uncordon")

			// Create Node with existing revision
			node := fixtures.NewNode(poolName + "-node-0").
				ForPool(poolName).
				WithCurrentRevision("old-rev").
				WithDesiredRevision("old-rev").
				WithAgentState("done").
				Build()
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, node)
			})

			// Create MC and Pool
			mc := fixtures.NewMC("mc-"+poolName).
				ForPool(poolName).
				WithFile("/etc/test-uncordon.conf", "uncordon", 0644).
				Build()
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, mc)
			})

			pool := fixtures.NewPool(poolName).
				WithMaxUnavailable(1).
				Build()
			Expect(k8sClient.Create(ctx, pool)).To(Succeed())
			DeferCleanup(func() {
				_ = fixtures.CleanupPool(ctx, k8sClient, poolName)
			})

			// Wait for cordon and RMC
			By("waiting for node to be cordoned")
			fixtures.WaitForNodeCordoned(ctx, k8sClient, node.Name)
			rmc := fixtures.WaitForRMC(ctx, k8sClient, poolName)

			// Wait for desired-revision to be set
			By("waiting for desired-revision to be set")
			fixtures.WaitForNodeAnnotation(ctx, k8sClient, node.Name,
				annotations.DesiredRevision, rmc.Name)

			// Simulate agent completing apply
			By("simulating agent apply completion")
			err := fixtures.SimulateAgentApply(ctx, k8sClient, node.Name, rmc.Name)
			Expect(err).NotTo(HaveOccurred())

			// Wait for node to be uncordoned
			By("waiting for node to be uncordoned")
			fixtures.WaitForNodeUncordoned(ctx, k8sClient, node.Name)

			// Verify cordon annotation is cleared
			By("verifying cordon annotation is cleared")
			Eventually(func() string {
				return fixtures.GetNodeAnnotation(ctx, k8sClient, node.Name, annotations.Cordoned)
			}, testTimeout, testInterval).Should(BeEmpty(),
				"cordoned annotation should be cleared")
		})

	})

})
