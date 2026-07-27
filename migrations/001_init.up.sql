CREATE TABLE IF NOT EXISTS products (
    id          SERIAL PRIMARY KEY,
    slug        VARCHAR(50) UNIQUE NOT NULL,
    name        VARCHAR(100) NOT NULL,
    description TEXT,
    price       INTEGER NOT NULL,
    currency    VARCHAR(10) DEFAULT 'KZT',
    is_active   BOOLEAN DEFAULT TRUE,
    created_at  TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS product_images (
    id          SERIAL PRIMARY KEY,
    product_id  INTEGER REFERENCES products(id) ON DELETE CASCADE,
    side        VARCHAR(10) NOT NULL CHECK (side IN ('front','back')),
    filename    VARCHAR(255) NOT NULL,
    sort_order  INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS product_variants (
    id          SERIAL PRIMARY KEY,
    product_id  INTEGER REFERENCES products(id) ON DELETE CASCADE,
    size        VARCHAR(5) NOT NULL CHECK (size IN ('S','M','L','XL')),
    stock       INTEGER DEFAULT 0,
    UNIQUE (product_id, size)
);

CREATE TABLE IF NOT EXISTS orders (
    id               SERIAL PRIMARY KEY,
    uuid             UUID UNIQUE DEFAULT gen_random_uuid(),
    session_id       VARCHAR(100),
    status           VARCHAR(20) DEFAULT 'pending'
                     CHECK (status IN ('pending','awaiting_payment','paid','cancelled')),
    total_amount     INTEGER NOT NULL,
    currency         VARCHAR(10) DEFAULT 'KZT',
    customer_name    VARCHAR(100),
    customer_email   VARCHAR(200),
    customer_phone   VARCHAR(50),
    delivery_address TEXT,
    nowpayments_id   VARCHAR(100),
    created_at       TIMESTAMP DEFAULT NOW(),
    updated_at       TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS order_items (
    id          SERIAL PRIMARY KEY,
    order_id    INTEGER REFERENCES orders(id) ON DELETE CASCADE,
    product_id  INTEGER REFERENCES products(id),
    variant_id  INTEGER REFERENCES product_variants(id),
    size        VARCHAR(5) NOT NULL,
    quantity    INTEGER NOT NULL DEFAULT 1,
    price       INTEGER NOT NULL
);

-- Seed
INSERT INTO products (slug, name, description, price) VALUES
('white', 'qzq.kiim · ШАЙ White',
 'Оверсайз футболка. Пиала с казахским орнаментом на груди. Белая.',
 790000)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO products (slug, name, description, price) VALUES
('black', 'qzq.kiim · ШАЙ Black',
 'Оверсайз футболка. Пиала с казахским орнаментом на груди. Чёрная.',
 790000)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO products (slug, name, description, price) VALUES
('besh', 'qzq.kiim · BESH',
 'Оверсайз футболка. Бешбармак на груди, BESH SAVED MY LIFE на спине.',
 790000)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO product_variants (product_id, size, stock)
SELECT id, unnest(ARRAY['S','M','L','XL']), 10
FROM products WHERE slug IN ('white','black','besh')
ON CONFLICT (product_id, size) DO NOTHING;

INSERT INTO product_images (product_id, side, filename, sort_order)
SELECT id, 'front', 'qzqwhitefront-removebg-preview.png', 1 FROM products WHERE slug = 'white'
ON CONFLICT DO NOTHING;

INSERT INTO product_images (product_id, side, filename, sort_order)
SELECT id, 'back', 'qzqwhiteback-removebg-preview.png', 2 FROM products WHERE slug = 'white'
ON CONFLICT DO NOTHING;

INSERT INTO product_images (product_id, side, filename, sort_order)
SELECT id, 'front', 'qzqblackfront-removebg-preview.png', 1 FROM products WHERE slug = 'black'
ON CONFLICT DO NOTHING;

INSERT INTO product_images (product_id, side, filename, sort_order)
SELECT id, 'back', 'qzqblackback-removebg-preview.png', 2 FROM products WHERE slug = 'black'
ON CONFLICT DO NOTHING;

INSERT INTO product_images (product_id, side, filename, sort_order)
SELECT id, 'front', 'beshfront-removebg-preview.png', 1 FROM products WHERE slug = 'besh'
ON CONFLICT DO NOTHING;

INSERT INTO product_images (product_id, side, filename, sort_order)
SELECT id, 'back', 'beshback-removebg-preview.png', 2 FROM products WHERE slug = 'besh'
ON CONFLICT DO NOTHING;
