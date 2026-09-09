-- +migrate Up
ALTER TABLE summaries
    ADD COLUMN used_messages_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN excluded_messages_count INTEGER NOT NULL DEFAULT 0;

UPDATE summaries s
SET used_messages_count = coverage.used_count,
    excluded_messages_count = GREATEST(s.messages_count - coverage.used_count, 0)
FROM (
    SELECT
        summary.id AS summary_id,
        COUNT(DISTINCT stm.collected_message_id)::INTEGER AS used_count
    FROM summaries summary
    LEFT JOIN summary_topics st ON st.summary_id = summary.id
    LEFT JOIN summary_topic_messages stm ON stm.topic_id = st.id
    GROUP BY summary.id
) coverage
WHERE coverage.summary_id = s.id;

CREATE TABLE summary_excluded_messages (
    id BIGSERIAL PRIMARY KEY,
    summary_id BIGINT NOT NULL REFERENCES summaries(id) ON DELETE CASCADE,
    collected_message_id BIGINT NOT NULL REFERENCES collected_messages(id) ON DELETE CASCADE,
    reason TEXT NOT NULL,
    UNIQUE (summary_id, collected_message_id)
);

CREATE INDEX summary_excluded_messages_summary_id_idx ON summary_excluded_messages (summary_id);

-- +migrate Down
DROP TABLE IF EXISTS summary_excluded_messages;

ALTER TABLE summaries
    DROP COLUMN IF EXISTS excluded_messages_count,
    DROP COLUMN IF EXISTS used_messages_count;
