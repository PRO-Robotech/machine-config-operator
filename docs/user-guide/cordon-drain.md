# Cordon/Drain — Безопасное обновление нод

MCO Lite автоматически изолирует ноды и эвакуирует поды перед применением конфигурации.

---

## Обзор процесса

```
┌─────────────┐
│   Normal    │ ← Нода доступна для scheduling
└──────┬──────┘
       │ maxUnavailable slot available
       ▼
┌─────────────┐
│   Cordon    │ ← node.spec.unschedulable = true
└──────┬──────┘   mco.in-cloud.io/cordoned = true
       │
       ▼
┌─────────────┐
│    Drain    │ ← Eviction подов (PDB-aware)
└──────┬──────┘   mco.in-cloud.io/drain-started-at = timestamp
       │
       ▼
┌─────────────┐
│    Apply    │ ← Agent применяет файлы/systemd
└──────┬──────┘   agent-state = applying
       │
       ▼
┌─────────────┐
│  Uncordon   │ ← node.spec.unschedulable = false
└─────────────┘   Удаляются все mco annotations
```

---

## Cordon

### Что происходит

1. Controller помечает ноду как `unschedulable`:
   ```yaml
   spec:
     unschedulable: true
   ```

2. Controller добавляет аннотацию:
   ```yaml
   annotations:
     mco.in-cloud.io/cordoned: "true"
   ```

### Эффект

- **Новые поды** НЕ будут размещаться на ноде
- **Существующие поды** продолжают работать
- Нода остаётся частью кластера

### Проверка

```bash
kubectl get node <name> -o jsonpath='{.spec.unschedulable}'
# true

kubectl get node <name> -o jsonpath='{.metadata.annotations.mco\.in-cloud\.io/cordoned}'
# true
```

---

## Drain

### Что происходит

1. Controller записывает время начала drain:
   ```yaml
   annotations:
     mco.in-cloud.io/drain-started-at: "2026-01-09T10:00:00Z"
   ```

2. Получает список подов на ноде (исключая mirror и DaemonSet)

3. Для каждого пода создаёт Eviction (см. `internal/controller/drain.go:EvictPod`):
   ```go
   eviction := &policyv1.Eviction{
       ObjectMeta: metav1.ObjectMeta{
           Name:      pod.Name,
           Namespace: pod.Namespace,
       },
   }
   if gracePeriod >= 0 {
       eviction.DeleteOptions = &metav1.DeleteOptions{
           GracePeriodSeconds: &gracePeriod,
       }
   }
   err := c.SubResource("eviction").Create(ctx, pod, eviction)
   ```

4. Ждёт завершения эвакуации всех подов

### PodDisruptionBudget

MCO Lite **уважает PDB**. Если эвакуация нарушит PDB:
- Eviction возвращает ошибку
- Controller повторяет попытку через `drainRetrySeconds`
- Инкрементируется `drain-retry-count`

```yaml
# Пример PDB
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: web-pdb
spec:
  minAvailable: 2
  selector:
    matchLabels:
      app: web
```

### Исключения из Drain

Следующие поды **не эвакуируются**:

| Тип пода | Причина |
|----------|---------|
| Mirror pods | Управляются kubelet, не API |
| DaemonSet pods | Будут пересозданы автоматически |
| Static pods | Определены в манифестах на ноде |

### Timeout и DrainStuck

Если drain занимает больше `drainTimeoutSeconds`:

1. Устанавливается condition `DrainStuck`:
   ```yaml
   conditions:
   - type: DrainStuck
     status: "True"
     reason: DrainTimeout
     message: "Node node-1 drain exceeded 3600s timeout"
   ```

2. Эмитится Event:
   ```
   Warning  DrainStuck  MCO drain timeout on node-1 after 3600s
   ```

3. Drain **продолжает попытки** — не отменяется

### Мониторинг Drain

```bash
# Время начала drain
kubectl get node <name> -o jsonpath='{.metadata.annotations.mco\.in-cloud\.io/drain-started-at}'

# Количество retry
kubectl get node <name> -o jsonpath='{.metadata.annotations.mco\.in-cloud\.io/drain-retry-count}'

# Условие DrainStuck
kubectl get mcp <pool> -o jsonpath='{.status.conditions[?(@.type=="DrainStuck")]}'
```

---

## Uncordon

### Что происходит

После успешного применения конфигурации (`agent-state = done`):

1. Controller снимает `unschedulable`:
   ```yaml
   spec:
     unschedulable: false
   ```

2. Controller удаляет аннотации:
   - `mco.in-cloud.io/cordoned`
   - `mco.in-cloud.io/drain-started-at`
   - `mco.in-cloud.io/drain-retry-count`

### Условия Uncordon

Нода uncordon только когда:
- `current-revision == desired-revision`
- `agent-state == done`

### Проверка

```bash
kubectl get node <name> -o jsonpath='{.spec.unschedulable}'
# <empty> или false
```

---

## Конфигурация Drain

### drainTimeoutSeconds

```yaml
spec:
  rollout:
    drainTimeoutSeconds: 3600  # 1 час (default)
```

| Значение | Сценарий |
|----------|----------|
| 300 | Stateless, быстрая эвакуация |
| 3600 | Default, общего назначения |
| 7200+ | Stateful с длительным shutdown |

### drainRetrySeconds

```yaml
spec:
  rollout:
    drainRetrySeconds: 300  # Retry каждые 5 минут
```

По умолчанию: `max(30, drainTimeoutSeconds/12)`

---

## Диагностика проблем

### Drain блокируется PDB

**Симптом:** `drain-retry-count` растёт, поды не эвакуируются

```bash
# Найти PDB
kubectl get pdb -A

# Проверить статус PDB
kubectl describe pdb <name> -n <namespace>
```

**Решение:**
- Увеличить количество реплик приложения
- Временно изменить PDB (осторожно!)
- Дождаться завершения drain другой ноды

### Pod застрял в Terminating

**Симптом:** Под не завершается, drain не может продолжиться

```bash
kubectl get pods -A --field-selector spec.nodeName=<node>,status.phase=Terminating
```

**Решение:**
- Проверить preStop hooks
- Проверить finalizers
- При необходимости — force delete:
  ```bash
  kubectl delete pod <name> -n <namespace> --grace-period=0 --force
  ```

### Drain timeout на пустой ноде

**Симптом:** DrainStuck даже без подов

**Причины:**
- Controller не получает обновления статуса
- Проблемы с API server

**Решение:**
- Проверить логи controller
- Проверить connectivity к API server

---

## Best Practices

### 1. Настройте PDB для критичных приложений

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: api-pdb
spec:
  minAvailable: "50%"  # Всегда 50% подов доступны
  selector:
    matchLabels:
      app: api
```

### 2. Используйте разумные gracePeriod

```yaml
# В Deployment
spec:
  template:
    spec:
      terminationGracePeriodSeconds: 30  # 30 секунд на graceful shutdown
```

### 3. Мониторьте drain метрики

```yaml
# Prometheus Alert
- alert: MCODrainSlow
  expr: mco_drain_duration_seconds > 600
  for: 1m
  labels:
    severity: warning
  annotations:
    summary: "MCO drain taking too long"
```

### 4. Имейте достаточно capacity

Убедитесь что другие ноды могут принять поды:

```bash
# Проверить доступные ресурсы
kubectl describe nodes | grep -A 5 "Allocated resources"
```

---

## Ручные операции

### Ручной uncordon ноды

```bash
# Kubernetes uncordon
kubectl uncordon <node>

# + Удалить MCO аннотации
kubectl annotate node <node> \
  mco.in-cloud.io/cordoned- \
  mco.in-cloud.io/drain-started-at- \
  mco.in-cloud.io/drain-retry-count-
```

### Ручной drain ноды

```bash
kubectl drain <node> \
  --ignore-daemonsets \
  --delete-emptydir-data \
  --grace-period=60 \
  --timeout=600s
```

---

## Связанные документы

- [Rolling Update](rolling-update.md) — управление раскаткой
- [Мониторинг статуса](status-monitoring.md) — отслеживание состояния
- [Устранение проблем](troubleshooting.md) — диагностика ошибок

