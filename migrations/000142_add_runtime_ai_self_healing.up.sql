ALTER TABLE project_ai_automation_configs
    ADD COLUMN IF NOT EXISTS runtime_diagnosis_enabled boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS runtime_billing_user_id text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS runtime_cooldown_seconds integer NOT NULL DEFAULT 300 CHECK (runtime_cooldown_seconds >= 0 AND runtime_cooldown_seconds <= 86400),
    ADD COLUMN IF NOT EXISTS runtime_daily_limit integer NOT NULL DEFAULT 20 CHECK (runtime_daily_limit >= 1 AND runtime_daily_limit <= 500),
    ADD COLUMN IF NOT EXISTS self_heal_mode text NOT NULL DEFAULT 'suggest_only' CHECK (self_heal_mode IN ('suggest_only', 'auto_l1', 'auto_l2')),
    ADD COLUMN IF NOT EXISTS stage_self_heal_modes_json text NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS allowed_auto_heal_tools_json text NOT NULL DEFAULT '[]',
    ADD COLUMN IF NOT EXISTS max_consecutive_auto_heals integer NOT NULL DEFAULT 2 CHECK (max_consecutive_auto_heals >= 1 AND max_consecutive_auto_heals <= 10),
    ADD COLUMN IF NOT EXISTS circuit_breaker_window_seconds integer NOT NULL DEFAULT 1800 CHECK (circuit_breaker_window_seconds >= 60 AND circuit_breaker_window_seconds <= 86400);

CREATE TABLE IF NOT EXISTS ai_runtime_incident_diagnoses (
    id text PRIMARY KEY,
    project_id text NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    application_id text NOT NULL DEFAULT '',
    deployment_target_id text NOT NULL DEFAULT '',
    incident_id text NOT NULL,
    billed_user_id text NOT NULL DEFAULT '',
    ai_run_id text NOT NULL DEFAULT '',
    status text NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    anomaly_type text NOT NULL DEFAULT '',
    root_cause_summary text NOT NULL DEFAULT '',
    confidence text NOT NULL DEFAULT 'medium' CHECK (confidence IN ('high', 'medium', 'low')),
    suggestions_json text NOT NULL DEFAULT '[]',
    proposed_action_json text NOT NULL DEFAULT '',
    auto_heal_mode text NOT NULL DEFAULT 'suggest_only' CHECK (auto_heal_mode IN ('suggest_only', 'auto_l1', 'auto_l2')),
    auto_heal_status text NOT NULL DEFAULT 'none' CHECK (auto_heal_status IN ('none', 'succeeded', 'failed', 'circuit_broken')),
    auto_heal_summary text NOT NULL DEFAULT '',
    circuit_breaker_tripped boolean NOT NULL DEFAULT false,
    error_code text NOT NULL DEFAULT '',
    inbox_synced_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ai_runtime_diagnoses_project_created
    ON ai_runtime_incident_diagnoses (project_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ai_runtime_diagnoses_target_created
    ON ai_runtime_incident_diagnoses (deployment_target_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ai_runtime_diagnoses_incident
    ON ai_runtime_incident_diagnoses (incident_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ai_runtime_diagnoses_reconcile
    ON ai_runtime_incident_diagnoses (status, created_at)
    WHERE inbox_synced_at IS NULL;

DROP INDEX IF EXISTS idx_inbox_action_requests_pending_project_type;
CREATE UNIQUE INDEX idx_inbox_action_requests_pending_project_type
    ON public.inbox_action_requests USING btree (project_id, type)
    WHERE ((project_id <> ''::text) AND (type <> 'runtime.self_heal'::text) AND (status = ANY (ARRAY['pending'::text, 'processing'::text])));

CREATE UNIQUE INDEX IF NOT EXISTS idx_inbox_action_requests_pending_self_heal_recipient
    ON public.inbox_action_requests USING btree (project_id, resource_id, recipient_user_id)
    WHERE ((type = 'runtime.self_heal'::text) AND (status = ANY (ARRAY['pending'::text, 'processing'::text])));
