DELETE FROM access_tokens
WHERE source = 'kubeconfig';

DROP TABLE IF EXISTS kube_access_bindings;

ALTER TABLE runtime_clusters
    DROP COLUMN IF EXISTS kube_gateway_cleanup_completed_at,
    DROP COLUMN IF EXISTS kube_gateway_drain_until,
    DROP COLUMN IF EXISTS kube_gateway_extra_resource_rules,
    DROP COLUMN IF EXISTS kube_gateway_enabled;

DELETE FROM runtime_observations
WHERE deployment_target_id IS NULL OR deployment_target_id = '';

-- The retired compatibility schema keyed observations by cluster/resource,
-- so one deployment target could legitimately have more than one row for a
-- period after moving between clusters. Restore the platform-owned key by
-- keeping the most recently observed row deterministically.
DELETE FROM runtime_observations AS observation
USING (
    SELECT id
    FROM (
        SELECT
            id,
            row_number() OVER (
                PARTITION BY deployment_target_id, period_start
                ORDER BY observed_at DESC, updated_at DESC, id DESC
            ) AS position
        FROM runtime_observations
    ) AS ranked
    WHERE ranked.position > 1
) AS duplicate
WHERE observation.id = duplicate.id;

DROP INDEX IF EXISTS idx_runtime_observations_application_id;
DROP INDEX IF EXISTS idx_runtime_observations_resource_uid;
DROP INDEX IF EXISTS idx_runtime_observations_resource_kind;
DROP INDEX IF EXISTS idx_runtime_observations_management_source;
DROP INDEX IF EXISTS idx_runtime_observations_resource_period;

ALTER TABLE runtime_observations
    DROP CONSTRAINT IF EXISTS runtime_observations_management_source_check,
    DROP CONSTRAINT IF EXISTS runtime_observations_target_period_unique,
    DROP COLUMN IF EXISTS application_id,
    DROP COLUMN IF EXISTS resource_uid,
    DROP COLUMN IF EXISTS resource_kind,
    DROP COLUMN IF EXISTS management_source,
    ALTER COLUMN deployment_target_id SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_runtime_observations_target_period
    ON runtime_observations(deployment_target_id, period_start);

UPDATE access_tokens
SET scope = (
    SELECT COALESCE(string_agg(part.value, ',' ORDER BY part.ordinality), '')
    FROM regexp_split_to_table(access_tokens.scope, '[[:space:],]+')
        WITH ORDINALITY AS part(value, ordinality)
    WHERE part.value <> '' AND part.value NOT LIKE 'kube:%'
)
WHERE scope ~ '(^|[[:space:],])kube:';

DELETE FROM access_tokens
WHERE btrim(scope) = '';

WITH stripped_oauth_application_scopes AS (
    SELECT
        application.id,
        COALESCE(string_agg(part.value, ',' ORDER BY part.ordinality), '') AS scope
    FROM oauth_applications AS application
    CROSS JOIN LATERAL regexp_split_to_table(application.allowed_scopes, '[[:space:],]+')
        WITH ORDINALITY AS part(value, ordinality)
    WHERE application.allowed_scopes ~ '(^|[[:space:],])kube:'
      AND part.value <> ''
      AND part.value NOT LIKE 'kube:%'
    GROUP BY application.id
    UNION ALL
    SELECT application.id, ''
    FROM oauth_applications AS application
    WHERE application.allowed_scopes ~ '(^|[[:space:],])kube:'
      AND NOT EXISTS (
          SELECT 1
          FROM regexp_split_to_table(application.allowed_scopes, '[[:space:],]+') AS part(value)
          WHERE part.value <> '' AND part.value NOT LIKE 'kube:%'
      )
)
UPDATE oauth_applications AS application
SET allowed_scopes = remaining.scope,
    revoked_at = CASE
        WHEN remaining.scope = '' THEN COALESCE(application.revoked_at, now())
        ELSE application.revoked_at
    END,
    updated_at = now()
FROM stripped_oauth_application_scopes AS remaining
WHERE application.id = remaining.id;

UPDATE oauth_grants
SET scope = (
    SELECT COALESCE(string_agg(part.value, ',' ORDER BY part.ordinality), '')
    FROM regexp_split_to_table(oauth_grants.scope, '[[:space:],]+')
        WITH ORDINALITY AS part(value, ordinality)
    WHERE part.value <> '' AND part.value NOT LIKE 'kube:%'
)
WHERE scope ~ '(^|[[:space:],])kube:';

UPDATE oauth_authorization_codes
SET scope = (
    SELECT COALESCE(string_agg(part.value, ',' ORDER BY part.ordinality), '')
    FROM regexp_split_to_table(oauth_authorization_codes.scope, '[[:space:],]+')
        WITH ORDINALITY AS part(value, ordinality)
    WHERE part.value <> '' AND part.value NOT LIKE 'kube:%'
)
WHERE scope ~ '(^|[[:space:],])kube:';

UPDATE oauth_refresh_tokens
SET scope = (
    SELECT COALESCE(string_agg(part.value, ',' ORDER BY part.ordinality), '')
    FROM regexp_split_to_table(oauth_refresh_tokens.scope, '[[:space:],]+')
        WITH ORDINALITY AS part(value, ordinality)
    WHERE part.value <> '' AND part.value NOT LIKE 'kube:%'
)
WHERE scope ~ '(^|[[:space:],])kube:';

UPDATE oauth_device_authorizations
SET scope = (
    SELECT COALESCE(string_agg(part.value, ',' ORDER BY part.ordinality), '')
    FROM regexp_split_to_table(oauth_device_authorizations.scope, '[[:space:],]+')
        WITH ORDINALITY AS part(value, ordinality)
    WHERE part.value <> '' AND part.value NOT LIKE 'kube:%'
)
WHERE scope ~ '(^|[[:space:],])kube:';

-- Empty scopes are invalid. Purge the retired grants and their transient
-- credentials instead of allowing device authorization to substitute a
-- recommended default scope on a later approval.
DELETE FROM access_tokens
WHERE oauth_grant_id IN (
    SELECT id FROM oauth_grants WHERE btrim(scope) = ''
);

DELETE FROM oauth_authorization_codes
WHERE btrim(scope) = '';

DELETE FROM oauth_refresh_tokens
WHERE btrim(scope) = '';

DELETE FROM oauth_device_authorizations
WHERE btrim(scope) = '';

DELETE FROM oauth_grants
WHERE btrim(scope) = '';
