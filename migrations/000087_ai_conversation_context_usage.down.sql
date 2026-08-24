ALTER TABLE ai.conversations
    DROP CONSTRAINT ai_conversations_context_usage_complete,
    DROP COLUMN context_usage_recorded_at,
    DROP COLUMN context_max_tokens_snapshot,
    DROP COLUMN context_used_tokens,
    DROP COLUMN context_usage_model_id,
    DROP COLUMN context_usage_run_id;
