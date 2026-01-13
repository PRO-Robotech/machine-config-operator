#!/bin/bash
# Pre-check: Create the test file that will be deleted

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/common.sh"
source "$SCRIPT_DIR/../lib/minikube.sh"

log_step "Pre-check for scenario: $(basename "$SCRIPT_DIR")"

# Create state directory
STATE_DIR="$SCRIPT_DIR/.state"
mkdir -p "$STATE_DIR"

# Create test directory and file
log_info "Creating test file that will be deleted..."
minikube_exec_sudo "mkdir -p /etc/mco-test"
minikube_exec_sudo "echo 'This file should be deleted by MCO agent' > /etc/mco-test/delete-me.txt"
minikube_exec_sudo "chmod 644 /etc/mco-test/delete-me.txt"

# Verify file was created
if minikube_file_exists "/etc/mco-test/delete-me.txt"; then
    test_passed "Test file created: /etc/mco-test/delete-me.txt"

    # Show file for debugging
    log_info "File content:"
    minikube_cat "/etc/mco-test/delete-me.txt"

    # Save state
    echo "created=true" > "$STATE_DIR/file-before.txt"
else
    test_failed "Failed to create test file"
    exit 1
fi

# Check if MachineConfig already exists
log_info "Checking if MachineConfig already exists..."
if kubectl get mc 50-file-absent-test >/dev/null 2>&1; then
    log_warn "MachineConfig 50-file-absent-test already exists!"
else
    log_info "MachineConfig does not exist (expected)"
fi

log_success "Pre-check completed - file ready for deletion test"
