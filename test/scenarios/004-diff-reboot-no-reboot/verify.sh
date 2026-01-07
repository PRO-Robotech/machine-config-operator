#!/bin/bash
# Scenario 004: Diff-Based Reboot - No Reboot Required
# Verifies that adding a non-reboot MC does NOT trigger reboot
#
# This test verifies the diff-based reboot logic: when only non-reboot
# files are added/changed, no reboot should be triggered.

set -euo pipefail
source "$(dirname "$0")/../lib/common.sh"

scenario_header

# Step 1: Verify agent reached idle state (run-scenario.sh waits for this)
log_step "Step 1: Verify agent state"
STATE=$(kubectl get node "$(get_node_name)" \
    -o jsonpath='{.metadata.annotations.mco\.in-cloud\.io/agent-state}' 2>/dev/null || echo "")
if [[ "$STATE" == "idle" || "$STATE" == "done" ]]; then
    test_passed "Agent state is '$STATE'"
else
    test_failed "Agent state: expected 'idle' or 'done', got '$STATE'"
fi

# Step 2: Verify no reboot was triggered
log_step "Step 2: Verify no reboot pending"
assert_no_reboot_pending

# Step 3: Verify file was created
log_step "Step 3: Verify file on node"
assert_file_exists "/etc/mco-scenario-004.conf" "Scenario file exists"
assert_file_contains "/etc/mco-scenario-004.conf" "scenario=004" "File has correct content"

# Step 4: Verify revision was updated
log_step "Step 4: Verify revision applied"
CURRENT=$(get_current_revision)
DESIRED=$(get_desired_revision)
if [[ "$CURRENT" == "$DESIRED" ]]; then
    test_passed "Current revision matches desired: $CURRENT"
else
    test_failed "Revision mismatch: current=$CURRENT, desired=$DESIRED"
fi

# Summary
print_summary
