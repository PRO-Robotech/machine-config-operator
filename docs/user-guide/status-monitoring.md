# Мониторинг статуса

Отслеживание состояния пулов и нод в MCO Lite.

---

## Обзор статусов

### Иерархия статусов

```
MachineConfigPool (status)
├── conditions: Ready, Updating, Draining, Degraded, PoolOverlap, DrainStuck
├── counters: machineCount, readyMachineCount, cordonedMachineCount, ...
└── revisions: targetRevision, currentRevision, lastSuccessfulRevision
    │
    └── Nodes (annotations)
        ├── mco.in-cloud.io/agent-state
        ├── mco.in-cloud.io/current-revision
        ├── mco.in-cloud.io/desired-revision
        ├── mco.in-cloud.io/cordoned
        ├── mco.in-cloud.io/drain-started-at
        └── mco.in-cloud.io/reboot-pending
```

---

## Статус пула

### Основные поля

```bash
kubectl get mcp worker -o yaml
```

```yaml
status:
  # Ревизии
  targetRevision: rendered-worker-a1b2c3d4e5      # Куда сходимся
  currentRevision: rendered-worker-a1b2c3d4e5     # Где сейчас
  lastSuccessfulRevision: rendered-worker-a1b2c3d4e5  # Последний успех

  # Счётчики
  machineCount: 5           # Всего нод
  readyMachineCount: 5      # Готовы (current=target + state=done)
  updatedMachineCount: 5    # Обновлены (current=target)
  updatingMachineCount: 0   # Обновляются (state=applying)
  degradedMachineCount: 0   # С ошибкой (state=error)
  cordonedMachineCount: 0   # Cordoned для обновления
  drainingMachineCount: 0   # В процессе drain
  pendingRebootCount: 0     # Ждут перезагрузки
```

### Интерпретация счётчиков

| Сценарий | machineCount | ready | updated | updating | cordoned | degraded |
|----------|--------------|-------|---------|----------|----------|----------|
| Всё хорошо | 5 | 5 | 5 | 0 | 0 | 0 |
| Раскатка | 5 | 2 | 2 | 1 | 1 | 0 |
| Drain блокирован | 5 | 2 | 2 | 0 | 1 | 0 |
| Частичный сбой | 5 | 3 | 3 | 0 | 0 | 2 |
| Полный сбой | 5 | 0 | 0 | 0 | 0 | 5 |

---

## Условия пула (Conditions)

```bash
kubectl get mcp worker -o jsonpath='{.status.conditions}' | jq .
```

### Ready — главный индикатор здоровья

```yaml
- type: Ready
  status: "True"      # Всё хорошо
  reason: AllNodesUpdated
  message: "All 5 nodes are at target revision"
```

| status | reason | Значение |
|--------|--------|----------|
| True | AllNodesUpdated | Все ноды обновлены и нет ошибок |
| False | RolloutInProgress | Идёт обновление |
| False | Degraded | Есть ноды с ошибками |
| False | NoMachineConfigs | Нет конфигов в пуле |

### Updating

```yaml
- type: Updating
  status: "True"      # Идёт раскатка
  reason: RolloutInProgress
  message: "Updating 3 nodes"
```

| status | Значение |
|--------|----------|
| True | Есть ноды не на target revision |
| False | Все ноды на target revision |

### Draining

```yaml
- type: Draining
  status: "True"      # Drain в процессе
  reason: NodesDraining
  message: "2 nodes are currently draining"
```

| status | Значение |
|--------|----------|
| True | На нодах выполняется drain |
| False | Нет активного drain |

### Degraded

```yaml
- type: Degraded
  status: "True"      # Есть проблемы
  reason: NodeErrors
  message: "2 nodes in error state"
```

| status | reason | Значение |
|--------|--------|----------|
| True | NodeErrors | Ноды в состоянии error |
| True | RenderFailed | Ошибка создания RenderedMachineConfig |
| False | NoErrors | Нет ошибок |

### PoolOverlap

```yaml
- type: PoolOverlap
  status: "True"      # Нода в нескольких пулах
  reason: NodeMatchesMultiplePools
  message: "Node node-1 matches pools: worker, infra"
```

| status | Значение |
|--------|----------|
| True | Обнаружена нода, матчащая несколько пулов |
| False | Нет overlap |

### DrainStuck

```yaml
- type: DrainStuck
  status: "True"      # Drain timeout
  reason: DrainTimeout
  message: "Node node-1 drain exceeded 3600s timeout"
```

| status | Значение |
|--------|----------|
| True | Drain занимает больше drainTimeoutSeconds |
| False | Drain в норме |

---

## Статус ноды

### Аннотации

```bash
kubectl get node NODE -o jsonpath='{.metadata.annotations}' | jq 'with_entries(select(.key | startswith("mco")))'
```

```json
{
  "mco.in-cloud.io/pool": "worker",
  "mco.in-cloud.io/desired-revision": "rendered-worker-a1b2c3d4e5",
  "mco.in-cloud.io/current-revision": "rendered-worker-a1b2c3d4e5",
  "mco.in-cloud.io/agent-state": "done",
  "mco.in-cloud.io/cordoned": "true",
  "mco.in-cloud.io/drain-started-at": "2026-01-09T10:00:00Z",
  "mco.in-cloud.io/drain-retry-count": "3",
  "mco.in-cloud.io/reboot-pending": "false"
}
```

### agent-state

| Состояние | Описание | Следующее |
|-----------|----------|-----------|
| `idle` | Agent ожидает команд | → `applying` при новой revision |
| `applying` | Применяет конфигурацию | → `done` или `error` |
| `done` | Конфигурация применена | → `idle` после stabilization |
| `error` | Ошибка применения | (ручное вмешательство) |

### Переходы состояний

```
                  ┌───────────┐
                  │   idle    │◄──────────────┐
                  └─────┬─────┘               │
                        │ desired ≠ current   │
                        ▼                     │
                  ┌───────────┐               │
         ┌───────▶│ applying  │───────┐       │
         │        └───────────┘       │       │
         │              │             │       │
         │    success   │   failure   │       │
         │              ▼             ▼       │
         │        ┌───────────┐ ┌───────────┐ │
         │        │   done    │ │   error   │ │
         │        └─────┬─────┘ └───────────┘ │
         │              │                     │
         │              └─────────────────────┘
         │                    next change
         │
         └─────── desired changes again
```

---

## Мониторинг в реальном времени

### Watch пулов

```bash
# Следить за статусом пулов
kubectl get mcp -w

# С таймштампом
watch -n 2 'kubectl get mcp'
```

### Watch нод

```bash
# Следить за аннотациями
watch -n 2 'kubectl get nodes -o custom-columns="\
NAME:.metadata.name,\
STATE:.metadata.annotations.mco\.in-cloud\.io/agent-state,\
CORDONED:.metadata.annotations.mco\.in-cloud\.io/cordoned,\
REV:.metadata.annotations.mco\.in-cloud\.io/current-revision"'
```

### Логи Controller

```bash
# Следить за логами
kubectl logs -n mco-system deployment/mco-controller -f

# Фильтр по уровню
kubectl logs -n mco-system deployment/mco-controller -f | grep -E "(INFO|WARN|ERROR)"
```

### Логи Agent

```bash
# Все агенты
kubectl logs -n mco-system -l app=mco-agent -f

# Конкретный агент
kubectl logs -n mco-system -l app=mco-agent --field-selector spec.nodeName=node-1 -f
```

---

## Dashboard команды

### Общая картина

```bash
echo "=== Pools ===" && kubectl get mcp && \
echo -e "\n=== MachineConfigs ===" && kubectl get mc && \
echo -e "\n=== RenderedMachineConfigs ===" && kubectl get rmc && \
echo -e "\n=== Nodes ===" && kubectl get nodes -o custom-columns='\
NAME:.metadata.name,\
STATE:.metadata.annotations.mco\.in-cloud\.io/agent-state,\
CORDONED:.metadata.annotations.mco\.in-cloud\.io/cordoned,\
REVISION:.metadata.annotations.mco\.in-cloud\.io/current-revision'
```

### Проблемные ноды

```bash
echo "=== Degraded Nodes ===" && \
kubectl get nodes -o json | jq -r '
  .items[] |
  select(.metadata.annotations["mco.in-cloud.io/agent-state"] == "error") |
  [.metadata.name, .metadata.annotations["mco.in-cloud.io/last-error"]] |
  @tsv'

echo -e "\n=== Cordoned Nodes ===" && \
kubectl get nodes -o json | jq -r '
  .items[] |
  select(.metadata.annotations["mco.in-cloud.io/cordoned"] == "true") |
  .metadata.name'

echo -e "\n=== Pending Reboot ===" && \
kubectl get nodes -o json | jq -r '
  .items[] |
  select(.metadata.annotations["mco.in-cloud.io/reboot-pending"] == "true") |
  .metadata.name'
```

---

## Prometheus Metrics

### Gauge метрики

| Метрика | Labels | Описание |
|---------|--------|----------|
| `mco_cordoned_nodes` | pool | Cordoned ноды по пулам |
| `mco_draining_nodes` | pool | Ноды в процессе drain |
| `mco_pool_overlap_nodes_total` | pool | Ноды в overlap конфликте |
| `mco_pool_overlap_conflicts_total` | — | Всего конфликтующих нод |

### Counter метрики

| Метрика | Labels | Описание |
|---------|--------|----------|
| `mco_pool_reconcile_total` | pool, result | Количество reconcile |
| `mco_drain_stuck_total` | pool | Количество drain timeout |

### Histogram метрики

| Метрика | Labels | Описание |
|---------|--------|----------|
| `mco_pool_reconcile_duration_seconds` | pool | Время reconcile |
| `mco_drain_duration_seconds` | pool, node | Время drain |

---

## Alerting (рекомендации)

### Degraded ноды

```yaml
- alert: MCONodeDegraded
  expr: increase(mco_pool_reconcile_total{result="error"}[5m]) > 0
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "MCO node is degraded"
```

### Drain Stuck

```yaml
- alert: MCODrainStuck
  expr: mco_drain_stuck_total > 0
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "MCO drain stuck on pool {{ $labels.pool }}"
```

### Pool Overlap

```yaml
- alert: MCOPoolOverlap
  expr: mco_pool_overlap_conflicts_total > 0
  for: 1m
  labels:
    severity: critical
  annotations:
    summary: "Nodes in multiple pools detected"
```

### Долгая раскатка

```yaml
- alert: MCORolloutSlow
  expr: mco_draining_nodes > 0
  for: 30m
  labels:
    severity: warning
  annotations:
    summary: "MCO rollout taking too long"
```

---

## Связанные документы

- [Rolling Update](rolling-update.md) — управление раскаткой
- [Cordon/Drain](cordon-drain.md) — безопасное обновление
- [Устранение проблем](troubleshooting.md) — диагностика ошибок

