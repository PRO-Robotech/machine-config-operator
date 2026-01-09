#!/bin/bash
# Verify: Check that MachineConfig was applied correctly

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/common.sh"
source "$SCRIPT_DIR/../lib/minikube.sh"

scenario_header

# =============================================================================
# File assertions
# =============================================================================
log_step "Verifying file creation..."

assert_file_exists "/etc/sysctl.d/99-mco-test.conf"
assert_file_mode "/etc/sysctl.d/99-mco-test.conf" "644"
assert_file_owner "/etc/sysctl.d/99-mco-test.conf" "root:root"

log_step "Verifying file content..."

assert_file_contains "/etc/sysctl.d/99-mco-test.conf" "fs.inotify.max_user_watches = 524288"
assert_file_contains "/etc/sysctl.d/99-mco-test.conf" "net.core.somaxconn = 4096"
assert_file_contains "/etc/sysctl.d/99-mco-test.conf" "vm.swappiness = 10"

# =============================================================================
# Sysctl value assertions
# =============================================================================
log_step "Verifying sysctl values..."

# Note: Agent should apply sysctl automatically, but if not, we can trigger it
# First check if values are already applied
NEEDS_SYSCTL_APPLY=false
CURRENT_INOTIFY=$(get_sysctl "fs.inotify.max_user_watches" 2>/dev/null || echo "0")
if [[ "$CURRENT_INOTIFY" != "524288" ]]; then
    log_info "Sysctl not yet applied, triggering sysctl --system..."
    NEEDS_SYSCTL_APPLY=true
    apply_sysctl
fi

assert_sysctl "fs.inotify.max_user_watches" "524288"
assert_sysctl "net.core.somaxconn" "4096"
assert_sysctl "vm.swappiness" "10"

# =============================================================================
# Kubernetes resource assertions
# =============================================================================
log_step "Verifying Kubernetes resources..."

# MachineConfig should exist
if kubectl get mc 50-sysctl-test >/dev/null 2>&1; then
    test_passed "MachineConfig 50-sysctl-test exists"
else
    test_failed "MachineConfig 50-sysctl-test not found"
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
