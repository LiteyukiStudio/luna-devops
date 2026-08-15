CREATE TABLE project_volume_quota_usage (
    project_id text PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    reserved_bytes bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_project_volume_quota_usage_reserved_bytes CHECK (reserved_bytes >= 0)
);

CREATE TABLE project_volume_quota_reservations (
    project_volume_id text PRIMARY KEY REFERENCES project_volumes(id) ON DELETE CASCADE,
    project_id text NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    committed_bytes bigint NOT NULL DEFAULT 0,
    pending_bytes bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_project_volume_quota_reservation_committed CHECK (committed_bytes >= 0),
    CONSTRAINT chk_project_volume_quota_reservation_pending CHECK (pending_bytes >= 0),
    CONSTRAINT chk_project_volume_quota_reservation_nonempty CHECK (committed_bytes + pending_bytes > 0)
);

CREATE INDEX idx_project_volume_quota_reservations_project
    ON project_volume_quota_reservations(project_id, project_volume_id);

-- Existing managed assets remain authoritative during upgrade. A failed
-- initial provision/import has no reservation, while failed expansion/delete
-- conservatively keeps its known desired capacity because the old observed
-- capacity is not available in this desired-state table.
WITH reservations AS (
    SELECT
        id AS project_volume_id,
        project_id,
        CASE
            WHEN lifecycle_state IN ('ready', 'deleting') THEN capacity_bytes
            WHEN lifecycle_state = 'error' AND pending_operation IN ('expand', 'delete') THEN capacity_bytes
            ELSE 0
        END AS committed_bytes,
        CASE
            WHEN lifecycle_state = 'provisioning' THEN capacity_bytes
            ELSE 0
        END AS pending_bytes
    FROM project_volumes
    WHERE deleted_at IS NULL
      AND ownership_mode = 'managed'
)
INSERT INTO project_volume_quota_reservations (
    project_volume_id,
    project_id,
    committed_bytes,
    pending_bytes,
    updated_at
)
SELECT
    project_volume_id,
    project_id,
    committed_bytes,
    pending_bytes,
    now()
FROM reservations
WHERE committed_bytes + pending_bytes > 0;

INSERT INTO project_volume_quota_usage (project_id, reserved_bytes, updated_at)
SELECT project_id, SUM(committed_bytes + pending_bytes), now()
FROM project_volume_quota_reservations
GROUP BY project_id;

CREATE OR REPLACE FUNCTION luna_project_volume_quota_limit_bytes()
RETURNS bigint
LANGUAGE plpgsql
AS $$
DECLARE
    raw_limit text;
    limit_gib numeric;
BEGIN
    SELECT value
    INTO raw_limit
    FROM app_configs
    WHERE key = 'storage.projectManagedCapacityLimitGiB';

    IF raw_limit IS NULL OR btrim(raw_limit) = '' OR btrim(raw_limit) = '0' THEN
        RETURN 0;
    END IF;
    IF btrim(raw_limit) !~ '^[0-9]+$' THEN
        RAISE EXCEPTION USING
            ERRCODE = 'PVR02',
            MESSAGE = 'project_volume_quota_config_invalid',
            CONSTRAINT = 'project_volume_quota_config';
    END IF;

    limit_gib := btrim(raw_limit)::numeric;
    IF limit_gib < 0 OR limit_gib > 1048576 THEN
        RAISE EXCEPTION USING
            ERRCODE = 'PVR02',
            MESSAGE = 'project_volume_quota_config_invalid',
            CONSTRAINT = 'project_volume_quota_config';
    END IF;
    RETURN (limit_gib * 1073741824)::bigint;
EXCEPTION
    WHEN invalid_text_representation OR numeric_value_out_of_range THEN
        RAISE EXCEPTION USING
            ERRCODE = 'PVR02',
            MESSAGE = 'project_volume_quota_config_invalid',
            CONSTRAINT = 'project_volume_quota_config';
END;
$$;

CREATE OR REPLACE FUNCTION luna_sync_project_volume_quota()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    old_committed bigint := 0;
    old_pending bigint := 0;
    old_total bigint := 0;
    next_committed bigint := 0;
    next_pending bigint := 0;
    next_total bigint := 0;
    usage_bytes bigint := 0;
    quota_limit_bytes bigint := 0;
    delta_bytes bigint := 0;
BEGIN
    IF TG_OP = 'UPDATE' AND NEW.project_id IS DISTINCT FROM OLD.project_id THEN
        RAISE EXCEPTION USING
            ERRCODE = 'PVR03',
            MESSAGE = 'project_volume_quota_project_immutable',
            CONSTRAINT = 'project_volume_quota_project_immutable';
    END IF;

    IF TG_OP = 'UPDATE'
       AND NEW.project_id IS NOT DISTINCT FROM OLD.project_id
       AND NEW.ownership_mode IS NOT DISTINCT FROM OLD.ownership_mode
       AND NEW.capacity_bytes IS NOT DISTINCT FROM OLD.capacity_bytes
       AND NEW.lifecycle_state IS NOT DISTINCT FROM OLD.lifecycle_state
       AND NEW.pending_operation IS NOT DISTINCT FROM OLD.pending_operation
       AND NEW.deleted_at IS NOT DISTINCT FROM OLD.deleted_at THEN
        RETURN NEW;
    END IF;

    -- Serializing on the durable project row makes reservations safe across
    -- API/Worker replicas without a process-local mutex or advisory lock.
    PERFORM id FROM projects WHERE id = NEW.project_id FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING
            ERRCODE = 'PVR03',
            MESSAGE = 'project_volume_quota_project_missing',
            CONSTRAINT = 'project_volume_quota_project';
    END IF;

    INSERT INTO project_volume_quota_usage(project_id, reserved_bytes, updated_at)
    VALUES (NEW.project_id, 0, now())
    ON CONFLICT (project_id) DO NOTHING;

    SELECT committed_bytes, pending_bytes
    INTO old_committed, old_pending
    FROM project_volume_quota_reservations
    WHERE project_volume_id = NEW.id
    FOR UPDATE;
    IF NOT FOUND THEN
        old_committed := 0;
        old_pending := 0;
    END IF;
    old_total := old_committed + old_pending;

    IF NEW.deleted_at IS NOT NULL OR NEW.ownership_mode <> 'managed' THEN
        next_committed := 0;
        next_pending := 0;
    ELSIF NEW.pending_operation IN ('provision', 'import') THEN
        next_committed := old_committed;
        IF NEW.lifecycle_state = 'error' THEN
            next_pending := 0;
        ELSE
            next_pending := GREATEST(NEW.capacity_bytes - next_committed, 0);
        END IF;
    ELSIF NEW.pending_operation = 'expand' THEN
        next_committed := old_committed;
        IF NEW.lifecycle_state = 'error' THEN
            next_pending := 0;
        ELSE
            next_pending := GREATEST(NEW.capacity_bytes - next_committed, 0);
        END IF;
    ELSIF NEW.pending_operation = 'delete' THEN
        next_committed := old_committed;
        next_pending := 0;
    ELSIF NEW.lifecycle_state = 'ready' THEN
        next_committed := NEW.capacity_bytes;
        next_pending := 0;
    ELSE
        next_committed := old_committed;
        next_pending := old_pending;
    END IF;

    next_total := next_committed + next_pending;
    delta_bytes := next_total - old_total;

    SELECT reserved_bytes
    INTO usage_bytes
    FROM project_volume_quota_usage
    WHERE project_id = NEW.project_id
    FOR UPDATE;
    IF delta_bytes > 0 THEN
        quota_limit_bytes := luna_project_volume_quota_limit_bytes();
        IF quota_limit_bytes > 0
           AND usage_bytes + delta_bytes > quota_limit_bytes THEN
            RAISE EXCEPTION USING
                ERRCODE = 'PVR01',
                MESSAGE = 'project_volume_quota_exceeded',
                CONSTRAINT = 'project_volume_quota_limit';
        END IF;
    END IF;

    IF next_total = 0 THEN
        DELETE FROM project_volume_quota_reservations
        WHERE project_volume_id = NEW.id;
    ELSE
        INSERT INTO project_volume_quota_reservations (
            project_volume_id,
            project_id,
            committed_bytes,
            pending_bytes,
            updated_at
        ) VALUES (
            NEW.id,
            NEW.project_id,
            next_committed,
            next_pending,
            now()
        )
        ON CONFLICT (project_volume_id) DO UPDATE SET
            project_id = EXCLUDED.project_id,
            committed_bytes = EXCLUDED.committed_bytes,
            pending_bytes = EXCLUDED.pending_bytes,
            updated_at = EXCLUDED.updated_at;
    END IF;

    UPDATE project_volume_quota_usage
    SET reserved_bytes = reserved_bytes + delta_bytes,
        updated_at = now()
    WHERE project_id = NEW.project_id;

    SELECT reserved_bytes
    INTO usage_bytes
    FROM project_volume_quota_usage
    WHERE project_id = NEW.project_id;
    IF usage_bytes < 0 THEN
        RAISE EXCEPTION USING
            ERRCODE = 'PVR03',
            MESSAGE = 'project_volume_quota_invariant_failed',
            CONSTRAINT = 'project_volume_quota_nonnegative';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_project_volumes_quota_insert
AFTER INSERT ON project_volumes
FOR EACH ROW
EXECUTE FUNCTION luna_sync_project_volume_quota();

CREATE TRIGGER trg_project_volumes_quota_update
AFTER UPDATE OF project_id, ownership_mode, capacity_bytes, lifecycle_state, pending_operation, deleted_at
ON project_volumes
FOR EACH ROW
EXECUTE FUNCTION luna_sync_project_volume_quota();

INSERT INTO billing_rate_rules (
    id,
    meter,
    unit,
    credits_per_unit,
    enabled,
    description,
    created_at,
    updated_at
)
VALUES (
    'brte_' || LEFT(md5('storage.transfer_gib'), 24),
    'storage.transfer_gib',
    'gib',
    0,
    false,
    'Volume transfer bytes',
    now(),
    now()
)
ON CONFLICT (meter) DO NOTHING;
