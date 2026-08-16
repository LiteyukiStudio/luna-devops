ALTER TABLE deployment_targets
  ALTER COLUMN stage DROP DEFAULT,
  ALTER COLUMN build_cpu_request SET DEFAULT '2',
  ALTER COLUMN build_memory_request SET DEFAULT '4Gi';

ALTER TABLE build_runs
  ALTER COLUMN build_cpu_request SET DEFAULT '2',
  ALTER COLUMN build_memory_request SET DEFAULT '4Gi';
