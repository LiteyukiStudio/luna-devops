INSERT INTO app_configs (key, value, updated_at)
VALUES ('ai.access.mode', 'all_authenticated', now())
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value,
    updated_at = EXCLUDED.updated_at
WHERE app_configs.value = 'admins';
