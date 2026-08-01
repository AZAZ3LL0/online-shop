# tech.md - Merch Shop (3 SKU), ядро проекта, соло-разработка

**Версия ядра: v4**

Changelog:
- v1 - первичная фиксация: стек, схема БД, контракты NOWPayments и Telegram, аналитика, админка (web + TG mini app), правила кода и коммитов, дорожная карта.
- v2 - добавлен §17: топ-5 практик кода с наибольшей отдачей для соло-разработки.
- v3 - ядро разделено на 4 файла (`tech.md`, `SKELETON.md`, `TASKS.md`, `CLAUDE.md`). Чек-лист скелета и детальные слайсы дорожной карты вынесены из `tech.md` в отдельные файлы, добавлена секция общих типов и пропсов UI-компонентов.
- v4 - зафиксированы решения стадий S4-S8, принятые по ходу реализации: QR-библиотека в стеке, пакет `internal/domain/settings`, подкоманда `set-webhook`, переменные окружения деплоя, границы валидации админки, сужение ручных переходов, поведение паузы магазина, момент создания корзины. Детали - §17.

Правило версии: файл меняется только append-only и только владельцем проекта. Любое изменение контракта (схема БД, HTTP-роут, payload вебхука, тип домена) бампает версию и добавляет строку в changelog.

---

## 0. Режим работы: соло + четыре файла ядра

Команды нет. Есть один разработчик (AZAZ3LL0) и N параллельных/последовательных сессий нейросети. Расхождение на стыках возникает не между людьми, а между сессиями: разные имена типов, разные допущения о схеме, разные паттерны на границах. Лечится единым замороженным ядром, разбитым по назначению на четыре файла:

- **`tech.md`** (этот файл) - общее ядро, читает каждая сессия без исключений. Проект, стек, архитектура, схема БД, контракты, общие типы, UI-примитивы, стратегия тестов, правила кода и коммитов, Definition of Done, дорожная карта по стадиям, решения по умолчанию.
- **`SKELETON.md`** - разовая фаза сборки скелета, читается один раз в начале, до первого фичевого слайса.
- **`TASKS.md`** - работа над фичами, открывается на каждую слайс-сессию поверх `tech.md`.
- **`CLAUDE.md`** - короткий указатель для автоподгрузки ядра в Claude Code.

Правила режима:

1. `tech.md` - единственный источник истины для контрактов и конвенций. Каждая сессия начинается с него (в Claude Code подтягивается через `CLAUDE.md`, в веб-чате прикладывается вручную вместе с нужным из `SKELETON.md`/`TASKS.md`).
2. Одна сессия = один вертикальный слайс = одна ветка = один PR. Сессия не вываливает всё приложение сразу, отдаёт сфокусированный дифф.
3. **CONTRACT GAP.** Нужного контракта (типа, поля, роута, статуса) нет в `tech.md` → сессия останавливается и выдаёт блок:
   ```
   CONTRACT GAP
   Нужно: <что>
   Зачем: <какая задача блокируется>
   Предлагаемая форма: <точное определение поля/типа/роута>
   ```
   Код с выдуманным типом не пишется. Владелец аппендит контракт в `tech.md`, бампает версию, сессия продолжает.
4. Тулинговый уровень вместо CODEOWNERS: ветка `main` защищена, мёрдж только через PR с зелёным CI; локальный pre-commit гоняет `gofmt`, `go vet`, `templ generate --check`, `golangci-lint`, `go test ./... -race`.
5. Слайсы не начинаются, пока не пройден чек-лист «скелет готов» из `SKELETON.md`.

## 1. Проект

Магазин одной коллекции: **три модели футболок**. Без личных кабинетов, без регистрации, без авторизации покупателя. Покупка в три шага: главная → корзина → оплата криптой. Статус заказа покупатель смотрит в Telegram-боте.

Цели:
- минимальное трение покупки: от захода до оплаты ≤ 4 экрана;
- продавец видит в админке, откуда пришёл клиент, что покупает и как меняется выручка при изменении цены;
- админка доступна и в браузере, и как Telegram Mini App с идентичной функциональностью.

Не входит в объём v1: мультиязычность, промокоды, отзывы, вишлист, доставка через API служб, возвраты через интерфейс.

---

## 2. Стек

| Слой | Технология | Фиксация |
|---|---|---|
| Язык | Go 1.23+ | `go.mod`, toolchain пинится |
| HTTP | stdlib `net/http` + `http.ServeMux` (роутинг Go 1.22) | без web-фреймворков |
| Шаблоны | `templ` (a-h/templ) | компиляция в Go-код, `templ generate` в CI |
| Клиентский JS | Alpine.js 3.x, вендорится в `web/static/vendor` | без CDN (CSP), без сборщика |
| CSS | Tailwind CSS 3.x (standalone CLI) | сборка в `web/static/css/app.css` |
| БД | PostgreSQL 16 | доступ через `pgx/v5` (pool) |
| Запросы | `sqlc` → типизированные Go-методы | SQL руками в `db/query/*.sql` |
| Миграции | `goose` | `migrations/*.sql`, применяются отдельным шагом деплоя |
| Графики | Chart.js 4.x, вендорится локально | без внешних CDN |
| Платежи | NOWPayments API v1 (invoice + IPN) | за интерфейсом + фейк |
| Telegram | Bot API через webhook, Mini App (initData) | за интерфейсом + фейк |
| Контейнеризация | Docker multi-stage + docker compose | app, postgres, caddy |
| Reverse proxy / TLS | Caddy | автосертификаты |
| Логи | `log/slog`, JSON в проде | request_id в каждой записи |
| Тесты | стандартный `testing`, `testcontainers-go` для Postgres, `net/http/httptest` | без сторонних DSL-ассертов |

Один бинарник, подкоманды: `app serve` (сайт + админка + вебхуки + фоновый воркер), `app migrate`, `app seed`, `app admin-password`.

---

## 3. Архитектура

Слоёная, зависимости направлены внутрь. Домен ничего не знает про HTTP, БД и Telegram — только про интерфейсы, которые сам объявляет. Это и есть требование ООП: инкапсуляция состояния в структурах, поведение через интерфейсы, внедрение зависимостей через конструкторы. Глобальных переменных и package-level состояния нет.

```
cmd/app/main.go                 # разбор подкоманд, сборка графа зависимостей, graceful shutdown
internal/config/                # чтение env, валидация на старте, Config-структура
internal/domain/
  catalog/                      # Product, Variant, Price; интерфейс Repository
  cart/                         # Cart, CartItem, правила количества и стока
  order/                        # Order, статус-машина, PublicToken
  payment/                      # Payment, статусы провайдера → внутренние
  analytics/                    # Visit, Event, агрегаты для админки
  notify/                       # Notification (outbox), правила ретраев
internal/storage/postgres/      # реализации Repository поверх sqlc, транзакции
internal/payments/nowpayments/  # Client (реальный), Fake, VerifySignature
internal/telegram/              # Client (реальный), Fake, webhook-роутер, initData-валидатор
internal/httpx/
  router.go                     # монтирование всех групп роутов
  middleware/                   # request_id, recover, logging, csrf, ratelimit, attribution, adminauth, securityheaders
  handler/shop/                 # публичные хендлеры
  handler/admin/                # админка (web + mini app используют одни хендлеры, разные layout)
  handler/webhook/              # nowpayments, telegram
internal/worker/                # выборка outbox, отправка в Telegram, ретраи
web/templates/                  # .templ: layouts, pages, components
web/static/                     # css, js, vendor, img
db/query/                       # .sql для sqlc
migrations/                     # goose
docker/                         # Dockerfile, compose, Caddyfile
```

DRY закрепляется тремя точками: (1) все деньги проходят через `money.Amount` (int64, минорные единицы + валюта), (2) весь UI собирается из компонентов `web/templates/components`, (3) любая работа с БД идёт через репозиторий домена, прямые запросы из хендлеров запрещены.

---

## 4. Модель данных (PostgreSQL)

Все `id` — `uuid` (`gen_random_uuid()`), все временные метки — `timestamptz`, все деньги — `bigint` в минорных единицах (центы) + отдельная колонка валюты. Каскадов на удаление нет, товары деактивируются флагом.

```
products
  id uuid pk
  slug text unique not null
  title text not null
  description text not null
  price_cents bigint not null           -- текущая цена, USD
  currency char(3) not null default 'USD'
  image_front text not null             -- путь в /static/img
  image_back text not null
  is_active bool not null default true
  sort_order int not null default 0
  created_at, updated_at timestamptz not null

product_variants
  id uuid pk
  product_id uuid fk -> products
  size text not null                    -- S|M|L|XL|XXL
  sku text unique not null
  stock int not null check (stock >= 0)
  reserved int not null default 0 check (reserved >= 0)
  unique (product_id, size)

price_history
  id uuid pk
  product_id uuid fk -> products
  old_price_cents bigint not null
  new_price_cents bigint not null
  changed_by uuid fk -> admin_users
  reason text
  created_at timestamptz not null
  index (product_id, created_at desc)

carts
  id uuid pk                            -- значение подписанной cookie cart_id
  visitor_id uuid                       -- связь с аналитикой
  created_at, updated_at timestamptz not null
  index (updated_at)                    -- уборка брошенных корзин

cart_items
  id uuid pk
  cart_id uuid fk -> carts
  variant_id uuid fk -> product_variants
  qty int not null check (qty between 1 and 10)
  unique (cart_id, variant_id)

orders
  id uuid pk
  number text unique not null           -- человекочитаемый, формат ORD-YYMMDD-XXXX
  public_token text unique not null     -- 32 hex, crypto/rand, для страницы статуса
  tg_link_code text unique not null     -- 16 hex, для deep-link в бота
  status text not null                  -- см. статус-машину §5.1
  subtotal_cents bigint not null
  shipping_cents bigint not null default 0
  total_cents bigint not null
  currency char(3) not null
  customer_name text not null
  customer_contact text not null        -- email или @username, обязателен один
  shipping_address text not null
  comment text
  visitor_id uuid
  first_touch jsonb not null            -- снимок атрибуции первого касания
  last_touch jsonb not null             -- снимок атрибуции сессии заказа
  expires_at timestamptz                -- срок резерва стока (30 мин от создания)
  paid_at, shipped_at, cancelled_at timestamptz
  created_at, updated_at timestamptz not null
  index (status, created_at desc), index (created_at)

order_items                             -- снимок, не ссылка на текущую цену
  id uuid pk
  order_id uuid fk -> orders
  variant_id uuid fk -> product_variants
  product_title text not null
  size text not null
  unit_price_cents bigint not null
  qty int not null
  index (order_id)

payments
  id uuid pk
  order_id uuid fk -> orders
  provider text not null default 'nowpayments'
  provider_payment_id text not null
  invoice_url text
  pay_currency text
  pay_amount numeric(38,18)
  actually_paid numeric(38,18)
  status text not null                  -- сырой статус провайдера
  created_at, updated_at timestamptz not null
  unique (provider, provider_payment_id)

payment_events                          -- сырой лог IPN, источник правды при разборе
  id uuid pk
  payment_id uuid fk -> payments
  provider_status text not null
  payload jsonb not null
  signature_ok bool not null
  dedup_key text unique not null        -- sha256(provider_payment_id|status|payload)
  received_at timestamptz not null

telegram_links
  id uuid pk
  order_id uuid fk -> orders
  chat_id bigint not null
  username text
  linked_at timestamptz not null
  unique (order_id, chat_id)
  index (chat_id)

notifications                           -- outbox исходящих сообщений в TG
  id uuid pk
  order_id uuid fk -> orders
  chat_id bigint not null
  kind text not null                    -- order_linked|status_changed|payment_failed
  payload jsonb not null
  dedup_key text unique not null        -- order_id|kind|status
  status text not null                  -- pending|sent|failed
  attempts int not null default 0
  next_attempt_at timestamptz not null
  last_error text
  created_at, sent_at timestamptz
  index (status, next_attempt_at)

telegram_updates                        -- дедуп входящих апдейтов бота
  update_id bigint pk
  received_at timestamptz not null

visits
  id uuid pk
  visitor_id uuid not null              -- cookie vid, 365 дней
  session_id uuid not null              -- cookie sid, 30 минут неактивности
  landing_path text not null
  referrer text
  utm_source, utm_medium, utm_campaign, utm_content, utm_term text
  user_agent text
  is_bot bool not null default false
  created_at timestamptz not null
  index (created_at), index (visitor_id)

events
  id bigserial pk
  visitor_id uuid not null
  session_id uuid not null
  type text not null                    -- page_view|product_view|add_to_cart|remove_from_cart|checkout_started|payment_created|order_paid
  payload jsonb not null default '{}'
  created_at timestamptz not null
  index (type, created_at), index (session_id)

admin_users
  id uuid pk
  login text unique not null
  password_hash text not null           -- argon2id
  telegram_id bigint unique             -- допуск в mini app
  created_at, last_login_at timestamptz

admin_sessions
  id uuid pk                            -- значение cookie, HttpOnly Secure SameSite=Lax
  admin_id uuid fk -> admin_users
  ip inet, user_agent text
  expires_at timestamptz not null
  created_at timestamptz not null

settings                                -- key-value для параметров без релиза
  key text pk                           -- shipping_cents, shop_paused, order_ttl_minutes
  value jsonb not null
  updated_at timestamptz not null
```

Резерв стока: при создании заказа `reserved += qty` в транзакции с проверкой `stock - reserved >= qty`. При `paid` → `stock -= qty, reserved -= qty`. При `expired|cancelled|failed` → `reserved -= qty`. Освобождение просроченных резервов — задача фонового воркера каждую минуту.

---

## 5. Контракты

### 5.1 Статус-машина заказа

Внутренние статусы (`orders.status`), переходы только вперёд по списку, откаты запрещены:

```
created → awaiting_payment → paid → shipped → delivered
             ↓                 ↓
          expired          refunded
             ↓
         cancelled
```

- `created` — заказ записан, платёж ещё не создан.
- `awaiting_payment` — инвойс NOWPayments создан, покупатель на странице оплаты.
- `paid` — IPN сообщил `finished` (или `confirmed` при полной сумме).
- `expired` — истёк `expires_at` без оплаты, резерв снят.
- `cancelled` — снят вручную из админки.
- `shipped`, `delivered` — выставляются вручную из админки.
- `refunded` — IPN `refunded`.

Правило применения перехода: если новый статус ≤ текущего по порядку — событие логируется, состояние не меняется. Это делает обработку IPN идемпотентной при повторной доставке и при перестановке порядка сообщений.

### 5.2 Публичные HTTP-роуты

```
GET  /                              главная: 3 карточки товара
GET  /product/{slug}                карточка товара, выбор размера
POST /cart/items                    добавить (variant_id, qty) → фрагмент корзины
PATCH /cart/items/{id}              изменить qty → фрагмент корзины
DELETE /cart/items/{id}             удалить → фрагмент корзины
GET  /cart                          страница корзины
GET  /checkout                      форма данных покупателя
POST /checkout                      создать заказ + инвойс → 303 на invoice_url
GET  /order/{public_token}          страница статуса заказа + кнопка Track in Telegram
GET  /order/{public_token}/status   JSON для поллинга Alpine (раз в 10 с)
GET  /payment/return/{public_token} возврат с NOWPayments
GET  /healthz                       liveness
```

Все мутирующие роуты требуют CSRF-токен (double submit cookie). Ответы на действия корзины — HTML-фрагменты `templ`, Alpine подменяет узел (никакого клиентского состояния корзины, источник истины — сервер).

### 5.3 Роуты админки

Одни и те же хендлеры, два layout: `AdminWeb` (полный) и `AdminMini` (компактный, под TG Mini App). Разделение по префиксу.

```
GET/POST /admin/login               логин по паролю (только web)
POST     /admin/logout
GET      /admin                     дашборд: выручка, заказы, конверсия, источники
GET      /admin/orders              список с фильтрами (статус, период, товар)
GET      /admin/orders/{id}         карточка заказа, история платежа, TG-привязка
POST     /admin/orders/{id}/status  перевод статуса вручную
GET      /admin/products            список товаров
POST     /admin/products/{id}/price смена цены (пишет price_history)
POST     /admin/products/{id}/stock правка стока по размерам
GET      /admin/analytics           графики и таблицы (§8.2)
GET      /admin/settings            доставка, TTL заказа, пауза магазина
GET      /admin/api/metrics         JSON для Chart.js (range, granularity)

GET      /tgapp                     точка входа Mini App, валидирует initData
POST     /tgapp/auth                initData → admin-сессия (короткая, 1 час)
GET      /tgapp/*                   те же данные, layout AdminMini
```

### 5.4 NOWPayments

Создание платежа (при `POST /checkout`):

```
POST https://api.nowpayments.io/v1/invoice
x-api-key: <NOWPAYMENTS_API_KEY>
{
  "price_amount": <total в USD, дробное>,
  "price_currency": "usd",
  "order_id": "<orders.number>",
  "order_description": "Merch order <number>",
  "ipn_callback_url": "https://<host>/webhooks/nowpayments",
  "success_url": "https://<host>/payment/return/<public_token>",
  "cancel_url":  "https://<host>/order/<public_token>"
}
→ { "id": "<invoice_id>", "invoice_url": "..." }
```

IPN:

```
POST /webhooks/nowpayments
Header: x-nowpayments-sig = HMAC_SHA512(sorted_json(body), NOWPAYMENTS_IPN_SECRET)
Body:  { payment_id, payment_status, pay_address, price_amount, price_currency,
         pay_amount, actually_paid, pay_currency, order_id, ... }
```

Правила обработки, обязательны к соблюдению:

1. Считать сырое тело, посчитать HMAC-SHA512 по JSON с **лексикографически отсортированными ключами**, сравнить через `hmac.Equal`. Подпись не совпала → 401, запись в `payment_events` с `signature_ok=false`, состояние не трогать.
2. Записать `payment_events` с `dedup_key`. Конфликт по уникальному ключу → 200 OK, выход (повтор доставки).
3. Маппинг статусов: `waiting|confirming|sending` → `awaiting_payment`; `confirmed|finished` при `actually_paid >= pay_amount` → `paid`; `partially_paid` → остаётся `awaiting_payment`, флаг в админку; `failed|expired` → `expired`; `refunded` → `refunded`.
4. Применить переход по правилу §5.1, в одной транзакции списать сток и поставить запись в `notifications`.
5. Всегда отвечать 200 после успешной записи события. Ретраи провайдера не должны множить эффект.

Реальный клиент и фейк реализуют один интерфейс:

```go
type Provider interface {
    CreateInvoice(ctx context.Context, in InvoiceRequest) (Invoice, error)
    VerifySignature(rawBody []byte, signature string) error
    ParseIPN(rawBody []byte) (IPN, error)
}
```

Фейк отдаёт детерминированный `invoice_url` вида `/dev/pay/{order_number}` и локальную страницу, которая шлёт валидно подписанный IPN с выбранным статусом. Вся разработка чекаута и статусов идёт против фейка, ключи провайдера не блокируют работу.

### 5.5 Telegram

**Привязка заказа.** На `POST /checkout` генерируется `tg_link_code`. Покупателю показываются два входа: на странице оплаты (до ухода на инвойс) и на `/order/{public_token}` (после возврата) — кнопка `Track order in Telegram` → `https://t.me/<BOT_USERNAME>?start=<tg_link_code>`. Плюс QR той же ссылки для оплаты с десктопа.

**Бот.** Только webhook, без long polling.

```
POST /webhooks/telegram/<TELEGRAM_WEBHOOK_PATH_SECRET>
Header: X-Telegram-Bot-Api-Secret-Token = <TELEGRAM_WEBHOOK_SECRET>
```

Обработка: несовпадение secret-token → 401. `update_id` уже в `telegram_updates` → 200, выход. Далее команды:

- `/start <code>` — найти заказ по `tg_link_code`, создать `telegram_links`, ответить текущим статусом и составом. Код не найден → нейтральный ответ без подсказок о существовании заказа.
- `/start` без кода — короткая справка.
- `/status` — статусы всех заказов, привязанных к этому `chat_id`.
- любое другое сообщение — подсказка по командам.

**Исходящие уведомления** идут только через outbox `notifications`: воркер берёт `pending` с `next_attempt_at <= now()`, шлёт, при ошибке — экспоненциальный бэкофф (30с, 2м, 10м, 1ч, 6ч, максимум 5 попыток), после исчерпания `failed` + запись в лог. `dedup_key` гарантирует одно сообщение на переход статуса.

**Mini App.** `initData` валидируется по алгоритму Telegram: `secret = HMAC_SHA256("WebAppData", bot_token)`, сверка `hash` от отсортированного `data_check_string`; `auth_date` не старше 15 минут; `user.id` обязан быть в `admin_users.telegram_id`. Успех → короткая admin-сессия. Промах → 403 без деталей.

Интерфейс:

```go
type Bot interface {
    SendMessage(ctx context.Context, chatID int64, text string, kb *Keyboard) error
    SetWebhook(ctx context.Context, url, secret string) error
}
```

Фейк пишет сообщения в лог и в таблицу — тесты и локальная разработка не требуют токена.

### 5.6 Атрибуция и события

Middleware `attribution` на каждом GET публичной страницы:
- нет cookie `vid` → выдать (uuid, 365 дней, HttpOnly, SameSite=Lax);
- нет cookie `sid` или истёк → новый `sid` (30 минут скользящего окна) + запись в `visits` с `landing_path`, `referrer`, всеми `utm_*` из query;
- очевидные боты (User-Agent по списку) помечаются `is_bot=true` и исключаются из метрик.

`first_touch` фиксируется в cookie `ft` (json, 365 дней) при первом визите и копируется в заказ; `last_touch` берётся из текущего `visits`. События пишутся сервером в местах их возникновения, не с клиента.

---

## 6. Общие типы

Типы, которые объявляются один раз и переиспользуются во всех слайсах. Сессия не заводит свою копию этих типов и не меняет их поля без `CONTRACT GAP`. Источник истины по полям — схема БД в §4, здесь только форма Go-значений поверх неё.

### 6.1 `internal/money`

```go
// Amount хранит деньги в минорных единицах (центах), без float.
type Amount struct {
    Cents    int64
    Currency string // ISO 4217, "USD"
}

func (a Amount) Add(b Amount) (Amount, error)   // ошибка при разной валюте
func (a Amount) Sub(b Amount) (Amount, error)
func (a Amount) String() string                 // "$12.00"
```

Любое поле `*_cents` в БД конвертируется в `Amount` на границе репозитория. В домене и во view-моделях `Amount`, не `int64`, не `float64`.

### 6.2 `internal/domain/order` (статусы, шарятся между `order`, `payment`, `notify`, админкой)

```go
type Status string

const (
    StatusCreated          Status = "created"
    StatusAwaitingPayment  Status = "awaiting_payment"
    StatusPaid             Status = "paid"
    StatusShipped          Status = "shipped"
    StatusDelivered        Status = "delivered"
    StatusExpired          Status = "expired"
    StatusCancelled        Status = "cancelled"
    StatusRefunded         Status = "refunded"
)

// CanTransition — единственное место, где проверяется допустимость перехода (§5.1).
func CanTransition(from, to Status) bool
```

### 6.3 `internal/domain/catalog`

```go
type Product struct {
    ID          uuid.UUID
    Slug        string
    Title       string
    Description string
    Price       money.Amount
    ImageFront  string
    ImageBack   string
    IsActive    bool
    Variants    []Variant
}

type Variant struct {
    ID       uuid.UUID
    Size     string // S|M|L|XL|XXL
    SKU      string
    Stock    int
    Reserved int
}

func (v Variant) Available() int { return v.Stock - v.Reserved }
```

### 6.4 Атрибуция (шарится между `attribution`-middleware, `order`, `analytics`)

```go
type Touch struct {
    VisitorID uuid.UUID
    UTMSource, UTMMedium, UTMCampaign, UTMContent, UTMTerm string
    Referrer, LandingPath string
}
```

`Touch` сериализуется в `jsonb` (`orders.first_touch`, `orders.last_touch`) и в cookie `ft`. Одна структура, никакого дублирования полей в SQL-запросах и в Go отдельно.

### 6.5 Доменные ошибки (шарятся между доменом и HTTP-слоем, см. §16.3)

```go
var (
    ErrOutOfStock      = errors.New("catalog: out of stock")
    ErrCartItemLimit   = errors.New("cart: qty out of range")
    ErrOrderExpired    = errors.New("order: reservation expired")
    ErrInvalidSignature = errors.New("payment: invalid webhook signature")
)
```

Новый доменный тип ошибки добавляется здесь, не изобретается заново внутри хендлера.

-e ---

## 7. Фронт

### 7.1 Страницы

- **Главная.** Hero-блок, три карточки. Карточка: изображение, название, цена, размеры. **Наведение курсора меняет обложку на вид сзади** — реализуется CSS через Tailwind `group`: два `<img>` в контейнере, `group-hover:opacity-0` для фронта, `group-hover:opacity-100` для спины, `transition-opacity duration-300`. JS для этого не используется. Для touch-устройств (`@media (hover: none)`) Alpine-директива переключает вид по тапу на бейдж `flip`.
- **Товар.** Крупные фото (фронт/спина, переключение по клику), описание, таблица размеров, выбор размера, кнопка в корзину. Отсутствующий размер — неактивен с пометкой `sold out`.
- **Корзина.** Позиции, изменение количества, удаление, сумма, доставка, итог, переход к оформлению.
- **Чекаут.** Имя, контакт (email или @username), адрес, комментарий, чекбокс согласия, кнопка `Pay with crypto`.
- **Статус заказа** `/order/{token}`. Номер, состав, сумма, текущий статус, таймер до истечения резерва, кнопка `Track in Telegram`, QR. Поллинг статуса раз в 10 секунд.

### 7.2 Компоненты `web/templates/components`

Собираются до начала слайсов, в `SKELETON.md`, дальше только используются. Дублирование разметки вместо компонента - нарушение DRY и повод отклонить PR на самопроверке. Каждый компонент - `templ`-функция с типизированными параметрами, эскиз сигнатуры:

```templ
templ Button(variant ButtonVariant, label string, attrs templ.Attributes)
templ Badge(text string, tone Tone)
templ Card(imageFront, imageBack, href, title string, price money.Amount)
templ Input(name, label, value string, kind string, err string)
templ Select(name, label string, options []Option, selected string)
templ Textarea(name, label, value string, maxRunes int)
templ Modal(id, title string, body templ.Component)
templ Table(headers []string, rows []templ.Component)
templ Alert(tone Tone, message string)
templ Money(amount money.Amount)
templ StatusPill(status order.Status)
templ EmptyState(title, hint string)
templ Pagination(page, totalPages int, baseURL string)
templ StatCard(label string, value string, delta string, tone Tone)
templ ChartCanvas(id string, kind string) // kind: "bar"|"line"|"funnel"
templ Toast(id string, tone Tone, message string)
```

`Tone` и `ButtonVariant` - закрытые перечисления (`type Tone string`, константы `ToneNeutral|ToneSuccess|ToneWarning|ToneDanger`), объявлены рядом с компонентами, не дублируются по местам использования.

### 7.3 Alpine

Только там, где сервер не справляется без перезагрузки: тап-флип карточки, счётчик количества, модалка, тосты, поллинг статуса, переключение диапазона графиков. Состояния приложения на клиенте нет — корзина живёт на сервере, ответы приходят HTML-фрагментами.

### 7.4 Стиль

Минимализм: белый/чёрный фон, один акцентный цвет, крупная типографика, никаких теней-градиентов. Мобильный приоритет, брейкпоинты Tailwind по умолчанию. Изображения `webp`, `loading="lazy"` кроме первого экрана, фиксированный `aspect-ratio` против сдвигов лейаута.

---

## 8. Админка

Один набор данных и хендлеров, два представления: браузер и Telegram Mini App. Mini App отличается только layout, шириной, отсутствием логин-формы и цветами из `themeParams` Telegram.

### 8.1 Дашборд

Верхний ряд `StatCard`: выручка за период, число заказов, средний чек, конверсия визит→оплата, доля брошенных корзин. Селектор периода: сегодня / 7 дней / 30 дней / произвольный.

### 8.2 Аналитика

- Выручка и заказы по дням (столбцы + линия), **поверх — маркеры смены цены** из `price_history`: сразу видно влияние цены на продажи.
- Воронка: визиты → просмотры товара → добавления в корзину → чекауты → оплаты, с процентами перехода.
- Источники трафика: таблица по `utm_source/medium/campaign` и referrer, с выручкой и конверсией по каждому.
- Топ размеров и товаров.
- Гео и устройства (по User-Agent, без сторонних трекеров).
- Все графики получают данные из `GET /admin/api/metrics?range=&granularity=`, рисуются Chart.js. Агрегаты считаются SQL-запросами, не в Go-цикле по строкам.

### 8.3 Управление ценой

Инлайн-редактирование цены товара, обязательное поле `reason`, запись в `price_history`, показ прошлой цены и дельты. Изменение цены **не влияет на созданные заказы** — они хранят снимок цены в `order_items`.

### 8.4 Заказы

Список с фильтрами и поиском по номеру, карточка с составом, суммой, атрибуцией, историей IPN, статусом TG-привязки. Ручные переходы `paid → shipped → delivered`, `cancelled`. Каждый ручной переход ставит уведомление в outbox — покупатель получает сообщение в боте.

### 8.5 Доступ

Web: логин + пароль (argon2id), сессия в БД, 12 часов, rate limit 5 попыток / 15 минут на IP. Mini App: только `initData` + allowlist `telegram_id`. Ни один админ-роут не доступен без валидной сессии; проверка в middleware, а не в хендлерах.

---

## 9. Безопасность

Обязательные требования, проверяются на каждом PR:

1. **Все SQL — параметризованные** (sqlc). Конкатенация значений в SQL запрещена.
2. **Вывод шаблонов экранируется** средствами templ. `templ.Raw` только для собственного статичного HTML, никогда для пользовательского ввода.
3. **CSRF** на всех POST/PATCH/DELETE: double submit cookie + проверка `Origin`.
4. **Секреты только из env**, `.env` в `.gitignore`, в репозитории лежит `.env.example` с пустыми значениями. Ключей в коде и в тестах нет.
5. **HMAC-проверки** обязательны и выполняются до парсинга тела: NOWPayments IPN (SHA-512), Telegram secret-token, Mini App initData (SHA-256). Сравнение только `hmac.Equal`.
6. **Rate limiting**: `POST /checkout` 10/час/IP, `/cart/*` 60/мин/IP, `/admin/login` 5/15мин/IP, вебхуки 120/мин.
7. **Заголовки** (middleware `securityheaders`): `Content-Security-Policy` без `unsafe-inline` для скриптов (Alpine и Chart.js вендорятся файлами), `X-Content-Type-Options: nosniff`, `Referrer-Policy: strict-origin-when-cross-origin`, `X-Frame-Options: DENY` для админки и `frame-ancestors https://web.telegram.org` для `/tgapp`, HSTS в проде.
8. **Cookies**: `HttpOnly`, `Secure`, `SameSite=Lax`, cart_id и admin session подписаны HMAC-ключом приложения.
9. **Валидация входа** на границе: длины строк, размеры, `qty` 1..10, e-mail/username по строгому шаблону. Ошибки валидации не раскрывают внутренности.
10. **Токены** (`public_token`, `tg_link_code`, session id) — `crypto/rand`, минимум 128 бит, сравнение постоянного времени.
11. **Логи не содержат** персональных данных покупателя, полных адресов, ключей, тел вебхуков с подписями.
12. **Контейнер**: непривилегированный пользователь, `distroless`/`scratch` финальный образ, read-only rootfs, БД не публикует порт наружу.
13. **Ошибки наружу** — обобщённые, детали только в логах с `request_id`.
14. `govulncheck` и `golangci-lint` — обязательные шаги CI.

---

## 10. Инфраструктура

### 10.1 Docker

`docker/Dockerfile` — multi-stage: сборка Tailwind → `templ generate` → `go build -trimpath -ldflags="-s -w"` → финальный минимальный образ с бинарником и `web/static`.

`docker/compose.yml`: `app`, `postgres` (том, healthcheck), `caddy` (TLS, проксирование). Профиль `dev` дополнительно поднимает `tailwind --watch` и `templ generate --watch`.

### 10.2 Конфигурация

Единственный модуль `internal/config`, читает env, валидирует на старте, падает при отсутствии обязательного значения. `.env.example`:

```
APP_ENV=dev
APP_BASE_URL=http://localhost:8080
APP_SECRET=                      # HMAC для cookie, 32+ байт
DATABASE_URL=postgres://app:app@postgres:5432/shop?sslmode=disable
NOWPAYMENTS_API_KEY=
NOWPAYMENTS_IPN_SECRET=
PAYMENTS_PROVIDER=fake           # fake|nowpayments
TELEGRAM_BOT_TOKEN=
TELEGRAM_BOT_USERNAME=
TELEGRAM_WEBHOOK_SECRET=
TELEGRAM_WEBHOOK_PATH_SECRET=
TELEGRAM_PROVIDER=fake           # fake|telegram
ADMIN_TELEGRAM_IDS=
ORDER_TTL_MINUTES=30
SHIPPING_CENTS=0
```

`PAYMENTS_PROVIDER=fake` и `TELEGRAM_PROVIDER=fake` — режим по умолчанию для разработки: приложение полностью работоспособно без единого внешнего ключа.

### 10.3 Миграции и сид

Миграции — только `goose`, только вперёд, применяются шагом деплоя и на эфемерном Postgres в CI. Ручная правка применённой миграции запрещена, изменение оформляется новой.

`app seed` заполняет три товара с размерами, ценами и изображениями-заглушками, тестовые визиты, события и заказы за 30 дней — чтобы графики админки не разрабатывались на пустой БД.

### 10.4 CI (GitHub Actions)

Гейт на PR: `gofmt -l` (пусто), `templ generate --check`, `go vet`, `golangci-lint`, `govulncheck`, `go test ./... -race`, миграции на сервисном Postgres, сборка образа. Деплой при мёрдже в `main`: сборка, push, `goose up`, рестарт compose-сервиса. Ветка `main` защищена, прямые пуши запрещены.

---

## 11. Тесты

Тест привязан к слайсу и PR, не к стадии. Слайс без тестов не мёрджится.

Главная ловушка соло-режима с нейросетью: сессия пишет код, потом пишет тесты, подтверждающие, что код делает то, что делает, вместе с багами. Правило: **тесты выводятся из критериев приёмки задачи, не из реализации**.

Обязательные типы:

1. **Контрактные на стыках.** Обработчик IPN проверяется против зафиксированного в §5.4 payload; фейковый провайдер валидирует исходящий запрос и падает на мусоре. То же для Telegram-апдейтов.
2. **Идемпотентность.** Один и тот же IPN, доставленный дважды (и трижды, вперемешку по порядку статусов) даёт ровно один переход, одно списание стока и одно сообщение в outbox. Тот же тест — на `update_id` бота и на воркер уведомлений.
3. **Путь ошибки.** Провайдер вернул 500 / таймаут / битую подпись / `partially_paid`; Telegram вернул 429. Проверяется через фейки, умеющие возвращать ошибку.
4. **Инварианты домена** (табличные и рандомизированные входы на чистой логике): сумма корзины = сумме позиций при любых наборах; `stock - reserved >= 0` при любой последовательности резерв/освобождение; статус-машина не откатывается назад ни при какой перестановке событий.
5. **Интеграционные на репозиториях** через `testcontainers-go` с реальным Postgres и применёнными миграциями. Моков БД нет.
6. **Один e2e-путь** на `httptest`: главная → добавить в корзину → чекаут → фейковый IPN `finished` → статус `paid` → уведомление в outbox.

Что не тестируется: вёрстка, CSS-hover, тексты.

---

## 12. Правила кода и коммитов

### Код

- **ООП по-Go**: поведение на структурах, зависимости через интерфейсы, объявленные потребителем; конструкторы `NewX(deps) *X` возвращают готовый к работе объект; глобального состояния нет; интерфейсы узкие (1–3 метода).
- **DRY**: третье повторение — повод для извлечения. Деньги, форматирование, валидация, доступ к БД, UI-разметка — по одной реализации на проект.
- **Ошибки** оборачиваются `fmt.Errorf("...: %w", err)`, доменные ошибки — типизированные значения (`var ErrOutOfStock = errors.New(...)`), паники наружу не выходят (middleware `recover`).
- `context.Context` первым аргументом во всём, что ходит в БД или в сеть; таймауты на каждый внешний вызов.
- Транзакции — на уровне сервиса домена, а не репозитория.
- Функция длиннее ~60 строк или вложенность больше 3 — режется.
- **Комментарии** — только на английском, кратко и по делу, объясняют *почему*, а не пересказывают код. Закомментированный код в PR не остаётся. Экспортируемые типы и методы — с godoc-строкой в одну строку.

### Коммиты

- Язык — только английский, во всём: сообщения коммитов, комментарии, названия веток, тексты PR.
- Формат фиксированный, всегда Conventional Commits: `type(scope): summary`. `type` ∈ `feat|fix|test|refactor|chore|docs`. `summary` в императиве, со строчной буквы, без точки, до 50 символов. Тело — только если нужно объяснить *почему*.
- Коммиты маленькие и по ходу работы, не одной свалкой в конце. Каждый коммит по возможности собирается и проходит тесты.
- **Никаких следов нейросети**: без `Co-Authored-By`, без `Generated with`, без упоминаний ассистента в теле коммита, в PR и в комментариях кода. Без эмодзи.
- Авторство фиксировано в репозитории:
  ```
  git config user.name  "AZAZ3LL0"
  git config user.email "sadrievsamat4@gmail.com"
  ```
- Ветки: `feat/<slice-name>`, `fix/<slice-name>`.

Примеры корректных сообщений:

```
feat(cart): add server-rendered cart fragment
fix(payments): verify ipn signature before parsing body
test(order): cover duplicate ipn delivery
refactor(admin): extract metrics query into repository
chore(ci): run govulncheck on pull requests
```

---

## 13. Definition of Done одной задачи

1. `gofmt`, `go vet`, `golangci-lint`, `govulncheck` — чисто.
2. `templ generate` выполнен, сгенерированный код закоммичен.
3. Тесты по доктрине §11 написаны из критериев приёмки и зелёные с `-race`.
4. Миграции применяются на чистой БД и не ломают существующие данные.
5. Никаких выдуманных типов и полей — всё из `tech.md`; при нехватке был `CONTRACT GAP` и бамп версии.
6. Использованы существующие компоненты и хелперы, дублей нет.
7. Секреты не попали в код, логи и тесты.
8. Самопроверка перед мёрджем по чек-листу из `TASKS.md` пройдена.

---

-e ---

## 14. Дорожная карта

Стадия = набор слайсов. Слайс = ветка = PR = одна сессия.

**S0. Скелет.** Собирается по `SKELETON.md`, вне слайсов TASKS.md. Всё из чек-листа «скелет готов» в `SKELETON.md`.

**S1. Каталог.**
- слайс: главная с тремя карточками и hover-сменой обложки (+ тап-флип на мобильном);
- слайс: страница товара, выбор размера, `sold out` по стоку.

**S2. Корзина.**
- слайс: добавление/изменение/удаление позиции, серверные фрагменты, лимиты `qty`;
- слайс: страница корзины, расчёт сумм (тесты инвариантов сумм).

**S3. Заказ и оплата.**
- слайс: чекаут, валидация, создание заказа, снимок цен, резерв стока, `expires_at`;
- слайс: интеграция NOWPayments за интерфейсом + фейк, создание инвойса, редирект;
- слайс: IPN-эндпоинт, проверка подписи, дедуп, статус-машина, списание стока (тесты идемпотентности и путей ошибки);
- слайс: страница `/order/{token}` с поллингом и таймером резерва;
- слайс: воркер освобождения просроченных резервов.

**S4. Telegram-статусы.**
- слайс: генерация `tg_link_code`, кнопка и QR `Track in Telegram` на чекауте и странице заказа;
- слайс: webhook бота, дедуп `update_id`, `/start <code>`, привязка, `/status`;
- слайс: outbox-уведомления на каждый переход статуса, ретраи с бэкоффом.

**S5. Админка (web).**
- слайс: аутентификация, сессии, rate limit, layout админки;
- слайс: заказы — список, фильтры, карточка, ручные переходы статуса;
- слайс: товары — правка цены с `price_history` и `reason`, правка стока;
- слайс: настройки (доставка, TTL, пауза магазина).

**S6. Аналитика.**
- слайс: middleware атрибуции, `visits`, `events`, отсечение ботов;
- слайс: SQL-агрегаты и `GET /admin/api/metrics`;
- слайс: дашборд — StatCards, выручка/заказы с маркерами смены цены;
- слайс: воронка и источники трафика.

**S7. TG Mini App админка.**
- слайс: валидация `initData`, allowlist, короткая сессия, `/tgapp`;
- слайс: layout AdminMini поверх тех же хендлеров, темизация из `themeParams`;
- слайс: адаптация графиков и таблиц под узкий экран.

**S8. Продакшн.**
- слайс: HTTPS и Caddy, домен, реальные ключи NOWPayments и бота, `setWebhook`;
- слайс: бэкапы БД по расписанию, ротация логов, `/healthz` в мониторинг;
- слайс: прогон полного сценария на реальном платеже минимальной суммы.

---

## 15. Решения по умолчанию

Приняты, чтобы не блокировать разработку. Меняются одной строкой здесь с бампом версии.

- Валюта витрины — USD, крипто-валюту выбирает покупатель на стороне NOWPayments.
- Размеры — S, M, L, XL, XXL для всех трёх моделей.
- Доставка — фиксированная стоимость из `settings.shipping_cents`, по умолчанию 0.
- Язык интерфейса — один (английский), i18n не закладывается.
- Резерв стока — 30 минут.
- Контакт покупателя — одно обязательное поле (email или @username), без верификации.
- Частичная оплата не считается успешной, разбирается вручную из админки.
- Возвраты — вне интерфейса, только фиксация статуса `refunded` по IPN.

---

## 16. Топ-5 практик с наибольшей отдачей

Отобраны по критерию «цена ошибки высокая, а сама практика дешёвая» — специально под соло-режим, где некому поймать баг на код-ревью.

1. **Длина строк — в рунах, а не в байтах.** `len(s)` в Go считает байты. Имя покупателя, комментарий к заказу, `reason` в `price_history` — всё в UTF-8, скорее всего с кириллицей. Валидация границ (`customer_name`, `comment`, `reason`) обязана идти через `utf8.RuneCountInString(s)`, иначе лимит в 100 символов на кириллице тихо обрежется на 50 и даст мусор в БД или 500-ку на границе. Это ровно тот класс бага, который не всплывёт на тестовых английских данных и вылезет только на реальном покупателе.
2. **Context с таймаутом на каждый внешний вызов, без исключений.** NOWPayments и Telegram — сети, которые рано или поздно подвиснут. Любой `http.Client` вызов из `internal/payments` и `internal/telegram` — с `context.WithTimeout` (рекомендуемо: 5с на создание инвойса, 10с на IPN-обработку, 3с на отправку сообщения в бота), плюс ретраи только на идемпотентных GET/проверках статуса. Без этого один зависший запрос к провайдеру блокирует горутину-хендлер и деградирует весь сайт под нагрузкой.
3. **Типизированные доменные ошибки + `errors.Is/As` по всей цепочке.** Никаких `errors.New("out of stock")`, сравниваемых по строке. Один раз объявленный `var ErrOutOfStock = errors.New(...)` в `domain/catalog`, оборачивается `%w` на каждом уровне, распознаётся `errors.Is` в HTTP-хендлере для выбора кода ответа (409 вместо общего 500). В соло-проекте это единственный способ через полгода не забыть, какие ошибки вообще возможны в бизнес-логике.
4. **Узкие интерфейсы, объявленные потребителем, а не производителем.** `Repository` в `domain/catalog` — 1–3 метода, ровно те, что нужны сервису прямо сейчас, а не «универсальный» CRUD с 10 методами про запас. Позволяет подменить Postgres-репозиторий фейком в тесте одной строкой и не тащит за собой методы, которые никогда не вызываются — меньше площади для ошибки на нейросессию, которая видит только сигнатуру интерфейса.
5. **`golangci-lint` + `go vet` + `-race` — обязательный шаг перед каждым коммитом, а не только в CI.** В pre-commit hook, локально, до пуша. В соло-режиме с нейросетью CI ловит проблему на 10–15 минут позже, чем могла бы; race-детектор особенно критичен здесь, потому что воркер outbox и HTTP-хендлеры одновременно трогают одни и те же таблицы через пул соединений — гонка по бизнес-логике (двойное списание стока) не всегда всплывает на тестах, но всплывает под реальным трафиком.

---

## 17. Решения S4-S8, зафиксированные постфактум

Всё перечисленное реализовано в коде и работает; раздел закрывает разрыв между
кодом и ядром. Порядок - по разделу, который дополняется.

### 17.1 Стек (§2)

**QR-кодирование - `rsc.io/qr`.** §7.1 требует QR на странице заказа, но §2 не
называл библиотеку. Кодирование серверное, результат отдаётся PNG в `data:`-URI,
внешних запросов и CDN нет (§9.7). Прямая зависимость в `go.mod`.

### 17.2 Архитектура (§3)

**`internal/domain/settings`** - типизированный слой над key-value таблицей
`settings`. Объявляет узкие интерфейсы для потребителей: `ShippingCents` для
корзины, `OrderTTL` для сервиса заказов (§16.4). В раскладке §3 пакет не назван.

### 17.3 Подкоманды (§2)

**`app set-webhook`** - пятая подкоманда бинарника. Регистрирует callback бота на
`APP_BASE_URL + /webhooks/telegram/<TELEGRAM_WEBHOOK_PATH_SECRET>` с заголовочным
секретом `TELEGRAM_WEBHOOK_SECRET`. Отдельный шаг деплоя, а не действие `serve`:
перезапуск staging-контейнера не должен переводить вебхук на себя. Отказывает при
`http`, при `localhost` и при `TELEGRAM_PROVIDER=fake`. Путь-секрет входит в URL,
поэтому в лог пишется только хост (§9.11).

### 17.4 Конфигурация (§10.2)

Четыре переменные читает только `docker/compose.yml` и `scripts/backup-db.sh`,
`internal/config` их не видит:

```
SITE_ADDRESS=:80                 # ":80" локально, голый домен в проде
POSTGRES_PASSWORD=               # обязателен, compose не стартует без него
BACKUP_DIR=/var/backups/qzq-shop
BACKUP_KEEP_DAYS=14
```

**`SHIPPING_CENTS` и `ORDER_TTL_MINUTES` перестали быть значениями и стали
умолчаниями.** Источник истины - таблица `settings` (§5.3, §15); переменная
окружения работает, только пока ключ ни разу не записан.

### 17.5 Статус-машина (§5.1)

**Ручные переходы сужены до `shipped`, `delivered`, `cancelled`**
(`order.ManualTargets`). §5.1 допускает `paid -> refunded`, но там же сказано, что
`refunded` приходит из IPN, поэтому админка такого перехода не предлагает.
`expired` принадлежит воркеру резервов. Судья перехода прежний - `CanTransition`,
`ManualTargets` только сужает его до того, что принадлежит человеку.

### 17.6 Настройки (§4, ключ `settings.shop_paused`)

§4 называет ключ, но нигде не сказано, что делает пауза. Реализовано так:
`POST /checkout` отвечает 503 и не резервирует сток, форма чекаута показывает
сообщение о паузе, каталог и корзина остаются доступны на чтение. Уже созданные
заказы пауза не трогает.

### 17.7 Границы валидации админки (§8.3, §8.4)

Ни одна из них не была названа в ядре:

| Что | Границы |
|---|---|
| `price_history.reason` | 3-200 рун |
| цена товара | 1 цент - $1 000 000 |
| сток по размеру | 0 - 100 000 |
| окно резерва (`order_ttl_minutes`) | 5 - 1440 минут |
| доставка (`shipping_cents`) | 0 - 100 000 |
| поиск по номеру заказа | не длиннее 40 рун |
| страница списка заказов | 20 по умолчанию, не больше 100 |

Длины везде считаются в рунах (§16.1).

### 17.8 Корзина (§4, таблица `carts`)

**Строка `carts` создаётся только при первом добавлении товара.** Раньше любое
чтение витрины открывало корзину и ставило cookie - включая заходы ботов, что
засоряло таблицу и аналитику. Чтения теперь рендерят то, на что уже указывает
подписанная cookie; корзину открывает только `POST /cart/items`. Форма чекаута
тоже читает: заход на `/checkout` без корзины уводит на `/cart`.

**Отрицательная стоимость доставки зажимается в ноль** при расчёте итога. Оба
пути записи её отбивают (валидация настроек и `config.Load`), но корзина -
последнее место, где кривая строка в key-value таблице ещё не превратилась в
скидку на заказ.

### 17.9 Mini App (§5.3)

`GET /tgapp/*` из §5.3 раскрыт до конкретных роутов: `/tgapp/orders`,
`/tgapp/orders/{id}`, `/tgapp/products`, `/tgapp/analytics` - те же хендлеры
админки под layout `AdminMini`. В навигации панели у аналитики появился свой
раздел.

### 17.10 Известное ограничение outbox (§4, `notifications.dedup_key`)

`dedup_key = order_id|kind|status` зафиксирован в §4 буквально. Таблица
`telegram_links` при этом допускает несколько чатов на один заказ
(`unique (order_id, chat_id)`). Следствие: когда за заказом следят два чата,
сообщение о смене статуса получает только первый - второе вставить не даёт
уникальный ключ.

Это расхождение двух контрактов, а не баг реализации. Расширение ключа до
`order_id|kind|status|chat_id` требует миграции и бампа версии, поэтому вынесено
из S4 отдельным решением. Текущее поведение закреплено тестом
`TestASecondChatCanFollowTheSameOrder`.
