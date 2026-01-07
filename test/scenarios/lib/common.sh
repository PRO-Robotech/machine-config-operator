#!/bin/bash
# Common functions for MCO scenario tests
# Source this file in scenario scripts: source "$(dirname "$0")/../lib/common.sh"

set -euo pipefail

# =============================================================================
# Colors
# =============================================================================
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# =============================================================================
# Logging
# =============================================================================
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[PASS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[FAIL]${NC} $1"
}

log_step() {
    echo -e "${YELLOW}[STEP]${NC} $1"
}

# =============================================================================
# Test state tracking
# =============================================================================
TESTS_PASSED=0
TESTS_FAILED=0

test_passed() {
    ((TESTS_PASSED++))
    log_success "$1"
}

test_failed() {
    ((TESTS_FAILED++))
    log_error "$1"
}

print_summary() {
    echo ""
    echo "============================================="
    echo -e "Tests passed: ${GREEN}${TESTS_PASSED}${NC}"
    echo -e "Tests failed: ${RED}${TESTS_FAILED}${NC}"
    echo "============================================="

    if [[ $TESTS_FAILED -gt 0 ]]; then
        return 1
    fi
    return 0
}

# =============================================================================
# Prerequisites
# =============================================================================
check_prerequisites() {
    log_step "Checking prerequisites..."

    # Check minikube
    if ! minikube status >/dev/null 2>&1; then
        log_error "Minikube is not running. Start with: minikube start"
        exit 2
    fi
    log_info "Minikube is running"

    # Check kubectl
    if ! kubectl cluster-info >/dev/null 2>&1; then
        log_error "Cannot connect to cluster"
        exit 2
    fi
    log_info "Cluster is accessible"

    # Check MCO components
    if ! kubectl get deployment -n mco-system mco-controller-manager >/dev/null 2>&1; then
        log_warn "MCO controller not found in mco-system namespace"
    else
        log_info "MCO controller is deployed"
    fi

    # Check MachineConfigPool CRD
    if ! kubectl get crd machineconfigpools.mco.in-cloud.io >/dev/null 2>&1; then
        log_error "MachineConfigPool CRD not installed"
        exit 2
    fi
    log_info "CRDs are installed"

    log_success "Prerequisites check passed"
}

# =============================================================================
# File assertions (via minikube ssh)
# =============================================================================
assert_file_exists() {
    local path="$1"
    local description="${2:-File exists: $path}"

    if minikube ssh "test -f '$path'" 2>/dev/null; then
        test_passed "$description"
        return 0
    else
        test_failed "File not found: $path"
        return 1
    fi
}

assert_file_not_exists() {
    local path="$1"
    local description="${2:-File does not exist: $path}"

    if minikube ssh "test -f '$path'" 2>/dev/null; then
        test_failed "File should not exist: $path"
        return 1
    else
        test_passed "$description"
        return 0
    fi
}

assert_file_contains() {
    local path="$1"
    local pattern="$2"
    local description="${3:-File $path contains: $pattern}"

    if minikube ssh "grep -q '$pattern' '$path'" 2>/dev/null; then
        test_passed "$description"
        return 0
    else
        test_failed "Pattern not found in $path: $pattern"
        return 1
    fi
}

assert_file_mode() {
    local path="$1"
    local expected_mode="$2"
    local description="${3:-File $path has mode $expected_mode}"

    local actual_mode
    actual_mode=$(minikube ssh "stat -c '%a' '$path'" 2>/dev/null | tr -d '\r\n')

    if [[ "$actual_mode" == "$expected_mode" ]]; then
        test_passed "$description"
        return 0
    else
        test_failed "File $path: expected mode $expected_mode, got $actual_mode"
        return 1
    fi
}

assert_file_owner() {
    local path="$1"
    local expected_owner="$2"
    local description="${3:-File $path owned by $expected_owner}"

    local actual_owner
    actual_owner=$(minikube ssh "stat -c '%U:%G' '$path'" 2>/dev/null | tr -d '\r\n')

    if [[ "$actual_owner" == "$expected_owner" ]]; then
        test_passed "$description"
        return 0
    else
        test_failed "File $path: expected owner $expected_owner, got $actual_owner"
        return 1
    fi
}

# =============================================================================
# Sysctl assertions
# =============================================================================
assert_sysctl() {
    local key="$1"
    local expected="$2"
    local description="${3:-sysctl $key = $expected}"

    local actual
    actual=$(minikube ssh "sysctl -n '$key'" 2>/dev/null | tr -d '\r\n')

    if [[ "$actual" == "$expected" ]]; then
        test_passed "$description"
        return 0
    else
        test_failed "sysctl $key: expected '$expected', got '$actual'"
        return 1
    fi
}

# =============================================================================
# Systemd assertions
# =============================================================================
assert_unit_exists() {
    local unit="$1"
    local description="${2:-Systemd unit exists: $unit}"

    if minikube ssh "systemctl cat '$unit'" >/dev/null 2>&1; then
        test_passed "$description"
        return 0
    else
        test_failed "Systemd unit not found: $unit"
        return 1
    fi
}

assert_unit_enabled() {
    local unit="$1"
    local description="${2:-Systemd unit enabled: $unit}"

    if minikube ssh "systemctl is-enabled '$unit'" 2>/dev/null | grep -q "enabled"; then
        test_passed "$description"
        return 0
    else
        test_failed "Systemd unit not enabled: $unit"
        return 1
    fi
}

assert_unit_active() {
    local unit="$1"
    local description="${2:-Systemd unit active: $unit}"

    if minikube ssh "systemctl is-active '$unit'" 2>/dev/null | grep -q "active"; then
        test_passed "$description"
        return 0
    else
        test_failed "Systemd unit not active: $unit"
        return 1
    fi
}

assert_unit_not_exists() {
    local unit="$1"
    local description="${2:-Systemd unit does not exist: $unit}"

    if minikube ssh "systemctl cat '$unit'" >/dev/null 2>&1; then
        test_failed "Systemd unit should not exist: $unit"
        return 1
    else
        test_passed "$description"
        return 0
    fi
}

# =============================================================================
# Kubernetes assertions
# =============================================================================
get_node_name() {
    kubectl get nodes -o jsonpath='{.items[0].metadata.name}' 2>/dev/null
}

assert_annotation() {
    local key="$1"
    local expected="$2"
    local description="${3:-Annotation $key = $expected}"

    local node
    node=$(get_node_name)

    # Escape dots in annotation key for jsonpath
    local escaped_key="${key//./\\.}"
    local actual
    actual=$(kubectl get node "$node" -o jsonpath="{.metadata.annotations['$escaped_key']}" 2>/dev/null)

    if [[ "$actual" == "$expected" ]]; then
        test_passed "$description"
        return 0
    else
        test_failed "Annotation $key: expected '$expected', got '$actual'"
        return 1
    fi
}

assert_annotation_matches() {
    local key="$1"
    local pattern="$2"
    local description="${3:-Annotation $key matches: $pattern}"

    local node
    node=$(get_node_name)

    local escaped_key="${key//./\\.}"
    local actual
    actual=$(kubectl get node "$node" -o jsonpath="{.metadata.annotations['$escaped_key']}" 2>/dev/null)

    if [[ "$actual" =~ $pattern ]]; then
        test_passed "$description"
        return 0
    else
        test_failed "Annotation $key: value '$actual' does not match pattern '$pattern'"
        return 1
    fi
}

# =============================================================================
# Wait helpers
# =============================================================================
wait_for_agent_idle() {
    local timeout="${1:-60}"
    log_step "Waiting for agent to become idle (timeout: ${timeout}s)..."

    local node
    node=$(get_node_name)

    local start_time
    start_time=$(date +%s)

    while true; do
        local state
        state=$(kubectl get node "$node" \
            -o jsonpath='{.metadata.annotations.mco\.in-cloud\.io/agent-state}' 2>/dev/null || echo "")

        if [[ "$state" == "idle" || "$state" == "done" ]]; then
            log_info "Agent state: $state"
            return 0
        fi

        local elapsed
        elapsed=$(($(date +%s) - start_time))
        if [[ $elapsed -ge $timeout ]]; then
            log_error "Timeout waiting for agent (current state: $state)"
            return 1
        fi

        log_info "Agent state: $state, waiting... (${elapsed}s/${timeout}s)"
        sleep 2
    done
}

wait_for_rmc() {
    local pool="${1:-workers}"
    local timeout="${2:-60}"
    log_step "Waiting for RenderedMachineConfig for pool '$pool' (timeout: ${timeout}s)..."

    local start_time
    start_time=$(date +%s)

    while true; do
        local rmc_count
        rmc_count=$(kubectl get rmc -l "mco.in-cloud.io/pool=$pool" --no-headers 2>/dev/null | wc -l)

        if [[ $rmc_count -gt 0 ]]; then
            local rmc_name
            rmc_name=$(kubectl get rmc -l "mco.in-cloud.io/pool=$pool" \
                -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
            log_info "RenderedMachineConfig created: $rmc_name"
            return 0
        fi

        local elapsed
        elapsed=$(($(date +%s) - start_time))
        if [[ $elapsed -ge $timeout ]]; then
            log_error "Timeout waiting for RenderedMachineConfig"
            return 1
        fi

        log_info "Waiting for RMC... (${elapsed}s/${timeout}s)"
        sleep 2
    done
}

# =============================================================================
# Manifest helpers
# =============================================================================
apply_manifest() {
    local manifest="$1"
    log_step "Applying manifest: $manifest"
    kubectl apply -f "$manifest"
}

delete_manifest() {
    local manifest="$1"
    log_step "Deleting manifest: $manifest"
    kubectl delete -f "$manifest" --ignore-not-found=true
}

# =============================================================================
# Scenario helpers
# =============================================================================
get_scenario_dir() {
    # Returns the directory containing the script that sourced common.sh
    # We look up the call stack to find the original script
    local i
    for ((i=${#BASH_SOURCE[@]}-1; i>=0; i--)); do
        local src="${BASH_SOURCE[$i]}"
        local dir="$(dirname "$(readlink -f "$src")")"
        # Skip lib directory
        if [[ "$(basename "$dir")" != "lib" ]]; then
            echo "$dir"
            return
        fi
    done
    # Fallback
    dirname "$(readlink -f "${BASH_SOURCE[1]}")"
}

get_scenario_name() {
    basename "$(get_scenario_dir)"
}

scenario_header() {
    local name
    name=$(get_scenario_name)
    echo ""
    echo "============================================="
    echo "Scenario: $name"
    echo "============================================="
    echo ""
}

# =============================================================================
# Reboot assertions (STORY-066)
# =============================================================================
assert_reboot_pending() {
    local expected="${1:-true}"
    local description="${2:-Reboot pending = $expected}"

    local node
    node=$(get_node_name)

    local actual
    actual=$(kubectl get node "$node" \
        -o jsonpath='{.metadata.annotations.mco\.in-cloud\.io/reboot-pending}' 2>/dev/null || echo "")

    # Handle empty annotation as "not pending"
    if [[ -z "$actual" ]]; then
        actual="false"
    fi

    if [[ "$actual" == "$expected" ]]; then
        test_passed "$description"
        return 0
    else
        test_failed "Reboot pending: expected '$expected', got '$actual'"
        return 1
    fi
}

assert_no_reboot_pending() {
    assert_reboot_pending "false" "No reboot pending"
}

wait_for_revision_applied() {
    local timeout="${1:-60}"
    log_step "Waiting for revision to be applied (current=desired) (timeout: ${timeout}s)..."

    local node
    node=$(get_node_name)

    local start_time
    start_time=$(date +%s)

    while true; do
        local current desired
        current=$(kubectl get node "$node" \
            -o jsonpath='{.metadata.annotations.mco\.in-cloud\.io/current-revision}' 2>/dev/null || echo "")
        desired=$(kubectl get node "$node" \
            -o jsonpath='{.metadata.annotations.mco\.in-cloud\.io/desired-revision}' 2>/dev/null || echo "")

        if [[ -n "$current" && "$current" == "$desired" ]]; then
            log_info "Revision applied: $current"
            return 0
        fi

        local elapsed
        elapsed=$(($(date +%s) - start_time))
        if [[ $elapsed -ge $timeout ]]; then
            log_error "Timeout waiting for revision (current: $current, desired: $desired)"
            return 1
        fi

        log_info "Waiting for revision... current=$current, desired=$desired (${elapsed}s/${timeout}s)"
        sleep 2
    done
}

get_current_revision() {
    local node
    node=$(get_node_name)
    kubectl get node "$node" \
        -o jsonpath='{.metadata.annotations.mco\.in-cloud\.io/current-revision}' 2>/dev/null
}

get_desired_revision() {
    local node
    node=$(get_node_name)
    kubectl get node "$node" \
        -o jsonpath='{.metadata.annotations.mco\.in-cloud\.io/desired-revision}' 2>/dev/null
}
