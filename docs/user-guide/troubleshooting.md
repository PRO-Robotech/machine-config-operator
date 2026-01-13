# Устранение проблем

Диагностика и решение типичных проблем MCO Lite.

---

## Быстрая диагностика

```bash
# 1. Статус пулов
kubectl get mcp

# 2. Проблемные условия
kubectl get mcp -o jsonpath='{range .items[*]}{.metadata.name}{": "}{range .status.conditions[?(@.status=="True")]}{.type}{" "}{end}{"\n"}{end}'

# 3. Degraded ноды
kubectl get nodes -o json | jq -r '
  .items[] |
  select(.metadata.annotations["mco.in-cloud.io/agent-state"] == "error") |
  [.metadata.name, .metadata.annotations["mco.in-cloud.io/last-error"]] |
  @tsv'

# 4. Cordoned ноды
kubectl get nodes -o json | jq -r '
  .items[] |
  select(.metadata.annotations["mco.in-cloud.io/cordoned"] == "true") |
  .metadata.name'

# 5. Логи Controller
kubectl logs -n mco-system deployment/mco-controller --tail=50 | grep -i error

# 6. Логи Agent
kubectl logs -n mco-system -l app=mco-agent --tail=50 | grep -i error
```

---

## Проблемы с пулами

### Condition: PoolOverlap

**Симптом:** Condition `PoolOverlap` = True

```bash
kubectl get mcp -o jsonpath='{.items[*].status.conditions[?(@.type=="PoolOverlap")]}'
```

**Причина:** Нода матчит селекторы нескольких пулов.

**Диагностика:**
```bash
# Найти конфликтующие ноды
kubectl get events --field-selector reason=PoolOverlap
```

**Решение:**
1. Изменить `nodeSelector` одного из пулов
2. Удалить лишний лейбл с ноды
3. Удалить один из конфликтующих пулов

### Condition: DrainStuck

**Симптом:** Condition `DrainStuck` = True

```bash
kubectl get mcp worker -o jsonpath='{.status.conditions[?(@.type=="DrainStuck")]}'
```

**Причина:** Drain занимает больше `drainTimeoutSeconds`.

**Диагностика:**
```bash
# Найти застрявшую ноду
kubectl get nodes -o json | jq -r '
  .items[] |
  select(.metadata.annotations["mco.in-cloud.io/cordoned"] == "true") |
  {name: .metadata.name, 
   started: .metadata.annotations["mco.in-cloud.io/drain-started-at"],
   retries: .metadata.annotations["mco.in-cloud.io/drain-retry-count"]}'

# Проверить поды на ноде
kubectl get pods -A --field-selector spec.nodeName=<node>

# Проверить PDB
kubectl get pdb -A
```

**Решение:**
1. Увеличить реплики приложения (чтобы PDB позволил эвакуацию)
2. Временно удалить/изменить PDB
3. Вручную удалить зависший под

### Condition: Degraded с Reason=RenderFailed

**Симптом:** Condition `Degraded` = True с `Reason: RenderFailed`

```bash
kubectl get mcp worker -o jsonpath='{.status.conditions[?(@.type=="Degraded")]}'
```

**Частые причины:**

| Сообщение | Проблема | Решение |
|-----------|----------|---------|
| `no MachineConfigs selected` | machineConfigSelector не нашёл MC | Проверить лейблы на MC |
| `invalid file path` | Относительный путь в MC | Использовать абсолютный путь |
| `hash collision` | Редкий конфликт хешей | Изменить содержимое MC |

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
| `failed to fetch RMC` | RMC удалён или недоступен | Проверить RMC существует |

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

### Нода застряла в cordoned

**Симптом:** Нода cordoned, drain не завершается

```bash
# Проверить статус
kubectl get node <name> -o jsonpath='{.metadata.annotations}'
```

**Решение (ручное uncordon):**
```bash
# Снять cordon
kubectl uncordon <node>

# Удалить MCO аннотации
kubectl annotate node <node> \
  mco.in-cloud.io/cordoned- \
  mco.in-cloud.io/drain-started-at- \
  mco.in-cloud.io/drain-retry-count-
```

### current-revision не обновляется

**Симптом:** `current-revision` ≠ `desired-revision`, но `state` = `done`

**Возможные причины:**
- Agent не может обновить аннотации (RBAC)
- Проблема с сетью

```bash
# Проверить RBAC
kubectl auth can-i update nodes --as=system:serviceaccount:mco-system:mco-agent
```

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
```

**Возможные причины:**
- Пул на паузе (`paused: true`)
- Debounce ещё не прошёл
- Нода cordoned предыдущим обновлением
- maxUnavailable не позволяет (другие ноды updating)

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

### Controller pending

**Симптом:** Controller pod в состоянии Pending

```bash
kubectl describe pod -n mco-system -l control-plane=controller-manager
```

**Возможные причины:**
- Все ноды cordoned (нет места для scheduling)
- Controller не имеет tolerations для control-plane

> **Примечание:** Controller имеет toleration для control-plane тейнтов.
> Если все worker ноды cordoned, он запустится на control-plane.

---

## Проблемы с файлами

### Файл не создаётся

```bash
# Проверить что файл в RMC
kubectl get rmc RENDERED_NAME -o jsonpath='{.spec.config.files}' | jq .

# Проверить путь на ноде (через agent pod)
kubectl exec -n mco-system <agent-pod> -- ls -la /host/path/to/file
```

**Частые причины:**

| Проблема | Решение |
|----------|---------|
| Родительская директория не существует | Agent создаёт её автоматически |
| Файл readonly (immutable) | Снять флаг: `chattr -i /path/to/file` |
| Диск заполнен | Освободить место |

### Неправильные права на файл

```bash
# Проверить mode в MC
kubectl get mc CONFIG_NAME -o jsonpath='{.spec.files[0].mode}'
```

**Помните:** mode в DECIMAL, не octal!
- 0644 = 420
- 0755 = 493

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

# Проверить RBAC
kubectl auth can-i --list --as=system:serviceaccount:mco-system:mco-controller-manager
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

# Conditions
kubectl get mcp -o jsonpath='{range .items[*]}{.metadata.name}{": "}{.status.conditions}{"\n"}{end}'

# Логи
kubectl logs -n mco-system deployment/mco-controller > controller.log
kubectl logs -n mco-system -l app=mco-agent > agent.log

# Дамп конфигурации
kubectl get mcp,mc,rmc -o yaml > config-dump.yaml
```

Откройте issue с этой информацией.

---

## Связанные документы

- [Мониторинг статуса](status-monitoring.md) — отслеживание состояний
- [Rolling Update](rolling-update.md) — управление раскаткой
- [Cordon/Drain](cordon-drain.md) — безопасное обновление

