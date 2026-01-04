# Установка MCO Lite

Это руководство описывает установку MCO Lite в Kubernetes кластер.

---

## Быстрая установка

### Из исходников

```bash
# Клонировать репозиторий
git clone https://github.com/your-org/machine-config.git
cd machine-config

# Установить CRD
make install

# Собрать и загрузить образы
make docker-build IMG=<your-registry>/mco-controller:latest
make docker-push IMG=<your-registry>/mco-controller:latest

# Развернуть оператор
make deploy IMG=<your-registry>/mco-controller:latest
```

### Проверка установки

```bash
# Проверить CRD
kubectl get crd | grep mco

# Ожидаемый вывод:
# machineconfigs.mco.in-cloud.io
# machineconfigpools.mco.in-cloud.io
# renderedmachineconfigs.mco.in-cloud.io

# Проверить Controller
kubectl get pods -n mco-system

# Ожидаемый вывод:
# NAME                              READY   STATUS    RESTARTS   AGE
# mco-controller-5d8f9c7b4f-xxxxx   1/1     Running   0          1m

# Проверить Agent (DaemonSet)
kubectl get ds -n mco-system

# Ожидаемый вывод:
# NAME        DESIRED   CURRENT   READY   AGE
# mco-agent   3         3         3       1m
```

---

## Установка в minikube

### Подготовка

```bash
# Запустить minikube (если не запущен)
minikube start --driver=docker --cpus=2 --memory=4096

# Переключить docker на minikube
eval $(minikube docker-env)
```

### Сборка и установка

```bash
# Установить CRD
make install

# Собрать образы в minikube
make docker-build IMG=mco-controller:latest

# Развернуть (с imagePullPolicy: Never для локальных образов)
make deploy IMG=mco-controller:latest
```

### Проверка

```bash
# Все поды должны быть Running
kubectl get pods -n mco-system -w

# Логи Controller
kubectl logs -n mco-system deployment/mco-controller -f

# Логи Agent на первой ноде
kubectl logs -n mco-system -l app=mco-agent --tail=50
```

---

## Установка в production

### Требования

- Container Registry для хранения образов
- Namespace `mco-system` (создаётся автоматически)
- RBAC настроен (создаётся автоматически)

### Шаг 1: Сборка образов

```bash
# Собрать и загрузить образы в ваш registry
export IMG=registry.example.com/mco/controller:v0.1.0
make docker-build docker-push IMG=$IMG
```

### Шаг 2: Установка CRD

```bash
make install
```

> **Важно:** CRD устанавливаются отдельно от оператора.
> Это позволяет обновлять оператор без потери данных.

### Шаг 3: Деплой оператора

```bash
make deploy IMG=$IMG
```

### Шаг 4: Проверка

```bash
# Все компоненты должны быть готовы
kubectl get pods -n mco-system

# Проверить логи на ошибки
kubectl logs -n mco-system deployment/mco-controller | grep -i error
```

---

## Конфигурация

### Параметры Controller

Controller настраивается через аргументы командной строки:

| Аргумент | По умолчанию | Описание |
|----------|--------------|----------|
| `--leader-elect` | true | Включить leader election |
| `--metrics-bind-address` | :8080 | Адрес для метрик |
| `--health-probe-bind-address` | :8081 | Адрес для health probes |

### Параметры Agent

Agent настраивается через environment variables:

| Переменная | По умолчанию | Описание |
|------------|--------------|----------|
| `NODE_NAME` | (из downward API) | Имя ноды |
| `LOG_LEVEL` | info | Уровень логирования |

---

## Удаление

### Удалить оператор (сохранить CRD и данные)

```bash
make undeploy
```

### Полное удаление (включая CRD и все данные)

```bash
# Удалить все MachineConfig, MachineConfigPool, RenderedMachineConfig
kubectl delete mc --all
kubectl delete mcp --all
kubectl delete rmc --all

# Удалить оператор
make undeploy

# Удалить CRD
make uninstall
```

> **Внимание:** Удаление CRD удалит все созданные ресурсы!

---

## Обновление

### Обновление оператора

```bash
# Собрать новую версию
export IMG=registry.example.com/mco/controller:v0.2.0
make docker-build docker-push IMG=$IMG

# Обновить деплой
make deploy IMG=$IMG
```

### Обновление CRD

```bash
# Применить новые CRD
make install

# Перезапустить оператор для подхвата изменений
kubectl rollout restart deployment/mco-controller -n mco-system
```

---

## Устранение проблем установки

### CRD не создаются

```bash
# Проверить права
kubectl auth can-i create crd
# Должно вернуть "yes"

# Проверить ошибки
kubectl get crd machineconfigs.mco.in-cloud.io -o yaml
```

### Controller не запускается

```bash
# Посмотреть события
kubectl describe pod -n mco-system -l control-plane=controller-manager

# Частые причины:
# - ImagePullBackOff: неверный образ или нет доступа к registry
# - CrashLoopBackOff: ошибка в конфигурации
```

### Agent не запускается

```bash
# Посмотреть статус DaemonSet
kubectl describe ds mco-agent -n mco-system

# Частые причины:
# - Нет прав на доступ к hostPath
# - Нет прав на работу с systemd (требуется privileged)
```

---

## Следующие шаги

- [Быстрый старт](quickstart.md) — создать первую конфигурацию
- [MachineConfig](../user-guide/machineconfig.md) — подробное руководство
