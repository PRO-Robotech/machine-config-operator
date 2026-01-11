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

var _ = Describe("Controller Resilience", Ordered, func() {
	var (
		ctx context.Context
	)

	BeforeAll(func() {
		ctx = context.Background()
		_ = ctx // suppress unused warning
		cleanupAllMCOResources()
	})

	AfterAll(func() {
		cleanupAllMCOResources()
	})

	// Controller должен оставаться стабильным во время drain операций
	Context("during drain operations", func() {
		const poolName = "e2e-controller-stability"
		const mcName = "e2e-controller-stability-mc"

		AfterEach(func() {
			_ = deleteResource("mc", mcName)
			_ = deleteResource("mcp", poolName)
			uncordonAllWorkerNodes()
		})

		It("controller pod should continue running without restarts", func() {
			By("recording initial controller pod state")
			initialPodName, initialRestarts := getControllerPodInfo()
			Expect(initialPodName).NotTo(BeEmpty(), "controller pod should be running")

			By("creating MachineConfigPool with MachineConfig to trigger rollout")
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
    - path: /etc/mco-test/controller-stability.txt
      content: "test file for controller stability check"
      mode: 0644
`, mcName, poolName)
			Expect(applyYAML(mcYAML)).To(Succeed())

			By("waiting for rollout to complete")
			Eventually(func() (bool, error) {
				return isPoolUpdated(context.Background(), poolName)
			}, 120*time.Second, 5*time.Second).Should(BeTrue(), "pool should complete rollout")

			By("verifying controller pod remains stable")
			currentPodName, currentRestarts := getControllerPodInfo()
			Expect(currentPodName).To(Equal(initialPodName),
				"controller pod name should remain unchanged")
			Expect(currentRestarts).To(Equal(initialRestarts),
				"controller pod should not have restarted")
		})
	})

	// Controller deployment должен иметь правильную конфигурацию
	Context("deployment configuration", func() {
		It("should have tolerations for unschedulable nodes", func() {
			By("retrieving controller pod tolerations")
			cmd := exec.Command("kubectl", "get", "pod", "-n", namespace,
				"-l", "control-plane=controller-manager",
				"-o", "jsonpath={.items[0].spec.tolerations}")
			output, err := testutil.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			tolerations := string(output)

			By("verifying toleration for node.kubernetes.io/unschedulable exists")
			Expect(tolerations).To(ContainSubstring("unschedulable"),
				"controller should tolerate unschedulable taint to continue running on cordoned nodes")
		})

		It("should have NODE_NAME environment variable for self-awareness", func() {
			By("retrieving controller container environment variables")
			cmd := exec.Command("kubectl", "get", "pod", "-n", namespace,
				"-l", "control-plane=controller-manager",
				"-o", "jsonpath={.items[0].spec.containers[0].env[*].name}")
			output, err := testutil.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			envVars := string(output)
			Expect(envVars).To(ContainSubstring("NODE_NAME"),
				"controller should have NODE_NAME environment variable to identify its host node")
		})

		It("should have POD_NAME environment variable", func() {
			By("retrieving controller container environment variables")
			cmd := exec.Command("kubectl", "get", "pod", "-n", namespace,
				"-l", "control-plane=controller-manager",
				"-o", "jsonpath={.items[0].spec.containers[0].env[*].name}")
			output, err := testutil.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			envVars := string(output)
			Expect(envVars).To(ContainSubstring("POD_NAME"),
				"controller should have POD_NAME environment variable")
		})

		It("should have POD_NAMESPACE environment variable", func() {
			By("retrieving controller container environment variables")
			cmd := exec.Command("kubectl", "get", "pod", "-n", namespace,
				"-l", "control-plane=controller-manager",
				"-o", "jsonpath={.items[0].spec.containers[0].env[*].name}")
			output, err := testutil.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			envVars := string(output)
			Expect(envVars).To(ContainSubstring("POD_NAMESPACE"),
				"controller should have POD_NAMESPACE environment variable")
		})
	})
})
