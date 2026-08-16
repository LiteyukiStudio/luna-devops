ALTER TABLE deployment_targets
  ALTER COLUMN stage SET DEFAULT 'prod',
  ALTER COLUMN build_cpu_request SET DEFAULT '1',
  ALTER COLUMN build_memory_request SET DEFAULT '1Gi';

ALTER TABLE build_runs
  ALTER COLUMN build_cpu_request SET DEFAULT '1',
  ALTER COLUMN build_memory_request SET DEFAULT '1Gi';
