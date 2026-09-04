DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM projects WHERE namespace_strategy <> 'project') THEN
        RAISE EXCEPTION 'projects contains unsupported namespace_strategy values';
    END IF;
    IF EXISTS (SELECT 1 FROM runtime_clusters WHERE type NOT IN ('kubernetes', 'k3s')) THEN
        RAISE EXCEPTION 'runtime_clusters contains unsupported type values';
    END IF;
    IF EXISTS (SELECT 1 FROM runtime_clusters WHERE gateway_provider <> 'gateway-api') THEN
        RAISE EXCEPTION 'runtime_clusters contains unsupported gateway_provider values';
    END IF;
END
$$;

ALTER TABLE projects
    DROP COLUMN IF EXISTS namespace_strategy;

ALTER TABLE runtime_clusters
    DROP COLUMN IF EXISTS type,
    DROP COLUMN IF EXISTS gateway_provider;
