-- Reference schema for local development and Agent repository tests.
-- Production installation remains owned by the platform golang-migrate job.
create schema if not exists ai;

create table if not exists ai.conversations (
  id text primary key,
  owner_user_id text not null,
  project_id text,
  model_id text,
  title text not null,
  title_source text not null default 'default' check (title_source in ('default', 'assistant', 'user')),
  status text not null default 'active' check (status = 'active'),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create index if not exists conversations_owner_updated on ai.conversations(owner_user_id, updated_at desc);
alter table ai.conversations
  add column if not exists title_source text not null default 'default'
  check (title_source in ('default', 'assistant', 'user'));
alter table ai.conversations add column if not exists model_id text;
update ai.conversations set title_source = 'user' where title <> '新会话' and title_source = 'default';

create table if not exists ai.turns (
  id text primary key,
  conversation_id text not null references ai.conversations(id) on delete cascade,
  turn_index integer not null,
  status text not null,
  input text not null,
  selected_run_id text not null,
  model_id text,
  created_at timestamptz not null default now(),
  unique(conversation_id, turn_index)
);

create table if not exists ai.runs (
  id text primary key,
  owner_user_id text not null,
  conversation_id text not null references ai.conversations(id) on delete cascade,
  turn_id text not null references ai.turns(id) on delete cascade,
  run_index integer not null,
  status text not null,
  row_version integer not null default 1,
  prompt_version text not null,
  tool_catalog_digest text not null,
  selected_operation_ids text[] not null default '{}',
  page_context jsonb not null default '{}',
  actor_session_id text not null,
  created_at timestamptz not null default now(),
  started_at timestamptz,
  completed_at timestamptz,
  error_code text,
  model_id text,
  model_name text,
  max_context_tokens bigint,
  max_output_tokens bigint,
  input_credits_per_million numeric(24,8),
  output_credits_per_million numeric(24,8),
  cached_input_credits_per_million numeric(24,8),
  client_instance_id text,
  trace_context jsonb not null default '{}'::jsonb
    check (jsonb_typeof(trace_context) = 'object'),
  next_item_position bigint not null default 0,
  next_event_sequence bigint not null default 1,
  unique(turn_id, run_index)
);
create index if not exists runs_queue on ai.runs(status, created_at) where status = 'queued';
alter table ai.turns add column if not exists model_id text;
update ai.conversations as conversation
set model_id = latest_turn.model_id
from (
  select distinct on (conversation_id) conversation_id, model_id
  from ai.turns
  where model_id is not null
  order by conversation_id, turn_index desc
) as latest_turn
where conversation.id = latest_turn.conversation_id
  and conversation.model_id is null;
alter table ai.runs add column if not exists model_id text;
alter table ai.runs add column if not exists model_name text;
alter table ai.runs add column if not exists max_context_tokens bigint;
alter table ai.runs add column if not exists max_output_tokens bigint;
alter table ai.runs add column if not exists input_credits_per_million numeric(24,8);
alter table ai.runs add column if not exists output_credits_per_million numeric(24,8);
alter table ai.runs add column if not exists cached_input_credits_per_million numeric(24,8);
alter table ai.runs add column if not exists selected_operation_ids text[] not null default '{}';

create table if not exists ai.model_credit_holds (
  id text primary key,
  run_id text not null references ai.runs(id) on delete cascade,
  owner_user_id text not null,
  operation text not null check (operation in ('assistant', 'summary', 'title')),
  attempt integer not null check (attempt > 0),
  state text not null check (state in ('held', 'released', 'usage_recorded', 'hold_deficit', 'reconciliation_required', 'settled')),
  model_id text not null,
  model_name text not null,
  max_context_tokens_snapshot bigint not null check (max_context_tokens_snapshot > 0),
  max_output_tokens_snapshot bigint not null check (max_output_tokens_snapshot > 0),
  input_credits_per_million numeric(24,8) not null,
  output_credits_per_million numeric(24,8) not null,
  cached_input_credits_per_million numeric(24,8) not null,
  max_risk_credits numeric(24,8) not null check (max_risk_credits >= 0),
  actual_credits numeric(24,8),
  provider_request_id text,
  response_id text,
  response_model text,
  failure_stage text,
  reconciliation_reason text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  expires_at timestamptz not null,
  unique (run_id, operation, attempt),
  constraint ai_model_credit_holds_state_valid check (
    (state in ('held', 'released') and actual_credits is null)
    or (state in ('usage_recorded', 'hold_deficit', 'settled') and actual_credits is not null)
    or state = 'reconciliation_required'
  )
);
create index if not exists ai_model_credit_holds_owner_state_idx
  on ai.model_credit_holds(owner_user_id, state);
create index if not exists ai_model_credit_holds_expiry_idx
  on ai.model_credit_holds(expires_at) where state = 'held';

create table if not exists ai.model_usages (
  id text primary key,
  credit_hold_id text not null unique references ai.model_credit_holds(id) on delete restrict,
  run_id text not null references ai.runs(id) on delete cascade,
  owner_user_id text not null,
  operation text not null check (operation in ('assistant', 'summary', 'title')),
  attempt integer not null check (attempt > 0),
  status text not null check (status = 'reported'),
  settlement_status text not null check (settlement_status in ('pending', 'reconciliation_required', 'settled')),
  model_id text not null,
  model_name text not null,
  max_context_tokens_snapshot bigint not null check (max_context_tokens_snapshot > 0),
  prompt_tokens bigint not null check (prompt_tokens >= 0),
  completion_tokens bigint not null check (completion_tokens >= 0),
  total_tokens bigint not null check (total_tokens = prompt_tokens + completion_tokens),
  cached_prompt_tokens bigint,
  cache_write_prompt_tokens bigint,
  reasoning_completion_tokens bigint,
  provider_request_id text,
  response_id text,
  response_model text,
  finish_reason text,
  call_type text not null check (call_type in ('stream', 'complete')),
  official_details jsonb not null default '{}'::jsonb,
  occurred_at timestamptz not null default now(),
  settled_at timestamptz,
  constraint ai_model_usages_official_relationships_valid check (
    total_tokens = prompt_tokens + completion_tokens
    and (cached_prompt_tokens is null or cached_prompt_tokens <= prompt_tokens)
    and (cache_write_prompt_tokens is null or cache_write_prompt_tokens <= prompt_tokens)
    and (coalesce(cached_prompt_tokens, 0) + coalesce(cache_write_prompt_tokens, 0) <= prompt_tokens)
    and (reasoning_completion_tokens is null or reasoning_completion_tokens <= completion_tokens)
  ),
  unique (run_id, operation, attempt)
);
create index if not exists ai_model_usages_settlement_idx
  on ai.model_usages(settlement_status, occurred_at);
create index if not exists ai_model_usages_run_idx
  on ai.model_usages(run_id, operation, occurred_at desc);

create table if not exists ai.items (
  id text primary key,
  run_id text not null references ai.runs(id) on delete cascade,
  turn_id text not null references ai.turns(id) on delete cascade,
  timeline_index integer not null,
  type text not null,
  status text not null,
  content jsonb not null,
  revision bigint not null default 1,
  created_at timestamptz not null default now(),
  unique(run_id, timeline_index)
);

create table if not exists ai.run_events (
  id text primary key,
  run_id text not null references ai.runs(id) on delete cascade,
  event_sequence bigint not null,
  type text not null,
  data jsonb not null,
  created_at timestamptz not null default now(),
  unique(run_id, event_sequence)
);

create table if not exists ai.tool_calls (
  id text primary key,
  run_id text not null references ai.runs(id) on delete cascade,
  operation_id text not null,
  status text not null,
  input_mode text not null default 'model',
  arguments jsonb not null,
  arguments_ciphertext text,
  arguments_hash text not null default '',
  attempt integer not null default 1,
  row_version integer not null default 1,
  approval_decision text check (approval_decision in ('approve', 'approve_always')),
  result jsonb,
  error_code text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

alter table ai.tool_calls add column if not exists input_mode text not null default 'model';
create index if not exists tool_calls_run_created on ai.tool_calls(run_id, created_at);

create table if not exists ai.tool_approval_exemptions (
  user_id text not null,
  operation_id text not null,
  source_tool_call_id text not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  primary key(user_id, operation_id)
);

create table if not exists ai.ui_actions (
  id text primary key,
  run_id text not null references ai.runs(id) on delete cascade,
  tool_call_id text not null unique,
  client_instance_id text not null,
  action jsonb not null,
  status text not null default 'pending' check (status in ('pending', 'succeeded', 'failed', 'expired')),
  attempts integer not null default 1 check (attempts > 0),
  expires_at timestamptz not null,
  acknowledged_at timestamptz,
  actual_path text,
  error_code text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create index if not exists ui_actions_pending_client on ai.ui_actions(client_instance_id, created_at) where status = 'pending';

create table if not exists ai.conversation_summaries (
  conversation_id text primary key references ai.conversations(id) on delete cascade,
  covered_through_turn_index integer not null check (covered_through_turn_index >= 0),
  compression_version integer not null check (compression_version = 1),
  source_turn_count integer not null check (source_turn_count > 0),
  content jsonb not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create index if not exists conversation_summaries_updated on ai.conversation_summaries(updated_at desc);

create table if not exists ai.idempotency_keys (
  owner_user_id text not null,
  idempotency_key text not null,
  request_hash text not null,
  turn_id text not null references ai.turns(id) on delete cascade,
  run_id text not null references ai.runs(id) on delete cascade,
  created_at timestamptz not null default now(),
  primary key(owner_user_id, idempotency_key)
);
