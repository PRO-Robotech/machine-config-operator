#!/bin/bash
# Cleanup: Remove MachineConfig and revert changes

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/common.sh"
source "$SCRIPT_DIR/../lib/minikube.sh"

log_step "Cleaning up scenario: $(basename "$SCRIPT_DIR")"

# Delete MachineConfig
log_info "Deleting MachineConfig..."
kubectl delete mc 50-sysctl-test --ignore-not-found=true

# Remove test file from node
log_info "Removing test file from node..."
minikube_rm "/etc/sysctl.d/99-mco-test.conf"

# Reload sysctl to revert to defaults
log_info "Reloading sysctl..."
apply_sysctl

# Wait for agent to settle
log_info "Waiting for agent to settle..."
sleep 5

# Clean up state directory
if [[ -d "$SCRIPT_DIR/.state" ]]; then
    log_info "Removing state directory..."
    rm -rf "$SCRIPT_DIR/.state"
fi

log_success "Cleanup completed"
