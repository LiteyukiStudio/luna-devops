DROP TABLE IF EXISTS ai.conversation_tool_approvals;

UPDATE ai.tool_calls
SET approval_decision = NULL
WHERE approval_decision = 'remembered'
  AND status IN ('proposed', 'awaiting_approval');

UPDATE ai.tool_calls
SET approval_decision = 'approve'
WHERE approval_decision IN ('approve_and_remember', 'remembered');

ALTER TABLE ai.tool_calls
    DROP CONSTRAINT IF EXISTS tool_calls_approval_decision_check;

ALTER TABLE ai.tool_calls
    ADD CONSTRAINT tool_calls_approval_decision_check
    CHECK ((approval_decision = 'approve'::text));
