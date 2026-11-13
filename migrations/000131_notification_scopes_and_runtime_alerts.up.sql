-- 通知渠道作用域与跨项目空间绑定、通知规则目标覆盖与告警节流策略、运行时巡检事件状态机。
ALTER TABLE notification_channels
    ADD COLUMN IF NOT EXISTS scope text DEFAULT 'global'::text NOT NULL,
    ADD COLUMN IF NOT EXISTS owner_ref text DEFAULT ''::text NOT NULL,
    ADD COLUMN IF NOT EXISTS preset_id text DEFAULT ''::text NOT NULL;

UPDATE notification_channels
SET scope = 'user',
    owner_ref = owner_user_id
WHERE owner_user_id <> '';

UPDATE notification_channels
SET scope = 'project'
WHERE owner_user_id = '' AND project_id <> '';

INSERT INTO scoped_resource_project_bindings (id, resource_type, resource_id, project_id, is_default, created_at, updated_at)
SELECT
    'srpb_' || md5(id || ':' || project_id),
    'notification_channel',
    id,
    project_id,
    false,
    COALESCE(created_at, now()),
    COALESCE(updated_at, now())
FROM notification_channels
WHERE owner_user_id = '' AND project_id <> ''
ON CONFLICT (resource_type, resource_id, project_id) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_notification_channels_scope_owner_ref
    ON notification_channels USING btree (scope, owner_ref);

ALTER TABLE notification_rules
    ADD COLUMN IF NOT EXISTS channel_targets_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    ADD COLUMN IF NOT EXISTS remind_interval_minutes integer DEFAULT 60 NOT NULL,
    ADD COLUMN IF NOT EXISTS max_reminders integer DEFAULT 3 NOT NULL,
    ADD COLUMN IF NOT EXISTS notify_on_recovery boolean DEFAULT true NOT NULL,
    ADD COLUMN IF NOT EXISTS time_zone text DEFAULT 'Asia/Shanghai'::text NOT NULL;

ALTER TABLE notification_deliveries
    ADD COLUMN IF NOT EXISTS target_override_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    ADD COLUMN IF NOT EXISTS time_zone text DEFAULT ''::text NOT NULL;

CREATE TABLE IF NOT EXISTS runtime_alert_incidents (
    id text PRIMARY KEY,
    project_id text NOT NULL,
    application_id text DEFAULT ''::text NOT NULL,
    deployment_target_id text DEFAULT ''::text NOT NULL,
    resource_id text DEFAULT ''::text NOT NULL,
    incident_kind text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    fingerprint text DEFAULT ''::text NOT NULL,
    consecutive_failures integer DEFAULT 1 NOT NULL,
    first_detected_at timestamp with time zone NOT NULL,
    last_observed_at timestamp with time zone NOT NULL,
    last_notified_at timestamp with time zone,
    notification_count integer DEFAULT 0 NOT NULL,
    rule_notifications_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    last_event_id text DEFAULT ''::text NOT NULL,
    inbox_coalesce_key text DEFAULT ''::text NOT NULL,
    resolved_at timestamp with time zone,
    summary_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_runtime_alert_incidents_active_resource_kind
    ON runtime_alert_incidents USING btree (project_id, resource_id, incident_kind)
    WHERE (resolved_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_runtime_alert_incidents_project_status
    ON runtime_alert_incidents USING btree (project_id, status);
CREATE INDEX IF NOT EXISTS idx_runtime_alert_incidents_deployment_target_id
    ON runtime_alert_incidents USING btree (deployment_target_id);
