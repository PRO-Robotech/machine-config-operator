#!/bin/bash
# Cleanup: Remove MachineConfig and test directory

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/common.sh"
source "$SCRIPT_DIR/../lib/minikube.sh"

log_step "Cleaning up scenario: $(basename "$SCRIPT_DIR")"

# Delete MachineConfig
log_info "Deleting MachineConfig..."
kubectl delete mc 50-file-absent-test --ignore-not-found=true

# Remove test directory (file should already be gone)
log_info "Removing test directory..."
minikube_exec_sudo "rm -rf /etc/mco-test"

# Wait for agent to settle
log_info "Waiting for agent to settle..."
sleep 5

# Clean up state directory
if [[ -d "$SCRIPT_DIR/.state" ]]; then
    log_info "Removing state directory..."
    rm -rf "$SCRIPT_DIR/.state"
fi

log_success "Cleanup completed"
