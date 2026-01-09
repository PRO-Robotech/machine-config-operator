#!/bin/bash
# Cleanup: Remove systemd units and MachineConfig

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/common.sh"
source "$SCRIPT_DIR/../lib/minikube.sh"

log_step "Cleaning up scenario: $(basename "$SCRIPT_DIR")"

# Stop and disable timer
log_info "Stopping and disabling timer..."
minikube_exec_sudo "systemctl stop mco-test.timer" || true
minikube_exec_sudo "systemctl disable mco-test.timer" || true

# Stop service if running
log_info "Stopping service..."
minikube_exec_sudo "systemctl stop mco-test.service" || true

# Delete MachineConfig
log_info "Deleting MachineConfig..."
kubectl delete mc 50-systemd-test --ignore-not-found=true

# Remove unit files
log_info "Removing unit files..."
minikube_rm "/etc/systemd/system/mco-test.service"
minikube_rm "/etc/systemd/system/mco-test.timer"

# Reload systemd
log_info "Reloading systemd daemon..."
systemd_reload

# Wait for agent to settle
log_info "Waiting for agent to settle..."
sleep 5

# Clean up state directory
if [[ -d "$SCRIPT_DIR/.state" ]]; then
    log_info "Removing state directory..."
    rm -rf "$SCRIPT_DIR/.state"
fi

log_success "Cleanup completed"
