# MCO Lite Test Scenarios

Integration test scenarios for MachineConfig Operator Lite.

## Structure

Each scenario is a self-contained test case with:
- `README.md` - Human-readable description
- `manifest.yaml` - MachineConfig(s) to apply
- `expected.yaml` - Machine-readable expectations
- `pre-check.sh` - Capture state before apply
- `verify.sh` - Verification after apply
- `cleanup.sh` - Rollback changes

## Prerequisites

- Minikube running: `minikube status`
- MCO deployed: `kubectl get pods -n mco-system`
- MachineConfigPool exists: `kubectl get mcp`

## Usage

```bash
# Run single scenario
./run-scenario.sh 001-sysctl-basic

# Run all scenarios
./run-all.sh

# Via Makefile
make test-scenario SCENARIO=001-sysctl-basic
make test-scenarios
```

## Scenarios

| ID | Name | Description |
|----|------|-------------|
| 001 | sysctl-basic | Apply sysctl parameters via file |
| 002 | systemd-unit | Create and manage systemd unit |
| 003 | file-absent | Remove a file (state: absent) |

## Writing New Scenarios

1. Create directory: `NNN-scenario-name/`
2. Copy template files from existing scenario
3. Edit `manifest.yaml` with your MachineConfig
4. Define expectations in `expected.yaml`
5. Implement `pre-check.sh`, `verify.sh`, `cleanup.sh`
6. Update this README

## Exit Codes

- `0` - All checks passed
- `1` - One or more checks failed
- `2` - Prerequisites not met
