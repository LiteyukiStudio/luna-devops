ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS web_console_enabled BOOLEAN NOT NULL DEFAULT TRUE;

ALTER TABLE deployment_targets
    ADD COLUMN IF NOT EXISTS web_console_enabled BOOLEAN;
