ALTER TABLE ai.conversations
    ADD COLUMN approval_mode text DEFAULT 'default'::text NOT NULL;

ALTER TABLE ai.conversations
    ADD CONSTRAINT ai_conversations_approval_mode_check
    CHECK ((approval_mode = ANY (ARRAY['strict'::text, 'default'::text, 'auto'::text, 'full_access'::text])));

ALTER TABLE ai.tool_calls
    DROP CONSTRAINT IF EXISTS tool_calls_approval_decision_check;

ALTER TABLE ai.tool_calls
    ADD CONSTRAINT tool_calls_approval_decision_check
    CHECK ((approval_decision = ANY (ARRAY['approve'::text, 'approve_and_remember'::text, 'remembered'::text, 'mode_auto'::text, 'mode_full_access'::text])));
