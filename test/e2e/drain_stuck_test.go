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

	"in-cloud.io/machine-config/test/utils"
)

var _ = Describe("Drain Stuck", Ordered, func() {
	var (
		ctx          context.Context
		poolName     = "drain-test-workers"
		pdbName      = "blocking-pdb"
		deployName   = "blocking-app"
		testNode     string
		testNs       = "default"
	)

	BeforeAll(func() {
		ctx = context.Background()

		By("getting a worker node for drain test")
		cmd := exec.Command("kubectl", "get", "nodes", "-o", "name")
		output, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		nodeNames := utils.GetNonEmptyLines(output)
		Expect(len(nodeNames)).To(BeNumerically(">=", 2), "Need at least 2 nodes")

		testNode = nodeNames[1][5:] // strip "node/", use second node
		By(fmt.Sprintf("Using node %s for drain test", testNode))

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
		cmd := exec.Command("kubectl", "uncordon", testNode)
		_, _ = utils.Run(cmd)
	})

	Context("when PDB blocks drain", func() {
		BeforeEach(func() {
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
				return utils.Run(cmd)
			}, 60*time.Second, 5*time.Second).Should(Equal("Running"))

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
    debounceSeconds: 5
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
			cmd := exec.Command("kubectl", "uncordon", testNode)
			_, _ = utils.Run(cmd)
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

			By("waiting for DrainStuck condition")
			Eventually(func() (string, error) {
				return getPoolCondition(ctx, poolName, "DrainStuck")
			}, 120*time.Second, 10*time.Second).Should(Equal("True"))
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
			}, 120*time.Second, 10*time.Second).Should(Equal("True"))

			By("deleting PDB to unblock drain")
			Expect(deleteResource("pdb", pdbName)).To(Succeed())

			By("waiting for DrainStuck to clear")
			Eventually(func() (string, error) {
				return getPoolCondition(ctx, poolName, "DrainStuck")
			}, 120*time.Second, 10*time.Second).Should(Equal("False"))
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
    debounceSeconds: 5
  reboot:
    strategy: Never
`, poolName, poolName)
			Expect(applyYAML(yaml)).To(Succeed())
		})

		AfterEach(func() {
			_ = deleteResource("mc", "drain-test-mc-fast")
			_ = deleteResource("mcp", poolName)

			By("uncordoning test node")
			cmd := exec.Command("kubectl", "uncordon", testNode)
			_, _ = utils.Run(cmd)
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
			}, 3*time.Minute, 10*time.Second).Should(BeTrue())

			By("verifying DrainStuck is not set")
			cond, err := getPoolCondition(ctx, poolName, "DrainStuck")
			if err == nil {
				Expect(cond).To(Or(Equal("False"), Equal("")))
			}
		})
	})
})
