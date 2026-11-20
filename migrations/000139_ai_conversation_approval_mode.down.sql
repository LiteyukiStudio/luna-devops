UPDATE ai.tool_calls
SET approval_decision = NULL
WHERE approval_decision IN ('mode_auto', 'mode_full_access')
  AND status IN ('proposed', 'awaiting_approval')
  AND execution_count = 0;

UPDATE ai.tool_calls
SET approval_decision = 'approve'
WHERE approval_decision IN ('mode_auto', 'mode_full_access');

ALTER TABLE ai.tool_calls
    DROP CONSTRAINT IF EXISTS tool_calls_approval_decision_check;

ALTER TABLE ai.tool_calls
    ADD CONSTRAINT tool_calls_approval_decision_check
    CHECK ((approval_decision = ANY (ARRAY['approve'::text, 'approve_and_remember'::text, 'remembered'::text])));

ALTER TABLE ai.conversations
    DROP CONSTRAINT IF EXISTS ai_conversations_approval_mode_check;

ALTER TABLE ai.conversations
    DROP COLUMN IF EXISTS approval_mode;
