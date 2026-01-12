# API Reference

Полная спецификация API MCO Lite v1alpha1.

---

## MachineConfig

**API Group:** mco.in-cloud.io/v1alpha1  
**Kind:** MachineConfig  
**Short Name:** mc  
**Scope:** Cluster

### Spec

```yaml
spec:
  priority: int              # 0-99999, default: 50
  files:                     # []FileSpec
    - path: string           # Required, absolute path
      content: string        # Required if state=present
      mode: int              # Decimal, default: 420 (0644)
      owner: string          # "user:group", default: "root:root"
      state: string          # "present" or "absent", default: "present"
  systemd:
    units:                   # []UnitSpec
      - name: string         # Required, e.g. "nginx.service"
        enabled: bool        # Optional
        state: string        # "started", "stopped", "restarted", "reloaded"
        mask: bool           # default: false
  reboot:
    required: bool           # default: false
    reason: string           # Optional description
```

### FileSpec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `path` | string | Yes | — | Absolute path on host filesystem |
| `content` | string | Yes* | — | File content (* required if state=present) |
| `mode` | int | No | 420 | Unix permissions in decimal |
| `owner` | string | No | "root:root" | Owner in user:group format |
| `state` | enum | No | "present" | "present" or "absent" |

### UnitSpec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | Yes | — | Unit name with extension (.service, .socket, etc.) |
| `enabled` | *bool | No | nil | Enable/disable autostart |
| `state` | enum | No | — | "started", "stopped", "restarted", "reloaded" |
| `mask` | bool | No | false | Mask unit (prevent starting) |

---

## MachineConfigPool

**API Group:** mco.in-cloud.io/v1alpha1  
**Kind:** MachineConfigPool  
**Short Name:** mcp  
**Scope:** Cluster

### Spec

```yaml
spec:
  nodeSelector:              # *metav1.LabelSelector
    matchLabels: {}
    matchExpressions: []
  machineConfigSelector:     # *metav1.LabelSelector
    matchLabels: {}
    matchExpressions: []
  rollout:
    maxUnavailable: IntOrString    # default: 1
    debounceSeconds: int           # 0-3600, default: 30
    applyTimeoutSeconds: int       # 60-3600, default: 600
    drainTimeoutSeconds: int       # 60-86400, default: 3600
    drainRetrySeconds: int         # 10-1800, default: auto
  reboot:
    strategy: string               # "Never" or "IfRequired", default: "Never"
    minIntervalSeconds: int        # default: 1800
  revisionHistory:
    limit: int                     # default: 5
  paused: bool                     # default: false
```

### RolloutConfig

| Field | Type | Required | Default | Range | Description |
|-------|------|----------|---------|-------|-------------|
| `maxUnavailable` | IntOrString | No | 1 | 1+ or % | Max nodes unavailable during update |
| `debounceSeconds` | int | No | 30 | 0-3600 | Delay before rendering |
| `applyTimeoutSeconds` | int | No | 600 | 60-3600 | Timeout for config apply |
| `drainTimeoutSeconds` | int | No | 3600 | 60-86400 | Timeout for node drain |
| `drainRetrySeconds` | int | No | auto | 10-1800 | Interval between drain retries |

### RebootConfig

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `strategy` | enum | No | "Never" | "Never" or "IfRequired" |
| `minIntervalSeconds` | int | No | 1800 | Min seconds between reboots |

### RevisionHistoryConfig

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `limit` | int | No | 5 | Max old RMCs to keep |

### Status

```yaml
status:
  targetRevision: string            # Target RMC name
  currentRevision: string           # Most common revision
  lastSuccessfulRevision: string    # Last successful revision
  machineCount: int                 # Total nodes
  readyMachineCount: int            # Nodes with state=done and current=target
  updatedMachineCount: int          # Nodes with current=target
  updatingMachineCount: int         # Nodes with state=applying
  degradedMachineCount: int         # Nodes with state=error
  cordonedMachineCount: int         # Cordoned nodes
  drainingMachineCount: int         # Nodes being drained
  pendingRebootCount: int           # Nodes with reboot-pending
  conditions: []metav1.Condition    # Status conditions
```

### Status Conditions

| Type | Status | Description |
|------|--------|-------------|
| `Ready` | True/False | All nodes updated and no errors |
| `Updating` | True/False | At least one node not at target revision |
| `Draining` | True/False | Drain operation in progress |
| `Degraded` | True/False | At least one node has error (incl. render failures) |
| `PoolOverlap` | True/False | Node matches multiple pools |
| `DrainStuck` | True/False | Drain exceeded timeout |

#### Condition Details

**Ready**
- `True`: All nodes have target revision and no errors
- `False` + `Reason=RolloutInProgress`: Nodes are being updated
- `False` + `Reason=Degraded`: Nodes have errors
- `False` + `Reason=NoMachineConfigs`: No configs in pool

**Degraded**
- `Reason=NodeErrors`: Nodes in error state
- `Reason=RenderFailed`: Failed to create RenderedMachineConfig

---

## RenderedMachineConfig

**API Group:** mco.in-cloud.io/v1alpha1  
**Kind:** RenderedMachineConfig  
**Short Name:** rmc  
**Scope:** Cluster

> **Note:** RenderedMachineConfig is created automatically by the controller. Do not create or modify manually.

### Spec

```yaml
spec:
  poolName: string           # Parent pool name
  revision: string           # Short hash (10 chars)
  configHash: string         # Full SHA256 hash
  sources:                   # []SourceReference
    - name: string           # MC name
      priority: int          # MC priority
  config:
    files: []FileSpec        # Merged files
    systemd:
      units: []UnitSpec      # Merged units
    reboot:
      required: bool
      reason: string
```

### Status

```yaml
status:
  observedGeneration: int
  conditions: []metav1.Condition
```

---

## Node Annotations

### Written by Controller

| Annotation | Format | Description |
|------------|--------|-------------|
| `mco.in-cloud.io/desired-revision` | `rendered-<pool>-<hash>` | Target revision |
| `mco.in-cloud.io/pool` | string | Pool name |
| `mco.in-cloud.io/cordoned` | "true" | Node cordoned by MCO |
| `mco.in-cloud.io/drain-started-at` | RFC3339 | Drain start time |
| `mco.in-cloud.io/drain-retry-count` | int string | Drain retry count |
| `mco.in-cloud.io/desired-revision-set-at` | RFC3339 | When desired was set |

### Written by Agent

| Annotation | Format | Description |
|------------|--------|-------------|
| `mco.in-cloud.io/current-revision` | `rendered-<pool>-<hash>` | Current revision |
| `mco.in-cloud.io/agent-state` | enum | "idle", "applying", "done", "error" |
| `mco.in-cloud.io/last-error` | string | Last error message |
| `mco.in-cloud.io/reboot-pending` | "true"/"false" | Reboot required |

### User-controlled

| Annotation | Format | Description |
|------------|--------|-------------|
| `mco.in-cloud.io/paused` | "true" | Exclude node from rollout |
| `mco.in-cloud.io/force-reboot` | "true" | Force reboot ignoring minInterval |

---

## Prometheus Metrics

### Gauges

| Metric | Labels | Description |
|--------|--------|-------------|
| `mco_cordoned_nodes` | pool | Cordoned nodes per pool |
| `mco_draining_nodes` | pool | Draining nodes per pool |
| `mco_pool_overlap_nodes_total` | pool | Overlap nodes per pool |
| `mco_pool_overlap_conflicts_total` | — | Total overlap conflicts |

### Counters

| Metric | Labels | Description |
|--------|--------|-------------|
| `mco_pool_reconcile_total` | pool, result | Reconciliations count |
| `mco_drain_stuck_total` | pool | Drain timeout events |

### Histograms

| Metric | Labels | Buckets | Description |
|--------|--------|---------|-------------|
| `mco_pool_reconcile_duration_seconds` | pool | 0.01-5.12s | Reconcile duration |
| `mco_drain_duration_seconds` | pool, node | 10s-2.8h | Drain duration |

---

## Kubernetes Events

### Pool Events

| Reason | Type | Description |
|--------|------|-------------|
| `RollingUpdate` | Normal | Rolling update started |
| `RolloutComplete` | Normal | All nodes updated |
| `PoolOverlap` | Warning | Overlap detected |
| `DrainStuck` | Warning | Drain timeout |

### Node Events

| Reason | Type | Description |
|--------|------|-------------|
| `NodeCordon` | Warning | Node cordoned for update (destructive) |
| `NodeDrain` | Warning | Drain started (destructive) |
| `DrainFailed` | Warning | Drain attempt failed, will retry |
| `NodeUncordoned` | Normal | Node returned to service |
| `ApplyStarted` | Normal | Config apply started |
| `ApplyComplete` | Normal | Config applied successfully |
| `ApplyFailed` | Warning | Config apply failed |
| `ApplyTimeout` | Warning | Apply exceeded timeout |

