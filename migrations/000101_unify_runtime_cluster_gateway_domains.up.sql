WITH raw_suffixes AS (
    SELECT cluster.id AS cluster_id,
           lower(trim(BOTH '.' FROM btrim(candidate.value))) AS suffix,
           candidate.position
    FROM runtime_clusters AS cluster
    CROSS JOIN LATERAL regexp_split_to_table(cluster.gateway_domain_suffixes, E'[\n,;]+')
        WITH ORDINALITY AS candidate(value, position)
    WHERE lower(trim(BOTH '.' FROM btrim(candidate.value))) <> ''
), fallback_cluster AS (
    SELECT cluster.id
    FROM runtime_clusters AS cluster
    WHERE cluster.scope = 'global'
      AND cluster.delete_status = 'active'
      AND cluster.deleted_at IS NULL
    ORDER BY CASE WHEN cluster.is_default THEN 0 ELSE 1 END,
             cluster.created_at,
             cluster.id
    LIMIT 1
), base_suffixes AS (
    SELECT raw.cluster_id,
           raw.suffix,
           0 AS priority,
           raw.position
    FROM raw_suffixes AS raw

    UNION ALL

    SELECT cluster.id AS cluster_id,
           lower(trim(BOTH '.' FROM btrim(cluster.gateway_root_domain))) AS suffix,
           0 AS priority,
           1::bigint AS position
    FROM runtime_clusters AS cluster
    WHERE NOT EXISTS (
        SELECT 1 FROM raw_suffixes AS raw WHERE raw.cluster_id = cluster.id
    )
      AND lower(trim(BOTH '.' FROM btrim(cluster.gateway_root_domain))) <> ''

    UNION ALL

    SELECT cluster.id AS cluster_id,
           lower(trim(BOTH '.' FROM btrim(config.value))) AS suffix,
           0 AS priority,
           1::bigint AS position
    FROM runtime_clusters AS cluster
    JOIN app_configs AS config ON config.key = 'gateway.rootDomain'
    WHERE NOT EXISTS (
        SELECT 1 FROM raw_suffixes AS raw WHERE raw.cluster_id = cluster.id
    )
      AND lower(trim(BOTH '.' FROM btrim(cluster.gateway_root_domain))) = ''
      AND lower(trim(BOTH '.' FROM btrim(config.value))) <> ''

    UNION ALL

    SELECT cluster.id AS cluster_id,
           'apps.local' AS suffix,
           0 AS priority,
           1::bigint AS position
    FROM runtime_clusters AS cluster
    WHERE NOT EXISTS (
        SELECT 1 FROM raw_suffixes AS raw WHERE raw.cluster_id = cluster.id
    )
      AND lower(trim(BOTH '.' FROM btrim(cluster.gateway_root_domain))) = ''
      AND NOT EXISTS (
          SELECT 1
          FROM app_configs AS config
          WHERE config.key = 'gateway.rootDomain'
            AND lower(trim(BOTH '.' FROM btrim(config.value))) <> ''
      )
), route_suffixes AS (
    SELECT COALESCE(NULLIF(btrim(target.cluster_id), ''), fallback.id) AS cluster_id,
           lower(trim(BOTH '.' FROM btrim(route.domain_suffix))) AS suffix,
           1 AS priority,
           row_number() OVER (ORDER BY route.created_at, route.id)::bigint AS position
    FROM gateway_routes AS route
    JOIN deployment_targets AS target ON target.id = route.deployment_target_id
    LEFT JOIN fallback_cluster AS fallback ON true
    WHERE COALESCE(NULLIF(btrim(target.cluster_id), ''), fallback.id) IS NOT NULL
      AND lower(trim(BOTH '.' FROM btrim(route.domain_suffix))) <> ''
), suffix_candidates AS (
    SELECT cluster_id, suffix, priority, position FROM base_suffixes
    UNION ALL
    SELECT cluster_id, suffix, priority, position FROM route_suffixes
), normalized_suffixes AS (
    SELECT DISTINCT ON (cluster_id, suffix)
           cluster_id,
           suffix,
           priority,
           position
    FROM suffix_candidates
    ORDER BY cluster_id, suffix, priority, position
)
UPDATE runtime_clusters AS cluster
SET gateway_domain_suffixes = COALESCE(
    (
        SELECT string_agg(suffix, E'\n' ORDER BY priority, position, suffix)
        FROM normalized_suffixes
        WHERE cluster_id = cluster.id
    ),
    'apps.local'
);

ALTER TABLE runtime_clusters
    DROP COLUMN IF EXISTS gateway_root_domain;

DELETE FROM app_configs
WHERE key IN ('gateway.rootDomain', 'gateway.publicScheme');
