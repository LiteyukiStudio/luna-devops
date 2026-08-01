ALTER TABLE ai.runs
    DROP COLUMN IF EXISTS next_event_sequence,
    DROP COLUMN IF EXISTS next_item_position;

ALTER TABLE ai.items
    DROP COLUMN IF EXISTS revision;
