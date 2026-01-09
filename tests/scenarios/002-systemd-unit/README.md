# Scenario 002: Systemd Unit

## Description

Verify systemd unit creation and management through MachineConfig.

## What We Test

1. Unit file creation in `/etc/systemd/system/`
2. Correct file permissions (0644)
3. Unit is enabled (starts on boot)
4. Unit is active (currently running)
5. Agent reaches idle state after apply

## Prerequisites

- Minikube running
- MCO controller and agent deployed
- MachineConfigPool "workers" exists

## MachineConfig

Creates a simple oneshot timer that runs every minute:
- `/etc/systemd/system/mco-test.service` - Simple oneshot that logs a message
- `/etc/systemd/system/mco-test.timer` - Timer that triggers the service

This is a safe, non-destructive test unit.

## Expected Results

1. Service and timer unit files exist
2. Timer is enabled and active
3. Service runs successfully when triggered
4. Agent annotation `mco.in-cloud.io/agent-state` = idle

## Manual Verification

```bash
# Check unit files
minikube ssh "cat /etc/systemd/system/mco-test.service"
minikube ssh "cat /etc/systemd/system/mco-test.timer"

# Check timer status
minikube ssh "systemctl status mco-test.timer"
minikube ssh "systemctl is-enabled mco-test.timer"

# Check service status
minikube ssh "systemctl status mco-test.service"

# View timer logs
minikube ssh "journalctl -u mco-test.service --no-pager -n 10"
```

## Cleanup

Removes:
- MachineConfig `50-systemd-test`
- Unit files from `/etc/systemd/system/`
- Stops and disables the timer
