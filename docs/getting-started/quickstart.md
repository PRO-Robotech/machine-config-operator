# Быстрый старт

Создайте первую конфигурацию за 5 минут.

---

## Предварительные требования

- Kubernetes кластер с MCO Lite ([установка](installation.md))
- kubectl с доступом к кластеру
- Ноды с лейблом `node-role.kubernetes.io/worker`

---

## Шаг 1: Создать MachineConfigPool

Пул определяет какие ноды и какие конфигурации связаны.

```bash
cat <<EOF | kubectl apply -f -
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfigPool
metadata:
  name: worker
spec:
  # Какие ноды входят в пул
  nodeSelector:
    matchLabels:
      node-role.kubernetes.io/worker: ""
  
  # Какие MachineConfig применяются
  machineConfigSelector:
    matchLabels:
      mco.in-cloud.io/pool: worker
  
  # Настройки раскатки
  rollout:
    maxUnavailable: 1      # По одной ноде
    debounceSeconds: 30    # Ждать 30с после изменения
  
  # Стратегия перезагрузки
  reboot:
    strategy: Never        # Никогда не перезагружать автоматически
EOF
```

**Проверка:**
```bash
kubectl get mcp worker
```

---

## Шаг 2: Добавить лейбл на ноды

Если ноды ещё не имеют лейбла worker:

```bash
# Получить список нод
kubectl get nodes

# Добавить лейбл (замените NODE_NAME)
kubectl label node NODE_NAME node-role.kubernetes.io/worker=""

# Проверить
kubectl get mcp worker -o jsonpath='{.status.machineCount}'
```

---

## Шаг 3: Создать MachineConfig

Создадим простой конфигурационный файл:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: my-first-config
  labels:
    mco.in-cloud.io/pool: worker    # Связь с пулом
spec:
  priority: 50
  files:
    - path: /etc/myapp/config.yaml
      content: |
        # My Application Config
        server:
          port: 8080
          host: 0.0.0.0
        logging:
          level: info
          format: json
      mode: 420    # 0644 в decimal
      owner: "root:root"
EOF
```

---

## Шаг 4: Дождаться применения

```bash
# Следить за статусом
kubectl get mcp worker -w

# Или проверить условия
kubectl get mcp worker -o jsonpath='{.status.conditions[?(@.type=="Updated")].status}'
```

Когда `Updated: True` — конфигурация применена на всех нодах.

---

## Шаг 5: Проверить результат

```bash
# Проверить аннотации ноды
kubectl get nodes -o custom-columns='\
NAME:.metadata.name,\
STATE:.metadata.annotations.mco\.in-cloud\.io/agent-state,\
REVISION:.metadata.annotations.mco\.in-cloud\.io/current-revision'

# Проверить файл на ноде (через agent pod)
AGENT_POD=$(kubectl get pod -n mco-system -l app=mco-agent -o name | head -1)
kubectl exec -n mco-system $AGENT_POD -- cat /host/etc/myapp/config.yaml
```

---

## Что произошло

```
1. Вы создали MachineConfig
         ↓
2. Controller увидел изменение
         ↓
3. [Debounce 30s] Ждал стабилизации
         ↓
4. Renderer создал RenderedMachineConfig
         ↓
5. Controller установил desired-revision на ноды
         ↓
6. Controller cordoned первую ноду
         ↓
7. Controller drain (эвакуация подов)
         ↓
8. Agent увидел изменение
         ↓
9. Agent применил файл
         ↓
10. Agent обновил current-revision
         ↓
11. Controller uncordoned ноду
         ↓
12. Повторил для следующей ноды
         ↓
13. Status Updated: True
```

---

## Следующий шаг: Обновить конфигурацию

```bash
# Изменить содержимое
kubectl patch mc my-first-config --type=merge -p '
spec:
  files:
    - path: /etc/myapp/config.yaml
      content: |
        # Updated config
        server:
          port: 9090
          host: 0.0.0.0
        logging:
          level: debug
          format: json
'

# Следить за rolling update
kubectl get mcp worker -w

# Проверить новый контент
kubectl exec -n mco-system $AGENT_POD -- cat /host/etc/myapp/config.yaml
```

---

## Очистка

```bash
# Удалить MachineConfig
kubectl delete mc my-first-config

# Удалить MachineConfigPool
kubectl delete mcp worker
```

---

## Частые вопросы

### Почему ноды обновляются по одной?

Это контролируется `maxUnavailable: 1`. Для более быстрого обновления:

```yaml
rollout:
  maxUnavailable: "50%"    # Половина нод одновременно
```

### Почему debounce 30 секунд?

Чтобы не делать множество рендеров при пакетных изменениях. Для быстрой разработки:

```yaml
rollout:
  debounceSeconds: 5
```

### Как проверить что пошло не так?

```bash
# Статус пула
kubectl describe mcp worker

# Логи controller
kubectl logs -n mco-system deployment/mco-controller

# Логи agent
kubectl logs -n mco-system -l app=mco-agent
```

---

## Следующие шаги

- [MachineConfig](../user-guide/machineconfig.md) — полное руководство по конфигурациям
- [MachineConfigPool](../user-guide/machineconfigpool.md) — настройка пулов
- [Rolling Update](../user-guide/rolling-update.md) — управление раскаткой
- [Примеры](../examples/README.md) — больше примеров

