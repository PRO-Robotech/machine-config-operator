# Архитектура MCO Lite

MCO Lite состоит из двух основных компонентов: **Controller** и **Agent**.

---

## Общая схема

```
                          ┌─────────────────────────────────────┐
                          │          Kubernetes API             │
                          │  ┌─────┐ ┌─────┐ ┌─────┐ ┌──────┐   │
                          │  │ MC  │ │ MCP │ │ RMC │ │ Node │   │
                          │  └──┬──┘ └──┬──┘ └──┬──┘ └──┬───┘   │
                          └────┼───────┼───────┼───────┼────────┘
                               │       │       │       │
         ┌─────────────────────┼───────┼───────┼───────┼──────────────┐
         │                     ▼       ▼       ▼       │              │
         │  CONTROLLER   ┌──────────────────────┐      │              │
         │  (Deployment) │      Renderer        │      │              │
         │               │  MC[] → RMC + hash   │      │              │
         │               └──────────┬───────────┘      │              │
         │                          │                  │              │
         │               ┌──────────▼───────────┐      │              │
         │               │   Rollout Manager    │      │              │
         │               │  debounce + desired  │──────┘              │
         │               └──────────┬───────────┘   writes            │
         │                          │            annotations          │
         │               ┌──────────▼───────────┐                     │
         │               │  Status Aggregator   │◄─────┐              │
         │               │  pool status + conds │      │              │
         │               └──────────────────────┘      │ reads        │
         └─────────────────────────────────────────────┼──────────────┘
                                                       │
    ┌──────────────────────────────────────────────────┼──────────────┐
    │  AGENT (DaemonSet, per node)                     │              │
    │                                                  │              │
    │  ┌──────────────┐    ┌──────────────┐    ┌───────┴──────┐       │
    │  │  Node Watch  │───▶│   Applier    │───▶│State Reporter│       │
    │  │  (own node)  │    │ files+systemd│    │  annotations │       │
    │  └──────────────┘    └──────────────┘    └──────────────┘       │
    │                                                                 │
    └─────────────────────────────────────────────────────────────────┘
```

---

## Controller

Controller запускается как **Deployment** (обычно 1 реплика) и отвечает за:

### Renderer

**Задача:** Создать единую конфигурацию из множества MachineConfig.

```
MachineConfig (priority: 30)  ─┐
MachineConfig (priority: 50)  ─┼──▶ RenderedMachineConfig
MachineConfig (priority: 70)  ─┘    (merged + hash)
```

**Алгоритм:**
1. Выбрать MachineConfig по `machineConfigSelector` пула
2. Отсортировать по приоритету (меньший → больший)
3. Слить файлы и systemd-юниты (последний побеждает)
4. Вычислить SHA256 хеш результата
5. Создать RenderedMachineConfig с именем `rendered-<pool>-<hash[:10]>`

### Rollout Manager

**Задача:** Управлять раскаткой конфигурации на ноды.

- Применяет **debounce** — ждёт N секунд после изменения перед рендером
- Записывает `desired-revision` в аннотации нод
- Контролирует скорость раскатки

### Status Aggregator

**Задача:** Агрегировать состояние нод в статус пула.

Читает аннотации нод и вычисляет:
- `machineCount` — всего нод
- `readyMachineCount` — ноды с applied конфигом
- `degradedMachineCount` — ноды с ошибками
- `updatingMachineCount` — ноды в процессе применения

---

## Agent

Agent запускается как **DaemonSet** на каждой ноде и отвечает за:

> **Важно:** Agent — это node-scoped демон, НЕ Kubernetes-контроллер.
> Он видит только свою ноду и не знает о других.

### Node Watch

**Задача:** Следить за аннотациями своей ноды.

```go
// Agent смотрит только на свою ноду
fieldSelector: metadata.name=<node-name>
```

Когда `desired-revision` меняется — запускает Applier.

### Applier

**Задача:** Применить конфигурацию на хост.

**Для файлов:**
1. Создать директорию (если не существует)
2. Записать файл атомарно (write to temp → rename)
3. Установить права и владельца
4. Для `state: absent` — удалить файл

**Для systemd:**
1. `systemctl enable/disable <unit>`
2. `systemctl start/stop/restart/reload <unit>`
3. Для `mask: true` — `systemctl mask <unit>`

### State Reporter

**Задача:** Сообщать о состоянии через аннотации.

| Аннотация | Описание |
|-----------|----------|
| `current-revision` | Последняя успешно применённая ревизия |
| `agent-state` | Текущее состояние: `idle`, `applying`, `done`, `error` |
| `last-error` | Текст ошибки (если `state=error`) |
| `reboot-pending` | `true` если требуется перезагрузка |

---

## Контракт аннотаций

**Принцип:** Controller пишет DESIRED, Agent пишет OBSERVED.

### Пишет Controller

| Аннотация | Пример значения |
|-----------|-----------------|
| `mco.in-cloud.io/desired-revision` | `rendered-worker-a1b2c3d4e5` |
| `mco.in-cloud.io/pool` | `worker` |

### Пишет Agent

| Аннотация | Пример значения |
|-----------|-----------------|
| `mco.in-cloud.io/current-revision` | `rendered-worker-a1b2c3d4e5` |
| `mco.in-cloud.io/agent-state` | `done` |
| `mco.in-cloud.io/last-error` | `failed to write /etc/foo` |
| `mco.in-cloud.io/reboot-pending` | `true` |

---

## Жизненный цикл конфигурации

```
1. DevOps создаёт MachineConfig
         │
         ▼
2. Controller видит изменение
         │
         ▼
3. [Debounce] Ждёт N секунд
         │
         ▼
4. Renderer создаёт RenderedMachineConfig
         │
         ▼
5. Rollout Manager записывает desired-revision на ноды
         │
         ▼
6. Agent видит изменение аннотации
         │
         ▼
7. Applier применяет файлы и systemd
         │
         ▼
8. State Reporter записывает current-revision = desired
         │
         ▼
9. Status Aggregator обновляет статус пула
```

---

## Требования к ресурсам

| Компонент | CPU | Memory | Примечание |
|-----------|-----|--------|------------|
| Controller | 100m | 128Mi | На весь кластер |
| Agent | 50m | 64Mi | На каждую ноду |

---

## Следующие шаги

- [Терминология](terminology.md) — ключевые понятия
- [Установка](../getting-started/installation.md) — развернуть MCO Lite
