#!/bin/bash
# Pre-check: Capture state before applying systemd unit

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/common.sh"
source "$SCRIPT_DIR/../lib/minikube.sh"

log_step "Pre-check for scenario: $(basename "$SCRIPT_DIR")"

# Create state directory
STATE_DIR="$SCRIPT_DIR/.state"
mkdir -p "$STATE_DIR"

# Check if units already exist
log_info "Checking if test units already exist..."

if minikube_file_exists "/etc/systemd/system/mco-test.service"; then
    log_warn "mco-test.service already exists!"
else
    log_info "mco-test.service does not exist (expected)"
fi

if minikube_file_exists "/etc/systemd/system/mco-test.timer"; then
    log_warn "mco-test.timer already exists!"
else
    log_info "mco-test.timer does not exist (expected)"
fi

# Check if MachineConfig already exists
log_info "Checking if MachineConfig already exists..."
if kubectl get mc 50-systemd-test >/dev/null 2>&1; then
    log_warn "MachineConfig 50-systemd-test already exists!"
else
    log_info "MachineConfig does not exist (expected)"
fi

# Check timer units already running
log_info "Checking for existing test timer..."
TIMER_STATUS=$(get_unit_status "mco-test.timer")
echo "mco-test.timer status: $TIMER_STATUS" > "$STATE_DIR/timer-before.txt"

log_success "Pre-check completed"
