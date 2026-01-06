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

var _ = Describe("Rolling Update", Ordered, func() {
	var (
		ctx      context.Context
		poolName = "e2e-workers"
	)

	BeforeAll(func() {
		ctx = context.Background()

		By("ensuring worker nodes are labeled")
		nodes, err := getWorkerNodes()
		if err != nil || len(nodes) == 0 {
			cmd := exec.Command("kubectl", "get", "nodes", "-o", "name")
			output, _ := utils.Run(cmd)
			nodeNames := utils.GetNonEmptyLines(output)
			for i, name := range nodeNames {
				if i > 0 {
					nodeName := name[5:] // strip "node/"
					_ = labelNode(nodeName, "node-role.kubernetes.io/worker=")
				}
			}
		}
	})

	AfterAll(func() {
		By("cleaning up test resources")
		_ = deleteResource("mcp", poolName)
		_ = deleteResource("mc", "e2e-test-mc")
	})

	Context("with maxUnavailable=1", func() {
		BeforeEach(func() {
			By("creating test MachineConfigPool")
			pool := createTestPool(poolName, 1)
			yaml := fmt.Sprintf(`
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
    debounceSeconds: 5
  reboot:
    strategy: Never
  paused: false
`, poolName, poolName)
			_ = pool
			Expect(applyYAML(yaml)).To(Succeed())

			By("waiting for pool to be ready")
			Eventually(func() (bool, error) {
				return isPoolUpdated(ctx, poolName)
			}, 60*time.Second, 5*time.Second).Should(BeTrue())
		})

		AfterEach(func() {
			_ = deleteResource("mc", "e2e-test-mc")
		})

		It("should update nodes sequentially", func() {
			By("creating test MachineConfig")
			yaml := fmt.Sprintf(`
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: e2e-test-mc
  labels:
    mco.in-cloud.io/pool: %s
spec:
  priority: 50
  files:
    - path: /etc/mco-test/e2e-rolling.conf
      content: |
        # E2E test config
        timestamp=%s
      mode: 0644
      owner: root:root
      state: present
`, poolName, time.Now().Format(time.RFC3339))
			Expect(applyYAML(yaml)).To(Succeed())

			By("verifying at most 1 node is cordoned at a time")
			maxCordoned := 0
			for i := 0; i < 30; i++ {
				count, err := getCordonedNodeCount(ctx, poolName)
				if err == nil && count > maxCordoned {
					maxCordoned = count
				}
				if count > 1 {
					Fail(fmt.Sprintf("Expected at most 1 cordoned node, got %d", count))
				}
				time.Sleep(2 * time.Second)
			}

			By("waiting for rollout to complete")
			Eventually(func() (bool, error) {
				return isPoolUpdated(ctx, poolName)
			}, 5*time.Minute, 10*time.Second).Should(BeTrue())
		})
	})

	Context("with maxUnavailable=2", func() {
		BeforeEach(func() {
			By("creating test MachineConfigPool with maxUnavailable=2")
			yaml := fmt.Sprintf(`
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
    maxUnavailable: 2
    debounceSeconds: 5
  reboot:
    strategy: Never
  paused: false
`, poolName, poolName)
			Expect(applyYAML(yaml)).To(Succeed())

			Eventually(func() (bool, error) {
				return isPoolUpdated(ctx, poolName)
			}, 60*time.Second, 5*time.Second).Should(BeTrue())
		})

		AfterEach(func() {
			_ = deleteResource("mc", "e2e-test-mc-2")
		})

		It("should update up to 2 nodes in parallel", func() {
			By("creating test MachineConfig")
			yaml := fmt.Sprintf(`
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: e2e-test-mc-2
  labels:
    mco.in-cloud.io/pool: %s
spec:
  priority: 50
  files:
    - path: /etc/mco-test/e2e-parallel.conf
      content: |
        # E2E parallel test
        timestamp=%s
      mode: 0644
      owner: root:root
      state: present
`, poolName, time.Now().Format(time.RFC3339))
			Expect(applyYAML(yaml)).To(Succeed())

			By("verifying at most 2 nodes are cordoned at a time")
			for i := 0; i < 30; i++ {
				count, err := getCordonedNodeCount(ctx, poolName)
				if err == nil && count > 2 {
					Fail(fmt.Sprintf("Expected at most 2 cordoned nodes, got %d", count))
				}
				time.Sleep(2 * time.Second)
			}

			By("waiting for rollout to complete")
			Eventually(func() (bool, error) {
				return isPoolUpdated(ctx, poolName)
			}, 5*time.Minute, 10*time.Second).Should(BeTrue())
		})
	})

	Context("with maxUnavailable=50%", func() {
		BeforeEach(func() {
			By("creating test MachineConfigPool with maxUnavailable=50%")
			yaml := fmt.Sprintf(`
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
    maxUnavailable: "50%%"
    debounceSeconds: 5
  reboot:
    strategy: Never
  paused: false
`, poolName, poolName)
			Expect(applyYAML(yaml)).To(Succeed())

			Eventually(func() (bool, error) {
				return isPoolUpdated(ctx, poolName)
			}, 60*time.Second, 5*time.Second).Should(BeTrue())
		})

		AfterEach(func() {
			_ = deleteResource("mc", "e2e-test-mc-pct")
		})

		It("should update ceil(50%*N) nodes", func() {
			By("creating test MachineConfig")
			yaml := fmt.Sprintf(`
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: e2e-test-mc-pct
  labels:
    mco.in-cloud.io/pool: %s
spec:
  priority: 50
  files:
    - path: /etc/mco-test/e2e-percent.conf
      content: |
        # E2E percentage test
        timestamp=%s
      mode: 0644
      owner: root:root
      state: present
`, poolName, time.Now().Format(time.RFC3339))
			Expect(applyYAML(yaml)).To(Succeed())

			By("waiting for rollout to complete")
			Eventually(func() (bool, error) {
				return isPoolUpdated(ctx, poolName)
			}, 5*time.Minute, 10*time.Second).Should(BeTrue())
		})
	})

	Context("with paused=true", func() {
		BeforeEach(func() {
			By("creating paused MachineConfigPool")
			yaml := fmt.Sprintf(`
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
    debounceSeconds: 5
  reboot:
    strategy: Never
  paused: true
`, poolName, poolName)
			Expect(applyYAML(yaml)).To(Succeed())
		})

		AfterEach(func() {
			_ = deleteResource("mc", "e2e-test-mc-paused")
		})

		It("should not update any nodes", func() {
			By("creating test MachineConfig")
			yaml := fmt.Sprintf(`
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: e2e-test-mc-paused
  labels:
    mco.in-cloud.io/pool: %s
spec:
  priority: 50
  files:
    - path: /etc/mco-test/e2e-paused.conf
      content: "# should not be applied"
      mode: 0644
      owner: root:root
      state: present
`, poolName)
			Expect(applyYAML(yaml)).To(Succeed())

			By("verifying no nodes are cordoned")
			Consistently(func() (int, error) {
				return getCordonedNodeCount(ctx, poolName)
			}, 30*time.Second, 5*time.Second).Should(Equal(0))
		})
	})
})
