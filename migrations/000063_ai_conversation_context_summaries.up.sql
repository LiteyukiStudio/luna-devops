CREATE TABLE ai.conversation_summaries (
    conversation_id text PRIMARY KEY REFERENCES ai.conversations(id) ON DELETE CASCADE,
    covered_through_turn_index integer NOT NULL CHECK (covered_through_turn_index >= 0),
    compression_version integer NOT NULL CHECK (compression_version = 1),
    source_turn_count integer NOT NULL CHECK (source_turn_count > 0),
    content jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ai_conversation_summaries_updated_idx
    ON ai.conversation_summaries (updated_at DESC);
