-- Reference schema for local development and Agent repository tests.
-- Production installation remains owned by the platform golang-migrate job.
create schema if not exists ai;

create table if not exists ai.conversations (
  id text primary key,
  owner_user_id text not null,
  project_id text,
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
update ai.conversations set title_source = 'user' where title <> '新会话' and title_source = 'default';

create table if not exists ai.turns (
  id text primary key,
  conversation_id text not null references ai.conversations(id) on delete cascade,
  turn_index integer not null,
  status text not null,
  input text not null,
  selected_run_id text not null,
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
  graph_version text not null,
  prompt_version text not null,
  tool_catalog_digest text not null,
  page_context jsonb not null default '{}',
  run_actor_grant_ciphertext text,
  lease_owner text,
  lease_expires_at timestamptz,
  heartbeat_at timestamptz,
  created_at timestamptz not null default now(),
  started_at timestamptz,
  completed_at timestamptz,
  error_code text,
  client_instance_id text,
  trace_context jsonb not null default '{}'::jsonb
    check (jsonb_typeof(trace_context) = 'object'),
  next_item_position bigint not null default 0,
  next_event_sequence bigint not null default 1,
  unique(turn_id, run_index)
);
create index if not exists runs_queue on ai.runs(status, lease_expires_at) where status = 'queued';

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
  arguments jsonb not null,
  arguments_ciphertext text,
  arguments_hash text not null,
  attempt integer not null default 1,
  row_version integer not null default 1,
  approval_expires_at timestamptz,
  mfa_purpose text,
  result jsonb,
  error_code text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create index if not exists tool_calls_run_created on ai.tool_calls(run_id, created_at);

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

create or replace function ai.claim_next_run(instance_id text, lease_seconds integer)
returns table(run_id text, owner_user_id text, lease_expires_at timestamptz)
language plpgsql security definer set search_path = pg_catalog, ai as $$
begin
  if length(instance_id) < 1 or length(instance_id) > 128 or lease_seconds < 5 or lease_seconds > 300 then
    raise exception 'invalid lease request';
  end if;
  return query
  with candidate as (
    select id from ai.runs
    where status='queued' and (ai.runs.lease_expires_at is null or ai.runs.lease_expires_at <= now())
    order by created_at for update skip locked limit 1
  )
  update ai.runs r set lease_owner=instance_id,
    lease_expires_at=now()+make_interval(secs=>lease_seconds), heartbeat_at=now()
  from candidate where r.id=candidate.id
  returning r.id,r.owner_user_id,r.lease_expires_at;
end $$;

create or replace function ai.renew_run_lease(p_run_id text, instance_id text, lease_seconds integer)
returns boolean language sql security definer set search_path = pg_catalog, ai as $$
  update ai.runs set lease_expires_at=now()+make_interval(secs=>lease_seconds), heartbeat_at=now()
  where id=p_run_id and lease_owner=instance_id and status in ('queued','running')
  returning true
$$;

create or replace function ai.release_run_lease(p_run_id text, instance_id text)
returns boolean language sql security definer set search_path = pg_catalog, ai as $$
  update ai.runs set lease_owner=null,lease_expires_at=null
  where id=p_run_id and lease_owner=instance_id returning true
$$;
