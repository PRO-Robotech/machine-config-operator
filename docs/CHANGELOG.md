# История изменений

Все значимые изменения проекта MCO Lite документируются в этом файле.

Формат основан на [Keep a Changelog](https://keepachangelog.com/ru/1.0.0/),
версионирование следует [Semantic Versioning](https://semver.org/lang/ru/).

---

## [0.1.2] - 2026-01-09

### Summary

Релиз стабилизации, исправляющий баги, выявленные при E2E тестировании, и улучшающий документацию.

### Исправлено

- **Drain Timeout Grace Period** — исправлена ошибка, при которой hardcoded 10-минутная проверка игнорировала `drainTimeoutSeconds` < 600s
  - Timeout теперь проверяется корректно согласно конфигурации
  - Retry intervals пропорциональны configured timeout
  - Добавлен minimum requeue floor (30s) для предотвращения busy-looping

- **Paused Node Unavailable Count** — исправлена ошибка, при которой паузированные ноды неправильно считались unavailable
  - Pause check теперь выполняется перед cordon/drain проверками
  - Паузированные ноды больше не потребляют `maxUnavailable` слоты

- **Duplicate Events in Retry Loop** — события `DrainStuck` и `RolloutComplete` больше не дублируются при retry

- **Hash Collision Dead Code** — исправлена обработка hash collision при создании RMC
  - При коллизии теперь создаётся RMC с suffix вместо RenderDegraded

- **Controller Tolerations** — контроллер теперь может запускаться на control-plane нодах
  - Добавлены tolerations для `node-role.kubernetes.io/control-plane` и `master`
  - Используется `priorityClassName: system-cluster-critical`
  - Решает проблему deadlock когда все worker ноды cordoned

- **RBAC for Events** — добавлены права на создание Kubernetes Events

- **E2E Test Cleanup** — улучшена очистка ресурсов между тестами
  - Добавлен `cleanupAllMCOResources()` helper
  - Ноды uncordon после каждого теста
  - Удаляются MCO аннотации

### Добавлено

- Конфигурируемый `drainRetrySeconds` для интервала retry drain
- `lastSuccessfulRevision` в статусе пула
- `desiredRevisionSetAt` аннотация для timeout detection

### Документация

- Полностью переработана пользовательская документация
- Добавлен глоссарий терминов
- Добавлена документация E2E тестов
- Добавлены диаграммы взаимодействий

---

## [0.1.1] - 2026-01-07

### Summary

Этот релиз добавляет Rolling Update с контролем maxUnavailable, оркестрацию Cordon/Drain нод, валидацию Pool Overlap и расширенную observability через новые conditions и metrics.

### Добавлено

#### Rolling Update

- **maxUnavailable** — контроль количества нод, которые могут быть unavailable во время обновления
  - Поддержка абсолютных значений (1, 2, 3) и процентов ("25%", "50%")
  - По умолчанию: 1 (последовательное обновление)
  - Использует ceiling для вычисления процентов

#### Cordon/Drain/Uncordon

- **Node Cordoning** — ноды помечаются unschedulable перед обновлением
- **Pod Eviction** — graceful eviction подов с учётом PDB
- **Drain Retry** — автоматический retry с exponential backoff
- **Uncordon on Complete** — автоматический uncordon после успешного обновления

#### Pool Overlap Validation

- **Overlap Detection** — обнаружение нод, матчащих несколько пулов
- **PoolOverlap Condition** — condition для статуса overlap
- **Conflict Blocking** — блокировка обновлений для конфликтующих нод

#### Status & Conditions

- **cordonedMachineCount** — количество cordoned нод в пуле
- **drainingMachineCount** — количество нод в процессе drain
- **DrainStuck Condition** — индикация превышения timeout drain
- **Degraded Condition** — мета-condition для здоровья пула

#### Metrics

- `mco_drain_duration_seconds` — гистограмма времени drain
- `mco_drain_stuck_total` — счётчик событий drain timeout
- `mco_cordoned_nodes` — gauge cordoned нод по пулам
- `mco_draining_nodes` — gauge draining нод по пулам
- `mco_pool_overlap_nodes_total` — gauge нод в overlap конфликте

### Изменено

- **RolloutConfig** — расширен полями maxUnavailable, drainTimeoutSeconds
- **Status Aggregation** — добавлен подсчёт cordon/drain
- **Condition Management** — добавлены DrainStuck и PoolOverlap conditions

### Breaking Changes

- Нет. v0.1.1 полностью обратно совместим с v0.1.0.

---

## [0.1.0] - 2026-01-04

### Добавлено

- Первоначальная реализация MCO Lite
- CRD: MachineConfig, MachineConfigPool, RenderedMachineConfig
- Controller с компонентами:
  - Renderer — слияние MachineConfig в RenderedMachineConfig
  - Rollout Manager — управление раскаткой конфигураций
  - Status Aggregator — агрегация статуса нод
- Agent с компонентами:
  - Node Watch — отслеживание аннотаций ноды
  - Applier — применение файлов и systemd
  - State Reporter — отчёт о состоянии
- Поддержка стратегий перезагрузки: Never, IfRequired
- Debounce для предотвращения частых ре-рендеров

### Известные ограничения

- Нет поддержки drop-in файлов для systemd
- Нет интеграции с внешними секретами
- Только Linux-ноды

---

## Migration Guide

### From v0.1.0 to v0.1.1

v0.1.1 полностью обратно совместим. Изменения в существующих конфигурациях не требуются.

#### Рекомендуемые обновления

1. **Включить контроль Rolling Update** (опционально):

```yaml
spec:
  rollout:
    maxUnavailable: 1      # Контроль параллелизма
    debounceSeconds: 30    # Existing
```

2. **Мониторинг новых conditions**:

```bash
# Проверка pool overlap
kubectl get mcp -o jsonpath='{.items[*].status.conditions[?(@.type=="PoolOverlap")]}'

# Проверка drain stuck
kubectl get mcp -o jsonpath='{.items[*].status.conditions[?(@.type=="DrainStuck")]}'
```

3. **Добавить Prometheus alerts** (рекомендуется):

```yaml
groups:
  - name: mco
    rules:
      - alert: MCODrainStuck
        expr: mco_drain_stuck_total > 0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "MCO drain stuck on pool {{ $labels.pool }}"

      - alert: MCOPoolOverlap
        expr: mco_pool_overlap_nodes_total > 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Nodes in multiple pools detected"
```

### From v0.1.1 to v0.1.2

v0.1.2 полностью обратно совместим. Исправления багов применяются автоматически.

#### Рекомендации

1. **Пересоздать controller deployment** для получения новых tolerations:

```bash
kubectl rollout restart deployment -n mco-system mco-controller
```

2. **Настроить drainRetrySeconds** при необходимости:

```yaml
spec:
  rollout:
    drainTimeoutSeconds: 3600
    drainRetrySeconds: 300    # Retry каждые 5 минут
```

---

## API Changes Summary

### v0.1.1 → v0.1.2

#### MachineConfigPoolSpec.Rollout

| Field | Change | Description |
|-------|--------|-------------|
| `drainRetrySeconds` | ADDED | Interval between drain retries |

#### MachineConfigPoolStatus

| Field | Change | Description |
|-------|--------|-------------|
| `lastSuccessfulRevision` | ADDED | Last successfully applied revision |

#### Node Annotations

| Annotation | Change | Description |
|------------|--------|-------------|
| `mco.in-cloud.io/desired-revision-set-at` | ADDED | Timestamp for timeout detection |

### v0.1.0 → v0.1.1

#### MachineConfigPoolSpec.Rollout

| Field | Change | Description |
|-------|--------|-------------|
| `maxUnavailable` | ADDED | IntOrString, default: 1 |
| `drainTimeoutSeconds` | ADDED | int, default: 3600 |

#### MachineConfigPoolStatus

| Field | Change | Description |
|-------|--------|-------------|
| `cordonedMachineCount` | ADDED | int32 |
| `drainingMachineCount` | ADDED | int32 |

#### Conditions

| Type | Change | Description |
|------|--------|-------------|
| `PoolOverlap` | ADDED | Overlap detection |
| `DrainStuck` | ADDED | Drain timeout |

#### Node Annotations

| Annotation | Change | Description |
|------------|--------|-------------|
| `mco.in-cloud.io/cordoned` | ADDED | Cordon status |
| `mco.in-cloud.io/drain-started-at` | ADDED | Drain start time |
| `mco.in-cloud.io/drain-retry-count` | ADDED | Retry count |

---

## Легенда

- **Добавлено** — новая функциональность
- **Изменено** — изменения в существующей функциональности
- **Устарело** — функции, которые будут удалены в будущих версиях
- **Удалено** — удалённые функции
- **Исправлено** — исправления ошибок
- **Безопасность** — исправления уязвимостей

