ALTER TABLE ai.conversations
    ADD COLUMN authorization_grant_ciphertext text,
    ADD COLUMN authorization_session_id text,
    ADD COLUMN authorization_catalog_digest text,
    ADD COLUMN authorization_expires_at timestamptz,
    ADD COLUMN authorization_updated_at timestamptz;

CREATE INDEX ai_conversations_authorization_expiry_idx
    ON ai.conversations (authorization_expires_at)
    WHERE authorization_grant_ciphertext IS NOT NULL;
