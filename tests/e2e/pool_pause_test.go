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
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"in-cloud.io/machine-config/tests/testutil"
)

var _ = Describe("Pool Pause E2E", func() {

	BeforeEach(func() {
		// Clean up any leftover resources from previous test runs
		cleanupAllMCOResources()
	})

	Context("Pool paused blocks rollout", func() {

		It("should not rollout while pool is paused and should resume after unpause", func() {
			poolName := "e2e-pool-pause"

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

			By("creating MachineConfigPool with paused=true")
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
  paused: true
`, poolName, poolName)
			Expect(applyYAML(poolYAML)).To(Succeed())
			DeferCleanup(func() {
				_ = deleteResource("mcp", poolName)
			})

			By("creating MachineConfig that would trigger rollout")
			mcName := "mc-e2e-pause"
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
  - path: /etc/mco-test/pause-test.conf
    content: "paused pool test"
    mode: 0644
    owner: "root:root"
    state: present
`, mcName, poolName)
			Expect(applyYAML(mcYAML)).To(Succeed())
			DeferCleanup(func() {
				_ = deleteResource("mc", mcName)
			})

			By("verifying rollout is blocked while paused (nodes should not be cordoned)")
			Consistently(func() (int, error) {
				return countCordonedNodes(context.Background())
			}, 15*time.Second, 2*time.Second).Should(Equal(0),
				"no nodes should be cordoned while pool is paused")

			By("unpausing the pool")
			unpauseYAML := fmt.Sprintf(`
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
			Expect(applyYAML(unpauseYAML)).To(Succeed())

			By("waiting for rollout to complete after unpause")
			Eventually(func() bool {
				updated, _ := isPoolUpdated(context.Background(), poolName)
				return updated
			}, 5*time.Minute, 2*time.Second).Should(BeTrue(),
				"pool should become updated after unpause")

			By("verifying all nodes are updated")
			mcp, err := getMCP(poolName)
			Expect(err).NotTo(HaveOccurred())
			Expect(mcp.Status.UpdatedMachineCount).To(Equal(mcp.Status.MachineCount),
				"all machines should be updated after unpause")
		})
	})
})
