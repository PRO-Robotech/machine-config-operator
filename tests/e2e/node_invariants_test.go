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
)

var _ = Describe("Node Invariants E2E", func() {

	BeforeEach(func() {
		// Clean up any leftover resources from previous test runs
		cleanupAllMCOResources()
	})

	Context("Node invariants after rollout completes", func() {

		It("should converge node annotations after rollout completes", func() {
			poolName := "e2e-node-invariants"

			By("creating MachineConfigPool")
			pool := createTestPool(poolName, 1)
			Expect(applyYAML(poolToYAML(pool))).To(Succeed())
			DeferCleanup(func() {
				_ = deleteResource("mcp", poolName)
			})

			By("creating MachineConfig")
			mcName := "mc-e2e-invariants"
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
  - path: /etc/mco-test/invariants.conf
    content: "invariants test"
    mode: 0644
    owner: "root:root"
    state: present
`, mcName, poolName)
			Expect(applyYAML(mcYAML)).To(Succeed())
			DeferCleanup(func() {
				_ = deleteResource("mc", mcName)
			})

			By("getting worker nodes")
			workerNodes, err := getWorkerNodes()
			Expect(err).NotTo(HaveOccurred())
			Expect(workerNodes).NotTo(BeEmpty())

			By("waiting for rollout to complete")
			Eventually(func() bool {
				updated, _ := isPoolUpdated(context.Background(), poolName)
				return updated
			}, 5*time.Minute, 2*time.Second).Should(BeTrue(),
				"pool should become fully updated")

			By("getting pool target revision")
			mcp, err := getMCP(poolName)
			Expect(err).NotTo(HaveOccurred())
			targetRevision := mcp.Status.TargetRevision
			Expect(targetRevision).NotTo(BeEmpty())

			By("verifying node invariants on all worker nodes")
			for _, nodeName := range workerNodes {
				By(fmt.Sprintf("checking invariants on node %s", nodeName))

				// Node annotations can lag behind MCP status by a few seconds (watch/update ordering).
				// Wait until the node converges before asserting invariants.
				Eventually(func() bool {
					desiredRevision, _ := getNodeAnnotation(nodeName, "mco.in-cloud.io/desired-revision")
					currentRevision, _ := getNodeAnnotation(nodeName, "mco.in-cloud.io/current-revision")
					agentState, _ := getNodeAnnotation(nodeName, "mco.in-cloud.io/agent-state")
					unschedulable, _ := isNodeUnschedulable(nodeName)

					if desiredRevision == "" || currentRevision == "" {
						return false
					}
					if desiredRevision != currentRevision {
						return false
					}
					if agentState != "done" && agentState != "idle" {
						return false
					}
					if unschedulable {
						return false
					}
					return true
				}, 2*time.Minute, 2*time.Second).Should(BeTrue(),
					fmt.Sprintf("node %s should converge annotations/state after rollout", nodeName))

				// Re-read for clearer expectation failures.
				desiredRevision, _ := getNodeAnnotation(nodeName, "mco.in-cloud.io/desired-revision")
				currentRevision, _ := getNodeAnnotation(nodeName, "mco.in-cloud.io/current-revision")
				agentState, _ := getNodeAnnotation(nodeName, "mco.in-cloud.io/agent-state")
				unschedulable, _ := isNodeUnschedulable(nodeName)

				Expect(desiredRevision).To(Equal(currentRevision),
					fmt.Sprintf("node %s: desired-revision should equal current-revision", nodeName))

				// Invariant 2: agent-state == done/idle
				Expect(agentState).To(Or(Equal("done"), Equal("idle")),
					fmt.Sprintf("node %s: agent-state should be 'done' or 'idle'", nodeName))

				// Invariant 3: spec.unschedulable == false (not cordoned)
				Expect(unschedulable).To(BeFalse(),
					fmt.Sprintf("node %s: should not be cordoned after rollout", nodeName))

				// Invariant 4: no drain annotations
				cordoned, _ := getNodeAnnotation(nodeName, "mco.in-cloud.io/cordoned")
				Expect(cordoned).To(BeEmpty(),
					fmt.Sprintf("node %s: cordoned annotation should be cleared", nodeName))

				drainStarted, _ := getNodeAnnotation(nodeName, "mco.in-cloud.io/drain-started-at")
				Expect(drainStarted).To(BeEmpty(),
					fmt.Sprintf("node %s: drain-started-at annotation should be cleared", nodeName))

				drainRetry, _ := getNodeAnnotation(nodeName, "mco.in-cloud.io/drain-retry-count")
				Expect(drainRetry).To(BeEmpty(),
					fmt.Sprintf("node %s: drain-retry-count annotation should be cleared", nodeName))
			}
		})
	})

	Context("Status accuracy after rollout", func() {
		const poolName = "e2e-status-accuracy"
		const mcName = "e2e-status-accuracy-mc"

		AfterEach(func() {
			_ = deleteResource("mc", mcName)
			_ = deleteResource("mcp", poolName)
			uncordonAllWorkerNodes()
		})

		It("pool status should accurately reflect actual node counts", func() {
			By("creating MachineConfigPool and MachineConfig")
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
    maxUnavailable: 1
    debounceSeconds: 1
  reboot:
    strategy: Never
  paused: false
`, poolName, poolName)
			Expect(applyYAML(poolYAML)).To(Succeed())

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
    - path: /etc/mco-test/status-accuracy.txt
      content: "test file for status accuracy verification"
      mode: 0644
`, mcName, poolName)
			Expect(applyYAML(mcYAML)).To(Succeed())

			By("waiting for rollout to complete")
			Eventually(func() (bool, error) {
				return isPoolUpdated(context.Background(), poolName)
			}, 120*time.Second, 5*time.Second).Should(BeTrue(), "pool should complete rollout")

			By("verifying status counts match actual node state")
			machineCount := getPoolMachineCount(context.Background(), poolName)
			updatedCount := getPoolUpdatedMachineCount(context.Background(), poolName)

			Expect(machineCount).To(BeNumerically(">", 0), "machineCount should be greater than 0")
			Expect(updatedCount).To(Equal(machineCount),
				"updatedMachineCount should equal machineCount after successful rollout")

			By("verifying status matches actual node count")
			workers, err := getWorkerNodes()
			Expect(err).NotTo(HaveOccurred())
			Expect(machineCount).To(Equal(len(workers)),
				"machineCount should match actual worker node count")
		})
	})
})
