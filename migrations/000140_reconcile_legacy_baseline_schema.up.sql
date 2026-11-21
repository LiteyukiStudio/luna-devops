-- Reconcile pre-baseline legacy schemas with the flattened 000103+ baseline contract.
-- Safe and idempotent on both legacy upgraded databases and freshly bootstrapped databases.

-- 1. Remove retired pre-baseline tables that are no longer part of the application schema.
DROP TABLE IF EXISTS public.step_up_assertions;
DROP TABLE IF EXISTS public.mfa_recovery_codes;
DROP TABLE IF EXISTS public.user_mfa_configs;
DROP TABLE IF EXISTS public.project_wallets;
DROP TABLE IF EXISTS public.worker_task_events;
DROP TABLE IF EXISTS public.environments;

-- 2. Rename legacy GORM acronym columns on public.auth_admission_policies.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'auth_admission_policies' AND column_name = 'allow_o_id_c_login'
    ) THEN
        IF NOT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'auth_admission_policies' AND column_name = 'allow_oidc_login'
        ) THEN
            ALTER TABLE public.auth_admission_policies RENAME COLUMN allow_o_id_c_login TO allow_oidc_login;
        ELSE
            ALTER TABLE public.auth_admission_policies DROP COLUMN allow_o_id_c_login;
        END IF;
    END IF;

    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'auth_admission_policies' AND column_name = 'allowed_o_id_c_groups'
    ) THEN
        IF NOT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'auth_admission_policies' AND column_name = 'allowed_oidc_groups'
        ) THEN
            ALTER TABLE public.auth_admission_policies RENAME COLUMN allowed_o_id_c_groups TO allowed_oidc_groups;
        ELSE
            ALTER TABLE public.auth_admission_policies DROP COLUMN allowed_o_id_c_groups;
        END IF;
    END IF;

    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'auth_admission_policies' AND column_name = 'require_verified_o_id_c_email'
    ) THEN
        IF NOT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'auth_admission_policies' AND column_name = 'require_verified_oidc_email'
        ) THEN
            ALTER TABLE public.auth_admission_policies RENAME COLUMN require_verified_o_id_c_email TO require_verified_oidc_email;
        ELSE
            ALTER TABLE public.auth_admission_policies DROP COLUMN require_verified_o_id_c_email;
        END IF;
    END IF;
END $$;

-- 3. Drop retired pre-baseline columns.
ALTER TABLE ai.runs
    DROP COLUMN IF EXISTS heartbeat_at,
    DROP COLUMN IF EXISTS lease_owner,
    DROP COLUMN IF EXISTS run_actor_grant_ciphertext;
ALTER TABLE ai.tool_calls
    DROP COLUMN IF EXISTS approval_expires_at,
    DROP COLUMN IF EXISTS mfa_purpose;
ALTER TABLE public.applications
    DROP COLUMN IF EXISTS data_retention_mode;
ALTER TABLE public.deployment_targets
    DROP COLUMN IF EXISTS runtime_config_set_ids,
    DROP COLUMN IF EXISTS service_port;
ALTER TABLE public.runtime_clusters
    DROP COLUMN IF EXISTS gateway_http_s_listener_name,
    DROP COLUMN IF EXISTS gateway_http_s_listener_port;

-- 4. Align column types, defaults, and NOT NULL constraints.
UPDATE ai.runs SET actor_session_id = COALESCE(actor_session_id, ''::text) WHERE actor_session_id IS NULL;
ALTER TABLE ai.runs
    ALTER COLUMN actor_session_id SET NOT NULL;
UPDATE public.access_tokens SET created_at = COALESCE(created_at, now()), oauth_application_id = COALESCE(oauth_application_id, ''::text), oauth_grant_id = COALESCE(oauth_grant_id, ''::text), updated_at = COALESCE(updated_at, now()), user_id = COALESCE(user_id, ''::text) WHERE created_at IS NULL OR oauth_application_id IS NULL OR oauth_grant_id IS NULL OR updated_at IS NULL OR user_id IS NULL;
ALTER TABLE public.access_tokens
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN oauth_application_id SET DEFAULT ''::text,
    ALTER COLUMN oauth_application_id SET NOT NULL,
    ALTER COLUMN oauth_grant_id SET DEFAULT ''::text,
    ALTER COLUMN oauth_grant_id SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL,
    ALTER COLUMN user_id SET NOT NULL;
UPDATE public.app_configs SET updated_at = COALESCE(updated_at, now()) WHERE updated_at IS NULL;
ALTER TABLE public.app_configs
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL,
    ALTER COLUMN value SET DEFAULT ''::text;
UPDATE public.app_template_installations SET created_at = COALESCE(created_at, now()), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR updated_at IS NULL;
ALTER TABLE public.app_template_installations
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.applications SET created_at = COALESCE(created_at, now()), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR updated_at IS NULL;
ALTER TABLE public.applications
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.artifact_registries SET capabilities = COALESCE(capabilities, ''::text), created_at = COALESCE(created_at, now()), created_by = COALESCE(created_by, ''::text), credential_ref = COALESCE(credential_ref, ''::text), namespace = COALESCE(namespace, ''::text), owner_ref = COALESCE(owner_ref, ''::text), updated_at = COALESCE(updated_at, now()) WHERE capabilities IS NULL OR created_at IS NULL OR created_by IS NULL OR credential_ref IS NULL OR namespace IS NULL OR owner_ref IS NULL OR updated_at IS NULL;
ALTER TABLE public.artifact_registries
    ALTER COLUMN capabilities SET DEFAULT ''::text,
    ALTER COLUMN capabilities SET NOT NULL,
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN created_by SET DEFAULT ''::text,
    ALTER COLUMN created_by SET NOT NULL,
    ALTER COLUMN credential_ref SET DEFAULT ''::text,
    ALTER COLUMN credential_ref SET NOT NULL,
    ALTER COLUMN namespace SET DEFAULT ''::text,
    ALTER COLUMN namespace SET NOT NULL,
    ALTER COLUMN owner_ref SET DEFAULT ''::text,
    ALTER COLUMN owner_ref SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.audit_logs SET created_at = COALESCE(created_at, now()), message = COALESCE(message, ''::text), user_id = COALESCE(user_id, ''::text) WHERE created_at IS NULL OR message IS NULL OR user_id IS NULL;
ALTER TABLE public.audit_logs
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN message SET DEFAULT ''::text,
    ALTER COLUMN message SET NOT NULL,
    ALTER COLUMN user_id SET DEFAULT ''::text,
    ALTER COLUMN user_id SET NOT NULL;
UPDATE public.auth_admission_policies SET allowed_email_domains = COALESCE(allowed_email_domains, ''::text), allowed_oidc_groups = COALESCE(allowed_oidc_groups, ''::text), created_at = COALESCE(created_at, now()), invited_emails = COALESCE(invited_emails, ''::text), updated_at = COALESCE(updated_at, now()) WHERE allowed_email_domains IS NULL OR allowed_oidc_groups IS NULL OR created_at IS NULL OR invited_emails IS NULL OR updated_at IS NULL;
ALTER TABLE public.auth_admission_policies
    ALTER COLUMN allowed_email_domains SET DEFAULT ''::text,
    ALTER COLUMN allowed_email_domains SET NOT NULL,
    ALTER COLUMN allowed_oidc_groups SET DEFAULT ''::text,
    ALTER COLUMN allowed_oidc_groups SET NOT NULL,
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN invited_emails SET DEFAULT ''::text,
    ALTER COLUMN invited_emails SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.auth_providers SET client_secret_ref = COALESCE(client_secret_ref, ''::text), created_at = COALESCE(created_at, now()), updated_at = COALESCE(updated_at, now()) WHERE client_secret_ref IS NULL OR created_at IS NULL OR updated_at IS NULL;
ALTER TABLE public.auth_providers
    ALTER COLUMN client_secret_ref SET DEFAULT ''::text,
    ALTER COLUMN client_secret_ref SET NOT NULL,
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.auth_registration_settings SET created_at = COALESCE(created_at, now()), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR updated_at IS NULL;
ALTER TABLE public.auth_registration_settings
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.billing_ledger_entries SET created_at = COALESCE(created_at, now()), created_by = COALESCE(created_by, ''::text), description = COALESCE(description, ''::text), meter = COALESCE(meter, ''::text), resource_id = COALESCE(resource_id, ''::text), resource_type = COALESCE(resource_type, ''::text), usage_record_id = COALESCE(usage_record_id, ''::text) WHERE created_at IS NULL OR created_by IS NULL OR description IS NULL OR meter IS NULL OR resource_id IS NULL OR resource_type IS NULL OR usage_record_id IS NULL;
ALTER TABLE public.billing_ledger_entries
    ALTER COLUMN amount_credits SET DEFAULT 0,
    ALTER COLUMN balance_after_credits SET DEFAULT 0,
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN created_by SET DEFAULT ''::text,
    ALTER COLUMN created_by SET NOT NULL,
    ALTER COLUMN description SET DEFAULT ''::text,
    ALTER COLUMN description SET NOT NULL,
    ALTER COLUMN meter SET DEFAULT ''::text,
    ALTER COLUMN meter SET NOT NULL,
    ALTER COLUMN resource_id SET DEFAULT ''::text,
    ALTER COLUMN resource_id SET NOT NULL,
    ALTER COLUMN resource_type SET DEFAULT ''::text,
    ALTER COLUMN resource_type SET NOT NULL,
    ALTER COLUMN usage_record_id SET DEFAULT ''::text,
    ALTER COLUMN usage_record_id SET NOT NULL;
UPDATE public.billing_rate_rules SET created_at = COALESCE(created_at, now()), description = COALESCE(description, ''::text), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR description IS NULL OR updated_at IS NULL;
ALTER TABLE public.billing_rate_rules
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN credits_per_unit SET DEFAULT 0,
    ALTER COLUMN description SET DEFAULT ''::text,
    ALTER COLUMN description SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.billing_usage_records SET application_id = COALESCE(application_id, ''::text), created_at = COALESCE(created_at, now()), updated_at = COALESCE(updated_at, now()) WHERE application_id IS NULL OR created_at IS NULL OR updated_at IS NULL;
ALTER TABLE public.billing_usage_records
    ALTER COLUMN amount_credits SET DEFAULT 0,
    ALTER COLUMN application_id SET DEFAULT ''::text,
    ALTER COLUMN application_id SET NOT NULL,
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN quantity SET DEFAULT 0,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.build_jobs SET created_at = COALESCE(created_at, now()), executor_id = COALESCE(executor_id, ''::text), executor_name = COALESCE(executor_name, ''::text), log_ref = COALESCE(log_ref, ''::text), message = COALESCE(message, ''::text), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR executor_id IS NULL OR executor_name IS NULL OR log_ref IS NULL OR message IS NULL OR updated_at IS NULL;
ALTER TABLE public.build_jobs
    ALTER COLUMN attempts TYPE integer USING attempts::integer,
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN executor_id SET DEFAULT ''::text,
    ALTER COLUMN executor_id SET NOT NULL,
    ALTER COLUMN executor_name SET DEFAULT ''::text,
    ALTER COLUMN executor_name SET NOT NULL,
    ALTER COLUMN log_ref SET DEFAULT ''::text,
    ALTER COLUMN log_ref SET NOT NULL,
    ALTER COLUMN message SET DEFAULT ''::text,
    ALTER COLUMN message SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.build_logs SET content = COALESCE(content, ''::text), created_at = COALESCE(created_at, now()), updated_at = COALESCE(updated_at, now()) WHERE content IS NULL OR created_at IS NULL OR updated_at IS NULL;
ALTER TABLE public.build_logs
    ALTER COLUMN content SET DEFAULT ''::text,
    ALTER COLUMN content SET NOT NULL,
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.build_runs SET application_id = COALESCE(application_id, ''::text), build_directory = COALESCE(build_directory, ''::text), build_variable_set_ids = COALESCE(build_variable_set_ids, ''::text), created_at = COALESCE(created_at, now()), created_by = COALESCE(created_by, ''::text), deployment_target_id = COALESCE(deployment_target_id, ''::text), image_digest = COALESCE(image_digest, ''::text), image_ref = COALESCE(image_ref, ''::text), source_author_email = COALESCE(source_author_email, ''::text), source_author_name = COALESCE(source_author_name, ''::text), source_branch = COALESCE(source_branch, ''::text), source_commit = COALESCE(source_commit, ''::text), source_tag = COALESCE(source_tag, ''::text), target_registry_id = COALESCE(target_registry_id, ''::text), target_repository = COALESCE(target_repository, ''::text), target_tag = COALESCE(target_tag, ''::text), triggered_by_email = COALESCE(triggered_by_email, ''::text), triggered_by_name = COALESCE(triggered_by_name, ''::text), updated_at = COALESCE(updated_at, now()) WHERE application_id IS NULL OR build_directory IS NULL OR build_variable_set_ids IS NULL OR created_at IS NULL OR created_by IS NULL OR deployment_target_id IS NULL OR image_digest IS NULL OR image_ref IS NULL OR source_author_email IS NULL OR source_author_name IS NULL OR source_branch IS NULL OR source_commit IS NULL OR source_tag IS NULL OR target_registry_id IS NULL OR target_repository IS NULL OR target_tag IS NULL OR triggered_by_email IS NULL OR triggered_by_name IS NULL OR updated_at IS NULL;
ALTER TABLE public.build_runs
    ALTER COLUMN application_id SET DEFAULT ''::text,
    ALTER COLUMN application_id SET NOT NULL,
    ALTER COLUMN build_directory SET DEFAULT ''::text,
    ALTER COLUMN build_directory SET NOT NULL,
    ALTER COLUMN build_variable_set_ids SET DEFAULT ''::text,
    ALTER COLUMN build_variable_set_ids SET NOT NULL,
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN created_by SET DEFAULT ''::text,
    ALTER COLUMN created_by SET NOT NULL,
    ALTER COLUMN deployment_target_id SET DEFAULT ''::text,
    ALTER COLUMN deployment_target_id SET NOT NULL,
    ALTER COLUMN image_digest SET DEFAULT ''::text,
    ALTER COLUMN image_digest SET NOT NULL,
    ALTER COLUMN image_ref SET DEFAULT ''::text,
    ALTER COLUMN image_ref SET NOT NULL,
    ALTER COLUMN source_author_email SET DEFAULT ''::text,
    ALTER COLUMN source_author_email SET NOT NULL,
    ALTER COLUMN source_author_name SET DEFAULT ''::text,
    ALTER COLUMN source_author_name SET NOT NULL,
    ALTER COLUMN source_branch SET DEFAULT ''::text,
    ALTER COLUMN source_branch SET NOT NULL,
    ALTER COLUMN source_commit SET DEFAULT ''::text,
    ALTER COLUMN source_commit SET NOT NULL,
    ALTER COLUMN source_tag SET DEFAULT ''::text,
    ALTER COLUMN source_tag SET NOT NULL,
    ALTER COLUMN target_registry_id SET DEFAULT ''::text,
    ALTER COLUMN target_registry_id SET NOT NULL,
    ALTER COLUMN target_repository SET DEFAULT ''::text,
    ALTER COLUMN target_repository SET NOT NULL,
    ALTER COLUMN target_tag SET DEFAULT ''::text,
    ALTER COLUMN target_tag SET NOT NULL,
    ALTER COLUMN triggered_by_email SET DEFAULT ''::text,
    ALTER COLUMN triggered_by_email SET NOT NULL,
    ALTER COLUMN triggered_by_name SET DEFAULT ''::text,
    ALTER COLUMN triggered_by_name SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.build_variable_sets SET created_at = COALESCE(created_at, now()), created_by = COALESCE(created_by, ''::text), owner_ref = COALESCE(owner_ref, ''::text), updated_at = COALESCE(updated_at, now()), variables = COALESCE(variables, ''::text) WHERE created_at IS NULL OR created_by IS NULL OR owner_ref IS NULL OR updated_at IS NULL OR variables IS NULL;
ALTER TABLE public.build_variable_sets
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN created_by SET DEFAULT ''::text,
    ALTER COLUMN created_by SET NOT NULL,
    ALTER COLUMN owner_ref SET DEFAULT ''::text,
    ALTER COLUMN owner_ref SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL,
    ALTER COLUMN variables SET DEFAULT ''::text,
    ALTER COLUMN variables SET NOT NULL;
UPDATE public.container_images SET application_id = COALESCE(application_id, ''::text), build_run_id = COALESCE(build_run_id, ''::text), created_at = COALESCE(created_at, now()), created_by = COALESCE(created_by, ''::text), digest = COALESCE(digest, ''::text), project_id = COALESCE(project_id, ''::text), source_commit = COALESCE(source_commit, ''::text), updated_at = COALESCE(updated_at, now()) WHERE application_id IS NULL OR build_run_id IS NULL OR created_at IS NULL OR created_by IS NULL OR digest IS NULL OR project_id IS NULL OR source_commit IS NULL OR updated_at IS NULL;
ALTER TABLE public.container_images
    ALTER COLUMN application_id SET DEFAULT ''::text,
    ALTER COLUMN application_id SET NOT NULL,
    ALTER COLUMN build_run_id SET DEFAULT ''::text,
    ALTER COLUMN build_run_id SET NOT NULL,
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN created_by SET DEFAULT ''::text,
    ALTER COLUMN created_by SET NOT NULL,
    ALTER COLUMN digest SET DEFAULT ''::text,
    ALTER COLUMN digest SET NOT NULL,
    ALTER COLUMN project_id SET DEFAULT ''::text,
    ALTER COLUMN project_id SET NOT NULL,
    ALTER COLUMN source_commit SET DEFAULT ''::text,
    ALTER COLUMN source_commit SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.deployment_target_hook_bindings SET created_at = COALESCE(created_at, now()), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR updated_at IS NULL;
ALTER TABLE public.deployment_target_hook_bindings
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN run_order TYPE integer USING run_order::integer,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.deployment_targets SET branch_pattern = COALESCE(branch_pattern, ''::text), build_directory = COALESCE(build_directory, ''::text), build_variable_set_ids = COALESCE(build_variable_set_ids, ''::text), created_at = COALESCE(created_at, now()), created_by = COALESCE(created_by, ''::text), image_ref = COALESCE(image_ref, ''::text), repository_binding_id = COALESCE(repository_binding_id, ''::text), tag_pattern = COALESCE(tag_pattern, ''::text), target_registry_id = COALESCE(target_registry_id, ''::text), target_repository = COALESCE(target_repository, ''::text), target_tag = COALESCE(target_tag, ''::text), updated_at = COALESCE(updated_at, now()) WHERE branch_pattern IS NULL OR build_directory IS NULL OR build_variable_set_ids IS NULL OR created_at IS NULL OR created_by IS NULL OR image_ref IS NULL OR repository_binding_id IS NULL OR tag_pattern IS NULL OR target_registry_id IS NULL OR target_repository IS NULL OR target_tag IS NULL OR updated_at IS NULL;
ALTER TABLE public.deployment_targets
    ALTER COLUMN branch_pattern SET DEFAULT ''::text,
    ALTER COLUMN branch_pattern SET NOT NULL,
    ALTER COLUMN build_directory SET DEFAULT ''::text,
    ALTER COLUMN build_directory SET NOT NULL,
    ALTER COLUMN build_variable_set_ids SET DEFAULT ''::text,
    ALTER COLUMN build_variable_set_ids SET NOT NULL,
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN created_by SET DEFAULT ''::text,
    ALTER COLUMN created_by SET NOT NULL,
    ALTER COLUMN image_ref SET DEFAULT ''::text,
    ALTER COLUMN image_ref SET NOT NULL,
    ALTER COLUMN replicas TYPE integer USING replicas::integer,
    ALTER COLUMN repository_binding_id SET DEFAULT ''::text,
    ALTER COLUMN repository_binding_id SET NOT NULL,
    ALTER COLUMN tag_pattern SET DEFAULT ''::text,
    ALTER COLUMN tag_pattern SET NOT NULL,
    ALTER COLUMN target_registry_id SET DEFAULT ''::text,
    ALTER COLUMN target_registry_id SET NOT NULL,
    ALTER COLUMN target_repository SET DEFAULT ''::text,
    ALTER COLUMN target_repository SET NOT NULL,
    ALTER COLUMN target_tag SET DEFAULT ''::text,
    ALTER COLUMN target_tag SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.email_registration_challenges SET created_at = COALESCE(created_at, now()), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR updated_at IS NULL;
ALTER TABLE public.email_registration_challenges
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.external_identities SET created_at = COALESCE(created_at, now()), email = COALESCE(email, ''::text), updated_at = COALESCE(updated_at, now()), username = COALESCE(username, ''::text) WHERE created_at IS NULL OR email IS NULL OR updated_at IS NULL OR username IS NULL;
ALTER TABLE public.external_identities
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN email SET DEFAULT ''::text,
    ALTER COLUMN email SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL,
    ALTER COLUMN username SET DEFAULT ''::text,
    ALTER COLUMN username SET NOT NULL;
UPDATE public.gateway_routes SET cname_name = COALESCE(cname_name, ''::text), cname_target = COALESCE(cname_target, ''::text), created_at = COALESCE(created_at, now()), created_by = COALESCE(created_by, ''::text), updated_at = COALESCE(updated_at, now()) WHERE cname_name IS NULL OR cname_target IS NULL OR created_at IS NULL OR created_by IS NULL OR updated_at IS NULL;
ALTER TABLE public.gateway_routes
    ALTER COLUMN cname_name SET DEFAULT ''::text,
    ALTER COLUMN cname_name SET NOT NULL,
    ALTER COLUMN cname_target SET DEFAULT ''::text,
    ALTER COLUMN cname_target SET NOT NULL,
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN created_by SET DEFAULT ''::text,
    ALTER COLUMN created_by SET NOT NULL,
    ALTER COLUMN service_port TYPE integer USING service_port::integer,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.git_accounts SET access_token_ref = COALESCE(access_token_ref, ''::text), avatar_url = COALESCE(avatar_url, ''::text), created_at = COALESCE(created_at, now()), external_user_id = COALESCE(external_user_id, ''::text), owner_ref = COALESCE(owner_ref, ''::text), refresh_token_ref = COALESCE(refresh_token_ref, ''::text), scopes = COALESCE(scopes, ''::text), updated_at = COALESCE(updated_at, now()) WHERE access_token_ref IS NULL OR avatar_url IS NULL OR created_at IS NULL OR external_user_id IS NULL OR owner_ref IS NULL OR refresh_token_ref IS NULL OR scopes IS NULL OR updated_at IS NULL;
ALTER TABLE public.git_accounts
    ALTER COLUMN access_token_ref SET DEFAULT ''::text,
    ALTER COLUMN access_token_ref SET NOT NULL,
    ALTER COLUMN avatar_url SET DEFAULT ''::text,
    ALTER COLUMN avatar_url SET NOT NULL,
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN external_user_id SET DEFAULT ''::text,
    ALTER COLUMN external_user_id SET NOT NULL,
    ALTER COLUMN owner_ref SET DEFAULT ''::text,
    ALTER COLUMN owner_ref SET NOT NULL,
    ALTER COLUMN refresh_token_ref SET DEFAULT ''::text,
    ALTER COLUMN refresh_token_ref SET NOT NULL,
    ALTER COLUMN scopes SET DEFAULT ''::text,
    ALTER COLUMN scopes SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.git_providers SET client_id = COALESCE(client_id, ''::text), client_secret_ref = COALESCE(client_secret_ref, ''::text), created_at = COALESCE(created_at, now()), owner_ref = COALESCE(owner_ref, ''::text), updated_at = COALESCE(updated_at, now()) WHERE client_id IS NULL OR client_secret_ref IS NULL OR created_at IS NULL OR owner_ref IS NULL OR updated_at IS NULL;
ALTER TABLE public.git_providers
    ALTER COLUMN base_url SET DEFAULT ''::text,
    ALTER COLUMN client_id SET DEFAULT ''::text,
    ALTER COLUMN client_id SET NOT NULL,
    ALTER COLUMN client_secret_ref SET DEFAULT ''::text,
    ALTER COLUMN client_secret_ref SET NOT NULL,
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN owner_ref SET DEFAULT ''::text,
    ALTER COLUMN owner_ref SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.hook_run_logs SET content = COALESCE(content, ''::text), created_at = COALESCE(created_at, now()), updated_at = COALESCE(updated_at, now()) WHERE content IS NULL OR created_at IS NULL OR updated_at IS NULL;
ALTER TABLE public.hook_run_logs
    ALTER COLUMN content SET DEFAULT ''::text,
    ALTER COLUMN content SET NOT NULL,
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.hook_runs SET application_id = COALESCE(application_id, ''::text), build_job_id = COALESCE(build_job_id, ''::text), build_run_id = COALESCE(build_run_id, ''::text), created_at = COALESCE(created_at, now()), deployment_target_id = COALESCE(deployment_target_id, ''::text), exit_code = COALESCE(exit_code, 0), hook_config_id = COALESCE(hook_config_id, ''::text), image_ref = COALESCE(image_ref, ''::text), message = COALESCE(message, ''::text), release_id = COALESCE(release_id, ''::text), updated_at = COALESCE(updated_at, now()) WHERE application_id IS NULL OR build_job_id IS NULL OR build_run_id IS NULL OR created_at IS NULL OR deployment_target_id IS NULL OR exit_code IS NULL OR hook_config_id IS NULL OR image_ref IS NULL OR message IS NULL OR release_id IS NULL OR updated_at IS NULL;
ALTER TABLE public.hook_runs
    ALTER COLUMN application_id SET DEFAULT ''::text,
    ALTER COLUMN application_id SET NOT NULL,
    ALTER COLUMN build_job_id SET DEFAULT ''::text,
    ALTER COLUMN build_job_id SET NOT NULL,
    ALTER COLUMN build_run_id SET DEFAULT ''::text,
    ALTER COLUMN build_run_id SET NOT NULL,
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN deployment_target_id SET DEFAULT ''::text,
    ALTER COLUMN deployment_target_id SET NOT NULL,
    ALTER COLUMN exit_code TYPE integer USING exit_code::integer,
    ALTER COLUMN exit_code SET DEFAULT 0,
    ALTER COLUMN exit_code SET NOT NULL,
    ALTER COLUMN hook_config_id SET DEFAULT ''::text,
    ALTER COLUMN hook_config_id SET NOT NULL,
    ALTER COLUMN image_ref SET DEFAULT ''::text,
    ALTER COLUMN image_ref SET NOT NULL,
    ALTER COLUMN message SET DEFAULT ''::text,
    ALTER COLUMN message SET NOT NULL,
    ALTER COLUMN release_id SET DEFAULT ''::text,
    ALTER COLUMN release_id SET NOT NULL,
    ALTER COLUMN timeout_seconds TYPE integer USING timeout_seconds::integer,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.inbox_action_requests SET created_at = COALESCE(created_at, now()), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR updated_at IS NULL;
ALTER TABLE public.inbox_action_requests
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.inbox_messages SET created_at = COALESCE(created_at, now()), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR updated_at IS NULL;
ALTER TABLE public.inbox_messages
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.oauth_applications SET created_at = COALESCE(created_at, now()), description = COALESCE(description, ''::text), homepage_url = COALESCE(homepage_url, ''::text), logo_url = COALESCE(logo_url, ''::text), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR description IS NULL OR homepage_url IS NULL OR logo_url IS NULL OR updated_at IS NULL;
ALTER TABLE public.oauth_applications
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN description SET DEFAULT ''::text,
    ALTER COLUMN description SET NOT NULL,
    ALTER COLUMN homepage_url SET DEFAULT ''::text,
    ALTER COLUMN homepage_url SET NOT NULL,
    ALTER COLUMN logo_url SET DEFAULT ''::text,
    ALTER COLUMN logo_url SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.oauth_authorization_codes SET created_at = COALESCE(created_at, now()) WHERE created_at IS NULL;
ALTER TABLE public.oauth_authorization_codes
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL;
UPDATE public.oauth_grants SET created_at = COALESCE(created_at, now()), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR updated_at IS NULL;
ALTER TABLE public.oauth_grants
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.oauth_refresh_tokens SET created_at = COALESCE(created_at, now()), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR updated_at IS NULL;
ALTER TABLE public.oauth_refresh_tokens
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
ALTER TABLE public.platform_events
    ALTER COLUMN created_at SET DEFAULT now();
UPDATE public.project_hook_configs SET created_at = COALESCE(created_at, now()), created_by = COALESCE(created_by, ''::text), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR created_by IS NULL OR updated_at IS NULL;
ALTER TABLE public.project_hook_configs
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN created_by SET DEFAULT ''::text,
    ALTER COLUMN created_by SET NOT NULL,
    ALTER COLUMN timeout_seconds TYPE integer USING timeout_seconds::integer,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.project_members SET created_at = COALESCE(created_at, now()), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR updated_at IS NULL;
ALTER TABLE public.project_members
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN dashboard_order TYPE integer USING dashboard_order::integer,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL,
    ALTER COLUMN use_count TYPE integer USING use_count::integer;
UPDATE public.project_pins SET created_at = COALESCE(created_at, now()), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR updated_at IS NULL;
ALTER TABLE public.project_pins
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN pinned_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.project_runtime_config_sets SET created_at = COALESCE(created_at, now()), created_by = COALESCE(created_by, ''::text), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR created_by IS NULL OR updated_at IS NULL;
ALTER TABLE public.project_runtime_config_sets
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN created_by SET DEFAULT ''::text,
    ALTER COLUMN created_by SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.project_topology_edges SET created_at = COALESCE(created_at, now()), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR updated_at IS NULL;
ALTER TABLE public.project_topology_edges
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN created_by SET DEFAULT ''::text,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.projects SET created_at = COALESCE(created_at, now()), description = COALESCE(description, ''::text), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR description IS NULL OR updated_at IS NULL;
ALTER TABLE public.projects
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN description SET DEFAULT ''::text,
    ALTER COLUMN description SET NOT NULL,
    ALTER COLUMN max_concurrent_builds TYPE integer USING max_concurrent_builds::integer,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.registry_credentials SET created_at = COALESCE(created_at, now()), created_by = COALESCE(created_by, ''::text), owner_ref = COALESCE(owner_ref, ''::text), password_ref = COALESCE(password_ref, ''::text), token_ref = COALESCE(token_ref, ''::text), updated_at = COALESCE(updated_at, now()), username = COALESCE(username, ''::text) WHERE created_at IS NULL OR created_by IS NULL OR owner_ref IS NULL OR password_ref IS NULL OR token_ref IS NULL OR updated_at IS NULL OR username IS NULL;
ALTER TABLE public.registry_credentials
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN created_by SET DEFAULT ''::text,
    ALTER COLUMN created_by SET NOT NULL,
    ALTER COLUMN owner_ref SET DEFAULT ''::text,
    ALTER COLUMN owner_ref SET NOT NULL,
    ALTER COLUMN password_ref SET DEFAULT ''::text,
    ALTER COLUMN password_ref SET NOT NULL,
    ALTER COLUMN token_ref SET DEFAULT ''::text,
    ALTER COLUMN token_ref SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL,
    ALTER COLUMN username SET DEFAULT ''::text,
    ALTER COLUMN username SET NOT NULL;
UPDATE public.release_logs SET content = COALESCE(content, ''::text), created_at = COALESCE(created_at, now()), updated_at = COALESCE(updated_at, now()) WHERE content IS NULL OR created_at IS NULL OR updated_at IS NULL;
ALTER TABLE public.release_logs
    ALTER COLUMN content SET DEFAULT ''::text,
    ALTER COLUMN content SET NOT NULL,
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.releases SET build_run_id = COALESCE(build_run_id, ''::text), created_at = COALESCE(created_at, now()), created_by = COALESCE(created_by, ''::text), message = COALESCE(message, ''::text), rollback_from_id = COALESCE(rollback_from_id, ''::text), updated_at = COALESCE(updated_at, now()) WHERE build_run_id IS NULL OR created_at IS NULL OR created_by IS NULL OR message IS NULL OR rollback_from_id IS NULL OR updated_at IS NULL;
ALTER TABLE public.releases
    ALTER COLUMN build_run_id SET DEFAULT ''::text,
    ALTER COLUMN build_run_id SET NOT NULL,
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN created_by SET DEFAULT ''::text,
    ALTER COLUMN created_by SET NOT NULL,
    ALTER COLUMN message SET DEFAULT ''::text,
    ALTER COLUMN message SET NOT NULL,
    ALTER COLUMN revision TYPE integer USING revision::integer,
    ALTER COLUMN rollback_from_id SET DEFAULT ''::text,
    ALTER COLUMN rollback_from_id SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.repository_bindings SET created_at = COALESCE(created_at, now()), credential_ref = COALESCE(credential_ref, ''::text), last_commit_sha = COALESCE(last_commit_sha, ''::text), last_event = COALESCE(last_event, ''::text), updated_at = COALESCE(updated_at, now()), webhook_id = COALESCE(webhook_id, ''::text), webhook_secret = COALESCE(webhook_secret, ''::text) WHERE created_at IS NULL OR credential_ref IS NULL OR last_commit_sha IS NULL OR last_event IS NULL OR updated_at IS NULL OR webhook_id IS NULL OR webhook_secret IS NULL;
ALTER TABLE public.repository_bindings
    ALTER COLUMN clone_url SET DEFAULT ''::text,
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN credential_ref SET DEFAULT ''::text,
    ALTER COLUMN credential_ref SET NOT NULL,
    ALTER COLUMN last_commit_sha SET DEFAULT ''::text,
    ALTER COLUMN last_commit_sha SET NOT NULL,
    ALTER COLUMN last_event SET DEFAULT ''::text,
    ALTER COLUMN last_event SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL,
    ALTER COLUMN webhook_id SET DEFAULT ''::text,
    ALTER COLUMN webhook_id SET NOT NULL,
    ALTER COLUMN webhook_secret SET DEFAULT ''::text,
    ALTER COLUMN webhook_secret SET NOT NULL;
UPDATE public.retained_volumes SET created_at = COALESCE(created_at, now()), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR updated_at IS NULL;
ALTER TABLE public.retained_volumes
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.runtime_clusters SET created_at = COALESCE(created_at, now()), created_by = COALESCE(created_by, ''::text), endpoint = COALESCE(endpoint, ''::text), kubeconfig_ref = COALESCE(kubeconfig_ref, ''::text), owner_ref = COALESCE(owner_ref, ''::text), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR created_by IS NULL OR endpoint IS NULL OR kubeconfig_ref IS NULL OR owner_ref IS NULL OR updated_at IS NULL;
ALTER TABLE public.runtime_clusters
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN created_by SET DEFAULT ''::text,
    ALTER COLUMN created_by SET NOT NULL,
    ALTER COLUMN endpoint SET DEFAULT ''::text,
    ALTER COLUMN endpoint SET NOT NULL,
    ALTER COLUMN kubeconfig_ref SET DEFAULT ''::text,
    ALTER COLUMN kubeconfig_ref SET NOT NULL,
    ALTER COLUMN max_concurrent_builds TYPE integer USING max_concurrent_builds::integer,
    ALTER COLUMN owner_ref SET DEFAULT ''::text,
    ALTER COLUMN owner_ref SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.scoped_resource_project_bindings SET created_at = COALESCE(created_at, now()), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR updated_at IS NULL;
ALTER TABLE public.scoped_resource_project_bindings
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.secret_values SET created_at = COALESCE(created_at, now()), created_by = COALESCE(created_by, ''::text), resource = COALESCE(resource, ''::text), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR created_by IS NULL OR resource IS NULL OR updated_at IS NULL;
ALTER TABLE public.secret_values
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN created_by SET DEFAULT ''::text,
    ALTER COLUMN created_by SET NOT NULL,
    ALTER COLUMN resource SET DEFAULT ''::text,
    ALTER COLUMN resource SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.service_bindings SET created_at = COALESCE(created_at, now()), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR updated_at IS NULL;
ALTER TABLE public.service_bindings
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN created_by SET DEFAULT ''::text,
    ALTER COLUMN target_port TYPE integer USING target_port::integer,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.system_component_installations SET created_at = COALESCE(created_at, now()), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR updated_at IS NULL;
ALTER TABLE public.system_component_installations
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.user_sessions SET created_at = COALESCE(created_at, now()), impersonator_id = COALESCE(impersonator_id, ''::text), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR impersonator_id IS NULL OR updated_at IS NULL;
ALTER TABLE public.user_sessions
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN impersonator_id SET DEFAULT ''::text,
    ALTER COLUMN impersonator_id SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.user_wallets SET created_at = COALESCE(created_at, now()), updated_at = COALESCE(updated_at, now()) WHERE created_at IS NULL OR updated_at IS NULL;
ALTER TABLE public.user_wallets
    ALTER COLUMN balance_credits SET DEFAULT 0,
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE public.users SET avatar_url = COALESCE(avatar_url, ''::text), created_at = COALESCE(created_at, now()), password = COALESCE(password, ''::text), updated_at = COALESCE(updated_at, now()) WHERE avatar_url IS NULL OR created_at IS NULL OR password IS NULL OR updated_at IS NULL;
ALTER TABLE public.users
    ALTER COLUMN avatar_url SET DEFAULT ''::text,
    ALTER COLUMN avatar_url SET NOT NULL,
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN password SET DEFAULT ''::text,
    ALTER COLUMN password SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;

-- 5. Restore missing UNIQUE and FOREIGN KEY constraints.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        WHERE n.nspname = 'public' AND cl.relname = 'project_members' AND c.conname = 'project_members_project_id_user_id_key'
    ) THEN
        ALTER TABLE public.project_members ADD CONSTRAINT project_members_project_id_user_id_key UNIQUE (project_id, user_id);
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        WHERE n.nspname = 'public' AND cl.relname = 'scoped_resource_project_bindings' AND c.conname = 'scoped_resource_project_bindi_resource_type_resource_id_pro_key'
    ) THEN
        ALTER TABLE public.scoped_resource_project_bindings ADD CONSTRAINT scoped_resource_project_bindi_resource_type_resource_id_pro_key UNIQUE (resource_type, resource_id, project_id);
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        WHERE n.nspname = 'public' AND cl.relname = 'access_tokens' AND c.conname = 'access_tokens_user_id_fkey'
    ) THEN
        ALTER TABLE public.access_tokens ADD CONSTRAINT access_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        WHERE n.nspname = 'public' AND cl.relname = 'applications' AND c.conname = 'applications_project_id_fkey'
    ) THEN
        ALTER TABLE public.applications ADD CONSTRAINT applications_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        WHERE n.nspname = 'public' AND cl.relname = 'container_images' AND c.conname = 'container_images_registry_id_fkey'
    ) THEN
        ALTER TABLE public.container_images ADD CONSTRAINT container_images_registry_id_fkey FOREIGN KEY (registry_id) REFERENCES artifact_registries(id) ON DELETE RESTRICT;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        WHERE n.nspname = 'public' AND cl.relname = 'external_identities' AND c.conname = 'external_identities_provider_id_fkey'
    ) THEN
        ALTER TABLE public.external_identities ADD CONSTRAINT external_identities_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES auth_providers(id) ON DELETE RESTRICT;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        WHERE n.nspname = 'public' AND cl.relname = 'external_identities' AND c.conname = 'external_identities_user_id_fkey'
    ) THEN
        ALTER TABLE public.external_identities ADD CONSTRAINT external_identities_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        WHERE n.nspname = 'public' AND cl.relname = 'git_accounts' AND c.conname = 'git_accounts_provider_id_fkey'
    ) THEN
        ALTER TABLE public.git_accounts ADD CONSTRAINT git_accounts_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES git_providers(id) ON DELETE RESTRICT;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        WHERE n.nspname = 'public' AND cl.relname = 'git_accounts' AND c.conname = 'git_accounts_user_id_fkey'
    ) THEN
        ALTER TABLE public.git_accounts ADD CONSTRAINT git_accounts_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        WHERE n.nspname = 'public' AND cl.relname = 'hook_run_logs' AND c.conname = 'hook_run_logs_project_id_fkey'
    ) THEN
        ALTER TABLE public.hook_run_logs ADD CONSTRAINT hook_run_logs_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        WHERE n.nspname = 'public' AND cl.relname = 'hook_runs' AND c.conname = 'hook_runs_project_id_fkey'
    ) THEN
        ALTER TABLE public.hook_runs ADD CONSTRAINT hook_runs_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        WHERE n.nspname = 'public' AND cl.relname = 'project_hook_configs' AND c.conname = 'project_hook_configs_project_id_fkey'
    ) THEN
        ALTER TABLE public.project_hook_configs ADD CONSTRAINT project_hook_configs_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        WHERE n.nspname = 'public' AND cl.relname = 'project_members' AND c.conname = 'project_members_project_id_fkey'
    ) THEN
        ALTER TABLE public.project_members ADD CONSTRAINT project_members_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        WHERE n.nspname = 'public' AND cl.relname = 'project_members' AND c.conname = 'project_members_user_id_fkey'
    ) THEN
        ALTER TABLE public.project_members ADD CONSTRAINT project_members_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        WHERE n.nspname = 'public' AND cl.relname = 'project_pins' AND c.conname = 'project_pins_project_id_fkey'
    ) THEN
        ALTER TABLE public.project_pins ADD CONSTRAINT project_pins_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        WHERE n.nspname = 'public' AND cl.relname = 'project_pins' AND c.conname = 'project_pins_user_id_fkey'
    ) THEN
        ALTER TABLE public.project_pins ADD CONSTRAINT project_pins_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        WHERE n.nspname = 'public' AND cl.relname = 'registry_credentials' AND c.conname = 'registry_credentials_registry_id_fkey'
    ) THEN
        ALTER TABLE public.registry_credentials ADD CONSTRAINT registry_credentials_registry_id_fkey FOREIGN KEY (registry_id) REFERENCES artifact_registries(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        WHERE n.nspname = 'public' AND cl.relname = 'repository_bindings' AND c.conname = 'repository_bindings_application_id_fkey'
    ) THEN
        ALTER TABLE public.repository_bindings ADD CONSTRAINT repository_bindings_application_id_fkey FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        WHERE n.nspname = 'public' AND cl.relname = 'repository_bindings' AND c.conname = 'repository_bindings_git_account_id_fkey'
    ) THEN
        ALTER TABLE public.repository_bindings ADD CONSTRAINT repository_bindings_git_account_id_fkey FOREIGN KEY (git_account_id) REFERENCES git_accounts(id) ON DELETE RESTRICT;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        WHERE n.nspname = 'public' AND cl.relname = 'repository_bindings' AND c.conname = 'repository_bindings_git_provider_id_fkey'
    ) THEN
        ALTER TABLE public.repository_bindings ADD CONSTRAINT repository_bindings_git_provider_id_fkey FOREIGN KEY (git_provider_id) REFERENCES git_providers(id) ON DELETE RESTRICT;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        WHERE n.nspname = 'public' AND cl.relname = 'repository_bindings' AND c.conname = 'repository_bindings_project_id_fkey'
    ) THEN
        ALTER TABLE public.repository_bindings ADD CONSTRAINT repository_bindings_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        WHERE n.nspname = 'public' AND cl.relname = 'scoped_resource_project_bindings' AND c.conname = 'scoped_resource_project_bindings_project_id_fkey'
    ) THEN
        ALTER TABLE public.scoped_resource_project_bindings ADD CONSTRAINT scoped_resource_project_bindings_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        WHERE n.nspname = 'public' AND cl.relname = 'user_sessions' AND c.conname = 'user_sessions_user_id_fkey'
    ) THEN
        ALTER TABLE public.user_sessions ADD CONSTRAINT user_sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = cl.relnamespace
        WHERE n.nspname = 'public' AND cl.relname = 'user_wallets' AND c.conname = 'user_wallets_user_id_fkey'
    ) THEN
        ALTER TABLE public.user_wallets ADD CONSTRAINT user_wallets_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
    END IF;
END $$;

-- 6. Reconcile missing baseline indexes and drop superseded pre-baseline indexes.
CREATE INDEX IF NOT EXISTS idx_artifact_registries_scope_owner ON public.artifact_registries USING btree (scope, owner_ref) WHERE (deleted_at IS NULL);
CREATE UNIQUE INDEX IF NOT EXISTS idx_artifact_registries_default_global ON public.artifact_registries USING btree (scope) WHERE ((deleted_at IS NULL) AND (scope = 'global'::text) AND is_default);
CREATE UNIQUE INDEX IF NOT EXISTS idx_artifact_registries_default_project ON public.artifact_registries USING btree (scope, owner_ref) WHERE ((deleted_at IS NULL) AND (scope = 'project'::text) AND is_default);
CREATE UNIQUE INDEX IF NOT EXISTS idx_artifact_registries_default_user ON public.artifact_registries USING btree (scope, owner_ref) WHERE ((deleted_at IS NULL) AND (scope = 'user'::text) AND is_default);
CREATE INDEX IF NOT EXISTS idx_container_images_application ON public.container_images USING btree (application_id) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_container_images_project ON public.container_images USING btree (project_id) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_container_images_registry_repo ON public.container_images USING btree (registry_id, repository) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_git_accounts_scope_owner_ref ON public.git_accounts USING btree (scope, owner_ref);
CREATE INDEX IF NOT EXISTS idx_git_accounts_user_provider ON public.git_accounts USING btree (user_id, provider_id) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_git_providers_scope_owner_ref ON public.git_providers USING btree (scope, owner_ref);
CREATE INDEX IF NOT EXISTS idx_project_members_use_count ON public.project_members USING btree (use_count);
CREATE INDEX IF NOT EXISTS idx_project_members_user_dashboard_order ON public.project_members USING btree (user_id, dashboard_order);
CREATE INDEX IF NOT EXISTS idx_project_pins_user_pinned_at ON public.project_pins USING btree (user_id, pinned_at DESC);
CREATE INDEX IF NOT EXISTS idx_registry_credentials_registry ON public.registry_credentials USING btree (registry_id) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_repository_bindings_project ON public.repository_bindings USING btree (project_id) WHERE (deleted_at IS NULL);
CREATE UNIQUE INDEX IF NOT EXISTS idx_repository_bindings_application_repo_active ON public.repository_bindings USING btree (application_id, git_account_id, owner, repo) WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_scoped_resource_project_bindings_type_id ON public.scoped_resource_project_bindings USING btree (resource_type, resource_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_scoped_resource_project_bindings_default_registry ON public.scoped_resource_project_bindings USING btree (project_id) WHERE ((resource_type = 'artifact_registry'::text) AND is_default);
DROP INDEX IF EXISTS public.idx_build_runs_build_environment_id;
DROP INDEX IF EXISTS public.idx_deployment_targets_delete_status;
DROP INDEX IF EXISTS public.idx_gateway_routes_delete_status;
DROP INDEX IF EXISTS public.idx_platform_events_created_at;
DROP INDEX IF EXISTS public.idx_platform_events_resource_id;
DROP INDEX IF EXISTS public.idx_platform_events_resource_type;
DROP INDEX IF EXISTS public.idx_platform_events_trace_id;
DROP INDEX IF EXISTS public.idx_project_pins_pinned_at;
DROP INDEX IF EXISTS public.idx_project_topology_edges_created_by;
DROP INDEX IF EXISTS public.idx_project_topology_edges_relation_type;
DROP INDEX IF EXISTS public.idx_projects_delete_status;
DROP INDEX IF EXISTS public.idx_registry_credentials_owner_ref;
DROP INDEX IF EXISTS public.idx_registry_credentials_scope;
DROP INDEX IF EXISTS public.idx_retained_volumes_claimed_by_application_id;
DROP INDEX IF EXISTS public.idx_retained_volumes_cluster_id;
DROP INDEX IF EXISTS public.idx_retained_volumes_project_id;
DROP INDEX IF EXISTS public.idx_retained_volumes_source_deployment_target_id;
DROP INDEX IF EXISTS public.idx_retained_volumes_status;
DROP INDEX IF EXISTS public.idx_scoped_resource_project_bindings_is_default;
DROP INDEX IF EXISTS public.idx_scoped_resource_project_bindings_resource_id;
DROP INDEX IF EXISTS public.idx_scoped_resource_project_bindings_resource_type;
DROP INDEX IF EXISTS public.idx_service_bindings_created_by;
DROP INDEX IF EXISTS public.idx_system_component_installations_namespace;
