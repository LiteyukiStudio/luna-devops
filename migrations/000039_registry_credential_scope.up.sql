ALTER TABLE registry_credentials RENAME COLUMN scope TO usage;

ALTER TABLE registry_credentials
ADD COLUMN scope text NOT NULL DEFAULT 'user',
ADD COLUMN owner_ref text NOT NULL DEFAULT '';

UPDATE registry_credentials
SET scope = 'user', owner_ref = created_by
WHERE coalesce(access_scope, 'personal') = 'personal';

UPDATE registry_credentials AS credential
SET scope = registry.scope,
    owner_ref = CASE WHEN registry.scope = 'user' THEN registry.owner_ref ELSE '' END
FROM artifact_registries AS registry
WHERE credential.registry_id = registry.id
  AND coalesce(credential.access_scope, 'personal') = 'registry';

INSERT INTO scoped_resource_project_bindings (id, resource_type, resource_id, project_id, is_default, created_at, updated_at)
SELECT 'srpb_' || md5(credential.id || ':' || binding.project_id),
       'registry_credential',
       credential.id,
       binding.project_id,
       false,
       now(),
       now()
FROM registry_credentials AS credential
JOIN artifact_registries AS registry ON registry.id = credential.registry_id
JOIN scoped_resource_project_bindings AS binding
  ON binding.resource_type = 'artifact_registry'
 AND binding.resource_id = registry.id
WHERE coalesce(credential.access_scope, 'personal') = 'registry'
  AND registry.scope = 'project'
ON CONFLICT (resource_type, resource_id, project_id) DO NOTHING;

ALTER TABLE registry_credentials DROP COLUMN access_scope;

CREATE INDEX IF NOT EXISTS idx_registry_credentials_scope_owner ON registry_credentials(scope, owner_ref);
