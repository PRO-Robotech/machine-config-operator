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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"in-cloud.io/machine-config/tests/testutil"
)

const (
	drainExclusionsConfigName = "mco-drain-config"
	drainExclusionsMCONs      = "machine-config-system"
)

// createDrainExclusionsConfigMap creates the drain config ConfigMap
func createDrainExclusionsConfigMap(configYAML string) error {
	yaml := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
  labels:
    mco.in-cloud.io/drain-config: "true"
data:
  config.yaml: |
%s
`, drainExclusionsConfigName, drainExclusionsMCONs, indentYAML(configYAML, 4))
	return applyYAML(yaml)
}

// deleteDrainExclusionsConfigMap deletes the drain config ConfigMap
func deleteDrainExclusionsConfigMap() error {
	cmd := exec.Command("kubectl", "delete", "configmap", drainExclusionsConfigName,
		"-n", drainExclusionsMCONs, "--ignore-not-found")
	_, err := testutil.Run(cmd)
	return err
}

// indentYAML adds indentation to each line of YAML
func indentYAML(yaml string, spaces int) string {
	indent := strings.Repeat(" ", spaces)
	lines := strings.Split(yaml, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = indent + line
		}
	}
	return strings.Join(lines, "\n")
}

var _ = Describe("Drain Exclusions", Ordered, func() {
	var (
		ctx      context.Context
		poolName = "drain-exclusions-test"
		testNode string
		testNs   = "default"
	)

	BeforeAll(func() {
		ctx = context.Background()

		// Clean up any leftover resources from previous test runs
		cleanupAllMCOResources()

		By("finding a worker node that is NOT running the controller")
		cmd := exec.Command("kubectl", "get", "pod", "-n", drainExclusionsMCONs,
			"-l", "control-plane=controller-manager", "-o", "jsonpath={.items[0].spec.nodeName}")
		controllerNode, err := testutil.Run(cmd)
		if err != nil {
			By(fmt.Sprintf("Warning: could not get controller node: %v", err))
			controllerNode = ""
		}
		By(fmt.Sprintf("Controller is running on node: %s", controllerNode))

		// Get all worker nodes (nodes with worker label)
		cmd = exec.Command("kubectl", "get", "nodes",
			"-l", "node-role.kubernetes.io/worker=", "-o", "name")
		output, err := testutil.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to get worker nodes")
		workerNodes := testutil.GetNonEmptyLines(output)
		By(fmt.Sprintf("Found %d worker nodes: %v", len(workerNodes), workerNodes))
		Expect(len(workerNodes)).To(BeNumerically(">=", 1), "Need at least 1 worker node")

		// Find a worker node that is NOT the controller node
		for _, nodeName := range workerNodes {
			name := strings.TrimPrefix(nodeName, "node/")
			if name != controllerNode {
				testNode = name
				break
			}
		}
		// If all workers are controller nodes (shouldn't happen), just use the first one
		if testNode == "" && len(workerNodes) > 0 {
			testNode = strings.TrimPrefix(workerNodes[0], "node/")
			By(fmt.Sprintf("Warning: using controller node %s as test node", testNode))
		}
		Expect(testNode).NotTo(BeEmpty(), "Could not find any worker node")
		By(fmt.Sprintf("Using node %s for drain exclusions test", testNode))

		By("labeling test node")
		Expect(labelNode(testNode, "node-role.kubernetes.io/drain-exclusions-test=")).To(Succeed())
	})

	AfterAll(func() {
		By("cleaning up all test resources")
		_ = deleteResource("mcp", poolName)
		_ = deleteDrainExclusionsConfigMap()
		_ = unlabelNode(testNode, "node-role.kubernetes.io/drain-exclusions-test")
		_ = uncordonNode(testNode)
	})

	// Helper: create test pod with specific properties
	createTestPod := func(name, namespace, nodeName string, labels map[string]string, tolerations string) error {
		labelsYAML := ""
		for k, v := range labels {
			labelsYAML += fmt.Sprintf("      %s: \"%s\"\n", k, v)
		}

		tolerationsYAML := ""
		if tolerations != "" {
			tolerationsYAML = fmt.Sprintf("  tolerations:\n%s\n", tolerations)
		}

		yaml := fmt.Sprintf(`
apiVersion: v1
kind: Pod
metadata:
  name: %s
  namespace: %s
  labels:
%s
spec:
  nodeSelector:
    kubernetes.io/hostname: %s
  terminationGracePeriodSeconds: 1
%s  containers:
  - name: app
    image: busybox
    command: ["sleep", "infinity"]
`, name, namespace, labelsYAML, nodeName, tolerationsYAML)
		return applyYAML(yaml)
	}

	// Helper: check if pod exists
	isPodRunning := func(name, namespace string) (bool, error) {
		cmd := exec.Command("kubectl", "get", "pod", name, "-n", namespace,
			"-o", "jsonpath={.status.phase}")
		output, err := testutil.Run(cmd)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return false, nil
			}
			return false, err
		}
		return output == "Running", nil
	}

	// Helper: wait for pod to be running
	waitForPodRunning := func(name, namespace string, timeout time.Duration) {
		Eventually(func() (bool, error) {
			return isPodRunning(name, namespace)
		}, timeout, 2*time.Second).Should(BeTrue(), fmt.Sprintf("pod %s/%s should be running", namespace, name))
	}

	// Helper: delete pod
	deletePod := func(name, namespace string) error {
		cmd := exec.Command("kubectl", "delete", "pod", name, "-n", namespace, "--ignore-not-found", "--grace-period=0", "--force")
		_, err := testutil.Run(cmd)
		return err
	}

	// Helper: setup test pool
	setupTestPool := func() {
		By("creating test MachineConfigPool")
		yaml := fmt.Sprintf(`
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfigPool
metadata:
  name: %s
spec:
  nodeSelector:
    matchLabels:
      node-role.kubernetes.io/drain-exclusions-test: ""
  machineConfigSelector:
    matchLabels:
      mco.in-cloud.io/pool: %s
  rollout:
    maxUnavailable: 1
    debounceSeconds: 1
    drainTimeoutSeconds: 120
    drainRetrySeconds: 10
  reboot:
    strategy: Never
`, poolName, poolName)
		Expect(applyYAML(yaml)).To(Succeed())
	}

	// Helper: setup node as previously managed
	setupNodeAsPreviouslyManaged := func() {
		_ = clearNodeMCOAnnotations(testNode)
		cmd := exec.Command("kubectl", "annotate", "node", testNode,
			"mco.in-cloud.io/current-revision=old-revision-before-test", "--overwrite")
		_, _ = testutil.Run(cmd)
		cmd = exec.Command("kubectl", "annotate", "node", testNode,
			"mco.in-cloud.io/pool="+poolName, "--overwrite")
		_, _ = testutil.Run(cmd)
		cmd = exec.Command("kubectl", "annotate", "node", testNode,
			"mco.in-cloud.io/agent-state=done", "--overwrite")
		_, _ = testutil.Run(cmd)
	}

	// Helper: trigger config update
	triggerConfigUpdate := func(mcName string) {
		By("creating MachineConfig to trigger update")
		yaml := fmt.Sprintf(`
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: %s
  labels:
    mco.in-cloud.io/pool: %s
spec:
  priority: 50
  files:
    - path: /etc/mco-test/drain-exclusions-%s.conf
      content: |
        # Drain exclusions test
        timestamp=%s
      mode: 0644
      owner: root:root
      state: present
`, mcName, poolName, mcName, time.Now().Format(time.RFC3339))
		Expect(applyYAML(yaml)).To(Succeed())
	}

	// Test: pods in excluded namespace should NOT be evicted
	Context("namespace exclusion rules", func() {
		const testPodName = "exclusion-ns-test-pod"

		BeforeEach(func() {
			By("cleaning up previous resources")
			_ = deleteResource("mc", "drain-exclusions-mc-ns")
			_ = deleteResource("mcp", poolName)
			_ = deleteDrainExclusionsConfigMap()
			_ = deletePod(testPodName, testNs)
			_ = uncordonNode(testNode)

			By("creating drain config with namespace exclusion")
			config := `
defaults:
  skipToleratAllPods: false
  maxEvictionAttempts: 3
rules:
  - namespaces:
      - default
`
			Expect(createDrainExclusionsConfigMap(config)).To(Succeed())

			setupNodeAsPreviouslyManaged()
			setupTestPool()
		})

		AfterEach(func() {
			_ = deleteResource("mc", "drain-exclusions-mc-ns")
			_ = deleteResource("mcp", poolName)
			_ = deleteDrainExclusionsConfigMap()
			_ = deletePod(testPodName, testNs)
			_ = uncordonNode(testNode)
		})

		It("should skip pods in excluded namespace", func() {
			By("creating test pod in default namespace")
			Expect(createTestPod(testPodName, testNs, testNode, map[string]string{"app": testPodName}, "")).To(Succeed())
			waitForPodRunning(testPodName, testNs, 60*time.Second)

			triggerConfigUpdate("drain-exclusions-mc-ns")

			By("waiting for pool update to complete (pod should remain during drain)")
			Eventually(func() (bool, error) {
				return isPoolUpdated(ctx, poolName)
			}, 3*time.Minute, 2*time.Second).Should(BeTrue(), "pool should complete update")

			By("verifying pod was NOT evicted (excluded by namespace rule)")
			running, err := isPodRunning(testPodName, testNs)
			Expect(err).NotTo(HaveOccurred())
			Expect(running).To(BeTrue(), "pod in excluded namespace should NOT be evicted")
		})
	})

	// Test: pods matching namespace prefix should NOT be evicted
	Context("namespace prefix exclusion rules", func() {
		const (
			testPodName = "exclusion-prefix-test-pod"
			testNsName  = "kube-test-exclusion"
		)

		BeforeEach(func() {
			By("cleaning up previous resources")
			_ = deleteResource("mc", "drain-exclusions-mc-prefix")
			_ = deleteResource("mcp", poolName)
			_ = deleteDrainExclusionsConfigMap()
			_ = deletePod(testPodName, testNsName)
			_ = exec.Command("kubectl", "delete", "ns", testNsName, "--ignore-not-found").Run()
			_ = uncordonNode(testNode)

			By("creating test namespace with kube- prefix")
			yaml := fmt.Sprintf(`
apiVersion: v1
kind: Namespace
metadata:
  name: %s
`, testNsName)
			Expect(applyYAML(yaml)).To(Succeed())

			By("creating drain config with namespace prefix exclusion")
			config := `
defaults:
  skipToleratAllPods: false
  maxEvictionAttempts: 3
rules:
  - namespacePrefixes:
      - "kube-"
`
			Expect(createDrainExclusionsConfigMap(config)).To(Succeed())

			setupNodeAsPreviouslyManaged()
			setupTestPool()
		})

		AfterEach(func() {
			_ = deleteResource("mc", "drain-exclusions-mc-prefix")
			_ = deleteResource("mcp", poolName)
			_ = deleteDrainExclusionsConfigMap()
			_ = deletePod(testPodName, testNsName)
			_ = exec.Command("kubectl", "delete", "ns", testNsName, "--ignore-not-found").Run()
			_ = uncordonNode(testNode)
		})

		It("should skip pods in namespace matching prefix", func() {
			By("creating test pod in kube-* namespace")
			Expect(createTestPod(testPodName, testNsName, testNode, map[string]string{"app": testPodName}, "")).To(Succeed())
			waitForPodRunning(testPodName, testNsName, 60*time.Second)

			triggerConfigUpdate("drain-exclusions-mc-prefix")

			By("waiting for pool update to complete (pod should remain during drain)")
			Eventually(func() (bool, error) {
				return isPoolUpdated(ctx, poolName)
			}, 3*time.Minute, 2*time.Second).Should(BeTrue())

			By("verifying pod was NOT evicted (excluded by namespace prefix)")
			running, err := isPodRunning(testPodName, testNsName)
			Expect(err).NotTo(HaveOccurred())
			Expect(running).To(BeTrue(), "pod in kube-* namespace should NOT be evicted")
		})
	})

	// Test: pods matching name pattern should NOT be evicted
	Context("pod name pattern exclusion rules", func() {
		const testPodName = "netshoot-test-pod"

		BeforeEach(func() {
			By("cleaning up previous resources")
			_ = deleteResource("mc", "drain-exclusions-mc-pattern")
			_ = deleteResource("mcp", poolName)
			_ = deleteDrainExclusionsConfigMap()
			_ = deletePod(testPodName, testNs)
			_ = uncordonNode(testNode)

			By("creating drain config with pod name pattern")
			config := `
defaults:
  skipToleratAllPods: false
  maxEvictionAttempts: 3
rules:
  - podNamePatterns:
      - "netshoot-*"
      - "debug-*"
`
			Expect(createDrainExclusionsConfigMap(config)).To(Succeed())

			setupNodeAsPreviouslyManaged()
			setupTestPool()
		})

		AfterEach(func() {
			_ = deleteResource("mc", "drain-exclusions-mc-pattern")
			_ = deleteResource("mcp", poolName)
			_ = deleteDrainExclusionsConfigMap()
			_ = deletePod(testPodName, testNs)
			_ = uncordonNode(testNode)
		})

		It("should skip pods matching name pattern", func() {
			By("creating test pod with matching name pattern")
			Expect(createTestPod(testPodName, testNs, testNode, map[string]string{"app": testPodName}, "")).To(Succeed())
			waitForPodRunning(testPodName, testNs, 60*time.Second)

			triggerConfigUpdate("drain-exclusions-mc-pattern")

			By("waiting for pool update to complete (pod should remain during drain)")
			Eventually(func() (bool, error) {
				return isPoolUpdated(ctx, poolName)
			}, 3*time.Minute, 2*time.Second).Should(BeTrue())

			By("verifying pod was NOT evicted (excluded by name pattern)")
			running, err := isPodRunning(testPodName, testNs)
			Expect(err).NotTo(HaveOccurred())
			Expect(running).To(BeTrue(), "pod matching netshoot-* pattern should NOT be evicted")
		})
	})

	// Test: pods matching label selector should NOT be evicted
	Context("pod label selector exclusion rules", func() {
		const testPodName = "labeled-skip-drain-pod"

		BeforeEach(func() {
			By("cleaning up previous resources")
			_ = deleteResource("mc", "drain-exclusions-mc-label")
			_ = deleteResource("mcp", poolName)
			_ = deleteDrainExclusionsConfigMap()
			_ = deletePod(testPodName, testNs)
			_ = uncordonNode(testNode)

			By("creating drain config with label selector")
			config := `
defaults:
  skipToleratAllPods: false
  maxEvictionAttempts: 3
rules:
  - podSelector:
      matchLabels:
        mco.in-cloud.io/skip-drain: "true"
`
			Expect(createDrainExclusionsConfigMap(config)).To(Succeed())

			setupNodeAsPreviouslyManaged()
			setupTestPool()
		})

		AfterEach(func() {
			_ = deleteResource("mc", "drain-exclusions-mc-label")
			_ = deleteResource("mcp", poolName)
			_ = deleteDrainExclusionsConfigMap()
			_ = deletePod(testPodName, testNs)
			_ = uncordonNode(testNode)
		})

		It("should skip pods with skip-drain label", func() {
			By("creating test pod with skip-drain label")
			labels := map[string]string{
				"app":                        testPodName,
				"mco.in-cloud.io/skip-drain": "true",
			}
			Expect(createTestPod(testPodName, testNs, testNode, labels, "")).To(Succeed())
			waitForPodRunning(testPodName, testNs, 60*time.Second)

			triggerConfigUpdate("drain-exclusions-mc-label")

			By("waiting for pool update to complete (pod should remain during drain)")
			Eventually(func() (bool, error) {
				return isPoolUpdated(ctx, poolName)
			}, 3*time.Minute, 2*time.Second).Should(BeTrue())

			By("verifying pod was NOT evicted (excluded by label)")
			running, err := isPodRunning(testPodName, testNs)
			Expect(err).NotTo(HaveOccurred())
			Expect(running).To(BeTrue(), "pod with skip-drain label should NOT be evicted")
		})
	})

	// Test: tolerate-all pods should be skipped when skipToleratAllPods=true
	Context("tolerate-all pods exclusion", func() {
		const testPodName = "tolerate-all-test-pod"

		BeforeEach(func() {
			By("cleaning up previous resources")
			_ = deleteResource("mc", "drain-exclusions-mc-tolerate")
			_ = deleteResource("mcp", poolName)
			_ = deleteDrainExclusionsConfigMap()
			_ = deletePod(testPodName, testNs)
			_ = uncordonNode(testNode)
		})

		AfterEach(func() {
			_ = deleteResource("mc", "drain-exclusions-mc-tolerate")
			_ = deleteResource("mcp", poolName)
			_ = deleteDrainExclusionsConfigMap()
			_ = deletePod(testPodName, testNs)
			_ = uncordonNode(testNode)
		})

		It("should skip tolerate-all pods when skipToleratAllPods=true", func() {
			By("creating drain config with skipToleratAllPods=true")
			config := `
defaults:
  skipToleratAllPods: true
  maxEvictionAttempts: 3
rules: []
`
			Expect(createDrainExclusionsConfigMap(config)).To(Succeed())

			setupNodeAsPreviouslyManaged()
			setupTestPool()

			By("creating pod with tolerate-all (operator: Exists)")
			tolerations := `  - operator: Exists`
			Expect(createTestPod(testPodName, testNs, testNode, map[string]string{"app": testPodName}, tolerations)).To(Succeed())
			waitForPodRunning(testPodName, testNs, 60*time.Second)

			triggerConfigUpdate("drain-exclusions-mc-tolerate")

			By("waiting for pool update to complete (pod should remain during drain)")
			Eventually(func() (bool, error) {
				return isPoolUpdated(ctx, poolName)
			}, 3*time.Minute, 2*time.Second).Should(BeTrue())

			By("verifying tolerate-all pod was NOT evicted")
			running, err := isPodRunning(testPodName, testNs)
			Expect(err).NotTo(HaveOccurred())
			Expect(running).To(BeTrue(), "tolerate-all pod should NOT be evicted when skipToleratAllPods=true")
		})

		It("should evict tolerate-all pods when skipToleratAllPods=false", func() {
			By("creating drain config with skipToleratAllPods=false")
			config := `
defaults:
  skipToleratAllPods: false
  maxEvictionAttempts: 3
rules: []
`
			Expect(createDrainExclusionsConfigMap(config)).To(Succeed())

			setupNodeAsPreviouslyManaged()
			setupTestPool()

			By("creating pod with tolerate-all (operator: Exists)")
			tolerations := `  - operator: Exists`
			Expect(createTestPod(testPodName, testNs, testNode, map[string]string{"app": testPodName}, tolerations)).To(Succeed())
			waitForPodRunning(testPodName, testNs, 60*time.Second)

			triggerConfigUpdate("drain-exclusions-mc-tolerate")

			By("waiting for pod to be evicted (normal drain behavior)")
			Eventually(func() (bool, error) {
				return isPodRunning(testPodName, testNs)
			}, 2*time.Minute, 2*time.Second).Should(BeFalse(), "tolerate-all pod SHOULD be evicted when skipToleratAllPods=false")

			By("verifying pool update completes")
			Eventually(func() (bool, error) {
				return isPoolUpdated(ctx, poolName)
			}, 3*time.Minute, 2*time.Second).Should(BeTrue())
		})
	})

	// Test: default behavior when ConfigMap not found
	Context("default behavior without ConfigMap", func() {
		const testPodName = "default-behavior-test-pod"

		BeforeEach(func() {
			By("cleaning up previous resources")
			_ = deleteResource("mc", "drain-exclusions-mc-default")
			_ = deleteResource("mcp", poolName)
			_ = deleteDrainExclusionsConfigMap()
			_ = deletePod(testPodName, testNs)
			_ = uncordonNode(testNode)

			By("ensuring NO drain config ConfigMap exists")
			Expect(deleteDrainExclusionsConfigMap()).To(Succeed())

			setupNodeAsPreviouslyManaged()
			setupTestPool()
		})

		AfterEach(func() {
			_ = deleteResource("mc", "drain-exclusions-mc-default")
			_ = deleteResource("mcp", poolName)
			_ = deletePod(testPodName, testNs)
			_ = uncordonNode(testNode)
		})

		It("should use default behavior when ConfigMap not found", func() {
			By("creating a regular test pod")
			Expect(createTestPod(testPodName, testNs, testNode, map[string]string{"app": testPodName}, "")).To(Succeed())
			waitForPodRunning(testPodName, testNs, 60*time.Second)

			triggerConfigUpdate("drain-exclusions-mc-default")

			By("waiting for pod to be evicted (normal drain behavior)")
			Eventually(func() (bool, error) {
				return isPodRunning(testPodName, testNs)
			}, 2*time.Minute, 2*time.Second).Should(BeFalse(), "pod SHOULD be evicted with default drain behavior")

			By("verifying pool update completes")
			Eventually(func() (bool, error) {
				return isPoolUpdated(ctx, poolName)
			}, 3*time.Minute, 2*time.Second).Should(BeTrue())
		})
	})
})
