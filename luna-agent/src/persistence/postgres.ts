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
import type { OfficialModelUsage, UsageUnavailableReason } from "../provider/provider.js"
import { agentMetrics, internalSpanOptions, withSpan } from "../telemetry.js"
import {
  RunStateConflictError,
  type ConversationListOptions,
  type Repository,
  type ModelAttemptMetadata,
  type ModelCallOperation,
  type TimelinePageOptions,
  type RunStreamBatch,
} from "./repository.js"
import { createTurnRequestHash } from "./create-turn-hash.js"
import { AgentDatabase, type AgentDatabaseOptions, type AgentDb, type AgentTx } from "./database.js"
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

  constructor(connectionString: string, options: AgentDatabaseOptions = {}) {
    this.database = new AgentDatabase(connectionString, options)
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

  async readiness() {
    return this.database.readiness()
  }

  async assertReady(): Promise<void> {
    await this.database.assertReady()
  }

  async createModelCreditHold(input: {
    id: string
    runId: string
    ownerUserId: string
    operation: ModelCallOperation
    requestedOutputTokens: number
    leaseSeconds: number
  }) {
    if (!Number.isSafeInteger(input.requestedOutputTokens) || input.requestedOutputTokens < 1
      || !Number.isSafeInteger(input.leaseSeconds) || input.leaseSeconds < 5 || input.leaseSeconds > 10_800)
      throw new Error("ai.model_output_limit_invalid")
    return withSpan("agent.credit_hold.create", internalSpanOptions({
      "luna.credit_hold.operation": input.operation,
    }), () => this.db.transaction(async tx => {
      const wallet = (await tx.execute<Record<string, unknown>>(sql`
        select balance_credits::numeric as balance_credits
        from user_wallets where user_id = ${input.ownerUserId} for update
      `)).rows[0]
      if (!wallet) throw new Error("ai.wallet_balance_insufficient")
      const run = (await tx.execute<Record<string, unknown>>(sql`
        select id, owner_user_id,
               coalesce(max_context_tokens, 524288)::bigint as max_context_tokens,
               coalesce(max_output_tokens, 65536)::bigint as max_output_tokens,
               coalesce(model_id, 'legacy') as model_id,
               coalesce(model_name, 'legacy') as model_name,
               coalesce(input_credits_per_million, 0)::numeric as input_price,
               coalesce(output_credits_per_million, 0)::numeric as output_price,
               coalesce(cached_input_credits_per_million, 0)::numeric as cached_input_price
        from ai.runs where id = ${input.runId} and owner_user_id = ${input.ownerUserId}
        for update
      `)).rows[0]
      if (!run) throw new Error("ai.run_not_found")
      await tx.execute(sql`
        update ai.model_credit_holds
        set state = 'reconciliation_required', reconciliation_reason = 'hold_expired', updated_at = now()
        where owner_user_id = ${input.ownerUserId}
          and state = 'held' and expires_at <= now()
      `)
      const decision = (await tx.execute<Record<string, unknown>>(sql`
        with owner_holds as (
          select coalesce(sum(coalesce(actual_credits, max_risk_credits)), 0)::numeric as credits
          from ai.model_credit_holds
          where owner_user_id = ${input.ownerUserId}
            and state in ('held', 'usage_recorded', 'hold_deficit', 'reconciliation_required')
        ), risk as (
          select least(${input.requestedOutputTokens}::bigint, ${String(run.max_output_tokens)}::bigint)::bigint as output_limit,
                 ((${String(run.max_context_tokens)}::numeric * greatest(${String(run.input_price)}::numeric, ${String(run.cached_input_price)}::numeric))
                  + (${String(run.max_output_tokens)}::numeric * ${String(run.output_price)}::numeric)) / 1000000 as max_risk_credits,
                 ${String(wallet.balance_credits)}::numeric - owner_holds.credits as wallet_credit_available
          from owner_holds
        )
        select output_limit, max_risk_credits,
               max_risk_credits <= wallet_credit_available as allowed
        from risk
      `)).rows[0]!
      if (decision.allowed !== true) throw new Error("ai.wallet_balance_insufficient")
      const maxOutputTokens = Number(decision.output_limit)
      const attemptRow = (await tx.execute<Record<string, unknown>>(sql`
        select coalesce(max(attempt), 0) + 1 as attempt
        from ai.model_credit_holds where run_id = ${input.runId} and operation = ${input.operation}
      `)).rows[0]!
      const attempt = Number(attemptRow.attempt)
      await tx.execute(sql`
        insert into ai.model_credit_holds (
          id, run_id, owner_user_id, operation, attempt, state, model_id, model_name,
          max_context_tokens_snapshot, max_output_tokens_snapshot,
          input_credits_per_million, output_credits_per_million,
          cached_input_credits_per_million, max_risk_credits, expires_at
        ) values (
          ${input.id}, ${input.runId}, ${input.ownerUserId}, ${input.operation}, ${attempt}, 'held',
          ${String(run.model_id)}, ${String(run.model_name)},
          ${String(run.max_context_tokens)}::bigint, ${String(run.max_output_tokens)}::bigint,
          ${String(run.input_price)}::numeric, ${String(run.output_price)}::numeric,
          ${String(run.cached_input_price)}::numeric, ${String(decision.max_risk_credits)}::numeric,
          now() + make_interval(secs => ${input.leaseSeconds})
        )
      `)
      return { id: input.id, attempt, maxOutputTokens }
    }))
  }

  async recordReportedModelUsage(creditHoldId: string, usage: OfficialModelUsage, metadata: ModelAttemptMetadata & { callType: "stream" | "complete" }) {
    return withSpan("agent.usage.persist", internalSpanOptions(), () => this.db.transaction(async tx => {
      const current = (await tx.execute<Record<string, unknown>>(sql`
        select * from ai.model_credit_holds where id = ${creditHoldId} for update
      `)).rows[0]
      if (!current) throw new Error("ai.credit_hold_not_found")
      if (current.state !== "held") throw new Error("ai.credit_hold_not_active")
      const cached = usage.cachedPromptTokens ?? 0
      const cacheWrite = usage.cacheWritePromptTokens ?? 0
      const normalPrompt = usage.promptTokens - cached - cacheWrite
      const actual = (await tx.execute<Record<string, unknown>>(sql`
        select calculated.credits,
               calculated.credits > ${String(current.max_risk_credits)}::numeric as deficit
        from (select ((${normalPrompt}::numeric * ${String(current.input_credits_per_million)}::numeric)
          + ((${cached}::numeric + ${cacheWrite}::numeric) * ${String(current.cached_input_credits_per_million)}::numeric)
          + (${usage.completionTokens}::numeric * ${String(current.output_credits_per_million)}::numeric)) / 1000000 as credits) calculated
      `)).rows[0]!
      const deficit = actual.deficit === true
      const usageId = createId("aiuse")
      await tx.execute(sql`
        insert into ai.model_usages (
          id, credit_hold_id, run_id, owner_user_id, operation, attempt, status, settlement_status,
          model_id, model_name, max_context_tokens_snapshot,
          prompt_tokens, completion_tokens, total_tokens, cached_prompt_tokens,
          cache_write_prompt_tokens, reasoning_completion_tokens,
          provider_request_id, response_id, response_model, finish_reason, call_type
        ) values (
          ${usageId}, ${creditHoldId}, ${String(current.run_id)}, ${String(current.owner_user_id)},
          ${String(current.operation)}, ${Number(current.attempt)}, 'reported',
          ${deficit ? "reconciliation_required" : "pending"},
          ${String(current.model_id)}, ${String(current.model_name)}, ${String(current.max_context_tokens_snapshot)}::bigint,
          ${usage.promptTokens}, ${usage.completionTokens}, ${usage.totalTokens},
          ${usage.cachedPromptTokens ?? null}, ${usage.cacheWritePromptTokens ?? null}, ${usage.reasoningCompletionTokens ?? null},
          ${metadata.providerRequestId ?? null}, ${metadata.responseId ?? null}, ${metadata.responseModel ?? null},
          ${metadata.finishReason ?? null}, ${metadata.callType}
        )
      `)
      if (current.operation === "assistant") {
        await tx.execute(sql`
          update ai.conversations conversation
          set context_usage_run_id = ${String(current.run_id)},
              context_usage_model_id = ${String(current.model_id)},
              context_used_tokens = ${usage.totalTokens}::bigint,
              context_max_tokens_snapshot = ${String(current.max_context_tokens_snapshot)}::bigint,
              context_usage_recorded_at = now()
          from ai.runs run
          where run.id = ${String(current.run_id)} and conversation.id = run.conversation_id
        `)
      }
      await tx.execute(sql`
        update ai.model_credit_holds
        set state = ${deficit ? "hold_deficit" : "usage_recorded"}, actual_credits = ${String(actual.credits)}::numeric,
            provider_request_id = ${metadata.providerRequestId ?? null}, response_id = ${metadata.responseId ?? null},
            response_model = ${metadata.responseModel ?? null},
            reconciliation_reason = ${deficit ? "hold_deficit" : null},
            updated_at = now()
        where id = ${creditHoldId} and state = 'held'
      `)
      return { reconciliationRequired: deficit }
    }))
  }

  async markModelUsageUnavailable(creditHoldId: string, reason: UsageUnavailableReason | "request_outcome_unknown", metadata: ModelAttemptMetadata): Promise<void> {
    await withSpan("agent.usage.reconciliation_required", internalSpanOptions(), async () => {
      await this.db.execute(sql`
        update ai.model_credit_holds
        set state = 'reconciliation_required', reconciliation_reason = ${reason},
            provider_request_id = ${metadata.providerRequestId ?? null}, response_id = ${metadata.responseId ?? null},
            response_model = ${metadata.responseModel ?? null}, failure_stage = ${metadata.failureStage ?? null},
            updated_at = now()
        where id = ${creditHoldId} and state = 'held'
      `)
    })
  }

  async releaseModelCreditHold(creditHoldId: string): Promise<void> {
    await withSpan("agent.credit_hold.release", internalSpanOptions(), async () => {
      await this.db.execute(sql`update ai.model_credit_holds set state = 'released', updated_at = now() where id = ${creditHoldId} and state = 'held'`)
    })
  }

  async getLatestReportedModelUsage(conversationId: string) {
    const row = (await this.db.execute<Record<string, unknown>>(sql`
      select usage.model_id, usage.prompt_tokens, usage.max_context_tokens_snapshot
      from ai.model_usages usage
      join ai.runs run on run.id = usage.run_id
      where run.conversation_id = ${conversationId}
        and usage.operation = 'assistant' and usage.status = 'reported'
      order by usage.occurred_at desc, usage.id desc limit 1
    `)).rows[0]
    if (!row) return undefined
    return { modelId: String(row.model_id), promptTokens: Number(row.prompt_tokens), maxContextTokensSnapshot: Number(row.max_context_tokens_snapshot) }
  }

  async createConversation(ownerUserId: string, title: string, projectId?: string, titleSource?: ConversationTitleSource, modelId?: string) {
    const row = (await this.db.insert(conversations).values({
      id: createId("aicnv"),
      ownerUserId,
      projectId: projectId ?? null,
      modelId: modelId ?? null,
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
    return this.updateConversation(ownerUserId, id, { title })
  }

  async updateConversation(ownerUserId: string, id: string, input: { title?: string, modelId?: string }) {
    return this.db.transaction(async (tx) => {
      const current = (await tx.select().from(conversations)
        .where(and(eq(conversations.id, id), eq(conversations.ownerUserId, ownerUserId)))
        .for("update"))[0]
      if (!current) return undefined
      const row = (await tx.update(conversations)
        .set({
          ...(input.title ? { title: input.title, titleSource: "user" as const } : {}),
          ...(input.modelId ? { modelId: input.modelId } : {}),
          updatedAt: sql`now()`,
        })
        .where(eq(conversations.id, id))
        .returning())[0]
      if (!row) return undefined
      if (input.title && (current.title !== input.title || current.titleSource !== "user")) {
        const latestRun = (await tx.select({ id: runs.id, status: runs.status }).from(runs)
          .where(eq(runs.conversationId, id))
          .orderBy(desc(runs.createdAt), desc(runs.id))
          .limit(1))[0]
        // 用户改名接口本身返回权威 Conversation；活动模型 session 的序号由 Redis
        // 独占，不能在其间从 PostgreSQL 分配同一 Run 的事件序号。
        if (latestRun && latestRun.status !== "running") {
          await this.appendEventWith(tx, latestRun.id, "conversation.title.updated", {
            title: input.title,
            titleSource: "user",
            locked: true,
          })
        }
      }
      return mapConversation(row)
    })
  }

  async renameConversationByAssistant(id: string, title: string, runId?: string) {
    return this.db.transaction(async (tx) => {
      if (runId) {
        const boundRun = (await tx.select({ id: runs.id }).from(runs)
          .where(and(eq(runs.id, runId), eq(runs.conversationId, id))))[0]
        if (!boundRun) throw new Error("ai.run_conversation_mismatch")
      }
      const row = (await tx.update(conversations)
        .set({ title, titleSource: "assistant", updatedAt: sql`now()` })
        .where(and(eq(conversations.id, id), ne(conversations.titleSource, "user")))
        .returning())[0]
      if (!row) return undefined
      if (runId) {
        await this.appendEventWith(tx, runId, "conversation.title.updated", {
          title,
          titleSource: "assistant",
          locked: false,
        })
      }
      return mapConversation(row)
    })
  }

  async deleteConversation(ownerUserId: string, id: string) {
    const deleted = await this.db.delete(conversations)
      .where(and(eq(conversations.id, id), eq(conversations.ownerUserId, ownerUserId)))
      .returning({ id: conversations.id })
    return deleted.length === 1
  }

  async createTurn(ownerUserId: string, input: CreateTurn): Promise<CreatedTurn> {
    const actorSessionId = input.actorSessionId
    if (!actorSessionId) throw new Error("ai.actor_session_required")
    return withSpan("agent.repository.turn.create", internalSpanOptions(), () => this.db.transaction(async (tx) => {
      const hash = createTurnRequestHash(input)
      const existing = (await tx.select().from(idempotencyKeys)
        .where(and(eq(idempotencyKeys.ownerUserId, ownerUserId), eq(idempotencyKeys.idempotencyKey, input.idempotencyKey))))[0]
      if (existing) {
        if (existing.requestHash !== hash) throw new Error("idempotency_conflict")
        return this.loadCreated(tx, existing.turnId, existing.runId)
      }
      const owned = (await tx.select({
        id: conversations.id,
        title: conversations.title,
        titleSource: conversations.titleSource,
      }).from(conversations)
        .where(and(eq(conversations.id, input.conversationId), eq(conversations.ownerUserId, ownerUserId)))
        .for("update"))[0]
      if (!owned) throw new Error("ai.conversation_not_found")
      if (input.modelId) {
        await tx.update(conversations)
          .set({ modelId: input.modelId, updatedAt: sql`now()` })
          .where(eq(conversations.id, input.conversationId))
      }
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
        modelId: input.modelId ?? null,
      })
      await tx.insert(runs).values({
        id: runId,
        ownerUserId,
        conversationId: input.conversationId,
        turnId,
        runIndex: 0,
        status: "queued",
        promptVersion: "system-v4",
        actorSessionId,
        toolCatalogDigest: input.toolCatalogDigest ?? "sha256:platform-tools-v1",
        selectedOperationIds: [],
        pageContext: input.pageContext,
        traceContext: input.traceContext ?? {},
        clientInstanceId: input.clientInstanceId ?? null,
        modelId: input.modelSnapshot?.id ?? input.modelId ?? null,
        modelName: input.modelSnapshot?.name ?? null,
        maxContextTokens: input.modelSnapshot?.maxContextTokens ?? null,
        maxOutputTokens: input.modelSnapshot?.maxOutputTokens ?? null,
        inputCreditsPerMillion: input.modelSnapshot?.inputCreditsPerMillion ?? null,
        outputCreditsPerMillion: input.modelSnapshot?.outputCreditsPerMillion ?? null,
        cachedInputCreditsPerMillion: input.modelSnapshot?.cachedInputCreditsPerMillion ?? null,
      })
      await tx.insert(idempotencyKeys).values({
        ownerUserId,
        idempotencyKey: input.idempotencyKey,
        requestHash: hash,
        turnId,
        runId,
      })
      const userItem = await this.appendItemWith(tx, { id: `${turnId}:input`, runId, turnId, type: "user_message", status: "completed", content: { parts: [{ type: "text", text: input.input }] } })
      await this.appendEventWith(tx, runId, "run.input_received", {
        initial: true,
        item: userItem,
        conversationTitle: owned.title,
        conversationTitleSource: owned.titleSource,
      })
      await this.appendEventWith(tx, runId, "run.queued", { state: "queued" })
      return this.loadCreated(tx, turnId, runId)
    }))
  }

  async getRun(ownerUserId: string, id: string) {
    const row = (await this.db.select().from(runs)
      .where(and(eq(runs.id, id), eq(runs.ownerUserId, ownerUserId))))[0]
    return row ? mapRun(row) : undefined
  }

  async getRunToolState(runId: string) {
    const row = (await this.db.select({
      toolCatalogDigest: runs.toolCatalogDigest,
      selectedOperationIds: runs.selectedOperationIds,
    }).from(runs).where(eq(runs.id, runId)))[0]
    return row
  }

  async getRunStreamPosition(runId: string) {
    return (await this.db.select({
      nextItemPosition: runs.nextItemPosition,
      nextEventSequence: runs.nextEventSequence,
      runVersion: runs.rowVersion,
    }).from(runs).where(eq(runs.id, runId)))[0]
  }

  async persistRunStreamBatch(batch: RunStreamBatch): Promise<void> {
    const startedAt = performance.now()
    let outcome = "succeeded"
    try {
      await withSpan("agent.repository.stream.persist", internalSpanOptions({
        "luna.stream.item_count": batch.items.length,
        "luna.stream.event_count": batch.events.length,
      }), () => this.db.transaction(async (tx) => {
        const run = (await tx.select({
          nextItemPosition: runs.nextItemPosition,
          nextEventSequence: runs.nextEventSequence,
          conversationId: runs.conversationId,
          turnId: runs.turnId,
          status: runs.status,
          rowVersion: runs.rowVersion,
        }).from(runs).where(eq(runs.id, batch.runId)).for("update"))[0]
        if (!run) throw new Error("ai.run_not_found")
        const itemIds = batch.items.map(item => item.id)
        const eventIds = batch.events.map(event => event.id)
        const existingItems = itemIds.length
          ? (await tx.select({ count: count() }).from(items).where(inArray(items.id, itemIds)))[0]?.count ?? 0
          : 0
        const existingEvents = eventIds.length
          ? (await tx.select({ count: count() }).from(runEvents).where(inArray(runEvents.id, eventIds)))[0]?.count ?? 0
          : 0
        const batchAlreadyPersisted = existingItems === itemIds.length && existingEvents === eventIds.length
          && run.nextEventSequence > batch.eventHighWatermark
          && run.nextItemPosition >= batch.expected.nextItemPosition + batch.items.length
        if (batchAlreadyPersisted && (!batch.terminal || run.status === batch.terminal.to)) return
        if (run.rowVersion !== batch.expectedRunVersion || run.status !== "running") {
          throw new RunStateConflictError(
            batch.runId,
            batch.terminal?.from ?? "running",
            batch.terminal?.to ?? "running",
            run.status,
          )
        }
        if (existingItems > 0 || existingEvents > 0) throw new Error("ai.stream_batch_partial_conflict")
        if (run.nextItemPosition !== batch.expected.nextItemPosition) throw new Error("ai.stream_item_position_conflict")
        const rebasedItems = batch.items
        const conflictingSequences = batch.events.length
          ? await tx.select({ sequence: runEvents.eventSequence }).from(runEvents).where(and(
              eq(runEvents.runId, batch.runId),
              inArray(runEvents.eventSequence, batch.events.map(event => event.sequence)),
            ))
          : []
        if (conflictingSequences.length) throw new Error("ai.stream_sequence_conflict")
        const rebasedEvents = batch.events
        if (rebasedItems.length) {
          await tx.insert(items).values(rebasedItems.map(item => ({
            id: item.id, runId: item.runId, turnId: item.turnId,
            timelineIndex: item.timelineIndex, type: item.type, status: item.status,
            content: item.content, revision: item.revision, createdAt: new Date(item.createdAt),
          })))
        }
        if (rebasedEvents.length) {
          await tx.insert(runEvents).values(rebasedEvents.map(event => ({
            id: event.id, runId: event.runId, eventSequence: event.sequence,
            type: event.type, data: event.data, createdAt: new Date(event.createdAt),
          })))
        }
        await tx.update(runs).set({
          nextItemPosition: run.nextItemPosition + rebasedItems.length,
          nextEventSequence: Math.max(
            run.nextEventSequence,
            batch.eventHighWatermark + 1,
            ...rebasedEvents.map(event => event.sequence + 1),
          ),
        }).where(eq(runs.id, batch.runId))
        if (rebasedItems.some(item => item.type === "assistant_message")) {
          await tx.update(conversations).set({ updatedAt: sql`now()` })
            .where(eq(conversations.id, run.conversationId))
        }
        if (batch.terminal) {
          if (batch.terminal.conversationTitle) {
            const renamed = await tx.update(conversations).set({
              title: batch.terminal.conversationTitle,
              titleSource: "assistant",
              updatedAt: sql`now()`,
            }).where(and(
              eq(conversations.id, run.conversationId), ne(conversations.titleSource, "user"),
            )).returning({ id: conversations.id })
            if (renamed.length) await this.appendEventWith(tx, batch.runId, "conversation.title.updated", {
              title: batch.terminal.conversationTitle, titleSource: "assistant", locked: false,
            })
          }
          const row = (await tx.update(runs).set({
            status: batch.terminal.to,
            rowVersion: sql`${runs.rowVersion} + 1`,
            completedAt: new Date(batch.terminal.completedAt),
            ...(batch.terminal.errorCode ? { errorCode: batch.terminal.errorCode } : {}),
          }).where(and(
            eq(runs.id, batch.runId),
            eq(runs.status, batch.terminal.from),
            eq(runs.rowVersion, batch.expectedRunVersion),
          )).returning({ rowVersion: runs.rowVersion }))[0]
          if (!row) throw new RunStateConflictError(batch.runId, batch.terminal.from, batch.terminal.to)
          await tx.update(turns).set({ status: batch.terminal.to }).where(eq(turns.id, run.turnId))
          await this.appendEventWith(tx, batch.runId, `run.${batch.terminal.to}`, {
            state: batch.terminal.to,
            rowVersion: row.rowVersion,
            ...(batch.terminal.errorCode ? { errorCode: batch.terminal.errorCode } : {}),
          })
        }
      }))
    }
    catch (error) {
      outcome = "failed"
      throw error
    }
    finally {
      agentMetrics.streamPersistenceDuration.record((performance.now() - startedAt) / 1000, { outcome })
    }
  }

  async touchRunSelectedOperations(runId: string, operationIds: string[], limit: number) {
    const boundedLimit = Math.max(1, Math.min(64, Math.floor(limit)))
    const requested = [...new Set(operationIds)].filter(operationId => /^[A-Za-z][A-Za-z0-9._-]{2,100}$/.test(operationId))
    return this.db.transaction(async (tx) => {
      const row = (await tx.select({ selectedOperationIds: runs.selectedOperationIds })
        .from(runs).where(eq(runs.id, runId)).for("update"))[0]
      if (!row) throw new Error("ai.run_not_found")
      const alreadySelectedOperationIds = requested.filter(operationId => row.selectedOperationIds.includes(operationId))
      const touched = new Set(requested)
      const reordered = [...row.selectedOperationIds.filter(operationId => !touched.has(operationId)), ...requested]
      const evictedOperationIds = reordered.slice(0, Math.max(0, reordered.length - boundedLimit))
      const selectedOperationIds = reordered.slice(-boundedLimit)
      if (requested.length || evictedOperationIds.length) {
        await tx.update(runs).set({ selectedOperationIds }).where(eq(runs.id, runId))
      }
      return { selectedOperationIds, alreadySelectedOperationIds, evictedOperationIds }
    })
  }

  async listActiveToolCatalogDigests() {
    const rows = await this.db.selectDistinct({ digest: runs.toolCatalogDigest }).from(runs)
      .where(inArray(runs.status, ["queued", "running", "waiting_approval", "waiting_input"]))
    return rows.map(row => row.digest)
  }

  async cancelRun(ownerUserId: string, id: string) {
    return this.db.transaction(async (tx) => {
      const row = (await tx.update(runs)
        .set({ status: "canceled", rowVersion: sql`${runs.rowVersion} + 1`, completedAt: sql`now()` })
        .where(and(
          eq(runs.id, id),
          eq(runs.ownerUserId, ownerUserId),
          sql`${runs.status} not in ('completed', 'failed', 'canceled', 'expired', 'interrupted')`,
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

  async claimNextQueuedRun() {
    return withSpan("agent.repository.run.claim", internalSpanOptions(), async () => this.db.transaction(async (tx) => {
      const raw = (await tx.execute<Record<string, unknown>>(sql`
        with candidate as (
          select id from ai.runs
          where status = 'queued'
          order by created_at, id
          for update skip locked
          limit 1
        )
        update ai.runs r
        set status = 'running',
            row_version = r.row_version + 1,
            started_at = coalesce(r.started_at, now())
        from candidate
        where r.id = candidate.id
        returning r.*
      `)).rows[0]
      if (!raw) return undefined
      const row = driverRow(runs, raw) as RunRow
      await tx.update(turns).set({ status: "running" }).where(eq(turns.id, row.turnId))
      await this.appendEventWith(tx, row.id, "run.running", { state: "running", rowVersion: row.rowVersion })
      return mapRun(row)
    }))
  }

  async listStaleRunningRuns(startedBefore: string) {
    return this.db.select({ id: runs.id, rowVersion: runs.rowVersion }).from(runs).where(and(
      eq(runs.status, "running"),
      lt(runs.startedAt, new Date(startedBefore)),
    ))
  }

  async interruptAbandonedRun(runId: string, expectedRunVersion: number, eventHighWatermark: number): Promise<boolean> {
    return this.db.transaction(async (tx) => {
      const row = (await tx.update(runs).set({
        status: "interrupted",
        rowVersion: sql`${runs.rowVersion} + 1`,
        completedAt: sql`now()`,
        errorCode: "ai.owner_lease_expired",
        nextEventSequence: sql`greatest(${runs.nextEventSequence}, ${eventHighWatermark + 1})`,
      }).where(and(
        eq(runs.id, runId), eq(runs.status, "running"), eq(runs.rowVersion, expectedRunVersion),
      )).returning({ turnId: runs.turnId, rowVersion: runs.rowVersion }))[0]
      if (!row) return false
      await tx.update(turns).set({ status: "interrupted" }).where(eq(turns.id, row.turnId))
      await this.appendEventWith(tx, runId, "run.interrupted", {
        state: "interrupted", rowVersion: row.rowVersion, errorCode: "ai.owner_lease_expired",
      })
      return true
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
      toolCatalogDigest: runs.toolCatalogDigest,
      selectedOperationIds: runs.selectedOperationIds,
      title: conversations.title,
      titleSource: conversations.titleSource,
      modelId: runs.modelId,
      modelName: runs.modelName,
      maxContextTokens: runs.maxContextTokens,
      maxOutputTokens: runs.maxOutputTokens,
      inputCreditsPerMillion: runs.inputCreditsPerMillion,
      outputCreditsPerMillion: runs.outputCreditsPerMillion,
      cachedInputCreditsPerMillion: runs.cachedInputCreditsPerMillion,
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
        createdAt: items.createdAt,
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
      toolCatalogDigest: row.toolCatalogDigest,
      selectedOperationIds: row.selectedOperationIds,
      ...(row.modelId && row.modelName ? {
        model: {
          id: row.modelId,
          name: row.modelName,
          maxContextTokens: row.maxContextTokens ?? 524_288,
          maxOutputTokens: row.maxOutputTokens ?? 65_536,
          inputCreditsPerMillion: row.inputCreditsPerMillion ?? "0",
          outputCreditsPerMillion: row.outputCreditsPerMillion ?? "0",
          cachedInputCreditsPerMillion: row.cachedInputCreditsPerMillion ?? "0",
        },
      } : {}),
      toolInteractions: currentToolInteractions.map(item => ({ itemId: item.id, type: item.type as "tool_call" | "tool_result", status: item.status, content: item.content })),
      history: historyRows.rows.map((item): ConversationHistoryEntry => ({
        turnIndex: item.turn_index,
        user: item.input,
        assistant: item.assistant,
        ...((toolInteractionsByTurn.get(item.turn_index)?.length ?? 0) > 0
          ? { toolInteractions: toolInteractionsByTurn.get(item.turn_index)!.map(tool => ({ type: tool.type, status: tool.status, content: tool.content, createdAt: tool.createdAt.toISOString() })) }
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

  async hasToolApprovalExemption(runId: string, operationId: string) {
    const row = (await this.db.execute<{ exists: boolean }>(sql`
      select exists(
        select 1
        from ai.runs r
        join ai.tool_approval_exemptions e on e.user_id = r.owner_user_id
        where r.id = ${runId} and e.operation_id = ${operationId}
      ) as exists
    `)).rows[0]
    return row?.exists === true
  }

  async grantToolApprovalExemption(runId: string, operationId: string, sourceToolCallId: string) {
    const inserted = await this.db.execute(sql`
      insert into ai.tool_approval_exemptions(user_id, operation_id, created_at, updated_at, source_tool_call_id)
      select owner_user_id, ${operationId}, now(), now(), ${sourceToolCallId}
      from ai.runs where id = ${runId}
      on conflict (user_id, operation_id) do update
      set updated_at = excluded.updated_at,
          source_tool_call_id = excluded.source_tool_call_id
      returning user_id
    `)
    if (inserted.rowCount !== 1) throw new Error("ai.run_not_found")
  }

  async listToolApprovalExemptions(ownerUserId: string) {
    const result = await this.db.execute<{ operationId: string, createdAt: Date | string }>(sql`
      select operation_id as "operationId", created_at as "createdAt"
      from ai.tool_approval_exemptions
      where user_id = ${ownerUserId}
      order by operation_id
    `)
    return result.rows.map(row => ({
      operationId: row.operationId,
      createdAt: row.createdAt instanceof Date ? row.createdAt.toISOString() : new Date(row.createdAt).toISOString(),
    }))
  }

  async revokeToolApprovalExemption(ownerUserId: string, operationId: string) {
    const deleted = await this.db.execute(sql`
      delete from ai.tool_approval_exemptions
      where user_id = ${ownerUserId} and operation_id = ${operationId}
    `)
    return (deleted.rowCount ?? 0) > 0
  }

  async appendRunInput(runId: string, text: string) {
    const updated = await this.db.update(turns)
      .set({ input: sql`${turns.input} || E'\n' || ${text}` })
      .from(runs)
      .where(and(eq(runs.id, runId), eq(turns.id, runs.turnId)))
      .returning({ id: turns.id })
    if (!updated.length) throw new Error("ai.run_not_found")
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

  async completeToolItemWithEvent(
    itemId: string,
    status: TimelineItem["status"],
    content: Record<string, unknown>,
    resultValue: Omit<TimelineItem, "id" | "timelineIndex" | "revision" | "createdAt"> & { id?: string },
    eventType: string,
    eventData: Record<string, unknown> = {},
  ) {
    return this.db.transaction(async (tx) => {
      const item = await this.updateItemWith(tx, itemId, status, content)
      const resultItem = await this.appendItemWith(tx, resultValue)
      if (resultItem.runId !== item.runId || resultItem.turnId !== item.turnId)
        throw new Error("ai.tool_result_binding_mismatch")
      const event = await this.appendEventWith(tx, item.runId, eventType, { ...eventData, item, resultItem })
      return { item, resultItem, event }
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
      const contextUsage = conversationRow.contextUsageRunId
        && conversationRow.contextUsageModelId
        && conversationRow.contextUsedTokens !== null
        && conversationRow.contextMaxTokensSnapshot !== null
        && conversationRow.contextUsageRecordedAt
        ? {
            status: "reported" as const,
            runId: conversationRow.contextUsageRunId,
            modelId: conversationRow.contextUsageModelId,
            usedTokens: conversationRow.contextUsedTokens,
            maxContextTokensSnapshot: conversationRow.contextMaxTokensSnapshot,
            recordedAt: conversationRow.contextUsageRecordedAt.toISOString(),
          }
        : undefined
      const constraints = [eq(turns.conversationId, conversationId)]
      if (options.beforeTurnIndex !== undefined) constraints.push(lt(turns.turnIndex, options.beforeTurnIndex))
      const recentTurns = await tx.select().from(turns)
        .where(and(...constraints))
        .orderBy(desc(turns.turnIndex))
        .limit(limit + 1)
      const hasOlder = recentTurns.length > limit
      const boundedTurns = recentTurns.slice(0, limit).reverse()
      if (boundedTurns.length === 0) {
        return { conversation, ...(contextUsage ? { contextUsage } : {}), turns: [], eventCursors: [], pageInfo: { hasOlder: false } }
      }

      const runIds = boundedTurns.map(turn => turn.selectedRunId)
      const runRows = await tx.select().from(runs)
        .where(and(inArray(runs.id, runIds), eq(runs.ownerUserId, ownerUserId)))
      const authorizedRunIds = runRows.map(run => run.id)
      // 最近一次主回答的实际输入量用于展示单次上下文占用；不再维护 Run 累计预算。
      const usageRows = authorizedRunIds.length === 0
        ? { rows: [] as { runId: string, latestPromptTokens: string, modelId: string, maxContextTokensSnapshot: string }[] }
        : await tx.execute<{ runId: string, latestPromptTokens: string, modelId: string, maxContextTokensSnapshot: string }>(sql`
            select distinct on (run_id)
                   run_id as "runId",
                   prompt_tokens as "latestPromptTokens",
                   model_id as "modelId",
                   max_context_tokens_snapshot as "maxContextTokensSnapshot"
            from ai.model_usages
            where run_id in ${authorizedRunIds}
              and operation = 'assistant'
              and status = 'reported'
            order by run_id, occurred_at desc, id desc
          `)
      const usageByRun = new Map(usageRows.rows.map(row => [row.runId, {
        latestPromptTokens: Number(row.latestPromptTokens),
        latestUsageModelId: row.modelId,
        latestUsageMaxContextTokensSnapshot: Number(row.maxContextTokensSnapshot),
      }]))
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
        const mappedRun = runRow ? mapRun(runRow) : undefined
        return {
          id: turn.id,
          turnIndex: turn.turnIndex,
          status: turn.status,
          input: turn.input,
          createdAt: turn.createdAt.toISOString(),
          ...(mappedRun ? { run: { ...mappedRun, ...usageByRun.get(mappedRun.id) } } : {}),
          items: itemsByRun.get(turn.selectedRunId) ?? [],
        }
      })
      return {
        conversation,
        ...(contextUsage ? { contextUsage } : {}),
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
