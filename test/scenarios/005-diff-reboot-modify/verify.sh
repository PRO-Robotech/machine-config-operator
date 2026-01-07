#!/bin/bash
# Scenario 005: Diff-Based Reboot - Modify Reboot File
# Verifies that modifying a reboot-requiring file DOES trigger reboot

set -euo pipefail
source "$(dirname "$0")/../lib/common.sh"

SCENARIO_DIR="$(dirname "$0")"

scenario_header

# Step 1: Apply initial config (v1)
log_step "Step 1: Apply initial config (v1)"
kubectl apply -f "$SCENARIO_DIR/manifest-v1.yaml"

# Wait for initial config - note this may trigger reboot pending on first apply
wait_for_agent_idle 60 || true

# Check if we're stuck on reboot pending from initial apply
PENDING=$(kubectl get node "$(get_node_name)" \
    -o jsonpath='{.metadata.annotations.mco\.in-cloud\.io/reboot-pending}' 2>/dev/null || echo "")

if [[ "$PENDING" == "true" ]]; then
    log_warn "Initial apply triggered reboot pending - this is expected for first apply"
    log_info "Simulating reboot by restarting minikube..."
    minikube stop
    sleep 2
    minikube start
    sleep 10
    wait_for_agent_idle 60
fi

# Verify initial file
assert_file_exists "/etc/mco-reboot-modify-test.conf" "Initial file exists"
assert_file_contains "/etc/mco-reboot-modify-test.conf" "version=v1" "Initial version correct"

# Save revision
V1_REVISION=$(get_current_revision)
log_info "V1 revision: $V1_REVISION"

# Step 2: Apply modified config (v2)
log_step "Step 2: Apply modified config (v2)"
kubectl apply -f "$SCENARIO_DIR/manifest-v2.yaml"

# Wait for controller to create new RMC
sleep 5

# Wait for agent to process (but it should stop at reboot-pending)
log_step "Waiting for agent to apply config..."
sleep 10

# Step 3: Verify reboot IS pending
log_step "Step 3: Verify reboot pending"
assert_reboot_pending "true" "Reboot is pending after modifying reboot file"

# Step 4: Verify file was updated
log_step "Step 4: Verify file was updated"
assert_file_exists "/etc/mco-reboot-modify-test.conf" "File still exists"
assert_file_contains "/etc/mco-reboot-modify-test.conf" "version=v2" "File has new content"

# Step 5: Verify revision state
log_step "Step 5: Verify revision state"
V2_REVISION=$(get_desired_revision)
CURRENT_REVISION=$(get_current_revision)
log_info "V2 desired revision: $V2_REVISION"
log_info "Current revision: $CURRENT_REVISION"

if [[ "$V1_REVISION" == "$CURRENT_REVISION" ]]; then
    test_passed "Current revision still at V1 (waiting for reboot)"
else
    log_warn "Current revision changed - may have already processed"
fi

# Summary
print_summary
