#!/bin/bash
# Scenario 005: Cleanup

set -euo pipefail
source "$(dirname "$0")/../lib/common.sh"

log_step "Cleaning up scenario 005..."

kubectl delete machineconfig 005-reboot-modify --ignore-not-found=true

log_success "Cleanup complete"
