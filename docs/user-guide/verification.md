# Проверка применения конфигурации

Как убедиться, что MachineConfig успешно применился на ноды.

---

## Быстрая проверка

### 1. Статус пула

```bash
kubectl get mcp worker
```

```
NAME     TARGET                      CURRENT                     READY   UPDATED   DEGRADED   AGE
worker   rendered-worker-a1b2c3d4e5  rendered-worker-a1b2c3d4e5  3       3         0          1h
```

| Колонка | Значение для успеха |
|---------|---------------------|
| TARGET | Имя текущего RMC |
| CURRENT | Совпадает с TARGET |
| READY | Равно общему числу нод |
| DEGRADED | 0 |

### 2. Аннотации ноды

```bash
kubectl get node NODE_NAME -o jsonpath='{.metadata.annotations}' | jq .
```

```json
{
  "mco.in-cloud.io/current-revision": "rendered-worker-a1b2c3d4e5",
  "mco.in-cloud.io/desired-revision": "rendered-worker-a1b2c3d4e5",
  "mco.in-cloud.io/agent-state": "done",
  "mco.in-cloud.io/pool": "worker"
}
```

**Успех когда:**
- `current-revision` == `desired-revision`
- `agent-state` == `done`

---

## Детальная проверка

### Проверка MachineConfig

```bash
# MC создан
kubectl get mc
# NAME          PRIORITY   AGE
# ntp-config    50         5m
# sysctl-base   40         5m

# Детали MC
kubectl describe mc ntp-config
```

### Проверка RenderedMachineConfig

```bash
# RMC создан
kubectl get rmc
# NAME                        POOL     REVISION     REBOOT   AGE
# rendered-worker-a1b2c3d4e5  worker   a1b2c3d4e5   false    5m

# Содержимое RMC (слитый конфиг)
kubectl get rmc rendered-worker-a1b2c3d4e5 -o yaml
```

Проверьте что в `spec.config`:
- Файлы из всех MC присутствуют
- Systemd юниты из всех MC присутствуют

### Проверка статуса пула

```bash
# Полный статус
kubectl get mcp worker -o yaml

# Только счётчики
kubectl get mcp worker -o jsonpath='{.status}' | jq .
```

```json
{
  "targetRevision": "rendered-worker-a1b2c3d4e5",
  "currentRevision": "rendered-worker-a1b2c3d4e5",
  "machineCount": 3,
  "readyMachineCount": 3,
  "updatedMachineCount": 3,
  "updatingMachineCount": 0,
  "degradedMachineCount": 0
}
```

### Проверка условий пула

```bash
kubectl get mcp worker -o jsonpath='{.status.conditions}' | jq .
```

```json
[
  {
    "type": "Updated",
    "status": "True",
    "reason": "AllNodesUpdated",
    "message": "All nodes are updated to target revision"
  },
  {
    "type": "Updating",
    "status": "False"
  },
  {
    "type": "Degraded",
    "status": "False"
  }
]
```

---

## Проверка всех нод

### Таблица статусов нод

```bash
kubectl get nodes -o custom-columns=\
'NAME:.metadata.name,'\
'POOL:.metadata.annotations.mco\.in-cloud\.io/pool,'\
'STATE:.metadata.annotations.mco\.in-cloud\.io/agent-state,'\
'CURRENT:.metadata.annotations.mco\.in-cloud\.io/current-revision,'\
'DESIRED:.metadata.annotations.mco\.in-cloud\.io/desired-revision'
```

```
NAME      POOL     STATE   CURRENT                     DESIRED
node-1    worker   done    rendered-worker-a1b2c3d4e5  rendered-worker-a1b2c3d4e5
node-2    worker   done    rendered-worker-a1b2c3d4e5  rendered-worker-a1b2c3d4e5
node-3    worker   done    rendered-worker-a1b2c3d4e5  rendered-worker-a1b2c3d4e5
```

### Найти ноды с проблемами

```bash
# Ноды в состоянии error
kubectl get nodes -o json | jq -r '
  .items[] |
  select(.metadata.annotations["mco.in-cloud.io/agent-state"] == "error") |
  .metadata.name'

# Ноды с pending reboot
kubectl get nodes -o json | jq -r '
  .items[] |
  select(.metadata.annotations["mco.in-cloud.io/reboot-pending"] == "true") |
  .metadata.name'

# Ноды где current != desired
kubectl get nodes -o json | jq -r '
  .items[] |
  select(.metadata.annotations["mco.in-cloud.io/current-revision"] !=
         .metadata.annotations["mco.in-cloud.io/desired-revision"]) |
  .metadata.name'
```

---

## Проверка на самой ноде

### Через SSH (если доступен)

```bash
# Проверить файл
ssh NODE_NAME "cat /etc/chrony.conf"

# Проверить права
ssh NODE_NAME "ls -la /etc/chrony.conf"

# Проверить владельца
ssh NODE_NAME "stat -c '%U:%G' /etc/chrony.conf"

# Проверить статус сервиса
ssh NODE_NAME "systemctl status chronyd"
ssh NODE_NAME "systemctl is-enabled chronyd"
```

### Через minikube

```bash
# Войти в ноду
minikube ssh

# Проверить файл
cat /etc/chrony.conf

# Проверить сервис
systemctl status chronyd
```

### Через kubectl exec (Agent debug pod)

Если Agent запускает debug контейнер:

```bash
kubectl exec -n mco-system -it POD_NAME -- cat /host/etc/chrony.conf
```

---

## Сценарии проверки

### Сценарий: Файл создан

```bash
# 1. Проверить что MC существует
kubectl get mc my-config

# 2. Проверить что RMC содержит файл
kubectl get rmc rendered-worker-xxx -o jsonpath='{.spec.config.files[*].path}'

# 3. Проверить аннотации ноды
kubectl get node NODE -o jsonpath='{.metadata.annotations.mco\.in-cloud\.io/agent-state}'
# Должно быть: done

# 4. (Опционально) Проверить файл на ноде
minikube ssh "cat /path/to/file"
```

### Сценарий: Сервис запущен

```bash
# 1. Проверить что unit в RMC
kubectl get rmc rendered-worker-xxx -o jsonpath='{.spec.config.systemd.units[*].name}'

# 2. Проверить статус ноды
kubectl get node NODE -o jsonpath='{.metadata.annotations.mco\.in-cloud\.io/agent-state}'

# 3. Проверить сервис на ноде
minikube ssh "systemctl status myservice"
```

### Сценарий: Требуется перезагрузка

```bash
# 1. Проверить reboot в RMC
kubectl get rmc rendered-worker-xxx -o jsonpath='{.spec.reboot}'
# {"required":true,"strategy":"Never"}

# 2. Проверить аннотацию reboot-pending
kubectl get node NODE -o jsonpath='{.metadata.annotations.mco\.in-cloud\.io/reboot-pending}'
# true

# 3. Проверить счётчик в пуле
kubectl get mcp worker -o jsonpath='{.status.pendingRebootCount}'
# 1
```

---

## Полезные алиасы

Добавьте в `~/.bashrc` или `~/.zshrc`:

```bash
# Статус всех пулов
alias mcp='kubectl get mcp'

# Статус нод с MCO аннотациями
alias mcnodes='kubectl get nodes -o custom-columns="NAME:.metadata.name,POOL:.metadata.annotations.mco\.in-cloud\.io/pool,STATE:.metadata.annotations.mco\.in-cloud\.io/agent-state,REVISION:.metadata.annotations.mco\.in-cloud\.io/current-revision"'

# Найти проблемные ноды
alias mcdegraded='kubectl get nodes -o json | jq -r ".items[] | select(.metadata.annotations[\"mco.in-cloud.io/agent-state\"] == \"error\") | .metadata.name"'
```

---

## Связанные документы

- [Мониторинг статуса](status-monitoring.md) — отслеживание состояний
- [Устранение проблем](troubleshooting.md) — диагностика ошибок
