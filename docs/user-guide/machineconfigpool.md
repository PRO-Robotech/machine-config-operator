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
    maxUnavailable: 1
    debounceSeconds: 30
    drainTimeoutSeconds: 3600
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

> **Важно:** Нода должна принадлежать **только одному** пулу.
> Если селекторы пересекаются — устанавливается condition `PoolOverlap`.

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

---

### spec.rollout

Настройки процесса раскатки конфигурации.

```yaml
spec:
  rollout:
    maxUnavailable: 1
    debounceSeconds: 30
    applyTimeoutSeconds: 600
    drainTimeoutSeconds: 3600
    drainRetrySeconds: 300
```

| Поле | Тип | По умолчанию | Диапазон | Описание |
|------|-----|--------------|----------|----------|
| `maxUnavailable` | IntOrString | 1 | 1+ или % | Макс. unavailable нод |
| `debounceSeconds` | int | 30 | 0-3600 | Задержка перед рендером |
| `applyTimeoutSeconds` | int | 600 | 60-3600 | Таймаут применения |
| `drainTimeoutSeconds` | int | 3600 | 60-86400 | Таймаут drain |
| `drainRetrySeconds` | int | auto | 10-1800 | Интервал retry drain |

#### maxUnavailable

Контролирует скорость раскатки:

```yaml
# Последовательное обновление (самое безопасное)
maxUnavailable: 1

# Параллельное обновление (быстрее)
maxUnavailable: 3

# Процент от общего числа нод
maxUnavailable: "25%"    # ceiling: 3 из 10
maxUnavailable: "50%"    # 5 из 10
```

#### debounceSeconds

Предотвращает множественные ре-рендеры:

```yaml
# Быстрое применение (для разработки)
debounceSeconds: 5

# Стандартное (для production)
debounceSeconds: 30

# Большая задержка (для пакетных обновлений)
debounceSeconds: 300
```

#### drainTimeoutSeconds

Таймаут для drain операции:

```yaml
# Быстрый drain (stateless apps)
drainTimeoutSeconds: 300    # 5 минут

# Стандартный
drainTimeoutSeconds: 3600   # 1 час

# Длительный (stateful apps)
drainTimeoutSeconds: 7200   # 2 часа
```

При превышении timeout устанавливается condition `DrainStuck`, но drain продолжает попытки.

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

---

### spec.revisionHistory

Управление хранением старых RenderedMachineConfig.

```yaml
spec:
  revisionHistory:
    limit: 5
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
- Существующие operations **не отменяются**

```bash
# Приостановить пул
kubectl patch mcp worker --type=merge -p '{"spec":{"paused":true}}'

# Возобновить пул
kubectl patch mcp worker --type=merge -p '{"spec":{"paused":false}}'
```

---

## Статус пула (status)

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
  cordonedMachineCount: 0
  drainingMachineCount: 0
  pendingRebootCount: 0
  conditions:
    - type: Ready
      status: "True"
    - type: Updating
      status: "False"
    - type: Draining
      status: "False"
    - type: Degraded
      status: "False"
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
| `cordonedMachineCount` | cordoned == true |
| `drainingMachineCount` | cordoned AND state != done |
| `pendingRebootCount` | reboot-pending == true |

### Условия (Conditions)

| Condition | True когда |
|-----------|------------|
| `Ready` | Все ноды имеют target revision и в состоянии done |
| `Updating` | Хотя бы одна нода применяет конфиг |
| `Draining` | Хотя бы одна нода в процессе drain |
| `Degraded` | Хотя бы одна нода в ошибке ИЛИ ошибка рендеринга (Reason=RenderFailed) |
| `PoolOverlap` | Нода матчит несколько пулов |
| `DrainStuck` | Drain превысил timeout |

---

## Типичные сценарии

### Worker pool (production)

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
    maxUnavailable: 1           # По одной ноде
    debounceSeconds: 30
    drainTimeoutSeconds: 3600
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
    maxUnavailable: 1
    debounceSeconds: 60
    drainTimeoutSeconds: 7200   # 2 часа — control plane критичен
  reboot:
    strategy: Never
  revisionHistory:
    limit: 10
```

### Staging pool (быстрое обновление)

```yaml
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfigPool
metadata:
  name: staging
spec:
  nodeSelector:
    matchLabels:
      environment: staging
  machineConfigSelector:
    matchLabels:
      mco.in-cloud.io/pool: staging
  rollout:
    maxUnavailable: "50%"       # Половина нод сразу
    debounceSeconds: 5
    drainTimeoutSeconds: 300
  reboot:
    strategy: IfRequired
    minIntervalSeconds: 300
```

---

## Полезные команды

```bash
# Список пулов
kubectl get mcp

# Детали пула
kubectl describe mcp worker

# Условия пула
kubectl get mcp worker -o jsonpath='{.status.conditions}' | jq .

# Счётчики нод
kubectl get mcp worker -o jsonpath='{.status}'

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
- [Rolling Update](rolling-update.md) — настройка раскатки
- [Cordon/Drain](cordon-drain.md) — безопасное обновление
- [Мониторинг статуса](status-monitoring.md) — отслеживание состояния

