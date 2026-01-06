# Устранение проблем

Диагностика и решение типичных проблем MCO Lite.

---

## Быстрая диагностика

```bash
# 1. Статус пулов
kubectl get mcp

# 2. Проблемные ноды
kubectl get nodes -o json | jq -r '
  .items[] |
  select(.metadata.annotations["mco.in-cloud.io/agent-state"] == "error") |
  [.metadata.name, .metadata.annotations["mco.in-cloud.io/last-error"]] |
  @tsv'

# 3. Логи Controller
kubectl logs -n mco-system deployment/mco-controller --tail=50 | grep -i error

# 4. Логи Agent
kubectl logs -n mco-system -l app=mco-agent --tail=50 | grep -i error
```

---

## Проблемы с пулами

### MachineConfigPool не создаётся

**Симптом:** `kubectl apply` завершается с ошибкой.

```bash
# Проверить валидацию
kubectl apply -f pool.yaml --dry-run=server
```

**Частые причины:**

| Ошибка | Решение |
|--------|---------|
| `invalid nodeSelector` | Проверить синтаксис matchLabels |
| `debounceSeconds out of range` | Значение 0-3600 |
| `invalid reboot strategy` | Только `Never` или `IfRequired` |

### Пул показывает 0 нод

**Симптом:** `machineCount: 0`

```bash
# Проверить nodeSelector пула
kubectl get mcp worker -o jsonpath='{.spec.nodeSelector}'

# Проверить лейблы на нодах
kubectl get nodes --show-labels

# Найти ноды с нужным лейблом
kubectl get nodes -l node-role.kubernetes.io/worker
```

**Решение:**
```bash
# Добавить лейбл на ноду
kubectl label node NODE_NAME node-role.kubernetes.io/worker=""
```

### RenderDegraded: True

**Симптом:** Condition `RenderDegraded` = True

```bash
# Посмотреть причину
kubectl get mcp worker -o jsonpath='{.status.conditions[?(@.type=="RenderDegraded")].message}'
```

**Частые причины:**

| Сообщение | Проблема | Решение |
|-----------|----------|---------|
| `no MachineConfigs selected` | machineConfigSelector не нашёл MC | Проверить лейблы на MC |
| `invalid file path` | Относительный путь в MC | Использовать абсолютный путь |
| `duplicate file path` | Конфликт файлов с одинаковым priority | Изменить priority одного из MC |

---

## Проблемы с нодами

### Нода в состоянии error

**Симптом:** `agent-state: error`

```bash
# Посмотреть ошибку
kubectl get node NODE -o jsonpath='{.metadata.annotations.mco\.in-cloud\.io/last-error}'
```

**Частые ошибки:**

| Ошибка | Причина | Решение |
|--------|---------|---------|
| `failed to write file` | Нет прав или путь не существует | Проверить путь и права Agent |
| `systemd unit not found` | Юнит не установлен | Установить пакет с юнитом |
| `failed to enable unit` | Невалидный юнит | Проверить имя юнита |
| `timeout applying config` | Слишком долгое применение | Увеличить applyTimeoutSeconds |

**Сбросить ошибку:**

1. Исправить причину ошибки
2. Пересоздать MachineConfig (или изменить его)
3. Agent автоматически повторит попытку

### Нода застряла в applying

**Симптом:** `agent-state: applying` долгое время

```bash
# Проверить логи Agent на этой ноде
kubectl logs -n mco-system -l app=mco-agent --field-selector spec.nodeName=NODE_NAME

# Проверить состояние пода Agent
kubectl get pod -n mco-system -l app=mco-agent --field-selector spec.nodeName=NODE_NAME
```

**Возможные причины:**
- Agent завис
- Длительная операция с systemd
- Проблема с сетью к API-серверу

**Решение:**
```bash
# Перезапустить Agent на ноде
kubectl delete pod -n mco-system -l app=mco-agent --field-selector spec.nodeName=NODE_NAME
```

### current-revision не обновляется

**Симптом:** `current-revision` ≠ `desired-revision`, но `state` = `done`

```bash
# Проверить что Agent работает
kubectl get pod -n mco-system -l app=mco-agent --field-selector spec.nodeName=NODE_NAME

# Проверить логи
kubectl logs -n mco-system -l app=mco-agent --field-selector spec.nodeName=NODE_NAME --tail=100
```

**Возможные причины:**
- Agent не может обновить аннотации (RBAC)
- Проблема с сетью

---

## Проблемы с Agent

### Agent не запускается

```bash
# Статус пода
kubectl describe pod -n mco-system -l app=mco-agent

# События
kubectl get events -n mco-system --field-selector involvedObject.kind=Pod
```

**Частые причины:**

| Ошибка | Решение |
|--------|---------|
| `ImagePullBackOff` | Проверить registry и credentials |
| `CrashLoopBackOff` | Смотреть логи: `kubectl logs POD_NAME -n mco-system` |
| `SecurityContext` | Agent требует privileged для systemd |

### Agent не видит изменения

**Симптом:** После изменения MC ничего не происходит

```bash
# Проверить что RMC создан
kubectl get rmc

# Проверить что desired-revision обновился
kubectl get node NODE -o jsonpath='{.metadata.annotations.mco\.in-cloud\.io/desired-revision}'

# Проверить логи Agent
kubectl logs -n mco-system POD_NAME -f
```

**Возможные причины:**
- Пул на паузе (`paused: true`)
- Debounce ещё не прошёл
- Agent не watch'ит свою ноду

---

## Проблемы с Controller

### Controller не запускается

```bash
# Статус пода
kubectl describe pod -n mco-system -l control-plane=controller-manager

# Логи
kubectl logs -n mco-system deployment/mco-controller
```

**Частые причины:**

| Ошибка | Решение |
|--------|---------|
| `CRD not found` | Запустить `make install` |
| `leader election failed` | Проверить RBAC |
| `webhook cert error` | Пересоздать сертификаты |

### Controller не создаёт RMC

```bash
# Логи Controller
kubectl logs -n mco-system deployment/mco-controller | grep -i render

# Проверить что MC выбираются
kubectl get mc -l mco.in-cloud.io/pool=worker
```

**Возможные причины:**
- MC не имеют правильных лейблов
- Пул на паузе
- Ошибка в MC (невалидные данные)

---

## Проблемы с файлами

### Файл не создаётся

```bash
# Проверить что файл в RMC
kubectl get rmc RENDERED_NAME -o jsonpath='{.spec.config.files}' | jq .

# Проверить путь
minikube ssh "ls -la /path/to/file"
```

**Частые причины:**

| Проблема | Решение |
|----------|---------|
| Родительская директория не существует | Agent создаёт её автоматически, проверить права |
| Файл readonly (immutable) | Снять флаг immutable: `chattr -i /path/to/file` |
| Диск заполнен | Освободить место |

### Неправильные права на файл

```bash
# Проверить mode в MC
kubectl get mc CONFIG_NAME -o jsonpath='{.spec.files[0].mode}'

# Помните: mode в DECIMAL, не octal!
# 0644 = 420
# 0755 = 493
```

---

## Проблемы с systemd

### Сервис не запускается

```bash
# Проверить на ноде
minikube ssh "systemctl status SERVICE_NAME"

# Журнал сервиса
minikube ssh "journalctl -u SERVICE_NAME -n 50"
```

**Частые причины:**

| Проблема | Решение |
|----------|---------|
| Юнит не существует | Установить пакет |
| Зависимости не удовлетворены | Проверить `journalctl` |
| Конфликт с другим юнитом | Проверить зависимости |

### Сервис masked

```bash
# Проверить статус
minikube ssh "systemctl status SERVICE_NAME"
# Loaded: masked (/dev/null; bad)
```

**Решение:**
```bash
# Unmask вручную
minikube ssh "sudo systemctl unmask SERVICE_NAME"

# Или через MC:
spec:
  systemd:
    units:
      - name: SERVICE_NAME.service
        mask: false
```

---

## Полезные команды для диагностики

```bash
# Полный дамп состояния
kubectl get mcp,mc,rmc -o yaml > mco-dump.yaml

# Все аннотации MCO на нодах
kubectl get nodes -o json | jq '.items[] | {name: .metadata.name, annotations: .metadata.annotations | with_entries(select(.key | startswith("mco")))}'

# Логи за последние 5 минут
kubectl logs -n mco-system deployment/mco-controller --since=5m
kubectl logs -n mco-system -l app=mco-agent --since=5m

# События в namespace mco-system
kubectl get events -n mco-system --sort-by='.lastTimestamp'
```

---

## Когда обращаться за помощью

Соберите информацию:

```bash
# Версия
kubectl version --short

# Статус компонентов
kubectl get pods -n mco-system
kubectl get mcp,mc,rmc

# Логи
kubectl logs -n mco-system deployment/mco-controller > controller.log
kubectl logs -n mco-system -l app=mco-agent > agent.log

# Дамп конфигурации
kubectl get mcp,mc,rmc -o yaml > config-dump.yaml
```

Откройте issue с этой информацией.

---

## Связанные документы

- [Проверка применения](verification.md) — как проверить что работает
- [Мониторинг статуса](status-monitoring.md) — отслеживание состояний
