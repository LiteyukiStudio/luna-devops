DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM (
            SELECT route.environment_id, route.deployment_target_id, target.id AS target_id,
                   target.environment_id AS target_environment_id
            FROM gateway_routes AS route
            LEFT JOIN deployment_targets AS target ON target.id = route.deployment_target_id

            UNION ALL

            SELECT run.environment_id, run.deployment_target_id, target.id AS target_id,
                   target.environment_id AS target_environment_id
            FROM hook_runs AS run
            LEFT JOIN deployment_targets AS target ON target.id = run.deployment_target_id

            UNION ALL

            SELECT release.environment_id, release.deployment_target_id, target.id AS target_id,
                   target.environment_id AS target_environment_id
            FROM releases AS release
            LEFT JOIN deployment_targets AS target ON target.id = release.deployment_target_id
        ) AS link
        WHERE btrim(link.environment_id) <> ''
          AND (
              btrim(link.deployment_target_id) = ''
              OR link.target_id IS NULL
              OR link.environment_id <> link.target_environment_id
          )
    ) THEN
        RAISE EXCEPTION 'environment domain removal found child rows without an equivalent deployment target association';
    END IF;
END
$$;

DROP INDEX IF EXISTS idx_deployment_targets_app_env_name_active;
DROP INDEX IF EXISTS idx_deployment_targets_environment_id;
DROP INDEX IF EXISTS idx_gateway_routes_environment_id;
DROP INDEX IF EXISTS idx_hook_runs_environment_id;
DROP INDEX IF EXISTS idx_releases_environment_id;

ALTER TABLE deployment_targets DROP COLUMN IF EXISTS environment_id;
ALTER TABLE gateway_routes DROP COLUMN IF EXISTS environment_id;
ALTER TABLE hook_runs DROP COLUMN IF EXISTS environment_id;
ALTER TABLE releases DROP COLUMN IF EXISTS environment_id;
