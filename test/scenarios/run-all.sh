#!/usr/bin/env bash
# Run all MCO test scenarios
#
# Usage: ./run-all.sh [options]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source common functions
source "$SCRIPT_DIR/lib/common.sh"

# =============================================================================
# Usage
# =============================================================================
usage() {
    echo "Usage: $0 [options]"
    echo ""
    echo "Options:"
    echo "  --skip-prerequisites    Skip prerequisite checks"
    echo "  --stop-on-failure       Stop after first failed scenario"
    echo "  --cleanup-all           Only run cleanup for all scenarios"
    echo "  --list                  List available scenarios"
    echo "  -h, --help              Show this help"
    exit 1
}

# =============================================================================
# Parse arguments
# =============================================================================
SKIP_PREREQUISITES=false
STOP_ON_FAILURE=false
CLEANUP_ALL=false
LIST_ONLY=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --skip-prerequisites)
            SKIP_PREREQUISITES=true
            shift
            ;;
        --stop-on-failure)
            STOP_ON_FAILURE=true
            shift
            ;;
        --cleanup-all)
            CLEANUP_ALL=true
            shift
            ;;
        --list)
            LIST_ONLY=true
            shift
            ;;
        -h|--help)
            usage
            ;;
        *)
            log_error "Unknown option: $1"
            usage
            ;;
    esac
done

# =============================================================================
# Find scenarios
# =============================================================================
SCENARIOS=()
for dir in "$SCRIPT_DIR"/[0-9][0-9][0-9]-*/; do
    if [[ -d "$dir" ]]; then
        SCENARIOS+=("$(basename "$dir")")
    fi
done

if [[ ${#SCENARIOS[@]} -eq 0 ]]; then
    log_error "No scenarios found"
    exit 1
fi

# List only mode
if [[ "$LIST_ONLY" == "true" ]]; then
    echo "Available scenarios:"
    for scenario in "${SCENARIOS[@]}"; do
        echo "  $scenario"
    done
    exit 0
fi

# =============================================================================
# Run scenarios
# =============================================================================
echo ""
echo "============================================="
echo "MCO Test Scenarios"
echo "============================================="
echo ""
echo "Found ${#SCENARIOS[@]} scenario(s):"
for scenario in "${SCENARIOS[@]}"; do
    echo "  - $scenario"
done
echo ""

# Check prerequisites once
if [[ "$SKIP_PREREQUISITES" == "false" && "$CLEANUP_ALL" == "false" ]]; then
    check_prerequisites
fi

# Track results (avoid associative arrays for macOS compatibility)
PASSED=0
FAILED=0
SKIPPED=0
RESULTS_SCENARIOS=""
RESULTS_STATUS=""

# Cleanup all mode
if [[ "$CLEANUP_ALL" == "true" ]]; then
    log_step "Running cleanup for all scenarios..."
    for scenario in "${SCENARIOS[@]}"; do
        log_info "Cleaning up: $scenario"
        "$SCRIPT_DIR/run-scenario.sh" "$scenario" --cleanup-only || true
    done
    log_success "Cleanup completed"
    exit 0
fi

# Run each scenario
for scenario in "${SCENARIOS[@]}"; do
    echo ""
    echo "---------------------------------------------"
    log_step "Running: $scenario"
    echo "---------------------------------------------"

    OPTS="--skip-prerequisites"

    if "$SCRIPT_DIR/run-scenario.sh" "$scenario" $OPTS; then
        RESULTS_SCENARIOS="$RESULTS_SCENARIOS $scenario"
        RESULTS_STATUS="$RESULTS_STATUS PASSED"
        ((PASSED++))
    else
        RESULTS_SCENARIOS="$RESULTS_SCENARIOS $scenario"
        RESULTS_STATUS="$RESULTS_STATUS FAILED"
        ((FAILED++))

        if [[ "$STOP_ON_FAILURE" == "true" ]]; then
            log_warn "Stopping due to --stop-on-failure"
            SKIPPED=$((${#SCENARIOS[@]} - PASSED - FAILED))
            break
        fi
    fi
done

# =============================================================================
# Summary
# =============================================================================
echo ""
echo "============================================="
echo "RESULTS SUMMARY"
echo "============================================="
echo ""

# Parse results arrays
SCENARIOS_ARR=($RESULTS_SCENARIOS)
STATUS_ARR=($RESULTS_STATUS)

for i in "${!SCENARIOS[@]}"; do
    scenario="${SCENARIOS[$i]}"
    # Find result for this scenario
    result="SKIPPED"
    for j in "${!SCENARIOS_ARR[@]}"; do
        if [[ "${SCENARIOS_ARR[$j]}" == "$scenario" ]]; then
            result="${STATUS_ARR[$j]}"
            break
        fi
    done

    case $result in
        PASSED)
            echo -e "  ${GREEN}[PASS]${NC} $scenario"
            ;;
        FAILED)
            echo -e "  ${RED}[FAIL]${NC} $scenario"
            ;;
        SKIPPED)
            echo -e "  ${YELLOW}[SKIP]${NC} $scenario"
            ((SKIPPED++))
            ;;
    esac
done

echo ""
echo "---------------------------------------------"
echo -e "Passed:  ${GREEN}${PASSED}${NC}"
echo -e "Failed:  ${RED}${FAILED}${NC}"
echo -e "Skipped: ${YELLOW}${SKIPPED}${NC}"
echo "---------------------------------------------"

if [[ $FAILED -gt 0 ]]; then
    exit 1
fi
exit 0
