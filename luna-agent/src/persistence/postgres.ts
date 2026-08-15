import { and, asc, count, desc, eq, gt, gte, ilike, inArray, isNull, lt, ne, sql } from "drizzle-orm"
import type { Pool } from "pg"
import type {
  ConversationHistoryEntry,
  ConversationSummary,
  ConversationTitleSource,
  CreatedTurn,
  CreateTurn,
  Run,
  TimelineItem,
  UIActionAcknowledgement,
} from "../domain.js"
import { createId } from "../id.js"
import { internalSpanOptions, withSpan } from "../telemetry.js"
import {
  RunStateConflictError,
  type ConversationListOptions,
  type Repository,
  type TimelinePageOptions,
} from "./repository.js"
import { createTurnRequestHash } from "./create-turn-hash.js"
import { AgentDatabase, type AgentDb, type AgentTx } from "./database.js"
import {
  mapConversation,
  mapConversationSummary,
  mapRun,
  mapRunEvent,
  mapTimelineItem,
  mapUIAction,
  timelineContentText,
} from "./mappers/domain.js"
import {
  conversations,
  conversationSummaries,
  idempotencyKeys,
  items,
  runEvents,
  runs,
  turns,
  uiActions,
  type RunRow,
  type UIActionRow,
} from "./schema/index.js"

/** Drizzle 事务回调或共享 db 实例均可执行的最小查询接口 */
type Querier = AgentDb | AgentTx

const historyTurnWindow = 8

export class PostgresRepository implements Repository {
  private readonly database: AgentDatabase

  constructor(connectionString: string) {
    this.database = new AgentDatabase(connectionString)
  }

  /** 连接池仅供既有 ToolCallStore 过渡使用；新代码不得直接使用 */
  get pool(): Pool {
    return this.database.pool
  }

  private get db(): AgentDb {
    return this.database.db
  }

  async close(): Promise<void> {
    await this.database.close()
  }

  async health(): Promise<boolean> {
    return this.database.health()
  }

  async createConversation(ownerUserId: string, title: string, projectId?: string, titleSource?: ConversationTitleSource) {
    const row = (await this.db.insert(conversations).values({
      id: createId("aicnv"),
      ownerUserId,
      projectId: projectId ?? null,
      title,
      titleSource: titleSource ?? (title === "新会话" ? "default" : "user"),
    }).returning())[0]
    if (!row) throw new Error("ai.persistence_failed")
    return mapConversation(row)
  }

  async findEmptyConversation(ownerUserId: string, projectId?: string) {
    const emptyTurns = this.db.select({ id: turns.id }).from(turns).where(eq(turns.conversationId, conversations.id))
    const row = (await this.db.select().from(conversations)
      .where(and(
        eq(conversations.ownerUserId, ownerUserId),
        projectId === undefined ? isNull(conversations.projectId) : eq(conversations.projectId, projectId),
        sql`not exists ${emptyTurns}`,
      ))
      .orderBy(desc(conversations.updatedAt))
      .limit(1))[0]
    return row ? mapConversation(row) : undefined
  }

  async listConversations(ownerUserId: string, page: number, pageSize: number, options: ConversationListOptions = {}) {
    const search = options.search?.trim()
    const filters = [eq(conversations.ownerUserId, ownerUserId)]
    if (search) filters.push(ilike(conversations.title, `%${escapeLikePattern(search)}%`))
    const direction = options.sortOrder === "asc" ? asc : desc
    const [rows, total] = await Promise.all([
      this.db.select().from(conversations)
        .where(and(...filters))
        .orderBy(direction(conversations.updatedAt), direction(conversations.id))
        .limit(pageSize)
        .offset((page - 1) * pageSize),
      this.db.select({ value: count() }).from(conversations)
        .where(and(...filters)),
    ])
    return { items: rows.map(mapConversation), total: total[0]?.value ?? 0 }
  }

  async getConversation(ownerUserId: string, id: string) {
    const row = (await this.db.select().from(conversations)
      .where(and(eq(conversations.id, id), eq(conversations.ownerUserId, ownerUserId))))[0]
    return row ? mapConversation(row) : undefined
  }

  async renameConversation(ownerUserId: string, id: string, title: string) {
    const row = (await this.db.update(conversations)
      .set({ title, titleSource: "user", updatedAt: sql`now()` })
      .where(and(eq(conversations.id, id), eq(conversations.ownerUserId, ownerUserId)))
      .returning())[0]
    return row ? mapConversation(row) : undefined
  }

  async renameConversationByAssistant(id: string, title: string) {
    const row = (await this.db.update(conversations)
      .set({ title, titleSource: "assistant", updatedAt: sql`now()` })
      .where(and(eq(conversations.id, id), ne(conversations.titleSource, "user")))
      .returning())[0]
    return row ? mapConversation(row) : undefined
  }

  async deleteConversation(ownerUserId: string, id: string) {
    const deleted = await this.db.delete(conversations)
      .where(and(eq(conversations.id, id), eq(conversations.ownerUserId, ownerUserId)))
      .returning({ id: conversations.id })
    return deleted.length === 1
  }

  async createTurn(ownerUserId: string, input: CreateTurn): Promise<CreatedTurn> {
    return withSpan("agent.repository.turn.create", internalSpanOptions(), () => this.db.transaction(async (tx) => {
      const hash = createTurnRequestHash(input)
      const existing = (await tx.select().from(idempotencyKeys)
        .where(and(eq(idempotencyKeys.ownerUserId, ownerUserId), eq(idempotencyKeys.idempotencyKey, input.idempotencyKey))))[0]
      if (existing) {
        if (existing.requestHash !== hash) throw new Error("idempotency_conflict")
        return this.loadCreated(tx, existing.turnId, existing.runId)
      }
      const owned = (await tx.select({ id: conversations.id }).from(conversations)
        .where(and(eq(conversations.id, input.conversationId), eq(conversations.ownerUserId, ownerUserId)))
        .for("update"))[0]
      if (!owned) throw new Error("ai.conversation_not_found")
      const index = (await tx.select({ value: count() }).from(turns)
        .where(eq(turns.conversationId, input.conversationId)))[0]?.value ?? 0
      const turnId = createId("aitrn")
      const runId = input.preallocatedRunId ?? createId("airun")
      await tx.insert(turns).values({
        id: turnId,
        conversationId: input.conversationId,
        turnIndex: index,
        status: "queued",
        input: input.input,
        selectedRunId: runId,
      })
      await tx.insert(runs).values({
        id: runId,
        ownerUserId,
        conversationId: input.conversationId,
        turnId,
        runIndex: 0,
        status: "queued",
        promptVersion: "system-v4",
        toolCatalogDigest: input.toolCatalogDigest ?? "sha256:platform-tools-v1",
        pageContext: input.pageContext,
        traceContext: input.traceContext ?? {},
        runActorGrantCiphertext: input.runActorGrantCiphertext ?? null,
        clientInstanceId: input.clientInstanceId ?? null,
      })
      await tx.insert(idempotencyKeys).values({
        ownerUserId,
        idempotencyKey: input.idempotencyKey,
        requestHash: hash,
        turnId,
        runId,
      })
      await this.appendItemWith(tx, { runId, turnId, type: "user_message", status: "completed", content: { parts: [{ type: "text", text: input.input }] } })
      await this.appendEventWith(tx, runId, "run.queued", { state: "queued" })
      return this.loadCreated(tx, turnId, runId)
    }))
  }

  async getRun(ownerUserId: string, id: string) {
    const row = (await this.db.select().from(runs)
      .where(and(eq(runs.id, id), eq(runs.ownerUserId, ownerUserId))))[0]
    return row ? mapRun(row) : undefined
  }

  async cancelRun(ownerUserId: string, id: string) {
    return this.db.transaction(async (tx) => {
      const row = (await tx.update(runs)
        .set({ status: "canceled", rowVersion: sql`${runs.rowVersion} + 1`, completedAt: sql`now()` })
        .where(and(
          eq(runs.id, id),
          eq(runs.ownerUserId, ownerUserId),
          sql`${runs.status} not in ('completed', 'failed', 'canceled', 'expired')`,
        ))
        .returning())[0]
      if (!row) return undefined
      await tx.update(turns).set({ status: "canceled" }).where(eq(turns.id, row.turnId))
      await this.appendEventWith(tx, id, "run.canceled", { state: "canceled", rowVersion: row.rowVersion })
      return mapRun(row)
    }).then(async (run) => {
      if (run) return run
      // 未命中条件更新时读取权威当前状态，保持与旧实现一致的返回语义
      return this.getRun(ownerUserId, id)
    })
  }

  async claimRun(instanceId: string, leaseSeconds: number) {
    return withSpan("agent.repository.run.claim", internalSpanOptions(), async () => {
      // 原子领取依赖数据库函数 ai.claim_next_run（FOR UPDATE SKIP LOCKED），
      // 由 golang-migrate 迁移维护，无法以 Drizzle 查询安全替代
      const raw = (await this.db.execute<Record<string, unknown>>(sql`
        select r.* from ai.claim_next_run(${instanceId}, ${leaseSeconds}) c
        join ai.runs r on r.id = c.run_id
      `)).rows[0]
      return raw ? mapRun(driverRow(runs, raw) as RunRow) : undefined
    })
  }

  async countActiveUserRuns(userId: string) {
    const rows = await this.db.select({ count: sql<number>`count(*)::int` })
      .from(runs)
      .where(and(
        eq(runs.ownerUserId, userId),
        inArray(runs.status, ["queued", "running"]),
      ))
    return rows[0]?.count ?? 0
  }

  async getExecutionInput(runId: string) {
    const row = (await this.db.select({
      input: turns.input,
      turnId: turns.id,
      turnIndex: turns.turnIndex,
      conversationId: runs.conversationId,
      pageContext: runs.pageContext,
      title: conversations.title,
      titleSource: conversations.titleSource,
    })
      .from(runs)
      .innerJoin(turns, eq(turns.id, runs.turnId))
      .innerJoin(conversations, eq(conversations.id, runs.conversationId))
      .where(eq(runs.id, runId)))[0]
    if (!row) return undefined

    // 最近历史轮次的 lateral 子查询保留原有聚合语义（按轮聚合 assistant 文本）
    const [currentToolInteractions, historyRows, historyItems] = await Promise.all([
      this.db.select({ id: items.id, type: items.type, status: items.status, content: items.content })
        .from(items)
        .where(and(eq(items.runId, runId), inArray(items.type, ["tool_call", "tool_result"])))
        .orderBy(asc(items.timelineIndex)),
      this.db.execute<{ turn_index: number, input: string, assistant: string }>(sql`
        select recent.turn_index, recent.input,
               coalesce(string_agg(i.content->'parts'->0->>'text', E'\n' order by i.timeline_index)
                 filter (where i.type = 'assistant_message'), '') assistant
        from (
          select turn_index, input, selected_run_id
          from ai.turns
          where conversation_id = ${row.conversationId} and turn_index < ${row.turnIndex}
          order by turn_index desc
          limit ${historyTurnWindow}
        ) recent
        left join ai.items i on i.run_id = recent.selected_run_id
        group by recent.turn_index, recent.input
        order by recent.turn_index
      `),
      this.db.select({
        turnIndex: turns.turnIndex,
        type: items.type,
        status: items.status,
        content: items.content,
        timelineIndex: items.timelineIndex,
      })
        .from(turns)
        .innerJoin(items, eq(items.runId, turns.selectedRunId))
        .where(and(
          eq(turns.conversationId, row.conversationId),
          lt(turns.turnIndex, row.turnIndex),
          gte(turns.turnIndex, Math.max(0, row.turnIndex - historyTurnWindow)),
          inArray(items.type, ["tool_call", "tool_result"]),
        ))
        .orderBy(asc(turns.turnIndex), asc(items.timelineIndex)),
    ])

    const toolInteractionsByTurn = new Map<number, typeof historyItems>()
    for (const item of historyItems) {
      const interactions = toolInteractionsByTurn.get(item.turnIndex) ?? []
      interactions.push(item)
      toolInteractionsByTurn.set(item.turnIndex, interactions)
    }
    return {
      turnId: row.turnId,
      turnIndex: row.turnIndex,
      conversationId: row.conversationId,
      input: row.input,
      pageContext: row.pageContext,
      toolInteractions: currentToolInteractions.map(item => ({ itemId: item.id, type: item.type as "tool_call" | "tool_result", status: item.status, content: item.content })),
      history: historyRows.rows.map((item): ConversationHistoryEntry => ({
        turnIndex: item.turn_index,
        user: item.input,
        assistant: item.assistant,
        ...((toolInteractionsByTurn.get(item.turn_index)?.length ?? 0) > 0
          ? { toolInteractions: toolInteractionsByTurn.get(item.turn_index)!.map(tool => ({ type: tool.type, status: tool.status, content: tool.content })) }
          : {}),
      })),
      conversation: { title: row.title, titleSource: row.titleSource },
    }
  }

  async getConversationSummary(conversationId: string) {
    const row = (await this.db.select().from(conversationSummaries)
      .where(eq(conversationSummaries.conversationId, conversationId)))[0]
    return row ? mapConversationSummary(row) : undefined
  }

  async listConversationHistory(conversationId: string, afterTurnIndex: number, beforeTurnIndex: number, limit: number) {
    const boundedTurns = await this.db.select({
      turnIndex: turns.turnIndex,
      input: turns.input,
      selectedRunId: turns.selectedRunId,
    })
      .from(turns)
      .where(and(
        eq(turns.conversationId, conversationId),
        gt(turns.turnIndex, afterTurnIndex),
        lt(turns.turnIndex, beforeTurnIndex),
      ))
      .orderBy(asc(turns.turnIndex))
      .limit(Math.max(0, limit))
    if (boundedTurns.length === 0) return []
    const runIds = boundedTurns.map(turn => turn.selectedRunId)
    const turnItems = await this.db.select({
      runId: items.runId,
      type: items.type,
      status: items.status,
      content: items.content,
      timelineIndex: items.timelineIndex,
    })
      .from(items)
      .where(and(inArray(items.runId, runIds), inArray(items.type, ["assistant_message", "tool_call", "tool_result"])))
      .orderBy(asc(items.runId), asc(items.timelineIndex))
    const byRun = new Map<string, typeof turnItems>()
    for (const item of turnItems) {
      const values = byRun.get(item.runId) ?? []
      values.push(item)
      byRun.set(item.runId, values)
    }
    return boundedTurns.map((turn) => {
      const runItems = byRun.get(turn.selectedRunId) ?? []
      const assistant = runItems
        .filter(item => item.type === "assistant_message")
        .map(item => timelineContentText(item.content))
        .filter(Boolean)
        .join("\n")
      const toolInteractions = runItems
        .filter(item => item.type === "tool_call" || item.type === "tool_result")
        .map(item => ({ type: item.type, status: item.status, content: item.content }))
      return {
        turnIndex: turn.turnIndex,
        user: turn.input,
        assistant,
        ...(toolInteractions.length ? { toolInteractions } : {}),
      }
    })
  }

  async saveConversationSummary(input: Omit<ConversationSummary, "createdAt" | "updatedAt">) {
    // 单调推进的水位条件属于原子 upsert 语义，使用最小范围 sql 模板保留
    const row = (await this.db.insert(conversationSummaries).values({
      conversationId: input.conversationId,
      coveredThroughTurnIndex: input.coveredThroughTurnIndex,
      compressionVersion: input.compressionVersion,
      sourceTurnCount: input.sourceTurnCount,
      content: input.content,
    })
      .onConflictDoUpdate({
        target: conversationSummaries.conversationId,
        set: {
          coveredThroughTurnIndex: sql`excluded.covered_through_turn_index`,
          compressionVersion: sql`excluded.compression_version`,
          sourceTurnCount: sql`excluded.source_turn_count`,
          content: sql`excluded.content`,
          updatedAt: sql`now()`,
        },
        setWhere: sql`${conversationSummaries.coveredThroughTurnIndex} < excluded.covered_through_turn_index`,
      })
      .returning())[0]
    if (row) return mapConversationSummary(row)
    const current = await this.getConversationSummary(input.conversationId)
    if (!current) throw new Error("ai.context_summary_persistence_failed")
    return current
  }

  async getRunActorGrantCiphertext(runId: string) {
    const row = (await this.db.select({ value: runs.runActorGrantCiphertext }).from(runs)
      .where(eq(runs.id, runId)))[0]
    return row?.value ?? undefined
  }

  async appendRunInput(runId: string, text: string) {
    const updated = await this.db.update(turns)
      .set({ input: sql`${turns.input} || E'\n' || ${text}` })
      .from(runs)
      .where(and(eq(runs.id, runId), eq(turns.id, runs.turnId)))
      .returning({ id: turns.id })
    if (!updated.length) throw new Error("ai.run_not_found")
  }

  async renewLease(runId: string, instanceId: string, leaseSeconds: number) {
    // 租约续期条件由数据库函数保证原子性
    const row = (await this.db.execute<{ renewed: boolean | null }>(
      sql`select ai.renew_run_lease(${runId}, ${instanceId}, ${leaseSeconds}) renewed`,
    )).rows[0]
    return Boolean(row?.renewed)
  }

  async releaseLease(runId: string, instanceId: string) {
    await this.db.execute(sql`select ai.release_run_lease(${runId}, ${instanceId})`)
  }

  async updateRun(runId: string, from: Run["status"], to: Run["status"], fields: Partial<Run> = {}) {
    return withSpan("agent.repository.run.transition", internalSpanOptions({
      "luna.run.expected_status": from,
      "luna.run.target_status": to,
    }), async (span) => this.db.transaction(async (tx) => {
      const startedAt = typeof fields.startedAt === "string" ? fields.startedAt : null
      const completedAt = typeof fields.completedAt === "string" ? fields.completedAt : null
      const errorCode = typeof fields.errorCode === "string" ? fields.errorCode : null
      const row = (await tx.update(runs)
        .set({
          status: to,
          rowVersion: sql`${runs.rowVersion} + 1`,
          startedAt: sql`coalesce(${startedAt}::timestamptz, ${runs.startedAt})`,
          completedAt: sql`coalesce(${completedAt}::timestamptz, ${runs.completedAt})`,
          errorCode: sql`coalesce(${errorCode}, ${runs.errorCode})`,
        })
        .where(and(eq(runs.id, runId), eq(runs.status, from)))
        .returning())[0]
      if (!row) {
        // 条件更新未命中时读取权威当前状态后抛出冲突，禁止改成先查后改的竞态实现
        const current = (await tx.select({ status: runs.status }).from(runs).where(eq(runs.id, runId)))[0]
        const conflict = new RunStateConflictError(runId, from, to, current?.status)
        span.setAttribute("luna.run.actual_status", conflict.actualStatus ?? "missing")
        throw conflict
      }
      await tx.update(turns).set({ status: to }).where(eq(turns.id, row.turnId))
      await this.appendEventWith(tx, runId, `run.${to}`, {
        state: to,
        rowVersion: row.rowVersion,
        ...(fields.errorCode ? { errorCode: fields.errorCode } : {}),
      })
      return mapRun(row)
    }))
  }

  async appendItem(value: Omit<TimelineItem, "id" | "timelineIndex" | "revision" | "createdAt"> & { id?: string }) {
    return this.appendItemWith(this.db, value)
  }

  async updateItem(itemId: string, status: TimelineItem["status"], content: Record<string, unknown>) {
    return this.updateItemWith(this.db, itemId, status, content)
  }

  async appendItemWithEvent(
    value: Omit<TimelineItem, "id" | "timelineIndex" | "revision" | "createdAt"> & { id?: string },
    eventType: string,
    eventData: Record<string, unknown> = {},
  ) {
    return this.db.transaction(async (tx) => {
      const item = await this.appendItemWith(tx, value)
      const event = await this.appendEventWith(tx, item.runId, eventType, { ...eventData, item })
      return { item, event }
    })
  }

  async updateItemWithEvent(
    itemId: string,
    status: TimelineItem["status"],
    content: Record<string, unknown>,
    eventType: string,
    eventData: Record<string, unknown> = {},
  ) {
    return this.db.transaction(async (tx) => {
      const item = await this.updateItemWith(tx, itemId, status, content)
      const event = await this.appendEventWith(tx, item.runId, eventType, { ...eventData, item })
      return { item, event }
    })
  }

  async finalizeStreamingItems(runId: string, status: Exclude<TimelineItem["status"], "streaming">) {
    await this.db.transaction(async (tx) => {
      const streaming = await tx.select().from(items)
        .where(and(eq(items.runId, runId), eq(items.status, "streaming")))
        .orderBy(asc(items.timelineIndex))
        .for("update")
      for (const current of streaming) {
        const item = await this.updateItemWith(tx, current.id, status, current.content)
        await this.appendEventWith(tx, runId, "item.finalized", { item })
      }
    })
  }

  async appendEvent(runId: string, type: string, data: Record<string, unknown>) {
    return withSpan("agent.repository.event.append", internalSpanOptions(), () => this.appendEventWith(this.db, runId, type, data))
  }

  async getEvents(ownerUserId: string, runId: string, after: number) {
    const rows = await this.db.select({ event: runEvents })
      .from(runEvents)
      .innerJoin(runs, eq(runs.id, runEvents.runId))
      .where(and(
        eq(runEvents.runId, runId),
        eq(runs.ownerUserId, ownerUserId),
        gt(runEvents.eventSequence, after),
      ))
      .orderBy(asc(runEvents.eventSequence))
    return rows.map(row => mapRunEvent(row.event))
  }

  async createUIAction(runId: string, toolCallId: string, action: Record<string, unknown>, expiresAt: string) {
    // insert ... select 保证 run 存在且已绑定客户端实例，on conflict 提供幂等；
    // 该 PostgreSQL 专属形态无法以 ORM insert values 表达，保留最小 sql 模板
    const row = (await this.db.execute<Record<string, unknown>>(sql`
      insert into ai.ui_actions (id, run_id, tool_call_id, client_instance_id, action, status, attempts, expires_at)
      select ${createId("aiuia")}, r.id, ${toolCallId}, r.client_instance_id, ${JSON.stringify(action)}::jsonb, 'pending', 1, ${expiresAt}::timestamptz
      from ai.runs r
      where r.id = ${runId} and r.client_instance_id is not null
      on conflict (tool_call_id) do update set tool_call_id = excluded.tool_call_id
      returning *
    `)).rows[0]
    if (!row) throw new Error("ai.client_instance_unavailable")
    return mapUIAction(driverRow(uiActions, row) as UIActionRow)
  }

  async listPendingUIActions(ownerUserId: string, clientInstanceId: string) {
    // 先把本客户端实例已过期的待办置为 expired，再领取剩余待办并递增投递次数
    await this.db.update(uiActions)
      .set({ status: "expired", updatedAt: sql`now()` })
      .from(runs)
      .where(and(
        eq(uiActions.runId, runs.id),
        eq(runs.ownerUserId, ownerUserId),
        eq(uiActions.clientInstanceId, clientInstanceId),
        eq(uiActions.status, "pending"),
        sql`${uiActions.expiresAt} <= now()`,
      ))
    const delivered = await this.db.update(uiActions)
      .set({ attempts: sql`${uiActions.attempts} + 1`, updatedAt: sql`now()` })
      .from(runs)
      .where(and(
        eq(uiActions.runId, runs.id),
        eq(runs.ownerUserId, ownerUserId),
        eq(uiActions.clientInstanceId, clientInstanceId),
        eq(uiActions.status, "pending"),
        sql`${uiActions.expiresAt} > now()`,
      ))
      .returning()
    return delivered
      .sort((left, right) => left.createdAt.getTime() - right.createdAt.getTime())
      .map(mapUIAction)
  }

  async acknowledgeUIAction(ownerUserId: string, clientInstanceId: string, actionId: string, acknowledgement: UIActionAcknowledgement) {
    const row = (await this.db.update(uiActions)
      .set({
        status: acknowledgement.status,
        acknowledgedAt: sql`now()`,
        actualPath: acknowledgement.actualPath ?? null,
        errorCode: acknowledgement.errorCode ?? null,
        updatedAt: sql`now()`,
      })
      .from(runs)
      .where(and(
        eq(uiActions.id, actionId),
        eq(uiActions.runId, runs.id),
        eq(runs.ownerUserId, ownerUserId),
        eq(uiActions.clientInstanceId, clientInstanceId),
        eq(uiActions.status, "pending"),
        sql`${uiActions.expiresAt} > now()`,
      ))
      .returning({ action: uiActions }))[0]
    if (row) return mapUIAction(row.action)
    const existing = (await this.db.select({ action: uiActions })
      .from(uiActions)
      .innerJoin(runs, eq(runs.id, uiActions.runId))
      .where(and(
        eq(uiActions.id, actionId),
        eq(runs.ownerUserId, ownerUserId),
        eq(uiActions.clientInstanceId, clientInstanceId),
      )))[0]
    return existing ? mapUIAction(existing.action) : undefined
  }

  async getTimeline(ownerUserId: string, conversationId: string, options: TimelinePageOptions = {}) {
    const limit = Math.max(1, Math.min(100, Math.trunc(options.limit ?? 30)))
    // Items 与 next_event_sequence 必须来自同一个 MVCC 快照。否则并发提交的
    // item+event 可能只被 cursor 查询看到，SSE 从过新的 cursor 恢复后会永久漏掉该 item。
    return this.db.transaction(async (tx) => {
      const conversationRow = (await tx.select().from(conversations)
        .where(and(eq(conversations.id, conversationId), eq(conversations.ownerUserId, ownerUserId))))[0]
      if (!conversationRow) return undefined
      const conversation = mapConversation(conversationRow)
      const constraints = [eq(turns.conversationId, conversationId)]
      if (options.beforeTurnIndex !== undefined) constraints.push(lt(turns.turnIndex, options.beforeTurnIndex))
      const recentTurns = await tx.select().from(turns)
        .where(and(...constraints))
        .orderBy(desc(turns.turnIndex))
        .limit(limit + 1)
      const hasOlder = recentTurns.length > limit
      const boundedTurns = recentTurns.slice(0, limit).reverse()
      if (boundedTurns.length === 0) {
        return { conversation, turns: [], eventCursors: [], pageInfo: { hasOlder: false } }
      }

      const runIds = boundedTurns.map(turn => turn.selectedRunId)
      const runRows = await tx.select().from(runs)
        .where(and(inArray(runs.id, runIds), eq(runs.ownerUserId, ownerUserId)))
      const authorizedRunIds = runRows.map(run => run.id)
      const itemRows = authorizedRunIds.length === 0
        ? []
        : await tx.select().from(items)
            .where(inArray(items.runId, authorizedRunIds))
            .orderBy(asc(items.runId), asc(items.timelineIndex))
      const runById = new Map(runRows.map(row => [row.id, row]))
      const itemsByRun = new Map<string, TimelineItem[]>()
      for (const row of itemRows) {
        const values = itemsByRun.get(row.runId) ?? []
        values.push(mapTimelineItem(row))
        itemsByRun.set(row.runId, values)
      }
      const pageTurns = boundedTurns.map((turn) => {
        const runRow = runById.get(turn.selectedRunId)
        return {
          id: turn.id,
          turnIndex: turn.turnIndex,
          status: turn.status,
          input: turn.input,
          createdAt: turn.createdAt.toISOString(),
          ...(runRow ? { run: mapRun(runRow) } : {}),
          items: itemsByRun.get(turn.selectedRunId) ?? [],
        }
      })
      return {
        conversation,
        turns: pageTurns,
        eventCursors: boundedTurns.flatMap((turn) => {
          const runRow = runById.get(turn.selectedRunId)
          return runRow ? [{ runId: runRow.id, after: Math.max(0, runRow.nextEventSequence - 1) }] : []
        }),
        pageInfo: {
          hasOlder,
          ...(hasOlder && pageTurns[0] ? { oldestTurnIndex: pageTurns[0].turnIndex } : {}),
        },
      }
    }, { isolationLevel: "repeatable read", accessMode: "read only" })
  }

  /** 原子推进 next_item_position 并插入 Timeline Item；计数器原子更新无法以 ORM 查询安全表达，保留最小 sql 模板 */
  private async appendItemWith(db: Querier, value: Omit<TimelineItem, "id" | "timelineIndex" | "revision" | "createdAt"> & { id?: string }) {
    const position = (await db.update(runs)
      .set({ nextItemPosition: sql`${runs.nextItemPosition} + 1` })
      .where(eq(runs.id, value.runId))
      .returning({ position: sql<number>`${runs.nextItemPosition} - 1` }))[0]?.position
    if (position === undefined) throw new Error("ai.run_not_found")
    const row = (await db.insert(items).values({
      id: value.id ?? createId("aiitm"),
      runId: value.runId,
      turnId: value.turnId,
      timelineIndex: position,
      type: value.type,
      status: value.status,
      content: value.content,
    }).returning())[0]
    if (!row) throw new Error("ai.persistence_failed")
    if (value.type === "user_message" || value.type === "assistant_message") {
      await db.update(conversations)
        .set({ updatedAt: row.createdAt })
        .from(runs)
        .where(and(eq(runs.id, value.runId), eq(conversations.id, runs.conversationId)))
    }
    return mapTimelineItem(row)
  }

  private async updateItemWith(db: Querier, itemId: string, status: TimelineItem["status"], content: Record<string, unknown>) {
    const row = (await db.update(items)
      .set({ status, content, revision: sql`${items.revision} + 1` })
      .where(eq(items.id, itemId))
      .returning())[0]
    if (!row) throw new Error("ai.item_not_found")
    return mapTimelineItem(row)
  }

  /** 原子推进 next_event_sequence 并插入事件，保证单 Run 事件序列单调且唯一 */
  private async appendEventWith(db: Querier, runId: string, type: string, data: Record<string, unknown>) {
    const sequence = (await db.update(runs)
      .set({ nextEventSequence: sql`${runs.nextEventSequence} + 1` })
      .where(eq(runs.id, runId))
      .returning({ sequence: sql<number>`${runs.nextEventSequence} - 1` }))[0]?.sequence
    if (sequence === undefined) throw new Error("ai.run_not_found")
    const row = (await db.insert(runEvents).values({
      id: createId("aievt"),
      runId,
      eventSequence: sequence,
      type,
      data,
    }).returning())[0]
    if (!row) throw new Error("ai.persistence_failed")
    return mapRunEvent(row)
  }

  private async loadCreated(tx: AgentTx, turnId: string, runId: string): Promise<CreatedTurn> {
    const turn = (await tx.select().from(turns).where(eq(turns.id, turnId)))[0]
    const run = (await tx.select().from(runs).where(eq(runs.id, runId)))[0]
    if (!turn || !run) throw new Error("ai.persistence_failed")
    return {
      turn: {
        id: turn.id,
        conversationId: turn.conversationId,
        turnIndex: turn.turnIndex,
        status: turn.status,
        input: turn.input,
        selectedRunId: turn.selectedRunId,
        createdAt: turn.createdAt.toISOString(),
      },
      run: mapRun(run),
    }
  }
}

/**
 * `db.execute(sql...)` 返回驱动原始行（snake_case 列名、timestamptz/bigint 为字符串）。
 * 统一在此按表定义逐列执行 `mapFromDriverValue`，转换为 ORM 等价的 camelCase 行，
 * 保证时间、JSONB、bigint 等类型与 Query API 返回完全一致。
 */
function driverRow(
  table: object,
  row: Record<string, unknown>,
): Record<string, unknown> {
  const result: Record<string, unknown> = {}
  for (const [fieldName, column] of Object.entries(table)) {
    if (!column || typeof column !== "object") continue
    const candidate = column as { name?: unknown, mapFromDriverValue?: unknown }
    if (typeof candidate.name !== "string" || !(candidate.name in row)) continue
    const value = row[candidate.name]
    result[fieldName] = value === null || typeof candidate.mapFromDriverValue !== "function"
      ? value
      : (candidate.mapFromDriverValue as (input: unknown) => unknown)(value)
  }
  return result
}

function escapeLikePattern(value: string): string {
  return value.replaceAll("\\", "\\\\").replaceAll("%", "\\%").replaceAll("_", "\\_")
}
