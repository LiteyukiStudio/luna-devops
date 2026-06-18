CREATE TABLE IF NOT EXISTS users (
  id text PRIMARY KEY,
  email text NOT NULL UNIQUE,
  name text NOT NULL,
  avatar_url text NOT NULL DEFAULT '',
  auth_type text NOT NULL DEFAULT 'local',
  role text NOT NULL DEFAULT 'user',
  language text NOT NULL DEFAULT 'zh-CN',
  password text NOT NULL DEFAULT '',
  disabled boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at);

CREATE TABLE IF NOT EXISTS user_sessions (
  id text PRIMARY KEY,
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  impersonator_id text NOT NULL DEFAULT '',
  token_hash text NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_impersonator_id ON user_sessions(impersonator_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_expires_at ON user_sessions(expires_at);

CREATE TABLE IF NOT EXISTS user_remember_tokens (
  id text PRIMARY KEY,
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash text NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_user_remember_tokens_user_id ON user_remember_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_user_remember_tokens_expires_at ON user_remember_tokens(expires_at);

CREATE TABLE IF NOT EXISTS auth_providers (
  id text PRIMARY KEY,
  type text NOT NULL,
  name text NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  issuer_url text NOT NULL,
  client_id text NOT NULL,
  client_secret_ref text NOT NULL DEFAULT '',
  scopes text NOT NULL DEFAULT 'openid profile email',
  group_claim text NOT NULL DEFAULT 'groups',
  email_claim text NOT NULL DEFAULT 'email',
  username_claim text NOT NULL DEFAULT 'preferred_username',
  is_default boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_auth_providers_deleted_at ON auth_providers(deleted_at);

CREATE TABLE IF NOT EXISTS external_identities (
  id text PRIMARY KEY,
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider_id text NOT NULL REFERENCES auth_providers(id) ON DELETE RESTRICT,
  subject text NOT NULL,
  email text NOT NULL DEFAULT '',
  email_verified boolean NOT NULL DEFAULT false,
  username text NOT NULL DEFAULT '',
  last_login_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_external_identities_provider_subject ON external_identities(provider_id, subject);
CREATE UNIQUE INDEX IF NOT EXISTS idx_external_identities_user_provider ON external_identities(user_id, provider_id);

CREATE TABLE IF NOT EXISTS auth_admission_policies (
  id text PRIMARY KEY,
  allow_local_login boolean NOT NULL DEFAULT true,
  allow_oidc_login boolean NOT NULL DEFAULT true,
  require_verified_oidc_email boolean NOT NULL DEFAULT true,
  allowed_email_domains text NOT NULL DEFAULT '',
  allowed_oidc_groups text NOT NULL DEFAULT '',
  invited_emails text NOT NULL DEFAULT '',
  default_role text NOT NULL DEFAULT 'user',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS projects (
  id text PRIMARY KEY,
  slug text NOT NULL,
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  namespace_strategy text NOT NULL DEFAULT 'project',
  max_concurrent_builds integer NOT NULL DEFAULT 2,
  delete_status text NOT NULL DEFAULT 'active',
  delete_message text NOT NULL DEFAULT '',
  delete_started_at timestamptz,
  delete_finished_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_slug_active ON projects(slug) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_projects_deleted_at ON projects(deleted_at);

CREATE TABLE IF NOT EXISTS project_members (
  id text PRIMARY KEY,
  project_id text NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role text NOT NULL,
  dashboard_order integer NOT NULL DEFAULT 0,
  last_used_at timestamptz,
  use_count integer NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_project_members_project_id ON project_members(project_id);
CREATE INDEX IF NOT EXISTS idx_project_members_user_id ON project_members(user_id);
CREATE INDEX IF NOT EXISTS idx_project_members_user_dashboard_order ON project_members(user_id, dashboard_order);
CREATE INDEX IF NOT EXISTS idx_project_members_last_used_at ON project_members(last_used_at);
CREATE INDEX IF NOT EXISTS idx_project_members_use_count ON project_members(use_count);

CREATE TABLE IF NOT EXISTS project_pins (
  id text PRIMARY KEY,
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  project_id text NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  pinned_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_project_pins_user_project ON project_pins(user_id, project_id);
CREATE INDEX IF NOT EXISTS idx_project_pins_user_id ON project_pins(user_id);
CREATE INDEX IF NOT EXISTS idx_project_pins_project_id ON project_pins(project_id);
CREATE INDEX IF NOT EXISTS idx_project_pins_user_pinned_at ON project_pins(user_id, pinned_at DESC);

CREATE TABLE IF NOT EXISTS project_hook_configs (
  id text PRIMARY KEY,
  project_id text NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name text NOT NULL,
  script text NOT NULL,
  shell text NOT NULL DEFAULT 'sh',
  timeout_seconds integer NOT NULL DEFAULT 300,
  failure_policy text NOT NULL DEFAULT 'fail',
  created_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_project_hook_configs_project_id ON project_hook_configs(project_id);
CREATE INDEX IF NOT EXISTS idx_project_hook_configs_created_by ON project_hook_configs(created_by);
CREATE INDEX IF NOT EXISTS idx_project_hook_configs_deleted_at ON project_hook_configs(deleted_at);

CREATE TABLE IF NOT EXISTS hook_runs (
  id text PRIMARY KEY,
  project_id text NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  hook_config_id text NOT NULL DEFAULT '',
  build_run_id text NOT NULL DEFAULT '',
  build_job_id text NOT NULL DEFAULT '',
  release_id text NOT NULL DEFAULT '',
  application_id text NOT NULL DEFAULT '',
  environment_id text NOT NULL DEFAULT '',
  deployment_target_id text NOT NULL DEFAULT '',
  name text NOT NULL,
  phase text NOT NULL,
  status text NOT NULL DEFAULT 'queued',
  script_snapshot text NOT NULL,
  shell text NOT NULL DEFAULT 'sh',
  image_ref text NOT NULL DEFAULT '',
  timeout_seconds integer NOT NULL DEFAULT 300,
  failure_policy text NOT NULL DEFAULT 'fail',
  exit_code integer NOT NULL DEFAULT 0,
  message text NOT NULL DEFAULT '',
  started_at timestamptz,
  finished_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_hook_runs_project_id ON hook_runs(project_id);
CREATE INDEX IF NOT EXISTS idx_hook_runs_hook_config_id ON hook_runs(hook_config_id);
CREATE INDEX IF NOT EXISTS idx_hook_runs_build_run_id ON hook_runs(build_run_id);
CREATE INDEX IF NOT EXISTS idx_hook_runs_build_job_id ON hook_runs(build_job_id);
CREATE INDEX IF NOT EXISTS idx_hook_runs_release_id ON hook_runs(release_id);
CREATE INDEX IF NOT EXISTS idx_hook_runs_application_id ON hook_runs(application_id);
CREATE INDEX IF NOT EXISTS idx_hook_runs_environment_id ON hook_runs(environment_id);
CREATE INDEX IF NOT EXISTS idx_hook_runs_deployment_target_id ON hook_runs(deployment_target_id);
CREATE INDEX IF NOT EXISTS idx_hook_runs_phase ON hook_runs(phase);
CREATE INDEX IF NOT EXISTS idx_hook_runs_status ON hook_runs(status);

CREATE TABLE IF NOT EXISTS hook_run_logs (
  id text PRIMARY KEY,
  hook_run_id text NOT NULL,
  project_id text NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  content text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_hook_run_logs_hook_run_id ON hook_run_logs(hook_run_id);
CREATE INDEX IF NOT EXISTS idx_hook_run_logs_project_id ON hook_run_logs(project_id);

CREATE TABLE IF NOT EXISTS access_tokens (
  id text PRIMARY KEY,
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name text NOT NULL,
  scope text NOT NULL,
  token_hash text NOT NULL,
  expires_at timestamptz,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_access_tokens_user_id ON access_tokens(user_id);

CREATE TABLE IF NOT EXISTS audit_logs (
  id text PRIMARY KEY,
  user_id text NOT NULL DEFAULT '',
  action text NOT NULL,
  resource text NOT NULL,
  success boolean NOT NULL DEFAULT true,
  message text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON audit_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource ON audit_logs(resource);

CREATE TABLE IF NOT EXISTS secret_values (
  id text PRIMARY KEY,
  cipher_ref text NOT NULL,
  created_by text NOT NULL DEFAULT '',
  resource text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_secret_values_created_by ON secret_values(created_by);
CREATE INDEX IF NOT EXISTS idx_secret_values_resource ON secret_values(resource);

CREATE TABLE IF NOT EXISTS scoped_resource_project_bindings (
  id text PRIMARY KEY,
  resource_type text NOT NULL,
  resource_id text NOT NULL,
  project_id text NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  is_default boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (resource_type, resource_id, project_id)
);
CREATE INDEX IF NOT EXISTS idx_scoped_resource_project_bindings_type_id ON scoped_resource_project_bindings(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_scoped_resource_project_bindings_project_id ON scoped_resource_project_bindings(project_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_scoped_resource_project_bindings_default_registry ON scoped_resource_project_bindings(project_id)
  WHERE resource_type = 'artifact_registry' AND is_default;

CREATE TABLE IF NOT EXISTS applications (
  id text PRIMARY KEY,
  project_id text NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  slug text NOT NULL,
  name text NOT NULL,
  icon text NOT NULL DEFAULT 'box',
  delete_status text NOT NULL DEFAULT 'active',
  delete_message text NOT NULL DEFAULT '',
  delete_started_at timestamptz,
  delete_finished_at timestamptz,
  data_retention_mode text NOT NULL DEFAULT 'retain',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_applications_project_slug_active ON applications(project_id, slug) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_applications_project_id ON applications(project_id);
CREATE INDEX IF NOT EXISTS idx_applications_slug ON applications(slug);
CREATE INDEX IF NOT EXISTS idx_applications_delete_status ON applications(delete_status);
CREATE INDEX IF NOT EXISTS idx_applications_deleted_at ON applications(deleted_at);

CREATE TABLE IF NOT EXISTS app_configs (
  key text PRIMARY KEY,
  value text NOT NULL DEFAULT '',
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS git_providers (
  id text PRIMARY KEY,
  type text NOT NULL,
  name text NOT NULL,
  base_url text NOT NULL DEFAULT '',
  scope text NOT NULL DEFAULT 'user',
  owner_ref text NOT NULL DEFAULT '',
  auth_type text NOT NULL DEFAULT 'oauth',
  client_id text NOT NULL DEFAULT '',
  client_secret_ref text NOT NULL DEFAULT '',
  enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_git_providers_owner_ref ON git_providers(owner_ref);
CREATE INDEX IF NOT EXISTS idx_git_providers_scope_owner_ref ON git_providers(scope, owner_ref);
CREATE INDEX IF NOT EXISTS idx_git_providers_deleted_at ON git_providers(deleted_at);

CREATE TABLE IF NOT EXISTS git_accounts (
  id text PRIMARY KEY,
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider_id text NOT NULL REFERENCES git_providers(id) ON DELETE RESTRICT,
  scope text NOT NULL DEFAULT 'user',
  owner_ref text NOT NULL DEFAULT '',
  external_user_id text NOT NULL DEFAULT '',
  username text NOT NULL,
  avatar_url text NOT NULL DEFAULT '',
  access_token_ref text NOT NULL DEFAULT '',
  refresh_token_ref text NOT NULL DEFAULT '',
  scopes text NOT NULL DEFAULT '',
  access_scope text NOT NULL DEFAULT 'personal',
  expires_at timestamptz,
  status text NOT NULL DEFAULT 'connected',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_git_accounts_user_id ON git_accounts(user_id);
CREATE INDEX IF NOT EXISTS idx_git_accounts_provider_id ON git_accounts(provider_id);
CREATE INDEX IF NOT EXISTS idx_git_accounts_owner_ref ON git_accounts(owner_ref);
CREATE INDEX IF NOT EXISTS idx_git_accounts_scope_owner_ref ON git_accounts(scope, owner_ref);
CREATE INDEX IF NOT EXISTS idx_git_accounts_user_provider ON git_accounts(user_id, provider_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_git_accounts_deleted_at ON git_accounts(deleted_at);

CREATE TABLE IF NOT EXISTS repository_bindings (
  id text PRIMARY KEY,
  project_id text NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  application_id text NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
  git_provider_id text NOT NULL REFERENCES git_providers(id) ON DELETE RESTRICT,
  git_account_id text NOT NULL REFERENCES git_accounts(id) ON DELETE RESTRICT,
  owner text NOT NULL,
  repo text NOT NULL,
  clone_url text NOT NULL DEFAULT '',
  default_branch text NOT NULL DEFAULT 'main',
  webhook_status text NOT NULL DEFAULT 'pending',
  webhook_id text NOT NULL DEFAULT '',
  webhook_secret text NOT NULL DEFAULT '',
  credential_ref text NOT NULL DEFAULT '',
  last_event text NOT NULL DEFAULT '',
  last_commit_sha text NOT NULL DEFAULT '',
  last_webhook_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_repository_bindings_application_repo_active ON repository_bindings(application_id, git_account_id, owner, repo) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_repository_bindings_project ON repository_bindings(project_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_repository_bindings_project_id ON repository_bindings(project_id);
CREATE INDEX IF NOT EXISTS idx_repository_bindings_application_id ON repository_bindings(application_id);
CREATE INDEX IF NOT EXISTS idx_repository_bindings_git_provider_id ON repository_bindings(git_provider_id);
CREATE INDEX IF NOT EXISTS idx_repository_bindings_git_account_id ON repository_bindings(git_account_id);
CREATE INDEX IF NOT EXISTS idx_repository_bindings_deleted_at ON repository_bindings(deleted_at);

CREATE TABLE IF NOT EXISTS artifact_registries (
  id text PRIMARY KEY,
  name text NOT NULL,
  provider text NOT NULL,
  endpoint text NOT NULL,
  namespace text NOT NULL DEFAULT '',
  scope text NOT NULL DEFAULT 'global',
  owner_ref text NOT NULL DEFAULT '',
  credential_ref text NOT NULL DEFAULT '',
  is_default boolean NOT NULL DEFAULT false,
  capabilities text NOT NULL DEFAULT '',
  created_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_artifact_registries_scope ON artifact_registries(scope);
CREATE INDEX IF NOT EXISTS idx_artifact_registries_owner_ref ON artifact_registries(owner_ref);
CREATE INDEX IF NOT EXISTS idx_artifact_registries_created_by ON artifact_registries(created_by);
CREATE INDEX IF NOT EXISTS idx_artifact_registries_deleted_at ON artifact_registries(deleted_at);
CREATE INDEX IF NOT EXISTS idx_artifact_registries_scope_owner ON artifact_registries(scope, owner_ref) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_artifact_registries_default_global ON artifact_registries(scope) WHERE deleted_at IS NULL AND scope = 'global' AND is_default;
CREATE UNIQUE INDEX IF NOT EXISTS idx_artifact_registries_default_project ON artifact_registries(scope, owner_ref) WHERE deleted_at IS NULL AND scope = 'project' AND is_default;
CREATE UNIQUE INDEX IF NOT EXISTS idx_artifact_registries_default_user ON artifact_registries(scope, owner_ref) WHERE deleted_at IS NULL AND scope = 'user' AND is_default;

CREATE TABLE IF NOT EXISTS registry_credentials (
  id text PRIMARY KEY,
  registry_id text NOT NULL REFERENCES artifact_registries(id) ON DELETE CASCADE,
  name text NOT NULL,
  username text NOT NULL DEFAULT '',
  password_ref text NOT NULL DEFAULT '',
  token_ref text NOT NULL DEFAULT '',
  scope text NOT NULL DEFAULT 'push-pull',
  access_scope text NOT NULL DEFAULT 'personal',
  created_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_registry_credentials_registry_id ON registry_credentials(registry_id);
CREATE INDEX IF NOT EXISTS idx_registry_credentials_created_by ON registry_credentials(created_by);
CREATE INDEX IF NOT EXISTS idx_registry_credentials_deleted_at ON registry_credentials(deleted_at);
CREATE INDEX IF NOT EXISTS idx_registry_credentials_registry ON registry_credentials(registry_id) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS container_images (
  id text PRIMARY KEY,
  project_id text NOT NULL DEFAULT '',
  application_id text NOT NULL DEFAULT '',
  registry_id text NOT NULL REFERENCES artifact_registries(id) ON DELETE RESTRICT,
  repository text NOT NULL,
  tag text NOT NULL,
  digest text NOT NULL DEFAULT '',
  image_ref text NOT NULL,
  source_commit text NOT NULL DEFAULT '',
  build_run_id text NOT NULL DEFAULT '',
  source_type text NOT NULL DEFAULT 'manual-image',
  scan_status text NOT NULL DEFAULT 'unknown',
  created_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_container_images_project_id ON container_images(project_id);
CREATE INDEX IF NOT EXISTS idx_container_images_application_id ON container_images(application_id);
CREATE INDEX IF NOT EXISTS idx_container_images_registry_id ON container_images(registry_id);
CREATE INDEX IF NOT EXISTS idx_container_images_build_run_id ON container_images(build_run_id);
CREATE INDEX IF NOT EXISTS idx_container_images_created_by ON container_images(created_by);
CREATE INDEX IF NOT EXISTS idx_container_images_deleted_at ON container_images(deleted_at);
CREATE INDEX IF NOT EXISTS idx_container_images_project ON container_images(project_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_container_images_application ON container_images(application_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_container_images_registry_repo ON container_images(registry_id, repository) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS deployment_target_hook_bindings (
  id text PRIMARY KEY,
  project_id text NOT NULL,
  application_id text NOT NULL,
  target_id text NOT NULL,
  hook_config_id text NOT NULL,
  phase text NOT NULL,
  run_order integer NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_deployment_target_hook_bindings_project_id ON deployment_target_hook_bindings(project_id);
CREATE INDEX IF NOT EXISTS idx_deployment_target_hook_bindings_application_id ON deployment_target_hook_bindings(application_id);
CREATE INDEX IF NOT EXISTS idx_deployment_target_hook_bindings_target_id ON deployment_target_hook_bindings(target_id);
CREATE INDEX IF NOT EXISTS idx_deployment_target_hook_bindings_hook_config_id ON deployment_target_hook_bindings(hook_config_id);
CREATE INDEX IF NOT EXISTS idx_deployment_target_hook_bindings_phase ON deployment_target_hook_bindings(phase);
CREATE UNIQUE INDEX IF NOT EXISTS idx_deployment_target_hook_bindings_target_hook ON deployment_target_hook_bindings(target_id, hook_config_id, phase);

CREATE TABLE IF NOT EXISTS build_runs (
  id text PRIMARY KEY,
  project_id text NOT NULL,
  application_id text NOT NULL DEFAULT '',
  deployment_target_id text NOT NULL DEFAULT '',
  build_labels text NOT NULL DEFAULT '',
  build_variable_set_ids text NOT NULL DEFAULT '',
  status text NOT NULL DEFAULT 'queued',
  trigger_type text NOT NULL DEFAULT 'manual',
  source_branch text NOT NULL DEFAULT '',
  source_tag text NOT NULL DEFAULT '',
  source_commit text NOT NULL DEFAULT '',
  dockerfile_path text NOT NULL DEFAULT 'Dockerfile',
  build_context text NOT NULL DEFAULT '.',
  build_directory text NOT NULL DEFAULT '',
  target_registry_id text NOT NULL DEFAULT '',
  target_repository text NOT NULL DEFAULT '',
  target_tag text NOT NULL DEFAULT '',
  image_ref text NOT NULL DEFAULT '',
  image_digest text NOT NULL DEFAULT '',
  cache_config text NOT NULL DEFAULT '',
  cpu_core_seconds bigint NOT NULL DEFAULT 0,
  memory_mb_seconds bigint NOT NULL DEFAULT 0,
  credit_cost bigint NOT NULL DEFAULT 0,
  started_at timestamptz,
  finished_at timestamptz,
  created_by text NOT NULL DEFAULT '',
  triggered_by_name text NOT NULL DEFAULT '',
  triggered_by_email text NOT NULL DEFAULT '',
  source_author_name text NOT NULL DEFAULT '',
  source_author_email text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_build_runs_project_id ON build_runs(project_id);
CREATE INDEX IF NOT EXISTS idx_build_runs_application_id ON build_runs(application_id);
CREATE INDEX IF NOT EXISTS idx_build_runs_deployment_target_id ON build_runs(deployment_target_id);
CREATE INDEX IF NOT EXISTS idx_build_runs_status ON build_runs(status);
CREATE INDEX IF NOT EXISTS idx_build_runs_target_registry_id ON build_runs(target_registry_id);
CREATE INDEX IF NOT EXISTS idx_build_runs_created_by ON build_runs(created_by);
CREATE INDEX IF NOT EXISTS idx_build_runs_deleted_at ON build_runs(deleted_at);

CREATE TABLE IF NOT EXISTS build_variable_sets (
  id text PRIMARY KEY,
  name text NOT NULL,
  scope text NOT NULL DEFAULT 'global',
  owner_ref text NOT NULL DEFAULT '',
  variables text NOT NULL DEFAULT '',
  secret_refs text NOT NULL DEFAULT '',
  enabled boolean NOT NULL DEFAULT true,
  created_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_build_variable_sets_scope ON build_variable_sets(scope);
CREATE INDEX IF NOT EXISTS idx_build_variable_sets_owner_ref ON build_variable_sets(owner_ref);
CREATE INDEX IF NOT EXISTS idx_build_variable_sets_created_by ON build_variable_sets(created_by);
CREATE INDEX IF NOT EXISTS idx_build_variable_sets_deleted_at ON build_variable_sets(deleted_at);

CREATE TABLE IF NOT EXISTS build_jobs (
  id text PRIMARY KEY,
  build_run_id text NOT NULL,
  project_id text NOT NULL,
  type text NOT NULL DEFAULT 'build',
  status text NOT NULL DEFAULT 'queued',
  builder_id text NOT NULL DEFAULT '',
  lease_token text NOT NULL DEFAULT '',
  lease_until timestamptz,
  last_heartbeat_at timestamptz,
  executor_id text NOT NULL DEFAULT '',
  executor_name text NOT NULL DEFAULT '',
  message text NOT NULL DEFAULT '',
  log_ref text NOT NULL DEFAULT '',
  attempts integer NOT NULL DEFAULT 0,
  started_at timestamptz,
  finished_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_build_jobs_build_run_id ON build_jobs(build_run_id);
CREATE INDEX IF NOT EXISTS idx_build_jobs_project_id ON build_jobs(project_id);
CREATE INDEX IF NOT EXISTS idx_build_jobs_status ON build_jobs(status);
CREATE INDEX IF NOT EXISTS idx_build_jobs_builder_id ON build_jobs(builder_id);
CREATE INDEX IF NOT EXISTS idx_build_jobs_lease_token ON build_jobs(lease_token);
CREATE INDEX IF NOT EXISTS idx_build_jobs_lease_until ON build_jobs(lease_until);
CREATE INDEX IF NOT EXISTS idx_build_jobs_last_heartbeat_at ON build_jobs(last_heartbeat_at);
CREATE INDEX IF NOT EXISTS idx_build_jobs_deleted_at ON build_jobs(deleted_at);

CREATE TABLE IF NOT EXISTS build_logs (
  id text PRIMARY KEY,
  build_run_id text NOT NULL,
  build_job_id text NOT NULL,
  project_id text NOT NULL,
  content text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_build_logs_build_run_id ON build_logs(build_run_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_build_logs_build_job_id ON build_logs(build_job_id);
CREATE INDEX IF NOT EXISTS idx_build_logs_project_id ON build_logs(project_id);

CREATE TABLE IF NOT EXISTS runtime_clusters (
  id text PRIMARY KEY,
  name text NOT NULL,
  type text NOT NULL DEFAULT 'kubernetes',
  endpoint text NOT NULL DEFAULT '',
  scope text NOT NULL DEFAULT 'global',
  owner_ref text NOT NULL DEFAULT '',
  kubeconfig_ref text NOT NULL DEFAULT '',
  is_default boolean NOT NULL DEFAULT false,
  max_concurrent_builds integer NOT NULL DEFAULT 4,
  status text NOT NULL DEFAULT 'unknown',
  last_checked_at timestamptz,
  created_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_runtime_clusters_scope ON runtime_clusters(scope);
CREATE INDEX IF NOT EXISTS idx_runtime_clusters_owner_ref ON runtime_clusters(owner_ref);
CREATE INDEX IF NOT EXISTS idx_runtime_clusters_created_by ON runtime_clusters(created_by);
CREATE INDEX IF NOT EXISTS idx_runtime_clusters_deleted_at ON runtime_clusters(deleted_at);

CREATE TABLE IF NOT EXISTS environments (
  id text PRIMARY KEY,
  project_id text NOT NULL,
  name text NOT NULL,
  slug text NOT NULL,
  stage text NOT NULL DEFAULT 'dev',
  cluster_id text NOT NULL DEFAULT '',
  namespace text NOT NULL DEFAULT '',
  replicas integer NOT NULL DEFAULT 1,
  cpu_request text NOT NULL DEFAULT '',
  memory_request text NOT NULL DEFAULT '',
  env_vars text NOT NULL DEFAULT '',
  config_refs text NOT NULL DEFAULT '',
  secret_refs text NOT NULL DEFAULT '',
  created_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_environments_project_id ON environments(project_id);
CREATE INDEX IF NOT EXISTS idx_environments_slug ON environments(slug);
CREATE INDEX IF NOT EXISTS idx_environments_cluster_id ON environments(cluster_id);
CREATE INDEX IF NOT EXISTS idx_environments_created_by ON environments(created_by);
CREATE INDEX IF NOT EXISTS idx_environments_deleted_at ON environments(deleted_at);

CREATE TABLE IF NOT EXISTS releases (
  id text PRIMARY KEY,
  project_id text NOT NULL,
  application_id text NOT NULL,
  environment_id text NOT NULL,
  deployment_target_id text NOT NULL DEFAULT '',
  build_run_id text NOT NULL DEFAULT '',
  image_ref text NOT NULL,
  type text NOT NULL DEFAULT 'deploy',
  status text NOT NULL DEFAULT 'pending',
  revision integer NOT NULL DEFAULT 1,
  rollback_from_id text NOT NULL DEFAULT '',
  message text NOT NULL DEFAULT '',
  started_at timestamptz,
  finished_at timestamptz,
  created_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_releases_project_id ON releases(project_id);
CREATE INDEX IF NOT EXISTS idx_releases_application_id ON releases(application_id);
CREATE INDEX IF NOT EXISTS idx_releases_environment_id ON releases(environment_id);
CREATE INDEX IF NOT EXISTS idx_releases_deployment_target_id ON releases(deployment_target_id);
CREATE INDEX IF NOT EXISTS idx_releases_build_run_id ON releases(build_run_id);
CREATE INDEX IF NOT EXISTS idx_releases_status ON releases(status);
CREATE INDEX IF NOT EXISTS idx_releases_rollback_from_id ON releases(rollback_from_id);
CREATE INDEX IF NOT EXISTS idx_releases_created_by ON releases(created_by);
CREATE INDEX IF NOT EXISTS idx_releases_deleted_at ON releases(deleted_at);

CREATE TABLE IF NOT EXISTS release_logs (
  id text PRIMARY KEY,
  release_id text NOT NULL,
  project_id text NOT NULL,
  content text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_release_logs_release_id ON release_logs(release_id);
CREATE INDEX IF NOT EXISTS idx_release_logs_project_id ON release_logs(project_id);

CREATE TABLE IF NOT EXISTS deployment_targets (
  id text PRIMARY KEY,
  project_id text NOT NULL,
  application_id text NOT NULL,
  environment_id text NOT NULL,
  name text NOT NULL,
  service_port integer NOT NULL DEFAULT 8080,
  delete_status text NOT NULL DEFAULT 'active',
  delete_message text NOT NULL DEFAULT '',
  delete_started_at timestamptz,
  delete_finished_at timestamptz,
  source_type text NOT NULL DEFAULT 'repository',
  repository_binding_id text NOT NULL DEFAULT '',
  dockerfile_path text NOT NULL DEFAULT 'Dockerfile',
  build_context text NOT NULL DEFAULT '.',
  build_directory text NOT NULL DEFAULT '',
  target_registry_id text NOT NULL DEFAULT '',
  target_repository text NOT NULL DEFAULT '',
  target_tag text NOT NULL DEFAULT '',
  image_ref text NOT NULL DEFAULT '',
  build_labels text NOT NULL DEFAULT '',
  build_variable_set_ids text NOT NULL DEFAULT '',
  build_hooks_enabled boolean NOT NULL DEFAULT true,
  auto_deploy boolean NOT NULL DEFAULT false,
  branch_pattern text NOT NULL DEFAULT '',
  tag_pattern text NOT NULL DEFAULT '',
  concurrency_policy text NOT NULL DEFAULT 'queue',
  env_vars text NOT NULL DEFAULT '',
  config_refs text NOT NULL DEFAULT '',
  secret_refs text NOT NULL DEFAULT '',
  data_retention_enabled boolean NOT NULL DEFAULT false,
  data_capacity text NOT NULL DEFAULT '',
  data_mount_path text NOT NULL DEFAULT '/data',
  data_volumes text NOT NULL DEFAULT '',
  require_approval boolean NOT NULL DEFAULT false,
  enabled boolean NOT NULL DEFAULT true,
  created_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_deployment_targets_project_id ON deployment_targets(project_id);
CREATE INDEX IF NOT EXISTS idx_deployment_targets_application_id ON deployment_targets(application_id);
CREATE INDEX IF NOT EXISTS idx_deployment_targets_environment_id ON deployment_targets(environment_id);
CREATE INDEX IF NOT EXISTS idx_deployment_targets_repository_binding_id ON deployment_targets(repository_binding_id);
CREATE INDEX IF NOT EXISTS idx_deployment_targets_target_registry_id ON deployment_targets(target_registry_id);
CREATE INDEX IF NOT EXISTS idx_deployment_targets_created_by ON deployment_targets(created_by);
CREATE INDEX IF NOT EXISTS idx_deployment_targets_deleted_at ON deployment_targets(deleted_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_deployment_targets_app_env_name_active ON deployment_targets(application_id, environment_id, name) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS gateway_routes (
  id text PRIMARY KEY,
  project_id text NOT NULL,
  application_id text NOT NULL,
  environment_id text NOT NULL DEFAULT '',
  deployment_target_id text NOT NULL DEFAULT '',
  host text NOT NULL,
  path text NOT NULL DEFAULT '/',
  service_port integer NOT NULL DEFAULT 80,
  tls_mode text NOT NULL DEFAULT 'http-only',
  certificate_status text NOT NULL DEFAULT 'disabled',
  cname_name text NOT NULL DEFAULT '',
  cname_target text NOT NULL DEFAULT '',
  dns_status text NOT NULL DEFAULT 'pending',
  status text NOT NULL DEFAULT 'pending',
  delete_status text NOT NULL DEFAULT 'active',
  delete_message text NOT NULL DEFAULT '',
  delete_started_at timestamptz,
  delete_finished_at timestamptz,
  is_default boolean NOT NULL DEFAULT false,
  created_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_gateway_routes_project_id ON gateway_routes(project_id);
CREATE INDEX IF NOT EXISTS idx_gateway_routes_application_id ON gateway_routes(application_id);
CREATE INDEX IF NOT EXISTS idx_gateway_routes_environment_id ON gateway_routes(environment_id);
CREATE INDEX IF NOT EXISTS idx_gateway_routes_deployment_target_id ON gateway_routes(deployment_target_id);
CREATE INDEX IF NOT EXISTS idx_gateway_routes_host ON gateway_routes(host);
CREATE INDEX IF NOT EXISTS idx_gateway_routes_created_by ON gateway_routes(created_by);
CREATE INDEX IF NOT EXISTS idx_gateway_routes_deleted_at ON gateway_routes(deleted_at);

CREATE TABLE IF NOT EXISTS worker_task_events (
  id text PRIMARY KEY,
  task_id text NOT NULL,
  task_type text NOT NULL,
  dedupe_key text NOT NULL,
  actor_id text NOT NULL DEFAULT '',
  resource_ref text NOT NULL DEFAULT '',
  status text NOT NULL,
  message text NOT NULL DEFAULT '',
  attempt integer NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_worker_task_events_task_id ON worker_task_events(task_id);
CREATE INDEX IF NOT EXISTS idx_worker_task_events_task_type ON worker_task_events(task_type);
CREATE INDEX IF NOT EXISTS idx_worker_task_events_dedupe_key ON worker_task_events(dedupe_key);
CREATE INDEX IF NOT EXISTS idx_worker_task_events_actor_id ON worker_task_events(actor_id);
CREATE INDEX IF NOT EXISTS idx_worker_task_events_resource_ref ON worker_task_events(resource_ref);
CREATE INDEX IF NOT EXISTS idx_worker_task_events_status ON worker_task_events(status);
