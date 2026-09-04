ALTER TABLE runtime_clusters
    ADD COLUMN IF NOT EXISTS gateway_root_domain text NOT NULL DEFAULT 'apps.local';

UPDATE runtime_clusters
SET gateway_root_domain = COALESCE(
    (
        SELECT btrim(candidate.value)
        FROM regexp_split_to_table(runtime_clusters.gateway_domain_suffixes, E'[\n,;]+')
            WITH ORDINALITY AS candidate(value, position)
        WHERE btrim(candidate.value) <> ''
        ORDER BY candidate.position
        LIMIT 1
    ),
    'apps.local'
);
