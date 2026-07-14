-- +migrate Up
CREATE TABLE summary_topic_messages (
    id BIGSERIAL PRIMARY KEY,
    topic_id BIGINT NOT NULL REFERENCES summary_topics(id) ON DELETE CASCADE,
    collected_message_id BIGINT NOT NULL REFERENCES collected_messages(id) ON DELETE CASCADE,
    cluster_index INTEGER NOT NULL DEFAULT 0,
    is_canonical BOOLEAN NOT NULL DEFAULT false,
    UNIQUE (topic_id, collected_message_id)
);

CREATE INDEX summary_topic_messages_topic_id_idx ON summary_topic_messages (topic_id);
CREATE INDEX summary_topic_messages_collected_message_id_idx ON summary_topic_messages (collected_message_id);

-- +migrate Down
DROP TABLE IF EXISTS summary_topic_messages;
