ALTER TABLE registry_credentials
    DROP COLUMN IF EXISTS tag_template,
    DROP COLUMN IF EXISTS repository_template;
