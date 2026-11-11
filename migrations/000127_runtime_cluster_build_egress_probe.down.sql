ALTER TABLE runtime_clusters DROP COLUMN IF EXISTS network_policy_probed_at;
ALTER TABLE runtime_clusters DROP COLUMN IF EXISTS network_policy_probe_code;
ALTER TABLE runtime_clusters DROP COLUMN IF EXISTS network_policy_enforcement;
ALTER TABLE runtime_clusters DROP COLUMN IF EXISTS build_private_egress_ports;
ALTER TABLE runtime_clusters DROP COLUMN IF EXISTS build_private_egress_cidrs;
ALTER TABLE runtime_clusters DROP COLUMN IF EXISTS build_egress_mode;
