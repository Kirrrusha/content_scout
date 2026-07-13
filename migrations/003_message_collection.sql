-- +migrate Up
CREATE TABLE message_collection_jobs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id BIGINT NOT NULL REFERENCES source_groups(id) ON DELETE CASCADE,
    mode TEXT NOT NULL,
    limit_count INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE collected_messages (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES message_collection_jobs(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    chat_id BIGINT NOT NULL REFERENCES telegram_chats(id) ON DELETE CASCADE,
    telegram_chat_id BIGINT NOT NULL,
    message_id BIGINT NOT NULL,
    date TIMESTAMPTZ NOT NULL,
    edit_date TIMESTAMPTZ,
    sender_id BIGINT NOT NULL DEFAULT 0,
    sender_name TEXT NOT NULL DEFAULT '',
    text TEXT NOT NULL DEFAULT '',
    caption TEXT NOT NULL DEFAULT '',
    url TEXT NOT NULL DEFAULT '',
    reply_to_id BIGINT,
    forwarded BOOLEAN NOT NULL DEFAULT false,
    has_media BOOLEAN NOT NULL DEFAULT false,
    media_type TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (job_id, chat_id, message_id)
);

CREATE INDEX message_collection_jobs_user_group_idx ON message_collection_jobs (user_id, group_id, status);
CREATE INDEX collected_messages_job_id_idx ON collected_messages (job_id);
CREATE INDEX collected_messages_chat_message_idx ON collected_messages (chat_id, message_id);

-- +migrate Down
DROP TABLE IF EXISTS collected_messages;
DROP TABLE IF EXISTS message_collection_jobs;
