-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS pgcrypto;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE products (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        text NOT NULL UNIQUE,
    title       text NOT NULL,
    description text NOT NULL,
    price_cents bigint NOT NULL,
    currency    char(3) NOT NULL DEFAULT 'USD',
    image_front text NOT NULL,
    image_back  text NOT NULL,
    is_active   boolean NOT NULL DEFAULT true,
    sort_order  integer NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE product_variants (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id uuid NOT NULL REFERENCES products (id),
    size       text NOT NULL,
    sku        text NOT NULL UNIQUE,
    stock      integer NOT NULL CHECK (stock >= 0),
    reserved   integer NOT NULL DEFAULT 0 CHECK (reserved >= 0),
    UNIQUE (product_id, size)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE admin_users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    login         text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    telegram_id   bigint UNIQUE,
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_login_at timestamptz
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE admin_sessions (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_id   uuid NOT NULL REFERENCES admin_users (id),
    ip         inet,
    user_agent text,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE price_history (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id      uuid NOT NULL REFERENCES products (id),
    old_price_cents bigint NOT NULL,
    new_price_cents bigint NOT NULL,
    changed_by      uuid REFERENCES admin_users (id),
    reason          text,
    created_at      timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX price_history_product_created_idx ON price_history (product_id, created_at DESC);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE carts (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    visitor_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX carts_updated_at_idx ON carts (updated_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE cart_items (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cart_id    uuid NOT NULL REFERENCES carts (id),
    variant_id uuid NOT NULL REFERENCES product_variants (id),
    qty        integer NOT NULL CHECK (qty BETWEEN 1 AND 10),
    UNIQUE (cart_id, variant_id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE orders (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    number           text NOT NULL UNIQUE,
    public_token     text NOT NULL UNIQUE,
    tg_link_code     text NOT NULL UNIQUE,
    status           text NOT NULL,
    subtotal_cents   bigint NOT NULL,
    shipping_cents   bigint NOT NULL DEFAULT 0,
    total_cents      bigint NOT NULL,
    currency         char(3) NOT NULL,
    customer_name    text NOT NULL,
    customer_contact text NOT NULL,
    shipping_address text NOT NULL,
    comment          text,
    visitor_id       uuid,
    first_touch      jsonb NOT NULL,
    last_touch       jsonb NOT NULL,
    expires_at       timestamptz,
    paid_at          timestamptz,
    shipped_at       timestamptz,
    cancelled_at     timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX orders_status_created_idx ON orders (status, created_at DESC);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX orders_created_idx ON orders (created_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE order_items (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id         uuid NOT NULL REFERENCES orders (id),
    variant_id       uuid NOT NULL REFERENCES product_variants (id),
    product_title    text NOT NULL,
    size             text NOT NULL,
    unit_price_cents bigint NOT NULL,
    qty              integer NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX order_items_order_idx ON order_items (order_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE payments (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id            uuid NOT NULL REFERENCES orders (id),
    provider            text NOT NULL DEFAULT 'nowpayments',
    provider_payment_id text NOT NULL,
    invoice_url         text,
    pay_currency        text,
    pay_amount          numeric(38, 18),
    actually_paid       numeric(38, 18),
    status              text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_payment_id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE payment_events (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_id      uuid REFERENCES payments (id),
    provider_status text NOT NULL,
    payload         jsonb NOT NULL,
    signature_ok    boolean NOT NULL,
    dedup_key       text NOT NULL UNIQUE,
    received_at     timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE telegram_links (
    id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id  uuid NOT NULL REFERENCES orders (id),
    chat_id   bigint NOT NULL,
    username  text,
    linked_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (order_id, chat_id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX telegram_links_chat_idx ON telegram_links (chat_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE notifications (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id        uuid REFERENCES orders (id),
    chat_id         bigint NOT NULL,
    kind            text NOT NULL,
    payload         jsonb NOT NULL,
    dedup_key       text NOT NULL UNIQUE,
    status          text NOT NULL,
    attempts        integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_error      text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    sent_at         timestamptz
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX notifications_status_next_attempt_idx ON notifications (status, next_attempt_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE telegram_updates (
    update_id   bigint PRIMARY KEY,
    received_at timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE visits (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    visitor_id    uuid NOT NULL,
    session_id    uuid NOT NULL,
    landing_path  text NOT NULL,
    referrer      text,
    utm_source    text,
    utm_medium    text,
    utm_campaign  text,
    utm_content   text,
    utm_term      text,
    user_agent    text,
    is_bot        boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX visits_created_idx ON visits (created_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX visits_visitor_idx ON visits (visitor_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE events (
    id         bigserial PRIMARY KEY,
    visitor_id uuid NOT NULL,
    session_id uuid NOT NULL,
    type       text NOT NULL,
    payload    jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX events_type_created_idx ON events (type, created_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX events_session_idx ON events (session_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE settings (
    key        text PRIMARY KEY,
    value      jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS settings;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS visits;
DROP TABLE IF EXISTS telegram_updates;
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS telegram_links;
DROP TABLE IF EXISTS payment_events;
DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS cart_items;
DROP TABLE IF EXISTS carts;
DROP TABLE IF EXISTS price_history;
DROP TABLE IF EXISTS admin_sessions;
DROP TABLE IF EXISTS admin_users;
DROP TABLE IF EXISTS product_variants;
DROP TABLE IF EXISTS products;
-- +goose StatementEnd
