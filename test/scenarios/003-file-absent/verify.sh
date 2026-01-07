#!/bin/bash
# Verify: Check that file was deleted

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/common.sh"
source "$SCRIPT_DIR/../lib/minikube.sh"

scenario_header

# =============================================================================
# File absence assertions
# =============================================================================
log_step "Verifying file deletion..."

# The main test: file should NOT exist
assert_file_not_exists "/etc/mco-test/delete-me.txt"

# Directory may still exist (MCO doesn't delete parent dirs)
log_info "Checking parent directory..."
if minikube_dir_exists "/etc/mco-test"; then
    log_info "Parent directory /etc/mco-test still exists (expected - MCO only deletes files)"
else
    log_info "Parent directory was also removed"
fi

# =============================================================================
# Kubernetes resource assertions
# =============================================================================
log_step "Verifying Kubernetes resources..."

# MachineConfig should exist
if kubectl get mc 50-file-absent-test >/dev/null 2>&1; then
    test_passed "MachineConfig 50-file-absent-test exists"
else
    test_failed "MachineConfig 50-file-absent-test not found"
fi

# =============================================================================
# Node annotation assertions
# =============================================================================
log_step "Verifying node annotations..."

# Agent state should be idle or done
NODE=$(get_node_name)
AGENT_STATE=$(kubectl get node "$NODE" -o jsonpath='{.metadata.annotations.mco\.in-cloud\.io/agent-state}' 2>/dev/null)
if [[ "$AGENT_STATE" == "idle" || "$AGENT_STATE" == "done" ]]; then
    test_passed "Agent state is $AGENT_STATE"
else
    test_failed "Agent state: expected 'idle' or 'done', got '$AGENT_STATE'"
fi

# Current revision should match pattern
assert_annotation_matches "mco.in-cloud.io/current-revision" "^worker-"

# No error annotation (or empty)
NODE=$(get_node_name)
LAST_ERROR=$(kubectl get node "$NODE" -o jsonpath='{.metadata.annotations.mco\.in-cloud\.io/last-error}' 2>/dev/null || echo "")
if [[ -z "$LAST_ERROR" ]]; then
    test_passed "No error annotation"
else
    log_warn "Last error: $LAST_ERROR"
fi

# =============================================================================
# Summary
# =============================================================================
print_summary
