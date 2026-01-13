# E2E Тесты — Сквозное тестирование

End-to-End (E2E) тесты проверяют работу MCO Lite в реальном Kubernetes-кластере (Kind).

---

## Обзор

E2E тесты запускаются в Kind-кластере и проверяют:
- Применение файлов на ноды
- Rolling Update с maxUnavailable
- Cordon/Drain поведение
- Pool Overlap detection
- Условия ошибок (DrainStuck, Degraded)
- Debounce механизм
- История ревизий
- Controller resilience (tolerations, self-node skip)

---

## Запуск тестов

### Локальный запуск

```bash
# Запустить все E2E тесты
make test-e2e

# Запустить с сохранением кластера для отладки
E2E_SKIP_TEARDOWN=true make test-e2e

# Запустить конкретный тест
make test-e2e-focus FOCUS="File Apply"
```

### В CI/CD

```yaml
# GitHub Actions
- name: Run E2E tests
  run: make test-e2e
  timeout-minutes: 15
```

---

## Каталог тестов

### Файловые операции

#### E2E-101: File Apply

**Что проверяет:** Создание файла на всех worker-нодах.

**Сценарий:**
1. Создаётся MachineConfigPool для worker-нод
2. Создаётся MachineConfig с файлом `/etc/mco-test/e2e-apply.conf`
3. Ожидается convergence пула
4. Проверяется наличие файла на всех нодах через agent pod

**Почему важно:** Базовая функция MCO — применение файлов. Если этот тест падает, система не работает.

```yaml
# Пример MachineConfig
spec:
  files:
  - path: /etc/mco-test/e2e-apply.conf
    content: "e2e test content"
    mode: 0644
```

#### E2E-102: File Absent

**Что проверяет:** Удаление файла с нод при `state: absent`.

**Сценарий:**
1. Создаётся файл через MachineConfig
2. Модифицируется MC с `state: absent`
3. Проверяется что файл удалён

**Почему важно:** MCO должен уметь не только создавать, но и удалять файлы.

---

### Инварианты нод

#### E2E-103: Node Invariants After Rollout

**Что проверяет:** Конвергенция аннотаций нод после завершения rollout.

**Ожидаемые инварианты:**
- `desired-revision` == `current-revision`
- `agent-state` == `done`
- `unschedulable` == false (uncordoned)
- `cordoned` annotation отсутствует
- `drain-started-at` annotation отсутствует
- `drain-retry-count` annotation отсутствует

**Почему важно:** После успешного обновления система должна прийти в "чистое" состояние.

---

### Перезагрузки

#### E2E-104: Reboot Pending

**Что проверяет:** Аннотация `reboot-pending` при `strategy=Never`.

**Сценарий:**
1. Создаётся MCP с `reboot.strategy: Never`
2. Создаётся MC с `reboot.required: true`
3. После применения проверяется `mco.in-cloud.io/reboot-pending=true`

**Почему важно:** При стратегии Never MCO не должен перезагружать ноды, но должен сигнализировать о необходимости.

---

### Пауза пула

#### E2E-105: Pool Paused Blocks Rollout

**Что проверяет:** Пауза пула блокирует раскатку.

**Сценарий:**
1. Создаётся MCP с `paused: true`
2. Создаётся новый MC
3. Проверяется что `desired-revision` НЕ устанавливается на ноды
4. Снимается пауза
5. Проверяется что rollout продолжается

**Почему важно:** Paused даёт возможность ручного контроля над раскаткой.

---

### История ревизий

#### E2E-106: Revision History Limit

**Что проверяет:** Соблюдение `revisionHistory.limit`.

**Сценарий:**
1. Создаётся MCP с `revisionHistory.limit: 2`
2. Создаётся несколько MC, каждый генерирует новый RMC
3. Проверяется что старые RMC удаляются
4. Количество RMC не превышает limit + 1 (текущий)

**Почему важно:** Предотвращает накопление устаревших ресурсов.

---

### Rolling Update

#### Rolling Update с maxUnavailable=1

**Что проверяет:** Последовательное обновление нод.

**Сценарий:**
1. Создаётся MCP с `maxUnavailable: 1`
2. Создаётся MC, триггерящий update
3. Проверяется что не более 1 ноды в состоянии updating одновременно
4. Проверяется что все ноды обновились

**Почему важно:** maxUnavailable=1 — самая безопасная стратегия для production.

#### Rolling Update с maxUnavailable=2

**Что проверяет:** Параллельное обновление нескольких нод.

**Сценарий:**
1. Создаётся MCP с `maxUnavailable: 2`
2. Создаётся MC
3. Проверяется что до 2 нод обновляются одновременно

**Почему важно:** Подтверждает корректность подсчёта unavailable нод.

---

### Debounce

#### E2E-108: Debounce Limits RMC Spam

**Что проверяет:** Debounce предотвращает множественные RMC.

**Сценарий:**
1. Создаётся MCP с `debounceSeconds: 3`
2. Быстро применяются 3 изменения MC (в течение 1 секунды)
3. Ожидается debounce + buffer
4. Проверяется что создан только 1 новый RMC (не 3)

**Почему важно:** Debounce экономит ресурсы при пакетных изменениях.

---

### Pool Overlap

#### Pool Overlap Detection

**Что проверяет:** Обнаружение нод в нескольких пулах.

**Сценарий:**
1. Создаётся нода с двумя ролями
2. Создаётся MCP-1 selecting role=worker
3. Создаётся MCP-2 selecting role=infra
4. Нода матчит оба пула
5. Проверяется condition `PoolOverlap=True` на обоих пулах

**Почему важно:** Overlap может привести к непредсказуемому поведению.

#### No Overlap When Pools Don't Overlap

**Что проверяет:** Отсутствие ложных срабатываний.

**Сценарий:**
1. Создаются два пула с непересекающимися селекторами
2. Проверяется `PoolOverlap=False` или отсутствие condition

---

### Drain Stuck

#### Drain Stuck Without Blocking

**Что проверяет:** Condition DrainStuck при превышении timeout.

**Сценарий:**
1. Создаётся Deployment + PDB блокирующий эвакуацию
2. Создаётся MCP с коротким `drainTimeoutSeconds`
3. Триггерится update
4. Ожидается timeout
5. Проверяется condition `DrainStuck=True`
6. Проверяется что drain **продолжает попытки**

**Почему важно:** DrainStuck — важный сигнал для мониторинга, но не должен блокировать систему.

---

## Матрица тестов

| Тест | Компонент | Критичность | Время |
|------|-----------|-------------|-------|
| E2E-101 | Agent (files) | Critical | ~30s |
| E2E-102 | Agent (files) | High | ~30s |
| E2E-103 | Controller + Agent | High | ~60s |
| E2E-104 | Agent (reboot) | Medium | ~60s |
| E2E-105 | Controller (pause) | Medium | ~60s |
| E2E-106 | Controller (history) | Low | ~120s |
| Rolling maxUnavailable=1 | Controller | Critical | ~120s |
| Rolling maxUnavailable=2 | Controller | High | ~120s |
| E2E-108 | Controller (debounce) | Medium | ~30s |
| Pool Overlap | Controller | High | ~60s |
| Drain Stuck | Controller (drain) | High | ~120s |

---

## Отладка падающих тестов

### Сохранить кластер

```bash
E2E_SKIP_TEARDOWN=true make test-e2e-focus FOCUS="Failed Test"
```

### Проверить состояние

```bash
# Статус пулов
kubectl get mcp

# Статус нод
kubectl get nodes -o custom-columns=\
NAME:.metadata.name,\
STATE:.metadata.annotations.mco\.in-cloud\.io/agent-state,\
REV:.metadata.annotations.mco\.in-cloud\.io/current-revision

# Логи controller
kubectl logs -n mco-system deployment/mco-controller

# Логи agent
kubectl logs -n mco-system -l app=mco-agent
```

### Типичные причины падений

| Симптом | Причина | Решение |
|---------|---------|---------|
| Timeout при ожидании | Slow Kind node | Увеличить timeout |
| State не конвергирует | Leftover resources | Добавить cleanup в BeforeEach |
| Annotations не читаются | JSONPath syntax | Проверить escaping точек |
| Pod pending | Node cordoned | Проверить uncordon в cleanup |

---

## Добавление новых тестов

### Структура теста

```go
var _ = Describe("My Feature", func() {
    BeforeEach(func() {
        // Очистка ресурсов
        cleanupAllMCOResources()
    })

    Context("scenario description", func() {
        It("should do something", func() {
            By("creating resources")
            // ...

            By("verifying state")
            Eventually(func() (bool, error) {
                return checkCondition()
            }, 60*time.Second, 2*time.Second).Should(BeTrue())
        })
    })
})
```

### Best Practices

1. **Всегда очищайте ресурсы** в BeforeEach/AfterEach
2. **Используйте Eventually** для асинхронных проверок
3. **Логируйте шаги** через `By("description")`
4. **Используйте DeferCleanup** для гарантированной очистки
5. **Не полагайтесь на порядок тестов** — каждый тест независим

---

## Связанные документы

- [Сценарии проверки](verification-scenarios.md) — ручная проверка
- [Устранение проблем](../user-guide/troubleshooting.md) — диагностика

