# Scenario 001: Sysctl Basic

## Description

Verify basic sysctl parameter application via configuration file.

## What We Test

1. File creation in `/etc/sysctl.d/`
2. Correct file permissions (0644) and ownership (root:root)
3. Sysctl parameters are applied without reboot
4. Agent reaches idle state after apply

## Prerequisites

- Minikube running
- MCO controller and agent deployed
- MachineConfigPool "workers" exists with nodeSelector matching minikube node

## MachineConfig

Creates `/etc/sysctl.d/99-mco-test.conf` with safe sysctl parameters:
- `fs.inotify.max_user_watches = 524288`
- `net.core.somaxconn = 4096`
- `vm.swappiness = 10`

## Expected Results

1. File exists at `/etc/sysctl.d/99-mco-test.conf`
2. File has mode 0644 and owner root:root
3. File contains expected sysctl parameters
4. After `sysctl --system`, values are applied
5. Agent annotation `mco.in-cloud.io/agent-state` = idle

## Manual Verification

```bash
# Check file exists and content
minikube ssh "cat /etc/sysctl.d/99-mco-test.conf"

# Check current sysctl values
minikube ssh "sysctl fs.inotify.max_user_watches"
minikube ssh "sysctl net.core.somaxconn"
minikube ssh "sysctl vm.swappiness"

# Apply sysctl (agent should do this automatically)
minikube ssh "sudo sysctl --system"

# Check node annotations
kubectl get nodes -o jsonpath='{.items[0].metadata.annotations}' | jq
```

## Cleanup

Removes:
- MachineConfig `50-sysctl-test`
- File `/etc/sysctl.d/99-mco-test.conf`
