# Глоссарий терминов

Полный справочник терминов и понятий MCO Lite.

---

## Custom Resources (CR)

### MachineConfig (MC)

**Фрагмент** конфигурации хоста. Определяет что нужно применить на ноду.

```yaml
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: my-config
spec:
  priority: 50
  files: [...]
  systemd: [...]
  reboot:
    required: false
```

| Характеристика | Значение |
|----------------|----------|
| Scope | Cluster (без namespace) |
| Short name | `mc` |
| Создаёт | DevOps/SRE |

### MachineConfigPool (MCP)

**Группа** нод и MachineConfig. Определяет какие конфиги применяются к каким нодам, и управляет процессом раскатки.

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
    maxUnavailable: 1
    debounceSeconds: 30
```

| Характеристика | Значение |
|----------------|----------|
| Scope | Cluster |
| Short name | `mcp` |
| Создаёт | DevOps/SRE |

### RenderedMachineConfig (RMC)

**Результат слияния** всех MachineConfig для пула. Создаётся Controller автоматически.

```yaml
apiVersion: mco.in-cloud.io/v1alpha1
kind: RenderedMachineConfig
metadata:
  name: rendered-worker-a1b2c3d4e5
spec:
  poolName: worker
  revision: a1b2c3d4e5
  configHash: a1b2c3d4e5f6g7h8i9j0...
  config:
    files: [...]    # merged
    systemd: [...]  # merged
```

| Характеристика | Значение |
|----------------|----------|
| Scope | Cluster |
| Short name | `rmc` |
| Создаёт | Controller (автоматически) |
| Immutable | Да (не изменяется после создания) |

---

## Ключевые понятия

### Revision (Ревизия)

**Короткий идентификатор** RenderedMachineConfig — первые 10 символов хеша.

```
rendered-worker-a1b2c3d4e5
                ^^^^^^^^^^
                revision
```

Используется в аннотациях нод для отслеживания версии конфигурации.

### ConfigHash (Хеш конфигурации)

**SHA256-хеш** содержимого RenderedMachineConfig. Гарантирует что два RMC с одинаковым содержимым будут иметь одинаковый хеш.

### Priority (Приоритет)

**Число от 0 до 99999** определяющее порядок слияния MachineConfig.

| Приоритет | Применяется | При конфликте |
|-----------|-------------|---------------|
| Меньший (0) | Первым | Проигрывает |
| Больший (99999) | Последним | Побеждает |

```
Priority 30: files: [/etc/a.conf: "base"]
Priority 50: files: [/etc/a.conf: "override"]  ← победит
```

**Tie-breaker:** При равном приоритете побеждает MachineConfig с **большим именем** (алфавитно):
```
Priority 50: name: "50-alpha" → content: "alpha"
Priority 50: name: "50-beta"  → content: "beta"   ← победит ("beta" > "alpha")
```

### Debounce (Отложенный рендер)

**Задержка перед созданием нового RMC** после изменения MachineConfig.

Зачем нужно:
- Предотвращает множество ре-рендеров при пакетных изменениях
- Даёт время применить несколько MC за один раз

```yaml
spec:
  rollout:
    debounceSeconds: 30  # подождать 30 сек после изменения
```

---

## Rolling Update

### maxUnavailable

**Максимальное количество нод**, которые могут быть недоступны одновременно во время обновления.

| Значение | Тип | Пример | Результат для 10 нод |
|----------|-----|--------|---------------------|
| `1` | Абсолютное | maxUnavailable: 1 | 1 нода |
| `"25%"` | Процент | maxUnavailable: "25%" | 3 ноды (ceiling) |

### Cordon

**Пометка ноды как unschedulable.** Новые поды не будут размещаться на ноде.

Kubernetes устанавливает `node.spec.unschedulable = true`.
MCO Lite дополнительно записывает аннотацию `mco.in-cloud.io/cordoned = true`.

### Drain

**Эвакуация подов** с ноды перед обновлением.

Последовательность:
1. Получить список подов на ноде
2. Отфильтровать mirror pods, DaemonSet pods
3. Для каждого пода создать Eviction с учётом PDB
4. Ждать завершения всех эвакуаций

### Uncordon

**Возврат ноды в работу** после успешного обновления.

Controller снимает `node.spec.unschedulable` и удаляет аннотации:
- `mco.in-cloud.io/cordoned`
- `mco.in-cloud.io/drain-started-at`
- `mco.in-cloud.io/drain-retry-count`

### PodDisruptionBudget (PDB)

**Kubernetes-ресурс**, определяющий минимальное количество доступных подов во время disruption.

MCO Lite **уважает PDB** при drain. Если PDB не позволяет эвакуировать под, drain будет повторяться.

### DrainStuck

**Состояние**, когда drain занимает больше времени чем `drainTimeoutSeconds`.

MCO Lite НЕ отменяет drain, а устанавливает condition `DrainStuck` для alerting.

---

## Состояния ноды

### agent-state

| Состояние | Описание | Переход |
|-----------|----------|---------|
| `idle` | Agent ожидает, конфигурация актуальна | → `applying` при новой revision |
| `applying` | Agent применяет новую конфигурацию | → `done` или `error` |
| `done` | Применение успешно завершено | → `idle` после stabilization |
| `error` | Произошла ошибка при применении | → `applying` при retry |

### current-revision vs desired-revision

| Аннотация | Кто пишет | Значение |
|-----------|-----------|----------|
| `desired-revision` | Controller | Какую ревизию нода **должна** иметь |
| `current-revision` | Agent | Какую ревизию нода **имеет** сейчас |

**Нода синхронизирована** когда: `current-revision == desired-revision`

### reboot-pending

**Аннотация**, указывающая что нода требует перезагрузки.

Устанавливается Agent когда:
- MachineConfig имеет `reboot.required: true`
- Пул имеет стратегию `Never`

Перезагрузка должна быть выполнена вручную или через внешний механизм.

---

## Состояния пула

### Status Conditions

| Condition | True когда | Severity |
|-----------|------------|----------|
| `Updated` | Все ноды имеют target revision | Info |
| `Updating` | Хотя бы одна нода применяет конфиг | Info |
| `Degraded` | Хотя бы одна нода в состоянии error | Warning |
| `RenderDegraded` | Ошибка при создании RMC | Critical |
| `PoolOverlap` | Нода матчит несколько пулов | Critical |
| `DrainStuck` | Drain превысил timeout | Warning |

### Счётчики нод

| Поле | Описание |
|------|----------|
| `machineCount` | Всего нод в пуле |
| `readyMachineCount` | current == target AND state == done |
| `updatedMachineCount` | current == target |
| `updatingMachineCount` | state == applying |
| `degradedMachineCount` | state == error |
| `cordonedMachineCount` | mco.in-cloud.io/cordoned == true |
| `drainingMachineCount` | cordoned AND NOT done |
| `pendingRebootCount` | reboot-pending == true |

### Pool Overlap

**Конфликт**, когда нода матчит nodeSelector нескольких пулов.

MCO Lite:
1. Обнаруживает overlap при reconcile
2. Устанавливает condition `PoolOverlap` на затронутых пулах
3. **Блокирует обновления** для конфликтующих нод
4. Эмитит Kubernetes Event с описанием конфликта

---

## Стратегии перезагрузки

### Never

Нода **никогда** не перезагружается автоматически.

- Если конфигурация требует перезагрузку — ставится `reboot-pending: true`
- DevOps должен перезагрузить ноду вручную

### IfRequired

Нода **перезагружается автоматически** если конфигурация этого требует.

- Соблюдается `minIntervalSeconds` между перезагрузками
- Перезагрузка происходит после успешного применения конфигурации

---

## Node Annotations — полный список

### Пишет Controller

| Аннотация | Формат | Описание |
|-----------|--------|----------|
| `mco.in-cloud.io/desired-revision` | `rendered-<pool>-<hash>` | Целевая ревизия |
| `mco.in-cloud.io/pool` | `worker`, `master` | Имя пула |
| `mco.in-cloud.io/cordoned` | `true` | Нода cordoned MCO |
| `mco.in-cloud.io/drain-started-at` | RFC3339 timestamp | Время начала drain |
| `mco.in-cloud.io/drain-retry-count` | `0`, `1`, `2`, ... | Количество retry |
| `mco.in-cloud.io/desired-revision-set-at` | RFC3339 timestamp | Время установки desired |

### Пишет Agent

| Аннотация | Формат | Описание |
|-----------|--------|----------|
| `mco.in-cloud.io/current-revision` | `rendered-<pool>-<hash>` | Текущая ревизия |
| `mco.in-cloud.io/agent-state` | `idle`, `applying`, `done`, `error` | Состояние агента |
| `mco.in-cloud.io/last-error` | Текст ошибки | Последняя ошибка |
| `mco.in-cloud.io/reboot-pending` | `true`, `false` | Требуется перезагрузка |

### Паузирует Node (опционально)

| Аннотация | Формат | Описание |
|-----------|--------|----------|
| `mco.in-cloud.io/paused` | `true` | Нода исключена из rollout |
| `mco.in-cloud.io/force-reboot` | `true` | Форсировать перезагрузку |

---

## Prometheus Metrics

### Gauge метрики

| Метрика | Labels | Описание |
|---------|--------|----------|
| `mco_cordoned_nodes` | pool | Количество cordoned нод |
| `mco_draining_nodes` | pool | Количество нод в процессе drain |
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

## Связанные документы

- [Архитектура](architecture.md) — как компоненты взаимодействуют
- [MachineConfig](../user-guide/machineconfig.md) — полное руководство
- [Мониторинг статуса](../user-guide/status-monitoring.md) — отслеживание состояний
- [Rolling Update](../user-guide/rolling-update.md) — настройка раскатки

