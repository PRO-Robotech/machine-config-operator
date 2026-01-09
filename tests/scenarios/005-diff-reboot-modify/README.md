# Scenario 005: Diff-Based Reboot - Modify Reboot File

## Description

Tests that modifying a file from a MachineConfig with `reboot.required: true`
DOES trigger reboot-pending.

This validates that the diff-based logic correctly identifies when changed
files require reboot.

## What We Test

1. Apply a MachineConfig with `reboot.required: true`
2. Modify the content of that MachineConfig
3. Verify: file is updated AND reboot-pending is set

## Prerequisites

- Minikube running
- MCO controller and agent deployed
- MachineConfigPool "worker" exists

## Expected Result

1. File is updated on the node with newList content
2. `reboot-pending` annotation is set to "true"
3. Current revision does NOT match desired revision (waiting for reboot)

## Manual Verification

```bash
# Check reboot pending
kubectl get nodes -o custom-columns='REBOOT:.metadata.annotations.mco\.in-cloud\.io/reboot-pending'

# Check file content
minikube ssh "cat /etc/mco-reboot-modify-test.conf"
```
