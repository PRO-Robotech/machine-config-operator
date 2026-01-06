# История изменений

Все значимые изменения проекта MCO Lite документируются в этом файле.

Формат основан на [Keep a Changelog](https://keepachangelog.com/ru/1.0.0/),
версионирование следует [Semantic Versioning](https://semver.org/lang/ru/).

---

## [Unreleased]

### Добавлено
- Пользовательская документация на русском языке
- Примеры манифестов с комментариями

---

## [0.1.1] - 2025-01-07

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

#### Documentation
- API Reference для v0.1.1
- Troubleshooting Guide

### Изменено
- **RolloutConfig** — расширен полями maxUnavailable
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
    maxUnavailable: 1      # NEW: Контроль параллелизма
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

#### Действий не требуется для
- Существующих MachineConfig ресурсов
- Существующих MachineConfigPool ресурсов (применяются defaults)
- Node annotations (новые аннотации аддитивны)
- Конфигурации Agent DaemonSet

---

## API Changes

### MachineConfigPoolSpec

| Поле | Изменение | Описание |
|------|-----------|----------|
| `spec.rollout.maxUnavailable` | ADDED | IntOrString, default: 1 |

### MachineConfigPoolStatus

| Поле | Изменение | Описание |
|------|-----------|----------|
| `status.cordonedMachineCount` | ADDED | int32 |
| `status.drainingMachineCount` | ADDED | int32 |

### Conditions

| Type | Изменение | Описание |
|------|-----------|----------|
| `PoolOverlap` | ADDED | Обнаружение overlap |
| `DrainStuck` | ADDED | Drain timeout |

### Node Annotations

| Annotation | Изменение | Описание |
|------------|-----------|----------|
| `mco.in-cloud.io/cordoned` | ADDED | Статус cordon |
| `mco.in-cloud.io/drain-started-at` | ADDED | Время начала drain |
| `mco.in-cloud.io/drain-retry-count` | ADDED | Счётчик retry |

---

## Легенда

- **Добавлено** — новая функциональность
- **Изменено** — изменения в существующей функциональности
- **Устарело** — функции, которые будут удалены в будущих версиях
- **Удалено** — удалённые функции
- **Исправлено** — исправления ошибок
- **Безопасность** — исправления уязвимостей
