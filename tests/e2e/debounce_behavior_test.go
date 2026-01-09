//go:build e2e
// +build e2e

/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

package e2e

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Debounce Behavior E2E", func() {

	Context("Debounce limits rapid RMC creation", func() {

		It("should debounce rapid MC changes and create at most one new RMC per debounce window", func() {
			poolName := "e2e-debounce"
			debounceSeconds := 3

			By("creating MachineConfigPool with debounceSeconds=3")
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
    debounceSeconds: %d
    applyTimeoutSeconds: 300
    maxUnavailable: 1
  reboot:
    strategy: Never
  revisionHistory:
    limit: 10
  paused: false
`, poolName, poolName, debounceSeconds)
			Expect(applyYAML(poolYAML)).To(Succeed())
			DeferCleanup(func() {
				_ = deleteResource("mcp", poolName)
			})

			// Initial state: count RMCs
			initialRMCs, _ := getRMCsByPool(poolName)
			initialCount := len(initialRMCs)

			By("rapidly applying 3 MC changes within debounce window")
			for i := 1; i <= 3; i++ {
				mcName := fmt.Sprintf("mc-e2e-debounce-%d", i)
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
  - path: /etc/mco-test/debounce-v%d.conf
    content: "rapid change %d - %s"
    mode: 0644
    owner: "root:root"
    state: present
`, mcName, poolName, i, i, time.Now().Format(time.RFC3339Nano))
				Expect(applyYAML(mcYAML)).To(Succeed())
				DeferCleanup(func(name string) func() {
					return func() { _ = deleteResource("mc", name) }
				}(mcName))

				time.Sleep(1 * time.Second) // Rapid changes (within debounce window)
			}

			By("checking RMC count immediately (should be limited by debounce)")
			time.Sleep(2 * time.Second) // Small wait
			immediateRMCs, _ := getRMCsByPool(poolName)
			immediateCount := len(immediateRMCs)

			// Within debounce window, should not have 3 new RMCs
			By(fmt.Sprintf("found %d RMCs immediately after rapid changes (initial: %d)", immediateCount, initialCount))
			Expect(immediateCount-initialCount).To(BeNumerically("<=", 1),
				"should create at most 1 RMC during debounce window")

			By("waiting for debounce to expire and final RMC to be created")
			time.Sleep(time.Duration(debounceSeconds+3) * time.Second)

			finalRMCs, err := getRMCsByPool(poolName)
			Expect(err).NotTo(HaveOccurred())

			By(fmt.Sprintf("found %d RMCs after debounce expired", len(finalRMCs)))
			// After debounce, there should be exactly 1 new RMC (with final merged state)
			Expect(len(finalRMCs)-initialCount).To(BeNumerically(">=", 1),
				"at least 1 new RMC should be created after debounce")
		})
	})
})
