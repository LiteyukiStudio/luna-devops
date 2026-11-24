ALTER TABLE public.runtime_observations RESET (
    autovacuum_vacuum_scale_factor,
    autovacuum_analyze_scale_factor
);

ALTER TABLE ai.run_events RESET (
    autovacuum_vacuum_scale_factor,
    autovacuum_analyze_scale_factor
);

DROP INDEX IF EXISTS ai.idx_ai_runs_retention_terminal;
