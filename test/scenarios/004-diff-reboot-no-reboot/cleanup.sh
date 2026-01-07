#!/bin/bash
# Scenario 004: Cleanup
# Removes MachineConfigs created by this scenario

set -euo pipefail
source "$(dirname "$0")/../lib/common.sh"

log_step "Cleaning up scenario 004..."

kubectl delete machineconfig 004-no-reboot-test --ignore-not-found=true

log_success "Cleanup complete"
