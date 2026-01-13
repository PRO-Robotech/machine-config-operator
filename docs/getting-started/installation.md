# Установка

Установка MCO Lite в Kubernetes кластер.

---

## Быстрая установка

```bash
# 1. Клонировать репозиторий
git clone https://github.com/your-org/machine-config.git
cd machine-config

# 2. Установить CRD
make install

# 3. Развернуть оператор
make deploy IMG=<your-registry>/mco-controller:latest
```

---

## Пошаговая установка

### 1. Установка CRD

```bash
# Из репозитория
make install

# Или вручную
kubectl apply -f config/crd/bases/
```

**Проверка:**
```bash
kubectl get crd | grep mco
# mco.in-cloud.io_machineconfigs.mco.in-cloud.io
# mco.in-cloud.io_machineconfigpools.mco.in-cloud.io
# mco.in-cloud.io_renderedmachineconfigs.mco.in-cloud.io
```

### 2. Сборка образов

```bash
# Controller
docker build -t <registry>/mco-controller:latest -f Dockerfile.controller .
docker push <registry>/mco-controller:latest

# Agent
docker build -t <registry>/mco-agent:latest -f Dockerfile.agent .
docker push <registry>/mco-agent:latest
```

### 3. Развёртывание

```bash
# С make
make deploy \
  CONTROLLER_IMG=<registry>/mco-controller:latest \
  AGENT_IMG=<registry>/mco-agent:latest

# Или с kustomize
cd config/default
kustomize edit set image controller=<registry>/mco-controller:latest
kustomize edit set image agent=<registry>/mco-agent:latest
kustomize build | kubectl apply -f -
```

**Проверка:**
```bash
# Controller
kubectl get deployment -n mco-system mco-controller
kubectl get pod -n mco-system -l control-plane=controller-manager

# Agent
kubectl get daemonset -n mco-system mco-agent
kubectl get pod -n mco-system -l app=mco-agent
```

---

## Установка в minikube

```bash
# 1. Запустить minikube с несколькими нодами
minikube start --nodes 3

# 2. Установить CRD
make install

# 3. Собрать образы внутри minikube
eval $(minikube docker-env)
make docker-build-controller IMG=mco-controller:local
make docker-build-agent IMG=mco-agent:local

# 4. Развернуть с imagePullPolicy: Never
make deploy IMG=mco-controller:local AGENT_IMG=mco-agent:local
```

---

## Установка в Kind

```bash
# 1. Создать Kind кластер
make kind-create

# 2. Собрать и загрузить образы
make docker-build-controller IMG=mco-controller:e2e
make docker-build-agent IMG=mco-agent:e2e
kind load docker-image mco-controller:e2e
kind load docker-image mco-agent:e2e

# 3. Установить и развернуть
make install
make deploy-e2e
```

---

## Конфигурация

### Controller параметры

Можно настроить через аргументы командной строки:

```yaml
# config/manager/manager.yaml
spec:
  containers:
    - name: manager
      args:
        - --leader-elect              # Leader election
        - --health-probe-bind-address=:8081
```

### Agent параметры

```yaml
# config/agent/daemonset.yaml
spec:
  containers:
    - name: agent
      args:
        - --host-root=/host           # Точка монтирования хоста
```

### Namespace

По умолчанию MCO Lite устанавливается в namespace `mco-system`.

Для изменения:
```bash
cd config/default
kustomize edit set namespace my-namespace
```

---

## Обновление

### Controller

```bash
# Обновить образ
kubectl set image deployment/mco-controller \
  -n mco-system \
  manager=<registry>/mco-controller:new-version

# Или через make
make deploy IMG=<registry>/mco-controller:new-version
```

### Agent

```bash
# Обновить образ в DaemonSet
kubectl set image daemonset/mco-agent \
  -n mco-system \
  agent=<registry>/mco-agent:new-version
```

### CRD

```bash
# При изменении CRD
make install
```

---

## Удаление

```bash
# Удалить развёртывание
make undeploy

# Удалить CRD (ВНИМАНИЕ: удалит все MC, MCP, RMC!)
make uninstall
```

---

## Проверка установки

```bash
# 1. Компоненты запущены
kubectl get pods -n mco-system

# 2. CRD доступны
kubectl api-resources | grep mco

# 3. RBAC настроен
kubectl auth can-i get machineconfigs --as=system:serviceaccount:mco-system:mco-controller-manager

# 4. Логи без ошибок
kubectl logs -n mco-system deployment/mco-controller --tail=50
```

---

## Следующие шаги

- [Быстрый старт](quickstart.md) — создать первую конфигурацию
- [MachineConfigPool](../user-guide/machineconfigpool.md) — настроить пулы

