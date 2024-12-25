-- +goose Up
-- +goose StatementBegin
CREATE TABLE products (
    id              SERIAL PRIMARY KEY NOT NULL,
    name            TEXT NOT NULL,
    price           BIGINT NOT NULL,
    stock           BIGINT NOT NULL,  
    size            TEXT NOT NULL,
    article_number  BIGINT NOT NULL,
    description     TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE products;
-- +goose StatementEnd
