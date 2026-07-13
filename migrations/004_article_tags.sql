-- +migrate Up
ALTER TABLE articles ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}';

-- +migrate Down
ALTER TABLE articles DROP COLUMN IF EXISTS tags;
