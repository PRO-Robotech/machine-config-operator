# Примеры конфигураций

Готовые примеры для типичных сценариев использования MCO Lite.

---

## Структура

```
examples/
├── basic/                       # Базовые примеры
│   ├── 01-ntp-config.yaml      # Настройка NTP
│   ├── 02-sysctl-config.yaml   # Параметры ядра
│   ├── 03-worker-pool.yaml     # Пул worker нод
│   └── 04-file-management.yaml # Работа с файлами
└── advanced/                    # Продвинутые примеры
    ├── 01-rolling-update.yaml  # Rolling Update с maxUnavailable
    ├── 02-drain-config.yaml    # Настройка drain
    ├── 03-multi-pool.yaml      # Несколько пулов
    └── 04-reboot-strategy.yaml # Стратегии перезагрузки
```

---

## Базовые примеры

| Файл | Описание |
|------|----------|
| [01-ntp-config.yaml](basic/01-ntp-config.yaml) | Синхронизация времени через chrony с Google/Cloudflare NTP |
| [02-sysctl-config.yaml](basic/02-sysctl-config.yaml) | Сетевые параметры ядра для Kubernetes |
| [03-worker-pool.yaml](basic/03-worker-pool.yaml) | Базовый MachineConfigPool для worker нод |
| [04-file-management.yaml](basic/04-file-management.yaml) | Создание файлов с разными правами и удаление файлов |

---

## Продвинутые примеры

| Файл | Описание |
|------|----------|
| [01-rolling-update.yaml](advanced/01-rolling-update.yaml) | Production rolling update с консервативными настройками |
| [02-drain-config.yaml](advanced/02-drain-config.yaml) | Drain для stateful workloads (4 часа timeout) |
| [03-multi-pool.yaml](advanced/03-multi-pool.yaml) | Несколько пулов: master, worker, gpu |
| [04-reboot-strategy.yaml](advanced/04-reboot-strategy.yaml) | MachineConfig с reboot.required и пул с IfRequired |

---

## Применение

```bash
# 1. Создать пул
kubectl apply -f examples/basic/03-worker-pool.yaml

# 2. Применить конфигурацию
kubectl apply -f examples/basic/01-ntp-config.yaml

# 3. Проверить статус
kubectl get mcp worker
kubectl get mc

# 4. Удалить
kubectl delete -f examples/basic/01-ntp-config.yaml
```

---

## Связанные документы

- [MachineConfig](../user-guide/machineconfig.md) — полное руководство
- [MachineConfigPool](../user-guide/machineconfigpool.md) — настройка пулов
- [Rolling Update](../user-guide/rolling-update.md) — управление раскаткой
