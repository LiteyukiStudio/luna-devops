CREATE TABLE IF NOT EXISTS project_ai_automation_configs (
    project_id text PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    delivery_diagnosis_enabled boolean NOT NULL DEFAULT false,
    delivery_billing_user_id text NOT NULL DEFAULT '',
    delivery_cooldown_seconds integer NOT NULL DEFAULT 300 CHECK (delivery_cooldown_seconds >= 0 AND delivery_cooldown_seconds <= 86400),
    delivery_daily_limit integer NOT NULL DEFAULT 20 CHECK (delivery_daily_limit >= 1 AND delivery_daily_limit <= 500),
    updated_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ai_delivery_diagnoses (
    id text PRIMARY KEY,
    project_id text NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    application_id text NOT NULL DEFAULT '',
    deployment_target_id text NOT NULL DEFAULT '',
    subject_type text NOT NULL CHECK (subject_type IN ('build_run', 'release')),
    subject_id text NOT NULL,
    billed_user_id text NOT NULL DEFAULT '',
    ai_run_id text NOT NULL DEFAULT '',
    status text NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    failure_code text NOT NULL DEFAULT '',
    root_cause_summary text NOT NULL DEFAULT '',
    confidence text NOT NULL DEFAULT 'medium' CHECK (confidence IN ('high', 'medium', 'low')),
    suggestions_json text NOT NULL DEFAULT '[]',
    proposed_action_json text NOT NULL DEFAULT '',
    error_code text NOT NULL DEFAULT '',
    notification_emitted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_ai_delivery_diagnoses_subject
    ON ai_delivery_diagnoses (subject_type, subject_id);

CREATE INDEX IF NOT EXISTS idx_ai_delivery_diagnoses_project_created
    ON ai_delivery_diagnoses (project_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ai_delivery_diagnoses_reconcile
    ON ai_delivery_diagnoses (status, created_at)
    WHERE notification_emitted_at IS NULL;
