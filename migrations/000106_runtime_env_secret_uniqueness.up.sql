-- Unified runtime environment: a key is either a public value or a secret,
-- never both. Existing overlaps are resolved the same way the worker already
-- renders them (the secret wins), so running workloads are unaffected.
-- Removed public values are kept in runtime_env_conflict_backups for the down
-- migration. Audit rows and notices record only key names and counts.

CREATE FUNCTION public.luna_runtime_env_object(raw text) RETURNS jsonb
    LANGUAGE plpgsql
    IMMUTABLE
    AS $$
BEGIN
    IF raw IS NULL OR btrim(raw) = '' THEN
        RETURN '{}'::jsonb;
    END IF;
    IF jsonb_typeof(raw::jsonb) = 'object' THEN
        RETURN raw::jsonb;
    END IF;
    RETURN NULL;
EXCEPTION WHEN others THEN
    RETURN NULL;
END;
$$;

CREATE TABLE public.runtime_env_conflict_backups (
    owner_type text NOT NULL,
    owner_id text NOT NULL,
    key text NOT NULL,
    value text NOT NULL,
    migrated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT runtime_env_conflict_backups_pkey PRIMARY KEY (owner_type, owner_id, key)
);

INSERT INTO public.runtime_env_conflict_backups (owner_type, owner_id, key, value)
SELECT 'deployment_target', target.id, public_entry.key, public_entry.value
FROM public.deployment_targets AS target
CROSS JOIN LATERAL jsonb_each_text(public.luna_runtime_env_object(target.env_vars)) AS public_entry
WHERE public.luna_runtime_env_object(target.secret_refs) ? public_entry.key;

INSERT INTO public.runtime_env_conflict_backups (owner_type, owner_id, key, value)
SELECT 'runtime_config_set', config_set.id, public_entry.key, public_entry.value
FROM public.project_runtime_config_sets AS config_set
CROSS JOIN LATERAL jsonb_each_text(public.luna_runtime_env_object(config_set.env_vars)) AS public_entry
WHERE public.luna_runtime_env_object(config_set.secret_refs) ? public_entry.key;

UPDATE public.deployment_targets AS target
SET env_vars = (public.luna_runtime_env_object(target.env_vars) - conflict.keys)::text
FROM (
    SELECT owner_id, array_agg(key) AS keys
    FROM public.runtime_env_conflict_backups
    WHERE owner_type = 'deployment_target'
    GROUP BY owner_id
) AS conflict
WHERE target.id = conflict.owner_id;

UPDATE public.project_runtime_config_sets AS config_set
SET env_vars = (public.luna_runtime_env_object(config_set.env_vars) - conflict.keys)::text
FROM (
    SELECT owner_id, array_agg(key) AS keys
    FROM public.runtime_env_conflict_backups
    WHERE owner_type = 'runtime_config_set'
    GROUP BY owner_id
) AS conflict
WHERE config_set.id = conflict.owner_id;

INSERT INTO public.audit_logs (id, user_id, action, resource, success, message, metadata, created_at)
SELECT
    'aud_' || substr(md5(owner_type || ':' || owner_id || ':runtime_env_conflict'), 1, 24),
    '',
    owner_type || '.runtime_env.conflict_resolved',
    owner_id,
    true,
    'public runtime variables shadowed by secrets were removed',
    jsonb_build_object('count', count(*), 'keys', jsonb_agg(key ORDER BY key)),
    now()
FROM public.runtime_env_conflict_backups
GROUP BY owner_type, owner_id;

DO $$
DECLARE
    resolved_count bigint;
    owner_count bigint;
    unparsable_count bigint;
BEGIN
    SELECT count(*), count(DISTINCT (owner_type, owner_id))
    INTO resolved_count, owner_count
    FROM public.runtime_env_conflict_backups;
    SELECT
        (SELECT count(*) FROM public.deployment_targets
         WHERE public.luna_runtime_env_object(env_vars) IS NULL OR public.luna_runtime_env_object(secret_refs) IS NULL)
      + (SELECT count(*) FROM public.project_runtime_config_sets
         WHERE public.luna_runtime_env_object(env_vars) IS NULL OR public.luna_runtime_env_object(secret_refs) IS NULL)
    INTO unparsable_count;
    RAISE NOTICE 'runtime environment name conflicts resolved: % keys across % owners; % owners skipped with unparsable JSON',
        resolved_count, owner_count, unparsable_count;
END;
$$;

DROP FUNCTION public.luna_runtime_env_object(text);
