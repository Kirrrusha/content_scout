-- +migrate Up
CREATE TABLE jobs (
    id BIGSERIAL PRIMARY KEY,
    type TEXT NOT NULL,
    status TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    attempt INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 4,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_at TIMESTAMPTZ,
    locked_by TEXT,
    lease_expires_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    deduplication_key TEXT
);

CREATE UNIQUE INDEX jobs_deduplication_key_active_idx
    ON jobs (deduplication_key)
    WHERE deduplication_key IS NOT NULL
      AND status IN ('pending', 'running', 'retry_wait');

CREATE INDEX jobs_claim_idx
    ON jobs (status, available_at, created_at)
    WHERE status IN ('pending', 'retry_wait');

CREATE INDEX jobs_lease_idx
    ON jobs (lease_expires_at)
    WHERE status = 'running';

-- +migrate Down
DROP TABLE IF EXISTS jobs;
