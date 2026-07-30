DROP TABLE IF EXISTS ai.ui_actions;

ALTER TABLE ai.runs
    DROP COLUMN IF EXISTS client_instance_id;
