# Каталог примеров

Готовые к использованию примеры MachineConfig и MachineConfigPool.

---

## Структура

```
examples/
├── basic/                      # Базовые сценарии
│   ├── 01-ntp-config.yaml      # Настройка NTP
│   ├── 02-sysctl-config.yaml   # Настройка sysctl
│   ├── 03-worker-pool.yaml     # Базовый worker pool
│   └── 04-file-management.yaml # Управление файлами
│
└── advanced/                   # Продвинутые сценарии
    ├── 01-multi-pool.yaml      # Несколько пулов
    ├── 02-reboot-strategy.yaml # Стратегии перезагрузки
    ├── 03-priority-merge.yaml  # Приоритеты и слияние
    └── 04-systemd-units.yaml   # Управление systemd
```

---

## Базовые примеры

| Файл | Описание | Что демонстрирует |
|------|----------|-------------------|
| [01-ntp-config.yaml](basic/01-ntp-config.yaml) | Настройка NTP через chrony | Файлы + systemd |
| [02-sysctl-config.yaml](basic/02-sysctl-config.yaml) | Настройка kernel параметров | Файлы + reboot |
| [03-worker-pool.yaml](basic/03-worker-pool.yaml) | Базовый пул для worker-нод | nodeSelector, machineConfigSelector |
| [04-file-management.yaml](basic/04-file-management.yaml) | Создание и удаление файлов | state: present/absent |

---

## Продвинутые примеры

| Файл | Описание | Что демонстрирует |
|------|----------|-------------------|
| [01-multi-pool.yaml](advanced/01-multi-pool.yaml) | Worker + Master пулы | Разные конфиги для разных ролей |
| [02-reboot-strategy.yaml](advanced/02-reboot-strategy.yaml) | Стратегии перезагрузки | Never vs IfRequired |
| [03-priority-merge.yaml](advanced/03-priority-merge.yaml) | Слияние конфигов по приоритету | priority, override |
| [04-systemd-units.yaml](advanced/04-systemd-units.yaml) | Управление systemd | enable, start, mask |

---

## Как использовать примеры

### Применение примера

```bash
# Применить один пример
kubectl apply -f docs/examples/basic/01-ntp-config.yaml

# Применить все базовые примеры
kubectl apply -f docs/examples/basic/

# Применить все примеры
kubectl apply -R -f docs/examples/
```

### Проверка результата

```bash
# Статус пулов
kubectl get mcp

# Статус MachineConfig
kubectl get mc

# RenderedMachineConfig
kubectl get rmc

# Статус нод
kubectl get nodes -o custom-columns=\
'NAME:.metadata.name,'\
'STATE:.metadata.annotations.mco\.in-cloud\.io/agent-state,'\
'REVISION:.metadata.annotations.mco\.in-cloud\.io/current-revision'
```

### Удаление примера

```bash
# Удалить конкретный пример
kubectl delete -f docs/examples/basic/01-ntp-config.yaml

# Удалить все примеры
kubectl delete -R -f docs/examples/
```

---

## Порядок применения

### Для начала работы

1. Сначала создайте пул:
   ```bash
   kubectl apply -f docs/examples/basic/03-worker-pool.yaml
   ```

2. Добавьте лейбл на ноду:
   ```bash
   kubectl label node NODE_NAME node-role.kubernetes.io/worker=""
   ```

3. Примените конфигурации:
   ```bash
   kubectl apply -f docs/examples/basic/01-ntp-config.yaml
   kubectl apply -f docs/examples/basic/02-sysctl-config.yaml
   ```

### Для очистки

```bash
# Удалить все MC и MCP
kubectl delete mc --all
kubectl delete mcp --all

# RMC удалятся автоматически (ownerReference)
```

---

## Адаптация примеров

### Изменение целевых нод

Замените `nodeSelector` в пуле:

```yaml
spec:
  nodeSelector:
    matchLabels:
      # Ваш лейбл вместо стандартного
      my-custom-role: "app-server"
```

### Изменение содержимого файлов

Отредактируйте `content` в MachineConfig:

```yaml
spec:
  files:
    - path: /etc/myapp/config.conf
      content: |
        # Ваше содержимое
        key = value
```

### Изменение параметров раскатки

```yaml
spec:
  rollout:
    debounceSeconds: 60        # Увеличить для production
    applyTimeoutSeconds: 1200  # Для больших конфигов
```

---

## Связанные документы

- [MachineConfig](../user-guide/machineconfig.md) — полная спецификация
- [MachineConfigPool](../user-guide/machineconfigpool.md) — настройка пулов
- [Быстрый старт](../getting-started/quickstart.md) — начало работы
