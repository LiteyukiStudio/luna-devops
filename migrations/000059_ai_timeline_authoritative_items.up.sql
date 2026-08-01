ALTER TABLE ai.items
    ADD COLUMN revision bigint NOT NULL DEFAULT 1;

ALTER TABLE ai.runs
    ADD COLUMN next_item_position bigint NOT NULL DEFAULT 0,
    ADD COLUMN next_event_sequence bigint NOT NULL DEFAULT 1;

UPDATE ai.runs AS run
SET next_item_position = COALESCE((
        SELECT MAX(item.timeline_index) + 1
        FROM ai.items AS item
        WHERE item.run_id = run.id
    ), 0),
    next_event_sequence = COALESCE((
        SELECT MAX(event.event_sequence) + 1
        FROM ai.run_events AS event
        WHERE event.run_id = run.id
    ), 1);
