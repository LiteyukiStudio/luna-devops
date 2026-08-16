CREATE TABLE IF NOT EXISTS runtime_observations (
  id text PRIMARY KEY,
  deployment_target_id text NOT NULL REFERENCES deployment_targets(id) ON DELETE CASCADE,
  period_start timestamptz NOT NULL,
  period_end timestamptz NOT NULL,
  desired_replicas integer NOT NULL,
  updated_replicas integer NOT NULL,
  ready_replicas integer NOT NULL,
  available_replicas integer NOT NULL,
  cpu_request text NOT NULL,
  memory_request text NOT NULL,
  workload_created_at timestamptz NOT NULL,
  status text NOT NULL,
  observation_code text NOT NULL,
  observed_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT runtime_observations_period_valid CHECK (period_end > period_start),
  CONSTRAINT runtime_observations_replicas_nonnegative CHECK (
    desired_replicas >= 0 AND updated_replicas >= 0 AND ready_replicas >= 0 AND available_replicas >= 0
  ),
  CONSTRAINT runtime_observations_target_period_unique UNIQUE (deployment_target_id, period_start)
);

CREATE INDEX IF NOT EXISTS idx_runtime_observations_period ON runtime_observations(period_start, period_end);
