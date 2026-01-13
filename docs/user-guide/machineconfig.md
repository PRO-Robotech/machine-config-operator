# MachineConfig — Руководство

MachineConfig — это **фрагмент конфигурации хоста**. Определяет файлы и systemd-юниты, которые должны быть на ноде.

---

## Базовая структура

```yaml
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: my-config
  labels:
    mco.in-cloud.io/pool: worker    # Required: связь с пулом
spec:
  priority: 50                       # Optional: порядок слияния
  files:                             # Optional: файлы для управления
    - path: /etc/example.conf
      content: "..."
  systemd:                           # Optional: systemd юниты
    units:
      - name: example.service
        enabled: true
  reboot:                            # Optional: требования перезагрузки
    required: false
```

---

## Полная спецификация

### spec.priority

| Поле | Тип | По умолчанию | Диапазон |
|------|-----|--------------|----------|
| `priority` | int | 50 | 0 - 99999 |

**Назначение:** Определяет порядок слияния при конфликтах.

- **Меньший** приоритет применяется **первым**
- **Больший** приоритет **побеждает** при конфликтах
- При **равном** приоритете побеждает **большее имя** (алфавитно)

```yaml
# Базовая конфигурация
spec:
  priority: 30

# Override конфигурация (победит при конфликте)
spec:
  priority: 100
```

**Tie-breaker по имени:**
```yaml
# При priority=50 у обоих:
# "50-alpha" → content: "alpha"
# "50-beta"  → content: "beta"  ← победит ("beta" > "alpha")
```

---

### spec.files

Список файлов для управления на хосте.

```yaml
spec:
  files:
    - path: /etc/myapp/config.yaml
      content: |
        key: value
        nested:
          option: true
      mode: 420
      owner: "root:root"
      state: present
```

#### Поля FileSpec

| Поле | Тип | Обязательное | По умолчанию | Описание |
|------|-----|--------------|--------------|----------|
| `path` | string | **Да** | — | Абсолютный путь к файлу |
| `content` | string | При state=present | — | Содержимое файла |
| `mode` | int | Нет | 420 (0644) | Unix-права в decimal |
| `owner` | string | Нет | "root:root" | Владелец в формате user:group |
| `state` | enum | Нет | "present" | present или absent |

#### path

```yaml
# Правильно — абсолютный путь
path: /etc/myapp/config.conf

# Неправильно — относительный путь
path: etc/myapp/config.conf    # ❌ Ошибка валидации
```

#### content

```yaml
# Простое содержимое
content: "key=value"

# Многострочное (используйте |)
content: |
  [section]
  key1 = value1
  key2 = value2

# Максимальный размер: 1 MB (1048576 bytes)
```

#### mode

Права файла в **decimal** (не octal!):

| Octal | Decimal | Описание |
|-------|---------|----------|
| 0644 | 420 | rw-r--r-- |
| 0600 | 384 | rw------- |
| 0755 | 493 | rwxr-xr-x |
| 0700 | 448 | rwx------ |

```yaml
# Конфиг файл (читаемый всеми)
mode: 420    # 0644

# Приватный ключ
mode: 384    # 0600

# Исполняемый скрипт
mode: 493    # 0755
```

#### owner

```yaml
# По имени
owner: "root:root"
owner: "nginx:nginx"
owner: "myuser:mygroup"

# По UID:GID
owner: "0:0"
owner: "1000:1000"
```

#### state

```yaml
# Файл должен существовать (по умолчанию)
state: present

# Файл должен быть удалён
state: absent
```

> **Примечание:** При `state: absent` поле `content` игнорируется.

---

### spec.systemd

Управление systemd-юнитами.

```yaml
spec:
  systemd:
    units:
      - name: nginx.service
        enabled: true
        state: started
```

#### Поля UnitSpec

| Поле | Тип | Обязательное | По умолчанию | Описание |
|------|-----|--------------|--------------|----------|
| `name` | string | **Да** | — | Имя юнита с расширением |
| `enabled` | *bool | Нет | nil | Запускать при загрузке |
| `state` | enum | Нет | — | Текущее состояние |
| `mask` | bool | Нет | false | Полностью заблокировать юнит |

#### name

Поддерживаемые типы юнитов:
- `.service` — сервисы
- `.socket` — сокеты
- `.timer` — таймеры
- `.mount` — точки монтирования
- `.target` — цели

#### enabled

```yaml
# Включить автозапуск
enabled: true     # systemctl enable

# Отключить автозапуск
enabled: false    # systemctl disable

# Не менять (по умолчанию)
enabled: null     # Пропустить enable/disable
```

#### state

| Значение | Команда | Описание |
|----------|---------|----------|
| `started` | systemctl start | Запустить сервис |
| `stopped` | systemctl stop | Остановить сервис |
| `restarted` | systemctl restart | Перезапустить |
| `reloaded` | systemctl reload | Перезагрузить конфиг |

#### mask

```yaml
# Полностью заблокировать юнит
mask: true    # systemctl mask

# Юнит нельзя запустить никак
# Даже через systemctl start
```

---

### spec.reboot

Требования к перезагрузке.

```yaml
spec:
  reboot:
    required: true
    reason: "Kernel parameters require reboot to take effect"
```

| Поле | Тип | По умолчанию | Описание |
|------|-----|--------------|----------|
| `required` | bool | false | Нужна ли перезагрузка |
| `reason` | string | — | Причина (информационно) |

> **Примечание:** Перезагрузка произойдёт только если стратегия пула `IfRequired`.
> При стратегии `Never` устанавливается аннотация `reboot-pending=true`.

---

## Метки (Labels)

### Обязательная метка

```yaml
metadata:
  labels:
    mco.in-cloud.io/pool: worker    # Связывает MC с пулом
```

MachineConfig применяется к пулу через `machineConfigSelector` пула:

```yaml
# MachineConfigPool
spec:
  machineConfigSelector:
    matchLabels:
      mco.in-cloud.io/pool: worker    # ← Совпадает с меткой MC
```

### Дополнительные метки

```yaml
metadata:
  labels:
    mco.in-cloud.io/pool: worker
    app.kubernetes.io/component: networking    # Для фильтрации
    environment: production                     # Для фильтрации
```

---

## Примеры использования

### Создание конфигурационного файла

```yaml
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: app-config
  labels:
    mco.in-cloud.io/pool: worker
spec:
  priority: 50
  files:
    - path: /etc/myapp/config.yaml
      content: |
        server:
          port: 8080
          host: 0.0.0.0
        logging:
          level: info
      mode: 420
      owner: "root:root"
```

### Удаление файла

```yaml
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: remove-legacy-config
  labels:
    mco.in-cloud.io/pool: worker
spec:
  files:
    - path: /etc/legacy/old-config.conf
      state: absent
```

### Управление сервисом

```yaml
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: nginx-service
  labels:
    mco.in-cloud.io/pool: worker
spec:
  systemd:
    units:
      - name: nginx.service
        enabled: true
        state: started
```

### Отключение сервиса

```yaml
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: disable-cups
  labels:
    mco.in-cloud.io/pool: worker
spec:
  systemd:
    units:
      - name: cups.service
        enabled: false
        state: stopped
        mask: true    # Полностью заблокировать
```

### Конфигурация с перезагрузкой

```yaml
apiVersion: mco.in-cloud.io/v1alpha1
kind: MachineConfig
metadata:
  name: sysctl-network
  labels:
    mco.in-cloud.io/pool: worker
spec:
  priority: 40
  files:
    - path: /etc/sysctl.d/99-network.conf
      content: |
        net.ipv4.ip_forward = 1
        net.core.somaxconn = 65535
      mode: 420
  reboot:
    required: true
    reason: "Kernel parameters require reboot"
```

---

## Слияние MachineConfig

Когда несколько MC выбраны одним пулом, они сливаются:

### Правила слияния файлов

1. Файлы сортируются по `path`
2. При одинаковом `path` — побеждает MC с **большим** priority
3. При одинаковом priority — побеждает MC с **большим** именем (алфавитно)

```yaml
# MC "base" (priority: 30)
files:
  - path: /etc/app.conf
    content: "base config"

# MC "override" (priority: 50)
files:
  - path: /etc/app.conf
    content: "override config"    # ← Этот победит

# Результат в RMC:
files:
  - path: /etc/app.conf
    content: "override config"
```

### Правила слияния systemd

1. Юниты дедуплицируются по `name`
2. При конфликте — побеждает MC с большим priority

---

## Типичные ошибки

### Относительный путь

```yaml
# ❌ Ошибка
files:
  - path: etc/myapp/config.conf

# ✅ Правильно
files:
  - path: /etc/myapp/config.conf
```

### Octal вместо decimal для mode

```yaml
# ❌ Неправильно (будет интерпретировано как decimal 644!)
mode: 0644

# ✅ Правильно
mode: 420    # 0644 в decimal
```

### Отсутствие метки пула

```yaml
# ❌ MC не будет связан с пулом
metadata:
  name: my-config
  # labels отсутствует

# ✅ Правильно
metadata:
  name: my-config
  labels:
    mco.in-cloud.io/pool: worker
```

---

## Связанные документы

- [MachineConfigPool](machineconfigpool.md) — настройка пулов
- [Мониторинг статуса](status-monitoring.md) — отслеживание состояния
- [Примеры](../examples/README.md) — готовые примеры

