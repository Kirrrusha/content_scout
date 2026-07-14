-- +migrate Up
ALTER TABLE jobs
    ADD COLUMN result JSONB NOT NULL DEFAULT '{}'::jsonb;

-- +migrate Down
ALTER TABLE jobs
    DROP COLUMN IF EXISTS result;
