-- +migrate Up
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    telegram_user_id BIGINT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX users_telegram_user_id_idx ON users (telegram_user_id);

-- +migrate Down
DROP TABLE IF EXISTS users;
