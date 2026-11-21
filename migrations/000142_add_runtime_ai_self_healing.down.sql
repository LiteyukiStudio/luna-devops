UPDATE public.inbox_messages
SET action_request_id = '', category = 'delivery'
WHERE action_request_id IN (
    SELECT id FROM public.inbox_action_requests WHERE type = 'runtime.self_heal'
);
DELETE FROM public.inbox_action_requests WHERE type = 'runtime.self_heal';
DROP INDEX IF EXISTS idx_inbox_action_requests_pending_self_heal_recipient;
DROP INDEX IF EXISTS idx_inbox_action_requests_pending_project_type;
CREATE UNIQUE INDEX idx_inbox_action_requests_pending_project_type
    ON public.inbox_action_requests USING btree (project_id, type)
    WHERE ((project_id <> ''::text) AND (status = ANY (ARRAY['pending'::text, 'processing'::text])));

DROP INDEX IF EXISTS idx_ai_runtime_diagnoses_reconcile;
DROP INDEX IF EXISTS idx_ai_runtime_diagnoses_incident;
DROP INDEX IF EXISTS idx_ai_runtime_diagnoses_target_created;
DROP INDEX IF EXISTS idx_ai_runtime_diagnoses_project_created;
DROP TABLE IF EXISTS ai_runtime_incident_diagnoses;

ALTER TABLE project_ai_automation_configs
    DROP COLUMN IF EXISTS circuit_breaker_window_seconds,
    DROP COLUMN IF EXISTS max_consecutive_auto_heals,
    DROP COLUMN IF EXISTS allowed_auto_heal_tools_json,
    DROP COLUMN IF EXISTS stage_self_heal_modes_json,
    DROP COLUMN IF EXISTS self_heal_mode,
    DROP COLUMN IF EXISTS runtime_daily_limit,
    DROP COLUMN IF EXISTS runtime_cooldown_seconds,
    DROP COLUMN IF EXISTS runtime_billing_user_id,
    DROP COLUMN IF EXISTS runtime_diagnosis_enabled;
