import { Pool, type PoolClient } from "pg"
import type {
  Conversation,
  ConversationHistoryEntry,
  ConversationTitleSource,
  CreatedTurn,
  CreateTurn,
  Run,
  TimelineItem,
  UIActionAcknowledgement,
  UIActionDelivery,
  UIActionStatus,
} from "../domain.js"
import { createId } from "../id.js"
import type { Repository } from "./repository.js"
import { createTurnRequestHash } from "./create-turn-hash.js"
import { normalizeEventSequence } from "../event-sequence.js"

type DbConversation = { id: string, owner_user_id: string, project_id: string | null, title: string, title_source: ConversationTitleSource, status: "active", created_at: Date, updated_at: Date }
type DbRun = { id: string, owner_user_id: string, conversation_id: string, turn_id: string, run_index: number, status: Run["status"], row_version: number, graph_version: "assistant-v1", prompt_version: Run["promptVersion"], tool_catalog_digest: string, page_context: Record<string, unknown>, trace_context: Record<string, string>, client_instance_id: string | null, created_at: Date, started_at: Date | null, completed_at: Date | null, error_code: string | null }
type DbUIAction = { id: string, run_id: string, tool_call_id: string, client_instance_id: string, action: Record<string, unknown>, status: UIActionStatus, attempts: number, expires_at: Date, acknowledged_at: Date | null, actual_path: string | null, error_code: string | null, created_at: Date, updated_at: Date }

export class PostgresRepository implements Repository {
  readonly pool: Pool
  constructor(connectionString: string) {
    this.pool = new Pool({ connectionString, max: 10, application_name: "luna-agent" })
  }
  async close(): Promise<void> { await this.pool.end() }
  async health(): Promise<boolean> {
    try { await this.pool.query("select 1"); return true } catch { return false }
  }
  async createConversation(ownerUserId: string, title: string, projectId?: string, titleSource?: ConversationTitleSource) {
    const row = (await this.pool.query<DbConversation>(
      `insert into ai.conversations(id, owner_user_id, project_id, title, title_source) values ($1,$2,$3,$4,$5) returning *`,
      [createId("aicnv"), ownerUserId, projectId ?? null, title, titleSource ?? (title === "新会话" ? "default" : "user")],
    )).rows[0]
    if (!row) throw new Error("ai.persistence_failed")
    return mapConversation(row)
  }
  async findEmptyConversation(ownerUserId: string, projectId?: string) {
    const row = (await this.pool.query<DbConversation>(
      `select c.* from ai.conversations c
       where c.owner_user_id=$1 and c.project_id is not distinct from $2
         and not exists (select 1 from ai.turns t where t.conversation_id=c.id)
       order by c.updated_at desc limit 1`,
      [ownerUserId, projectId ?? null],
    )).rows[0]
    return row ? mapConversation(row) : undefined
  }
  async listConversations(ownerUserId: string, page: number, pageSize: number) {
    const [rows, count] = await Promise.all([
      this.pool.query<DbConversation>(`select * from ai.conversations where owner_user_id=$1 order by updated_at desc limit $2 offset $3`, [ownerUserId, pageSize, (page - 1) * pageSize]),
      this.pool.query<{ count: string }>(`select count(*) from ai.conversations where owner_user_id=$1`, [ownerUserId]),
    ])
    return { items: rows.rows.map(mapConversation), total: Number(count.rows[0]?.count ?? 0) }
  }
  async getConversation(ownerUserId: string, id: string) {
    const row = (await this.pool.query<DbConversation>(`select * from ai.conversations where id=$1 and owner_user_id=$2`, [id, ownerUserId])).rows[0]
    return row ? mapConversation(row) : undefined
  }
  async renameConversation(ownerUserId: string, id: string, title: string) {
    const row = (await this.pool.query<DbConversation>(
      `update ai.conversations set title=$3,title_source='user',updated_at=now() where id=$1 and owner_user_id=$2 returning *`,
      [id, ownerUserId, title],
    )).rows[0]
    return row ? mapConversation(row) : undefined
  }
  async renameConversationByAssistant(id: string, title: string) {
    const row = (await this.pool.query<DbConversation>(
      `update ai.conversations set title=$2,title_source='assistant',updated_at=now()
       where id=$1 and title_source <> 'user' returning *`,
      [id, title],
    )).rows[0]
    return row ? mapConversation(row) : undefined
  }
  async deleteConversation(ownerUserId: string, id: string) {
    return (await this.pool.query(`delete from ai.conversations where id=$1 and owner_user_id=$2`, [id, ownerUserId])).rowCount === 1
  }
  async createTurn(ownerUserId: string, input: CreateTurn): Promise<CreatedTurn> {
    const client = await this.pool.connect()
    try {
      await client.query("begin")
      const hash = createTurnRequestHash(input)
      const existing = await client.query<{ request_hash: string, turn_id: string, run_id: string }>(
        `select request_hash,turn_id,run_id from ai.idempotency_keys where owner_user_id=$1 and idempotency_key=$2`,
        [ownerUserId, input.idempotencyKey],
      )
      if (existing.rows[0]) {
        if (existing.rows[0].request_hash !== hash) throw new Error("idempotency_conflict")
        const created = await this.loadCreated(client, existing.rows[0].turn_id, existing.rows[0].run_id)
        await client.query("commit")
        return created
      }
      const owned = await client.query(`select 1 from ai.conversations where id=$1 and owner_user_id=$2 for update`, [input.conversationId, ownerUserId])
      if (!owned.rowCount) throw new Error("ai.conversation_not_found")
      const index = Number((await client.query<{ count: string }>(`select count(*) from ai.turns where conversation_id=$1`, [input.conversationId])).rows[0]?.count ?? 0)
      const turnId = createId("aitrn")
      const runId = input.preallocatedRunId ?? createId("airun")
      await client.query(`insert into ai.turns(id,conversation_id,turn_index,status,input,selected_run_id) values($1,$2,$3,'queued',$4,$5)`, [turnId, input.conversationId, index, input.input, runId])
      await client.query(
        `insert into ai.runs(id,owner_user_id,conversation_id,turn_id,run_index,status,graph_version,prompt_version,tool_catalog_digest,page_context,trace_context,run_actor_grant_ciphertext,client_instance_id) values($1,$2,$3,$4,0,'queued','assistant-v1','system-v4',$5,$6,$7,$8,$9)`,
        [runId, ownerUserId, input.conversationId, turnId, input.toolCatalogDigest ?? "sha256:platform-tools-v1", JSON.stringify(input.pageContext), JSON.stringify(input.traceContext ?? {}), input.runActorGrantCiphertext ?? null, input.clientInstanceId ?? null],
      )
      await client.query(`insert into ai.idempotency_keys(owner_user_id,idempotency_key,request_hash,turn_id,run_id) values($1,$2,$3,$4,$5)`, [ownerUserId, input.idempotencyKey, hash, turnId, runId])
      await this.appendItemWith(client, { runId, turnId, type: "user_message", status: "completed", content: { parts: [{ type: "text", text: input.input }] } })
      await this.appendEventWith(client, runId, "run.queued", { state: "queued" })
      const created = await this.loadCreated(client, turnId, runId)
      await client.query("commit")
      return created
    } catch (error) {
      await client.query("rollback")
      throw error
    } finally { client.release() }
  }
  async getRun(ownerUserId: string, id: string) {
    const row = (await this.pool.query<DbRun>(`select * from ai.runs where id=$1 and owner_user_id=$2`, [id, ownerUserId])).rows[0]
    return row ? mapRun(row) : undefined
  }
  async cancelRun(ownerUserId: string, id: string) {
    const client = await this.pool.connect()
    try {
      await client.query("begin")
      const row = (await client.query<DbRun>(
        `update ai.runs set status='canceled',row_version=row_version+1,completed_at=now()
         where id=$1 and owner_user_id=$2 and status not in ('completed','failed','canceled','expired') returning *`,
        [id, ownerUserId],
      )).rows[0]
      if (!row) {
        await client.query("rollback")
        return this.getRun(ownerUserId, id)
      }
      await client.query(`update ai.turns set status='canceled' where id=$1`, [row.turn_id])
      await this.appendEventWith(client, id, "run.canceled", { state: "canceled", rowVersion: row.row_version })
      await client.query("commit")
      return mapRun(row)
    } catch (error) {
      await client.query("rollback")
      throw error
    } finally {
      client.release()
    }
  }
  async claimRun(instanceId: string, leaseSeconds: number) {
    const row = (await this.pool.query<DbRun>(`select r.* from ai.claim_next_run($1,$2) c join ai.runs r on r.id=c.run_id`, [instanceId, leaseSeconds])).rows[0]
    return row ? mapRun(row) : undefined
  }
  async getExecutionInput(runId: string) {
    const row = (await this.pool.query<{
      input: string
      page_context: Record<string, unknown>
      turn_id: string
      turn_index: number
      conversation_id: string
      title: string
      title_source: ConversationTitleSource
    }>(
      `select t.input,t.id as turn_id,t.turn_index,r.conversation_id,r.page_context,c.title,c.title_source
       from ai.runs r
       join ai.turns t on t.id=r.turn_id
       join ai.conversations c on c.id=r.conversation_id
       where r.id=$1`,
      [runId],
    )).rows[0]
    if (!row) return undefined
    const [results, historyRows] = await Promise.all([
      this.pool.query<{ content: unknown }>(`select content from ai.items where run_id=$1 and type='tool_result' order by timeline_index`, [runId]),
      this.pool.query<{ turn_index: number, input: string, assistant: string }>(
        `select recent.turn_index,recent.input,
                coalesce(string_agg(i.content->'parts'->0->>'text', E'\n' order by i.timeline_index)
                  filter (where i.type='assistant_message'), '') assistant
         from (
           select turn_index,input,selected_run_id
           from ai.turns
           where conversation_id=$1 and turn_index<$2
           order by turn_index desc
           limit 6
         ) recent
         left join ai.items i on i.run_id=recent.selected_run_id
         group by recent.turn_index,recent.input
         order by recent.turn_index`,
        [row.conversation_id, row.turn_index],
      ),
    ])
    return {
      turnId: row.turn_id,
      turnIndex: row.turn_index,
      input: row.input,
      pageContext: row.page_context,
      toolResults: results.rows.map(item => item.content),
      history: historyRows.rows.map((item): ConversationHistoryEntry => ({
        turnIndex: item.turn_index,
        user: truncateHistoryText(item.input, 2000),
        assistant: truncateHistoryText(item.assistant, 4000),
      })),
      conversation: { title: row.title, titleSource: row.title_source },
    }
  }
  async getRunActorGrantCiphertext(runId: string) {
    return (await this.pool.query<{ run_actor_grant_ciphertext: string | null }>(`select run_actor_grant_ciphertext from ai.runs where id=$1`, [runId])).rows[0]?.run_actor_grant_ciphertext ?? undefined
  }
  async appendRunInput(runId: string, text: string) {
    const result = await this.pool.query(`update ai.turns t set input=t.input || E'\\n' || $2 from ai.runs r where r.id=$1 and t.id=r.turn_id`, [runId, text])
    if (!result.rowCount) throw new Error("ai.run_not_found")
  }
  async renewLease(runId: string, instanceId: string, leaseSeconds: number) {
    return Boolean((await this.pool.query<{ renewed: boolean }>(`select ai.renew_run_lease($1,$2,$3) renewed`, [runId, instanceId, leaseSeconds])).rows[0]?.renewed)
  }
  async releaseLease(runId: string, instanceId: string) { await this.pool.query(`select ai.release_run_lease($1,$2)`, [runId, instanceId]) }
  async updateRun(runId: string, from: Run["status"], to: Run["status"], fields: Partial<Run> = {}) {
    const client = await this.pool.connect()
    try {
      await client.query("begin")
      const row = (await client.query<DbRun>(
        `update ai.runs set status=$3,row_version=row_version+1,started_at=coalesce($4,started_at),completed_at=coalesce($5,completed_at),error_code=coalesce($6,error_code) where id=$1 and status=$2 returning *`,
        [runId, from, to, fields.startedAt ?? null, fields.completedAt ?? null, fields.errorCode ?? null],
      )).rows[0]
      if (!row) throw new Error("ai.run_state_conflict")
      await client.query(`update ai.turns set status=$2 where id=$1`, [row.turn_id, to])
      await this.appendEventWith(client, runId, `run.${to}`, {
        state: to,
        rowVersion: row.row_version,
        ...(fields.errorCode ? { errorCode: fields.errorCode } : {}),
      })
      await client.query("commit")
      return mapRun(row)
    } catch (error) { await client.query("rollback"); throw error } finally { client.release() }
  }
  async appendItem(value: Omit<TimelineItem, "id" | "timelineIndex" | "createdAt"> & { id?: string }) { return this.appendItemWith(this.pool, value) }
  async updateItem(itemId: string, status: TimelineItem["status"], content: Record<string, unknown>) {
    const row = (await this.pool.query<{ id: string, run_id: string, turn_id: string, timeline_index: number, type: TimelineItem["type"], status: TimelineItem["status"], content: Record<string, unknown>, created_at: Date }>(
      `update ai.items set status=$2,content=$3 where id=$1 returning *`,
      [itemId, status, JSON.stringify(content)],
    )).rows[0]
    if (!row) throw new Error("ai.item_not_found")
    return { id: row.id, runId: row.run_id, turnId: row.turn_id, timelineIndex: row.timeline_index, type: row.type, status: row.status, content: row.content, createdAt: row.created_at.toISOString() }
  }
  async finalizeStreamingItems(runId: string, status: Exclude<TimelineItem["status"], "streaming">) {
    await this.pool.query(`update ai.items set status=$2 where run_id=$1 and status='streaming'`, [runId, status])
  }
  async appendEvent(runId: string, type: string, data: Record<string, unknown>) { return this.appendEventWith(this.pool, runId, type, data) }
  async getEvents(ownerUserId: string, runId: string, after: number) {
    const result = await this.pool.query<{ id: string, run_id: string, event_sequence: number, type: string, data: Record<string, unknown>, created_at: Date }>(
      `select e.* from ai.run_events e join ai.runs r on r.id=e.run_id where e.run_id=$1 and r.owner_user_id=$2 and e.event_sequence>$3 order by e.event_sequence`, [runId, ownerUserId, after],
    )
    return result.rows.map(e => ({ id: e.id, runId: e.run_id, sequence: normalizeEventSequence(e.event_sequence), type: e.type, data: e.data, createdAt: e.created_at.toISOString() }))
  }
  async createUIAction(runId: string, toolCallId: string, action: Record<string, unknown>, expiresAt: string) {
    const row = (await this.pool.query<DbUIAction>(
      `insert into ai.ui_actions(id,run_id,tool_call_id,client_instance_id,action,status,attempts,expires_at)
       select $1,r.id,$2,r.client_instance_id,$3,'pending',1,$4
       from ai.runs r
       where r.id=$5 and r.client_instance_id is not null
       on conflict (tool_call_id) do update set tool_call_id=excluded.tool_call_id
       returning *`,
      [createId("aiuia"), toolCallId, JSON.stringify(action), expiresAt, runId],
    )).rows[0]
    if (!row) throw new Error("ai.client_instance_unavailable")
    return mapUIAction(row)
  }
  async listPendingUIActions(ownerUserId: string, clientInstanceId: string) {
    await this.pool.query(
      `update ai.ui_actions a set status='expired',updated_at=now()
       from ai.runs r
       where a.run_id=r.id and r.owner_user_id=$1 and a.client_instance_id=$2
         and a.status='pending' and a.expires_at <= now()`,
      [ownerUserId, clientInstanceId],
    )
    const result = await this.pool.query<DbUIAction>(
      `update ai.ui_actions a set attempts=a.attempts+1,updated_at=now()
       from ai.runs r
       where a.run_id=r.id and r.owner_user_id=$1 and a.client_instance_id=$2
         and a.status='pending' and a.expires_at > now()
       returning a.*`,
      [ownerUserId, clientInstanceId],
    )
    return result.rows.sort((left, right) => left.created_at.getTime() - right.created_at.getTime()).map(mapUIAction)
  }
  async acknowledgeUIAction(ownerUserId: string, clientInstanceId: string, actionId: string, acknowledgement: UIActionAcknowledgement) {
    const row = (await this.pool.query<DbUIAction>(
      `update ai.ui_actions a
       set status=$4,acknowledged_at=now(),actual_path=$5,error_code=$6,updated_at=now()
       from ai.runs r
       where a.id=$1 and a.run_id=r.id and r.owner_user_id=$2 and a.client_instance_id=$3
         and a.status='pending' and a.expires_at > now()
       returning a.*`,
      [actionId, ownerUserId, clientInstanceId, acknowledgement.status, acknowledgement.actualPath ?? null, acknowledgement.errorCode ?? null],
    )).rows[0]
    if (row) return mapUIAction(row)
    const existing = (await this.pool.query<DbUIAction>(
      `select a.* from ai.ui_actions a join ai.runs r on r.id=a.run_id
       where a.id=$1 and r.owner_user_id=$2 and a.client_instance_id=$3`,
      [actionId, ownerUserId, clientInstanceId],
    )).rows[0]
    return existing ? mapUIAction(existing) : undefined
  }
  async getTimeline(ownerUserId: string, conversationId: string) {
    const conversation = await this.getConversation(ownerUserId, conversationId)
    if (!conversation) return undefined
    const result = await this.pool.query<{ id: string, turn_index: number, status: string, input: string, selected_run_id: string, created_at: Date }>(`select * from ai.turns where conversation_id=$1 order by turn_index`, [conversationId])
    const turns = await Promise.all(result.rows.map(async turn => {
      const run = await this.getRun(ownerUserId, turn.selected_run_id)
      const items = run ? await this.pool.query<{ id: string, run_id: string, turn_id: string, timeline_index: number, type: TimelineItem["type"], status: TimelineItem["status"], content: Record<string, unknown>, created_at: Date }>(`select * from ai.items where run_id=$1 order by timeline_index`, [run.id]) : undefined
      return { id: turn.id, turnIndex: turn.turn_index, status: turn.status, input: turn.input, createdAt: turn.created_at.toISOString(), ...(run ? { run } : {}), items: items?.rows.map(i => ({ id: i.id, runId: i.run_id, turnId: i.turn_id, timelineIndex: i.timeline_index, type: i.type, status: i.status, content: i.content, createdAt: i.created_at.toISOString() })) ?? [] }
    }))
    return { conversation, turns }
  }
  private async appendItemWith(client: Pick<PoolClient, "query"> | Pool, value: Omit<TimelineItem, "id" | "timelineIndex" | "createdAt"> & { id?: string }) {
    const row = (await client.query<{ id: string, run_id: string, turn_id: string, timeline_index: number, type: TimelineItem["type"], status: TimelineItem["status"], content: Record<string, unknown>, created_at: Date }>(
      `insert into ai.items(id,run_id,turn_id,timeline_index,type,status,content) select $1,$2,$3,coalesce(max(timeline_index)+1,0),$4,$5,$6 from ai.items where run_id=$2 returning *`,
      [value.id ?? createId("aiitm"), value.runId, value.turnId, value.type, value.status, JSON.stringify(value.content)],
    )).rows[0]
    if (!row) throw new Error("ai.persistence_failed")
    return { id: row.id, runId: row.run_id, turnId: row.turn_id, timelineIndex: row.timeline_index, type: row.type, status: row.status, content: row.content, createdAt: row.created_at.toISOString() }
  }
  private async appendEventWith(client: Pick<PoolClient, "query"> | Pool, runId: string, type: string, data: Record<string, unknown>) {
    const row = (await client.query<{ id: string, run_id: string, event_sequence: number, type: string, data: Record<string, unknown>, created_at: Date }>(
      `insert into ai.run_events(id,run_id,event_sequence,type,data) select $1,$2,coalesce(max(event_sequence)+1,1),$3,$4 from ai.run_events where run_id=$2 returning *`,
      [createId("aievt"), runId, type, JSON.stringify(data)],
    )).rows[0]
    if (!row) throw new Error("ai.persistence_failed")
    return { id: row.id, runId: row.run_id, sequence: normalizeEventSequence(row.event_sequence), type: row.type, data: row.data, createdAt: row.created_at.toISOString() }
  }
  private async loadCreated(client: PoolClient, turnId: string, runId: string): Promise<CreatedTurn> {
    const turn = (await client.query<{ id: string, conversation_id: string, turn_index: number, status: Run["status"], input: string, selected_run_id: string, created_at: Date }>(`select * from ai.turns where id=$1`, [turnId])).rows[0]
    const run = (await client.query<DbRun>(`select * from ai.runs where id=$1`, [runId])).rows[0]
    if (!turn || !run) throw new Error("ai.persistence_failed")
    return { turn: { id: turn.id, conversationId: turn.conversation_id, turnIndex: turn.turn_index, status: turn.status, input: turn.input, selectedRunId: turn.selected_run_id, createdAt: turn.created_at.toISOString() }, run: mapRun(run) }
  }
}

function mapConversation(row: DbConversation): Conversation {
  return {
    id: row.id,
    ownerUserId: row.owner_user_id,
    title: row.title,
    titleSource: row.title_source,
    status: row.status,
    createdAt: row.created_at.toISOString(),
    updatedAt: row.updated_at.toISOString(),
    ...(row.project_id ? { projectId: row.project_id } : {}),
  }
}
function mapRun(row: DbRun): Run {
  return { id: row.id, conversationId: row.conversation_id, turnId: row.turn_id, runIndex: row.run_index, status: row.status, rowVersion: row.row_version, graphVersion: row.graph_version, promptVersion: row.prompt_version, toolCatalogDigest: row.tool_catalog_digest, pageContext: row.page_context, ...(Object.keys(row.trace_context ?? {}).length ? { traceContext: row.trace_context } : {}), createdAt: row.created_at.toISOString(), ...(row.client_instance_id ? { clientInstanceId: row.client_instance_id } : {}), ...(row.started_at ? { startedAt: row.started_at.toISOString() } : {}), ...(row.completed_at ? { completedAt: row.completed_at.toISOString() } : {}), ...(row.error_code ? { errorCode: row.error_code } : {}) }
}
function mapUIAction(row: DbUIAction): UIActionDelivery {
  return {
    id: row.id,
    runId: row.run_id,
    toolCallId: row.tool_call_id,
    clientInstanceId: row.client_instance_id,
    action: row.action,
    status: row.status,
    attempts: row.attempts,
    expiresAt: row.expires_at.toISOString(),
    ...(row.acknowledged_at ? { acknowledgedAt: row.acknowledged_at.toISOString() } : {}),
    ...(row.actual_path ? { actualPath: row.actual_path } : {}),
    ...(row.error_code ? { errorCode: row.error_code } : {}),
    createdAt: row.created_at.toISOString(),
    updatedAt: row.updated_at.toISOString(),
  }
}

function truncateHistoryText(value: string, maxLength: number) {
  return [...value].slice(0, maxLength).join("")
}
