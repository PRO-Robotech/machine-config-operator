#!/bin/bash
# Pre-check: Capture state before applying MachineConfig
# This is useful for comparison and rollback

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/common.sh"
source "$SCRIPT_DIR/../lib/minikube.sh"

log_step "Pre-check for scenario: $(basename "$SCRIPT_DIR")"

# Create state directory
STATE_DIR="$SCRIPT_DIR/.state"
mkdir -p "$STATE_DIR"

# Capture current sysctl values
log_info "Capturing current sysctl values..."
capture_sysctl_state "$STATE_DIR/sysctl-before.txt" \
    "fs.inotify.max_user_watches" \
    "net.core.somaxconn" \
    "vm.swappiness"

cat "$STATE_DIR/sysctl-before.txt"

# Capture file state (should not exist before test)
log_info "Checking if test file already exists..."
if minikube_file_exists "/etc/sysctl.d/99-mco-test.conf"; then
    log_warn "Test file already exists! Capturing current state..."
    capture_file_state "/etc/sysctl.d/99-mco-test.conf" "$STATE_DIR/file-before.txt"
else
    log_info "Test file does not exist (expected)"
fi

# Check if MachineConfig already exists
log_info "Checking if MachineConfig already exists..."
if kubectl get mc 50-sysctl-test >/dev/null 2>&1; then
    log_warn "MachineConfig 50-sysctl-test already exists!"
else
    log_info "MachineConfig does not exist (expected)"
fi

log_success "Pre-check completed"
