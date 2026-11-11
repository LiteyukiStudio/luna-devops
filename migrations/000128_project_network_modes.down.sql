ALTER TABLE deployment_targets DROP COLUMN IF EXISTS internal_only;
ALTER TABLE projects DROP COLUMN IF EXISTS network_egress_allowlist;
ALTER TABLE projects DROP COLUMN IF EXISTS network_egress_mode;
ALTER TABLE projects DROP COLUMN IF EXISTS network_ingress_mode;
