-- +migrate Up
CREATE TABLE telegram_sessions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    storage_path TEXT NOT NULL,
    status TEXT NOT NULL,
    last_connected TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id)
);

CREATE TABLE telegram_folders (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    telegram_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    synced_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, telegram_id)
);

CREATE TABLE telegram_chats (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    telegram_chat_id BIGINT NOT NULL,
    title TEXT NOT NULL,
    username TEXT,
    type TEXT NOT NULL,
    is_archived BOOLEAN NOT NULL DEFAULT false,
    is_muted BOOLEAN NOT NULL DEFAULT false,
    unread_count INTEGER NOT NULL DEFAULT 0,
    last_message_id BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, telegram_chat_id)
);

CREATE TABLE source_groups (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE source_group_chats (
    group_id BIGINT NOT NULL REFERENCES source_groups(id) ON DELETE CASCADE,
    chat_id BIGINT NOT NULL REFERENCES telegram_chats(id) ON DELETE CASCADE,
    priority INTEGER NOT NULL DEFAULT 0,
    enabled BOOLEAN NOT NULL DEFAULT true,
    PRIMARY KEY (group_id, chat_id)
);

CREATE TABLE read_positions (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    chat_id BIGINT NOT NULL REFERENCES telegram_chats(id) ON DELETE CASCADE,
    last_summarized_message_id BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, chat_id)
);

CREATE TABLE summary_jobs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_type TEXT NOT NULL,
    source_id BIGINT NOT NULL,
    status TEXT NOT NULL,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE summaries (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES summary_jobs(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    overview TEXT NOT NULL,
    messages_count INTEGER NOT NULL DEFAULT 0,
    sources_count INTEGER NOT NULL DEFAULT 0,
    topics_count INTEGER NOT NULL DEFAULT 0,
    markdown TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE summary_topics (
    id BIGSERIAL PRIMARY KEY,
    summary_id BIGINT NOT NULL REFERENCES summaries(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    short_summary TEXT NOT NULL,
    full_summary TEXT NOT NULL,
    category TEXT NOT NULL,
    importance INTEGER NOT NULL DEFAULT 0,
    confidence TEXT NOT NULL,
    messages_count INTEGER NOT NULL DEFAULT 0,
    sources_count INTEGER NOT NULL DEFAULT 0,
    position INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE articles (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    slug TEXT NOT NULL,
    type TEXT NOT NULL,
    status TEXT NOT NULL,
    content_markdown TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, slug)
);

CREATE TABLE article_sources (
    id BIGSERIAL PRIMARY KEY,
    article_id BIGINT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    telegram_chat_id BIGINT NOT NULL,
    message_id BIGINT NOT NULL,
    source_title TEXT NOT NULL,
    source_url TEXT NOT NULL,
    published_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE obsidian_exports (
    id BIGSERIAL PRIMARY KEY,
    article_id BIGINT REFERENCES articles(id) ON DELETE SET NULL,
    summary_id BIGINT REFERENCES summaries(id) ON DELETE SET NULL,
    file_name TEXT NOT NULL,
    vault_path TEXT NOT NULL,
    export_method TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    exported_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (content_hash)
);

CREATE INDEX telegram_chats_user_id_idx ON telegram_chats (user_id);
CREATE INDEX source_groups_user_id_idx ON source_groups (user_id);
CREATE INDEX summary_jobs_user_id_status_idx ON summary_jobs (user_id, status);
CREATE INDEX summaries_job_id_idx ON summaries (job_id);
CREATE INDEX articles_user_id_status_idx ON articles (user_id, status);

-- +migrate Down
DROP TABLE IF EXISTS obsidian_exports;
DROP TABLE IF EXISTS article_sources;
DROP TABLE IF EXISTS articles;
DROP TABLE IF EXISTS summary_topics;
DROP TABLE IF EXISTS summaries;
DROP TABLE IF EXISTS summary_jobs;
DROP TABLE IF EXISTS read_positions;
DROP TABLE IF EXISTS source_group_chats;
DROP TABLE IF EXISTS source_groups;
DROP TABLE IF EXISTS telegram_chats;
DROP TABLE IF EXISTS telegram_folders;
DROP TABLE IF EXISTS telegram_sessions;
