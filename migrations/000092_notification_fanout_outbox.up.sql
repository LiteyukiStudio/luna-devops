ALTER TABLE platform_events
    ADD COLUMN notification_fanout_status text DEFAULT ''::text NOT NULL,
    ADD COLUMN fanout_traceparent text DEFAULT ''::text NOT NULL,
    ADD COLUMN fanout_tracestate text DEFAULT ''::text NOT NULL,
    ADD CONSTRAINT chk_platform_events_notification_fanout_status
        CHECK (notification_fanout_status = ANY (ARRAY[''::text, 'pending'::text, 'completed'::text]));

CREATE INDEX idx_platform_events_notification_fanout_status
    ON platform_events USING btree (notification_fanout_status);

ALTER TABLE notification_deliveries
    ADD COLUMN traceparent text DEFAULT ''::text NOT NULL,
    ADD COLUMN tracestate text DEFAULT ''::text NOT NULL;
