DROP INDEX IF EXISTS ai.ai_conversations_authorization_expiry_idx;

ALTER TABLE ai.conversations
    DROP COLUMN IF EXISTS authorization_updated_at,
    DROP COLUMN IF EXISTS authorization_expires_at,
    DROP COLUMN IF EXISTS authorization_catalog_digest,
    DROP COLUMN IF EXISTS authorization_session_id,
    DROP COLUMN IF EXISTS authorization_grant_ciphertext;
