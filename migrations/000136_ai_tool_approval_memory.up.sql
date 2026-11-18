CREATE TABLE IF NOT EXISTS ai.conversation_tool_approvals (
    conversation_id text NOT NULL REFERENCES ai.conversations(id) ON DELETE CASCADE,
    operation_id text NOT NULL,
    granted_by_tool_call_id text NOT NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    PRIMARY KEY (conversation_id, operation_id),
    CONSTRAINT conversation_tool_approvals_operation_id_nonempty CHECK (length(btrim(operation_id)) > 0),
    CONSTRAINT conversation_tool_approvals_tool_call_id_nonempty CHECK (length(btrim(granted_by_tool_call_id)) > 0)
);

ALTER TABLE ai.tool_calls
    DROP CONSTRAINT IF EXISTS tool_calls_approval_decision_check;

ALTER TABLE ai.tool_calls
    ADD CONSTRAINT tool_calls_approval_decision_check
    CHECK ((approval_decision = ANY (ARRAY['approve'::text, 'approve_and_remember'::text, 'remembered'::text])));
