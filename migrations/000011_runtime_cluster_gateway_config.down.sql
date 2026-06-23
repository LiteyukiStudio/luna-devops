ALTER TABLE runtime_clusters
  DROP COLUMN IF EXISTS gateway_public_scheme,
  DROP COLUMN IF EXISTS gateway_root_domain;
