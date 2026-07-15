DELETE FROM scoped_resource_project_bindings
WHERE resource_type = 'git_account'
  AND resource_id IN (
    SELECT id
    FROM git_accounts
    WHERE coalesce(access_scope, 'personal') = 'personal'
  );

UPDATE git_accounts
SET scope = 'user', owner_ref = user_id
WHERE coalesce(access_scope, 'personal') = 'personal';

ALTER TABLE git_accounts DROP COLUMN IF EXISTS access_scope;
