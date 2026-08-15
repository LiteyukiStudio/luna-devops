CREATE SCHEMA IF NOT EXISTS ai;

CREATE TABLE ai.conversations (
    id text PRIMARY KEY,
    owner_user_id text NOT NULL,
    project_id text,
    title text NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status = 'active'),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ai_conversations_owner_updated_idx
    ON ai.conversations (owner_user_id, updated_at DESC);

CREATE TABLE ai.turns (
    id text PRIMARY KEY,
    conversation_id text NOT NULL REFERENCES ai.conversations(id) ON DELETE CASCADE,
    turn_index integer NOT NULL,
    status text NOT NULL,
    input text NOT NULL,
    selected_run_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (conversation_id, turn_index)
);

CREATE TABLE ai.runs (
    id text PRIMARY KEY,
    owner_user_id text NOT NULL,
    conversation_id text NOT NULL REFERENCES ai.conversations(id) ON DELETE CASCADE,
    turn_id text NOT NULL REFERENCES ai.turns(id) ON DELETE CASCADE,
    run_index integer NOT NULL,
    status text NOT NULL,
    row_version integer NOT NULL DEFAULT 1,
    prompt_version text NOT NULL,
    tool_catalog_digest text NOT NULL,
    page_context jsonb NOT NULL DEFAULT '{}'::jsonb,
    run_actor_grant_ciphertext text,
    lease_owner text,
    lease_expires_at timestamptz,
    heartbeat_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    completed_at timestamptz,
    error_code text,
    UNIQUE (turn_id, run_index)
);

CREATE INDEX ai_runs_queue_idx
    ON ai.runs (status, lease_expires_at)
    WHERE status = 'queued';

CREATE TABLE ai.items (
    id text PRIMARY KEY,
    run_id text NOT NULL REFERENCES ai.runs(id) ON DELETE CASCADE,
    turn_id text NOT NULL REFERENCES ai.turns(id) ON DELETE CASCADE,
    timeline_index integer NOT NULL,
    type text NOT NULL,
    status text NOT NULL,
    content jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (run_id, timeline_index)
);

CREATE TABLE ai.run_events (
    id text PRIMARY KEY,
    run_id text NOT NULL REFERENCES ai.runs(id) ON DELETE CASCADE,
    event_sequence bigint NOT NULL,
    type text NOT NULL,
    data jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (run_id, event_sequence)
);

CREATE TABLE ai.tool_calls (
    id text PRIMARY KEY,
    run_id text NOT NULL REFERENCES ai.runs(id) ON DELETE CASCADE,
    operation_id text NOT NULL,
    status text NOT NULL,
    arguments jsonb NOT NULL,
    arguments_hash text NOT NULL,
    attempt integer NOT NULL DEFAULT 1,
    row_version integer NOT NULL DEFAULT 1,
    approval_expires_at timestamptz,
    mfa_purpose text,
    result jsonb,
    error_code text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ai_tool_calls_run_created_idx
    ON ai.tool_calls (run_id, created_at);

CREATE TABLE ai.idempotency_keys (
    owner_user_id text NOT NULL,
    idempotency_key text NOT NULL,
    request_hash text NOT NULL,
    turn_id text NOT NULL REFERENCES ai.turns(id) ON DELETE CASCADE,
    run_id text NOT NULL REFERENCES ai.runs(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (owner_user_id, idempotency_key)
);

CREATE OR REPLACE FUNCTION ai.claim_next_run(instance_id text, lease_seconds integer)
RETURNS TABLE(run_id text, owner_user_id text, lease_expires_at timestamptz)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, ai
AS $$
BEGIN
    IF length(instance_id) < 1
       OR length(instance_id) > 128
       OR lease_seconds < 5
       OR lease_seconds > 300 THEN
        RAISE EXCEPTION 'invalid lease request';
    END IF;

    RETURN QUERY
    WITH candidate AS (
        SELECT id
        FROM ai.runs
        WHERE status = 'queued'
          AND (ai.runs.lease_expires_at IS NULL OR ai.runs.lease_expires_at <= now())
        ORDER BY created_at
        FOR UPDATE SKIP LOCKED
        LIMIT 1
    )
    UPDATE ai.runs AS run
    SET lease_owner = instance_id,
        lease_expires_at = now() + make_interval(secs => lease_seconds),
        heartbeat_at = now()
    FROM candidate
    WHERE run.id = candidate.id
    RETURNING run.id, run.owner_user_id, run.lease_expires_at;
END;
$$;

CREATE OR REPLACE FUNCTION ai.renew_run_lease(
    p_run_id text,
    instance_id text,
    lease_seconds integer
)
RETURNS boolean
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, ai
AS $$
    UPDATE ai.runs
    SET lease_expires_at = now() + make_interval(secs => lease_seconds),
        heartbeat_at = now()
    WHERE id = p_run_id
      AND lease_owner = instance_id
      AND status IN ('queued', 'running')
    RETURNING true
$$;

CREATE OR REPLACE FUNCTION ai.release_run_lease(p_run_id text, instance_id text)
RETURNS boolean
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog, ai
AS $$
    UPDATE ai.runs
    SET lease_owner = NULL,
        lease_expires_at = NULL
    WHERE id = p_run_id
      AND lease_owner = instance_id
    RETURNING true
$$;
