//go:build e2e
// +build e2e

/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"in-cloud.io/machine-config/tests/testutil"
)

var _ = Describe("Drain Stuck", Ordered, func() {
	var (
		ctx        context.Context
		poolName   = "drain-test-workers"
		pdbName    = "blocking-pdb"
		deployName = "blocking-app"
		testNode   string
		testNs     = "default"
	)

	BeforeAll(func() {
		ctx = context.Background()

		// Clean up any leftover resources from previous test runs
		cleanupAllMCOResources()

		By("finding a worker node that is NOT running the controller")
		// The controller skips drain for its own node to prevent self-disruption.
		// We need to find a worker node that is NOT hosting the controller pod.

		// Get the node where controller is running
		cmd := exec.Command("kubectl", "get", "pod", "-n", "machine-config-system",
			"-l", "control-plane=controller-manager", "-o", "jsonpath={.items[0].spec.nodeName}")
		controllerNode, err := testutil.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		By(fmt.Sprintf("Controller is running on node: %s", controllerNode))

		// Get all worker nodes (not control-plane)
		cmd = exec.Command("kubectl", "get", "nodes",
			"-l", "!node-role.kubernetes.io/control-plane", "-o", "name")
		output, err := testutil.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		workerNodes := testutil.GetNonEmptyLines(output)
		Expect(len(workerNodes)).To(BeNumerically(">=", 2), "Need at least 2 worker nodes")

		// Find a worker node that is NOT the controller node
		for _, nodeName := range workerNodes {
			name := nodeName[5:] // strip "node/"
			if name != controllerNode {
				testNode = name
				break
			}
		}
		Expect(testNode).NotTo(BeEmpty(), "Could not find a worker node that is not the controller node")
		By(fmt.Sprintf("Using node %s for drain test (controller is on %s)", testNode, controllerNode))

		By("labeling test node")
		Expect(labelNode(testNode, "node-role.kubernetes.io/drain-test=")).To(Succeed())
	})

	AfterAll(func() {
		By("cleaning up all test resources")
		_ = deleteResource("mcp", poolName)
		_ = deleteResource("mc", "drain-test-mc")
		_ = deleteResource("deployment", deployName)
		_ = deleteResource("pdb", pdbName)
		_ = unlabelNode(testNode, "node-role.kubernetes.io/drain-test")

		By("uncordoning test node if cordoned")
		_ = uncordonNode(testNode)
	})

	Context("when PDB blocks drain", func() {
		BeforeEach(func() {
			By("cleaning up any leftover resources from previous tests")
			_ = deleteResource("mc", "drain-test-mc")
			_ = deleteResource("mcp", poolName)
			_ = deleteResource("pdb", pdbName)
			_ = deleteResource("deployment", deployName)
			_ = uncordonNode(testNode)

			By("setting up node as previously managed by MCO (not a new node)")
			// The controller skips cordon/drain for newly added nodes.
			// To trigger the drain flow, we need the node to appear as previously
			// managed by MCO with an old revision that differs from the target.
			_ = clearNodeMCOAnnotations(testNode)
			// Set a dummy current-revision so the node is not considered "new"
			cmd := exec.Command("kubectl", "annotate", "node", testNode,
				"mco.in-cloud.io/current-revision=old-revision-before-drain-test", "--overwrite")
			_, _ = testutil.Run(cmd)
			// Set pool annotation
			cmd = exec.Command("kubectl", "annotate", "node", testNode,
				"mco.in-cloud.io/pool="+poolName, "--overwrite")
			_, _ = testutil.Run(cmd)
			// Set agent-state to done
			cmd = exec.Command("kubectl", "annotate", "node", testNode,
				"mco.in-cloud.io/agent-state=done", "--overwrite")
			_, _ = testutil.Run(cmd)

			By("ensuring no old pods exist from previous test")
			// Wait for any old pods to be fully deleted to avoid race conditions.
			Eventually(func() (int, error) {
				cmd := exec.Command("kubectl", "get", "pods", "-l", "app="+deployName,
					"-o", "jsonpath={.items}", "-n", testNs)
				output, err := testutil.Run(cmd)
				if err != nil {
					return 0, err
				}
				if output == "" || output == "[]" {
					return 0, nil
				}
				// Count non-empty items
				return len(output), nil
			}, 30*time.Second, 1*time.Second).Should(Equal(0), "old pods should be deleted")

			By("creating blocking deployment on test node")
			yaml := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: %s
  template:
    metadata:
      labels:
        app: %s
    spec:
      nodeSelector:
        kubernetes.io/hostname: %s
      terminationGracePeriodSeconds: 5
      containers:
      - name: app
        image: busybox
        command: ["sleep", "infinity"]
`, deployName, testNs, deployName, deployName, testNode)
			Expect(applyYAML(yaml)).To(Succeed())

			By("waiting for pod to be running")
			Eventually(func() (string, error) {
				cmd := exec.Command("kubectl", "get", "pods", "-l", "app="+deployName,
					"-o", "jsonpath={.items[0].status.phase}", "-n", testNs)
				return testutil.Run(cmd)
			}, 60*time.Second, 2*time.Second).Should(Equal("Running"))

			By("creating PDB that blocks eviction")
			yaml = fmt.Sprintf(`
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: %s
  namespace: %s
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app: %s
`, pdbName, testNs, deployName)
			Expect(applyYAML(yaml)).To(Succeed())

			By("creating test MachineConfigPool")
			yaml = fmt.Sprintf(`
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfigPool
metadata:
  name: %s
spec:
  nodeSelector:
    matchLabels:
      node-role.kubernetes.io/drain-test: ""
  machineConfigSelector:
    matchLabels:
      mco.in-cloud.io/pool: %s
  rollout:
    maxUnavailable: 1
    debounceSeconds: 1
    drainTimeoutSeconds: 60
    drainRetrySeconds: 10
  reboot:
    strategy: Never
`, poolName, poolName)
			Expect(applyYAML(yaml)).To(Succeed())
		})

		AfterEach(func() {
			_ = deleteResource("mc", "drain-test-mc")
			_ = deleteResource("mcp", poolName)
			_ = deleteResource("pdb", pdbName)
			_ = deleteResource("deployment", deployName)

			By("uncordoning test node")
			_ = uncordonNode(testNode)
		})

		It("should set DrainStuck condition after timeout", func() {
			By("creating MachineConfig to trigger update")
			yaml := fmt.Sprintf(`
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: drain-test-mc
  labels:
    mco.in-cloud.io/pool: %s
spec:
  priority: 50
  files:
    - path: /etc/mco-test/drain-stuck.conf
      content: |
        # Drain stuck test
        timestamp=%s
      mode: 0644
      owner: root:root
      state: present
`, poolName, time.Now().Format(time.RFC3339))
			Expect(applyYAML(yaml)).To(Succeed())

			By("waiting for RMC to be created (controller processing)")
			Eventually(func() (bool, error) {
				cmd := exec.Command("kubectl", "get", "rmc", "-l", "mco.in-cloud.io/pool="+poolName,
					"-o", "jsonpath={.items[0].metadata.name}")
				output, err := testutil.Run(cmd)
				if err != nil {
					return false, nil // Not found yet
				}
				return output != "", nil
			}, 30*time.Second, 2*time.Second).Should(BeTrue(), "RMC should be created")

			By("waiting for node to be cordoned (controller started rollout)")
			// The controller should cordon the node because:
			// 1. Node's current-revision differs from target RMC
			// 2. Node has pool annotation matching this pool
			// 3. Node has agent-state=done (can be updated)
			Eventually(func() (bool, error) {
				cmd := exec.Command("kubectl", "get", "node", testNode, "-o", "jsonpath={.spec.unschedulable}")
				output, err := testutil.Run(cmd)
				if err != nil {
					return false, err
				}
				return output == "true", nil
			}, 120*time.Second, 2*time.Second).Should(BeTrue(), "node should be cordoned to start drain")

			By("verifying drain has started (DrainStartedAt annotation)")
			Eventually(func() (bool, error) {
				cmd := exec.Command("kubectl", "get", "node", testNode,
					"-o", "jsonpath={.metadata.annotations['mco\\.in-cloud\\.io/drain-started-at']}")
				output, err := testutil.Run(cmd)
				if err != nil {
					return false, err
				}
				return output != "", nil
			}, 30*time.Second, 2*time.Second).Should(BeTrue(), "drain-started-at annotation should be set")

			By("waiting for DrainStuck condition (drain timeout 60s + buffer)")
			Eventually(func() (string, error) {
				return getPoolCondition(ctx, poolName, "DrainStuck")
			}, 120*time.Second, 2*time.Second).Should(Equal("True"))
		})

		It("should clear DrainStuck when blocker is removed", func() {
			By("creating MachineConfig to trigger update")
			yaml := fmt.Sprintf(`
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: drain-test-mc
  labels:
    mco.in-cloud.io/pool: %s
spec:
  priority: 50
  files:
    - path: /etc/mco-test/drain-clear.conf
      content: |
        # Drain clear test
        timestamp=%s
      mode: 0644
      owner: root:root
      state: present
`, poolName, time.Now().Format(time.RFC3339))
			Expect(applyYAML(yaml)).To(Succeed())

			By("waiting for DrainStuck to be set")
			Eventually(func() (string, error) {
				return getPoolCondition(ctx, poolName, "DrainStuck")
			}, 120*time.Second, 2*time.Second).Should(Equal("True"))

			By("deleting PDB to unblock drain")
			Expect(deleteResource("pdb", pdbName)).To(Succeed())

			By("waiting for DrainStuck to clear (after drain retries)")
			// After PDB is removed, controller needs time to:
			// 1. Retry drain (which should now succeed)
			// 2. Clear the DrainStuck condition
			// The retry interval is 30s by default, so we need to wait longer
			Eventually(func() (string, error) {
				return getPoolCondition(ctx, poolName, "DrainStuck")
			}, 180*time.Second, 2*time.Second).Should(Equal("False"))
		})
	})

	Context("when drain completes without blocking", func() {
		BeforeEach(func() {
			By("creating test MachineConfigPool")
			yaml := fmt.Sprintf(`
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfigPool
metadata:
  name: %s
spec:
  nodeSelector:
    matchLabels:
      node-role.kubernetes.io/drain-test: ""
  machineConfigSelector:
    matchLabels:
      mco.in-cloud.io/pool: %s
  rollout:
    maxUnavailable: 1
    debounceSeconds: 1
    drainTimeoutSeconds: 60
    drainRetrySeconds: 10
  reboot:
    strategy: Never
`, poolName, poolName)
			Expect(applyYAML(yaml)).To(Succeed())
		})

		AfterEach(func() {
			_ = deleteResource("mc", "drain-test-mc-fast")
			_ = deleteResource("mcp", poolName)

			By("uncordoning test node")
			_ = uncordonNode(testNode)
		})

		It("should complete without DrainStuck", func() {
			By("creating MachineConfig to trigger update")
			yaml := fmt.Sprintf(`
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: drain-test-mc-fast
  labels:
    mco.in-cloud.io/pool: %s
spec:
  priority: 50
  files:
    - path: /etc/mco-test/drain-fast.conf
      content: |
        # Fast drain test
        timestamp=%s
      mode: 0644
      owner: root:root
      state: present
`, poolName, time.Now().Format(time.RFC3339))
			Expect(applyYAML(yaml)).To(Succeed())

			By("waiting for pool to be updated")
			Eventually(func() (bool, error) {
				return isPoolUpdated(ctx, poolName)
			}, 3*time.Minute, 2*time.Second).Should(BeTrue())

			By("verifying DrainStuck is not set")
			cond, err := getPoolCondition(ctx, poolName, "DrainStuck")
			if err == nil {
				Expect(cond).To(Or(Equal("False"), Equal("")))
			}
		})
	})
})
