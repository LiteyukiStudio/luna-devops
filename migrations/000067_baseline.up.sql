-- Luna DevOps pre-release baseline through schema version 67.


CREATE SCHEMA IF NOT EXISTS ai;

CREATE TABLE ai.conversation_summaries (
    conversation_id text NOT NULL,
    covered_through_turn_index integer NOT NULL,
    compression_version integer NOT NULL,
    source_turn_count integer NOT NULL,
    content jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT conversation_summaries_compression_version_check CHECK ((compression_version = 1)),
    CONSTRAINT conversation_summaries_covered_through_turn_index_check CHECK ((covered_through_turn_index >= 0)),
    CONSTRAINT conversation_summaries_source_turn_count_check CHECK ((source_turn_count > 0))
);

CREATE TABLE ai.conversations (
    id text NOT NULL,
    owner_user_id text NOT NULL,
    project_id text,
    title text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    title_source text DEFAULT 'default'::text NOT NULL,
    CONSTRAINT conversations_status_check CHECK ((status = 'active'::text)),
    CONSTRAINT conversations_title_source_check CHECK ((title_source = ANY (ARRAY['default'::text, 'assistant'::text, 'user'::text])))
);

CREATE TABLE ai.idempotency_keys (
    owner_user_id text NOT NULL,
    idempotency_key text NOT NULL,
    request_hash text NOT NULL,
    turn_id text NOT NULL,
    run_id text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE ai.items (
    id text NOT NULL,
    run_id text NOT NULL,
    turn_id text NOT NULL,
    timeline_index integer NOT NULL,
    type text NOT NULL,
    status text NOT NULL,
    content jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    revision bigint DEFAULT 1 NOT NULL
);

CREATE TABLE ai.run_events (
    id text NOT NULL,
    run_id text NOT NULL,
    event_sequence bigint NOT NULL,
    type text NOT NULL,
    data jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE ai.runs (
    id text NOT NULL,
    owner_user_id text NOT NULL,
    conversation_id text NOT NULL,
    turn_id text NOT NULL,
    run_index integer NOT NULL,
    status text NOT NULL,
    row_version integer DEFAULT 1 NOT NULL,
    prompt_version text NOT NULL,
    tool_catalog_digest text NOT NULL,
    page_context jsonb DEFAULT '{}'::jsonb NOT NULL,
    actor_session_id text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    error_code text,
    trace_context jsonb DEFAULT '{}'::jsonb NOT NULL,
    next_item_position bigint DEFAULT 0 NOT NULL,
    next_event_sequence bigint DEFAULT 1 NOT NULL,
    CONSTRAINT ai_runs_trace_context_object_check CHECK ((jsonb_typeof(trace_context) = 'object'::text))
);

CREATE TABLE ai.tool_calls (
    id text NOT NULL,
    run_id text NOT NULL,
    operation_id text NOT NULL,
    status text NOT NULL,
    arguments jsonb NOT NULL,
    arguments_hash text DEFAULT ''::text NOT NULL,
    attempt integer DEFAULT 1 NOT NULL,
    row_version integer DEFAULT 1 NOT NULL,
    approval_decision text,
    result jsonb,
    error_code text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    arguments_ciphertext text,
    CONSTRAINT tool_calls_approval_decision_check CHECK ((approval_decision = 'approve'::text))
);

CREATE TABLE ai.turns (
    id text NOT NULL,
    conversation_id text NOT NULL,
    turn_index integer NOT NULL,
    status text NOT NULL,
    input text NOT NULL,
    selected_run_id text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE access_tokens (
    id text NOT NULL,
    user_id text NOT NULL,
    name text NOT NULL,
    scope text NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    source text DEFAULT 'personal'::text NOT NULL,
    oauth_application_id text DEFAULT ''::text NOT NULL,
    oauth_grant_id text DEFAULT ''::text NOT NULL
);

CREATE TABLE app_configs (
    key text NOT NULL,
    value text DEFAULT ''::text NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE app_template_installations (
    id text NOT NULL,
    template_id text NOT NULL,
    template_version text DEFAULT ''::text NOT NULL,
    project_id text NOT NULL,
    application_id text NOT NULL,
    deployment_target_id text DEFAULT ''::text NOT NULL,
    release_id text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'installed'::text NOT NULL,
    message text DEFAULT ''::text NOT NULL,
    values_snapshot text DEFAULT '{}'::text NOT NULL,
    created_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);

CREATE TABLE applications (
    id text NOT NULL,
    project_id text NOT NULL,
    identifier text NOT NULL,
    name text NOT NULL,
    icon text DEFAULT 'box'::text NOT NULL,
    delete_status text DEFAULT 'active'::text NOT NULL,
    delete_message text DEFAULT ''::text NOT NULL,
    delete_started_at timestamp with time zone,
    delete_finished_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);

CREATE TABLE artifact_registries (
    id text NOT NULL,
    name text NOT NULL,
    provider text NOT NULL,
    endpoint text NOT NULL,
    namespace text DEFAULT ''::text NOT NULL,
    scope text DEFAULT 'global'::text NOT NULL,
    owner_ref text DEFAULT ''::text NOT NULL,
    credential_ref text DEFAULT ''::text NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    capabilities text DEFAULT ''::text NOT NULL,
    created_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);

CREATE TABLE audit_logs (
    id text NOT NULL,
    user_id text DEFAULT ''::text NOT NULL,
    action text NOT NULL,
    resource text NOT NULL,
    success boolean DEFAULT true NOT NULL,
    message text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE auth_admission_policies (
    id text NOT NULL,
    allow_local_login boolean DEFAULT true NOT NULL,
    allow_oidc_login boolean DEFAULT true NOT NULL,
    require_verified_oidc_email boolean DEFAULT true NOT NULL,
    allowed_email_domains text DEFAULT ''::text NOT NULL,
    allowed_oidc_groups text DEFAULT ''::text NOT NULL,
    invited_emails text DEFAULT ''::text NOT NULL,
    default_role text DEFAULT 'user'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE auth_providers (
    id text NOT NULL,
    type text NOT NULL,
    name text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    issuer_url text NOT NULL,
    client_id text NOT NULL,
    client_secret_ref text DEFAULT ''::text NOT NULL,
    scopes text DEFAULT 'openid profile email'::text NOT NULL,
    group_claim text DEFAULT 'groups'::text NOT NULL,
    email_claim text DEFAULT 'email'::text NOT NULL,
    username_claim text DEFAULT 'preferred_username'::text NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);

CREATE TABLE auth_registration_settings (
    id text NOT NULL,
    allow_email_registration boolean DEFAULT false NOT NULL,
    allow_oidc_registration boolean DEFAULT true NOT NULL,
    allow_external_identity_password boolean DEFAULT false NOT NULL,
    smtp_host text DEFAULT ''::text NOT NULL,
    smtp_port integer DEFAULT 587 NOT NULL,
    smtp_security text DEFAULT 'starttls'::text NOT NULL,
    smtp_username text DEFAULT ''::text NOT NULL,
    smtp_password_ref text DEFAULT ''::text NOT NULL,
    smtp_from_address text DEFAULT ''::text NOT NULL,
    smtp_from_name text DEFAULT 'Luna DevOps'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE billing_ledger_entries (
    id text NOT NULL,
    project_id text DEFAULT ''::text,
    type text NOT NULL,
    amount_credits numeric(24,8) DEFAULT 0 NOT NULL,
    balance_after_credits numeric(24,8) DEFAULT 0 NOT NULL,
    reason text NOT NULL,
    meter text DEFAULT ''::text NOT NULL,
    usage_record_id text DEFAULT ''::text NOT NULL,
    resource_type text DEFAULT ''::text NOT NULL,
    resource_id text DEFAULT ''::text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    created_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    idempotency_key text DEFAULT ''::text NOT NULL,
    user_id text DEFAULT ''::text NOT NULL
);

CREATE TABLE billing_rate_rules (
    id text NOT NULL,
    meter text NOT NULL,
    unit text NOT NULL,
    credits_per_unit numeric(24,8) DEFAULT 0 NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE billing_usage_records (
    id text NOT NULL,
    project_id text NOT NULL,
    application_id text DEFAULT ''::text NOT NULL,
    meter text NOT NULL,
    quantity numeric(24,8) DEFAULT 0 NOT NULL,
    unit text NOT NULL,
    amount_credits numeric(24,8) DEFAULT 0 NOT NULL,
    resource_type text NOT NULL,
    resource_id text NOT NULL,
    period_start timestamp with time zone NOT NULL,
    period_end timestamp with time zone NOT NULL,
    status text DEFAULT 'settled'::text NOT NULL,
    metadata text DEFAULT ''::text NOT NULL,
    settled_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    billed_user_id text DEFAULT ''::text NOT NULL
);

CREATE TABLE build_environment_configs (
    id text NOT NULL,
    scope text NOT NULL,
    scope_ref text NOT NULL,
    variables text DEFAULT '{}'::text NOT NULL,
    secret_refs text DEFAULT '{}'::text NOT NULL,
    updated_by text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE build_jobs (
    id text NOT NULL,
    build_run_id text NOT NULL,
    project_id text NOT NULL,
    type text DEFAULT 'build'::text NOT NULL,
    status text DEFAULT 'queued'::text NOT NULL,
    builder_id text DEFAULT ''::text NOT NULL,
    lease_token text DEFAULT ''::text NOT NULL,
    lease_until timestamp with time zone,
    last_heartbeat_at timestamp with time zone,
    executor_id text DEFAULT ''::text NOT NULL,
    executor_name text DEFAULT ''::text NOT NULL,
    message text DEFAULT ''::text NOT NULL,
    log_ref text DEFAULT ''::text NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);

CREATE TABLE build_logs (
    id text NOT NULL,
    build_run_id text NOT NULL,
    build_job_id text NOT NULL,
    project_id text NOT NULL,
    content text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE build_runs (
    id text NOT NULL,
    project_id text NOT NULL,
    application_id text DEFAULT ''::text NOT NULL,
    deployment_target_id text DEFAULT ''::text NOT NULL,
    build_labels text DEFAULT ''::text NOT NULL,
    build_variable_set_ids text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'queued'::text NOT NULL,
    trigger_type text DEFAULT 'manual'::text NOT NULL,
    source_branch text DEFAULT ''::text NOT NULL,
    source_tag text DEFAULT ''::text NOT NULL,
    source_commit text DEFAULT ''::text NOT NULL,
    dockerfile_path text DEFAULT 'Dockerfile'::text NOT NULL,
    build_context text DEFAULT '.'::text NOT NULL,
    build_directory text DEFAULT ''::text NOT NULL,
    build_args text DEFAULT ''::text NOT NULL,
    target_registry_id text DEFAULT ''::text NOT NULL,
    target_repository text DEFAULT ''::text NOT NULL,
    target_tag text DEFAULT ''::text NOT NULL,
    image_ref text DEFAULT ''::text NOT NULL,
    image_digest text DEFAULT ''::text NOT NULL,
    cache_config text DEFAULT ''::text NOT NULL,
    cpu_core_seconds bigint DEFAULT 0 NOT NULL,
    memory_mb_seconds bigint DEFAULT 0 NOT NULL,
    credit_cost bigint DEFAULT 0 NOT NULL,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    created_by text DEFAULT ''::text NOT NULL,
    triggered_by_name text DEFAULT ''::text NOT NULL,
    triggered_by_email text DEFAULT ''::text NOT NULL,
    source_author_name text DEFAULT ''::text NOT NULL,
    source_author_email text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    build_cpu_request text DEFAULT '1'::text NOT NULL,
    build_memory_request text DEFAULT '1Gi'::text NOT NULL,
    build_environment_id text DEFAULT ''::text NOT NULL,
    build_timeout_seconds integer DEFAULT 1800 NOT NULL,
    build_definition_mode text DEFAULT 'repository_dockerfile'::text NOT NULL,
    build_template_id text DEFAULT ''::text NOT NULL,
    build_template_version text DEFAULT ''::text NOT NULL,
    build_template_values text DEFAULT '{}'::text NOT NULL,
    build_template_dockerfile text DEFAULT ''::text NOT NULL,
    build_template_checksum text DEFAULT ''::text NOT NULL,
    build_variables_snapshot text DEFAULT '{}'::text NOT NULL,
    build_secret_refs_snapshot text DEFAULT '{}'::text NOT NULL
);

CREATE TABLE build_variable_sets (
    id text NOT NULL,
    name text NOT NULL,
    scope text DEFAULT 'global'::text NOT NULL,
    owner_ref text DEFAULT ''::text NOT NULL,
    variables text DEFAULT ''::text NOT NULL,
    secret_refs text DEFAULT ''::text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    created_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);

CREATE TABLE container_images (
    id text NOT NULL,
    project_id text DEFAULT ''::text NOT NULL,
    application_id text DEFAULT ''::text NOT NULL,
    registry_id text NOT NULL,
    repository text NOT NULL,
    tag text NOT NULL,
    digest text DEFAULT ''::text NOT NULL,
    image_ref text NOT NULL,
    source_commit text DEFAULT ''::text NOT NULL,
    build_run_id text DEFAULT ''::text NOT NULL,
    source_type text DEFAULT 'manual-image'::text NOT NULL,
    scan_status text DEFAULT 'unknown'::text NOT NULL,
    created_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);

CREATE TABLE deployment_target_hook_bindings (
    id text NOT NULL,
    project_id text NOT NULL,
    application_id text NOT NULL,
    target_id text NOT NULL,
    hook_config_id text NOT NULL,
    phase text NOT NULL,
    run_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE deployment_targets (
    id text NOT NULL,
    project_id text NOT NULL,
    application_id text NOT NULL,
    environment_id text DEFAULT ''::text NOT NULL,
    name text NOT NULL,
    stage text DEFAULT 'prod'::text NOT NULL,
    cluster_id text DEFAULT ''::text NOT NULL,
    namespace text DEFAULT ''::text NOT NULL,
    replicas integer DEFAULT 1 NOT NULL,
    cpu_request text DEFAULT '1'::text NOT NULL,
    memory_request text DEFAULT '1Gi'::text NOT NULL,
    delete_status text DEFAULT 'active'::text NOT NULL,
    delete_message text DEFAULT ''::text NOT NULL,
    delete_started_at timestamp with time zone,
    delete_finished_at timestamp with time zone,
    source_type text DEFAULT 'repository'::text NOT NULL,
    repository_binding_id text DEFAULT ''::text NOT NULL,
    dockerfile_path text DEFAULT 'Dockerfile'::text NOT NULL,
    build_context text DEFAULT '.'::text NOT NULL,
    build_directory text DEFAULT ''::text NOT NULL,
    build_args text DEFAULT ''::text NOT NULL,
    build_environment_id text DEFAULT ''::text NOT NULL,
    build_cpu_request text DEFAULT '1'::text NOT NULL,
    build_memory_request text DEFAULT '1Gi'::text NOT NULL,
    target_registry_id text DEFAULT ''::text NOT NULL,
    target_repository text DEFAULT ''::text NOT NULL,
    target_tag text DEFAULT ''::text NOT NULL,
    image_ref text DEFAULT ''::text NOT NULL,
    build_labels text DEFAULT ''::text NOT NULL,
    build_variable_set_ids text DEFAULT ''::text NOT NULL,
    build_hooks_enabled boolean DEFAULT true NOT NULL,
    auto_deploy boolean DEFAULT false NOT NULL,
    branch_pattern text DEFAULT ''::text NOT NULL,
    tag_pattern text DEFAULT ''::text NOT NULL,
    concurrency_policy text DEFAULT 'queue'::text NOT NULL,
    runtime_config_refs text DEFAULT ''::text NOT NULL,
    env_vars text DEFAULT ''::text NOT NULL,
    config_refs text DEFAULT ''::text NOT NULL,
    secret_refs text DEFAULT ''::text NOT NULL,
    data_retention_enabled boolean DEFAULT false NOT NULL,
    data_capacity text DEFAULT ''::text NOT NULL,
    data_mount_path text DEFAULT '/data'::text NOT NULL,
    data_volumes text DEFAULT ''::text NOT NULL,
    require_approval boolean DEFAULT false NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    created_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    build_timeout_seconds integer DEFAULT 1800 NOT NULL,
    cpu_limit text DEFAULT ''::text NOT NULL,
    memory_limit text DEFAULT ''::text NOT NULL,
    image_pull_policy text DEFAULT ''::text NOT NULL,
    container_command text DEFAULT ''::text NOT NULL,
    container_args text DEFAULT ''::text NOT NULL,
    readiness_probe text DEFAULT ''::text NOT NULL,
    liveness_probe text DEFAULT ''::text NOT NULL,
    startup_probe text DEFAULT ''::text NOT NULL,
    run_as_user text DEFAULT ''::text NOT NULL,
    run_as_group text DEFAULT ''::text NOT NULL,
    fs_group text DEFAULT ''::text NOT NULL,
    fs_group_change_policy text DEFAULT ''::text NOT NULL,
    read_only_root_filesystem boolean DEFAULT false NOT NULL,
    allow_privilege_escalation text DEFAULT ''::text NOT NULL,
    capability_add text DEFAULT ''::text NOT NULL,
    capability_drop text DEFAULT ''::text NOT NULL,
    node_selector text DEFAULT ''::text NOT NULL,
    tolerations text DEFAULT ''::text NOT NULL,
    affinity text DEFAULT ''::text NOT NULL,
    topology_spread_constraints text DEFAULT ''::text NOT NULL,
    priority_class_name text DEFAULT ''::text NOT NULL,
    service_type text DEFAULT ''::text NOT NULL,
    service_annotations text DEFAULT ''::text NOT NULL,
    service_external_traffic_policy text DEFAULT ''::text NOT NULL,
    service_session_affinity text DEFAULT ''::text NOT NULL,
    data_storage_class_name text DEFAULT ''::text NOT NULL,
    data_access_mode text DEFAULT ''::text NOT NULL,
    lifecycle text DEFAULT ''::text NOT NULL,
    init_containers text DEFAULT ''::text NOT NULL,
    sidecar_containers text DEFAULT ''::text NOT NULL,
    auto_scaling_enabled boolean DEFAULT false NOT NULL,
    auto_scaling_min_replicas integer DEFAULT 1 NOT NULL,
    auto_scaling_max_replicas integer DEFAULT 1 NOT NULL,
    auto_scaling_cpu_percent integer DEFAULT 0 NOT NULL,
    auto_scaling_memory_percent integer DEFAULT 0 NOT NULL,
    data_volume_mode text DEFAULT ''::text NOT NULL,
    workload_type text DEFAULT 'Deployment'::text NOT NULL,
    auto_scaling_behavior text DEFAULT ''::text NOT NULL,
    service_account_name text DEFAULT ''::text NOT NULL,
    automount_service_account_token text DEFAULT ''::text NOT NULL,
    web_console_enabled boolean,
    build_definition_mode text DEFAULT 'repository_dockerfile'::text NOT NULL,
    build_template_id text DEFAULT ''::text NOT NULL,
    build_template_version text DEFAULT ''::text NOT NULL,
    build_template_values text DEFAULT '{}'::text NOT NULL,
    kubernetes_name text DEFAULT ''::text NOT NULL,
    service_ports text DEFAULT ''::text NOT NULL,
    config_files text DEFAULT ''::text NOT NULL,
    secret_files text DEFAULT ''::text NOT NULL
);

CREATE TABLE deployment_volume_mounts (
    id text NOT NULL,
    project_id text NOT NULL,
    application_id text NOT NULL,
    deployment_target_id text NOT NULL,
    source_type text NOT NULL,
    project_volume_id text,
    logical_name text NOT NULL,
    mount_path text,
    device_path text,
    read_only boolean DEFAULT false NOT NULL,
    exclusive boolean DEFAULT false NOT NULL,
    activation_state text DEFAULT 'reserved'::text NOT NULL,
    empty_dir_medium text DEFAULT ''::text NOT NULL,
    empty_dir_size_limit text DEFAULT ''::text NOT NULL,
    last_error_code text DEFAULT ''::text NOT NULL,
    last_error_message text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    CONSTRAINT chk_deployment_volume_mounts_activation_state CHECK ((activation_state = ANY (ARRAY['reserved'::text, 'active'::text, 'release_pending'::text, 'error'::text]))),
    CONSTRAINT chk_deployment_volume_mounts_source_fields CHECK ((((source_type = 'project_volume'::text) AND (project_volume_id IS NOT NULL) AND (((mount_path IS NOT NULL) AND (device_path IS NULL)) OR ((mount_path IS NULL) AND (device_path IS NOT NULL))) AND (empty_dir_medium = ''::text) AND (empty_dir_size_limit = ''::text)) OR ((source_type = 'empty_dir'::text) AND (project_volume_id IS NULL) AND (mount_path IS NOT NULL) AND (device_path IS NULL)))),
    CONSTRAINT chk_deployment_volume_mounts_source_type CHECK ((source_type = ANY (ARRAY['project_volume'::text, 'empty_dir'::text])))
);

CREATE TABLE email_registration_challenges (
    id text NOT NULL,
    email text NOT NULL,
    code_hash text NOT NULL,
    language text DEFAULT 'zh-CN'::text NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE external_identities (
    id text NOT NULL,
    user_id text NOT NULL,
    provider_id text NOT NULL,
    subject text NOT NULL,
    email text DEFAULT ''::text NOT NULL,
    email_verified boolean DEFAULT false NOT NULL,
    username text DEFAULT ''::text NOT NULL,
    last_login_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE gateway_routes (
    id text NOT NULL,
    project_id text NOT NULL,
    application_id text NOT NULL,
    environment_id text DEFAULT ''::text NOT NULL,
    deployment_target_id text DEFAULT ''::text NOT NULL,
    host text NOT NULL,
    path text DEFAULT '/'::text NOT NULL,
    service_port integer DEFAULT 80 NOT NULL,
    tls_mode text DEFAULT 'http-only'::text NOT NULL,
    cname_name text DEFAULT ''::text NOT NULL,
    cname_target text DEFAULT ''::text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    delete_status text DEFAULT 'active'::text NOT NULL,
    delete_message text DEFAULT ''::text NOT NULL,
    delete_started_at timestamp with time zone,
    delete_finished_at timestamp with time zone,
    is_default boolean DEFAULT false NOT NULL,
    created_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    request_headers text DEFAULT ''::text NOT NULL,
    response_headers text DEFAULT ''::text NOT NULL,
    parent_gateway_name text DEFAULT ''::text NOT NULL,
    parent_gateway_namespace text DEFAULT ''::text NOT NULL,
    section_name text DEFAULT ''::text NOT NULL,
    path_match_type text DEFAULT 'PathPrefix'::text NOT NULL,
    url_rewrite text DEFAULT ''::text NOT NULL,
    request_redirect text DEFAULT ''::text NOT NULL,
    backend_weight bigint DEFAULT 1 NOT NULL,
    hostname_aliases text DEFAULT ''::text NOT NULL,
    domain_suffix text DEFAULT ''::text NOT NULL
);

CREATE TABLE git_accounts (
    id text NOT NULL,
    user_id text NOT NULL,
    provider_id text NOT NULL,
    scope text DEFAULT 'user'::text NOT NULL,
    owner_ref text DEFAULT ''::text NOT NULL,
    external_user_id text DEFAULT ''::text NOT NULL,
    username text NOT NULL,
    avatar_url text DEFAULT ''::text NOT NULL,
    access_token_ref text DEFAULT ''::text NOT NULL,
    refresh_token_ref text DEFAULT ''::text NOT NULL,
    scopes text DEFAULT ''::text NOT NULL,
    expires_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);

CREATE TABLE git_providers (
    id text NOT NULL,
    type text NOT NULL,
    name text NOT NULL,
    base_url text DEFAULT ''::text NOT NULL,
    scope text DEFAULT 'user'::text NOT NULL,
    owner_ref text DEFAULT ''::text NOT NULL,
    auth_type text DEFAULT 'oauth'::text NOT NULL,
    client_id text DEFAULT ''::text NOT NULL,
    client_secret_ref text DEFAULT ''::text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);

CREATE TABLE hook_run_logs (
    id text NOT NULL,
    hook_run_id text NOT NULL,
    project_id text NOT NULL,
    content text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE hook_runs (
    id text NOT NULL,
    project_id text NOT NULL,
    hook_config_id text DEFAULT ''::text NOT NULL,
    build_run_id text DEFAULT ''::text NOT NULL,
    build_job_id text DEFAULT ''::text NOT NULL,
    release_id text DEFAULT ''::text NOT NULL,
    application_id text DEFAULT ''::text NOT NULL,
    environment_id text DEFAULT ''::text NOT NULL,
    deployment_target_id text DEFAULT ''::text NOT NULL,
    name text NOT NULL,
    phase text NOT NULL,
    status text DEFAULT 'queued'::text NOT NULL,
    script_snapshot text NOT NULL,
    shell text DEFAULT 'sh'::text NOT NULL,
    image_ref text DEFAULT ''::text NOT NULL,
    timeout_seconds integer DEFAULT 300 NOT NULL,
    failure_policy text DEFAULT 'fail'::text NOT NULL,
    exit_code integer DEFAULT 0 NOT NULL,
    message text DEFAULT ''::text NOT NULL,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE inbox_action_requests (
    id text NOT NULL,
    type text NOT NULL,
    requester_user_id text NOT NULL,
    recipient_user_id text NOT NULL,
    project_id text DEFAULT ''::text NOT NULL,
    resource_type text DEFAULT ''::text NOT NULL,
    resource_id text DEFAULT ''::text NOT NULL,
    payload_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    row_version bigint DEFAULT 1 NOT NULL,
    expires_at timestamp with time zone,
    responded_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT chk_inbox_action_requests_row_version CHECK ((row_version > 0)),
    CONSTRAINT chk_inbox_action_requests_status CHECK ((status = ANY (ARRAY['pending'::text, 'processing'::text, 'completed'::text, 'rejected'::text, 'cancelled'::text, 'expired'::text, 'failed'::text])))
);

CREATE TABLE inbox_messages (
    id text NOT NULL,
    recipient_user_id text NOT NULL,
    type text NOT NULL,
    category text NOT NULL,
    priority text NOT NULL,
    actor_id text DEFAULT ''::text NOT NULL,
    project_id text DEFAULT ''::text NOT NULL,
    resource_type text DEFAULT ''::text NOT NULL,
    resource_id text DEFAULT ''::text NOT NULL,
    title_key text NOT NULL,
    content_key text NOT NULL,
    params_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    action_request_id text DEFAULT ''::text NOT NULL,
    deep_link text DEFAULT ''::text NOT NULL,
    group_key text DEFAULT ''::text NOT NULL,
    dedup_key text,
    read_at timestamp with time zone,
    archived_at timestamp with time zone,
    expires_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT chk_inbox_messages_category CHECK ((category = ANY (ARRAY['action'::text, 'project'::text, 'billing'::text, 'security'::text, 'delivery'::text, 'system'::text]))),
    CONSTRAINT chk_inbox_messages_priority CHECK ((priority = ANY (ARRAY['low'::text, 'normal'::text, 'high'::text, 'critical'::text])))
);

CREATE TABLE notification_channels (
    id text NOT NULL,
    project_id text DEFAULT ''::text NOT NULL,
    name text NOT NULL,
    adapter_kind text NOT NULL,
    config_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    secret_refs_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    created_by text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone
);

CREATE TABLE notification_deliveries (
    id text NOT NULL,
    project_id text DEFAULT ''::text NOT NULL,
    event_id text NOT NULL,
    event_type text NOT NULL,
    severity text DEFAULT ''::text NOT NULL,
    channel_id text NOT NULL,
    adapter_kind text NOT NULL,
    rule_id text DEFAULT ''::text NOT NULL,
    template_id text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    attempt_count bigint DEFAULT 0 NOT NULL,
    duration_millis bigint DEFAULT 0 NOT NULL,
    error_message text DEFAULT ''::text NOT NULL,
    request_snapshot jsonb DEFAULT '{}'::jsonb NOT NULL,
    response_snippet text DEFAULT ''::text NOT NULL,
    queued_at timestamp with time zone,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    event_json jsonb DEFAULT '{}'::jsonb NOT NULL
);

CREATE TABLE notification_rules (
    id text NOT NULL,
    project_id text DEFAULT ''::text NOT NULL,
    name text NOT NULL,
    event_types_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    filter_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    channel_ids_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    template_id text DEFAULT ''::text NOT NULL,
    locale text DEFAULT ''::text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    created_by text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone
);

CREATE TABLE notification_templates (
    id text NOT NULL,
    project_id text DEFAULT ''::text NOT NULL,
    name text NOT NULL,
    event_type text NOT NULL,
    adapter_kind text NOT NULL,
    locale text DEFAULT ''::text NOT NULL,
    subject_template text DEFAULT ''::text NOT NULL,
    body_template text DEFAULT ''::text NOT NULL,
    json_body_template text DEFAULT ''::text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    created_by text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone
);

CREATE TABLE oauth_applications (
    id text NOT NULL,
    owner_user_id text,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    homepage_url text DEFAULT ''::text NOT NULL,
    logo_url text DEFAULT ''::text NOT NULL,
    client_id text NOT NULL,
    client_secret_hash text NOT NULL,
    redirect_uris text NOT NULL,
    allowed_scopes text NOT NULL,
    access_token_lifetime_days integer DEFAULT 30 NOT NULL,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE oauth_authorization_codes (
    id text NOT NULL,
    application_id text NOT NULL,
    grant_id text NOT NULL,
    user_id text NOT NULL,
    code_hash text NOT NULL,
    redirect_uri text NOT NULL,
    scope text NOT NULL,
    code_challenge text NOT NULL,
    code_challenge_method text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE oauth_device_authorizations (
    id text NOT NULL,
    application_id text NOT NULL,
    grant_id text,
    user_id text,
    device_code_hash text NOT NULL,
    user_code_hash text NOT NULL,
    status text NOT NULL,
    interval_seconds integer NOT NULL,
    last_polled_at timestamp with time zone,
    expires_at timestamp with time zone NOT NULL,
    approved_at timestamp with time zone,
    denied_at timestamp with time zone,
    consumed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE oauth_grants (
    id text NOT NULL,
    application_id text NOT NULL,
    user_id text NOT NULL,
    scope text NOT NULL,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE oauth_refresh_tokens (
    id text NOT NULL,
    application_id text NOT NULL,
    grant_id text NOT NULL,
    user_id text NOT NULL,
    token_hash text NOT NULL,
    scope text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE platform_events (
    id text NOT NULL,
    type text NOT NULL,
    category text NOT NULL,
    severity text NOT NULL,
    status text NOT NULL,
    project_id text DEFAULT ''::text NOT NULL,
    application_id text DEFAULT ''::text NOT NULL,
    deployment_target_id text DEFAULT ''::text NOT NULL,
    resource_type text DEFAULT ''::text NOT NULL,
    resource_id text DEFAULT ''::text NOT NULL,
    actor_id text DEFAULT ''::text NOT NULL,
    summary_key text DEFAULT ''::text NOT NULL,
    message text DEFAULT ''::text NOT NULL,
    detail_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    links_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    correlation_id text DEFAULT ''::text NOT NULL,
    trace_id text DEFAULT ''::text NOT NULL,
    dedup_key text,
    occurred_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE project_hook_configs (
    id text NOT NULL,
    project_id text NOT NULL,
    name text NOT NULL,
    script text NOT NULL,
    shell text DEFAULT 'sh'::text NOT NULL,
    timeout_seconds integer DEFAULT 300 NOT NULL,
    failure_policy text DEFAULT 'fail'::text NOT NULL,
    created_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);

CREATE TABLE project_members (
    id text NOT NULL,
    project_id text NOT NULL,
    user_id text NOT NULL,
    role text NOT NULL,
    dashboard_order integer DEFAULT 0 NOT NULL,
    last_used_at timestamp with time zone,
    use_count integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE project_pins (
    id text NOT NULL,
    user_id text NOT NULL,
    project_id text NOT NULL,
    pinned_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE project_runtime_config_sets (
    id text NOT NULL,
    project_id text NOT NULL,
    name text NOT NULL,
    env_vars text DEFAULT ''::text NOT NULL,
    config_files text DEFAULT ''::text NOT NULL,
    secret_refs text DEFAULT ''::text NOT NULL,
    secret_files text DEFAULT ''::text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    delete_status text DEFAULT 'active'::text NOT NULL,
    delete_message text DEFAULT ''::text NOT NULL,
    delete_started_at timestamp with time zone,
    delete_finished_at timestamp with time zone,
    created_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);

CREATE TABLE project_topology_edges (
    id text NOT NULL,
    project_id text NOT NULL,
    source_application_id text NOT NULL,
    source_deployment_target_id text DEFAULT ''::text NOT NULL,
    target_application_id text NOT NULL,
    target_deployment_target_id text DEFAULT ''::text NOT NULL,
    relation_type text NOT NULL,
    protocol text DEFAULT ''::text NOT NULL,
    port integer DEFAULT 0 NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    created_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT project_topology_edges_check CHECK ((source_application_id <> target_application_id)),
    CONSTRAINT project_topology_edges_port_check CHECK (((port >= 0) AND (port <= 65535))),
    CONSTRAINT project_topology_edges_protocol_check CHECK ((protocol = ANY (ARRAY[''::text, 'http'::text, 'https'::text, 'tcp'::text]))),
    CONSTRAINT project_topology_edges_relation_type_check CHECK ((relation_type = ANY (ARRAY['depends_on'::text, 'calls'::text, 'reads_writes'::text, 'publishes_to'::text, 'consumes_from'::text])))
);

CREATE TABLE project_volumes (
    id text NOT NULL,
    project_id text NOT NULL,
    display_name text NOT NULL,
    cluster_id text NOT NULL,
    namespace text NOT NULL,
    claim_name text NOT NULL,
    ownership_mode text NOT NULL,
    source_kind text NOT NULL,
    source_snapshot_name text DEFAULT ''::text NOT NULL,
    lifecycle_state text DEFAULT 'provisioning'::text NOT NULL,
    pending_operation text DEFAULT 'provision'::text NOT NULL,
    capacity_request text NOT NULL,
    capacity_bytes bigint NOT NULL,
    storage_class_name text DEFAULT ''::text NOT NULL,
    access_mode text NOT NULL,
    volume_mode text NOT NULL,
    source_application_id text,
    source_application_name text DEFAULT ''::text NOT NULL,
    source_deployment_target_id text,
    created_by text NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    idempotency_key_hash text DEFAULT ''::text NOT NULL,
    idempotency_request_hash text DEFAULT ''::text NOT NULL,
    last_error_code text DEFAULT ''::text NOT NULL,
    last_error_message text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    CONSTRAINT chk_project_volumes_access_mode CHECK ((access_mode = ANY (ARRAY['ReadWriteOnce'::text, 'ReadWriteOncePod'::text, 'ReadOnlyMany'::text, 'ReadWriteMany'::text]))),
    CONSTRAINT chk_project_volumes_capacity CHECK ((capacity_bytes > 0)),
    CONSTRAINT chk_project_volumes_idempotency_pair CHECK (((idempotency_key_hash = ''::text) = (idempotency_request_hash = ''::text))),
    CONSTRAINT chk_project_volumes_lifecycle_state CHECK ((lifecycle_state = ANY (ARRAY['provisioning'::text, 'ready'::text, 'deleting'::text, 'error'::text]))),
    CONSTRAINT chk_project_volumes_ownership_mode CHECK ((ownership_mode = ANY (ARRAY['managed'::text, 'referenced'::text]))),
    CONSTRAINT chk_project_volumes_pending_operation CHECK ((pending_operation = ANY (ARRAY[''::text, 'provision'::text, 'expand'::text, 'delete'::text, 'import'::text]))),
    CONSTRAINT chk_project_volumes_referenced_source CHECK (((ownership_mode <> 'referenced'::text) OR (source_kind = 'existing_claim'::text))),
    CONSTRAINT chk_project_volumes_revision CHECK ((revision > 0)),
    CONSTRAINT chk_project_volumes_snapshot_source CHECK ((((source_kind = 'snapshot_restore'::text) AND (source_snapshot_name <> ''::text)) OR ((source_kind <> 'snapshot_restore'::text) AND (source_snapshot_name = ''::text)))),
    CONSTRAINT chk_project_volumes_source_kind CHECK ((source_kind = ANY (ARRAY['blank'::text, 'managed'::text, 'retained'::text, 'archive_import'::text, 'snapshot_restore'::text, 'existing_claim'::text]))),
    CONSTRAINT chk_project_volumes_volume_mode CHECK ((volume_mode = ANY (ARRAY['Filesystem'::text, 'Block'::text])))
);

CREATE TABLE projects (
    id text NOT NULL,
    identifier text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    namespace_strategy text DEFAULT 'project'::text NOT NULL,
    max_concurrent_builds integer DEFAULT 2 NOT NULL,
    delete_status text DEFAULT 'active'::text NOT NULL,
    delete_message text DEFAULT ''::text NOT NULL,
    delete_started_at timestamp with time zone,
    delete_finished_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    billing_owner_user_id text DEFAULT ''::text NOT NULL,
    system_key text DEFAULT ''::text NOT NULL,
    web_console_enabled boolean DEFAULT true NOT NULL,
    kubernetes_namespace text DEFAULT ''::text NOT NULL
);

CREATE TABLE registry_credentials (
    id text NOT NULL,
    registry_id text NOT NULL,
    name text NOT NULL,
    username text DEFAULT ''::text NOT NULL,
    password_ref text DEFAULT ''::text NOT NULL,
    token_ref text DEFAULT ''::text NOT NULL,
    usage text DEFAULT 'push-pull'::text NOT NULL,
    created_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    repository_template text DEFAULT ''::text NOT NULL,
    tag_template text DEFAULT ''::text NOT NULL,
    scope text DEFAULT 'user'::text NOT NULL,
    owner_ref text DEFAULT ''::text NOT NULL
);

CREATE TABLE release_logs (
    id text NOT NULL,
    release_id text NOT NULL,
    project_id text NOT NULL,
    content text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE releases (
    id text NOT NULL,
    project_id text NOT NULL,
    application_id text NOT NULL,
    environment_id text NOT NULL,
    deployment_target_id text DEFAULT ''::text NOT NULL,
    build_run_id text DEFAULT ''::text NOT NULL,
    image_ref text NOT NULL,
    force_image_pull boolean DEFAULT false NOT NULL,
    type text DEFAULT 'deploy'::text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    revision integer DEFAULT 1 NOT NULL,
    rollback_from_id text DEFAULT ''::text NOT NULL,
    message text DEFAULT ''::text NOT NULL,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    created_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);

CREATE TABLE repository_bindings (
    id text NOT NULL,
    project_id text NOT NULL,
    application_id text NOT NULL,
    git_provider_id text NOT NULL,
    git_account_id text NOT NULL,
    owner text NOT NULL,
    repo text NOT NULL,
    clone_url text DEFAULT ''::text NOT NULL,
    default_branch text DEFAULT 'main'::text NOT NULL,
    webhook_id text DEFAULT ''::text NOT NULL,
    webhook_secret text DEFAULT ''::text NOT NULL,
    credential_ref text DEFAULT ''::text NOT NULL,
    last_event text DEFAULT ''::text NOT NULL,
    last_commit_sha text DEFAULT ''::text NOT NULL,
    last_webhook_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    webhook_enabled boolean DEFAULT true NOT NULL
);

CREATE TABLE retained_volumes (
    id text NOT NULL,
    project_id text NOT NULL,
    source_application_id text NOT NULL,
    source_application_name text DEFAULT ''::text NOT NULL,
    source_deployment_target_id text NOT NULL,
    cluster_id text NOT NULL,
    namespace text NOT NULL,
    claim_name text NOT NULL,
    volume_name text DEFAULT 'data'::text NOT NULL,
    mount_path text DEFAULT '/data'::text NOT NULL,
    capacity text DEFAULT ''::text NOT NULL,
    storage_class_name text DEFAULT ''::text NOT NULL,
    access_mode text DEFAULT ''::text NOT NULL,
    volume_mode text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'retained'::text NOT NULL,
    claimed_by_application_id text DEFAULT ''::text NOT NULL,
    claimed_by_target_id text DEFAULT ''::text NOT NULL,
    last_error text DEFAULT ''::text NOT NULL,
    retained_at timestamp with time zone NOT NULL,
    claimed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT retained_volumes_status_check CHECK ((status = ANY (ARRAY['retaining'::text, 'retained'::text, 'reserved'::text, 'claimed'::text, 'deleting'::text, 'delete_failed'::text])))
);

CREATE TABLE runtime_clusters (
    id text NOT NULL,
    name text NOT NULL,
    type text DEFAULT 'kubernetes'::text NOT NULL,
    endpoint text DEFAULT ''::text NOT NULL,
    scope text DEFAULT 'global'::text NOT NULL,
    owner_ref text DEFAULT ''::text NOT NULL,
    kubeconfig_ref text DEFAULT ''::text NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    max_concurrent_builds integer DEFAULT 4 NOT NULL,
    gateway_root_domain text DEFAULT 'apps.local'::text NOT NULL,
    gateway_public_scheme text DEFAULT 'http'::text NOT NULL,
    created_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    gateway_controller_type text DEFAULT 'traefik'::text NOT NULL,
    gateway_external_tls_mode text DEFAULT 'none'::text NOT NULL,
    gateway_forwarded_headers_mode text DEFAULT 'preserve'::text NOT NULL,
    gateway_trusted_proxy_cidrs text DEFAULT ''::text NOT NULL,
    gateway_default_request_headers text DEFAULT ''::text NOT NULL,
    gateway_default_response_headers text DEFAULT ''::text NOT NULL,
    gateway_provider text DEFAULT 'gateway-api'::text NOT NULL,
    gateway_class_name text DEFAULT 'traefik'::text NOT NULL,
    gateway_name text DEFAULT 'luna-gateway'::text NOT NULL,
    gateway_namespace text DEFAULT 'kube-system'::text NOT NULL,
    gateway_public_port bigint DEFAULT 80 NOT NULL,
    gateway_http_listener_name text DEFAULT 'web'::text NOT NULL,
    gateway_http_listener_port bigint DEFAULT 8080 NOT NULL,
    gateway_https_listener_name text DEFAULT 'websecure'::text NOT NULL,
    gateway_https_listener_port bigint DEFAULT 8443 NOT NULL,
    gateway_domain_suffixes text DEFAULT ''::text NOT NULL,
    gateway_tls_secret_name text DEFAULT ''::text NOT NULL,
    gateway_tls_secret_namespace text DEFAULT ''::text NOT NULL,
    gateway_cert_issuer_kind text DEFAULT 'ClusterIssuer'::text NOT NULL,
    gateway_cert_issuer_name text DEFAULT ''::text NOT NULL,
    gateway_certificate_namespace text DEFAULT ''::text NOT NULL,
    gateway_wildcard_cert_enabled boolean DEFAULT false NOT NULL,
    gateway_wildcard_cert_domain text DEFAULT ''::text NOT NULL,
    gateway_wildcard_cert_secret_name text DEFAULT ''::text NOT NULL
);

CREATE TABLE scoped_resource_project_bindings (
    id text NOT NULL,
    resource_type text NOT NULL,
    resource_id text NOT NULL,
    project_id text NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE secret_values (
    id text NOT NULL,
    cipher_ref text NOT NULL,
    created_by text DEFAULT ''::text NOT NULL,
    resource text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE service_bindings (
    id text NOT NULL,
    project_id text NOT NULL,
    source_application_id text NOT NULL,
    source_deployment_target_id text NOT NULL,
    target_application_id text NOT NULL,
    target_deployment_target_id text NOT NULL,
    target_port_name text NOT NULL,
    target_port integer NOT NULL,
    protocol text NOT NULL,
    path text DEFAULT ''::text NOT NULL,
    injection_mode text NOT NULL,
    url_env_var text DEFAULT ''::text NOT NULL,
    host_env_var text DEFAULT ''::text NOT NULL,
    port_env_var text DEFAULT ''::text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    created_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    secret_map text DEFAULT ''::text NOT NULL,
    CONSTRAINT service_bindings_check CHECK ((source_deployment_target_id <> target_deployment_target_id)),
    CONSTRAINT service_bindings_check1 CHECK ((((injection_mode = 'url'::text) AND (url_env_var <> ''::text) AND (host_env_var = ''::text) AND (port_env_var = ''::text)) OR ((injection_mode = 'host_port'::text) AND (url_env_var = ''::text) AND (host_env_var <> ''::text) AND (port_env_var <> ''::text)))),
    CONSTRAINT service_bindings_injection_mode_check CHECK ((injection_mode = ANY (ARRAY['url'::text, 'host_port'::text]))),
    CONSTRAINT service_bindings_protocol_check CHECK ((protocol = ANY (ARRAY['http'::text, 'https'::text, 'tcp'::text]))),
    CONSTRAINT service_bindings_target_port_check CHECK (((target_port >= 1) AND (target_port <= 65535)))
);

CREATE TABLE system_component_installations (
    id text NOT NULL,
    component_id text NOT NULL,
    component_version text DEFAULT ''::text NOT NULL,
    runtime_cluster_id text NOT NULL,
    namespace text DEFAULT 'luna-system'::text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    message text DEFAULT ''::text NOT NULL,
    controller_type text DEFAULT ''::text NOT NULL,
    mode text DEFAULT ''::text NOT NULL,
    config text DEFAULT '{}'::text NOT NULL,
    report_token_hash text DEFAULT ''::text NOT NULL,
    last_error text DEFAULT ''::text NOT NULL,
    installed_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    project_id text DEFAULT ''::text NOT NULL,
    application_id text DEFAULT ''::text NOT NULL,
    deployment_target_id text DEFAULT ''::text NOT NULL,
    release_id text DEFAULT ''::text NOT NULL
);

CREATE TABLE user_remember_tokens (
    id text NOT NULL,
    user_id text NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    family_id text NOT NULL,
    consumed_at timestamp with time zone,
    revoked_at timestamp with time zone
);

CREATE TABLE user_sessions (
    id text NOT NULL,
    user_id text NOT NULL,
    impersonator_id text DEFAULT ''::text NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    remember_family_id text DEFAULT ''::text NOT NULL,
    primary_authenticated_at timestamp with time zone
);

CREATE TABLE user_wallets (
    id text NOT NULL,
    user_id text NOT NULL,
    balance_credits numeric(24,8) DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE users (
    id text NOT NULL,
    email text NOT NULL,
    name text NOT NULL,
    avatar_url text DEFAULT ''::text NOT NULL,
    role text DEFAULT 'user'::text NOT NULL,
    language text DEFAULT 'zh-CN'::text NOT NULL,
    password text DEFAULT ''::text NOT NULL,
    disabled boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    brand_color_preset text DEFAULT ''::text NOT NULL,
    interface_style text DEFAULT ''::text NOT NULL
);

CREATE TABLE volume_transfer_parts (
    transfer_id text NOT NULL,
    part_number integer NOT NULL,
    byte_offset bigint NOT NULL,
    size bigint NOT NULL,
    etag text NOT NULL,
    sha256 text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chk_volume_transfer_parts_number CHECK ((part_number > 0)),
    CONSTRAINT chk_volume_transfer_parts_offset CHECK ((byte_offset >= 0)),
    CONSTRAINT chk_volume_transfer_parts_sha256 CHECK ((sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT chk_volume_transfer_parts_size CHECK ((size > 0))
);

CREATE TABLE volume_transfers (
    id text NOT NULL,
    project_id text NOT NULL,
    project_volume_id text NOT NULL,
    direction text NOT NULL,
    format text NOT NULL,
    consistency_mode text NOT NULL,
    state text DEFAULT 'created'::text NOT NULL,
    object_key text NOT NULL,
    multipart_upload_id text DEFAULT ''::text NOT NULL,
    source_filename text DEFAULT ''::text NOT NULL,
    expected_bytes bigint DEFAULT 0 NOT NULL,
    transferred_bytes bigint DEFAULT 0 NOT NULL,
    processed_files bigint DEFAULT 0 NOT NULL,
    phase text DEFAULT ''::text NOT NULL,
    sha256 text DEFAULT ''::text NOT NULL,
    actor_id text NOT NULL,
    callback_token_hash text DEFAULT ''::text NOT NULL,
    callback_token_expires_at timestamp with time zone,
    expires_at timestamp with time zone NOT NULL,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    object_deleted_at timestamp with time zone,
    last_error_code text DEFAULT ''::text NOT NULL,
    last_error_message text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chk_volume_transfers_consistency_mode CHECK ((consistency_mode = ANY (ARRAY['snapshot'::text, 'live'::text, 'unmounted'::text]))),
    CONSTRAINT chk_volume_transfers_direction CHECK ((direction = ANY (ARRAY['import'::text, 'export'::text]))),
    CONSTRAINT chk_volume_transfers_expected_bytes CHECK ((expected_bytes >= 0)),
    CONSTRAINT chk_volume_transfers_format CHECK ((format = ANY (ARRAY['tar_gz'::text, 'raw_zst'::text]))),
    CONSTRAINT chk_volume_transfers_processed_files CHECK ((processed_files >= 0)),
    CONSTRAINT chk_volume_transfers_sha256 CHECK (((sha256 = ''::text) OR (sha256 ~ '^[0-9a-f]{64}$'::text))),
    CONSTRAINT chk_volume_transfers_state CHECK ((state = ANY (ARRAY['created'::text, 'uploading'::text, 'queued'::text, 'running'::text, 'succeeded'::text, 'failed'::text, 'cancelled'::text, 'expired'::text]))),
    CONSTRAINT chk_volume_transfers_transferred_bytes CHECK ((transferred_bytes >= 0))
);

ALTER TABLE ONLY ai.conversation_summaries
    ADD CONSTRAINT conversation_summaries_pkey PRIMARY KEY (conversation_id);

ALTER TABLE ONLY ai.conversations
    ADD CONSTRAINT conversations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY ai.idempotency_keys
    ADD CONSTRAINT idempotency_keys_pkey PRIMARY KEY (owner_user_id, idempotency_key);

ALTER TABLE ONLY ai.items
    ADD CONSTRAINT items_pkey PRIMARY KEY (id);

ALTER TABLE ONLY ai.items
    ADD CONSTRAINT items_run_id_timeline_index_key UNIQUE (run_id, timeline_index);

ALTER TABLE ONLY ai.run_events
    ADD CONSTRAINT run_events_pkey PRIMARY KEY (id);

ALTER TABLE ONLY ai.run_events
    ADD CONSTRAINT run_events_run_id_event_sequence_key UNIQUE (run_id, event_sequence);

ALTER TABLE ONLY ai.runs
    ADD CONSTRAINT runs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY ai.runs
    ADD CONSTRAINT runs_turn_id_run_index_key UNIQUE (turn_id, run_index);

ALTER TABLE ONLY ai.tool_calls
    ADD CONSTRAINT tool_calls_pkey PRIMARY KEY (id);

ALTER TABLE ONLY ai.turns
    ADD CONSTRAINT turns_conversation_id_turn_index_key UNIQUE (conversation_id, turn_index);

ALTER TABLE ONLY ai.turns
    ADD CONSTRAINT turns_pkey PRIMARY KEY (id);

ALTER TABLE ONLY access_tokens
    ADD CONSTRAINT access_tokens_pkey PRIMARY KEY (id);

ALTER TABLE ONLY app_configs
    ADD CONSTRAINT app_configs_pkey PRIMARY KEY (key);

ALTER TABLE ONLY app_template_installations
    ADD CONSTRAINT app_template_installations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY applications
    ADD CONSTRAINT applications_pkey PRIMARY KEY (id);

ALTER TABLE ONLY artifact_registries
    ADD CONSTRAINT artifact_registries_pkey PRIMARY KEY (id);

ALTER TABLE ONLY audit_logs
    ADD CONSTRAINT audit_logs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY auth_admission_policies
    ADD CONSTRAINT auth_admission_policies_pkey PRIMARY KEY (id);

ALTER TABLE ONLY auth_providers
    ADD CONSTRAINT auth_providers_pkey PRIMARY KEY (id);

ALTER TABLE ONLY auth_registration_settings
    ADD CONSTRAINT auth_registration_settings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY billing_ledger_entries
    ADD CONSTRAINT billing_ledger_entries_pkey PRIMARY KEY (id);

ALTER TABLE ONLY billing_rate_rules
    ADD CONSTRAINT billing_rate_rules_pkey PRIMARY KEY (id);

ALTER TABLE ONLY billing_usage_records
    ADD CONSTRAINT billing_usage_records_pkey PRIMARY KEY (id);

ALTER TABLE ONLY build_environment_configs
    ADD CONSTRAINT build_environment_configs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY build_jobs
    ADD CONSTRAINT build_jobs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY build_logs
    ADD CONSTRAINT build_logs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY build_runs
    ADD CONSTRAINT build_runs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY build_variable_sets
    ADD CONSTRAINT build_variable_sets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY container_images
    ADD CONSTRAINT container_images_pkey PRIMARY KEY (id);

ALTER TABLE ONLY deployment_target_hook_bindings
    ADD CONSTRAINT deployment_target_hook_bindings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY deployment_targets
    ADD CONSTRAINT deployment_targets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY deployment_volume_mounts
    ADD CONSTRAINT deployment_volume_mounts_pkey PRIMARY KEY (id);

ALTER TABLE ONLY email_registration_challenges
    ADD CONSTRAINT email_registration_challenges_pkey PRIMARY KEY (id);

ALTER TABLE ONLY external_identities
    ADD CONSTRAINT external_identities_pkey PRIMARY KEY (id);

ALTER TABLE ONLY gateway_routes
    ADD CONSTRAINT gateway_routes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY git_accounts
    ADD CONSTRAINT git_accounts_pkey PRIMARY KEY (id);

ALTER TABLE ONLY git_providers
    ADD CONSTRAINT git_providers_pkey PRIMARY KEY (id);

ALTER TABLE ONLY hook_run_logs
    ADD CONSTRAINT hook_run_logs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY hook_runs
    ADD CONSTRAINT hook_runs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY inbox_action_requests
    ADD CONSTRAINT inbox_action_requests_pkey PRIMARY KEY (id);

ALTER TABLE ONLY inbox_messages
    ADD CONSTRAINT inbox_messages_pkey PRIMARY KEY (id);

ALTER TABLE ONLY notification_channels
    ADD CONSTRAINT notification_channels_pkey PRIMARY KEY (id);

ALTER TABLE ONLY notification_deliveries
    ADD CONSTRAINT notification_deliveries_pkey PRIMARY KEY (id);

ALTER TABLE ONLY notification_rules
    ADD CONSTRAINT notification_rules_pkey PRIMARY KEY (id);

ALTER TABLE ONLY notification_templates
    ADD CONSTRAINT notification_templates_pkey PRIMARY KEY (id);

ALTER TABLE ONLY oauth_applications
    ADD CONSTRAINT oauth_applications_pkey PRIMARY KEY (id);

ALTER TABLE ONLY oauth_authorization_codes
    ADD CONSTRAINT oauth_authorization_codes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY oauth_device_authorizations
    ADD CONSTRAINT oauth_device_authorizations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY oauth_grants
    ADD CONSTRAINT oauth_grants_pkey PRIMARY KEY (id);

ALTER TABLE ONLY oauth_refresh_tokens
    ADD CONSTRAINT oauth_refresh_tokens_pkey PRIMARY KEY (id);

ALTER TABLE ONLY platform_events
    ADD CONSTRAINT platform_events_pkey PRIMARY KEY (id);

ALTER TABLE ONLY project_hook_configs
    ADD CONSTRAINT project_hook_configs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY project_members
    ADD CONSTRAINT project_members_pkey PRIMARY KEY (id);

ALTER TABLE ONLY project_members
    ADD CONSTRAINT project_members_project_id_user_id_key UNIQUE (project_id, user_id);

ALTER TABLE ONLY project_pins
    ADD CONSTRAINT project_pins_pkey PRIMARY KEY (id);

ALTER TABLE ONLY project_runtime_config_sets
    ADD CONSTRAINT project_runtime_config_sets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY project_topology_edges
    ADD CONSTRAINT project_topology_edges_pkey PRIMARY KEY (id);

ALTER TABLE ONLY project_volumes
    ADD CONSTRAINT project_volumes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY projects
    ADD CONSTRAINT projects_pkey PRIMARY KEY (id);

ALTER TABLE ONLY registry_credentials
    ADD CONSTRAINT registry_credentials_pkey PRIMARY KEY (id);

ALTER TABLE ONLY release_logs
    ADD CONSTRAINT release_logs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY releases
    ADD CONSTRAINT releases_pkey PRIMARY KEY (id);

ALTER TABLE ONLY repository_bindings
    ADD CONSTRAINT repository_bindings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY retained_volumes
    ADD CONSTRAINT retained_volumes_claim_unique UNIQUE (cluster_id, namespace, claim_name);

ALTER TABLE ONLY retained_volumes
    ADD CONSTRAINT retained_volumes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY runtime_clusters
    ADD CONSTRAINT runtime_clusters_pkey PRIMARY KEY (id);

ALTER TABLE ONLY scoped_resource_project_bindings
    ADD CONSTRAINT scoped_resource_project_bindi_resource_type_resource_id_pro_key UNIQUE (resource_type, resource_id, project_id);

ALTER TABLE ONLY scoped_resource_project_bindings
    ADD CONSTRAINT scoped_resource_project_bindings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY secret_values
    ADD CONSTRAINT secret_values_pkey PRIMARY KEY (id);

ALTER TABLE ONLY service_bindings
    ADD CONSTRAINT service_bindings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY system_component_installations
    ADD CONSTRAINT system_component_installations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY user_remember_tokens
    ADD CONSTRAINT user_remember_tokens_pkey PRIMARY KEY (id);

ALTER TABLE ONLY user_sessions
    ADD CONSTRAINT user_sessions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY user_wallets
    ADD CONSTRAINT user_wallets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);

ALTER TABLE ONLY volume_transfer_parts
    ADD CONSTRAINT volume_transfer_parts_pkey PRIMARY KEY (transfer_id, part_number);

ALTER TABLE ONLY volume_transfers
    ADD CONSTRAINT volume_transfers_pkey PRIMARY KEY (id);

CREATE INDEX ai_conversation_summaries_updated_idx ON ai.conversation_summaries USING btree (updated_at DESC);

CREATE INDEX ai_conversations_owner_updated_idx ON ai.conversations USING btree (owner_user_id, updated_at DESC);

CREATE INDEX ai_runs_queue_idx ON ai.runs USING btree (status, created_at) WHERE (status = 'queued'::text);

CREATE INDEX ai_tool_calls_run_created_idx ON ai.tool_calls USING btree (run_id, created_at);

CREATE INDEX idx_access_tokens_oauth_application_id ON access_tokens USING btree (oauth_application_id);

CREATE INDEX idx_access_tokens_oauth_grant_id ON access_tokens USING btree (oauth_grant_id);

CREATE INDEX idx_access_tokens_source ON access_tokens USING btree (source);

CREATE UNIQUE INDEX idx_access_tokens_token_hash ON access_tokens USING btree (token_hash);

CREATE INDEX idx_access_tokens_user_id ON access_tokens USING btree (user_id);

CREATE INDEX idx_app_template_installations_application_id ON app_template_installations USING btree (application_id);

CREATE INDEX idx_app_template_installations_created_by ON app_template_installations USING btree (created_by);

CREATE INDEX idx_app_template_installations_deleted_at ON app_template_installations USING btree (deleted_at);

CREATE INDEX idx_app_template_installations_deployment_target_id ON app_template_installations USING btree (deployment_target_id);

CREATE INDEX idx_app_template_installations_project_id ON app_template_installations USING btree (project_id);

CREATE INDEX idx_app_template_installations_release_id ON app_template_installations USING btree (release_id);

CREATE INDEX idx_app_template_installations_status ON app_template_installations USING btree (status);

CREATE INDEX idx_app_template_installations_template_id ON app_template_installations USING btree (template_id);

CREATE INDEX idx_applications_delete_status ON applications USING btree (delete_status);

CREATE INDEX idx_applications_deleted_at ON applications USING btree (deleted_at);

CREATE INDEX idx_applications_identifier ON applications USING btree (identifier);

CREATE INDEX idx_applications_project_id ON applications USING btree (project_id);

CREATE UNIQUE INDEX idx_applications_project_identifier_active ON applications USING btree (project_id, identifier) WHERE (deleted_at IS NULL);

CREATE INDEX idx_artifact_registries_created_by ON artifact_registries USING btree (created_by);

CREATE UNIQUE INDEX idx_artifact_registries_default_global ON artifact_registries USING btree (scope) WHERE ((deleted_at IS NULL) AND (scope = 'global'::text) AND is_default);

CREATE UNIQUE INDEX idx_artifact_registries_default_project ON artifact_registries USING btree (scope, owner_ref) WHERE ((deleted_at IS NULL) AND (scope = 'project'::text) AND is_default);

CREATE UNIQUE INDEX idx_artifact_registries_default_user ON artifact_registries USING btree (scope, owner_ref) WHERE ((deleted_at IS NULL) AND (scope = 'user'::text) AND is_default);

CREATE INDEX idx_artifact_registries_deleted_at ON artifact_registries USING btree (deleted_at);

CREATE INDEX idx_artifact_registries_owner_ref ON artifact_registries USING btree (owner_ref);

CREATE INDEX idx_artifact_registries_scope ON artifact_registries USING btree (scope);

CREATE INDEX idx_artifact_registries_scope_owner ON artifact_registries USING btree (scope, owner_ref) WHERE (deleted_at IS NULL);

CREATE INDEX idx_audit_logs_action ON audit_logs USING btree (action);

CREATE INDEX idx_audit_logs_resource ON audit_logs USING btree (resource);

CREATE INDEX idx_audit_logs_user_id ON audit_logs USING btree (user_id);

CREATE INDEX idx_auth_providers_deleted_at ON auth_providers USING btree (deleted_at);

CREATE INDEX idx_billing_ledger_entries_created_by ON billing_ledger_entries USING btree (created_by);

CREATE INDEX idx_billing_ledger_entries_idempotency_key ON billing_ledger_entries USING btree (idempotency_key);

CREATE INDEX idx_billing_ledger_entries_meter ON billing_ledger_entries USING btree (meter);

CREATE INDEX idx_billing_ledger_entries_project_id ON billing_ledger_entries USING btree (project_id);

CREATE INDEX idx_billing_ledger_entries_reason ON billing_ledger_entries USING btree (reason);

CREATE INDEX idx_billing_ledger_entries_resource_id ON billing_ledger_entries USING btree (resource_id);

CREATE INDEX idx_billing_ledger_entries_resource_type ON billing_ledger_entries USING btree (resource_type);

CREATE INDEX idx_billing_ledger_entries_type ON billing_ledger_entries USING btree (type);

CREATE INDEX idx_billing_ledger_entries_usage_record_id ON billing_ledger_entries USING btree (usage_record_id);

CREATE INDEX idx_billing_ledger_entries_user_id ON billing_ledger_entries USING btree (user_id);

CREATE UNIQUE INDEX idx_billing_ledger_entries_user_idempotency ON billing_ledger_entries USING btree (user_id, idempotency_key) WHERE (idempotency_key <> ''::text);

CREATE UNIQUE INDEX idx_billing_rate_rules_meter ON billing_rate_rules USING btree (meter);

CREATE INDEX idx_billing_usage_records_application_id ON billing_usage_records USING btree (application_id);

CREATE INDEX idx_billing_usage_records_billed_user_id ON billing_usage_records USING btree (billed_user_id);

CREATE INDEX idx_billing_usage_records_meter ON billing_usage_records USING btree (meter);

CREATE INDEX idx_billing_usage_records_period_end ON billing_usage_records USING btree (period_end);

CREATE INDEX idx_billing_usage_records_period_start ON billing_usage_records USING btree (period_start);

CREATE INDEX idx_billing_usage_records_project_id ON billing_usage_records USING btree (project_id);

CREATE INDEX idx_billing_usage_records_resource_id ON billing_usage_records USING btree (resource_id);

CREATE INDEX idx_billing_usage_records_resource_type ON billing_usage_records USING btree (resource_type);

CREATE INDEX idx_billing_usage_records_status ON billing_usage_records USING btree (status);

CREATE UNIQUE INDEX idx_billing_usage_resource_meter ON billing_usage_records USING btree (meter, resource_type, resource_id);

CREATE INDEX idx_build_environment_configs_updated_by ON build_environment_configs USING btree (updated_by);

CREATE UNIQUE INDEX idx_build_environment_scope_ref ON build_environment_configs USING btree (scope, scope_ref);

CREATE INDEX idx_build_jobs_build_run_id ON build_jobs USING btree (build_run_id);

CREATE INDEX idx_build_jobs_builder_id ON build_jobs USING btree (builder_id);

CREATE INDEX idx_build_jobs_deleted_at ON build_jobs USING btree (deleted_at);

CREATE INDEX idx_build_jobs_last_heartbeat_at ON build_jobs USING btree (last_heartbeat_at);

CREATE INDEX idx_build_jobs_lease_token ON build_jobs USING btree (lease_token);

CREATE INDEX idx_build_jobs_lease_until ON build_jobs USING btree (lease_until);

CREATE INDEX idx_build_jobs_project_id ON build_jobs USING btree (project_id);

CREATE INDEX idx_build_jobs_status ON build_jobs USING btree (status);

CREATE UNIQUE INDEX idx_build_logs_build_job_id ON build_logs USING btree (build_job_id);

CREATE INDEX idx_build_logs_build_run_id ON build_logs USING btree (build_run_id);

CREATE INDEX idx_build_logs_project_id ON build_logs USING btree (project_id);

CREATE INDEX idx_build_runs_application_id ON build_runs USING btree (application_id);

CREATE INDEX idx_build_runs_build_template_id ON build_runs USING btree (build_template_id);

CREATE INDEX idx_build_runs_created_by ON build_runs USING btree (created_by);

CREATE INDEX idx_build_runs_deleted_at ON build_runs USING btree (deleted_at);

CREATE INDEX idx_build_runs_deployment_target_id ON build_runs USING btree (deployment_target_id);

CREATE INDEX idx_build_runs_project_id ON build_runs USING btree (project_id);

CREATE INDEX idx_build_runs_retention_terminal ON build_runs USING btree (finished_at, id) WHERE (status = ANY (ARRAY['succeeded'::text, 'failed'::text, 'canceled'::text, 'lost'::text, 'timeout'::text]));

CREATE INDEX idx_build_runs_status ON build_runs USING btree (status);

CREATE INDEX idx_build_runs_target_registry_id ON build_runs USING btree (target_registry_id);

CREATE INDEX idx_build_variable_sets_created_by ON build_variable_sets USING btree (created_by);

CREATE INDEX idx_build_variable_sets_deleted_at ON build_variable_sets USING btree (deleted_at);

CREATE INDEX idx_build_variable_sets_owner_ref ON build_variable_sets USING btree (owner_ref);

CREATE INDEX idx_build_variable_sets_scope ON build_variable_sets USING btree (scope);

CREATE INDEX idx_container_images_application ON container_images USING btree (application_id) WHERE (deleted_at IS NULL);

CREATE INDEX idx_container_images_application_id ON container_images USING btree (application_id);

CREATE INDEX idx_container_images_build_run_id ON container_images USING btree (build_run_id);

CREATE INDEX idx_container_images_created_by ON container_images USING btree (created_by);

CREATE INDEX idx_container_images_deleted_at ON container_images USING btree (deleted_at);

CREATE INDEX idx_container_images_project ON container_images USING btree (project_id) WHERE (deleted_at IS NULL);

CREATE INDEX idx_container_images_project_id ON container_images USING btree (project_id);

CREATE INDEX idx_container_images_registry_id ON container_images USING btree (registry_id);

CREATE INDEX idx_container_images_registry_repo ON container_images USING btree (registry_id, repository) WHERE (deleted_at IS NULL);

CREATE INDEX idx_deployment_target_hook_bindings_application_id ON deployment_target_hook_bindings USING btree (application_id);

CREATE INDEX idx_deployment_target_hook_bindings_hook_config_id ON deployment_target_hook_bindings USING btree (hook_config_id);

CREATE INDEX idx_deployment_target_hook_bindings_phase ON deployment_target_hook_bindings USING btree (phase);

CREATE INDEX idx_deployment_target_hook_bindings_project_id ON deployment_target_hook_bindings USING btree (project_id);

CREATE UNIQUE INDEX idx_deployment_target_hook_bindings_target_hook ON deployment_target_hook_bindings USING btree (target_id, hook_config_id, phase);

CREATE INDEX idx_deployment_target_hook_bindings_target_id ON deployment_target_hook_bindings USING btree (target_id);

CREATE UNIQUE INDEX idx_deployment_targets_app_env_name_active ON deployment_targets USING btree (application_id, environment_id, name) WHERE (deleted_at IS NULL);

CREATE INDEX idx_deployment_targets_application_id ON deployment_targets USING btree (application_id);

CREATE UNIQUE INDEX idx_deployment_targets_application_stage_active ON deployment_targets USING btree (application_id, stage) WHERE (deleted_at IS NULL);

CREATE INDEX idx_deployment_targets_build_environment_id ON deployment_targets USING btree (build_environment_id);

CREATE INDEX idx_deployment_targets_build_template_id ON deployment_targets USING btree (build_template_id);

CREATE INDEX idx_deployment_targets_cluster_id ON deployment_targets USING btree (cluster_id);

CREATE INDEX idx_deployment_targets_created_by ON deployment_targets USING btree (created_by);

CREATE INDEX idx_deployment_targets_deleted_at ON deployment_targets USING btree (deleted_at);

CREATE INDEX idx_deployment_targets_environment_id ON deployment_targets USING btree (environment_id);

CREATE INDEX idx_deployment_targets_project_id ON deployment_targets USING btree (project_id);

CREATE INDEX idx_deployment_targets_repository_binding_id ON deployment_targets USING btree (repository_binding_id);

CREATE INDEX idx_deployment_targets_target_registry_id ON deployment_targets USING btree (target_registry_id);

CREATE INDEX idx_deployment_volume_mounts_deleted_at ON deployment_volume_mounts USING btree (deleted_at);

CREATE UNIQUE INDEX idx_deployment_volume_mounts_device_path_active ON deployment_volume_mounts USING btree (deployment_target_id, device_path) WHERE ((deleted_at IS NULL) AND (device_path IS NOT NULL));

CREATE UNIQUE INDEX idx_deployment_volume_mounts_exclusive_active ON deployment_volume_mounts USING btree (project_volume_id) WHERE ((deleted_at IS NULL) AND (exclusive = true) AND (activation_state = ANY (ARRAY['reserved'::text, 'active'::text, 'release_pending'::text, 'error'::text])));

CREATE UNIQUE INDEX idx_deployment_volume_mounts_logical_name_active ON deployment_volume_mounts USING btree (deployment_target_id, logical_name) WHERE (deleted_at IS NULL);

CREATE UNIQUE INDEX idx_deployment_volume_mounts_mount_path_active ON deployment_volume_mounts USING btree (deployment_target_id, mount_path) WHERE ((deleted_at IS NULL) AND (mount_path IS NOT NULL));

CREATE INDEX idx_deployment_volume_mounts_project_target ON deployment_volume_mounts USING btree (project_id, deployment_target_id) WHERE (deleted_at IS NULL);

CREATE INDEX idx_deployment_volume_mounts_project_volume ON deployment_volume_mounts USING btree (project_volume_id, activation_state) WHERE (deleted_at IS NULL);

CREATE INDEX idx_email_registration_challenges_consumed_at ON email_registration_challenges USING btree (consumed_at);

CREATE INDEX idx_email_registration_challenges_email ON email_registration_challenges USING btree (email);

CREATE INDEX idx_email_registration_challenges_expires_at ON email_registration_challenges USING btree (expires_at);

CREATE UNIQUE INDEX idx_external_identities_provider_subject ON external_identities USING btree (provider_id, subject);

CREATE UNIQUE INDEX idx_external_identities_user_provider ON external_identities USING btree (user_id, provider_id);

CREATE INDEX idx_gateway_routes_application_id ON gateway_routes USING btree (application_id);

CREATE INDEX idx_gateway_routes_created_by ON gateway_routes USING btree (created_by);

CREATE INDEX idx_gateway_routes_deleted_at ON gateway_routes USING btree (deleted_at);

CREATE INDEX idx_gateway_routes_deployment_target_id ON gateway_routes USING btree (deployment_target_id);

CREATE INDEX idx_gateway_routes_environment_id ON gateway_routes USING btree (environment_id);

CREATE INDEX idx_gateway_routes_host ON gateway_routes USING btree (host);

CREATE INDEX idx_gateway_routes_project_id ON gateway_routes USING btree (project_id);

CREATE INDEX idx_git_accounts_deleted_at ON git_accounts USING btree (deleted_at);

CREATE INDEX idx_git_accounts_owner_ref ON git_accounts USING btree (owner_ref);

CREATE INDEX idx_git_accounts_provider_id ON git_accounts USING btree (provider_id);

CREATE INDEX idx_git_accounts_scope_owner_ref ON git_accounts USING btree (scope, owner_ref);

CREATE INDEX idx_git_accounts_user_id ON git_accounts USING btree (user_id);

CREATE INDEX idx_git_accounts_user_provider ON git_accounts USING btree (user_id, provider_id) WHERE (deleted_at IS NULL);

CREATE INDEX idx_git_providers_deleted_at ON git_providers USING btree (deleted_at);

CREATE INDEX idx_git_providers_owner_ref ON git_providers USING btree (owner_ref);

CREATE INDEX idx_git_providers_scope_owner_ref ON git_providers USING btree (scope, owner_ref);

CREATE UNIQUE INDEX idx_hook_run_logs_hook_run_id ON hook_run_logs USING btree (hook_run_id);

CREATE INDEX idx_hook_run_logs_project_id ON hook_run_logs USING btree (project_id);

CREATE INDEX idx_hook_run_logs_retention_parent ON hook_run_logs USING btree (hook_run_id, id);

CREATE INDEX idx_hook_runs_application_id ON hook_runs USING btree (application_id);

CREATE INDEX idx_hook_runs_build_job_id ON hook_runs USING btree (build_job_id);

CREATE INDEX idx_hook_runs_build_run_id ON hook_runs USING btree (build_run_id);

CREATE INDEX idx_hook_runs_deployment_target_id ON hook_runs USING btree (deployment_target_id);

CREATE INDEX idx_hook_runs_environment_id ON hook_runs USING btree (environment_id);

CREATE INDEX idx_hook_runs_hook_config_id ON hook_runs USING btree (hook_config_id);

CREATE INDEX idx_hook_runs_phase ON hook_runs USING btree (phase);

CREATE INDEX idx_hook_runs_project_id ON hook_runs USING btree (project_id);

CREATE INDEX idx_hook_runs_release_id ON hook_runs USING btree (release_id);

CREATE INDEX idx_hook_runs_retention_terminal ON hook_runs USING btree (finished_at, id) WHERE (status = ANY (ARRAY['succeeded'::text, 'failed'::text]));

CREATE INDEX idx_hook_runs_status ON hook_runs USING btree (status);

CREATE UNIQUE INDEX idx_inbox_action_requests_pending_project_type ON inbox_action_requests USING btree (project_id, type) WHERE ((project_id <> ''::text) AND (status = ANY (ARRAY['pending'::text, 'processing'::text])));

CREATE INDEX idx_inbox_action_requests_project_id ON inbox_action_requests USING btree (project_id);

CREATE INDEX idx_inbox_action_requests_recipient_user_id ON inbox_action_requests USING btree (recipient_user_id);

CREATE INDEX idx_inbox_action_requests_requester_user_id ON inbox_action_requests USING btree (requester_user_id);

CREATE INDEX idx_inbox_action_requests_status ON inbox_action_requests USING btree (status);

CREATE INDEX idx_inbox_action_requests_type ON inbox_action_requests USING btree (type);

CREATE INDEX idx_inbox_messages_action_request_id ON inbox_messages USING btree (action_request_id);

CREATE INDEX idx_inbox_messages_category ON inbox_messages USING btree (category);

CREATE UNIQUE INDEX idx_inbox_messages_dedup_key ON inbox_messages USING btree (dedup_key) WHERE (dedup_key IS NOT NULL);

CREATE INDEX idx_inbox_messages_priority ON inbox_messages USING btree (priority);

CREATE INDEX idx_inbox_messages_project_id ON inbox_messages USING btree (project_id);

CREATE INDEX idx_inbox_messages_recipient_user_id ON inbox_messages USING btree (recipient_user_id);

CREATE INDEX idx_inbox_messages_type ON inbox_messages USING btree (type);

CREATE INDEX idx_notification_channels_adapter_kind ON notification_channels USING btree (adapter_kind);

CREATE INDEX idx_notification_channels_deleted_at ON notification_channels USING btree (deleted_at);

CREATE INDEX idx_notification_channels_project_id ON notification_channels USING btree (project_id);

CREATE INDEX idx_notification_deliveries_adapter_kind ON notification_deliveries USING btree (adapter_kind);

CREATE INDEX idx_notification_deliveries_channel_id ON notification_deliveries USING btree (channel_id);

CREATE UNIQUE INDEX idx_notification_deliveries_event_channel ON notification_deliveries USING btree (event_id, channel_id);

CREATE INDEX idx_notification_deliveries_event_id ON notification_deliveries USING btree (event_id);

CREATE INDEX idx_notification_deliveries_event_type ON notification_deliveries USING btree (event_type);

CREATE INDEX idx_notification_deliveries_project_id ON notification_deliveries USING btree (project_id);

CREATE INDEX idx_notification_deliveries_retention_terminal ON notification_deliveries USING btree (finished_at, id) WHERE (status = ANY (ARRAY['succeeded'::text, 'failed'::text]));

CREATE INDEX idx_notification_deliveries_rule_id ON notification_deliveries USING btree (rule_id);

CREATE INDEX idx_notification_deliveries_severity ON notification_deliveries USING btree (severity);

CREATE INDEX idx_notification_deliveries_status ON notification_deliveries USING btree (status);

CREATE INDEX idx_notification_deliveries_template_id ON notification_deliveries USING btree (template_id);

CREATE INDEX idx_notification_rules_deleted_at ON notification_rules USING btree (deleted_at);

CREATE INDEX idx_notification_rules_project_id ON notification_rules USING btree (project_id);

CREATE INDEX idx_notification_rules_template_id ON notification_rules USING btree (template_id);

CREATE INDEX idx_notification_templates_adapter_kind ON notification_templates USING btree (adapter_kind);

CREATE INDEX idx_notification_templates_deleted_at ON notification_templates USING btree (deleted_at);

CREATE INDEX idx_notification_templates_event_type ON notification_templates USING btree (event_type);

CREATE INDEX idx_notification_templates_locale ON notification_templates USING btree (locale);

CREATE INDEX idx_notification_templates_project_id ON notification_templates USING btree (project_id);

CREATE UNIQUE INDEX idx_oauth_applications_client_id ON oauth_applications USING btree (client_id);

CREATE INDEX idx_oauth_applications_owner_user_id ON oauth_applications USING btree (owner_user_id);

CREATE INDEX idx_oauth_applications_revoked_at ON oauth_applications USING btree (revoked_at);

CREATE INDEX idx_oauth_authorization_codes_application_id ON oauth_authorization_codes USING btree (application_id);

CREATE UNIQUE INDEX idx_oauth_authorization_codes_code_hash ON oauth_authorization_codes USING btree (code_hash);

CREATE INDEX idx_oauth_authorization_codes_consumed_at ON oauth_authorization_codes USING btree (consumed_at);

CREATE INDEX idx_oauth_authorization_codes_expires_at ON oauth_authorization_codes USING btree (expires_at);

CREATE INDEX idx_oauth_authorization_codes_grant_id ON oauth_authorization_codes USING btree (grant_id);

CREATE INDEX idx_oauth_authorization_codes_user_id ON oauth_authorization_codes USING btree (user_id);

CREATE INDEX idx_oauth_device_authorizations_application_id ON oauth_device_authorizations USING btree (application_id);

CREATE INDEX idx_oauth_device_authorizations_consumed_at ON oauth_device_authorizations USING btree (consumed_at);

CREATE UNIQUE INDEX idx_oauth_device_authorizations_device_code_hash ON oauth_device_authorizations USING btree (device_code_hash);

CREATE INDEX idx_oauth_device_authorizations_expires_at ON oauth_device_authorizations USING btree (expires_at);

CREATE INDEX idx_oauth_device_authorizations_grant_id ON oauth_device_authorizations USING btree (grant_id);

CREATE INDEX idx_oauth_device_authorizations_status ON oauth_device_authorizations USING btree (status);

CREATE UNIQUE INDEX idx_oauth_device_authorizations_user_code_hash ON oauth_device_authorizations USING btree (user_code_hash);

CREATE INDEX idx_oauth_device_authorizations_user_id ON oauth_device_authorizations USING btree (user_id);

CREATE UNIQUE INDEX idx_oauth_grants_active_application_user ON oauth_grants USING btree (application_id, user_id) WHERE (revoked_at IS NULL);

CREATE INDEX idx_oauth_grants_application_id ON oauth_grants USING btree (application_id);

CREATE INDEX idx_oauth_grants_revoked_at ON oauth_grants USING btree (revoked_at);

CREATE INDEX idx_oauth_grants_user_id ON oauth_grants USING btree (user_id);

CREATE INDEX idx_oauth_refresh_tokens_application_id ON oauth_refresh_tokens USING btree (application_id);

CREATE INDEX idx_oauth_refresh_tokens_consumed_at ON oauth_refresh_tokens USING btree (consumed_at);

CREATE INDEX idx_oauth_refresh_tokens_expires_at ON oauth_refresh_tokens USING btree (expires_at);

CREATE INDEX idx_oauth_refresh_tokens_grant_id ON oauth_refresh_tokens USING btree (grant_id);

CREATE INDEX idx_oauth_refresh_tokens_revoked_at ON oauth_refresh_tokens USING btree (revoked_at);

CREATE UNIQUE INDEX idx_oauth_refresh_tokens_token_hash ON oauth_refresh_tokens USING btree (token_hash);

CREATE INDEX idx_oauth_refresh_tokens_user_id ON oauth_refresh_tokens USING btree (user_id);

CREATE INDEX idx_platform_events_actor_id ON platform_events USING btree (actor_id);

CREATE INDEX idx_platform_events_application_id ON platform_events USING btree (application_id);

CREATE INDEX idx_platform_events_category ON platform_events USING btree (category);

CREATE INDEX idx_platform_events_correlation_id ON platform_events USING btree (correlation_id);

CREATE UNIQUE INDEX idx_platform_events_dedup_key ON platform_events USING btree (dedup_key) WHERE (dedup_key IS NOT NULL);

CREATE INDEX idx_platform_events_deployment_target_id ON platform_events USING btree (deployment_target_id);

CREATE INDEX idx_platform_events_occurred_at ON platform_events USING btree (occurred_at DESC);

CREATE INDEX idx_platform_events_project_id ON platform_events USING btree (project_id);

CREATE INDEX idx_platform_events_resource ON platform_events USING btree (resource_type, resource_id);

CREATE INDEX idx_platform_events_retention ON platform_events USING btree (occurred_at, id);

CREATE INDEX idx_platform_events_severity ON platform_events USING btree (severity);

CREATE INDEX idx_platform_events_status ON platform_events USING btree (status);

CREATE INDEX idx_platform_events_type ON platform_events USING btree (type);

CREATE INDEX idx_project_hook_configs_created_by ON project_hook_configs USING btree (created_by);

CREATE INDEX idx_project_hook_configs_deleted_at ON project_hook_configs USING btree (deleted_at);

CREATE INDEX idx_project_hook_configs_project_id ON project_hook_configs USING btree (project_id);

CREATE INDEX idx_project_members_last_used_at ON project_members USING btree (last_used_at);

CREATE INDEX idx_project_members_project_id ON project_members USING btree (project_id);

CREATE INDEX idx_project_members_use_count ON project_members USING btree (use_count);

CREATE INDEX idx_project_members_user_dashboard_order ON project_members USING btree (user_id, dashboard_order);

CREATE INDEX idx_project_members_user_id ON project_members USING btree (user_id);

CREATE INDEX idx_project_pins_project_id ON project_pins USING btree (project_id);

CREATE INDEX idx_project_pins_user_id ON project_pins USING btree (user_id);

CREATE INDEX idx_project_pins_user_pinned_at ON project_pins USING btree (user_id, pinned_at DESC);

CREATE UNIQUE INDEX idx_project_pins_user_project ON project_pins USING btree (user_id, project_id);

CREATE INDEX idx_project_runtime_config_sets_created_by ON project_runtime_config_sets USING btree (created_by);

CREATE INDEX idx_project_runtime_config_sets_delete_status ON project_runtime_config_sets USING btree (delete_status);

CREATE INDEX idx_project_runtime_config_sets_deleted_at ON project_runtime_config_sets USING btree (deleted_at);

CREATE INDEX idx_project_runtime_config_sets_project_id ON project_runtime_config_sets USING btree (project_id);

CREATE UNIQUE INDEX idx_project_topology_edges_identity ON project_topology_edges USING btree (project_id, source_application_id, source_deployment_target_id, target_application_id, target_deployment_target_id, relation_type, protocol, port);

CREATE INDEX idx_project_topology_edges_project_id ON project_topology_edges USING btree (project_id);

CREATE INDEX idx_project_topology_edges_source_application_id ON project_topology_edges USING btree (source_application_id);

CREATE INDEX idx_project_topology_edges_source_target_id ON project_topology_edges USING btree (source_deployment_target_id);

CREATE INDEX idx_project_topology_edges_target_application_id ON project_topology_edges USING btree (target_application_id);

CREATE INDEX idx_project_topology_edges_target_target_id ON project_topology_edges USING btree (target_deployment_target_id);

CREATE UNIQUE INDEX idx_project_volumes_claim_active ON project_volumes USING btree (cluster_id, namespace, claim_name) WHERE (deleted_at IS NULL);

CREATE INDEX idx_project_volumes_cluster ON project_volumes USING btree (cluster_id) WHERE (deleted_at IS NULL);

CREATE INDEX idx_project_volumes_deleted_at ON project_volumes USING btree (deleted_at);

CREATE UNIQUE INDEX idx_project_volumes_display_name_active ON project_volumes USING btree (project_id, lower(display_name)) WHERE (deleted_at IS NULL);

CREATE UNIQUE INDEX idx_project_volumes_idempotency_active ON project_volumes USING btree (project_id, idempotency_key_hash) WHERE ((deleted_at IS NULL) AND (idempotency_key_hash <> ''::text));

CREATE INDEX idx_project_volumes_project_lifecycle_created ON project_volumes USING btree (project_id, lifecycle_state, created_at DESC) WHERE (deleted_at IS NULL);

CREATE INDEX idx_projects_billing_owner_user_id ON projects USING btree (billing_owner_user_id);

CREATE INDEX idx_projects_deleted_at ON projects USING btree (deleted_at);

CREATE UNIQUE INDEX idx_projects_identifier_active ON projects USING btree (identifier) WHERE (deleted_at IS NULL);

CREATE UNIQUE INDEX idx_projects_system_key ON projects USING btree (system_key) WHERE (system_key <> ''::text);

CREATE INDEX idx_registry_credentials_created_by ON registry_credentials USING btree (created_by);

CREATE INDEX idx_registry_credentials_deleted_at ON registry_credentials USING btree (deleted_at);

CREATE INDEX idx_registry_credentials_registry ON registry_credentials USING btree (registry_id) WHERE (deleted_at IS NULL);

CREATE INDEX idx_registry_credentials_registry_id ON registry_credentials USING btree (registry_id);

CREATE INDEX idx_registry_credentials_scope_owner ON registry_credentials USING btree (scope, owner_ref);

CREATE INDEX idx_release_logs_project_id ON release_logs USING btree (project_id);

CREATE UNIQUE INDEX idx_release_logs_release_id ON release_logs USING btree (release_id);

CREATE INDEX idx_release_logs_retention_parent ON release_logs USING btree (release_id, id);

CREATE INDEX idx_releases_application_id ON releases USING btree (application_id);

CREATE INDEX idx_releases_build_run_id ON releases USING btree (build_run_id);

CREATE INDEX idx_releases_created_by ON releases USING btree (created_by);

CREATE INDEX idx_releases_deleted_at ON releases USING btree (deleted_at);

CREATE INDEX idx_releases_deployment_target_id ON releases USING btree (deployment_target_id);

CREATE INDEX idx_releases_environment_id ON releases USING btree (environment_id);

CREATE INDEX idx_releases_project_id ON releases USING btree (project_id);

CREATE INDEX idx_releases_retention_terminal ON releases USING btree (finished_at, id) WHERE (status = ANY (ARRAY['succeeded'::text, 'failed'::text]));

CREATE INDEX idx_releases_rollback_from_id ON releases USING btree (rollback_from_id);

CREATE INDEX idx_releases_status ON releases USING btree (status);

CREATE INDEX idx_repository_bindings_application_id ON repository_bindings USING btree (application_id);

CREATE UNIQUE INDEX idx_repository_bindings_application_repo_active ON repository_bindings USING btree (application_id, git_account_id, owner, repo) WHERE (deleted_at IS NULL);

CREATE INDEX idx_repository_bindings_deleted_at ON repository_bindings USING btree (deleted_at);

CREATE INDEX idx_repository_bindings_git_account_id ON repository_bindings USING btree (git_account_id);

CREATE INDEX idx_repository_bindings_git_provider_id ON repository_bindings USING btree (git_provider_id);

CREATE INDEX idx_repository_bindings_project ON repository_bindings USING btree (project_id) WHERE (deleted_at IS NULL);

CREATE INDEX idx_repository_bindings_project_id ON repository_bindings USING btree (project_id);

CREATE INDEX idx_retained_volumes_claimed_target ON retained_volumes USING btree (claimed_by_target_id);

CREATE INDEX idx_retained_volumes_project_status ON retained_volumes USING btree (project_id, status);

CREATE INDEX idx_retained_volumes_source_application ON retained_volumes USING btree (source_application_id);

CREATE INDEX idx_runtime_clusters_created_by ON runtime_clusters USING btree (created_by);

CREATE INDEX idx_runtime_clusters_deleted_at ON runtime_clusters USING btree (deleted_at);

CREATE INDEX idx_runtime_clusters_owner_ref ON runtime_clusters USING btree (owner_ref);

CREATE INDEX idx_runtime_clusters_scope ON runtime_clusters USING btree (scope);

CREATE UNIQUE INDEX idx_scoped_resource_project_bindings_default_registry ON scoped_resource_project_bindings USING btree (project_id) WHERE ((resource_type = 'artifact_registry'::text) AND is_default);

CREATE INDEX idx_scoped_resource_project_bindings_project_id ON scoped_resource_project_bindings USING btree (project_id);

CREATE INDEX idx_scoped_resource_project_bindings_type_id ON scoped_resource_project_bindings USING btree (resource_type, resource_id);

CREATE INDEX idx_secret_values_created_by ON secret_values USING btree (created_by);

CREATE INDEX idx_secret_values_resource ON secret_values USING btree (resource);

CREATE INDEX idx_service_bindings_project_enabled ON service_bindings USING btree (project_id, enabled);

CREATE INDEX idx_service_bindings_project_id ON service_bindings USING btree (project_id);

CREATE INDEX idx_service_bindings_source_application_id ON service_bindings USING btree (source_application_id);

CREATE UNIQUE INDEX idx_service_bindings_source_host_env ON service_bindings USING btree (source_deployment_target_id, host_env_var) WHERE (host_env_var <> ''::text);

CREATE UNIQUE INDEX idx_service_bindings_source_port_env ON service_bindings USING btree (source_deployment_target_id, port_env_var) WHERE (port_env_var <> ''::text);

CREATE INDEX idx_service_bindings_source_target_id ON service_bindings USING btree (source_deployment_target_id);

CREATE UNIQUE INDEX idx_service_bindings_source_url_env ON service_bindings USING btree (source_deployment_target_id, url_env_var) WHERE (url_env_var <> ''::text);

CREATE INDEX idx_service_bindings_target_application_id ON service_bindings USING btree (target_application_id);

CREATE INDEX idx_service_bindings_target_target_id ON service_bindings USING btree (target_deployment_target_id);

CREATE UNIQUE INDEX idx_system_component_cluster ON system_component_installations USING btree (component_id, runtime_cluster_id);

CREATE INDEX idx_system_component_installations_application_id ON system_component_installations USING btree (application_id);

CREATE INDEX idx_system_component_installations_component_id ON system_component_installations USING btree (component_id);

CREATE INDEX idx_system_component_installations_controller_type ON system_component_installations USING btree (controller_type);

CREATE INDEX idx_system_component_installations_deployment_target_id ON system_component_installations USING btree (deployment_target_id);

CREATE INDEX idx_system_component_installations_installed_by ON system_component_installations USING btree (installed_by);

CREATE INDEX idx_system_component_installations_project_id ON system_component_installations USING btree (project_id);

CREATE INDEX idx_system_component_installations_release_id ON system_component_installations USING btree (release_id);

CREATE INDEX idx_system_component_installations_runtime_cluster_id ON system_component_installations USING btree (runtime_cluster_id);

CREATE INDEX idx_system_component_installations_status ON system_component_installations USING btree (status);

CREATE INDEX idx_user_remember_tokens_consumed_at ON user_remember_tokens USING btree (consumed_at);

CREATE INDEX idx_user_remember_tokens_expires_at ON user_remember_tokens USING btree (expires_at);

CREATE INDEX idx_user_remember_tokens_family_id ON user_remember_tokens USING btree (family_id);

CREATE INDEX idx_user_remember_tokens_retention_expiry ON user_remember_tokens USING btree (expires_at, id);

CREATE INDEX idx_user_remember_tokens_revoked_at ON user_remember_tokens USING btree (revoked_at);

CREATE UNIQUE INDEX idx_user_remember_tokens_token_hash ON user_remember_tokens USING btree (token_hash);

CREATE INDEX idx_user_remember_tokens_user_family ON user_remember_tokens USING btree (user_id, family_id);

CREATE INDEX idx_user_remember_tokens_user_id ON user_remember_tokens USING btree (user_id);

CREATE INDEX idx_user_sessions_expires_at ON user_sessions USING btree (expires_at);

CREATE INDEX idx_user_sessions_impersonator_id ON user_sessions USING btree (impersonator_id);

CREATE INDEX idx_user_sessions_primary_authenticated_at ON user_sessions USING btree (primary_authenticated_at);

CREATE INDEX idx_user_sessions_remember_family_id ON user_sessions USING btree (remember_family_id);

CREATE INDEX idx_user_sessions_retention_expiry ON user_sessions USING btree (expires_at, id);

CREATE UNIQUE INDEX idx_user_sessions_token_hash ON user_sessions USING btree (token_hash);

CREATE INDEX idx_user_sessions_user_id ON user_sessions USING btree (user_id);

CREATE INDEX idx_user_sessions_user_remember_family ON user_sessions USING btree (user_id, remember_family_id);

CREATE UNIQUE INDEX idx_user_wallets_user_id ON user_wallets USING btree (user_id);

CREATE INDEX idx_users_deleted_at ON users USING btree (deleted_at);

CREATE UNIQUE INDEX idx_users_email ON users USING btree (email);

CREATE INDEX idx_volume_transfer_parts_transfer_offset ON volume_transfer_parts USING btree (transfer_id, byte_offset);

CREATE UNIQUE INDEX idx_volume_transfers_active_export ON volume_transfers USING btree (project_volume_id) WHERE ((direction = 'export'::text) AND (state = ANY (ARRAY['created'::text, 'uploading'::text, 'queued'::text, 'running'::text])));

CREATE UNIQUE INDEX idx_volume_transfers_active_import ON volume_transfers USING btree (project_volume_id) WHERE ((direction = 'import'::text) AND (state = ANY (ARRAY['created'::text, 'uploading'::text, 'queued'::text, 'running'::text])));

CREATE INDEX idx_volume_transfers_expired_objects ON volume_transfers USING btree (expires_at, id) WHERE ((object_deleted_at IS NULL) AND (state = ANY (ARRAY['created'::text, 'uploading'::text, 'succeeded'::text, 'failed'::text, 'cancelled'::text, 'expired'::text])));

CREATE INDEX idx_volume_transfers_project_state_created ON volume_transfers USING btree (project_id, state, created_at DESC);

CREATE INDEX idx_volume_transfers_volume_created ON volume_transfers USING btree (project_volume_id, created_at DESC);

ALTER TABLE ONLY ai.conversation_summaries
    ADD CONSTRAINT conversation_summaries_conversation_id_fkey FOREIGN KEY (conversation_id) REFERENCES ai.conversations(id) ON DELETE CASCADE;

ALTER TABLE ONLY ai.idempotency_keys
    ADD CONSTRAINT idempotency_keys_run_id_fkey FOREIGN KEY (run_id) REFERENCES ai.runs(id) ON DELETE CASCADE;

ALTER TABLE ONLY ai.idempotency_keys
    ADD CONSTRAINT idempotency_keys_turn_id_fkey FOREIGN KEY (turn_id) REFERENCES ai.turns(id) ON DELETE CASCADE;

ALTER TABLE ONLY ai.items
    ADD CONSTRAINT items_run_id_fkey FOREIGN KEY (run_id) REFERENCES ai.runs(id) ON DELETE CASCADE;

ALTER TABLE ONLY ai.items
    ADD CONSTRAINT items_turn_id_fkey FOREIGN KEY (turn_id) REFERENCES ai.turns(id) ON DELETE CASCADE;

ALTER TABLE ONLY ai.run_events
    ADD CONSTRAINT run_events_run_id_fkey FOREIGN KEY (run_id) REFERENCES ai.runs(id) ON DELETE CASCADE;

ALTER TABLE ONLY ai.runs
    ADD CONSTRAINT runs_conversation_id_fkey FOREIGN KEY (conversation_id) REFERENCES ai.conversations(id) ON DELETE CASCADE;

ALTER TABLE ONLY ai.runs
    ADD CONSTRAINT runs_turn_id_fkey FOREIGN KEY (turn_id) REFERENCES ai.turns(id) ON DELETE CASCADE;

ALTER TABLE ONLY ai.tool_calls
    ADD CONSTRAINT tool_calls_run_id_fkey FOREIGN KEY (run_id) REFERENCES ai.runs(id) ON DELETE CASCADE;

ALTER TABLE ONLY ai.turns
    ADD CONSTRAINT turns_conversation_id_fkey FOREIGN KEY (conversation_id) REFERENCES ai.conversations(id) ON DELETE CASCADE;

ALTER TABLE ONLY access_tokens
    ADD CONSTRAINT access_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY app_template_installations
    ADD CONSTRAINT app_template_installations_application_id_fkey FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE CASCADE;

ALTER TABLE ONLY app_template_installations
    ADD CONSTRAINT app_template_installations_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

ALTER TABLE ONLY applications
    ADD CONSTRAINT applications_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

ALTER TABLE ONLY container_images
    ADD CONSTRAINT container_images_registry_id_fkey FOREIGN KEY (registry_id) REFERENCES artifact_registries(id) ON DELETE RESTRICT;

ALTER TABLE ONLY deployment_volume_mounts
    ADD CONSTRAINT deployment_volume_mounts_application_id_fkey FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE RESTRICT;

ALTER TABLE ONLY deployment_volume_mounts
    ADD CONSTRAINT deployment_volume_mounts_deployment_target_id_fkey FOREIGN KEY (deployment_target_id) REFERENCES deployment_targets(id) ON DELETE RESTRICT;

ALTER TABLE ONLY deployment_volume_mounts
    ADD CONSTRAINT deployment_volume_mounts_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE RESTRICT;

ALTER TABLE ONLY deployment_volume_mounts
    ADD CONSTRAINT deployment_volume_mounts_project_volume_id_fkey FOREIGN KEY (project_volume_id) REFERENCES project_volumes(id) ON DELETE RESTRICT;

ALTER TABLE ONLY external_identities
    ADD CONSTRAINT external_identities_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES auth_providers(id) ON DELETE RESTRICT;

ALTER TABLE ONLY external_identities
    ADD CONSTRAINT external_identities_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY git_accounts
    ADD CONSTRAINT git_accounts_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES git_providers(id) ON DELETE RESTRICT;

ALTER TABLE ONLY git_accounts
    ADD CONSTRAINT git_accounts_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY hook_run_logs
    ADD CONSTRAINT hook_run_logs_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

ALTER TABLE ONLY hook_runs
    ADD CONSTRAINT hook_runs_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

ALTER TABLE ONLY oauth_applications
    ADD CONSTRAINT oauth_applications_owner_user_id_fkey FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY oauth_authorization_codes
    ADD CONSTRAINT oauth_authorization_codes_application_id_fkey FOREIGN KEY (application_id) REFERENCES oauth_applications(id) ON DELETE CASCADE;

ALTER TABLE ONLY oauth_authorization_codes
    ADD CONSTRAINT oauth_authorization_codes_grant_id_fkey FOREIGN KEY (grant_id) REFERENCES oauth_grants(id) ON DELETE CASCADE;

ALTER TABLE ONLY oauth_authorization_codes
    ADD CONSTRAINT oauth_authorization_codes_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY oauth_device_authorizations
    ADD CONSTRAINT oauth_device_authorizations_application_id_fkey FOREIGN KEY (application_id) REFERENCES oauth_applications(id) ON DELETE CASCADE;

ALTER TABLE ONLY oauth_device_authorizations
    ADD CONSTRAINT oauth_device_authorizations_grant_id_fkey FOREIGN KEY (grant_id) REFERENCES oauth_grants(id) ON DELETE CASCADE;

ALTER TABLE ONLY oauth_device_authorizations
    ADD CONSTRAINT oauth_device_authorizations_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY oauth_grants
    ADD CONSTRAINT oauth_grants_application_id_fkey FOREIGN KEY (application_id) REFERENCES oauth_applications(id) ON DELETE CASCADE;

ALTER TABLE ONLY oauth_grants
    ADD CONSTRAINT oauth_grants_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY oauth_refresh_tokens
    ADD CONSTRAINT oauth_refresh_tokens_application_id_fkey FOREIGN KEY (application_id) REFERENCES oauth_applications(id) ON DELETE CASCADE;

ALTER TABLE ONLY oauth_refresh_tokens
    ADD CONSTRAINT oauth_refresh_tokens_grant_id_fkey FOREIGN KEY (grant_id) REFERENCES oauth_grants(id) ON DELETE CASCADE;

ALTER TABLE ONLY oauth_refresh_tokens
    ADD CONSTRAINT oauth_refresh_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY project_hook_configs
    ADD CONSTRAINT project_hook_configs_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

ALTER TABLE ONLY project_members
    ADD CONSTRAINT project_members_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

ALTER TABLE ONLY project_members
    ADD CONSTRAINT project_members_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY project_pins
    ADD CONSTRAINT project_pins_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

ALTER TABLE ONLY project_pins
    ADD CONSTRAINT project_pins_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY project_topology_edges
    ADD CONSTRAINT project_topology_edges_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

ALTER TABLE ONLY project_topology_edges
    ADD CONSTRAINT project_topology_edges_source_application_id_fkey FOREIGN KEY (source_application_id) REFERENCES applications(id) ON DELETE CASCADE;

ALTER TABLE ONLY project_topology_edges
    ADD CONSTRAINT project_topology_edges_target_application_id_fkey FOREIGN KEY (target_application_id) REFERENCES applications(id) ON DELETE CASCADE;

ALTER TABLE ONLY project_volumes
    ADD CONSTRAINT project_volumes_cluster_id_fkey FOREIGN KEY (cluster_id) REFERENCES runtime_clusters(id) ON DELETE RESTRICT;

ALTER TABLE ONLY project_volumes
    ADD CONSTRAINT project_volumes_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE RESTRICT;

ALTER TABLE ONLY project_volumes
    ADD CONSTRAINT project_volumes_source_application_id_fkey FOREIGN KEY (source_application_id) REFERENCES applications(id) ON DELETE SET NULL;

ALTER TABLE ONLY project_volumes
    ADD CONSTRAINT project_volumes_source_deployment_target_id_fkey FOREIGN KEY (source_deployment_target_id) REFERENCES deployment_targets(id) ON DELETE SET NULL;

ALTER TABLE ONLY registry_credentials
    ADD CONSTRAINT registry_credentials_registry_id_fkey FOREIGN KEY (registry_id) REFERENCES artifact_registries(id) ON DELETE CASCADE;

ALTER TABLE ONLY repository_bindings
    ADD CONSTRAINT repository_bindings_application_id_fkey FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE CASCADE;

ALTER TABLE ONLY repository_bindings
    ADD CONSTRAINT repository_bindings_git_account_id_fkey FOREIGN KEY (git_account_id) REFERENCES git_accounts(id) ON DELETE RESTRICT;

ALTER TABLE ONLY repository_bindings
    ADD CONSTRAINT repository_bindings_git_provider_id_fkey FOREIGN KEY (git_provider_id) REFERENCES git_providers(id) ON DELETE RESTRICT;

ALTER TABLE ONLY repository_bindings
    ADD CONSTRAINT repository_bindings_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

ALTER TABLE ONLY scoped_resource_project_bindings
    ADD CONSTRAINT scoped_resource_project_bindings_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

ALTER TABLE ONLY service_bindings
    ADD CONSTRAINT service_bindings_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

ALTER TABLE ONLY service_bindings
    ADD CONSTRAINT service_bindings_source_application_id_fkey FOREIGN KEY (source_application_id) REFERENCES applications(id) ON DELETE RESTRICT;

ALTER TABLE ONLY service_bindings
    ADD CONSTRAINT service_bindings_source_deployment_target_id_fkey FOREIGN KEY (source_deployment_target_id) REFERENCES deployment_targets(id) ON DELETE RESTRICT;

ALTER TABLE ONLY service_bindings
    ADD CONSTRAINT service_bindings_target_application_id_fkey FOREIGN KEY (target_application_id) REFERENCES applications(id) ON DELETE RESTRICT;

ALTER TABLE ONLY service_bindings
    ADD CONSTRAINT service_bindings_target_deployment_target_id_fkey FOREIGN KEY (target_deployment_target_id) REFERENCES deployment_targets(id) ON DELETE RESTRICT;

ALTER TABLE ONLY user_remember_tokens
    ADD CONSTRAINT user_remember_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY user_sessions
    ADD CONSTRAINT user_sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY user_wallets
    ADD CONSTRAINT user_wallets_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY volume_transfer_parts
    ADD CONSTRAINT volume_transfer_parts_transfer_id_fkey FOREIGN KEY (transfer_id) REFERENCES volume_transfers(id) ON DELETE CASCADE;

ALTER TABLE ONLY volume_transfers
    ADD CONSTRAINT volume_transfers_project_id_fkey FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE RESTRICT;

ALTER TABLE ONLY volume_transfers
    ADD CONSTRAINT volume_transfers_project_volume_id_fkey FOREIGN KEY (project_volume_id) REFERENCES project_volumes(id) ON DELETE RESTRICT;

COMMENT ON COLUMN ai.tool_calls.arguments IS
    'Redacted tool arguments for audit and UI projection only.';

COMMENT ON COLUMN ai.tool_calls.arguments_ciphertext IS
    'Authenticated encrypted executable tool arguments; never expose to clients.';


INSERT INTO oauth_applications (
    id,
    owner_user_id,
    name,
    description,
    homepage_url,
    logo_url,
    client_id,
    client_secret_hash,
    redirect_uris,
    allowed_scopes,
    access_token_lifetime_days,
    created_at,
    updated_at
) VALUES (
    'oapp_luna_cli',
    NULL,
    'Luna CLI',
    'Built-in public OAuth client for Luna CLI device authorization.',
    '',
    '',
    'luna-cli',
    '',
    '[]',
    '*',
    1,
    now(),
    now()
) ON CONFLICT (client_id) DO NOTHING;

INSERT INTO app_configs (key, value, updated_at)
VALUES ('ai.access.mode', 'all_authenticated', now())
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value,
    updated_at = EXCLUDED.updated_at
WHERE app_configs.value = 'admins';

INSERT INTO billing_rate_rules (
    id,
    meter,
    unit,
    credits_per_unit,
    enabled,
    description,
    created_at,
    updated_at
)
VALUES
    ('brte_' || LEFT(md5('build.cpu_vcpu_minute'), 24), 'build.cpu_vcpu_minute', 'vcpu_minute', 10, true, 'Build CPU usage', now(), now()),
    ('brte_' || LEFT(md5('build.memory_gib_minute'), 24), 'build.memory_gib_minute', 'gib_minute', 2, true, 'Build memory usage', now(), now()),
    ('brte_' || LEFT(md5('runtime.cpu_vcpu_hour'), 24), 'runtime.cpu_vcpu_hour', 'vcpu_hour', 30, true, 'Runtime CPU usage', now(), now()),
    ('brte_' || LEFT(md5('runtime.memory_gib_hour'), 24), 'runtime.memory_gib_hour', 'gib_hour', 6, true, 'Runtime memory usage', now(), now()),
    ('brte_' || LEFT(md5('storage.gib_day'), 24), 'storage.gib_day', 'gib_day', 1, true, 'Persistent storage usage', now(), now()),
    ('brte_' || LEFT(md5('gateway.egress_gib'), 24), 'gateway.egress_gib', 'gib', 1, true, 'Gateway response egress traffic', now(), now()),
    ('brte_' || LEFT(md5('gateway.requests_1000'), 24), 'gateway.requests_1000', '1000_requests', 0, false, 'Gateway request count', now(), now()),
    ('brte_' || LEFT(md5('ai.input_tokens_1000'), 24), 'ai.input_tokens_1000', '1000_tokens', 1, true, 'AI model input tokens', now(), now()),
    ('brte_' || LEFT(md5('ai.output_tokens_1000'), 24), 'ai.output_tokens_1000', '1000_tokens', 4, true, 'AI model output tokens', now(), now())
ON CONFLICT (meter) DO NOTHING;
