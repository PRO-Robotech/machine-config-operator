# MCO Lite — Документация

> **Machine Config Operator Lite** — Kubernetes-оператор для декларативного управления конфигурацией хостов.

---

## О проекте

MCO Lite позволяет централизованно управлять конфигурацией Linux-хостов в Kubernetes-кластере:
- Создавать и обновлять файлы на нодах
- Управлять systemd-сервисами
- Контролировать перезагрузки нод

Оператор вдохновлён OpenShift Machine Config Operator, но спроектирован проще и работает с любым дистрибутивом Linux.

---

## Для кого эта документация

Документация предназначена для **DevOps-инженеров** и **SRE-специалистов**, которые:
- Устанавливают и настраивают MCO Lite в кластере
- Создают конфигурации для управления нодами
- Мониторят состояние применения конфигураций

---

## Навигация по документации

### Концепции

| Документ | Описание |
|----------|----------|
| [Обзор проекта](concepts/overview.md) | Что такое MCO Lite, ключевые особенности |
| [Архитектура](concepts/architecture.md) | Компоненты: Controller и Agent |
| [Терминология](concepts/terminology.md) | Глоссарий терминов и понятий |

### Начало работы

| Документ | Описание |
|----------|----------|
| [Требования](getting-started/prerequisites.md) | Что нужно для установки |
| [Установка](getting-started/installation.md) | Установка в minikube и production |
| [Быстрый старт](getting-started/quickstart.md) | Первая конфигурация за 5 минут |

### Руководство пользователя

| Документ | Описание |
|----------|----------|
| [MachineConfig](user-guide/machineconfig.md) | Создание конфигураций хоста |
| [MachineConfigPool](user-guide/machineconfigpool.md) | Группировка нод и конфигов |
| [Проверка применения](user-guide/verification.md) | Как убедиться что конфиг применился |
| [Мониторинг статуса](user-guide/status-monitoring.md) | Отслеживание состояния нод |
| [Устранение проблем](user-guide/troubleshooting.md) | Диагностика ошибок |

### Примеры

| Документ | Описание |
|----------|----------|
| [Каталог примеров](examples/README.md) | Все примеры с описанием |
| [Базовые примеры](examples/basic/) | NTP, sysctl, пулы, файлы |
| [Продвинутые примеры](examples/advanced/) | Мульти-пулы, перезагрузки, приоритеты |

---

## Быстрые ссылки

```bash
# Установка CRD
make install

# Деплой оператора
make deploy IMG=<your-registry>/mco-controller:latest

# Применение конфигурации
kubectl apply -f docs/examples/basic/01-ntp-config.yaml

# Проверка статуса пула
kubectl get mcp

# Проверка статуса нод
kubectl get nodes -o custom-columns=\
NAME:.metadata.name,\
REVISION:.metadata.annotations.mco\\.in-cloud\\.io/current-revision,\
STATE:.metadata.annotations.mco\\.in-cloud\\.io/agent-state
```

---

## История изменений

См. [CHANGELOG.md](CHANGELOG.md)

---

## Связанные ресурсы

- [Внутренняя документация](../documentation/) — ADR, PRD, Epic/Story для разработчиков
- [API Reference](../api/v1alpha1/) — Go-типы API
- [Примеры манифестов](../config/samples/) — Примеры из репозитория
