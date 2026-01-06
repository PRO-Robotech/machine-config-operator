# Быстрый старт

Создайте первую конфигурацию MCO Lite за 5 минут.

---

## Шаг 1: Проверка установки

```bash
# Убедитесь что MCO Lite установлен
kubectl get crd | grep mco
# machineconfigs.mco.in-cloud.io
# machineconfigpools.mco.in-cloud.io
# renderedmachineconfigs.mco.in-cloud.io

kubectl get pods -n mco-system
# NAME                              READY   STATUS
# mco-controller-xxxxx              1/1     Running
# mco-agent-xxxxx                   1/1     Running
```

---

## Шаг 2: Создание MachineConfigPool

Сначала создадим пул для worker-нод:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfigPool
metadata:
  name: worker
spec:
  # Select nodes with label node-role.kubernetes.io/worker
  nodeSelector:
    matchLabels:
      node-role.kubernetes.io/worker: ""
  # Select MachineConfigs with label mco.in-cloud.io/pool: worker
  machineConfigSelector:
    matchLabels:
      mco.in-cloud.io/pool: worker
  rollout:
    debounceSeconds: 5    # Wait 5 sec before rendering
  reboot:
    strategy: Never       # Never auto-reboot
EOF
```

Проверка:

```bash
kubectl get mcp
# NAME     TARGET   CURRENT   READY   UPDATED   DEGRADED   AGE
# worker                      0       0         0          5s
```

---

## Шаг 3: Добавление лейбла на ноду

Если у вашей ноды нет лейбла `worker`, добавьте его:

```bash
# Посмотреть ноды
kubectl get nodes

# Добавить лейбл (замените NODE_NAME на имя вашей ноды)
kubectl label node NODE_NAME node-role.kubernetes.io/worker=""
```

Проверка:

```bash
kubectl get mcp worker
# NAME     TARGET   CURRENT   READY   UPDATED   DEGRADED   AGE
# worker                      1       0         0          1m
#                             ^-- теперь 1 нода в пуле
```

---

## Шаг 4: Создание MachineConfig

Создадим конфигурацию NTP:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: ntp-config
  labels:
    mco.in-cloud.io/pool: worker    # Must match pool's machineConfigSelector
spec:
  priority: 50
  files:
    - path: /etc/chrony.conf
      content: |
        # MCO Lite managed NTP configuration
        server time.google.com iburst
        server time.cloudflare.com iburst
        driftfile /var/lib/chrony/drift
        makestep 1.0 3
        rtcsync
      mode: 420   # 0644 in decimal
      owner: "root:root"
      state: present
  systemd:
    units:
      - name: chronyd.service
        enabled: true
        state: started
  reboot:
    required: false
EOF
```

---

## Шаг 5: Проверка результата

### 5.1 RenderedMachineConfig создан

```bash
kubectl get rmc
# NAME                        POOL     REVISION     REBOOT   AGE
# rendered-worker-a1b2c3d4e5  worker   a1b2c3d4e5   false    10s
```

### 5.2 Статус пула обновился

```bash
kubectl get mcp worker
# NAME     TARGET                      CURRENT                     READY  ...
# worker   rendered-worker-a1b2c3d4e5  rendered-worker-a1b2c3d4e5  1      ...
```

### 5.3 Аннотации ноды

```bash
kubectl get node NODE_NAME -o jsonpath='{.metadata.annotations}' | jq .
# {
#   "mco.in-cloud.io/current-revision": "rendered-worker-a1b2c3d4e5",
#   "mco.in-cloud.io/desired-revision": "rendered-worker-a1b2c3d4e5",
#   "mco.in-cloud.io/agent-state": "done",
#   "mco.in-cloud.io/pool": "worker"
# }
```

### 5.4 Файл на ноде (опционально)

Если есть SSH-доступ к ноде:

```bash
ssh NODE_NAME cat /etc/chrony.conf
# # MCO Lite managed NTP configuration
# server time.google.com iburst
# ...
```

Или через kubectl (если Agent имеет debug возможности):

```bash
# В minikube
minikube ssh "cat /etc/chrony.conf"
```

---

## Что произошло?

```
1. Вы создали MachineConfigPool "worker"
   ↓
2. Pool выбрал ноды по nodeSelector
   ↓
3. Вы создали MachineConfig "ntp-config"
   ↓
4. Controller увидел новый MC
   ↓
5. [Debounce 5 сек]
   ↓
6. Renderer создал RenderedMachineConfig
   ↓
7. Controller записал desired-revision на ноду
   ↓
8. Agent увидел изменение
   ↓
9. Applier записал файл и запустил chronyd
   ↓
10. Agent записал current-revision = desired-revision
    ↓
11. Status Aggregator обновил статус пула
```

---

## Следующие шаги

### Изменить конфигурацию

```bash
# Обновить содержимое файла
kubectl edit mc ntp-config
# Измените content и сохраните

# Посмотреть как создаётся новый RMC
kubectl get rmc -w
```

### Добавить ещё один MachineConfig

```bash
kubectl apply -f docs/examples/basic/02-sysctl-config.yaml
```

### Удалить конфигурацию

```bash
kubectl delete mc ntp-config
# RMC обновится автоматически (без ntp-config)
```

---

## Полезные команды

```bash
# Статус всех пулов
kubectl get mcp

# Все MachineConfig
kubectl get mc

# Все RenderedMachineConfig
kubectl get rmc

# Статус конкретной ноды
kubectl get node NODE_NAME \
  -o custom-columns='NAME:.metadata.name,STATE:.metadata.annotations.mco\.in-cloud\.io/agent-state,REVISION:.metadata.annotations.mco\.in-cloud\.io/current-revision'

# Логи Controller
kubectl logs -n mco-system deployment/mco-controller -f

# Логи Agent
kubectl logs -n mco-system -l app=mco-agent -f
```

---

## Дальнейшее изучение

- [MachineConfig](../user-guide/machineconfig.md) — все возможности конфигурации
- [Примеры](../examples/README.md) — готовые примеры для разных сценариев
- [Проверка применения](../user-guide/verification.md) — детальная диагностика
