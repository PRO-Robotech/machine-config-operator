# Scenario 003: File Absent

## Description

Verify file deletion through MachineConfig with `state: absent`.

## What We Test

1. Create a file first (via pre-check)
2. Apply MachineConfig with `state: absent`
3. Verify file is deleted
4. Agent reaches idle state after apply

## Prerequisites

- Minikube running
- MCO controller and agent deployed
- MachineConfigPool "workers" exists

## Test Flow

This is a two-phase test:

**Phase 1 (pre-check.sh):**
- Create test file `/etc/mco-test/delete-me.txt`
- Verify file exists before main test

**Phase 2 (apply + verify):**
- Apply MachineConfig with `state: absent`
- Verify file is deleted

## MachineConfig

Deletes `/etc/mco-test/delete-me.txt` if it exists.

## Expected Results

1. File `/etc/mco-test/delete-me.txt` does NOT exist
2. Agent annotation `mco.in-cloud.io/agent-state` = idle
3. No errors in agent state

## Manual Verification

```bash
# Before applying - create test file
minikube ssh "sudo mkdir -p /etc/mco-test"
minikube ssh "sudo echo 'test content' | sudo tee /etc/mco-test/delete-me.txt"
minikube ssh "ls -la /etc/mco-test/"

# Apply MachineConfig
kubectl apply -f manifest.yaml

# Wait and verify
sleep 10
minikube ssh "ls -la /etc/mco-test/"  # File should be gone
```

## Cleanup

- Removes MachineConfig `50-file-absent-test`
- Removes `/etc/mco-test/` directory if it exists
