ALTER TABLE ai.tool_calls
    ADD COLUMN arguments_ciphertext text;

COMMENT ON COLUMN ai.tool_calls.arguments IS
    'Redacted tool arguments for audit and UI projection only.';

COMMENT ON COLUMN ai.tool_calls.arguments_ciphertext IS
    'Authenticated encrypted executable tool arguments; never expose to clients.';
