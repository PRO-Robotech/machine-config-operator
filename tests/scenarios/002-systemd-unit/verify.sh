#!/bin/bash
# Verify: Check that systemd units were created and activated correctly

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/common.sh"
source "$SCRIPT_DIR/../lib/minikube.sh"

scenario_header

# =============================================================================
# File assertions
# =============================================================================
log_step "Verifying unit file creation..."

assert_file_exists "/etc/systemd/system/mco-test.service"
assert_file_mode "/etc/systemd/system/mco-test.service" "644"
assert_file_owner "/etc/systemd/system/mco-test.service" "root:root"

assert_file_exists "/etc/systemd/system/mco-test.timer"
assert_file_mode "/etc/systemd/system/mco-test.timer" "644"
assert_file_owner "/etc/systemd/system/mco-test.timer" "root:root"

log_step "Verifying unit file content..."

assert_file_contains "/etc/systemd/system/mco-test.service" "Type=oneshot"
assert_file_contains "/etc/systemd/system/mco-test.service" "MCO Lite Test Service"
assert_file_contains "/etc/systemd/system/mco-test.timer" "OnBootSec=1min"
assert_file_contains "/etc/systemd/system/mco-test.timer" "mco-test.service"

# =============================================================================
# Systemd unit assertions (skipped in minikube - no systemd)
# =============================================================================
log_step "Verifying systemd units..."

# Check if systemd D-Bus is available (not just binary)
if minikube ssh "systemctl is-system-running" >/dev/null 2>&1; then
    # Timer should be known to systemd
    assert_unit_exists "mco-test.timer"
    assert_unit_exists "mco-test.service"

    # Timer should be enabled and active
    assert_unit_enabled "mco-test.timer"
    assert_unit_active "mco-test.timer"

    # Show timer status for debugging
    log_info "Timer status:"
    minikube_exec "systemctl status mco-test.timer --no-pager" || true

    # Show next trigger time
    log_info "Next trigger:"
    minikube_exec "systemctl list-timers mco-test.timer --no-pager" || true
else
    log_warn "Systemd not available in minikube - skipping unit state checks"
    log_info "Unit files were created, but systemd operations are no-op in this environment"
fi

# =============================================================================
# Kubernetes resource assertions
# =============================================================================
log_step "Verifying Kubernetes resources..."

# MachineConfig should exist
if kubectl get mc 50-systemd-test >/dev/null 2>&1; then
    test_passed "MachineConfig 50-systemd-test exists"
else
    test_failed "MachineConfig 50-systemd-test not found"
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

# =============================================================================
# Summary
# =============================================================================
print_summary
