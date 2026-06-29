ALTER TABLE registry_credentials
    ADD COLUMN IF NOT EXISTS repository_template text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS tag_template text NOT NULL DEFAULT '';
