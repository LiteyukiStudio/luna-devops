UPDATE ai.tool_calls
SET approval_decision = 'approve'
WHERE approval_decision = 'approve_always';

UPDATE users
SET brand_color_preset = CASE
    WHEN lower(btrim(brand_color_preset)) IN (
        'aurora', 'harbor', 'sunset', 'botanical', 'meadow', 'citrus',
        'gold', 'orange', 'red', 'pink', 'violet', 'blue', 'cyan', 'teal', 'green', 'lime'
    ) THEN lower(btrim(brand_color_preset))
    ELSE ''
END;

UPDATE app_configs
SET value = CASE
        WHEN lower(btrim(value)) IN (
            'aurora', 'harbor', 'sunset', 'botanical', 'meadow', 'citrus',
            'gold', 'orange', 'red', 'pink', 'violet', 'blue', 'cyan', 'teal', 'green', 'lime'
        ) THEN lower(btrim(value))
        ELSE 'blue'
    END,
    updated_at = now()
WHERE key = 'site.brandColorPreset';

ALTER TABLE ai.tool_calls
    DROP CONSTRAINT IF EXISTS tool_calls_approval_decision_check;

ALTER TABLE ai.tool_calls
    ADD CONSTRAINT tool_calls_approval_decision_check
    CHECK (approval_decision = 'approve');

ALTER TABLE ai.runs
    DROP COLUMN IF EXISTS client_instance_id;

ALTER TABLE ai.runs
    ADD COLUMN IF NOT EXISTS execution_snapshot_ciphertext text;

DROP TABLE IF EXISTS ai.ui_actions;
DROP TABLE IF EXISTS ai.tool_approval_exemptions;
