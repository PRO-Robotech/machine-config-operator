# Требования для установки

Перед установкой MCO Lite убедитесь, что ваше окружение соответствует требованиям.

---

## Kubernetes кластер

| Требование | Минимум | Рекомендуется |
|------------|---------|---------------|
| Версия Kubernetes | 1.24+ | 1.28+ |
| Ноды | 1 | 3+ |
| ОС на нодах | Любой Linux с systemd | Ubuntu 22.04+ |

### Поддерживаемые дистрибутивы

MCO Lite работает с любым Linux, где есть:
- **systemd** — для управления сервисами
- **Стандартная файловая система** — для управления файлами

Протестировано на:
- Ubuntu 20.04, 22.04, 24.04
- Debian 11, 12
- Rocky Linux 8, 9
- Fedora 38+

---

## Инструменты

### kubectl

```bash
# Проверка версии
kubectl version --client

# Минимальная версия: 1.24+
```

Установка: [kubernetes.io/docs/tasks/tools](https://kubernetes.io/docs/tasks/tools/)

### Доступ к кластеру

```bash
# Проверка подключения
kubectl cluster-info

# Должен показать адрес API-сервера
```

---

## Права доступа

### Для установки CRD

Требуются права на создание:
- CustomResourceDefinition
- ClusterRole / ClusterRoleBinding
- Namespace
- Deployment / DaemonSet
- ServiceAccount

```bash
# Проверка прав (должно вернуть "yes")
kubectl auth can-i create customresourcedefinitions
kubectl auth can-i create clusterroles
```

### Для использования

После установки, для создания MachineConfig/MachineConfigPool нужны права:
- `create`, `get`, `list`, `watch`, `update`, `delete` на ресурсы MCO

---

## Сетевые требования

### Agent → API Server

Agent на каждой ноде должен иметь доступ к Kubernetes API:
- Обычно через ServiceAccount token (автоматически)
- Порт: 443 (или кастомный порт API-сервера)

### Controller → Agent

Прямое соединение **не требуется**. Коммуникация через:
- Kubernetes API (аннотации нод)
- RenderedMachineConfig ресурсы

---

## Локальная разработка (опционально)

### minikube

Для тестирования на локальной машине:

```bash
# Установка minikube
# macOS
brew install minikube

# Linux
curl -LO https://storage.googleapis.com/minikube/releases/latest/minikube-linux-amd64
sudo install minikube-linux-amd64 /usr/local/bin/minikube

# Запуск кластера
minikube start --driver=docker --cpus=2 --memory=4096
```

### kind (альтернатива)

```bash
# Установка kind
go install sigs.k8s.io/kind@latest

# Создание кластера
kind create cluster --name mco-test
```

---

## Проверка готовности

Выполните эти команды для проверки:

```bash
# 1. kubectl работает
kubectl version --client
# ✓ Client Version: v1.28.0

# 2. Есть подключение к кластеру
kubectl cluster-info
# ✓ Kubernetes control plane is running at https://...

# 3. Есть права на CRD
kubectl auth can-i create crd
# ✓ yes

# 4. Ноды доступны
kubectl get nodes
# ✓ NAME       STATUS   ROLES           AGE
#   node-1     Ready    control-plane   1d
```

---

## Следующие шаги

- [Установка](installation.md) — установка MCO Lite в кластер
- [Быстрый старт](quickstart.md) — первая конфигурация за 5 минут
