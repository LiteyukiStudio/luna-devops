import { and, asc, count, desc, eq, gt, gte, ilike, inArray, isNull, lt, ne, sql } from "drizzle-orm"
import type { Pool } from "pg"
import type {
  ConversationHistoryEntry,
  ConversationSummary,
  ConversationToolInteraction,
  ConversationTitleSource,
  CreatedTurn,
  CreateTurn,
  Run,
  RunCheckpoint,
  RunCheckpointAdvance,
  RunExecutionSnapshot,
  TimelineItem,
} from "../domain.js"
import { createId } from "../id.js"
import type { PayloadCipher } from "../payload-cipher.js"
import { defaultRuntimeSettings, runtimeSettingsSnapshot, type RuntimeSettings } from "../runtime-settings.js"
import { agentMetrics, internalSpanOptions, withSpan } from "../telemetry.js"
import {
  RunStateConflictError,
  RunCheckpointConflictError,
  selectOperationsWithExecutedProtection,
  type ConversationListOptions,
  type InterruptRunOptions,
  type Repository,
  type OrphanedRunRecovery,
  type RecordReportedModelUsageInput,
  type ResolveWaitingExternalRunInput,
  type RunLeaseRenewalOutcome,
  type TimelinePageOptions,
  type RunStreamBatch,
  type UpdateRunOptions,
} from "./repository.js"
import { createTurnRequestHash } from "./create-turn-hash.js"
import { AgentDatabase, type AgentDatabaseOptions, type AgentDb, type AgentTx } from "./database.js"
import { activeRunningRunVersionFence } from "./run-fence.js"
import {
  mapConversation,
  mapConversationSummary,
  mapClaimedRun,
  mapRun,
  mapRunCheckpoint,
  mapRunEvent,
  mapTimelineItem,
  timelineContentText,
  timelineReasoningText,
} from "./mappers/domain.js"
import {
  conversations,
  conversationSummaries,
  idempotencyKeys,
  items,
  runCheckpoints,
  runEvents,
  runs,
  toolCalls,
  turns,
  type RunRow,
} from "./schema/index.js"

/** Drizzle 事务回调或共享 db 实例均可执行的最小查询接口 */
type Querier = AgentDb | AgentTx

const historyTurnWindow = 8

export class PostgresRepository implements Repository {
  private readonly database: AgentDatabase

  constructor(
    connectionString: string,
    options: AgentDatabaseOptions = {},
    private readonly runExecutionSnapshotCipher?: PayloadCipher,
  ) {
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

  async assertPositiveAvailableBalance(ownerUserId: string): Promise<void> {
    await this.assertPositiveAvailableBalanceWith(this.db, ownerUserId)
  }

  async resolveLiveActorRole(userId: string, sessionId?: string): Promise<string | undefined> {
    if (sessionId) {
      const row = (await this.db.execute<{ role: string }>(sql`
        select u.role
        from users u
        join user_sessions s on s.user_id = u.id
        where u.id = ${userId}
          and u.disabled = false
          and s.id = ${sessionId}
          and s.expires_at > now()
        limit 1
      `)).rows[0]
      return row?.role
    }
    const row = (await this.db.execute<{ role: string }>(sql`
      select u.role
      from users u
      where u.id = ${userId}
        and u.disabled = false
      limit 1
    `)).rows[0]
    return row?.role
  }

  private async assertPositiveAvailableBalanceWith(db: Querier, ownerUserId: string): Promise<void> {
    const row = (await db.execute<{ allowed: boolean }>(sql`
      with wallet as (
        select balance_credits::numeric as balance_credits
        from user_wallets
        where user_id = ${ownerUserId}
      ), pending_billing as (
        select coalesce(sum(amount_credits), 0)::numeric as credits
        from billing_usage_records
        where billed_user_id = ${ownerUserId}
          and status = 'pending'
      ), pending_ai as (
        select coalesce(sum((
          ((prompt_tokens - coalesce(cached_prompt_tokens, 0) - coalesce(cache_write_prompt_tokens, 0))::numeric * input_credits_per_million)
          + ((coalesce(cached_prompt_tokens, 0) + coalesce(cache_write_prompt_tokens, 0))::numeric * cached_input_credits_per_million)
          + (completion_tokens::numeric * output_credits_per_million)
        ) / 1000000), 0)::numeric as credits
        from ai.model_usages
        where owner_user_id = ${ownerUserId}
          and status = 'reported'
          and settlement_status = 'pending'
      )
      select exists(select 1 from wallet)
        and ((select balance_credits from wallet) - pending_billing.credits - pending_ai.credits) > 0 as allowed
      from pending_billing, pending_ai
    `)).rows[0]
    if (row?.allowed !== true) throw new Error("ai.wallet_balance_insufficient")
  }

  async recordReportedModelUsage(input: RecordReportedModelUsageInput): Promise<{ id: string, attempt: number }> {
    return withSpan("agent.usage.persist", internalSpanOptions({
      "luna.usage.operation": input.operation,
    }), () => this.db.transaction(async tx => {
      const run = (await tx.execute<Record<string, unknown>>(sql`
        select id, owner_user_id,
               coalesce(model_id, 'legacy') as model_id,
               coalesce(model_name, 'legacy') as model_name,
               coalesce(max_context_tokens, 524288)::bigint as max_context_tokens,
               coalesce(max_output_tokens, 65536)::bigint as max_output_tokens,
               coalesce(input_credits_per_million, 0)::numeric as input_price,
               coalesce(output_credits_per_million, 0)::numeric as output_price,
               coalesce(cached_input_credits_per_million, 0)::numeric as cached_input_price
        from ai.runs
        where id = ${input.runId} and owner_user_id = ${input.ownerUserId}
      `)).rows[0]
      if (!run && !input.model) throw new Error("ai.run_not_found")
      const modelId = input.model?.id ?? String(run!.model_id)
      const modelName = input.model?.name ?? String(run!.model_name)
      const maxContextTokens = input.model?.maxContextTokens ?? Number(run!.max_context_tokens)
      const maxOutputTokens = input.model?.maxOutputTokens ?? Number(run!.max_output_tokens)
      const inputPrice = input.model?.inputCreditsPerMillion ?? String(run!.input_price)
      const outputPrice = input.model?.outputCreditsPerMillion ?? String(run!.output_price)
      const cachedInputPrice = input.model?.cachedInputCreditsPerMillion ?? String(run!.cached_input_price)
      const usageId = createId("aiuse")
      const inserted = (await tx.execute<{ attempt: number }>(sql`
        insert into ai.model_usages (
          id, run_id, owner_user_id, operation, attempt, status, settlement_status,
          model_id, model_name, max_context_tokens_snapshot, max_output_tokens_snapshot,
          input_credits_per_million, output_credits_per_million, cached_input_credits_per_million,
          prompt_tokens, completion_tokens, total_tokens, cached_prompt_tokens,
          cache_write_prompt_tokens, reasoning_completion_tokens,
          provider_request_id, response_id, response_model, finish_reason, call_type
        ) values (
          ${usageId}, ${input.runId}, ${input.ownerUserId}, ${input.operation},
          (select coalesce(max(attempt), 0) + 1 from ai.model_usages where run_id = ${input.runId} and operation = ${input.operation}),
          'reported', 'pending',
          ${modelId}, ${modelName}, ${maxContextTokens}::bigint, ${maxOutputTokens}::bigint,
          ${inputPrice}::numeric, ${outputPrice}::numeric, ${cachedInputPrice}::numeric,
          ${input.usage.inputTokens}, ${input.usage.outputTokens}, ${input.usage.totalTokens},
          ${input.usage.cacheReadInputTokens ?? null}, ${input.usage.cacheWriteInputTokens ?? null}, ${input.usage.reasoningOutputTokens ?? null},
          ${input.providerRequestId ?? null}, ${input.responseId ?? null}, ${input.responseModel ?? null},
          ${input.finishReason ?? null}, ${input.callType}
        )
        returning attempt
      `)).rows[0]!
      if (input.operation === "assistant") {
        await tx.execute(sql`
          update ai.conversations conversation
          set context_usage_run_id = ${input.runId},
              context_usage_model_id = ${modelId},
              context_used_tokens = ${input.usage.totalTokens}::bigint,
              context_max_tokens_snapshot = ${maxContextTokens}::bigint,
              context_usage_recorded_at = now()
          from ai.runs run
          where run.id = ${input.runId} and conversation.id = run.conversation_id
        `)
      }
      return { id: usageId, attempt: Number(inserted.attempt) }
    }))
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
        // 用户改名接口本身返回权威 Conversation；活动模型 session 的事件序号由
        // Agent 内存流会话单写，不能在其间从 PostgreSQL 分配同一 Run 的事件序号。
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
        await this.lockAndVerifyRunningRunVersion(tx, runId)
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

  async listActiveConversationRuns(ownerUserId: string, conversationId: string): Promise<Run[]> {
    const rows = await this.db.select().from(runs)
      .where(and(
        eq(runs.conversationId, conversationId),
        eq(runs.ownerUserId, ownerUserId),
        inArray(runs.status, ["queued", "running", "waiting_approval", "waiting_input", "waiting_external"]),
      ))
      .orderBy(asc(runs.createdAt), asc(runs.id))
    return rows.map(mapRun)
  }

  async deleteConversation(ownerUserId: string, id: string) {
    return this.db.transaction(async (tx) => {
      const owned = (await tx.select({ id: conversations.id }).from(conversations)
        .where(and(eq(conversations.id, id), eq(conversations.ownerUserId, ownerUserId)))
        .for("update"))[0]
      if (!owned) return false
      const activeRuns = await tx.select({ id: runs.id, turnId: runs.turnId }).from(runs)
        .where(and(
          eq(runs.conversationId, id),
          eq(runs.ownerUserId, ownerUserId),
          inArray(runs.status, ["queued", "running", "waiting_approval", "waiting_input", "waiting_external"]),
        ))
        .for("update")
      if (activeRuns.length > 0) {
        const activeRunIds = activeRuns.map(run => run.id)
        const activeTurnIds = [...new Set(activeRuns.map(run => run.turnId))]
        await tx.update(runs)
          .set({ status: "canceled", wakeAt: null, leaseExpiresAt: null, rowVersion: sql`${runs.rowVersion} + 1`, completedAt: sql`now()` })
          .where(inArray(runs.id, activeRunIds))
        await tx.update(turns)
          .set({ status: "canceled" })
          .where(inArray(turns.id, activeTurnIds))
        await tx.update(toolCalls).set({
          status: "canceled",
          rowVersion: sql`${toolCalls.rowVersion} + 1`,
          result: { code: "ai.run_canceled", retryable: false },
          errorCode: "ai.run_canceled",
          updatedAt: sql`now()`,
        }).where(and(
          inArray(toolCalls.runId, activeRunIds),
          sql`${toolCalls.status} not in ('succeeded', 'failed', 'rejected', 'canceled', 'skipped')`,
        ))
      }
      const deleted = await tx.delete(conversations)
        .where(and(eq(conversations.id, id), eq(conversations.ownerUserId, ownerUserId)))
        .returning({ id: conversations.id })
      return deleted.length === 1
    })
  }

  async createTurn(ownerUserId: string, input: CreateTurn): Promise<CreatedTurn> {
    const actorSessionId = input.actorSessionId
    if (!actorSessionId) throw new Error("ai.actor_session_required")
    return withSpan("agent.repository.turn.create", internalSpanOptions(), () => this.db.transaction(async (tx) => {
      await tx.execute(sql`select pg_advisory_xact_lock(hashtext(${`${ownerUserId}:${input.idempotencyKey}`}))`)
      const hash = createTurnRequestHash(input)
      const existing = (await tx.select().from(idempotencyKeys)
        .where(and(eq(idempotencyKeys.ownerUserId, ownerUserId), eq(idempotencyKeys.idempotencyKey, input.idempotencyKey))))[0]
      if (existing) {
        if (existing.requestHash !== hash) throw new Error("idempotency_conflict")
        return { ...(await this.loadCreated(tx, existing.turnId, existing.runId)), replayed: true }
      }
      const owned = (await tx.select({
        id: conversations.id,
        title: conversations.title,
        titleSource: conversations.titleSource,
      }).from(conversations)
        .where(and(eq(conversations.id, input.conversationId), eq(conversations.ownerUserId, ownerUserId)))
        .for("update"))[0]
      if (!owned) throw new Error("ai.conversation_not_found")
      if (input.modelSnapshot && input.pageContext.__lunaDirectToolAction !== true) {
        await this.assertPositiveAvailableBalanceWith(tx, ownerUserId)
      }
      if (input.modelId) {
        await tx.update(conversations)
          .set({ modelId: input.modelId, updatedAt: sql`now()` })
          .where(eq(conversations.id, input.conversationId))
      }
      const previousSelected = input.pageContext.__lunaDirectToolAction === true
        ? undefined
        : (await tx.select({ selectedOperationIds: runs.selectedOperationIds })
            .from(turns)
            .innerJoin(runs, eq(turns.selectedRunId, runs.id))
            .where(and(
              eq(turns.conversationId, input.conversationId),
              sql`coalesce((${runs.pageContext} ->> '__lunaDirectToolAction')::boolean, false) = false`,
            ))
            .orderBy(desc(turns.turnIndex))
            .limit(1))[0]
      const inheritedOperationIds = (previousSelected?.selectedOperationIds ?? [])
        .filter(operationId => /^[A-Za-z][A-Za-z0-9._-]{2,100}$/.test(operationId))
        .slice(-16)
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
        selectedOperationIds: inheritedOperationIds,
        pageContext: input.pageContext,
        traceContext: input.traceContext ?? {},
        modelId: input.modelSnapshot?.id ?? input.modelId ?? null,
        modelName: input.modelSnapshot?.name ?? null,
        maxContextTokens: input.modelSnapshot?.maxContextTokens ?? null,
        maxOutputTokens: input.modelSnapshot?.maxOutputTokens ?? null,
        inputCreditsPerMillion: input.modelSnapshot?.inputCreditsPerMillion ?? null,
        outputCreditsPerMillion: input.modelSnapshot?.outputCreditsPerMillion ?? null,
        cachedInputCreditsPerMillion: input.modelSnapshot?.cachedInputCreditsPerMillion ?? null,
      })
      const insertedIdempotency = await tx.insert(idempotencyKeys).values({
        ownerUserId,
        idempotencyKey: input.idempotencyKey,
        requestHash: hash,
        turnId,
        runId,
      }).onConflictDoNothing({
        target: [idempotencyKeys.ownerUserId, idempotencyKeys.idempotencyKey],
      }).returning({ turnId: idempotencyKeys.turnId })
      if (insertedIdempotency.length === 0) {
        await tx.delete(turns).where(eq(turns.id, turnId))
        const raceWinner = (await tx.select().from(idempotencyKeys)
          .where(and(eq(idempotencyKeys.ownerUserId, ownerUserId), eq(idempotencyKeys.idempotencyKey, input.idempotencyKey))))[0]
        if (!raceWinner || raceWinner.requestHash !== hash) throw new Error("idempotency_conflict")
        return { ...(await this.loadCreated(tx, raceWinner.turnId, raceWinner.runId)), replayed: true }
      }
      const userItem = await this.appendItemWith(tx, { id: `${turnId}:input`, runId, turnId, type: "user_message", status: "completed", content: { parts: [{ type: "text", text: input.input }] } })
      await this.appendEventWith(tx, runId, "run.input_received", {
        initial: true,
        item: userItem,
        conversationTitle: owned.title,
        conversationTitleSource: owned.titleSource,
      })
      await this.appendEventWith(tx, runId, "run.queued", { state: "queued" })
      return { ...(await this.loadCreated(tx, turnId, runId)), replayed: false }
    }))
  }

  async getRun(ownerUserId: string, id: string) {
    const row = (await this.db.select().from(runs)
      .where(and(eq(runs.id, id), eq(runs.ownerUserId, ownerUserId))))[0]
    return row ? mapRun(row) : undefined
  }

  async assertRunningRunVersion(runId: string, expectedRunVersion: number): Promise<void> {
    await this.db.transaction(async (tx) => {
      await this.lockAndVerifyRunningRunVersion(tx, runId, expectedRunVersion)
    })
  }

  private async lockAndVerifyRunningRunVersion(tx: AgentTx, runId: string, expectedRunVersion?: number): Promise<void> {
    const effective = expectedRunVersion ?? activeRunningRunVersionFence(runId)
    if (effective === undefined) return
    const current = (await tx.select({ status: runs.status, rowVersion: runs.rowVersion })
      .from(runs)
      .where(eq(runs.id, runId))
      .for("update"))[0]
    if (!current || current.status !== "running" || current.rowVersion !== effective) {
      throw new RunStateConflictError(runId, "running", "running", current?.status, current?.rowVersion)
    }
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
            run.rowVersion,
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
          const lingeringStreaming = await tx.select().from(items)
            .where(and(eq(items.runId, batch.runId), eq(items.status, "streaming")))
            .orderBy(asc(items.timelineIndex))
            .for("update")
          const terminalToolStatuses = new Set(["succeeded", "failed", "rejected", "canceled", "skipped"])
          const finalizedStatus = batch.terminal.to === "completed" ? "completed" : "failed"
          for (const current of lingeringStreaming) {
            const itemStatus = current.type === "tool_call" ? "failed" : finalizedStatus
            const itemContent = current.type === "tool_call" && (typeof current.content.status !== "string" || !terminalToolStatuses.has(current.content.status))
              ? { ...current.content, status: "failed" }
              : current.content
            const finalized = await this.updateItemWith(tx, current.id, itemStatus, itemContent)
            await this.appendEventWith(tx, batch.runId, "item.finalized", { item: finalized })
          }
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
            wakeAt: null,
            leaseExpiresAt: null,
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

  async touchRunSelectedOperations(
    runId: string,
    operationIds: string[],
    limit: number,
    protectedOperationIds?: ReadonlySet<string>,
  ) {
    const requested = [...new Set(operationIds)].filter(operationId => /^[A-Za-z][A-Za-z0-9._-]{2,100}$/.test(operationId))
    return this.db.transaction(async (tx) => {
      await this.lockAndVerifyRunningRunVersion(tx, runId)
      const row = (await tx.select({ selectedOperationIds: runs.selectedOperationIds })
        .from(runs).where(eq(runs.id, runId)).for("update"))[0]
      if (!row) throw new Error("ai.run_not_found")
      const executedRows = await tx.selectDistinct({ operationId: toolCalls.operationId })
        .from(toolCalls)
        .where(and(
          eq(toolCalls.runId, runId),
          ne(toolCalls.inputMode, "verification"),
          sql`not (${toolCalls.status} = 'skipped' and ${toolCalls.executionCount} = 0)`,
        ))
      const executedInRun = new Set<string>([
        ...executedRows.map(item => item.operationId),
        ...(protectedOperationIds ?? []),
      ])
      const selection = selectOperationsWithExecutedProtection(row.selectedOperationIds, requested, executedInRun, limit)
      if (requested.length || selection.evictedOperationIds.length) {
        await tx.update(runs).set({ selectedOperationIds: selection.selectedOperationIds }).where(eq(runs.id, runId))
      }
      return selection
    })
  }

  async listActiveToolCatalogDigests() {
    const rows = await this.db.selectDistinct({ digest: runs.toolCatalogDigest }).from(runs)
      .where(inArray(runs.status, ["queued", "running", "waiting_approval", "waiting_input", "waiting_external"]))
    return rows.map(row => row.digest)
  }

  async cancelRun(ownerUserId: string, id: string, expectedRowVersion?: number) {
    return this.db.transaction(async (tx) => {
      const conditions = [
        eq(runs.id, id),
        eq(runs.ownerUserId, ownerUserId),
        sql`${runs.status} not in ('completed', 'failed', 'canceled', 'expired', 'interrupted')`,
        ...(expectedRowVersion !== undefined ? [eq(runs.rowVersion, expectedRowVersion)] : []),
      ]
      const row = (await tx.update(runs)
        .set({ status: "canceled", wakeAt: null, leaseExpiresAt: null, rowVersion: sql`${runs.rowVersion} + 1`, completedAt: sql`now()` })
        .where(and(...conditions))
        .returning())[0]
      if (!row) return undefined
      await tx.update(turns).set({ status: "canceled" }).where(eq(turns.id, row.turnId))
      const canceledResult = { code: "ai.run_canceled", retryable: false }
      const canceledCalls = await tx.update(toolCalls).set({
        status: "canceled",
        rowVersion: sql`${toolCalls.rowVersion} + 1`,
        result: canceledResult,
        errorCode: "ai.run_canceled",
        updatedAt: sql`now()`,
      }).where(and(
        eq(toolCalls.runId, id),
        sql`${toolCalls.status} not in ('succeeded', 'failed', 'rejected', 'canceled', 'skipped')`,
      )).returning({ id: toolCalls.id })
      if (canceledCalls.length > 0) {
        const toolCallIdByItemId = new Map(canceledCalls.map(call => [`${call.id}:item`, call.id]))
        const toolItems = await tx.select().from(items)
          .where(and(eq(items.runId, id), inArray(items.id, [...toolCallIdByItemId.keys()])))
          .orderBy(asc(items.timelineIndex))
          .for("update")
        for (const current of toolItems) {
          const toolCallId = toolCallIdByItemId.get(current.id)
          if (!toolCallId) continue
          const item = await this.updateItemWith(tx, current.id, "completed", {
            ...current.content,
            status: "canceled",
            result: canceledResult,
            errorCode: "ai.run_canceled",
          })
          await this.appendEventWith(tx, id, "item.finalized", { itemId: item.id, toolCallId, item })
        }
      }
      const remainingStreaming = await tx.select().from(items)
        .where(and(eq(items.runId, id), eq(items.status, "streaming")))
        .orderBy(asc(items.timelineIndex))
        .for("update")
      for (const current of remainingStreaming) {
        const nextStatus = current.type === "tool_call" ? "completed" : "failed"
        const nextContent = current.type === "tool_call"
          ? { ...current.content, status: "canceled", result: canceledResult, errorCode: "ai.run_canceled" }
          : current.content
        const item = await this.updateItemWith(tx, current.id, nextStatus, nextContent)
        const toolCallId = typeof current.content.toolCallId === "string" ? current.content.toolCallId : undefined
        await this.appendEventWith(tx, id, "item.finalized", {
          itemId: item.id,
          ...(toolCallId ? { toolCallId } : {}),
          item,
        })
      }
      await this.appendEventWith(tx, id, "run.canceled", { state: "canceled", rowVersion: row.rowVersion })
      return mapRun(row)
    }).then(async (run) => {
      if (run) return run
      // 未命中条件更新时读取权威当前状态，保持与旧实现一致的返回语义
      return this.getRun(ownerUserId, id)
    })
  }

  async claimNextQueuedRun(executionSnapshot?: RunExecutionSnapshot, userConcurrentRuns?: number) {
    const candidateToolCatalogDigest = executionSnapshot?.toolCatalogDigest ?? null
    const maxPerUser = userConcurrentRuns ?? executionSnapshot?.runtimeSettings.userConcurrentRuns
    return this.db.transaction(async (tx) => {
      const candidate = (maxPerUser === undefined
        ? await tx.execute<Record<string, unknown>>(sql`
            select q.id, q.execution_snapshot_ciphertext
            from ai.runs q
            where q.status = 'queued'
              and (
                coalesce((q.page_context ->> '__lunaDirectToolAction')::boolean, false) = false
                or exists (
                  select 1
                  from ai.tool_calls tc
                  inner join ai.items it on it.id = (tc.id || ':item') and it.run_id = q.id
                  where tc.run_id = q.id
                    and tc.input_mode = 'direct'
                    and (
                      (tc.status = 'proposed' and tc.error_code is null)
                      or tc.status = 'rejected'
                      or tc.execution_count > 0
                    )
                )
              )
            order by q.created_at, q.id
            for update of q skip locked
            limit 1
          `)
        : await tx.execute<Record<string, unknown>>(sql`
            select q.id, q.execution_snapshot_ciphertext
            from ai.runs q
            where q.status = 'queued'
              and (
                coalesce((q.page_context ->> '__lunaDirectToolAction')::boolean, false) = false
                or exists (
                  select 1
                  from ai.tool_calls tc
                  inner join ai.items it on it.id = (tc.id || ':item') and it.run_id = q.id
                  where tc.run_id = q.id
                    and tc.input_mode = 'direct'
                    and (
                      (tc.status = 'proposed' and tc.error_code is null)
                      or tc.status = 'rejected'
                      or tc.execution_count > 0
                    )
                )
              )
              and pg_try_advisory_xact_lock(hashtext('ai.run_claim_owner'), hashtext(q.owner_user_id))
              and (
                select count(*)::int
                from ai.runs active
                where active.owner_user_id = q.owner_user_id
                  and active.status = 'running'
              ) < ${Math.max(1, Math.floor(maxPerUser))}
            order by q.created_at, q.id
            for update of q skip locked
            limit 1
          `)).rows[0]
      if (!candidate || typeof candidate.id !== "string") return undefined
      const candidateId = candidate.id
      return withSpan("agent.repository.run.claim", internalSpanOptions({ "luna.run.id": candidateId }), async () => {
        const candidateCiphertext = candidate.execution_snapshot_ciphertext === null && executionSnapshot
          ? this.encryptRunExecutionSnapshot(candidateId, executionSnapshot)
          : null
        const raw = (await tx.execute<Record<string, unknown>>(sql`
          update ai.runs r
          set status = 'running',
              wake_at = null,
              lease_expires_at = now() + interval '60 seconds',
              row_version = r.row_version + 1,
              started_at = coalesce(r.started_at, now()),
              tool_catalog_digest = case
                when r.execution_snapshot_ciphertext is null
                  then coalesce(${candidateToolCatalogDigest}, r.tool_catalog_digest)
                else r.tool_catalog_digest
              end,
              execution_snapshot_ciphertext = coalesce(r.execution_snapshot_ciphertext, ${candidateCiphertext})
          where r.id = ${candidateId}
            and r.status = 'queued'
          returning r.*
        `)).rows[0]
        if (!raw) return undefined
        const row = driverRow(runs, raw) as RunRow
        const restoredSnapshot = row.executionSnapshotCiphertext
          ? this.decryptRunExecutionSnapshot(row.id, row.executionSnapshotCiphertext)
          : undefined
        await tx.update(turns).set({ status: "running" }).where(eq(turns.id, row.turnId))
        await this.appendEventWith(tx, row.id, "run.running", { state: "running", rowVersion: row.rowVersion })
        return mapClaimedRun(row, restoredSnapshot)
      })
    })
  }

  async renewRunLease(runId: string, expectedRowVersion: number, leaseMs: number): Promise<RunLeaseRenewalOutcome> {
    const boundedLeaseMs = Math.max(1_000, Math.floor(leaseMs))
    const result = await this.db.execute<{ id: string }>(sql`
      update ai.runs
      set lease_expires_at = now() + (${boundedLeaseMs}::bigint * interval '1 millisecond')
      where id = ${runId}
        and status = 'running'
        and row_version = ${expectedRowVersion}
        and lease_expires_at > now()
      returning id
    `)
    if (result.rows.length === 1) return "renewed"
    const current = (await this.db.select({ status: runs.status }).from(runs).where(eq(runs.id, runId)))[0]
    return current?.status === "canceled" ? "canceled" : "lost"
  }

  async updateRunToolCatalogDigest(runId: string, toolCatalogDigest: string, expectedRowVersion?: number): Promise<void> {
    await this.db.transaction(async (tx) => {
      const current = (await tx.select({
        rowVersion: runs.rowVersion,
        executionSnapshotCiphertext: runs.executionSnapshotCiphertext,
      }).from(runs).where(eq(runs.id, runId)).for("update"))[0]
      if (!current || (expectedRowVersion !== undefined && current.rowVersion !== expectedRowVersion)) return
      let nextCiphertext = current.executionSnapshotCiphertext
      if (current.executionSnapshotCiphertext && this.runExecutionSnapshotCipher) {
        const snapshot = this.decryptRunExecutionSnapshot(runId, current.executionSnapshotCiphertext)
        nextCiphertext = this.encryptRunExecutionSnapshot(runId, {
          runtimeSettings: snapshot.runtimeSettings,
          toolCatalogDigest,
        })
      }
      const conditions = [
        eq(runs.id, runId),
        ...(expectedRowVersion !== undefined ? [eq(runs.rowVersion, expectedRowVersion)] : []),
      ]
      await tx.update(runs).set({
        toolCatalogDigest,
        executionSnapshotCiphertext: nextCiphertext,
      }).where(and(...conditions))
    })
  }

  async claimDueWaitingExternalRuns(limit: number, leaseMs: number, now?: Date): Promise<Run[]> {
    const boundedLimit = Math.max(1, Math.min(64, Math.floor(limit)))
    const boundedLeaseMs = Math.max(1_000, Math.floor(leaseMs))
    const referenceIso = (now ?? new Date()).toISOString()
    const nextLeaseIso = new Date(Date.parse(referenceIso) + boundedLeaseMs).toISOString()
    const result = await this.db.execute<Record<string, unknown>>(sql`
      update ai.runs r
      set wake_at = ${nextLeaseIso}::timestamptz
      where r.id in (
        select candidate.id
        from ai.runs candidate
        where candidate.status = 'waiting_external'
          and candidate.wake_at is not null
          and candidate.wake_at <= ${referenceIso}::timestamptz
        order by candidate.wake_at asc, candidate.id asc
        for update skip locked
        limit ${boundedLimit}
      )
      returning r.*
    `)
    if (result.rows.length === 0) return []
    return withSpan("agent.repository.run.claim_due_external", internalSpanOptions({
      "luna.claimed_runs.count": result.rows.length,
    }), async () => result.rows.map(raw => mapRun(driverRow(runs, raw) as RunRow)))
  }

  async rescheduleWaitingExternalRun(
    runId: string,
    expectedCheckpointRevision: number,
    pollCount: number,
    wakeAt: string,
  ): Promise<boolean> {
    return this.db.transaction(async (tx) => {
      const runRow = (await tx.select({ id: runs.id }).from(runs)
        .where(and(eq(runs.id, runId), eq(runs.status, "waiting_external")))
        .for("update"))[0]
      if (!runRow) return false
      const checkpoint = (await tx.select().from(runCheckpoints)
        .where(eq(runCheckpoints.runId, runId))
        .for("update"))[0]
      if (!checkpoint || checkpoint.revision !== expectedCheckpointRevision || !checkpoint.state.waitingTask)
        return false
      await tx.update(runCheckpoints).set({
        revision: expectedCheckpointRevision + 1,
        state: {
          ...checkpoint.state,
          waitingTask: {
            ...checkpoint.state.waitingTask,
            pollCount,
          },
        },
        updatedAt: sql`now()`,
      }).where(eq(runCheckpoints.runId, runId))
      await tx.update(runs).set({
        wakeAt: new Date(wakeAt),
      }).where(and(eq(runs.id, runId), eq(runs.status, "waiting_external")))
      return true
    })
  }

  async resolveWaitingExternalRun(input: ResolveWaitingExternalRunInput): Promise<boolean> {
    return withSpan("agent.repository.run.resolve_external", internalSpanOptions(), () => this.db.transaction(async (tx) => {
      const runRow = (await tx.select({ id: runs.id, turnId: runs.turnId }).from(runs)
        .where(and(eq(runs.id, input.runId), eq(runs.status, "waiting_external")))
        .for("update"))[0]
      if (!runRow) return false
      const checkpoint = (await tx.select({ revision: runCheckpoints.revision }).from(runCheckpoints)
        .where(eq(runCheckpoints.runId, input.runId))
        .for("update"))[0]
      if (!checkpoint || checkpoint.revision !== input.expectedCheckpointRevision) return false
      const item = await this.updateItemWith(tx, input.itemId, input.itemStatus, input.itemContent)
      await this.appendEventWith(tx, input.runId, input.eventType, {
        ...input.eventData,
        itemId: item.id,
        toolCallId: input.toolCallId,
        item,
      })
      await tx.update(runCheckpoints).set({
        revision: input.expectedCheckpointRevision + 1,
        phase: input.nextPhase,
        state: this.encryptCheckpointState(input.runId, input.nextCheckpointState),
        updatedAt: sql`now()`,
      }).where(eq(runCheckpoints.runId, input.runId))
      const updated = (await tx.update(runs).set({
        status: "queued",
        wakeAt: null,
        leaseExpiresAt: null,
        rowVersion: sql`${runs.rowVersion} + 1`,
      }).where(and(eq(runs.id, input.runId), eq(runs.status, "waiting_external")))
        .returning({ rowVersion: runs.rowVersion }))[0]
      if (!updated) return false
      await tx.update(turns).set({ status: "queued" }).where(eq(turns.id, runRow.turnId))
      await this.appendEventWith(tx, input.runId, "run.queued", {
        state: "queued",
        rowVersion: updated.rowVersion,
      })
      return true
    }))
  }

  private encryptRunExecutionSnapshot(runId: string, snapshot: RunExecutionSnapshot): string {
    if (!this.runExecutionSnapshotCipher) throw new Error("ai.run_execution_snapshot_key_unavailable")
    const minimalPayload = {
      runtimeSettings: runtimeSettingsSnapshot(snapshot.runtimeSettings),
      ...(snapshot.toolCatalogDigest ? { toolCatalogDigest: snapshot.toolCatalogDigest } : {}),
    }
    return this.runExecutionSnapshotCipher.encrypt(JSON.stringify(minimalPayload), runId)
  }

  private decryptRunExecutionSnapshot(runId: string, ciphertext: string): RunExecutionSnapshot {
    if (!this.runExecutionSnapshotCipher) throw new Error("ai.run_execution_snapshot_key_unavailable")
    try {
      const parsed: unknown = JSON.parse(this.runExecutionSnapshotCipher.decrypt(ciphertext, runId))
      if (!isRecord(parsed) || !isRuntimeSettings(parsed.runtimeSettings))
        throw new Error("ai.run_execution_snapshot_invalid")
      if (parsed.toolCatalogDigest !== undefined && typeof parsed.toolCatalogDigest !== "string")
        throw new Error("ai.run_execution_snapshot_invalid")
      // 显式忽略历史存量行中可能存在的 providerConfig，防止旧 apiKey 或大体积 toolCatalog 驻留内存
      return Object.freeze({
        runtimeSettings: runtimeSettingsSnapshot(parsed.runtimeSettings),
        ...(typeof parsed.toolCatalogDigest === "string" ? { toolCatalogDigest: parsed.toolCatalogDigest } : {}),
      })
    }
    catch (error) {
      if (error instanceof Error && error.message === "ai.run_execution_snapshot_key_unavailable") throw error
      throw new Error("ai.run_execution_snapshot_invalid", { cause: error })
    }
  }

  async listRunningRuns(now?: Date) {
    const reference = now
      ? sql`${now.toISOString()}::timestamptz`
      : sql`now()`
    return this.db.select({ id: runs.id, rowVersion: runs.rowVersion })
      .from(runs)
      .where(and(
        eq(runs.status, "running"),
        sql`(${runs.leaseExpiresAt} is null or ${runs.leaseExpiresAt} <= ${reference})`,
      ))
      .orderBy(asc(runs.leaseExpiresAt), asc(runs.id))
  }

  async interruptOrphanedRun(runId: string, expectedRunVersion: number, options?: InterruptRunOptions): Promise<boolean> {
    return this.db.transaction(async (tx) => {
      const row = await this.interruptRunningRow(tx, runId, expectedRunVersion, options?.ignoreActiveLease === true)
      return row !== undefined
    })
  }

  /**
   * 启动对账：只有 checkpoint 能证明续跑位置时才把孤儿 Run 重新排队，
   * 其余情况沿用 interrupted。恢复次数有上限，避免崩溃-恢复循环。
   */
  async recoverOrphanedRun(runId: string, expectedRunVersion: number, maxRecoveries: number): Promise<OrphanedRunRecovery> {
    return this.db.transaction(async (tx) => {
      const checkpoint = (await tx.select({ recoveries: runCheckpoints.recoveries })
        .from(runCheckpoints)
        .where(eq(runCheckpoints.runId, runId))
        .for("update"))[0]
      if (!checkpoint || checkpoint.recoveries >= maxRecoveries) {
        const interrupted = await this.interruptRunningRow(tx, runId, expectedRunVersion, false)
        return interrupted === undefined ? "skipped" : "interrupted"
      }
      const row = (await tx.update(runs).set({
        status: "queued",
        leaseExpiresAt: null,
        rowVersion: sql`${runs.rowVersion} + 1`,
      }).where(and(
        eq(runs.id, runId),
        eq(runs.status, "running"),
        eq(runs.rowVersion, expectedRunVersion),
        sql`(${runs.leaseExpiresAt} is null or ${runs.leaseExpiresAt} <= now())`,
      )).returning({ turnId: runs.turnId, rowVersion: runs.rowVersion }))[0]
      if (!row) return "skipped"
      await tx.update(turns).set({ status: "queued" }).where(eq(turns.id, row.turnId))
      await tx.update(runCheckpoints).set({
        recoveries: sql`${runCheckpoints.recoveries} + 1`,
        revision: sql`${runCheckpoints.revision} + 1`,
        updatedAt: sql`now()`,
      }).where(eq(runCheckpoints.runId, runId))
      await this.appendEventWith(tx, runId, "run.requeued", {
        state: "queued",
        rowVersion: row.rowVersion,
        reason: "ai.agent_restarted",
        recoveries: checkpoint.recoveries + 1,
      })
      return "requeued"
    })
  }

  async loadRunCheckpoint(runId: string) {
    const row = (await this.db.select().from(runCheckpoints).where(eq(runCheckpoints.runId, runId)))[0]
    if (!row) return undefined
    const mapped = mapRunCheckpoint(row)
    return { ...mapped, state: this.decryptCheckpointState(runId, mapped.state) }
  }

  /**
   * revision 单调推进：只有当前 revision 等于 expectedRevision 才能写入。
   * 同一 revision 的重放返回已存状态（幂等），更旧的 revision 抛冲突，
   * 避免重启后的恢复用陈旧状态覆盖新状态。
   */
  async saveRunCheckpoint(runId: string, advance: RunCheckpointAdvance): Promise<RunCheckpoint> {
    const revision = advance.expectedRevision + 1
    const persistedState = this.encryptCheckpointState(runId, advance.state)
    return this.db.transaction(async (tx) => {
      await this.lockAndVerifyRunningRunVersion(tx, runId, advance.expectedRunVersion)
      const row = (await tx.insert(runCheckpoints).values({
        runId,
        revision,
        phase: advance.phase,
        modelStep: advance.modelStep,
        toolCallCount: advance.toolCallCount,
        state: persistedState,
      })
        .onConflictDoUpdate({
          target: runCheckpoints.runId,
          set: {
            revision: sql`excluded.revision`,
            phase: sql`excluded.phase`,
            modelStep: sql`excluded.model_step`,
            toolCallCount: sql`excluded.tool_call_count`,
            state: sql`excluded.state`,
            updatedAt: sql`now()`,
          },
          setWhere: sql`${runCheckpoints.revision} = excluded.revision - 1`,
        })
        .returning())[0]
      if (row) {
        const mapped = mapRunCheckpoint(row)
        return { ...mapped, state: this.decryptCheckpointState(runId, mapped.state) }
      }
      const currentRow = (await tx.select().from(runCheckpoints).where(eq(runCheckpoints.runId, runId)))[0]
      const current = currentRow ? mapRunCheckpoint(currentRow) : undefined
      if (current && current.revision === revision) {
        return { ...current, state: this.decryptCheckpointState(runId, current.state) }
      }
      throw new RunCheckpointConflictError(runId, advance.expectedRevision, current?.revision)
    })
  }

  private encryptCheckpointState(runId: string, state: RunCheckpoint["state"]): RunCheckpoint["state"] {
    if (!this.runExecutionSnapshotCipher || !state.pendingBatchCalls?.length) return state
    return {
      ...state,
      pendingBatchCalls: state.pendingBatchCalls.map((call) => {
        if (call.argumentsCiphertext) {
          return {
            id: call.id,
            operationId: call.operationId,
            argumentsCiphertext: call.argumentsCiphertext,
            ...(call.argumentError ? { argumentError: call.argumentError } : {}),
          }
        }
        const rawArgs = call.arguments ?? {}
        const ciphertext = this.runExecutionSnapshotCipher!.encrypt(JSON.stringify(rawArgs), `${runId}:pending:${call.id}`)
        return {
          id: call.id,
          operationId: call.operationId,
          argumentsCiphertext: ciphertext,
          ...(call.argumentError ? { argumentError: call.argumentError } : {}),
        }
      }),
    }
  }

  private decryptCheckpointState(runId: string, state: RunCheckpoint["state"]): RunCheckpoint["state"] {
    if (!state.pendingBatchCalls?.length) return state
    return {
      ...state,
      pendingBatchCalls: state.pendingBatchCalls.map((call) => {
        if (!call.argumentsCiphertext) {
          return { ...call, arguments: call.arguments ?? {} }
        }
        if (!this.runExecutionSnapshotCipher) {
          throw new Error("ai.tool_arguments_key_unavailable")
        }
        const decrypted = JSON.parse(this.runExecutionSnapshotCipher.decrypt(call.argumentsCiphertext, `${runId}:pending:${call.id}`)) as Record<string, unknown>
        return {
          id: call.id,
          operationId: call.operationId,
          arguments: decrypted,
          ...(call.argumentError ? { argumentError: call.argumentError } : {}),
        }
      }),
    }
  }

  private async interruptRunningRow(tx: AgentTx, runId: string, expectedRunVersion: number, ignoreActiveLease = false) {
    const conditions = [
      eq(runs.id, runId),
      eq(runs.status, "running"),
      eq(runs.rowVersion, expectedRunVersion),
      ...(ignoreActiveLease ? [] : [sql`(${runs.leaseExpiresAt} is null or ${runs.leaseExpiresAt} <= now())`]),
    ]
    const row = (await tx.update(runs).set({
      status: "interrupted",
      leaseExpiresAt: null,
      rowVersion: sql`${runs.rowVersion} + 1`,
      completedAt: sql`now()`,
      errorCode: "ai.agent_restarted",
    }).where(and(...conditions)).returning({ turnId: runs.turnId, rowVersion: runs.rowVersion }))[0]
    if (!row) return undefined
    await tx.update(turns).set({ status: "interrupted" }).where(eq(turns.id, row.turnId))
    await this.appendEventWith(tx, runId, "run.interrupted", {
      state: "interrupted", rowVersion: row.rowVersion, errorCode: "ai.agent_restarted",
    })
    return row
  }

  async countActiveUserRuns(userId: string) {
    const rows = await this.db.select({ count: sql<number>`count(*)::int` })
      .from(runs)
      .where(and(
        eq(runs.ownerUserId, userId),
        eq(runs.status, "running"),
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
      contextUsedTokens: conversations.contextUsedTokens,
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
        .where(and(
          eq(items.runId, runId),
          ne(items.id, `${row.turnId}:input`),
          inArray(items.type, ["reasoning_summary", "progress", "tool_call", "tool_result", "user_message"]),
        ))
        .orderBy(asc(items.timelineIndex)),
      this.db.execute<{ turn_index: number, input: string, supplemental_input: string, page_context: Record<string, unknown>, assistant: string, reasoning: string }>(sql`
        select recent.turn_index, recent.input, recent.page_context,
               coalesce(string_agg(i.content->'parts'->0->>'text', E'\n\n' order by i.timeline_index)
                 filter (where i.type = 'user_message' and i.id <> (recent.turn_id || ':input')), '') supplemental_input,
               coalesce(string_agg(i.content->'parts'->0->>'text', E'\n' order by i.timeline_index)
                 filter (where i.type = 'assistant_message'), '') assistant,
               coalesce(string_agg(i.content->>'summary', E'\n' order by i.timeline_index)
                 filter (where i.type = 'reasoning_summary'), '') reasoning
        from (
          select t.id as turn_id, t.turn_index, t.input, t.selected_run_id, r.page_context
          from ai.turns t
          inner join ai.runs r on r.id = t.selected_run_id
          where t.conversation_id = ${row.conversationId} and t.turn_index < ${row.turnIndex}
          order by t.turn_index desc
          limit ${historyTurnWindow}
        ) recent
        left join ai.items i on i.run_id = recent.selected_run_id
        group by recent.turn_id, recent.turn_index, recent.input, recent.page_context
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
      toolCatalogDigest: row.toolCatalogDigest,
      selectedOperationIds: row.selectedOperationIds,
      ...(row.contextUsedTokens !== null ? { lastTotalTokens: row.contextUsedTokens } : {}),
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
      toolInteractions: currentToolInteractions.map(item => ({ itemId: item.id, type: item.type as ConversationToolInteraction["type"], status: item.status, content: item.content })),
      history: historyRows.rows.map((item): ConversationHistoryEntry => ({
        turnIndex: item.turn_index,
        user: item.supplemental_input ? `${item.input}\n\n${item.supplemental_input}` : item.input,
        assistant: item.assistant,
        ...(item.reasoning ? { reasoning: item.reasoning } : {}),
        pageContext: item.page_context,
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
      turnId: turns.id,
      turnIndex: turns.turnIndex,
      input: turns.input,
      selectedRunId: turns.selectedRunId,
      pageContext: runs.pageContext,
    })
      .from(turns)
      .innerJoin(runs, eq(runs.id, turns.selectedRunId))
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
      id: items.id,
      runId: items.runId,
      type: items.type,
      status: items.status,
      content: items.content,
      timelineIndex: items.timelineIndex,
    })
      .from(items)
      .where(and(inArray(items.runId, runIds), inArray(items.type, ["user_message", "assistant_message", "reasoning_summary", "tool_call", "tool_result"])))
      .orderBy(asc(items.runId), asc(items.timelineIndex))
    const byRun = new Map<string, typeof turnItems>()
    for (const item of turnItems) {
      const values = byRun.get(item.runId) ?? []
      values.push(item)
      byRun.set(item.runId, values)
    }
    return boundedTurns.map((turn) => {
      const runItems = byRun.get(turn.selectedRunId) ?? []
      const supplementalInputs = runItems
        .filter(item => item.type === "user_message" && item.id !== `${turn.turnId}:input`)
        .map(item => timelineContentText(item.content))
        .filter(Boolean)
      const user = supplementalInputs.length > 0 ? `${turn.input}\n\n${supplementalInputs.join("\n\n")}` : turn.input
      const assistant = runItems
        .filter(item => item.type === "assistant_message")
        .map(item => timelineContentText(item.content))
        .filter(Boolean)
        .join("\n")
      const toolInteractions = runItems
        .filter(item => item.type === "tool_call" || item.type === "tool_result")
        .map(item => ({ type: item.type, status: item.status, content: item.content }))
      const reasoning = runItems
        .filter(item => item.type === "reasoning_summary")
        .map(item => timelineReasoningText(item.content))
        .filter(Boolean)
        .join("\n")
      return {
        turnIndex: turn.turnIndex,
        user,
        assistant,
        ...(reasoning ? { reasoning } : {}),
        pageContext: turn.pageContext,
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

  async appendRunInput(runId: string, text: string) {
    return this.db.transaction(async (tx) => {
      const boundRun = (await tx.select({ turnId: runs.turnId }).from(runs).where(eq(runs.id, runId)).for("update"))[0]
      if (!boundRun) throw new Error("ai.run_not_found")
      return this.appendItemWith(tx, {
        runId,
        turnId: boundRun.turnId,
        type: "user_message",
        status: "completed",
        content: { parts: [{ type: "text", text }] },
      })
    })
  }

  async updateRun(runId: string, from: Run["status"], to: Run["status"], fields: UpdateRunOptions = {}) {
    return withSpan("agent.repository.run.transition", internalSpanOptions({
      "luna.run.expected_status": from,
      "luna.run.target_status": to,
    }), async (span) => this.db.transaction(async (tx) => {
      const startedAt = typeof fields.startedAt === "string" ? fields.startedAt : null
      const completedAt = typeof fields.completedAt === "string" ? fields.completedAt : null
      const errorCode = typeof fields.errorCode === "string" ? fields.errorCode : null
      const wakeAtPatch = fields.clearWakeAt || to !== "waiting_external"
        ? { wakeAt: null }
        : typeof fields.wakeAt === "string"
          ? { wakeAt: new Date(fields.wakeAt) }
          : {}
      const leasePatch = to === "running"
        ? { leaseExpiresAt: sql`now() + interval '60 seconds'` }
        : { leaseExpiresAt: null }
      const conditions = [
        eq(runs.id, runId),
        eq(runs.status, from),
        ...(fields.expectedRowVersion !== undefined ? [eq(runs.rowVersion, fields.expectedRowVersion)] : []),
      ]
      const row = (await tx.update(runs)
        .set({
          status: to,
          rowVersion: sql`${runs.rowVersion} + 1`,
          startedAt: sql`coalesce(${startedAt}::timestamptz, ${runs.startedAt})`,
          completedAt: sql`coalesce(${completedAt}::timestamptz, ${runs.completedAt})`,
          errorCode: sql`coalesce(${errorCode}, ${runs.errorCode})`,
          ...wakeAtPatch,
          ...leasePatch,
        })
        .where(and(...conditions))
        .returning())[0]
      if (!row) {
        // 条件更新未命中时读取权威当前状态后抛出冲突，禁止改成先查后改的竞态实现
        const current = (await tx.select({ status: runs.status, rowVersion: runs.rowVersion }).from(runs).where(eq(runs.id, runId)))[0]
        const conflict = new RunStateConflictError(runId, from, to, current?.status, current?.rowVersion)
        span.setAttribute("luna.run.actual_status", conflict.actualStatus ?? "missing")
        throw conflict
      }
      await tx.update(turns).set({ status: to }).where(eq(turns.id, row.turnId))
      await this.appendEventWith(tx, runId, `run.${to}`, {
        state: to,
        rowVersion: row.rowVersion,
        ...(fields.errorCode ? { errorCode: fields.errorCode } : {}),
        ...(fields.eventData ? fields.eventData : {}),
      })
      return mapRun(row)
    }))
  }

  async appendItem(value: Omit<TimelineItem, "id" | "timelineIndex" | "revision" | "createdAt"> & { id?: string }) {
    if (activeRunningRunVersionFence(value.runId) !== undefined) {
      return this.db.transaction(async (tx) => {
        await this.lockAndVerifyRunningRunVersion(tx, value.runId)
        return this.appendItemWith(tx, value)
      })
    }
    return this.appendItemWith(this.db, value)
  }

  async updateItem(itemId: string, status: TimelineItem["status"], content: Record<string, unknown>) {
    return this.db.transaction(async (tx) => {
      const existing = (await tx.select({ runId: items.runId }).from(items).where(eq(items.id, itemId)))[0]
      if (!existing) throw new Error("ai.item_not_found")
      await this.lockAndVerifyRunningRunVersion(tx, existing.runId)
      return this.updateItemWith(tx, itemId, status, content)
    })
  }

  async appendItemWithEvent(
    value: Omit<TimelineItem, "id" | "timelineIndex" | "revision" | "createdAt"> & { id?: string },
    eventType: string,
    eventData: Record<string, unknown> = {},
  ) {
    return this.db.transaction(async (tx) => {
      await this.lockAndVerifyRunningRunVersion(tx, value.runId)
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
      const existing = (await tx.select({ runId: items.runId }).from(items).where(eq(items.id, itemId)))[0]
      if (!existing) throw new Error("ai.item_not_found")
      await this.lockAndVerifyRunningRunVersion(tx, existing.runId)
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
      await this.lockAndVerifyRunningRunVersion(tx, resultValue.runId)
      const item = await this.updateItemWith(tx, itemId, status, content)
      const resultItem = await this.appendItemWith(tx, resultValue)
      if (resultItem.runId !== item.runId || resultItem.turnId !== item.turnId)
        throw new Error("ai.tool_result_binding_mismatch")
      const event = await this.appendEventWith(tx, item.runId, eventType, { ...eventData, item, resultItem })
      return { item, resultItem, event }
    })
  }

  async finalizeStreamingItems(runId: string, status: Exclude<TimelineItem["status"], "streaming">, expectedRowVersion?: number) {
    await this.db.transaction(async (tx) => {
      if (expectedRowVersion !== undefined) {
        const current = (await tx.select({ rowVersion: runs.rowVersion }).from(runs).where(eq(runs.id, runId)).for("update"))[0]
        if (!current || current.rowVersion !== expectedRowVersion) return
      }
      const streaming = await tx.select().from(items)
        .where(and(eq(items.runId, runId), eq(items.status, "streaming")))
        .orderBy(asc(items.timelineIndex))
        .for("update")
      const terminalToolStatuses = new Set(["succeeded", "failed", "rejected", "canceled", "skipped"])
      for (const current of streaming) {
        const itemStatus = current.type === "tool_call" ? "failed" : status
        const itemContent = current.type === "tool_call" && (typeof current.content.status !== "string" || !terminalToolStatuses.has(current.content.status))
          ? { ...current.content, status: "failed" }
          : current.content
        const item = await this.updateItemWith(tx, current.id, itemStatus, itemContent)
        await this.appendEventWith(tx, runId, "item.finalized", { item })
      }
    })
  }

  async appendEvent(runId: string, type: string, data: Record<string, unknown>) {
    return withSpan("agent.repository.event.append", internalSpanOptions(), async () => {
      if (activeRunningRunVersionFence(runId) !== undefined) {
        return this.db.transaction(async (tx) => {
          await this.lockAndVerifyRunningRunVersion(tx, runId)
          return this.appendEventWith(tx, runId, type, data)
        })
      }
      return this.appendEventWith(this.db, runId, type, data)
    })
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

  async hasRememberedToolApproval(runId: string, operationId: string): Promise<boolean> {
    const result = await this.pool.query<{ granted: boolean }>(
      `select exists (
         select 1
         from ai.runs r
         join ai.conversation_tool_approvals cta
           on cta.conversation_id = r.conversation_id
          and cta.operation_id = $2
         where r.id = $1
       ) as granted`,
      [runId, operationId],
    )
    return result.rows[0]?.granted === true
  }

  async rememberToolApproval(runId: string, operationId: string, grantedByToolCallId: string): Promise<void> {
    const inserted = await this.pool.query<{ conversation_id: string }>(
      `insert into ai.conversation_tool_approvals (conversation_id, operation_id, granted_by_tool_call_id)
       select r.conversation_id, $2, $3
       from ai.runs r
       where r.id = $1
       on conflict (conversation_id, operation_id) do nothing
       returning conversation_id`,
      [runId, operationId, grantedByToolCallId],
    )
    if (inserted.rowCount === 0) {
      const exists = await this.pool.query<{ id: string }>(
        `select id from ai.runs where id = $1`,
        [runId],
      )
      if (exists.rowCount === 0) throw new Error("ai.run_not_found")
    }
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

const runtimeSettingKeys = Object.keys(defaultRuntimeSettings) as Array<keyof RuntimeSettings>

function isRuntimeSettings(input: unknown): input is RuntimeSettings {
  if (!isRecord(input)) return false
  return runtimeSettingKeys.every((key) => {
    const value = input[key]
    return typeof value === "number" && Number.isFinite(value)
  })
}

function isRecord(input: unknown): input is Record<string, unknown> {
  return typeof input === "object" && input !== null && !Array.isArray(input)
}

function escapeLikePattern(value: string): string {
  return value.replaceAll("\\", "\\\\").replaceAll("%", "\\%").replaceAll("_", "\\_")
}
