-- +migrate Up
CREATE TABLE summary_topic_sources (
    id BIGSERIAL PRIMARY KEY,
    topic_id BIGINT NOT NULL REFERENCES summary_topics(id) ON DELETE CASCADE,
    chat_id BIGINT NOT NULL REFERENCES telegram_chats(id) ON DELETE CASCADE,
    telegram_chat_id BIGINT NOT NULL,
    title TEXT NOT NULL,
    username TEXT,
    UNIQUE (topic_id, chat_id)
);

CREATE INDEX summary_topic_sources_topic_id_idx ON summary_topic_sources (topic_id);

-- +migrate Down
DROP TABLE IF EXISTS summary_topic_sources;
