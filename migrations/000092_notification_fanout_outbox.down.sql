ALTER TABLE notification_deliveries
    DROP COLUMN tracestate,
    DROP COLUMN traceparent;

DROP INDEX IF EXISTS idx_platform_events_notification_fanout_status;

ALTER TABLE platform_events
    DROP CONSTRAINT IF EXISTS chk_platform_events_notification_fanout_status,
    DROP COLUMN fanout_tracestate,
    DROP COLUMN fanout_traceparent,
    DROP COLUMN notification_fanout_status;
