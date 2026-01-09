#!/bin/bash
# Minikube-specific helper functions
# Source after common.sh: source "$(dirname "$0")/../lib/minikube.sh"

# =============================================================================
# Minikube SSH helpers
# =============================================================================

# Execute command in minikube and return output
minikube_exec() {
    local cmd="$1"
    minikube ssh "$cmd" 2>/dev/null | tr -d '\r'
}

# Execute command as root in minikube
minikube_exec_sudo() {
    local cmd="$1"
    minikube ssh "sudo $cmd" 2>/dev/null | tr -d '\r'
}

# Read file content from minikube
minikube_cat() {
    local path="$1"
    minikube_exec "cat '$path'"
}

# Check if file exists in minikube
minikube_file_exists() {
    local path="$1"
    minikube ssh "test -f '$path'" 2>/dev/null
}

# Check if directory exists in minikube
minikube_dir_exists() {
    local path="$1"
    minikube ssh "test -d '$path'" 2>/dev/null
}

# Get file stats from minikube
minikube_stat() {
    local path="$1"
    local format="${2:-%a %U:%G}"  # default: mode owner:group
    minikube_exec "stat -c '$format' '$path'"
}

# =============================================================================
# Sysctl helpers
# =============================================================================

# Get sysctl value
get_sysctl() {
    local key="$1"
    minikube_exec "sysctl -n '$key'"
}

# Apply sysctl from file (simulate what agent should do)
apply_sysctl() {
    log_step "Applying sysctl configuration..."
    minikube_exec_sudo "sysctl --system"
}

# =============================================================================
# Systemd helpers
# =============================================================================

# Reload systemd daemon
systemd_reload() {
    minikube_exec_sudo "systemctl daemon-reload"
}

# Get unit status
get_unit_status() {
    local unit="$1"
    minikube_exec "systemctl is-active '$unit' 2>/dev/null || echo 'unknown'"
}

# Get unit enabled status
get_unit_enabled() {
    local unit="$1"
    minikube_exec "systemctl is-enabled '$unit' 2>/dev/null || echo 'unknown'"
}

# =============================================================================
# State capture helpers
# =============================================================================

# Capture file state before test (for comparison/rollback)
capture_file_state() {
    local path="$1"
    local output_file="$2"

    echo "# File state capture: $path" > "$output_file"
    echo "# Captured at: $(date -Iseconds)" >> "$output_file"
    echo "" >> "$output_file"

    if minikube_file_exists "$path"; then
        echo "exists=true" >> "$output_file"
        echo "mode=$(minikube_stat "$path" '%a')" >> "$output_file"
        echo "owner=$(minikube_stat "$path" '%U:%G')" >> "$output_file"
        echo "content<<EOF" >> "$output_file"
        minikube_cat "$path" >> "$output_file"
        echo "EOF" >> "$output_file"
    else
        echo "exists=false" >> "$output_file"
    fi
}

# Capture sysctl values before test
capture_sysctl_state() {
    local output_file="$1"
    shift
    local keys=("$@")

    echo "# Sysctl state capture" > "$output_file"
    echo "# Captured at: $(date -Iseconds)" >> "$output_file"
    echo "" >> "$output_file"

    for key in "${keys[@]}"; do
        local value
        value=$(get_sysctl "$key" 2>/dev/null || echo "N/A")
        echo "$key=$value" >> "$output_file"
    done
}

# =============================================================================
# Cleanup helpers
# =============================================================================

# Remove file from minikube
minikube_rm() {
    local path="$1"
    log_info "Removing file: $path"
    minikube_exec_sudo "rm -f '$path'"
}

# Restore file from captured state
restore_file_state() {
    local state_file="$1"

    if [[ ! -f "$state_file" ]]; then
        log_warn "State file not found: $state_file"
        return 1
    fi

    local path
    path=$(grep -m1 "^# File state capture:" "$state_file" | cut -d: -f2 | xargs)

    local exists
    exists=$(grep "^exists=" "$state_file" | cut -d= -f2)

    if [[ "$exists" == "false" ]]; then
        log_info "Restoring: removing file $path"
        minikube_rm "$path"
    else
        log_info "Restoring: file $path (keeping current state)"
        # For full restore, would need to write content back
        # Left as-is for now since cleanup usually just removes test files
    fi
}

# =============================================================================
# Debugging helpers
# =============================================================================

# Show file content (for debugging)
show_file() {
    local path="$1"
    echo "=== $path ==="
    minikube_cat "$path" || echo "(file not found)"
    echo "=== end ==="
}

# Show all node annotations
show_annotations() {
    local node
    node=$(get_node_name)
    log_info "Annotations for node: $node"
    kubectl get node "$node" -o jsonpath='{.metadata.annotations}' | jq -r 'to_entries[] | "\(.key) = \(.value)"' 2>/dev/null || \
    kubectl get node "$node" -o yaml | grep -A 100 "annotations:" | head -50
}

# Show MCO resources status
show_mco_status() {
    log_info "MachineConfigs:"
    kubectl get mc 2>/dev/null || echo "  (none)"

    log_info "MachineConfigPools:"
    kubectl get mcp 2>/dev/null || echo "  (none)"

    log_info "RenderedMachineConfigs:"
    kubectl get rmc 2>/dev/null || echo "  (none)"
}
