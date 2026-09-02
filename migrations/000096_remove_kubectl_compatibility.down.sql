-- Intentionally irreversible. Restoring the retired compatibility gateway,
-- generated credentials, scopes, or their runtime observations would
-- reintroduce a removed product and cannot recover deleted data. Only the
-- pre-existing target-period constraint shape is restored so the remaining
-- version 95 schema can continue rolling back safely.
DROP INDEX IF EXISTS idx_runtime_observations_target_period;

ALTER TABLE runtime_observations
    DROP CONSTRAINT IF EXISTS runtime_observations_target_period_unique,
    ALTER COLUMN deployment_target_id SET NOT NULL,
    ADD CONSTRAINT runtime_observations_target_period_unique
        UNIQUE (deployment_target_id, period_start);
