# Scenario 004: Diff-Based Reboot - No Reboot Required

## Description

Tests that adding a MachineConfig with `reboot.required: false` does NOT trigger
a reboot, even when there are other MachineConfigs with `reboot.required: true`
in the pool.

This is the key use case for diff-based reboot logic.

## What We Test

1. Node has a MachineConfig with `reboot.required: true` applied
2. Add a NEW MachineConfig with `reboot.required: false`
3. Verify: file is applied WITHOUT triggering reboot-pending

## Prerequisites

- Minikube running
- MCO controller and agent deployed
- MachineConfigPool "worker" exists

## Expected Result

1. New file is created on the node
2. Agent state becomes idle/done
3. `reboot-pending` annotation is NOT set
4. Current revision matches desired revision

## Manual Verification

```bash
# Check agent state
kubectl get nodes -o custom-columns='STATE:.metadata.annotations.mco\.in-cloud\.io/agent-state,REBOOT:.metadata.annotations.mco\.in-cloud\.io/reboot-pending'

# Check file on node
minikube ssh "cat /etc/mco-no-reboot-test.conf"
```
