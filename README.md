# Сервис «Сейчас ищут»

Сервис считает самые частые поисковые запросы за последние 5 минут. Данные приходят из Kafka. На выходе есть API и метрики.

# Ссылка: ```https://github.com/leonidlivshits/WB-Trial-Task```

## 1. Локальный запуск и примеры API

### Требования
- Docker
- Docker Compose

### Запуск
1. Создайте файл окружения:

```powershell
Copy-Item .env.example .env
```

2. Заполните переменные в `.env`:
- `POSTGRES_PASSWORD`
- `HTTP_ADMIN_TOKEN`
- `STORAGE_STOPLIST_POSTGRES_DSN`

3. Запустите сервис:

```powershell
docker compose up -d --build
```

### Проверка
- Swagger UI: `http://localhost:8080/swagger/`
- OpenAPI: `http://localhost:8080/openapi.yaml`
- Health: `http://localhost:8080/api/v1/health`
- Top: `http://localhost:8080/api/v1/top?n=10`
- Metrics: `http://localhost:8080/metrics`

### Примеры запросов

Публичные ручки:

```bash
curl http://localhost:8080/api/v1/health
curl "http://localhost:8080/api/v1/top?n=10"
curl http://localhost:8080/metrics
```

Админ-ручки (PowerShell):

```powershell
$h = @{ "X-Admin-Token" = "<admin-token>" }

Invoke-RestMethod -Method Get -Uri "http://localhost:8080/api/v1/stop-words" -Headers $h

Invoke-RestMethod -Method Post -Uri "http://localhost:8080/api/v1/stop-words" `
  -Headers @{ "X-Admin-Token"="<admin-token>"; "Content-Type"="application/json" } `
  -Body '{"word":"бесплатно"}'

Invoke-RestMethod -Method Delete -Uri "http://localhost:8080/api/v1/stop-words/бесплатно" -Headers $h
```

Примечание по Swagger: в `Authorize` нужно вводить только значение токена `dev-admin-token`.

## 2. Контракт данных из брокера

Основные файлы:
- `docs/contracts/search_event.v1.md`
- `docs/schemas/search_event.v1.schema.json`
- `docs/contracts/examples/valid/01_authenticated_full.json`

Пример сообщения:

```json
{
  "schema_version": 1,
  "event_type": "search.query.submitted",
  "event_id": "0196b4d0-5f0f-7c2f-a5b4-8c4c9b9f3a10",
  "ingested_at_ms": 1779656405123,
  "query_raw": "Купить   айфон !",
  "source": "search_bar",
  "ip_hash": "ip_7c9f4dA1b2",
  "session_id_hash": "s_92ab10QwEr",
  "auth_state": "authenticated",
  "user_id_hash": "u_18ac0ePzRt"
}
```

Почему нужны эти поля:
- `event_id`: убрать повторно доставленные события.
- `ingested_at_ms`: считать окно 5 минут.
- `query_raw`: исходная строка для нормализации.
- `ip_hash`, `session_id_hash`, `user_id_hash`: ограничить накрутку.
- `auth_state`: правило обязательности `user_id_hash`.
- `source`, `platform`, `app_version`, `producer_region`: разбор всплесков и диагностика.
- `is_test_traffic`, `is_replay`: отделить тесты и повторный прогон от онлайн-трафика.
- `normalizer_version`: понимать, какой версией правил обработан запрос.

## 3. Почему такая архитектура

### Почему выбран Kafka + in-memory агрегатор
- Низкая задержка на `GET /api/v1/top`.
- Kafka дает надежную доставку и возможность дочитать события после перезапуска.

### Как хранится топ
- Кольцевой буфер на 300 секунд.
- Отдельный словарь общих счетов по запросам.
- Готовый срез для чтения.

Причина выбора:
- запись быстрая;
- чтение быстрое;
- поведение легко тестировать.

### Как хранится стоп-лист
- PostgreSQL (`stop_words`, `stop_list_state`).

Причина выбора:
- данные не теряются после перезапуска;
- список можно менять через API;

## 4. Компромиссы и бизнес-решения

### Компромиссы
- Смещение в Kafka сохраняется после обработки события.
  - Плюс: событие не теряется.
  - Минус: после перезапуска возможны повторы; они убираются по `event_id`.
- Топ считается в памяти.
  - Плюс: очень быстрое чтение.
  - Минус: после рестарта нужно дочитать поток из Kafka.
- Режим защиты от накрутки: если не уверены - не учитываем.
  - Плюс: меньше мусора в топе.
  - Минус: возможны редкие ложные отклонения.

### Неоднозначности в постановке и принятые решения
1. Что считать одинаковым запросом  
Решение: нормализация строки (регистр, пробелы, лишние символы, стоп-слова).

2. По какому времени считать окно  
Решение: по `ingested_at_ms` (время приема сервисом).

3. Что делать с повторами доставки  
Решение: повтор по `event_id` не влияет на результат.

4. Как ограничить вклад одного источника  
Решение: источник не может бесконечно увеличивать один и тот же запрос в пределах окна/лимита.

## Тесты

```bash
go test ./...
go test -tags=integration ./internal/adapters/storage/postgres
go test -run ^$ -bench BenchmarkGetTopReadHeavy -benchmem ./internal/adapters/httpapi
```

## Проверки в GitHub Actions

Workflow: `.github/workflows/ci.yml`
- unit тесты;
- integration тесты с PostgreSQL;
- замер скорости чтения `/api/v1/top` с сохранением файла отчета.
