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

var _ = Describe("Pool Overlap", Ordered, func() {
	var (
		ctx         context.Context
		workersPool = "overlap-workers"
		infraPool   = "overlap-infra"
		testNode    string
	)

	BeforeAll(func() {
		ctx = context.Background()

		// Clean up any leftover resources from previous test runs
		cleanupAllMCOResources()

		By("getting a worker node for overlap test")
		cmd := exec.Command("kubectl", "get", "nodes", "-o", "name")
		output, err := testutil.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		nodeNames := testutil.GetNonEmptyLines(output)
		Expect(len(nodeNames)).To(BeNumerically(">=", 2), "Need at least 2 nodes")

		testNode = nodeNames[1][5:] // strip "node/", use second node
		By(fmt.Sprintf("Using node %s for overlap test", testNode))
	})

	AfterAll(func() {
		By("cleaning up test resources")
		_ = deleteResource("mcp", workersPool)
		_ = deleteResource("mcp", infraPool)
		_ = unlabelNode(testNode, "role")
		_ = unlabelNode(testNode, "role2")
	})

	Context("when pools have overlapping selectors", func() {
		BeforeEach(func() {
			By("labeling test node with both roles")
			Expect(labelNode(testNode, "role=worker")).To(Succeed())
			Expect(labelNode(testNode, "role2=infra")).To(Succeed())

			By("creating workers pool")
			yaml := fmt.Sprintf(`
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfigPool
metadata:
  name: %s
spec:
  nodeSelector:
    matchLabels:
      role: worker
  machineConfigSelector:
    matchLabels:
      mco.in-cloud.io/pool: %s
  rollout:
    maxUnavailable: 1
    debounceSeconds: 1
  reboot:
    strategy: Never
`, workersPool, workersPool)
			Expect(applyYAML(yaml)).To(Succeed())

			By("creating infra pool (will overlap)")
			yaml = fmt.Sprintf(`
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfigPool
metadata:
  name: %s
spec:
  nodeSelector:
    matchLabels:
      role2: infra
  machineConfigSelector:
    matchLabels:
      mco.in-cloud.io/pool: %s
  rollout:
    maxUnavailable: 1
    debounceSeconds: 1
  reboot:
    strategy: Never
`, infraPool, infraPool)
			Expect(applyYAML(yaml)).To(Succeed())
		})

		AfterEach(func() {
			_ = deleteResource("mcp", workersPool)
			_ = deleteResource("mcp", infraPool)
		})

		It("should detect overlap and set PoolOverlap condition", func() {
			By("waiting for PoolOverlap condition on workers pool")
			Eventually(func() (string, error) {
				return getPoolCondition(ctx, workersPool, "PoolOverlap")
			}, 60*time.Second, 2*time.Second).Should(Equal("True"))

			By("verifying PoolOverlap on infra pool")
			Eventually(func() (string, error) {
				return getPoolCondition(ctx, infraPool, "PoolOverlap")
			}, 60*time.Second, 2*time.Second).Should(Equal("True"))
		})

		It("should clear overlap when conflict is resolved", func() {
			By("waiting for overlap to be detected first")
			Eventually(func() (string, error) {
				return getPoolCondition(ctx, workersPool, "PoolOverlap")
			}, 60*time.Second, 2*time.Second).Should(Equal("True"))

			By("removing the conflicting label from node")
			Expect(unlabelNode(testNode, "role2")).To(Succeed())

			By("waiting for PoolOverlap to clear on workers pool")
			Eventually(func() (string, error) {
				return getPoolCondition(ctx, workersPool, "PoolOverlap")
			}, 60*time.Second, 2*time.Second).Should(Equal("False"))
		})
	})

	Context("when pools do not overlap", func() {
		BeforeEach(func() {
			By("labeling node with only worker role")
			Expect(labelNode(testNode, "role=worker")).To(Succeed())
			_ = unlabelNode(testNode, "role2")

			By("creating non-overlapping pools")
			yaml := fmt.Sprintf(`
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfigPool
metadata:
  name: %s
spec:
  nodeSelector:
    matchLabels:
      role: worker
  machineConfigSelector:
    matchLabels:
      mco.in-cloud.io/pool: %s
  rollout:
    maxUnavailable: 1
    debounceSeconds: 1
  reboot:
    strategy: Never
`, workersPool, workersPool)
			Expect(applyYAML(yaml)).To(Succeed())

			yaml = fmt.Sprintf(`
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfigPool
metadata:
  name: %s
spec:
  nodeSelector:
    matchLabels:
      role: storage
  machineConfigSelector:
    matchLabels:
      mco.in-cloud.io/pool: %s
  rollout:
    maxUnavailable: 1
    debounceSeconds: 1
  reboot:
    strategy: Never
`, infraPool, infraPool)
			Expect(applyYAML(yaml)).To(Succeed())
		})

		AfterEach(func() {
			_ = deleteResource("mcp", workersPool)
			_ = deleteResource("mcp", infraPool)
		})

		It("should not have PoolOverlap condition set", func() {
			By("waiting for PoolOverlap to be False (no overlap)")
			Eventually(func() (string, error) {
				return getPoolCondition(ctx, workersPool, "PoolOverlap")
			}, 30*time.Second, 2*time.Second).Should(Equal("False"))
		})
	})
})
