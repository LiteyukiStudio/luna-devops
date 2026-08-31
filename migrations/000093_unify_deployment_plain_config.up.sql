DO $migration$
DECLARE
    invalid_target_id text;
BEGIN
    SELECT candidate.id
      INTO invalid_target_id
      FROM (
          SELECT id,
                 CASE WHEN btrim(env_vars) = '' THEN '{}'::jsonb ELSE env_vars::jsonb END
                 || CASE WHEN btrim(config_refs) = '' THEN '{}'::jsonb ELSE config_refs::jsonb END AS merged_values
            FROM deployment_targets
      ) AS candidate
     WHERE jsonb_typeof(candidate.merged_values) <> 'object'
     LIMIT 1;

    IF invalid_target_id IS NOT NULL THEN
        RAISE EXCEPTION 'deployment target % has non-object plain configuration', invalid_target_id;
    END IF;

    SELECT candidate.id
      INTO invalid_target_id
      FROM (
          SELECT id,
                 CASE WHEN btrim(env_vars) = '' THEN '{}'::jsonb ELSE env_vars::jsonb END
                 || CASE WHEN btrim(config_refs) = '' THEN '{}'::jsonb ELSE config_refs::jsonb END AS merged_values
            FROM deployment_targets
      ) AS candidate
     WHERE (SELECT count(*) FROM jsonb_object_keys(candidate.merged_values)) > 128
        OR EXISTS (
            SELECT 1
              FROM jsonb_each_text(candidate.merged_values) AS entry(entry_key, entry_value)
             WHERE length(entry.entry_key) > 128
                OR entry.entry_key !~ '^[A-Za-z_][A-Za-z0-9_]*$'
                OR char_length(entry.entry_value) > 8192
        )
     LIMIT 1;

    IF invalid_target_id IS NOT NULL THEN
        RAISE EXCEPTION 'deployment target % has plain configuration that cannot be migrated safely', invalid_target_id;
    END IF;
END
$migration$;

UPDATE deployment_targets
   SET env_vars = (
       CASE WHEN btrim(env_vars) = '' THEN '{}'::jsonb ELSE env_vars::jsonb END
       || CASE WHEN btrim(config_refs) = '' THEN '{}'::jsonb ELSE config_refs::jsonb END
   )::text
 WHERE CASE WHEN btrim(config_refs) = '' THEN '{}'::jsonb ELSE config_refs::jsonb END <> '{}'::jsonb;

ALTER TABLE deployment_targets
    DROP COLUMN config_refs;
