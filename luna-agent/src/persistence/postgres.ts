import { createHash } from "node:crypto"
import { Pool, type PoolClient } from "pg"
import type { Conversation, CreatedTurn, CreateTurn, Run, TimelineItem } from "../domain.js"
import { createId } from "../id.js"
import type { Repository } from "./repository.js"
import { normalizeEventSequence } from "../event-sequence.js"

type DbConversation = { id: string, owner_user_id: string, project_id: string | null, title: string, status: "active", created_at: Date, updated_at: Date }
type DbRun = { id: string, owner_user_id: string, conversation_id: string, turn_id: string, run_index: number, status: Run["status"], row_version: number, graph_version: "assistant-v1", prompt_version: "system-v1", tool_catalog_digest: string, page_context: Record<string, unknown>, created_at: Date, started_at: Date | null, completed_at: Date | null, error_code: string | null }

export class PostgresRepository implements Repository {
  readonly pool: Pool
  constructor(connectionString: string) {
    this.pool = new Pool({ connectionString, max: 10, application_name: "luna-agent" })
  }
  async close(): Promise<void> { await this.pool.end() }
  async health(): Promise<boolean> {
    try { await this.pool.query("select 1"); return true } catch { return false }
  }
  async createConversation(ownerUserId: string, title: string, projectId?: string) {
    const row = (await this.pool.query<DbConversation>(
      `insert into ai.conversations(id, owner_user_id, project_id, title) values ($1,$2,$3,$4) returning *`,
      [createId("aicnv"), ownerUserId, projectId ?? null, title],
    )).rows[0]
    if (!row) throw new Error("ai.persistence_failed")
    return mapConversation(row)
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
    const row = (await this.pool.query<DbConversation>(`update ai.conversations set title=$3,updated_at=now() where id=$1 and owner_user_id=$2 returning *`, [id, ownerUserId, title])).rows[0]
    return row ? mapConversation(row) : undefined
  }
  async deleteConversation(ownerUserId: string, id: string) {
    return (await this.pool.query(`delete from ai.conversations where id=$1 and owner_user_id=$2`, [id, ownerUserId])).rowCount === 1
  }
  async createTurn(ownerUserId: string, input: CreateTurn): Promise<CreatedTurn> {
    const client = await this.pool.connect()
    try {
      await client.query("begin")
      const hash = createHash("sha256").update(JSON.stringify(input)).digest("hex")
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
        `insert into ai.runs(id,owner_user_id,conversation_id,turn_id,run_index,status,graph_version,prompt_version,tool_catalog_digest,page_context,run_actor_grant_ciphertext) values($1,$2,$3,$4,0,'queued','assistant-v1','system-v1','sha256:p0-readonly',$5,$6)`,
        [runId, ownerUserId, input.conversationId, turnId, JSON.stringify(input.pageContext), input.runActorGrantCiphertext ?? null],
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
    const run = await this.getRun(ownerUserId, id)
    if (!run || isTerminal(run.status)) return run
    return this.updateRun(id, run.status, "canceled", { completedAt: new Date().toISOString() })
  }
  async claimRun(instanceId: string, leaseSeconds: number) {
    const row = (await this.pool.query<DbRun>(`select r.* from ai.claim_next_run($1,$2) c join ai.runs r on r.id=c.run_id`, [instanceId, leaseSeconds])).rows[0]
    return row ? mapRun(row) : undefined
  }
  async getExecutionInput(runId: string) {
    const row = (await this.pool.query<{ input: string, page_context: Record<string, unknown> }>(
      `select t.input,r.page_context from ai.runs r join ai.turns t on t.id=r.turn_id where r.id=$1`, [runId],
    )).rows[0]
    if (!row) return undefined
    const results = await this.pool.query<{ content: unknown }>(`select content from ai.items where run_id=$1 and type='tool_result' order by timeline_index`, [runId])
    const run = await this.pool.query<{ turn_id: string }>(`select turn_id from ai.runs where id=$1`, [runId])
    return { turnId: run.rows[0]!.turn_id, input: row.input, pageContext: row.page_context, toolResults: results.rows.map(item => item.content) }
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
      await this.appendEventWith(client, runId, `run.${to}`, { state: to, rowVersion: row.row_version })
      await client.query("commit")
      return mapRun(row)
    } catch (error) { await client.query("rollback"); throw error } finally { client.release() }
  }
  async appendItem(value: Omit<TimelineItem, "id" | "timelineIndex" | "createdAt">) { return this.appendItemWith(this.pool, value) }
  async appendEvent(runId: string, type: string, data: Record<string, unknown>) { return this.appendEventWith(this.pool, runId, type, data) }
  async getEvents(ownerUserId: string, runId: string, after: number) {
    const result = await this.pool.query<{ id: string, run_id: string, event_sequence: number, type: string, data: Record<string, unknown>, created_at: Date }>(
      `select e.* from ai.run_events e join ai.runs r on r.id=e.run_id where e.run_id=$1 and r.owner_user_id=$2 and e.event_sequence>$3 order by e.event_sequence`, [runId, ownerUserId, after],
    )
    return result.rows.map(e => ({ id: e.id, runId: e.run_id, sequence: normalizeEventSequence(e.event_sequence), type: e.type, data: e.data, createdAt: e.created_at.toISOString() }))
  }
  async getTimeline(ownerUserId: string, conversationId: string) {
    const conversation = await this.getConversation(ownerUserId, conversationId)
    if (!conversation) return undefined
    const result = await this.pool.query<{ id: string, turn_index: number, status: string, input: string, selected_run_id: string }>(`select * from ai.turns where conversation_id=$1 order by turn_index`, [conversationId])
    const turns = await Promise.all(result.rows.map(async turn => {
      const run = await this.getRun(ownerUserId, turn.selected_run_id)
      const items = run ? await this.pool.query<{ id: string, run_id: string, turn_id: string, timeline_index: number, type: TimelineItem["type"], status: TimelineItem["status"], content: Record<string, unknown>, created_at: Date }>(`select * from ai.items where run_id=$1 order by timeline_index`, [run.id]) : undefined
      return { id: turn.id, turnIndex: turn.turn_index, status: turn.status, input: turn.input, ...(run ? { run } : {}), items: items?.rows.map(i => ({ id: i.id, runId: i.run_id, turnId: i.turn_id, timelineIndex: i.timeline_index, type: i.type, status: i.status, content: i.content, createdAt: i.created_at.toISOString() })) ?? [] }
    }))
    return { conversation, turns }
  }
  private async appendItemWith(client: Pick<PoolClient, "query"> | Pool, value: Omit<TimelineItem, "id" | "timelineIndex" | "createdAt">) {
    const row = (await client.query<{ id: string, run_id: string, turn_id: string, timeline_index: number, type: TimelineItem["type"], status: TimelineItem["status"], content: Record<string, unknown>, created_at: Date }>(
      `insert into ai.items(id,run_id,turn_id,timeline_index,type,status,content) select $1,$2,$3,coalesce(max(timeline_index)+1,0),$4,$5,$6 from ai.items where run_id=$2 returning *`,
      [createId("aiitm"), value.runId, value.turnId, value.type, value.status, JSON.stringify(value.content)],
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
  return { id: row.id, ownerUserId: row.owner_user_id, title: row.title, status: row.status, createdAt: row.created_at.toISOString(), updatedAt: row.updated_at.toISOString(), ...(row.project_id ? { projectId: row.project_id } : {}) }
}
function mapRun(row: DbRun): Run {
  return { id: row.id, conversationId: row.conversation_id, turnId: row.turn_id, runIndex: row.run_index, status: row.status, rowVersion: row.row_version, graphVersion: row.graph_version, promptVersion: row.prompt_version, toolCatalogDigest: row.tool_catalog_digest, pageContext: row.page_context, createdAt: row.created_at.toISOString(), ...(row.started_at ? { startedAt: row.started_at.toISOString() } : {}), ...(row.completed_at ? { completedAt: row.completed_at.toISOString() } : {}), ...(row.error_code ? { errorCode: row.error_code } : {}) }
}
function isTerminal(status: Run["status"]) { return ["completed", "failed", "canceled", "expired"].includes(status) }
