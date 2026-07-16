ALTER TABLE registry_credentials
ADD COLUMN access_scope text NOT NULL DEFAULT 'personal';

UPDATE registry_credentials
SET access_scope = CASE WHEN scope = 'user' THEN 'personal' ELSE 'registry' END;

DELETE FROM scoped_resource_project_bindings
WHERE resource_type = 'registry_credential';

DROP INDEX IF EXISTS idx_registry_credentials_scope_owner;

ALTER TABLE registry_credentials
DROP COLUMN owner_ref,
DROP COLUMN scope;

ALTER TABLE registry_credentials RENAME COLUMN usage TO scope;
