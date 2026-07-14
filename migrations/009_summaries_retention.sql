-- +migrate Up
CREATE INDEX summaries_created_at_idx ON summaries (created_at);

-- +migrate Down
DROP INDEX IF EXISTS summaries_created_at_idx;
