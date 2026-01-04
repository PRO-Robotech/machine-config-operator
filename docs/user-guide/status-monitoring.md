# Мониторинг статуса

Отслеживание состояния пулов и нод в MCO Lite.

---

## Обзор статусов

### Иерархия статусов

```
MachineConfigPool (status)
├── conditions: Updated, Updating, Degraded, RenderDegraded
├── counters: machineCount, readyMachineCount, ...
└── revisions: targetRevision, currentRevision
    │
    └── Nodes (annotations)
        ├── mco.in-cloud.io/agent-state
        ├── mco.in-cloud.io/current-revision
        ├── mco.in-cloud.io/desired-revision
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
  unavailableMachineCount: 0  # Недоступны
  pendingRebootCount: 0     # Ждут перезагрузки
```

### Интерпретация счётчиков

| Сценарий | machineCount | ready | updated | updating | degraded |
|----------|--------------|-------|---------|----------|----------|
| Всё хорошо | 5 | 5 | 5 | 0 | 0 |
| Раскатка | 5 | 2 | 2 | 3 | 0 |
| Частичный сбой | 5 | 3 | 3 | 0 | 2 |
| Полный сбой | 5 | 0 | 0 | 0 | 5 |

---

## Условия пула (Conditions)

```bash
kubectl get mcp worker -o jsonpath='{.status.conditions}' | jq .
```

### Updated

```yaml
- type: Updated
  status: "True"      # Все ноды на target revision
  reason: AllNodesUpdated
  message: "All 5 nodes are updated"
```

| status | Значение |
|--------|----------|
| True | Все ноды имеют target revision |
| False | Есть ноды с другой revision |

### Updating

```yaml
- type: Updating
  status: "True"      # Идёт раскатка
  reason: NodesUpdating
  message: "3 of 5 nodes are applying configuration"
```

| status | Значение |
|--------|----------|
| True | Хотя бы одна нода в состоянии `applying` |
| False | Нет нод в состоянии `applying` |

### Degraded

```yaml
- type: Degraded
  status: "True"      # Есть проблемы
  reason: NodesDegraded
  message: "2 nodes are in error state"
```

| status | Значение |
|--------|----------|
| True | Хотя бы одна нода в состоянии `error` |
| False | Нет нод в состоянии `error` |

### RenderDegraded

```yaml
- type: RenderDegraded
  status: "True"      # Ошибка рендеринга
  reason: RenderFailed
  message: "Failed to render: invalid file path"
```

| status | Значение |
|--------|----------|
| True | Ошибка при создании RenderedMachineConfig |
| False | Рендеринг работает нормально |

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
  "mco.in-cloud.io/last-error": "",
  "mco.in-cloud.io/reboot-pending": "false"
}
```

### agent-state

| Состояние | Описание | Следующее |
|-----------|----------|-----------|
| `idle` | Agent ожидает команд | — |
| `applying` | Применяет конфигурацию | done или error |
| `done` | Конфигурация применена | idle |
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
watch -n 2 'kubectl get nodes -o custom-columns="NAME:.metadata.name,STATE:.metadata.annotations.mco\.in-cloud\.io/agent-state,REV:.metadata.annotations.mco\.in-cloud\.io/current-revision"'
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
echo -e "\n=== Nodes ===" && kubectl get nodes -o custom-columns='NAME:.metadata.name,STATE:.metadata.annotations.mco\.in-cloud\.io/agent-state,REVISION:.metadata.annotations.mco\.in-cloud\.io/current-revision'
```

### Проблемные ноды

```bash
echo "=== Degraded Nodes ===" && \
kubectl get nodes -o json | jq -r '
  .items[] |
  select(.metadata.annotations["mco.in-cloud.io/agent-state"] == "error") |
  [.metadata.name, .metadata.annotations["mco.in-cloud.io/last-error"]] |
  @tsv'

echo -e "\n=== Pending Reboot ===" && \
kubectl get nodes -o json | jq -r '
  .items[] |
  select(.metadata.annotations["mco.in-cloud.io/reboot-pending"] == "true") |
  .metadata.name'
```

---

## Метрики (будущее)

> **Примечание:** Экспорт метрик планируется в будущих версиях.

Планируемые метрики:

```
# Gauge
mco_pool_machine_count{pool="worker"} 5
mco_pool_ready_machine_count{pool="worker"} 5
mco_pool_degraded_machine_count{pool="worker"} 0

# Counter
mco_renders_total{pool="worker"} 15
mco_apply_success_total{node="node-1"} 10
mco_apply_failure_total{node="node-1"} 2

# Histogram
mco_apply_duration_seconds_bucket{le="10"} 5
```

---

## Алерты (рекомендации)

### Degraded ноды

```yaml
# Пример PrometheusRule (будущее)
- alert: MCONodeDegraded
  expr: mco_pool_degraded_machine_count > 0
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "MCO node is degraded"
```

### Долгая раскатка

```yaml
- alert: MCORolloutStuck
  expr: mco_pool_updating_machine_count > 0
  for: 30m
  labels:
    severity: warning
```

---

## Связанные документы

- [Проверка применения](verification.md) — детальная проверка
- [Устранение проблем](troubleshooting.md) — диагностика ошибок
