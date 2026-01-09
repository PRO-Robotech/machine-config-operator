//go:build e2e
// +build e2e

/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

package e2e

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcov1alpha1 "in-cloud.io/machine-config/api/v1alpha1"
)

var _ = Describe("File Apply E2E", func() {

	BeforeEach(func() {
		// Clean up any leftover resources from previous test runs
		cleanupAllMCOResources()
	})

	Context("File present on all worker nodes", func() {

		It("should apply file content on all worker nodes via agent /host", func() {
			poolName := "e2e-file-apply"
			testFile := "/etc/mco-test/e2e-apply.conf"
			timestamp := time.Now().Format(time.RFC3339)

			By("creating MachineConfigPool")
			pool := createTestPool(poolName, 1)
			Expect(applyYAML(poolToYAML(pool))).To(Succeed())
			DeferCleanup(func() {
				_ = deleteResource("mcp", poolName)
			})

			By("creating MachineConfig with test file")
			mcName := "mc-e2e-file-apply"
			mcYAML := fmt.Sprintf(`
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: %s
  labels:
    mco.in-cloud.io/pool: %s
spec:
  priority: 50
  files:
  - path: %s
    content: |
      # E2E Test File
      timestamp=%s
      test=true
    mode: 0644
    owner: "root:root"
    state: present
`, mcName, poolName, testFile, timestamp)
			Expect(applyYAML(mcYAML)).To(Succeed())
			DeferCleanup(func() {
				_ = deleteResource("mc", mcName)
			})

			By("labeling worker nodes to join pool")
			workerNodes, err := getWorkerNodes()
			Expect(err).NotTo(HaveOccurred())
			Expect(workerNodes).NotTo(BeEmpty())

			for _, node := range workerNodes {
				Expect(labelNode(node, fmt.Sprintf("node-role.kubernetes.io/worker="))).To(Succeed())
			}

			By("waiting for pool to be fully updated")
			Eventually(func() bool {
				updated, err := isPoolUpdated(ctx, poolName)
				if err != nil {
					return false
				}
				return updated
			}, 5*time.Minute, 2*time.Second).Should(BeTrue(),
				"pool should become fully updated")

			By("getting agent pods by node")
			agentPods, err := listAgentPodsByNode()
			Expect(err).NotTo(HaveOccurred())
			Expect(agentPods).NotTo(BeEmpty())

			By("verifying file exists on all worker nodes")
			for _, nodeName := range workerNodes {
				podName, ok := agentPods[nodeName]
				if !ok {
					Skip(fmt.Sprintf("no agent pod found on node %s", nodeName))
				}

				exists, err := fileExistsOnHost(podName, testFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeTrue(),
					fmt.Sprintf("file %s should exist on node %s", testFile, nodeName))

				content, err := readFileOnHost(podName, testFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(content).To(ContainSubstring(timestamp),
					fmt.Sprintf("file content should contain timestamp on node %s", nodeName))
			}
		})
	})

	Context("File absent deletes file on all worker nodes", func() {

		It("should delete files with state=absent on all worker nodes", func() {
			poolName := "e2e-file-absent"
			testFile := "/etc/mco-test/e2e-absent.conf"

			By("creating MachineConfigPool")
			pool := createTestPool(poolName, 1)
			Expect(applyYAML(poolToYAML(pool))).To(Succeed())
			DeferCleanup(func() {
				_ = deleteResource("mcp", poolName)
			})

			By("step 1: creating MachineConfig with file present")
			mcName := "mc-e2e-file-absent"
			mcYAMLPresent := fmt.Sprintf(`
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: %s
  labels:
    mco.in-cloud.io/pool: %s
spec:
  priority: 50
  files:
  - path: %s
    content: "temporary file for deletion test"
    mode: 0644
    owner: "root:root"
    state: present
`, mcName, poolName, testFile)
			Expect(applyYAML(mcYAMLPresent)).To(Succeed())
			DeferCleanup(func() {
				_ = deleteResource("mc", mcName)
			})

			By("labeling worker nodes")
			workerNodes, err := getWorkerNodes()
			Expect(err).NotTo(HaveOccurred())

			for _, node := range workerNodes {
				Expect(labelNode(node, "node-role.kubernetes.io/worker=")).To(Succeed())
			}

			By("waiting for file to be created")
			Eventually(func() bool {
				updated, _ := isPoolUpdated(ctx, poolName)
				return updated
			}, 5*time.Minute, 2*time.Second).Should(BeTrue())

			By("capturing current target revision")
			mcpBefore, err := getMCP(poolName)
			Expect(err).NotTo(HaveOccurred())
			prevTarget := mcpBefore.Status.TargetRevision
			Expect(prevTarget).NotTo(BeEmpty())

			By("verifying file was created")
			agentPods, err := listAgentPodsByNode()
			Expect(err).NotTo(HaveOccurred())

			for _, nodeName := range workerNodes {
				if podName, ok := agentPods[nodeName]; ok {
					exists, err := fileExistsOnHost(podName, testFile)
					Expect(err).NotTo(HaveOccurred())
					Expect(exists).To(BeTrue(),
						fmt.Sprintf("file should exist before deletion on %s", nodeName))
				}
			}

			By("step 2: updating MachineConfig with state=absent")
			mcYAMLAbsent := fmt.Sprintf(`
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: %s
  labels:
    mco.in-cloud.io/pool: %s
spec:
  priority: 50
  files:
  - path: %s
    state: absent
`, mcName, poolName, testFile)
			Expect(applyYAML(mcYAMLAbsent)).To(Succeed())

			By("waiting for pool target revision to change (new RMC)")
			Eventually(func() string {
				mcp, err := getMCP(poolName)
				if err != nil {
					return ""
				}
				return mcp.Status.TargetRevision
			}, 2*time.Minute, 2*time.Second).ShouldNot(Equal(prevTarget))

			By("waiting for new revision to be fully applied")
			Eventually(func() bool {
				updated, _ := isPoolUpdated(ctx, poolName)
				return updated
			}, 5*time.Minute, 2*time.Second).Should(BeTrue())

			By("verifying file was deleted")
			for _, nodeName := range workerNodes {
				if podName, ok := agentPods[nodeName]; ok {
					Eventually(func() bool {
						exists, err := fileExistsOnHost(podName, testFile)
						if err != nil {
							return true
						}
						return exists
					}, 2*time.Minute, 2*time.Second).Should(BeFalse(),
						fmt.Sprintf("file should NOT exist after deletion on %s", nodeName))
				}
			}
		})
	})
})

// poolToYAML converts a MachineConfigPool to YAML string
func poolToYAML(pool *mcov1alpha1.MachineConfigPool) string {
	maxUnavailable := "1"
	if pool.Spec.Rollout.MaxUnavailable != nil {
		maxUnavailable = pool.Spec.Rollout.MaxUnavailable.String()
	}

	return fmt.Sprintf(`
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfigPool
metadata:
  name: %s
spec:
  nodeSelector:
    matchLabels:
      node-role.kubernetes.io/worker: ""
  machineConfigSelector:
    matchLabels:
      mco.in-cloud.io/pool: %s
  rollout:
    debounceSeconds: 1
    applyTimeoutSeconds: 300
    maxUnavailable: %s
  reboot:
    strategy: Never
  revisionHistory:
    limit: 3
  paused: false
`, pool.Name, pool.Name, maxUnavailable)
}

var ctx = context.Background()
