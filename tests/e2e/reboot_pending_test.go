//go:build e2e
// +build e2e

/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

package e2e

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"in-cloud.io/machine-config/tests/testutil"
)

var _ = Describe("Reboot Pending E2E", func() {

	BeforeEach(func() {
		// Clean up any leftover resources from previous test runs
		cleanupAllMCOResources()
	})

	Context("Reboot required with strategy=Never", func() {

		It("should set reboot-pending and keep node not updated when reboot is required and strategy=Never", func() {
			poolName := "e2e-reboot-pending"

			By("ensuring worker nodes are labeled")
			workerNodes, err := getWorkerNodes()
			if err != nil || len(workerNodes) == 0 {
				cmd := exec.Command("kubectl", "get", "nodes", "-o", "name")
				output, _ := testutil.Run(cmd)
				nodeNames := testutil.GetNonEmptyLines(output)
				for _, name := range nodeNames {
					nodeName := strings.TrimPrefix(name, "node/")
					if nodeName == "" || strings.Contains(nodeName, "control-plane") {
						continue
					}
					_ = labelNode(nodeName, "node-role.kubernetes.io/worker=")
				}
			}

			workerNodes, err = getWorkerNodes()
			Expect(err).NotTo(HaveOccurred())
			Expect(workerNodes).NotTo(BeEmpty(), "expected at least one worker node")

			By("creating MachineConfigPool with reboot strategy=Never")
			poolYAML := fmt.Sprintf(`
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
    maxUnavailable: 1
  reboot:
    strategy: Never
  revisionHistory:
    limit: 3
  paused: false
`, poolName, poolName)
			Expect(applyYAML(poolYAML)).To(Succeed())
			DeferCleanup(func() {
				_ = deleteResource("mcp", poolName)
			})

			By("creating MachineConfig with reboot.required=true")
			mcName := "mc-e2e-reboot-req"
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
  - path: /etc/mco-test/reboot-pending.conf
    content: "requires reboot"
    mode: 0644
    owner: "root:root"
    state: present
  reboot:
    required: true
    reason: "E2E test requires reboot"
`, mcName, poolName)
			Expect(applyYAML(mcYAML)).To(Succeed())
			DeferCleanup(func() {
				_ = deleteResource("mc", mcName)
			})

			By("waiting for at least one node to have reboot-pending=true")
			var nodeWithRebootPending string
			Eventually(func() bool {
				for _, nodeName := range workerNodes {
					rebootPending, _ := getNodeAnnotation(nodeName, "mco.in-cloud.io/reboot-pending")
					if rebootPending == "true" {
						nodeWithRebootPending = nodeName
						return true
					}
				}
				return false
			}, 3*time.Minute, 2*time.Second).Should(BeTrue(),
				"at least one node should have reboot-pending=true")

			By("verifying pool status shows pending reboot")
			mcp, err := getMCP(poolName)
			Expect(err).NotTo(HaveOccurred())
			Expect(mcp.Status.PendingRebootCount).To(BeNumerically(">=", 1),
				"pool should have pendingRebootCount >= 1")

			By("verifying rollout is not complete")
			Expect(mcp.Status.UpdatedMachineCount).To(BeNumerically("<", mcp.Status.MachineCount),
				"updatedMachineCount should be less than machineCount while reboot is pending")

			By(fmt.Sprintf("verifying file is applied on the node with reboot-pending (%s)", nodeWithRebootPending))
			agentPods, err := listAgentPodsByNode()
			Expect(err).NotTo(HaveOccurred())
			Expect(agentPods).NotTo(BeEmpty(), "expected to find agent pods")

			podName, ok := agentPods[nodeWithRebootPending]
			Expect(ok).To(BeTrue(), "expected to find agent pod for node %s", nodeWithRebootPending)

			exists, err := fileExistsOnHost(podName, "/etc/mco-test/reboot-pending.conf")
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue(),
				"file should be applied on node %s even if reboot is pending", nodeWithRebootPending)
		})
	})
})
