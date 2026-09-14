-- Move legacy retained PVC records into the project volume center. The
-- Kubernetes claim remains untouched until its first managed mutation.

CREATE FUNCTION public.luna_retained_volume_capacity_bytes(raw_capacity text) RETURNS bigint
    LANGUAGE plpgsql
    IMMUTABLE STRICT
    AS $$
DECLARE
    normalized text := btrim(raw_capacity);
    parts text[];
    multiplier numeric;
    capacity_bytes numeric;
    decimal_exponent integer;
BEGIN
    parts := regexp_match(normalized, '^([0-9]+)(Ki|Mi|Gi|Ti|Pi|Ei|k|M|G|T|P|E)?$');
    IF parts IS NOT NULL THEN
        multiplier := CASE COALESCE(parts[2], '')
            WHEN '' THEN 1
            WHEN 'k' THEN 1000
            WHEN 'M' THEN 1000000
            WHEN 'G' THEN 1000000000
            WHEN 'T' THEN 1000000000000
            WHEN 'P' THEN 1000000000000000
            WHEN 'E' THEN 1000000000000000000
            WHEN 'Ki' THEN 1024
            WHEN 'Mi' THEN 1048576
            WHEN 'Gi' THEN 1073741824
            WHEN 'Ti' THEN 1099511627776
            WHEN 'Pi' THEN 1125899906842624
            WHEN 'Ei' THEN 1152921504606846976
        END;
        capacity_bytes := parts[1]::numeric * multiplier;
    ELSE
        parts := regexp_match(normalized, '^([0-9]+)[eE]([+-]?[0-9]+)$');
        IF parts IS NULL THEN
            RAISE EXCEPTION USING
                ERRCODE = 'PVR04',
                MESSAGE = 'legacy_retained_volume_capacity_invalid';
        END IF;
        decimal_exponent := parts[2]::integer;
        IF decimal_exponent < -18 OR decimal_exponent > 18 THEN
            RAISE EXCEPTION USING
                ERRCODE = 'PVR04',
                MESSAGE = 'legacy_retained_volume_capacity_invalid';
        END IF;
        capacity_bytes := parts[1]::numeric * power(10::numeric, decimal_exponent);
    END IF;

    IF capacity_bytes <= 0
       OR capacity_bytes <> trunc(capacity_bytes)
       OR capacity_bytes > 9223372036854775807 THEN
        RAISE EXCEPTION USING
            ERRCODE = 'PVR04',
            MESSAGE = 'legacy_retained_volume_capacity_invalid';
    END IF;
    RETURN capacity_bytes::bigint;
EXCEPTION
    WHEN invalid_text_representation OR numeric_value_out_of_range THEN
        RAISE EXCEPTION USING
            ERRCODE = 'PVR04',
            MESSAGE = 'legacy_retained_volume_capacity_invalid';
END;
$$;

CREATE TEMP TABLE luna_retained_volume_bridge_candidates ON COMMIT DROP AS
WITH active_global_runtime_clusters AS (
    SELECT cluster.id
    FROM public.runtime_clusters AS cluster
    WHERE cluster.deleted_at IS NULL
      AND cluster.delete_status = 'active'
      AND cluster.scope = 'global'
),
single_global_runtime_cluster AS (
    SELECT min(cluster.id) AS id
    FROM active_global_runtime_clusters AS cluster
    HAVING count(*) = 1
),
resolved_retained_candidates AS (
    SELECT
        retained.*,
        active_cluster.id AS resolved_cluster_id,
        'pvol_' || left(encode(sha256(
            convert_to('luna-devops/volume-center-backfill/v1', 'UTF8') || decode('00', 'hex') ||
            convert_to('pvol', 'UTF8') || decode('00', 'hex') ||
            convert_to('retained', 'UTF8') || decode('00', 'hex') ||
            convert_to(btrim(retained.id), 'UTF8')
        ), 'hex'), 24) AS project_volume_id
    FROM public.retained_volumes AS retained
    JOIN public.projects AS active_project
      ON active_project.id = retained.project_id
     AND active_project.deleted_at IS NULL
     AND active_project.delete_status = 'active'
    LEFT JOIN single_global_runtime_cluster AS default_cluster
      ON btrim(retained.cluster_id) = ''
    JOIN public.runtime_clusters AS active_cluster
      ON active_cluster.id = COALESCE(NULLIF(btrim(retained.cluster_id), ''), default_cluster.id)
     AND active_cluster.deleted_at IS NULL
     AND active_cluster.delete_status = 'active'
    WHERE retained.status = 'retained'
),
retained_candidates AS (
    SELECT retained.*
    FROM resolved_retained_candidates AS retained
    WHERE NOT EXISTS (
          SELECT 1
          FROM public.project_volumes AS volume
          WHERE volume.id = retained.project_volume_id
             OR (
                 volume.cluster_id = retained.resolved_cluster_id
                 AND volume.namespace = retained.namespace
                 AND volume.claim_name = retained.claim_name
                 AND volume.deleted_at IS NULL
             )
      )
)
SELECT
    retained.id AS legacy_id,
    retained.project_volume_id,
    retained.project_id,
    'migrated-' || left(btrim(retained.claim_name), 86) || '-' ||
        substr(retained.project_volume_id, 6, 24) AS display_name,
    retained.resolved_cluster_id AS cluster_id,
    retained.namespace,
    retained.claim_name,
    btrim(retained.capacity) AS capacity_request,
    public.luna_retained_volume_capacity_bytes(retained.capacity) AS capacity_bytes,
    btrim(retained.storage_class_name) AS storage_class_name,
    btrim(retained.access_mode) AS access_mode,
    COALESCE(NULLIF(btrim(retained.volume_mode), ''), 'Filesystem') AS volume_mode,
    CASE
        WHEN EXISTS (
            SELECT 1 FROM public.applications AS application
            WHERE application.id = retained.source_application_id
              AND application.project_id = retained.project_id
        ) THEN NULLIF(btrim(retained.source_application_id), '')
        ELSE NULL
    END AS source_application_id,
    retained.source_application_name,
    CASE
        WHEN EXISTS (
            SELECT 1 FROM public.deployment_targets AS target
            WHERE target.id = retained.source_deployment_target_id
              AND target.project_id = retained.project_id
        ) THEN NULLIF(btrim(retained.source_deployment_target_id), '')
        ELSE NULL
    END AS source_deployment_target_id,
    retained.retained_at AS created_at
FROM retained_candidates AS retained;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM luna_retained_volume_bridge_candidates
        WHERE btrim(legacy_id) = ''
           OR btrim(project_id) = ''
           OR btrim(cluster_id) = ''
           OR btrim(namespace) = ''
           OR btrim(claim_name) = ''
           OR access_mode NOT IN ('ReadWriteOnce', 'ReadWriteOncePod', 'ReadOnlyMany', 'ReadWriteMany')
           OR volume_mode NOT IN ('Filesystem', 'Block')
    ) THEN
        RAISE EXCEPTION USING
            ERRCODE = 'PVR05',
            MESSAGE = 'legacy_retained_volume_spec_invalid';
    END IF;
END;
$$;

-- Match the quota migration's treatment of already-existing physical assets:
-- account for them as committed usage without applying today's creation cap.
SELECT project.id
FROM public.projects AS project
WHERE project.id IN (
    SELECT DISTINCT project_id
    FROM luna_retained_volume_bridge_candidates
)
ORDER BY project.id
FOR UPDATE;

ALTER TABLE public.project_volumes DISABLE TRIGGER trg_project_volumes_quota_insert;

INSERT INTO public.project_volumes (
    id,
    project_id,
    display_name,
    cluster_id,
    namespace,
    claim_name,
    ownership_mode,
    source_kind,
    source_snapshot_name,
    lifecycle_state,
    pending_operation,
    capacity_request,
    capacity_bytes,
    storage_class_name,
    access_mode,
    volume_mode,
    source_application_id,
    source_application_name,
    source_deployment_target_id,
    created_by,
    revision,
    created_at,
    updated_at
)
SELECT
    project_volume_id,
    project_id,
    display_name,
    cluster_id,
    namespace,
    claim_name,
    'managed',
    'retained',
    '',
    'ready',
    '',
    capacity_request,
    capacity_bytes,
    storage_class_name,
    access_mode,
    volume_mode,
    source_application_id,
    source_application_name,
    source_deployment_target_id,
    'system:volume-center-migration',
    1,
    created_at,
    created_at
FROM luna_retained_volume_bridge_candidates;

ALTER TABLE public.project_volumes ENABLE TRIGGER trg_project_volumes_quota_insert;

INSERT INTO public.project_volume_quota_reservations (
    project_volume_id,
    project_id,
    committed_bytes,
    pending_bytes,
    updated_at
)
SELECT project_volume_id, project_id, capacity_bytes, 0, now()
FROM luna_retained_volume_bridge_candidates;

INSERT INTO public.project_volume_quota_usage (project_id, reserved_bytes, updated_at)
SELECT
    affected.project_id,
    sum(reservation.committed_bytes + reservation.pending_bytes),
    now()
FROM (
    SELECT DISTINCT project_id
    FROM luna_retained_volume_bridge_candidates
) AS affected
JOIN public.project_volume_quota_reservations AS reservation
  ON reservation.project_id = affected.project_id
GROUP BY affected.project_id
ON CONFLICT (project_id) DO UPDATE SET
    reserved_bytes = EXCLUDED.reserved_bytes,
    updated_at = EXCLUDED.updated_at;

DROP FUNCTION public.luna_retained_volume_capacity_bytes(text);
