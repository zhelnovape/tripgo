# TripGo — репозиторий для лабораторных работ

Учебный проект курса «Разработка микросервисов на Go».

## Инструкция запуска

### Требования

| Инструмент | Назначение |
| :-- | :-- |
| **Go 1.27+** | Среда выполнения |
| **Docker** | Контейнеризация |
| **tripgoctl** | Внутренний СLI |
| **golangci-lint v2.12.2** | Линтер и форматтеры. Отдельно ставить не нужно: `make lint` запускает именно эту версию через `go run` (`GOLANGCI_LINT_VERSION` в `Makefile`) |

Локальные интеграции: ``curl``, ``jq``, ``psql``, ``goose`` и ``shasum``.

```bash
# Установка CLI
curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL \
  https://github.com/course-go-autumn-2026/course-infra/releases/latest/download/install-tripgoctl \
  | sh
```

### Команды запуска

```bash
# Первичный запуск (Один раз при настройке)
tripgoctl cluster start

# Подготовка окружения
tripgoctl environment start && make migrate

# Запуск микросервиса
make run

# Вывод адресов и ссылок (Jaeger, Grafana, Prometheus и Console)
tripgoctl connect
```

### Тесты, линтер и генерация кода

```bash
# Линтер
make lint

# Unit-тесты с -race и покрытие пакетов domain и usecase
make test

# Интеграционные тесты репозитория (нужна база с миграциями)
tripgoctl environment start && make migrate
make test-integration

# Код по OpenAPI и моки
make generate
```

`make generate` перегенерирует код по OpenAPI и запускает `go generate ./...`: моки usecase'ов и ручек пересоздаются из директив `//go:generate` в их `contract.go`.

`make test` запускает все unit-тесты с `-race`, затем отдельно считает покрытие `internal/domain` и `internal/usecase` и печатает итог последней строкой.

Интеграционные тесты помечены тегом `//go:build integration`, поэтому `make test` их не запускает. Адрес базы они берут из `TEST_DATABASE_URL`, а если его нет — из `DATABASE_URL` (Makefile подхватывает его из `.env`). Каждый тест сам создаёт свои поездки и удаляет их после себя, поэтому повторный запуск тоже проходит.

### Проверка телеметрии

```bash
# Ссылки на Jaeger и Grafana
tripgoctl connect

# Логи сервиса в JSON: у каждой записи внутри запроса есть trace_id и span_id
make run 2>&1 | tee service.log
```

- Jaeger: сервис `trip-service`, трейсы называются по шаблону маршрута, например `POST /api/v1/trips`.
- Grafana → Explore → Prometheus: метрики `http_server_request_duration_seconds`, `http_server_active_requests`, `db_client_operation_duration_seconds`, `tripgo_trips_created_total`, `tripgo_trips_completed_total`.
- С работы 3: `tripgo_positions_saved_total`, `tripgo_stale_checks_total{result}`, `tripgo_position_requests_total{result}`, `tripgo_position_requests_duration_seconds`, `tripgo_position_requests_queue_size`, `tripgo_position_requests_workers_active`.
- Jaeger: спан `positions.scan` на каждый тик и спан `positions.request_position` на каждую задачу. Задача связана со сканом через span link, внутри неё — спан `POST /api/v1/push`.
- Логи заглушки пушей: `tripgoctl environment logs push-service`.

### Список env-переменных

| Переменная | Назначение |
| :-- | :-- |
| ``HTTP_ADDR`` | Хост и порт для запуска |
| ``HTTP_READ_TIMEOUT`` | Макс. время чтения всего запроса |
| ``HTTP_WRITE_TIMEOUT`` | Макс. время записи ответа клиенту |
| ``HTTP_READ_HEADER_TIMEOUT``| Время чтения заголовков запроса |
| ``HTTP_IDLE_TIMEOUT`` | Время ожидания Keep-alive соединения |
| ``DATABASE_URL`` | DSN-ссылка для подключения к БД |
| ``DATABASE_MAX_CONNS`` | Макс. число соединений в пуле |
| ``DATABASE_MIN_CONNS`` | Мин. число соединений в пуле |
| ``DATABASE_MAX_CONN_LIFETIME`` | Макс. время жизни соединения |
| ``DATABASE_CONNECT_TIMEOUT`` | Таймаут на установку каждого соединения с БД (старт и новые соединения пула) |
| ``DATABASE_QUERY_TIMEOUT`` | Таймаут на выполнение одного SQL-запроса |
| ``SHUTDOWN_TIMEOUT``| Общий бюджет на остановку процесса: HTTP-сервер, фоновый опрос координат, закрытие клиентов, пула БД и телеметрии |
| ``LOG_LEVEL`` | Уровень логирования (`debug`, `info`, `warn`, `error`) |
| ``OTEL_SERVICE_NAME`` | Имя сервиса в телеметрии (`trip-service`) |
| ``OTEL_EXPORTER_OTLP_ENDPOINT`` | Адрес коллектора OpenTelemetry |
| ``OTEL_EXPORTER_OTLP_PROTOCOL`` | Протокол экспорта: `http/protobuf` или `grpc` |
| ``OTEL_RESOURCE_ATTRIBUTES`` | Общие атрибуты: `service.namespace`, `deployment.environment` |
| ``OTEL_TRACES_SAMPLER`` | Какие трейсы сохранять (`parentbased_always_on`) |
| ``OTEL_METRIC_EXPORT_INTERVAL`` | Как часто отправлять метрики, в миллисекундах |
| ``POSITION_CHECK_INTERVAL`` | Период тикера сканера и пауза между повторными пушами по одной поездке, по умолчанию `30s` |
| ``POSITION_STALE_AFTER`` | Сколько поездка может молчать, прежде чем попадёт в опрос, по умолчанию `60s` |
| ``POSITION_WORKERS`` | Число воркеров пула, по умолчанию `4` |
| ``POSITION_QUEUE_SIZE`` | Ёмкость очереди запросов координат, по умолчанию `100` |
| ``PUSH_HTTP_URL`` | Адрес Push Service, обязательная; записывает `tripgoctl` в `.env` |
| ``PUSH_ADMIN_URL`` | Админка заглушки Push Service, нужна только для ручной проверки |
| ``PUSH_REQUEST_TIMEOUT`` | Таймаут одного запроса в Push Service, по умолчанию `1s` |
| ``TEST_DATABASE_URL`` | Необязательная: база для интеграционных тестов, иначе берётся `DATABASE_URL` |

Переменные `OTEL_*`, кроме `OTEL_METRIC_EXPORT_INTERVAL`, записывает в `.env` сам `tripgoctl`. `OTEL_METRIC_EXPORT_INTERVAL` задана в `environment.toml`, чтобы метрики уходили раз в 5 секунд, а не раз в минуту.

## Архитектура

### Слои

```
cmd/service/main.go          читает конфиг, вызывает container.New, запускает сервер, ждёт сигнала
internal/
  config/                    чтение и проверка переменных окружения
  container/                 создаёт все зависимости и связывает их — единственное такое место
  domain/trip/               поездка: статусы, правила, переход в completed, ошибки домена
  model/                     данные запросов (CreateTrip, SavePosition) и StaleTrip для сканера
  usecase/trip/              сценарии: create, get, finish
  usecase/position/          сценарии: save, list
  http/                      транспорт
    trip/create|get|finish/  ручки, по пакету на ручку
    position/save|list/      ручки координат
    request/                 строгий разбор JSON-тела и tripId из пути
    domainerror/             перевод ошибки в HTTP-ответ — одно место на весь сервис
    middleware/              Recover → Tracing → Logging → Metrics
    health/, ready/          служебные ручки
    response/                запись JSON-ответов
  repository/trip/           SQL для таблиц trips и trip_status_history, выборка поездок для опроса
  repository/position/       SQL для таблицы trip_positions
  worker/positions/          сканер, очередь и пул воркеров опроса устаревших поездок
  gateway/push/              HTTP-клиент Push Service
  infra/
    postgres/                пул pgx и трейсер запросов
    postgres/transaction/    менеджер транзакций
    telemetry/               OpenTelemetry и логгер slog
    clock/                   текущее время
  generated/api/             код, сгенерированный по OpenAPI
```

### Кто от кого зависит

Стрелка значит «импортирует». Зависимости идут внутрь, к домену:

```
                 cmd/service
                      │
               internal/container
                      │
   ┌──────────┬───────┴───────┬──────────────┐
   ▼          ▼               ▼              ▼
  http     usecase        repository       infra
   │          │               │
   └──────────┼───────────────┘
              ▼
       model, domain
```

| Пакет | Что импортирует | Что ему нельзя |
| :-- | :-- | :-- |
| `domain` | стандартную библиотеку и `uuid` | `net/http`, `chi`, `pgx`, `squirrel`, `otel`, `api` |
| `usecase` | `domain`, `model`, `slog`, `otel/metric` | `net/http`, `chi`, `pgx`, `squirrel`, `api` |
| `http` | `domain`, `model`, `api`, `chi`, `otel` | `repository`, `infra` |
| `repository` | `domain`, `model`, `pgx`, `squirrel`, менеджер транзакций | — |
| `worker` | `model`, `otel` | `repository`, `gateway` — только через интерфейсы |
| `gateway` | `net/http`, `otel` | `domain`, `usecase` |
| `container` | всё | — |

Запреты для `domain` и `usecase` проверяет `depguard` в `.golangci.yml`, поэтому нарушение ломает `make lint`.

### Где объявлены интерфейсы и почему там

Интерфейсы лежат в `contract.go` того пакета, который их использует, и они неэкспортируемые:

| Где | Интерфейсы |
| :-- | :-- |
| `usecase/trip/create`, `usecase/trip/finish` | `tripRepository`, `txManager`, `clock` |
| `usecase/trip/get` | `tripRepository` |
| `usecase/position/save` | `tripRepository`, `positionRepository`, `positionRequestCanceller`, `txManager`, `clock` |
| `usecase/position/list` | `tripRepository`, `positionRepository` |
| `http/position/save`, `list` | `positionSaver`, `positionLister` |
| `worker/positions` | `staleTripSource` — выборка из бд, `pusher` — гейтвей Push Service |
| `http/trip/create`, `get`, `finish` | `tripCreator`, `tripGetter`, `tripFinisher` — нужный ручке usecase |
| `http/ready` | `pinger` — проверка базы |
| `repository/trip` | `querier` — пул или открытая транзакция |

Почему у потребителя:

1. Usecase сам говорит, что ему нужно. Интерфейс узкий: в нём только те методы, которые этот usecase вызывает. Такой интерфейс легко замокать.
2. Реализация ничего не знает о том, кто её использует. Репозиторий просто имеет методы `Save`, `Get`, `Finish`, а Go сам проверит, что он подходит под интерфейс.
3. Usecase не импортирует ни репозиторий, ни драйвер базы. Поэтому в тестах вместо них подставляются моки.
4. В лабораторной работе 4 появится gRPC. Для него заводятся пакеты `internal/grpc/trip/create` и т. д. со своим `contract.go`, а usecase'ы не меняются: им всё равно, кто их вызывает.

Моки создаёт `mockgen` в `mocks_test.go` рядом с `contract.go`, отдельного пакета `mocks/` нет.

### Отступления от раскладки из задания

- `internal/generated/api/` вместо `internal/generated/`: у пакета своё имя `api`, а в работе 4 рядом появится код из `.proto`.
- `http/response`, `http/health`, `http/ready` — пакетов нет в схеме задания. Запись JSON нужна всем ручкам, а `health` и `ready` — тоже ручки, поэтому у каждой свой пакет.
- `http/trip/convert.go` — перевод поездки в ответ API общий для трёх ручек, чтобы не копировать его три раза.
- `cmd/service/` взят из задания ЛР2; в `conventions.md` написано `cmd/trip-service/`.
- В репозитории файлы разделены так: `save.go` — все записи (`Save`, `Finish`, `SaveStatusChange`), `fetch.go` — чтение.

## Лабораторная работа 1. HTTP API и PostgreSQL

В лабораторной работе 1 сделан работающий сервис: поднят веб-сервер, накачены миграции, запись и чтение данных через pgx, реализован свой менеджер транзакций.

### Решения

**Менеджер транзакций**

Менеджер транзакций (`internal/trip/tx_manager.go`) – это интерфейс `TxManager` с одним методом:

```go
Do(ctx context.Context, fn func(ctx context.Context) error) error
```

Как он работает:

1. `Do` открывает транзакцию через `pool.BeginTx` и кладёт её в `context`. Если `fn` вернула ошибку или паникнула, происходит `Rollback` (паника пробрасывается дальше), иначе – `Commit`.
2. Вложенный `Do`: если в `context` уже лежит транзакция, `Do` не открывает новую, а просто вызывает `fn` с тем же контекстом. `Commit`/`Rollback` делает только внешний `Do`, поэтому ошибка во вложенном вызове откатывает всё целиком.
3. `Rollback` выполняется с `context.WithoutCancel(ctx)`, чтобы он прошёл, даже если клиент уже отменил запрос.
4. На `Begin`, `Commit` и `Rollback` стоит таймаут `DATABASE_QUERY_TIMEOUT`, чтобы сервис не зависал, когда БД не отвечает.
5. Репозиторий сам про транзакции ничего не знает. Метод `executor(ctx, pool)` проверяет контекст: если есть транзакция, запрос идёт в неё, если нет — напрямую в пул.

В транзакции выполняются две операции сервиса:

- `CreateTrip` — создание поездки в `trips` + запись в `trip_status_history`.
- `FinishTrip` — перевод поездки в статус `completed` + запись в историю.

`GetTrip` — это одно чтение, поэтому транзакция ему не нужна.

**Уровень изоляции – Read Committed**

`Read Committed` – стандартный для PostgreSQL уровень изоляции.

Почему его достаточно:

- Транзакции короткие: два запроса, каждый работает с одной строкой.
- От гонок защищают сами механизмы БД: уникальный индекс (см. ниже) и условие `UPDATE ... WHERE id = $1 AND status = 'active'`.
- Повтор транзакции: более строгие уровни (`Repeatable Read`, `Serializable`) при конфликтах возвращают ошибку сериализации (`40001`).

**Запрет двух активных поездок – частичный уникальный индекс**

Запрет сделан на уровне БД, в миграции `00001_create_trips.sql`:

```sql
CREATE UNIQUE INDEX trips_active_driver_idx ON trips (driver_id) WHERE status = 'active';
```

Индекс учитывает только активные поездки. Когда поездка завершается, она выпадает из индекса, и водитель снова может взять заказ.

Если водитель уже занят, `INSERT` падает с ошибкой `23505` (`unique_violation`). 

Функция `isActiveDriverViolation` в `repository.go` проверяет код ошибки и имя индекса и превращает её в `ErrDriverBusy`, а хендлер отдаёт `409 Conflict` с кодом `driver_busy`. Транзакция при этом откатывается, поэтому лишняя запись в историю тоже не появляется.

## Лабораторная работа 2. Архитектура, качество кода и observability

Новых ручек и миграций нет. Код разложен по слоям, добавлены доменная модель, тесты, линтер, логи, метрики и трейсы.

Код из работы 1 переехал: менеджер транзакций — в `internal/infra/postgres/transaction`, проверка `isActiveDriverViolation` — в `internal/repository/trip/convert.go`, функция `executor` стала методом `conn` репозитория. Уровень Read Committed теперь задаётся внутри менеджера транзакций.

### Решения

**Сборка зависимостей – `container`**

`container.New` создаёт логгер, телеметрию, пул базы, репозиторий, usecase'ы, ручки и роутер и связывает их между собой. Больше нигде объекты не создаются, а `main` только запускает сервер и ждёт сигнала. DI-фреймворка нет, всё собрано вручную.

**Доменная модель – `domain/trip`**

- `trip.New(...)` создаёт поездку в статусе `active` и сразу проверяет её через `Validate()`: id не пустые, координаты в границах, цена не отрицательная, `active` без `finished_at`, `completed` с ним.
- `Finish(at)` переводит поездку в `completed`. Если она уже завершена, возвращается `ErrTripCompleted`. Метод не ходит в базу, поэтому проверяется обычным тестом.
- Ошибки домена `ErrTripNotFound`, `ErrTripCompleted`, `ErrDriverBusy` лежат в `domain/trip/errors.go`. Для ошибок валидации есть тип `ValidationError`, в нём поле и причина.

**Завершение поездки**

Usecase читает поездку, вызывает доменный `Finish`, затем репозиторий делает `UPDATE ... WHERE id = $1 AND status = 'active'`. Если два запроса пришли одновременно, второй `UPDATE` не найдёт активную поездку и вернёт `ErrTripCompleted`. Защита от гонки та же, что в работе 1.

**Единый маппинг ошибок – `http/domainerror`**

- `Problem(err)` — единственный `switch` по ошибкам: `ValidationError` → 400, `ErrTripNotFound` → 404, `ErrTripCompleted` и `ErrDriverBusy` → 409, всё остальное → 500.
- Ручка не разбирает ошибку, а сразу отдаёт её в `domainerror.Write`.
- Для 500 клиент видит только `internal_error`, а текст ошибки пишется в лог — один раз, в этом месте.
- Ошибки разбора JSON и неверный `tripId` тоже оформляются как `ValidationError`, поэтому все ошибки идут через один маппер. Ручка создания дополнительно отклоняет тело, в котором нет обязательного поля контракта, поле равно `null`, после JSON-объекта есть ещё данные или есть ключ, отличающийся от поля контракта только регистром (`PRICE`, `Latitude`): `encoding/json` сопоставляет ключи без учёта регистра, и такой ключ перезаписал бы значение.

**Время – интерфейс `clock`**

Usecase'ы не вызывают `time.Now()` сами, время приходит через интерфейс. В тестах его подменяет мок, поэтому можно проверить точное время старта и завершения.

**Тесты**

| Что | Как |
| :-- | :-- |
| домен | табличные тесты без моков |
| usecase'ы | моки `gomock` для репозитория, менеджера транзакций и `clock` |
| маппинг ошибок | табличный тест: ошибка → статус и `code`, для 500 нет деталей |
| ручки | `httptest` и мок usecase'а, в том числе тело без обязательных полей, с `null`, с данными после JSON и с ключами в другом регистре |
| middleware и логгер | имя спана и `http.route` — шаблон, уровень лога, метрики, `Recover`; при панике — запись `request handled`, статус спана `Error` с `error.type` и замер длительности с кодом 500 |
| репозиторий | реальный PostgreSQL, тег `integration`, есть тест на откат обеих таблиц и на вложенный `Do` |

Покрытие `domain` и `usecase` — 100%.

**Линтер**

`.golangci.yml` скопирован из материалов курса без изменений. Что пришлось поправить в коде:

- `main` разделён на `main` и `run`, иначе `log.Fatalf` срабатывал бы после `defer` (`gocritic: exitAfterDefer`).
- Переводы чисел в `int32` сделаны без риска переполнения (`gosec G115`).
- В лог больше не попадают данные запроса через `log.Printf` (`gosec G706`, подделка записей в логе).
- Повторяющиеся строки в репозитории и строка `UNKNOWN` в `infra/postgres/tracer.go` вынесены в константы (`goconst`).
- Срез атрибутов в middleware `Metrics` создаётся сразу с нужной ёмкостью (`prealloc`).

Версия линтера зафиксирована (`v2.12.2`): в разных версиях набор замечаний отличается, поэтому `make lint` запускает именно её через `go run`.

`nolint` в коде нет.

**Логи – `slog`**

- JSON в stdout, уровень из `LOG_LEVEL`, в каждой записи поле `service`.
- `trace_id` и `span_id` добавляет обёртка над обработчиком `slog` (`infra/telemetry/logger.go`): если в контексте есть спан, она берёт id из него.
- Логгер передаётся зависимостью, `slog.Default()` в коде нет.
- Middleware пишет одну запись `request handled` на запрос: метод, шаблон маршрута, статус, время. Для 5xx уровень `Error`, для остального `Info`. Тела запроса и ответа не пишутся.
- Запись делается в `defer`, поэтому появляется и при панике в обработчике: статус `500`, уровень `Error`. Паника при этом летит дальше в `Recover`.
- `trip created` и `trip completed` пишут usecase'ы, а не middleware.

**Трейсы**

- Корневой спан создаёт `otelhttp`. Его имя — `{метод} {шаблон маршрута}`, а шаблон берётся у роутера chi через `router.Find`. Так в имени и в `http.route` стоит `/api/v1/trips/{tripId}`, а не конкретный id.
- Спаны запросов к базе создаёт `otelpgx` на уровне пула, а не в каждом методе репозитория. Небольшая обёртка (`infra/postgres/tracer.go`) добавляет `db.operation.name`, `db.collection.name` и `error.type`, потому что сам `otelpgx` не ставит `db.collection.name`.
- Ошибка помечает спан статусом `Error` и записывает `error.type`. Для базы это код PostgreSQL, например `23505`, для HTTP — код 5xx.
- Статус спана выставляется в `defer`, поэтому при панике в обработчике спан тоже получает `Error` и `error.type = 500`, хотя ответ отправляет `Recover` уже после закрытия спана.

**Метрики**

- `http.server.request.duration` и `http.server.active_requests` пишет свой middleware `Metrics`. Метрики `otelhttp` выключены, потому что он не знает шаблон маршрута chi.
- Замер длительности тоже делается в `defer`: при панике он записывается с кодом `500`, и такой запрос учитывается в доле ошибок.
- `db.client.operation.duration` пишет та же обёртка над `otelpgx`.
- `tripgo.trips.created` и `tripgo.trips.completed` увеличивают usecase'ы после успешной транзакции.
- В `http.route` только шаблон: если положить туда id, на каждую поездку появится своя серия, и метрика перестанет складываться.

**Коллектор недоступен**

Экспорт идёт в фоне, поэтому сервис работает как обычно, а ошибки экспорта пишутся в лог как `Warn`. При остановке сервис пробует отправить последние данные. Если коллектора нет, он пишет `Warn` и всё равно завершается нормально.


## Лабораторная работа 3. Координаты, очередь и worker pool

Добавлены приём координат, маршрут поездки и фоновый опрос поездок, по которым давно не было координат.

```bash
# Проверка вручную
tripgoctl environment start && make migrate && make run

# Создать поездку и не слать координаты: через POSITION_STALE_AFTER в логах заглушки
# появится REQUEST_POSITION, дальше — не чаще раза в POSITION_CHECK_INTERVAL
tripgoctl environment logs push-service

# Слать координаты каждые 5 секунд — команд по этой поездке быть не должно
while true; do
  curl -sS -X POST localhost:8080/api/v1/trips/$TRIP_ID/positions \
    -H 'content-type: application/json' \
    -d "{\"coordinates\":{\"latitude\":59.93,\"longitude\":30.33},\"recorded_at\":\"$(date -u +%FT%TZ)\"}" > /dev/null
  sleep 5
done

# Медленный Push Service: сканер продолжает тикать, ручки отвечают быстро
curl -fsS -X POST "$PUSH_ADMIN_URL/admin/behaviour" -H 'content-type: application/json' -d '{"latency_ms":5000}'
curl -fsS -X POST "$PUSH_ADMIN_URL/admin/behaviour" -H 'content-type: application/json' -d '{"latency_ms":0}'
```

### Ручки координат

| Ручка | Ответ | Ошибки |
| :-- | :-- | :-- |
| `POST /api/v1/trips/{tripId}/positions` | `201` и сохранённая точка | `400 invalid_request` — координаты вне диапазона, `recorded_at` не разобрался или позже текущего времени больше чем на минуту; `404 trip_not_found`; `409 trip_completed` |
| `GET /api/v1/trips/{tripId}/positions` | `200` и `{"positions": [...]}` с сортировкой по `recorded_at`, затем по `id` | `400 invalid_request` — неверный `tripId`; `404 trip_not_found` |

Тело `POST` разбирается так же строго, как у создания поездки: пропущенные обязательные поля, `null`, лишние ключи и ключи в другом регистре дают `400`. Общий код вынесен в `http/request`. Пустой маршрут отдаётся как `{"positions": []}`, а не `null`.

### Миграции

- `00003_add_trip_positions.sql` — колонка `trips.last_position_at` и таблица `trip_positions` с индексом `(trip_id, recorded_at, id)` под выдачу маршрута.
- `00004_create_trips_stale_positions_idx.sql` — индекс под выборку поездок для опроса:

```sql
CREATE INDEX CONCURRENTLY trips_active_last_seen_idx
    ON trips ((COALESCE(last_position_at, started_at)), id)
    WHERE status = 'active';
```

- Индекс частичный: со временем почти все поездки завершены, а сканер их не читает.
- Он построен по тому же выражению `COALESCE(...)`, по которому запрос фильтрует и сортирует. Индекс по двум отдельным колонкам под такое условие не подходит.
- `id` вторым ключом даёт однозначный порядок при одинаковом времени.
- `CONCURRENTLY` не блокирует запись в `trips`, поэтому миграция идёт без транзакции (`-- +goose NO TRANSACTION`).

План запроса на 50 000 поездок — `Bitmap Index Scan on trips_active_last_seen_idx`, seq scan нет.

### Сохранение координаты

В одной транзакции через `TxManager`:

1. `Get` поездки — поездки нет → `404`;
2. `EnsureActive` в домене — поездка завершена → `409`;
3. `UPDATE trips SET last_position_at = GREATEST(last_position_at, $recorded_at) WHERE id = $id AND status = 'active'`;
4. `INSERT` в `trip_positions`.

Координаты приходят не по порядку, поэтому сравнение сделано в самом `UPDATE` через `GREATEST`: опоздавшая точка попадает в маршрут, но `last_position_at` не уменьшает. `GREATEST` пропускает `NULL`, поэтому первая координата тоже записывается. Если читать значение в Go и потом писать, два параллельных запроса перезапишут друг друга.

Условие `status = 'active'` в `UPDATE` закрывает гонку с завершением поездки: если её завершили между шагами 1 и 3, `UPDATE` не найдёт строку, клиент получит `409`, а транзакция откатится.

После коммита usecase снимает поездку с очереди опроса (`Cancel`) и увеличивает `tripgo.positions.saved`.

### Опрос устаревших поездок

```
              каждые POSITION_CHECK_INTERVAL
  +----------+     ListStale      +------------+
  |  сканер  | -----------------> | PostgreSQL |
  | 1 горут. | <----------------- +------------+
  +----------+   поездки, первыми самые давно молчащие
       |
       | Enqueue, без блокировки:
       |   уже в «сейчас в очереди» -> duplicate
       |   в канале есть место      -> queued
       |   канал полон              -> dropped + Warn
       v
  +------------------------------+      Cancel(tripId)
  | очередь: chan Task           | <--- POST /positions
  | ёмкость POSITION_QUEUE_SIZE  |      после коммита
  | + map «сейчас в очереди»     |
  +------------------------------+
       |
       v
  +------------------------------+   POST /api/v1/push, REQUEST_POSITION
  | POSITION_WORKERS воркеров    | ------------------------------------> Push Service
  | живут всё время работы       |   таймаут PUSH_REQUEST_TIMEOUT
  +------------------------------+
```

Сканер, очередь и воркеры — `internal/worker/positions`, клиент Push Service — `internal/gateway/push`.

**Выборка.** Один запрос `ListStale` в `repository/trip/stale.go`:

```sql
SELECT id, driver_id, COALESCE(last_position_at, started_at)
FROM trips
WHERE status = 'active'
  AND COALESCE(last_position_at, started_at) < now() - $1 * interval '1 microsecond'
ORDER BY COALESCE(last_position_at, started_at), id
```

Время считает Postgres через `now()`, в Go сравнения нет. Если координат не было, отсчёт идёт от `started_at`. Состояние поездок в памяти не хранится, поэтому после рестарта опрос продолжается сам со следующего тика.

**Сканер.** Одна горутина с тикером. Обход идёт синхронно в той же горутине, поэтому тики не накладываются: если обход затянулся, `time.Ticker` выбрасывает лишние тики, а не копит их. В очередь сканер пишет через `select` с `default`, поэтому медленный Push Service его не останавливает. Ошибка выборки пишется в лог, и сканер ждёт следующего тика.

**Очередь и дедупликация.** Буферизованный канал и `map[tripID]seq` под мьютексом (`queue.go`, метод `Enqueue`). Под одним локом `Enqueue` проверяет map, пишет в канал без блокировки и только при успехе добавляет поездку в map. Проверка и постановка атомарны, поэтому одна поездка не окажется в очереди дважды, даже если сканер и хендлер работают одновременно. Воркер удаляет поездку из map только после обработки. Поэтому тик, пришедший раньше, чем воркер закончил, получит `duplicate`.

**Отмена.** `Cancel` удаляет поездку из map. Задача остаётся в канале, но воркер перед вызовом проверяет, что поездка всё ещё в map и с тем же номером `seq`. Если нет, задача учитывается как `cancelled`, и в Push Service запрос не уходит. Номер нужен для случая «отменили, а следующий тик поставил поездку снова»: в канале тогда две задачи, и пуш уйдёт только по свежей.

**Пауза между пушами.** Отметку об отправке мы не храним. Если устройство не ответило, поездка снова попадёт в выборку на следующем тике, поэтому пауза между повторными пушами равна `POSITION_CHECK_INTERVAL`.

**Что будет, если очередь заполнена.** Поездка не ставится, растёт `tripgo.stale_checks{result="dropped"}`, в лог пишется `Warn` с `trip_id`. Поездка не теряется: она по-прежнему без свежих координат и попадёт в выборку на следующем тике. Память не растёт: в очереди не больше `POSITION_QUEUE_SIZE` задач, а горутин всегда `POSITION_WORKERS + 1`.

**Ошибки Push Service.** Одна попытка, без ретраев. Воркер пишет ошибку в лог как `Warn`, учитывает её в `tripgo.position_requests{result="error"}` и берёт следующую задачу. Гейтвей различает ошибку транспорта (`TransportError`: нет соединения, таймаут) и ответ с кодом `4xx`/`5xx` (`StatusError`). У `StatusError` есть метод `Temporary()` для `429` и `5xx` — пригодится для ретраев в работе 5.

**Гейтвей.** Интерфейс `pusher` объявлен у потребителя, в `worker/positions/contract.go`. Реализация — свой `http.Client` с `Timeout` и таймаутами транспорта, `http.DefaultClient` не используется. Тело ответа дочитывается и закрывается в `defer` при любом коде. Транспорт `otelhttp` создаёт отдельный спан вызова и передаёт `traceparent` в заголовках.

| Параметр | По умолчанию | Что делает |
| :-- | :-- | :-- |
| `POSITION_CHECK_INTERVAL` | `30s` | период тикера; он же пауза между повторными пушами по одной поездке |
| `POSITION_STALE_AFTER` | `60s` | сколько поездка может молчать до попадания в выборку |
| `POSITION_WORKERS` | `4` | сколько одновременных запросов в Push Service |
| `POSITION_QUEUE_SIZE` | `100` | ёмкость очереди; всё сверх неё за тик — `dropped` |
| `PUSH_HTTP_URL` | из `.env` | адрес Push Service, без него сервис не стартует |
| `PUSH_REQUEST_TIMEOUT` | `1s` | таймаут одной задачи и HTTP-клиента |

### Остановка

Порядок задан в `cmd/service/main.go`, функция `shutdown`:

1. `Server.Shutdown` — HTTP перестаёт принимать запросы и дожидается текущих;
2. `PositionsPoller.Stop` — сканер останавливается, очередь закрывается, воркеры дорабатывают оставшиеся задачи;
3. `Container.Close` — закрываются HTTP-клиент Push Service, пул БД и телеметрия.

Все шаги делят общий бюджет `SHUTDOWN_TIMEOUT`, и ошибка одного шага не отменяет следующие. Если на шаге 2 бюджет кончился, контекст воркеров отменяется: текущие запросы прерываются, а необработанные задачи остаются в канале. По-настоящему они не теряются: поездки по-прежнему без координат, и после рестарта их найдёт сканер.

Что сломается в обратном порядке:
- если закрыть пул БД раньше сервера, хендлер координат не допишет транзакцию;
- если остановить воркеры раньше сканера, сканер продолжит ставить задачи в очередь, которую никто не разбирает. А закрыть канал, пока в него пишут, нельзя: это паника.

Фоновые горутины работают на своём контексте жизни приложения (`context.WithoutCancel` от сигнального), а не на контексте запроса или сигнала. Поэтому по `SIGTERM` они не обрываются сразу, а останавливаются в свою очередь.

### Решения

**Почему не горутина на поездку.** При 100 000 активных поездок наивная схема — это 100 000 горутин с таймерами и столько же независимых запросов в бд и Push Service без всякого ограничения. После массового обрыва связи они ударят по соседу почти одновременно. Таймеры живут в памяти и пропадают при рестарте, а при нескольких подах каждая поездка опрашивалась бы с каждого.

В нашей схеме на тик идёт один запрос к бд, одновременно в Push Service не больше `POSITION_WORKERS` вызовов, в памяти не больше `POSITION_QUEUE_SIZE` задач. При 100 000 молчащих поездок за тик в очередь встанут 100, остальные получат `dropped` и вернутся в следующей выборке. Пропускная способность — `POSITION_WORKERS / время ответа Push Service`. Это видно по метрикам и меняется конфигом.

**Push Service отвечает 5 секунд, а тик — раз в 5 секунд.** Каждая задача обрывается по `PUSH_REQUEST_TIMEOUT` (1 секунда) и считается `error`, воркеры заняты почти всё время (`workers.active` = `POSITION_WORKERS`). Сканер при этом не блокируется: тики идут по расписанию, поездки, которые ещё в очереди, дают `duplicate`, новые сверх ёмкости — `dropped`. HTTP-ручки не страдают: `Cancel` — одна операция с map под мьютексом.

**Упрощения** (п. 5.6 задания):
- выборка идёт без `LIMIT`;
- очередь — FIFO-канал, и поездка, которая не ответила, на следующем тике встаёт в хвост;
- время последнего пуша не хранится.

### Метрики и трейсы

| Метрика | Где пишется |
| :-- | :-- |
| `tripgo.positions.saved` | usecase сохранения координаты, после коммита |
| `tripgo.stale_checks{result=queued\|duplicate\|dropped}` | сканер, на каждую постановку |
| `tripgo.position_requests{result=success\|error\|cancelled}` | воркер, на каждую задачу |
| `tripgo.position_requests.duration{result}` | воркер, длительность вызова Push Service |
| `tripgo.position_requests.queue.size` | observable gauge, длина канала |
| `tripgo.position_requests.workers.active` | observable gauge, атомарный счётчик занятых воркеров |

Спан задачи начинается как новый корень и ссылается на спан скана через span link: обработка идёт асинхронно и переживает тик, который её породил. Внутри спана задачи — спан HTTP-вызова Push Service.

### Тесты

| Сценарий из задания | Где |
| :-- | :-- |
| 1. выборка: свежая и завершённая не попадают, без координат попадает | `repository/trip/stale_integration_test.go` |
| 2. повторный тик не добавляет поездку в очередь | `TestScan_TripAlreadyQueuedIsDuplicate` |
| 3. координата между постановкой и обработкой отменяет отправку | `TestPool_PositionBetweenEnqueueAndProcessingCancelsRequest`, `TestPool_StaleCopyAfterRequeueIsSkipped` |
| 4. переполнение даёт `dropped`, а не блокировку | `TestScan_QueueOverflowDropsWithoutBlocking` |
| 5. N задач обрабатываются `POSITION_WORKERS` воркерами | `TestPool_ProcessesTasksWithFixedNumberOfWorkers` |
| 6. по отмене контекста горутин не остаётся | `TestPoller_StopsOnContextCancel`, `TestPoller_StopDrainsQueue`, `TestPoller_StopTimeoutAbortsInFlightRequest` |
| 7. ошибка Push Service не роняет воркер | `TestPool_PushErrorDoesNotKillWorker` |
| 8. сохранение координаты обновляет `last_position_at` и атомарно откатывается | `repository/position/repo_integration_test.go` |

- Время подменяемое: тикер спрятан за интерфейсом `Ticker`, в тестах — `fakeTicker`, тик отправляется в канал. `time.Sleep` в тестах нет.
- В `worker/positions` стоит `goleak.VerifyTestMain`.
- Тесты пула лежат в пакете `positions`, а не `positions_test`: им нужен доступ к каналу очереди и к `scan`, чтобы проверять обход синхронно.
- Гейтвей проверяется через `httptest`: `202`, коды `4xx`/`5xx`, ошибка транспорта, таймаут, закрытие тела при любом коде, `traceparent` и отдельный спан.
- Ручки координат и usecase'ы покрыты так же, как ручки поездок; покрытие `domain` и `usecase` по-прежнему 100%.

### Что ещё изменилось

- `GET /api/v1/trips/{tripId}` отдаёт `last_position_at`.
- Время из бд приводится к UTC при чтении, поэтому в ответах `...Z`, а не часовой пояс машины.
- `environment.toml` обновлён до работы 3: добавлен компонент `push`.
