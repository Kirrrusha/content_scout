-- +migrate Up
CREATE TABLE summary_schedules (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id BIGINT NOT NULL REFERENCES source_groups(id) ON DELETE CASCADE,
    cron TEXT NOT NULL,
    timezone TEXT NOT NULL DEFAULT 'UTC',
    enabled BOOLEAN NOT NULL DEFAULT true,
    summary_type TEXT NOT NULL DEFAULT 'standard',
    quiet_hours_start TEXT NOT NULL DEFAULT '',
    quiet_hours_end TEXT NOT NULL DEFAULT '',
    export_to_obsidian BOOLEAN NOT NULL DEFAULT false,
    last_run_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE schedule_runs (
    id BIGSERIAL PRIMARY KEY,
    schedule_id BIGINT NOT NULL REFERENCES summary_schedules(id) ON DELETE CASCADE,
    collection_job_id BIGINT REFERENCES message_collection_jobs(id) ON DELETE SET NULL,
    summary_id BIGINT REFERENCES summaries(id) ON DELETE SET NULL,
    export_id BIGINT REFERENCES obsidian_exports(id) ON DELETE SET NULL,
    status TEXT NOT NULL,
    error TEXT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX summary_schedules_due_idx ON summary_schedules (enabled, last_run_at);
CREATE INDEX schedule_runs_schedule_id_idx ON schedule_runs (schedule_id, started_at DESC);

-- +migrate Down
DROP TABLE IF EXISTS schedule_runs;
DROP TABLE IF EXISTS summary_schedules;
