ALTER TABLE ai.runs
    ADD COLUMN selected_operation_ids text[] NOT NULL DEFAULT '{}';

COMMENT ON COLUMN ai.runs.selected_operation_ids IS
    'Run-scoped LRU operationId selection only; full catalog schemas remain in the Agent catalog snapshot.';
