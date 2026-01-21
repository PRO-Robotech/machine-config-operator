# История изменений

Все значимые изменения проекта MCO Lite документируются в этом файле.

Формат основан на [Keep a Changelog](https://keepachangelog.com/ru/1.0.0/),
версионирование следует [Semantic Versioning](https://semver.org/lang/ru/).

---

## [0.1.3] - 2026-01-21

### Summary

Релиз добавляет конфигурируемые исключения из drain через ConfigMap и исправляет критический баг с uncordon нод.

### Добавлено

- **Drain Exclusions via ConfigMap** — гибкое управление исключениями из drain
  - ConfigMap с label `mco.in-cloud.io/drain-config=true` для конфигурации
  - Правила исключения по namespace, namespacePrefix, podNamePattern, podSelector
  - Опция `defaults.skipToleratAllPods` для пропуска подов с tolerations: [{operator: Exists}]
  - OR-логика между правилами, AND-логика внутри правила
  - Sample ConfigMap: `config/samples/mco-drain-config.yaml`

### Исправлено

- **Нода остаётся cordoned при idle агенте** — исправлена логика uncordon
  - Uncordon теперь проверяет **только** совпадение revision (`current-revision == desired-revision`)
  - Agent state (`idle`, `applying`, `done`) больше не влияет на решение uncordon
  - Решает проблему deadlock когда нода оставалась cordoned навсегда при `agent-state=idle`

### Документация

- Добавлена документация по конфигурации Drain Exclusions
- Обновлены условия uncordon

---

## [0.1.2] - 2026-01-11

### Summary

Критический hotfix-релиз, исправляющий production-баги, выявленные при развёртывании v0.1.1 в реальном кластере. Включает breaking change в API условий (Conditions).

### ⚠️ Breaking Changes

- **Условия (Conditions) API** — изменена структура условий пула:
  - **УДАЛЕНО:** `Updated` → используйте `Ready` вместо
  - **УДАЛЕНО:** `RenderDegraded` → используйте `Degraded` с `Reason=RenderFailed`
  - **ДОБАВЛЕНО:** `Ready` — главный индикатор готовности (True когда все ноды обновлены и нет ошибок)
  - **ДОБАВЛЕНО:** `Draining` — True когда выполняется drain на нодах
  - **СОХРАНЕНО:** `Updating`, `Degraded`, `PoolOverlap`, `DrainStuck`
  - При обновлении с v0.1.1 устаревшие условия удаляются автоматически

### Исправлено

- **Rollout без MachineConfigs** — контроллер больше не начинает rollout при пустом пуле
  - MCP без MachineConfig не запускает cordon/drain нод
  - Решает проблему ложного обновления после создания пула

- **Self-eviction контроллера** — контроллер больше не эвиктит свой собственный под
  - Поды из namespace `machine-config-system` исключены из eviction
  - Решает проблему deadlock когда контроллер убивает сам себя

- **Нода остаётся cordoned после reboot** — исправлена логика uncordon после перезагрузки
  - Agent корректно устанавливает состояние `done` после startup
  - Нода uncordon в течение 30 секунд после перезагрузки

- **Stale status data** — статус пула теперь использует актуальные данные
  - Ноды re-fetch перед вычислением статуса
  - `updatedMachineCount` корректно отображает количество обновлённых нод

- **Новые ноды блокируются** — новые ноды в пуле больше не проходят через ненужный cordon/drain
  - Ноды без MCO аннотаций определяются как "новые"
  - `desired-revision` устанавливается напрямую без cordon/drain

- **Condition timestamp spam** — условия `DrainStuck` и `PoolOverlap` больше не вызывают постоянные обновления статуса
  - `LastTransitionTime` сохраняется если статус не изменился
  - Решает проблему "DDoS" MCP ресурса

- **Controller Tolerations** — контроллер может работать на cordoned нодах
  - Добавлены tolerations: `node.kubernetes.io/unschedulable`, `not-ready`, `unreachable`
  - Решает проблему недоступности контроллера при cordoned нодах

### Изменено

- **Типы событий** — деструктивные действия теперь Warning:
  - `NodeCordon` — Warning (нода становится unschedulable)
  - `NodeDrain` — Warning (поды эвакуируются)
  - Добавлено событие `DrainFailed` (Warning) при ошибках drain

### Добавлено

- Конфигурируемый `drainRetrySeconds` для интервала retry drain
- `lastSuccessfulRevision` в статусе пула
- `desiredRevisionSetAt` аннотация для timeout detection

### Документация

- Полностью переработана пользовательская документация
- Добавлен глоссарий терминов
- Добавлена документация E2E тестов

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

⚠️ **Breaking Change:** v0.1.2 изменяет API условий (Conditions).

#### Автоматическая миграция

При первом запуске контроллера v0.1.2:
- Устаревшие условия `Updated` и `RenderDegraded` автоматически удаляются
- Новые условия `Ready` и `Draining` создаются автоматически
- Логируется сообщение: "cleaned up legacy conditions from v0.1.x"

#### Обновление мониторинга

**Обязательно** обновите Prometheus alerts и dashboards:

```promql
# СТАРОЕ: Updated condition
kube_customresource_mco_machineconfigpool_status_condition{condition="Updated", status="true"}

# НОВОЕ: Ready condition
kube_customresource_mco_machineconfigpool_status_condition{condition="Ready", status="true"}
```

#### Рекомендации

1. **Обновить CRD и controller**:

```bash
make install
make deploy IMG=<your-registry>/mco-controller:v0.1.2
```

2. **Перезапустить controller** для получения новых tolerations:

```bash
kubectl rollout restart deployment -n machine-config-system machine-config-controller-manager
```

3. **Настроить drainRetrySeconds** при необходимости:

```yaml
spec:
  rollout:
    drainTimeoutSeconds: 3600
    drainRetrySeconds: 300    # Retry каждые 5 минут
```

4. **Добавить alert для нового условия Draining**:

```yaml
- alert: MCODrainingTooLong
  expr: |
    kube_customresource_mco_machineconfigpool_status_condition{condition="Draining", status="true"} == 1
  for: 30m
  labels:
    severity: warning
  annotations:
    summary: "MCO draining nodes for too long"
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

#### Conditions (⚠️ Breaking)

| Type | Change | Description |
|------|--------|-------------|
| `Ready` | ADDED | Primary health indicator (replaces Updated) |
| `Draining` | ADDED | True when drain in progress |
| `Updated` | REMOVED | Use `Ready` instead |
| `RenderDegraded` | REMOVED | Use `Degraded` with Reason=RenderFailed |

#### Node Annotations

| Annotation | Change | Description |
|------------|--------|-------------|
| `mco.in-cloud.io/desired-revision-set-at` | ADDED | Timestamp for timeout detection |

#### Kubernetes Events

| Event | Change | Description |
|-------|--------|-------------|
| `NodeCordon` | CHANGED | Now Warning type (was Normal) |
| `NodeDrain` | CHANGED | Now Warning type (was Normal) |
| `DrainFailed` | ADDED | Warning on drain failures |

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

