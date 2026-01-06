# MachineConfigPool — Руководство

MachineConfigPool (MCP) — это **группа нод и конфигураций**. Определяет какие MachineConfig применяются к каким нодам, и управляет процессом раскатки.

---

## Базовая структура

```yaml
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfigPool
metadata:
  name: worker
spec:
  nodeSelector:                        # Какие ноды входят в пул
    matchLabels:
      node-role.kubernetes.io/worker: ""
  machineConfigSelector:               # Какие MC применяются
    matchLabels:
      mco.in-cloud.io/pool: worker
  rollout:                             # Настройки раскатки
    debounceSeconds: 30
  reboot:                              # Политика перезагрузки
    strategy: Never
  revisionHistory:                     # Хранение старых ревизий
    limit: 5
  paused: false                        # Приостановка пула
```

---

## Полная спецификация

### spec.nodeSelector

Выбирает ноды, которые входят в пул.

```yaml
spec:
  nodeSelector:
    matchLabels:
      node-role.kubernetes.io/worker: ""
```

#### Использование matchLabels

```yaml
# Все ноды с лейблом worker
nodeSelector:
  matchLabels:
    node-role.kubernetes.io/worker: ""

# Ноды в конкретной зоне
nodeSelector:
  matchLabels:
    topology.kubernetes.io/zone: "us-east-1a"

# Несколько условий (AND)
nodeSelector:
  matchLabels:
    node-role.kubernetes.io/worker: ""
    environment: production
```

#### Использование matchExpressions

```yaml
nodeSelector:
  matchExpressions:
    - key: node-role.kubernetes.io/worker
      operator: Exists
    - key: environment
      operator: In
      values: ["production", "staging"]
```

> **Важно:** Нода может принадлежать только **одному** пулу.
> Если селекторы пересекаются — поведение не определено.

---

### spec.machineConfigSelector

Выбирает MachineConfig, которые применяются к пулу.

```yaml
spec:
  machineConfigSelector:
    matchLabels:
      mco.in-cloud.io/pool: worker
```

Все MachineConfig с этой меткой будут объединены в RenderedMachineConfig для данного пула.

```yaml
# Комбинированный селектор
machineConfigSelector:
  matchLabels:
    mco.in-cloud.io/pool: worker
  matchExpressions:
    - key: app.kubernetes.io/component
      operator: In
      values: ["base", "network"]
```

---

### spec.rollout

Настройки процесса раскатки конфигурации.

```yaml
spec:
  rollout:
    debounceSeconds: 30
    applyTimeoutSeconds: 600
```

| Поле | Тип | По умолчанию | Диапазон | Описание |
|------|-----|--------------|----------|----------|
| `debounceSeconds` | int | 30 | 0-3600 | Задержка перед рендером |
| `applyTimeoutSeconds` | int | 600 | 60-3600 | Таймаут применения |

#### debounceSeconds

**Зачем нужно:**
- Предотвращает множественные ре-рендеры при пакетных изменениях
- Позволяет применить несколько MC за один раз

```yaml
# Быстрое применение (для разработки)
debounceSeconds: 5

# Стандартное (для production)
debounceSeconds: 30

# Большая задержка (для пакетных обновлений)
debounceSeconds: 300
```

#### applyTimeoutSeconds

Если нода не применила конфиг за это время — она помечается как degraded.

```yaml
# Стандартный таймаут
applyTimeoutSeconds: 600    # 10 минут

# Для больших конфигов
applyTimeoutSeconds: 1200   # 20 минут
```

---

### spec.reboot

Политика перезагрузки нод.

```yaml
spec:
  reboot:
    strategy: Never
    minIntervalSeconds: 1800
```

| Поле | Тип | По умолчанию | Описание |
|------|-----|--------------|----------|
| `strategy` | enum | Never | Стратегия перезагрузки |
| `minIntervalSeconds` | int | 1800 | Мин. интервал между перезагрузками |

#### strategy

| Значение | Поведение |
|----------|-----------|
| `Never` | Ноды **никогда** не перезагружаются автоматически |
| `IfRequired` | Ноды перезагружаются если MC требует (`reboot.required: true`) |

```yaml
# Production: ручные перезагрузки
reboot:
  strategy: Never

# Dev/Test: автоматические перезагрузки
reboot:
  strategy: IfRequired
  minIntervalSeconds: 600    # Не чаще раза в 10 минут
```

#### minIntervalSeconds

Защита от "reboot storm" — минимальный интервал между перезагрузками одной ноды.

```yaml
# Консервативно (production)
minIntervalSeconds: 3600    # 1 час

# Умеренно
minIntervalSeconds: 1800    # 30 минут

# Агрессивно (dev)
minIntervalSeconds: 300     # 5 минут
```

---

### spec.revisionHistory

Управление хранением старых RenderedMachineConfig.

```yaml
spec:
  revisionHistory:
    limit: 5
```

| Поле | Тип | По умолчанию | Описание |
|------|-----|--------------|----------|
| `limit` | int | 5 | Макс. количество старых RMC |

```yaml
# Хранить историю
limit: 10

# Минимум (экономия ресурсов)
limit: 2

# Без ограничений (не рекомендуется)
limit: 0
```

> **Примечание:** Текущий RMC (target) не считается в limit.

---

### spec.paused

Приостанавливает все операции пула.

```yaml
spec:
  paused: true    # Пул остановлен
```

Когда `paused: true`:
- Новые RMC **не создаются**
- Ноды **не обновляются** (desired-revision не меняется)
- Статус пула **не обновляется**

**Используйте для:**
- Плановых работ
- Расследования проблем
- Ручного контроля раскатки

```bash
# Приостановить пул
kubectl patch mcp worker --type=merge -p '{"spec":{"paused":true}}'

# Возобновить пул
kubectl patch mcp worker --type=merge -p '{"spec":{"paused":false}}'
```

---

## Статус пула (status)

Статус автоматически обновляется Controller.

```yaml
status:
  targetRevision: rendered-worker-a1b2c3d4e5
  currentRevision: rendered-worker-a1b2c3d4e5
  lastSuccessfulRevision: rendered-worker-a1b2c3d4e5
  machineCount: 3
  readyMachineCount: 3
  updatedMachineCount: 3
  updatingMachineCount: 0
  degradedMachineCount: 0
  unavailableMachineCount: 0
  pendingRebootCount: 0
  conditions:
    - type: Updated
      status: "True"
```

### Ревизии

| Поле | Описание |
|------|----------|
| `targetRevision` | Целевая ревизия (куда ноды должны сходиться) |
| `currentRevision` | Текущая ревизия (most common среди нод) |
| `lastSuccessfulRevision` | Последняя успешно применённая ревизия |

### Счётчики нод

| Поле | Формула |
|------|---------|
| `machineCount` | Всего нод в пуле |
| `readyMachineCount` | current == target AND state == done |
| `updatedMachineCount` | current == target |
| `updatingMachineCount` | state == applying |
| `degradedMachineCount` | state == error |
| `pendingRebootCount` | reboot-pending == true |

### Условия (Conditions)

| Condition | True когда |
|-----------|------------|
| `Updated` | Все ноды имеют target revision |
| `Updating` | Хотя бы одна нода применяет конфиг |
| `Degraded` | Хотя бы одна нода в ошибке |
| `RenderDegraded` | Ошибка при создании RMC |

---

## Типичные сценарии

### Worker pool

```yaml
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfigPool
metadata:
  name: worker
spec:
  nodeSelector:
    matchLabels:
      node-role.kubernetes.io/worker: ""
  machineConfigSelector:
    matchLabels:
      mco.in-cloud.io/pool: worker
  rollout:
    debounceSeconds: 30
  reboot:
    strategy: Never
  revisionHistory:
    limit: 5
```

### Control plane pool

```yaml
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfigPool
metadata:
  name: master
spec:
  nodeSelector:
    matchLabels:
      node-role.kubernetes.io/control-plane: ""
  machineConfigSelector:
    matchLabels:
      mco.in-cloud.io/pool: master
  rollout:
    debounceSeconds: 60          # Больше задержка
    applyTimeoutSeconds: 900     # Больше таймаут
  reboot:
    strategy: IfRequired         # Авто-перезагрузка
    minIntervalSeconds: 3600     # Не чаще раза в час
  revisionHistory:
    limit: 10                    # Больше истории
```

### GPU pool

```yaml
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfigPool
metadata:
  name: gpu
spec:
  nodeSelector:
    matchLabels:
      node.kubernetes.io/gpu: "true"
  machineConfigSelector:
    matchLabels:
      mco.in-cloud.io/pool: gpu
  rollout:
    debounceSeconds: 60
  reboot:
    strategy: IfRequired    # Драйверы могут требовать перезагрузку
    minIntervalSeconds: 1800
```

---

## Полезные команды

```bash
# Список пулов
kubectl get mcp

# Детали пула
kubectl describe mcp worker

# Статус в формате wide
kubectl get mcp -o wide

# Условия пула
kubectl get mcp worker -o jsonpath='{.status.conditions}'

# Ноды в пуле
kubectl get nodes -l node-role.kubernetes.io/worker

# Приостановить пул
kubectl patch mcp worker --type=merge -p '{"spec":{"paused":true}}'

# Возобновить
kubectl patch mcp worker --type=merge -p '{"spec":{"paused":false}}'
```

---

## Связанные документы

- [MachineConfig](machineconfig.md) — создание конфигураций
- [Мониторинг статуса](status-monitoring.md) — отслеживание состояния
- [Примеры](../examples/README.md) — готовые примеры пулов
