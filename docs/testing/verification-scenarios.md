# Сценарии проверки

Практические сценарии для проверки корректной работы MCO Lite.

---

## Базовые сценарии

### 1. Проверка применения файла

**Цель:** Убедиться что файл создаётся на ноде.

```bash
# 1. Создать MachineConfig
cat <<EOF | kubectl apply -f -
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: test-file
  labels:
    mco.in-cloud.io/pool: worker
spec:
  priority: 50
  files:
    - path: /etc/mco-test/verification.conf
      content: |
        # Created by MCO verification
        timestamp: $(date -Iseconds)
      mode: 420
EOF

# 2. Дождаться применения
kubectl wait --for=condition=Ready mcp/worker --timeout=120s

# 3. Проверить файл на ноде
kubectl exec -n mco-system $(kubectl get pod -n mco-system -l app=mco-agent -o name | head -1) -- \
  cat /host/etc/mco-test/verification.conf

# 4. Очистить
kubectl delete mc test-file
```

**Ожидаемый результат:** Файл существует с правильным содержимым.

---

### 2. Проверка удаления файла

**Цель:** Убедиться что файл удаляется при `state: absent`.

```bash
# 1. Сначала создать файл (см. сценарий 1)

# 2. Изменить на absent
kubectl patch mc test-file --type=merge -p '{"spec":{"files":[{"path":"/etc/mco-test/verification.conf","state":"absent"}]}}'

# 3. Дождаться применения
kubectl wait --for=condition=Ready mcp/worker --timeout=120s

# 4. Проверить что файл удалён
kubectl exec -n mco-system $(kubectl get pod -n mco-system -l app=mco-agent -o name | head -1) -- \
  ls /host/etc/mco-test/verification.conf 2>&1 || echo "File deleted (expected)"
```

**Ожидаемый результат:** Файл не существует.

---

### 3. Проверка Rolling Update

**Цель:** Убедиться что ноды обновляются последовательно.

```bash
# 1. Создать пул с maxUnavailable=1
cat <<EOF | kubectl apply -f -
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfigPool
metadata:
  name: test-rolling
spec:
  nodeSelector:
    matchLabels:
      node-role.kubernetes.io/worker: ""
  machineConfigSelector:
    matchLabels:
      mco.in-cloud.io/pool: test-rolling
  rollout:
    maxUnavailable: 1
    debounceSeconds: 5
EOF

# 2. Создать MC
cat <<EOF | kubectl apply -f -
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: test-rolling-mc
  labels:
    mco.in-cloud.io/pool: test-rolling
spec:
  files:
    - path: /etc/mco-test/rolling.conf
      content: "v1"
EOF

# 3. Мониторить прогресс
watch -n 1 'kubectl get nodes -o custom-columns="\
NAME:.metadata.name,\
CORDONED:.metadata.annotations.mco\.in-cloud\.io/cordoned,\
STATE:.metadata.annotations.mco\.in-cloud\.io/agent-state"'

# В другом терминале обновить MC:
kubectl patch mc test-rolling-mc --type=merge -p '{"spec":{"files":[{"path":"/etc/mco-test/rolling.conf","content":"v2"}]}}'
```

**Ожидаемый результат:** Только 1 нода cordoned/applying в каждый момент времени.

---

### 4. Проверка Pool Overlap

**Цель:** Убедиться что overlap детектируется.

```bash
# 1. Добавить лейбл на ноду
kubectl label node $(kubectl get nodes -o name | head -1 | cut -d/ -f2) overlap-test=true

# 2. Создать два пула с пересекающимися селекторами
cat <<EOF | kubectl apply -f -
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfigPool
metadata:
  name: overlap-pool-1
spec:
  nodeSelector:
    matchLabels:
      overlap-test: "true"
---
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfigPool
metadata:
  name: overlap-pool-2
spec:
  nodeSelector:
    matchLabels:
      overlap-test: "true"
EOF

# 3. Проверить condition
kubectl get mcp -o jsonpath='{range .items[*]}{.metadata.name}: {.status.conditions[?(@.type=="PoolOverlap")].status}{"\n"}{end}'

# 4. Очистить
kubectl delete mcp overlap-pool-1 overlap-pool-2
kubectl label node $(kubectl get nodes -o name | head -1 | cut -d/ -f2) overlap-test-
```

**Ожидаемый результат:** `PoolOverlap: True` на обоих пулах.

---

### 5. Проверка Pause

**Цель:** Убедиться что paused пул не обновляет ноды.

```bash
# 1. Создать paused пул
cat <<EOF | kubectl apply -f -
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfigPool
metadata:
  name: test-pause
spec:
  nodeSelector:
    matchLabels:
      node-role.kubernetes.io/worker: ""
  machineConfigSelector:
    matchLabels:
      mco.in-cloud.io/pool: test-pause
  paused: true
EOF

# 2. Создать MC
cat <<EOF | kubectl apply -f -
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: test-pause-mc
  labels:
    mco.in-cloud.io/pool: test-pause
spec:
  files:
    - path: /etc/mco-test/pause.conf
      content: "should not appear"
EOF

# 3. Подождать и проверить что desired-revision НЕ установлен
sleep 10
kubectl get nodes -o jsonpath='{.items[*].metadata.annotations.mco\.in-cloud\.io/desired-revision}'
# Должен быть пустым

# 4. Unpause
kubectl patch mcp test-pause --type=merge -p '{"spec":{"paused":false}}'

# 5. Проверить что теперь desired-revision установлен
kubectl wait --for=condition=Ready mcp/test-pause --timeout=120s
```

**Ожидаемый результат:** При paused — ноды не обновляются; после unpause — обновляются.

---

## Сценарии обработки ошибок

### 6. Проверка обработки ошибки Agent

**Цель:** Убедиться что ошибка применения корректно репортится.

```bash
# Создать MC с невалидным путём (permission denied ожидается)
cat <<EOF | kubectl apply -f -
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: test-error
  labels:
    mco.in-cloud.io/pool: worker
spec:
  files:
    - path: /proc/test-invalid
      content: "should fail"
EOF

# Проверить состояние
kubectl get nodes -o jsonpath='{range .items[*]}{.metadata.name}: {.metadata.annotations.mco\.in-cloud\.io/agent-state}{"\n"}{end}'

# Проверить ошибку
kubectl get nodes -o jsonpath='{range .items[*]}{.metadata.name}: {.metadata.annotations.mco\.in-cloud\.io/last-error}{"\n"}{end}'
```

**Ожидаемый результат:** `agent-state: error` и сообщение об ошибке.

---

### 7. Проверка reboot-pending

**Цель:** Убедиться что reboot-pending устанавливается при strategy=Never.

```bash
cat <<EOF | kubectl apply -f -
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfigPool
metadata:
  name: test-reboot
spec:
  nodeSelector:
    matchLabels:
      node-role.kubernetes.io/worker: ""
  machineConfigSelector:
    matchLabels:
      mco.in-cloud.io/pool: test-reboot
  reboot:
    strategy: Never
---
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: test-reboot-mc
  labels:
    mco.in-cloud.io/pool: test-reboot
spec:
  files:
    - path: /etc/mco-test/reboot.conf
      content: "requires reboot"
  reboot:
    required: true
    reason: "Test reboot pending"
EOF

# Проверить reboot-pending
kubectl wait --for=condition=Ready mcp/test-reboot --timeout=120s
kubectl get nodes -o jsonpath='{range .items[*]}{.metadata.name}: {.metadata.annotations.mco\.in-cloud\.io/reboot-pending}{"\n"}{end}'
```

**Ожидаемый результат:** `reboot-pending: true` на нодах.

---

## Checklist для production

### Перед деплоем

- [ ] CRD установлены: `kubectl get crd | grep mco`
- [ ] Controller запущен: `kubectl get pod -n mco-system -l control-plane=controller-manager`
- [ ] Agent запущен на всех нодах: `kubectl get ds -n mco-system`
- [ ] RBAC настроен: `kubectl auth can-i --list --as=system:serviceaccount:mco-system:mco-controller-manager`

### После создания пула

- [ ] Ноды выбираются: `kubectl get mcp <name> -o jsonpath='{.status.machineCount}'`
- [ ] Нет overlap: `kubectl get mcp <name> -o jsonpath='{.status.conditions[?(@.type=="PoolOverlap")].status}'`
- [ ] RMC создан: `kubectl get rmc -l pool=<name>`

### После обновления

- [ ] Ready condition True: `kubectl get mcp <name> -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}'`
- [ ] Updating False: `kubectl get mcp <name> -o jsonpath='{.status.conditions[?(@.type=="Updating")].status}'`
- [ ] Draining False: `kubectl get mcp <name> -o jsonpath='{.status.conditions[?(@.type=="Draining")].status}'`
- [ ] Все ноды ready: `kubectl get mcp <name> -o jsonpath='{.status.readyMachineCount}'`
- [ ] Нет degraded: `kubectl get mcp <name> -o jsonpath='{.status.degradedMachineCount}'`
- [ ] Нет cordoned: `kubectl get mcp <name> -o jsonpath='{.status.cordonedMachineCount}'`

---

## Связанные документы

- [E2E тесты](e2e-tests.md) — автоматизированные тесты
- [Устранение проблем](../user-guide/troubleshooting.md) — диагностика ошибок

