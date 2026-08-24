ALTER TABLE ai.conversations
    ADD COLUMN context_usage_run_id text,
    ADD COLUMN context_usage_model_id text,
    ADD COLUMN context_used_tokens bigint,
    ADD COLUMN context_max_tokens_snapshot bigint,
    ADD COLUMN context_usage_recorded_at timestamptz,
    ADD CONSTRAINT ai_conversations_context_usage_complete CHECK (
        (
            context_usage_run_id IS NULL
            AND context_usage_model_id IS NULL
            AND context_used_tokens IS NULL
            AND context_max_tokens_snapshot IS NULL
            AND context_usage_recorded_at IS NULL
        )
        OR (
            context_usage_run_id IS NOT NULL
            AND context_usage_model_id IS NOT NULL
            AND context_used_tokens IS NOT NULL
            AND context_used_tokens >= 0
            AND context_max_tokens_snapshot IS NOT NULL
            AND context_max_tokens_snapshot > 0
            AND context_usage_recorded_at IS NOT NULL
        )
    );

WITH latest_usage AS (
    SELECT DISTINCT ON (run.conversation_id)
        run.conversation_id,
        usage.run_id,
        usage.model_id,
        usage.total_tokens,
        usage.max_context_tokens_snapshot,
        usage.occurred_at
    FROM ai.model_usages usage
    JOIN ai.runs run ON run.id = usage.run_id
    WHERE usage.operation = 'assistant' AND usage.status = 'reported'
    ORDER BY run.conversation_id, usage.occurred_at DESC, usage.id DESC
)
UPDATE ai.conversations conversation
SET context_usage_run_id = latest_usage.run_id,
    context_usage_model_id = latest_usage.model_id,
    context_used_tokens = latest_usage.total_tokens,
    context_max_tokens_snapshot = latest_usage.max_context_tokens_snapshot,
    context_usage_recorded_at = latest_usage.occurred_at
FROM latest_usage
WHERE conversation.id = latest_usage.conversation_id;

COMMENT ON COLUMN ai.conversations.context_used_tokens IS
    'Latest confirmed assistant Provider total_tokens for continuous conversation context display; not a historical sum.';
