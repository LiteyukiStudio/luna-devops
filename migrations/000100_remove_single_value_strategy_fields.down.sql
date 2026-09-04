ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS namespace_strategy text NOT NULL DEFAULT 'project';

ALTER TABLE runtime_clusters
    ADD COLUMN IF NOT EXISTS type text NOT NULL DEFAULT 'kubernetes',
    ADD COLUMN IF NOT EXISTS gateway_provider text NOT NULL DEFAULT 'gateway-api';
