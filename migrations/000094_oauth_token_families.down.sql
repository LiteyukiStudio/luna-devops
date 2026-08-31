-- Authorization codes created by the family-aware flow have no grant until
-- exchange. A rollback cannot represent those pending codes in the old schema,
-- so retain them as consumed records linked to revoked tombstone grants.
INSERT INTO oauth_grants (
    id,
    application_id,
    user_id,
    scope,
    revoked_at,
    created_at,
    updated_at
)
SELECT
    'ogrt_rollback_' || substr(md5(code.id), 1, 24),
    code.application_id,
    code.user_id,
    code.scope,
    now(),
    now(),
    now()
FROM oauth_authorization_codes AS code
WHERE code.grant_id IS NULL
ON CONFLICT (id) DO NOTHING;

UPDATE oauth_authorization_codes
SET
    grant_id = 'ogrt_rollback_' || substr(md5(id), 1, 24),
    consumed_at = COALESCE(consumed_at, now())
WHERE grant_id IS NULL;

ALTER TABLE oauth_authorization_codes
    ALTER COLUMN grant_id SET NOT NULL;

DROP INDEX IF EXISTS idx_oauth_refresh_tokens_family_id;

ALTER TABLE oauth_refresh_tokens
    DROP COLUMN family_id;

DROP INDEX IF EXISTS idx_access_tokens_oauth_family_id;

ALTER TABLE access_tokens
    DROP COLUMN oauth_family_id;
