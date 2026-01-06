# Терминология

Глоссарий ключевых терминов и понятий MCO Lite.

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

**Группа** нод и MachineConfig. Определяет какие конфиги применяются к каким нодам.

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

## Состояния ноды

### agent-state

| Состояние | Описание |
|-----------|----------|
| `idle` | Agent ожидает, конфигурация актуальна |
| `applying` | Agent применяет новую конфигурацию |
| `done` | Применение успешно завершено |
| `error` | Произошла ошибка при применении |

### current-revision vs desired-revision

| Аннотация | Кто пишет | Значение |
|-----------|-----------|----------|
| `desired-revision` | Controller | Какую ревизию нода **должна** иметь |
| `current-revision` | Agent | Какую ревизию нода **имеет** сейчас |

**Нода синхронизирована** когда: `current-revision == desired-revision`

---

## Состояния пула

### Conditions (Условия)

| Condition | True когда |
|-----------|------------|
| `Updated` | Все ноды имеют target revision |
| `Updating` | Хотя бы одна нода применяет конфиг |
| `Degraded` | Хотя бы одна нода в состоянии error |
| `RenderDegraded` | Ошибка при создании RMC |

### Счётчики нод

| Поле | Описание |
|------|----------|
| `machineCount` | Всего нод в пуле |
| `readyMachineCount` | current == target И state == done |
| `updatedMachineCount` | current == target |
| `updatingMachineCount` | state == applying |
| `degradedMachineCount` | state == error |
| `pendingRebootCount` | reboot-pending == true |

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

## Аннотации ноды

### Полный список

| Аннотация | Формат | Кто пишет |
|-----------|--------|-----------|
| `mco.in-cloud.io/desired-revision` | `rendered-<pool>-<hash>` | Controller |
| `mco.in-cloud.io/current-revision` | `rendered-<pool>-<hash>` | Agent |
| `mco.in-cloud.io/pool` | `worker`, `master` | Controller |
| `mco.in-cloud.io/agent-state` | `idle`, `applying`, `done`, `error` | Agent |
| `mco.in-cloud.io/last-error` | текст ошибки | Agent |
| `mco.in-cloud.io/reboot-pending` | `true`, `false` | Agent |

---

## Связанные документы

- [Архитектура](architecture.md) — как компоненты взаимодействуют
- [MachineConfig](../user-guide/machineconfig.md) — полное руководство
- [Мониторинг статуса](../user-guide/status-monitoring.md) — отслеживание состояний
