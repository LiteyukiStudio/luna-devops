UPDATE ai.items AS item
SET status = 'failed',
    revision = item.revision + 1,
    content = jsonb_set(
        jsonb_set(item.content, '{status}', '"failed"'::jsonb, true),
        '{errorCode}',
        '"ai.agent_restarted"'::jsonb,
        true
    )
FROM ai.runs AS run
WHERE item.run_id = run.id
  AND run.status = 'waiting_external'
  AND item.status = 'streaming';

UPDATE ai.run_checkpoints
SET phase = 'model',
    state = state - 'waitingTask' - 'pendingBatchId' - 'pendingBatchCalls',
    updated_at = now()
WHERE phase = 'waiting_external';

UPDATE ai.turns
SET status = 'interrupted'
WHERE status = 'waiting_external';

UPDATE ai.runs
SET status = 'interrupted',
    completed_at = COALESCE(completed_at, now()),
    error_code = COALESCE(error_code, 'ai.agent_restarted'),
    row_version = row_version + 1
WHERE status = 'waiting_external';

DROP INDEX IF EXISTS ai.ai_runs_waiting_external_wake_idx;

ALTER TABLE ai.runs
    DROP COLUMN IF EXISTS wake_at;
