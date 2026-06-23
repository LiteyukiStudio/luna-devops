ALTER TABLE runtime_clusters
  ADD COLUMN IF NOT EXISTS gateway_root_domain text NOT NULL DEFAULT 'apps.local',
  ADD COLUMN IF NOT EXISTS gateway_public_scheme text NOT NULL DEFAULT 'http';

UPDATE runtime_clusters
SET gateway_root_domain = COALESCE(NULLIF((SELECT value FROM app_configs WHERE key = 'gateway.rootDomain'), ''), 'apps.local')
WHERE gateway_root_domain = '' OR gateway_root_domain = 'apps.local';

UPDATE runtime_clusters
SET gateway_public_scheme = CASE
  WHEN LOWER(COALESCE(NULLIF((SELECT value FROM app_configs WHERE key = 'gateway.publicScheme'), ''), 'http')) = 'https' THEN 'https'
  ELSE 'http'
END
WHERE gateway_public_scheme = '' OR gateway_public_scheme = 'http';
