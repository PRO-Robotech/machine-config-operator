#!/bin/bash
# Run a single MCO test scenario
#
# Usage: ./run-scenario.sh <scenario-name>
# Example: ./run-scenario.sh 001-sysctl-basic

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source common functions
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/minikube.sh"

# =============================================================================
# Usage
# =============================================================================
usage() {
    echo "Usage: $0 <scenario-name> [options]"
    echo ""
    echo "Options:"
    echo "  --skip-prerequisites    Skip prerequisite checks"
    echo "  --skip-cleanup          Don't run cleanup after test"
    echo "  --cleanup-only          Only run cleanup, no test"
    echo "  --debug                 Show debug output"
    echo "  -h, --help              Show this help"
    echo ""
    echo "Examples:"
    echo "  $0 001-sysctl-basic"
    echo "  $0 002-systemd-unit --skip-cleanup"
    exit 1
}

# =============================================================================
# Parse arguments
# =============================================================================
SCENARIO=""
SKIP_PREREQUISITES=false
SKIP_CLEANUP=false
CLEANUP_ONLY=false
DEBUG=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --skip-prerequisites)
            SKIP_PREREQUISITES=true
            shift
            ;;
        --skip-cleanup)
            SKIP_CLEANUP=true
            shift
            ;;
        --cleanup-only)
            CLEANUP_ONLY=true
            shift
            ;;
        --debug)
            DEBUG=true
            set -x
            shift
            ;;
        -h|--help)
            usage
            ;;
        -*)
            log_error "Unknown option: $1"
            usage
            ;;
        *)
            if [[ -z "$SCENARIO" ]]; then
                SCENARIO="$1"
            else
                log_error "Multiple scenarios specified"
                usage
            fi
            shift
            ;;
    esac
done

if [[ -z "$SCENARIO" ]]; then
    log_error "Scenario name required"
    usage
fi

# =============================================================================
# Find scenario directory
# =============================================================================
SCENARIO_DIR="$SCRIPT_DIR/$SCENARIO"

if [[ ! -d "$SCENARIO_DIR" ]]; then
    log_error "Scenario not found: $SCENARIO"
    echo ""
    echo "Available scenarios:"
    for dir in "$SCRIPT_DIR"/[0-9][0-9][0-9]-*/; do
        if [[ -d "$dir" ]]; then
            echo "  $(basename "$dir")"
        fi
    done
    exit 1
fi

# =============================================================================
# Run scenario
# =============================================================================
echo ""
echo "============================================="
echo "Scenario: $SCENARIO"
echo "============================================="
echo ""

# Check prerequisites
if [[ "$SKIP_PREREQUISITES" == "false" && "$CLEANUP_ONLY" == "false" ]]; then
    check_prerequisites
fi

# Cleanup only mode
if [[ "$CLEANUP_ONLY" == "true" ]]; then
    log_step "Running cleanup only..."
    if [[ -x "$SCENARIO_DIR/cleanup.sh" ]]; then
        "$SCENARIO_DIR/cleanup.sh"
        log_success "Cleanup completed"
    else
        log_warn "No cleanup script found"
    fi
    exit 0
fi

# Track overall result
RESULT=0

# Phase 1: Pre-check
if [[ -x "$SCENARIO_DIR/pre-check.sh" ]]; then
    log_step "Running pre-check..."
    "$SCENARIO_DIR/pre-check.sh" || true
fi

# Phase 2: Apply manifest
if [[ -f "$SCENARIO_DIR/manifest.yaml" ]]; then
    # Capture current revision before applying
    CURRENT_REV=$(kubectl get nodes -o jsonpath='{.items[0].metadata.annotations.mco\.in-cloud\.io/current-revision}' 2>/dev/null || echo "")
    log_info "Current revision before apply: $CURRENT_REV"

    log_step "Applying manifest..."
    kubectl apply -f "$SCENARIO_DIR/manifest.yaml"

    # Wait for debounce (controller needs time to create new RMC)
    log_info "Waiting for controller debounce (7s)..."
    sleep 7

    # Wait for new revision to be set as desired
    log_step "Waiting for new revision..."
    for i in {1..30}; do
        DESIRED_REV=$(kubectl get nodes -o jsonpath='{.items[0].metadata.annotations.mco\.in-cloud\.io/desired-revision}' 2>/dev/null || echo "")
        if [[ -n "$DESIRED_REV" && "$DESIRED_REV" != "$CURRENT_REV" ]]; then
            log_info "New revision: $DESIRED_REV"
            break
        fi
        sleep 2
    done

    # Wait for agent to apply the new revision
    log_step "Waiting for agent to apply new revision..."
    if ! wait_for_agent_idle 120; then
        log_error "Agent did not reach idle state"
        RESULT=1
    fi

    # Verify current revision matches desired
    FINAL_REV=$(kubectl get nodes -o jsonpath='{.items[0].metadata.annotations.mco\.in-cloud\.io/current-revision}' 2>/dev/null || echo "")
    log_info "Final revision: $FINAL_REV"
else
    log_warn "No manifest.yaml found in $SCENARIO_DIR"
fi

# Phase 3: Verify
if [[ -x "$SCENARIO_DIR/verify.sh" ]]; then
    log_step "Running verification..."
    if ! "$SCENARIO_DIR/verify.sh"; then
        RESULT=1
    fi
else
    log_warn "No verify.sh found in $SCENARIO_DIR"
fi

# Phase 4: Cleanup
if [[ "$SKIP_CLEANUP" == "false" ]]; then
    if [[ -x "$SCENARIO_DIR/cleanup.sh" ]]; then
        log_step "Running cleanup..."
        "$SCENARIO_DIR/cleanup.sh" || log_warn "Cleanup had errors"
    fi
fi

# Summary
echo ""
echo "============================================="
if [[ $RESULT -eq 0 ]]; then
    log_success "Scenario $SCENARIO: PASSED"
else
    log_error "Scenario $SCENARIO: FAILED"
fi
echo "============================================="

exit $RESULT
