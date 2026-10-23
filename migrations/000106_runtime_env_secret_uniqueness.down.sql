-- Restore public values removed by the up migration and drop its audit rows.

UPDATE public.deployment_targets AS target
SET env_vars = (
    CASE WHEN btrim(target.env_vars) = '' THEN '{}'::jsonb ELSE target.env_vars::jsonb END
    || restored.entries
)::text
FROM (
    SELECT owner_id, jsonb_object_agg(key, value) AS entries
    FROM public.runtime_env_conflict_backups
    WHERE owner_type = 'deployment_target'
    GROUP BY owner_id
) AS restored
WHERE target.id = restored.owner_id;

UPDATE public.project_runtime_config_sets AS config_set
SET env_vars = (
    CASE WHEN btrim(config_set.env_vars) = '' THEN '{}'::jsonb ELSE config_set.env_vars::jsonb END
    || restored.entries
)::text
FROM (
    SELECT owner_id, jsonb_object_agg(key, value) AS entries
    FROM public.runtime_env_conflict_backups
    WHERE owner_type = 'runtime_config_set'
    GROUP BY owner_id
) AS restored
WHERE config_set.id = restored.owner_id;

DELETE FROM public.audit_logs
WHERE action IN ('deployment_target.runtime_env.conflict_resolved', 'runtime_config_set.runtime_env.conflict_resolved');

DROP TABLE public.runtime_env_conflict_backups;
