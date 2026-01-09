# Требования

Что нужно для установки и использования MCO Lite.

---

## Kubernetes кластер

### Минимальные требования

| Компонент | Версия | Примечание |
|-----------|--------|------------|
| Kubernetes | ≥ 1.26 | API server, kubelet |
| kubectl | ≥ 1.26 | Для управления |
| Linux nodes | любой | С systemd |

### Рекомендуемые требования

| Компонент | Версия | Примечание |
|-----------|--------|------------|
| Kubernetes | 1.28+ | Для лучшей совместимости |
| Nodes | ≥ 2 | Для тестирования rolling update |

---

## Node requirements

### Операционная система

MCO Lite работает с любым Linux дистрибутивом:
- Ubuntu 20.04+
- Debian 11+
- CentOS 8+ / Rocky Linux 8+
- Fedora 35+
- Amazon Linux 2

### Системные требования

| Требование | Описание |
|------------|----------|
| systemd | Для управления сервисами |
| hostPID | Agent требует доступ к host PID namespace |
| hostPath | Agent монтирует `/` хоста как `/host` |
| privileged | Agent требует privileged mode для systemd |

---

## Ресурсы

### Controller

| Ресурс | Request | Limit |
|--------|---------|-------|
| CPU | 100m | 500m |
| Memory | 128Mi | 256Mi |

### Agent (на каждой ноде)

| Ресурс | Request | Limit |
|--------|---------|-------|
| CPU | 50m | 200m |
| Memory | 64Mi | 128Mi |

---

## RBAC

MCO Lite требует следующие права:

### Controller

- `MachineConfig`: get, list, watch
- `MachineConfigPool`: get, list, watch, update, patch
- `RenderedMachineConfig`: get, list, watch, create, update, delete
- `Node`: get, list, watch, update, patch
- `Pod`: get, list, watch
- `Pod/eviction`: create
- `Event`: create, patch

### Agent

- `Node` (своя нода): get, patch
- `RenderedMachineConfig`: get

---

## Для разработки

### Локальный запуск

| Инструмент | Версия | Назначение |
|------------|--------|------------|
| Go | ≥ 1.22 | Компиляция |
| Docker | ≥ 20.10 | Сборка образов |
| Kind | ≥ 0.20 | Локальный кластер |
| make | — | Build система |

### Тестирование

```bash
# Установить инструменты
make install-tools

# Локальный кластер
make kind-create

# Запустить тесты
make test
make test-e2e
```

---

## Сетевые требования

### Внутрикластерные

| Источник | Назначение | Порт | Описание |
|----------|------------|------|----------|
| Controller | API Server | 6443 | Kubernetes API |
| Agent | API Server | 6443 | Kubernetes API |

### Внешние

MCO Lite **не требует** внешних сетевых подключений после установки.

---

## Следующие шаги

- [Установка](installation.md) — развернуть MCO Lite
- [Быстрый старт](quickstart.md) — первая конфигурация

