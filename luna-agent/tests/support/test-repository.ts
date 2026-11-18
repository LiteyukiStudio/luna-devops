import type {
  Conversation,
  ConversationHistoryEntry,
  ConversationSummary,
  ConversationToolInteraction,
  ConversationTitleSource,
  ClaimedRun,
  CreatedTurn,
  CreateTurn,
  Run,
  RunCheckpoint,
  RunCheckpointAdvance,
  RunEvent,
  RunExecutionSnapshot,
  TimelineItem,
  Turn,
} from "../../src/domain.js"
import { createId } from "../../src/id.js"
import {
  RunCheckpointConflictError,
  RunStateConflictError,
  selectOperationsWithExecutedProtection,
  type ConversationListOptions,
  type InterruptRunOptions,
  type RecordReportedModelUsageInput,
  type ResolveWaitingExternalRunInput,
  type Repository,
  type RunLeaseRenewalOutcome,
  type TimelinePageOptions,
  type ModelCallOperation,
  type RunStreamBatch,
  type UpdateRunOptions,
} from "../../src/persistence/repository.js"
import type { OfficialModelUsage } from "../../src/provider/provider.js"
import { createTurnRequestHash } from "../../src/persistence/create-turn-hash.js"
import { activeRunningRunVersionFence } from "../../src/persistence/run-fence.js"
import { runtimeSettingsSnapshot } from "../../src/runtime-settings.js"
import { MemoryToolCallStore } from "../../src/tools/orchestrator.js"

type StoredRun = Run & { ownerUserId: string }

export class TestRepository implements Repository {
  readonly toolCallStore = new MemoryToolCallStore()
  private readonly conversations = new Map<string, Conversation>()
  private readonly turns = new Map<string, Turn>()
  private readonly runs = new Map<string, StoredRun>()
  private readonly runLeaseExpiresAt = new Map<string, number>()
  private readonly executionSnapshots = new Map<string, RunExecutionSnapshot>()
  private readonly items: TimelineItem[] = []
  private readonly events: RunEvent[] = []
  private readonly summaries = new Map<string, ConversationSummary>()
  private readonly idempotency = new Map<string, { hash: string, created: CreatedTurn }>()
  private readonly idempotencyLocks = new Map<string, Promise<unknown>>()
  private readonly walletBalances = new Map<string, number>()
  private readonly modelUsages: Array<{
    id: string
    runId: string
    ownerUserId: string
    operation: ModelCallOperation
    attempt: number
    usage: OfficialModelUsage
  }> = []
  private readonly contextUsageByConversation = new Map<string, NonNullable<Awaited<ReturnType<Repository["getTimeline"]>>>["contextUsage"]>()
  private readonly streamHighWatermarks = new Map<string, number>()
  private readonly checkpoints = new Map<string, RunCheckpoint>()

  setWalletBalance(ownerUserId: string, balanceCredits: number) {
    this.walletBalances.set(ownerUserId, balanceCredits)
  }

  /** 测试辅助：模拟持有 running 状态的旧副本崩溃后租约已过期（或历史无租约行）。 */
  expireRunLease(runId: string, expiredAtMs = Date.now() - 1_000) {
    this.runLeaseExpiresAt.set(runId, expiredAtMs)
  }

  recordedModelUsages() {
    return [...this.modelUsages]
  }

  async health(): Promise<boolean> { return true }
  async readiness() { return { database: true, schema: true } }

  async createConversation(ownerUserId: string, title: string, projectId?: string, titleSource?: ConversationTitleSource, modelId?: string): Promise<Conversation> {
    const now = new Date().toISOString()
    const value: Conversation = {
      id: createId("aicnv"), ownerUserId, title,
      titleSource: titleSource ?? (title === "新会话" ? "default" : "user"),
      status: "active", createdAt: now, updatedAt: now,
      ...(projectId ? { projectId } : {}),
      ...(modelId ? { modelId } : {}),
    }
    this.conversations.set(value.id, value)
    return value
  }

  async findEmptyConversation(ownerUserId: string, projectId?: string) {
    return [...this.conversations.values()]
      .filter(item => item.ownerUserId === ownerUserId
        && item.projectId === projectId
        && ![...this.turns.values()].some(turn => turn.conversationId === item.id))
      .sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))[0]
  }

  async listConversations(ownerUserId: string, page: number, pageSize: number, options: ConversationListOptions = {}) {
    const search = options.search?.trim().toLowerCase()
    const direction = options.sortOrder === "asc" ? 1 : -1
    const all = [...this.conversations.values()]
      .filter(item => item.ownerUserId === ownerUserId
        && (!search || item.title.toLowerCase().includes(search)))
      .sort((a, b) => direction * (a.updatedAt.localeCompare(b.updatedAt) || a.id.localeCompare(b.id)))
    return { items: all.slice((page - 1) * pageSize, page * pageSize), total: all.length }
  }

  async getConversation(ownerUserId: string, id: string) {
    const value = this.conversations.get(id)
    return value?.ownerUserId === ownerUserId ? value : undefined
  }

  async renameConversation(ownerUserId: string, id: string, title: string) {
    return this.updateConversation(ownerUserId, id, { title })
  }

  async updateConversation(ownerUserId: string, id: string, input: { title?: string, modelId?: string }) {
    const value = await this.getConversation(ownerUserId, id)
    if (!value) return undefined
    const next: Conversation = {
      ...value,
      ...(input.title ? { title: input.title, titleSource: "user" as const } : {}),
      ...(input.modelId ? { modelId: input.modelId } : {}),
      updatedAt: new Date().toISOString(),
    }
    this.conversations.set(id, next)
    if (input.title && (value.title !== input.title || value.titleSource !== "user")) {
      const latestRun = [...this.runs.values()]
        .filter(run => run.conversationId === id)
        .sort((left, right) => right.createdAt.localeCompare(left.createdAt) || right.id.localeCompare(left.id))[0]
      if (latestRun && latestRun.status !== "running") {
        await this.appendEventUnchecked(latestRun.id, "conversation.title.updated", {
          title: input.title,
          titleSource: "user",
          locked: true,
        })
      }
    }
    return next
  }

  async renameConversationByAssistant(id: string, title: string, runId?: string) {
    if (runId) this.checkRunningRunVersion(runId)
    const value = this.conversations.get(id)
    if (!value || value.titleSource === "user") return undefined
    if (runId && this.runs.get(runId)?.conversationId !== id) throw new Error("ai.run_conversation_mismatch")
    const next: Conversation = { ...value, title, titleSource: "assistant", updatedAt: new Date().toISOString() }
    this.conversations.set(id, next)
    if (runId) {
      await this.appendEventUnchecked(runId, "conversation.title.updated", {
        title,
        titleSource: "assistant",
        locked: false,
      })
    }
    return next
  }

  async listActiveConversationRuns(ownerUserId: string, conversationId: string): Promise<Run[]> {
    return [...this.runs.values()]
      .filter(run => run.ownerUserId === ownerUserId
        && run.conversationId === conversationId
        && ["queued", "running", "waiting_approval", "waiting_input", "waiting_external"].includes(run.status))
  }

  async deleteConversation(ownerUserId: string, id: string) {
    if (!await this.getConversation(ownerUserId, id)) return false
    for (const run of await this.listActiveConversationRuns(ownerUserId, id)) {
      await this.cancelRun(ownerUserId, run.id)
    }
    this.conversations.delete(id)
    this.summaries.delete(id)
    this.contextUsageByConversation.delete(id)
    const turnIds = [...this.turns.values()].filter(t => t.conversationId === id).map(t => t.id)
    for (const turnId of turnIds) this.turns.delete(turnId)
    for (const [runId, run] of this.runs) {
      if (run.conversationId !== id) continue
      this.runs.delete(runId)
      this.runLeaseExpiresAt.delete(runId)
      this.executionSnapshots.delete(runId)
    }
    return true
  }

  async assertPositiveAvailableBalance(ownerUserId: string): Promise<void> {
    const balance = this.walletBalances.get(ownerUserId) ?? 1_000_000
    if (!(balance > 0)) throw new Error("ai.wallet_balance_insufficient")
  }

  async createTurn(ownerUserId: string, input: CreateTurn): Promise<CreatedTurn> {
    const key = `${ownerUserId}:${input.idempotencyKey}`
    const previousLock = this.idempotencyLocks.get(key) ?? Promise.resolve()
    let releaseLock!: () => void
    const currentLock = new Promise<void>((resolve) => { releaseLock = resolve })
    this.idempotencyLocks.set(key, previousLock.then(() => currentLock, () => currentLock))
    await previousLock.catch(() => undefined)
    try {
      if (!await this.getConversation(ownerUserId, input.conversationId)) throw new Error("ai.conversation_not_found")
      const hash = createTurnRequestHash(input)
      const previous = this.idempotency.get(key)
      if (previous) {
        if (previous.hash !== hash) throw new Error("idempotency_conflict")
        const latestTurn = this.turns.get(previous.created.turn.id) ?? previous.created.turn
        const latestRun = this.runs.get(previous.created.run.id) ?? previous.created.run
        return { turn: latestTurn, run: latestRun, replayed: true }
      }
      if (input.pageContext.__lunaDirectToolAction !== true) {
        await this.assertPositiveAvailableBalance(ownerUserId)
      }
      const now = new Date().toISOString()
      if (input.modelId) {
        const conversation = this.conversations.get(input.conversationId)!
        this.conversations.set(input.conversationId, { ...conversation, modelId: input.modelId, updatedAt: now })
      }
      const previousTurns = [...this.turns.values()]
        .filter(t => t.conversationId === input.conversationId)
        .sort((left, right) => right.turnIndex - left.turnIndex)
      const turnIndex = previousTurns.length
      const previousConversationRun = input.pageContext.__lunaDirectToolAction === true
        ? undefined
        : previousTurns
            .map(t => this.runs.get(t.selectedRunId))
            .find((candidate): candidate is StoredRun => Boolean(candidate && candidate.pageContext.__lunaDirectToolAction !== true))
      const inheritedOperationIds = (previousConversationRun?.selectedOperationIds ?? [])
        .filter(operationId => /^[A-Za-z][A-Za-z0-9._-]{2,100}$/.test(operationId))
        .slice(-16)
      const runId = input.preallocatedRunId ?? createId("airun")
      const turn: Turn = {
        id: createId("aitrn"), conversationId: input.conversationId, turnIndex, status: "queued",
        input: input.input, selectedRunId: runId, createdAt: now,
        ...(input.modelId ? { modelId: input.modelId } : {}),
      }
      const run: StoredRun = {
        id: runId, conversationId: input.conversationId, turnId: turn.id, runIndex: 0,
        status: "queued", rowVersion: 1, promptVersion: "system-v4",
        toolCatalogDigest: input.toolCatalogDigest ?? "sha256:platform-tools-v1", pageContext: input.pageContext,
        selectedOperationIds: inheritedOperationIds,
        actorSessionId: input.actorSessionId ?? `test:${ownerUserId}`,
        ...(input.traceContext ? { traceContext: input.traceContext } : {}),
        createdAt: now, ownerUserId,
        ...(input.modelSnapshot ? { model: input.modelSnapshot } : {}),
      }
      this.turns.set(turn.id, turn)
      this.runs.set(run.id, run)
      const created = { turn, run }
      this.idempotency.set(key, { hash, created })
      const userItem = await this.appendItemUnchecked({ id: `${turn.id}:input`, runId, turnId: turn.id, type: "user_message", status: "completed", content: { parts: [{ type: "text", text: input.input }] } })
      await this.appendEventUnchecked(runId, "run.input_received", {
        initial: true,
        item: structuredClone(userItem),
        conversationTitle: this.conversations.get(input.conversationId)?.title,
        conversationTitleSource: this.conversations.get(input.conversationId)?.titleSource,
      })
      await this.appendEventUnchecked(runId, "run.queued", { state: "queued" })
      return { ...created, replayed: false }
    }
    finally {
      releaseLock()
    }
  }

  async getRun(ownerUserId: string, id: string) {
    const value = this.runs.get(id)
    return value?.ownerUserId === ownerUserId ? value : undefined
  }
  async assertRunningRunVersion(runId: string, expectedRunVersion: number): Promise<void> {
    this.checkRunningRunVersion(runId, expectedRunVersion)
  }
  private checkRunningRunVersion(runId: string, expectedRunVersion?: number): void {
    const effective = expectedRunVersion ?? activeRunningRunVersionFence(runId)
    if (effective === undefined) return
    const current = this.runs.get(runId)
    if (!current || current.status !== "running" || current.rowVersion !== effective) {
      throw new RunStateConflictError(runId, "running", "running", current?.status, current?.rowVersion)
    }
  }
  async getRunToolState(runId: string) {
    const run = this.runs.get(runId)
    return run ? { toolCatalogDigest: run.toolCatalogDigest, selectedOperationIds: [...run.selectedOperationIds] } : undefined
  }
  async getRunStreamPosition(runId: string) {
    const run = this.runs.get(runId)
    if (!run) return undefined
    return {
      nextItemPosition: this.items.filter(item => item.runId === runId).length,
      nextEventSequence: Math.max(
        this.streamHighWatermarks.get(runId) ?? 0,
        ...this.events.filter(event => event.runId === runId).map(event => event.sequence),
      ) + 1,
      runVersion: run.rowVersion,
    }
  }
  async persistRunStreamBatch(batch: RunStreamBatch): Promise<void> {
    const position = await this.getRunStreamPosition(batch.runId)
    if (!position) throw new Error("ai.run_not_found")
    const existingItemCount = batch.items.filter(item => this.items.some(candidate => candidate.id === item.id)).length
    const existingEventCount = batch.events.filter(event => this.events.some(candidate => candidate.id === event.id)).length
    const run = this.runs.get(batch.runId)!
    const alreadyPersisted = existingItemCount === batch.items.length && existingEventCount === batch.events.length
      && (this.streamHighWatermarks.get(batch.runId) ?? 0) >= batch.eventHighWatermark
    if (alreadyPersisted && (!batch.terminal || run.status === batch.terminal.to)) return
    if (run.rowVersion !== batch.expectedRunVersion || run.status !== "running") {
      throw new RunStateConflictError(batch.runId, batch.terminal?.from ?? "running", batch.terminal?.to ?? "running", run.status, run.rowVersion)
    }
    if (existingItemCount || existingEventCount) throw new Error("ai.stream_batch_partial_conflict")
    if (position.nextItemPosition !== batch.expected.nextItemPosition) throw new Error("ai.stream_item_position_conflict")
    if (batch.events.some(event => this.events.some(candidate => candidate.runId === batch.runId && candidate.sequence === event.sequence))) {
      throw new Error("ai.stream_sequence_conflict")
    }
    this.items.push(...batch.items.map(item => structuredClone(item)))
    this.events.push(...batch.events.map(event => structuredClone(event)))
    this.streamHighWatermarks.set(batch.runId, batch.eventHighWatermark)
    if (batch.terminal) {
      const finalizedStatus = batch.terminal.to === "completed" ? "completed" : "failed"
      await this.finalizeStreamingItems(batch.runId, finalizedStatus)
      if (batch.terminal.conversationTitle) {
        const conversation = this.conversations.get(run.conversationId)
        if (conversation && conversation.titleSource !== "user") {
          conversation.title = batch.terminal.conversationTitle
          conversation.titleSource = "assistant"
          await this.appendEventUnchecked(batch.runId, "conversation.title.updated", {
            title: batch.terminal.conversationTitle, titleSource: "assistant", locked: false,
          })
        }
      }
      delete run.wakeAt
      this.runLeaseExpiresAt.delete(batch.runId)
      Object.assign(run, {
        status: batch.terminal.to,
        completedAt: batch.terminal.completedAt,
        ...(batch.terminal.errorCode ? { errorCode: batch.terminal.errorCode } : {}),
        rowVersion: run.rowVersion + 1,
      })
      const turn = this.turns.get(run.turnId)
      if (turn) turn.status = batch.terminal.to
      await this.appendEventUnchecked(batch.runId, `run.${batch.terminal.to}`, {
        state: batch.terminal.to,
        rowVersion: run.rowVersion,
        ...(batch.terminal.errorCode ? { errorCode: batch.terminal.errorCode } : {}),
      })
    }
  }
  async touchRunSelectedOperations(
    runId: string,
    operationIds: string[],
    limit: number,
    protectedOperationIds?: ReadonlySet<string>,
  ) {
    this.checkRunningRunVersion(runId)
    const run = this.runs.get(runId)
    if (!run) throw new Error("ai.run_not_found")
    const requested = [...new Set(operationIds)].filter(operationId => /^[A-Za-z][A-Za-z0-9._-]{2,100}$/.test(operationId))
    const executedInRun = new Set<string>([
      ...[...this.toolCallStore.records.values()]
        .filter(call => call.runId === runId && call.inputMode !== "verification" && !(call.status === "skipped" && call.executionCount === 0))
        .map(call => call.operationId),
      ...(protectedOperationIds ?? []),
    ])
    const selection = selectOperationsWithExecutedProtection(run.selectedOperationIds, requested, executedInRun, limit)
    run.selectedOperationIds = [...selection.selectedOperationIds]
    return selection
  }
  async listActiveToolCatalogDigests() {
    return [...new Set([...this.runs.values()]
      .filter(run => ["queued", "running", "waiting_approval", "waiting_input", "waiting_external"].includes(run.status))
      .map(run => run.toolCatalogDigest))]
  }

  async cancelRun(ownerUserId: string, id: string, expectedRowVersion?: number) {
    const run = this.runs.get(id)
    if (run?.ownerUserId !== ownerUserId) return undefined
    if (!run || ["completed", "failed", "canceled", "expired", "interrupted"].includes(run.status)) return run
    if (expectedRowVersion !== undefined && run.rowVersion !== expectedRowVersion) return run
    run.status = "canceled"
    delete run.wakeAt
    this.runLeaseExpiresAt.delete(id)
    run.completedAt = new Date().toISOString()
    run.rowVersion += 1
    const turn = this.turns.get(run.turnId)
    if (turn) turn.status = "canceled"
    const canceledResult = { code: "ai.run_canceled", retryable: false }
    const activeToolStatuses = new Set(["proposed", "awaiting_approval", "running"])
    for (const call of this.toolCallStore.records.values()) {
      if (call.runId !== id || !activeToolStatuses.has(call.status)) continue
      await this.toolCallStore.update(call.id, call.status, {
        status: "canceled",
        rowVersion: call.rowVersion + 1,
        result: canceledResult,
        errorCode: "ai.run_canceled",
      })
    }
    for (const item of this.items) {
      if (item.runId !== id) continue
      if (item.type === "tool_call") {
        const contentStatus = typeof item.content.status === "string" ? item.content.status : undefined
        const active = activeToolStatuses.has(contentStatus ?? "") || item.status === "streaming"
        if (!active) continue
        item.status = "completed"
        item.content = { ...item.content, status: "canceled", result: canceledResult, errorCode: "ai.run_canceled" }
        item.revision += 1
        await this.appendEventUnchecked(id, "item.finalized", {
          itemId: item.id,
          ...(typeof item.content.toolCallId === "string" ? { toolCallId: item.content.toolCallId } : {}),
          item: structuredClone(item),
        })
      }
      else if (item.status === "streaming") {
        item.status = "failed"
        item.revision += 1
        await this.appendEventUnchecked(id, "item.finalized", { itemId: item.id, item: structuredClone(item) })
      }
    }
    await this.appendEventUnchecked(id, "run.canceled", { state: "canceled", rowVersion: run.rowVersion })
    return run
  }

  async claimNextQueuedRun(executionSnapshot?: RunExecutionSnapshot, userConcurrentRuns?: number): Promise<ClaimedRun | undefined> {
    const maxPerUser = userConcurrentRuns ?? executionSnapshot?.runtimeSettings.userConcurrentRuns
    const run = [...this.runs.values()].find((item) => {
      if (item.status !== "queued") return false
      if (item.pageContext.__lunaDirectToolAction === true) {
        const readyDirect = [...this.toolCallStore.records.values()].some(call =>
          call.runId === item.id
          && call.inputMode === "direct"
          && ((call.status === "proposed" && !call.errorCode) || call.status === "rejected" || call.executionCount > 0)
          && this.items.some(timelineItem => timelineItem.runId === item.id && timelineItem.id === `${call.id}:item`),
        )
        if (!readyDirect) return false
      }
      if (maxPerUser === undefined) return true
      let runningCount = 0
      for (const candidate of this.runs.values()) {
        if (candidate.ownerUserId === item.ownerUserId && candidate.status === "running")
          runningCount += 1
      }
      return runningCount < Math.max(1, Math.floor(maxPerUser))
    })
    if (!run) return undefined
    if (!this.executionSnapshots.has(run.id) && executionSnapshot) {
      const snapshot = cloneExecutionSnapshot(executionSnapshot)
      this.executionSnapshots.set(run.id, snapshot)
      if (snapshot.toolCatalogDigest) run.toolCatalogDigest = snapshot.toolCatalogDigest
    }
    run.status = "running"
    delete run.wakeAt
    this.runLeaseExpiresAt.set(run.id, Date.now() + 60_000)
    run.startedAt ??= new Date().toISOString()
    run.rowVersion += 1
    const turn = this.turns.get(run.turnId)
    if (turn) turn.status = "running"
    await this.appendEventUnchecked(run.id, "run.running", { state: "running", rowVersion: run.rowVersion })
    const restoredSnapshot = this.executionSnapshots.get(run.id)
    return {
      ...run,
      ...(restoredSnapshot ? { executionSnapshot: cloneExecutionSnapshot(restoredSnapshot) } : {}),
    }
  }

  async renewRunLease(runId: string, expectedRowVersion: number, leaseMs: number): Promise<RunLeaseRenewalOutcome> {
    const run = this.runs.get(runId)
    if (!run) return "lost"
    const leaseExpiresAt = this.runLeaseExpiresAt.get(runId)
    if (
      run.status === "running"
      && run.rowVersion === expectedRowVersion
      && leaseExpiresAt !== undefined
      && leaseExpiresAt > Date.now()
    ) {
      const boundedLeaseMs = Math.max(1_000, Math.floor(leaseMs))
      this.runLeaseExpiresAt.set(runId, Date.now() + boundedLeaseMs)
      return "renewed"
    }
    return run.status === "canceled" ? "canceled" : "lost"
  }

  async updateRunToolCatalogDigest(runId: string, toolCatalogDigest: string, expectedRowVersion?: number): Promise<void> {
    const run = this.runs.get(runId)
    if (!run || (expectedRowVersion !== undefined && run.rowVersion !== expectedRowVersion)) return
    run.toolCatalogDigest = toolCatalogDigest
    const existingSnapshot = this.executionSnapshots.get(runId)
    if (existingSnapshot) {
      this.executionSnapshots.set(runId, cloneExecutionSnapshot({
        runtimeSettings: existingSnapshot.runtimeSettings,
        toolCatalogDigest,
      }))
    }
  }

  async claimDueWaitingExternalRuns(limit: number, leaseMs: number, now?: Date): Promise<Run[]> {
    const boundedLimit = Math.max(1, Math.min(64, Math.floor(limit)))
    const boundedLeaseMs = Math.max(1_000, Math.floor(leaseMs))
    const referenceMs = (now ?? new Date()).getTime()
    const nextLeaseIso = new Date(referenceMs + boundedLeaseMs).toISOString()
    const due = [...this.runs.values()]
      .filter(run => run.status === "waiting_external" && run.wakeAt !== undefined && Date.parse(run.wakeAt) <= referenceMs)
      .sort((a, b) => (a.wakeAt ?? "").localeCompare(b.wakeAt ?? "") || a.id.localeCompare(b.id))
      .slice(0, boundedLimit)
    for (const run of due) {
      run.wakeAt = nextLeaseIso
    }
    return due.map(run => structuredClone(run))
  }

  async rescheduleWaitingExternalRun(
    runId: string,
    expectedCheckpointRevision: number,
    pollCount: number,
    wakeAt: string,
  ): Promise<boolean> {
    const run = this.runs.get(runId)
    const checkpoint = this.checkpoints.get(runId)
    if (!run || run.status !== "waiting_external" || !checkpoint || checkpoint.revision !== expectedCheckpointRevision || !checkpoint.state.waitingTask)
      return false
    checkpoint.revision = expectedCheckpointRevision + 1
    checkpoint.state = {
      ...structuredClone(checkpoint.state),
      waitingTask: {
        ...structuredClone(checkpoint.state.waitingTask),
        pollCount,
      },
    }
    checkpoint.updatedAt = new Date().toISOString()
    run.wakeAt = wakeAt
    return true
  }

  async resolveWaitingExternalRun(input: ResolveWaitingExternalRunInput): Promise<boolean> {
    const run = this.runs.get(input.runId)
    const checkpoint = this.checkpoints.get(input.runId)
    if (!run || run.status !== "waiting_external" || !checkpoint || checkpoint.revision !== input.expectedCheckpointRevision)
      return false
    const item = await this.updateItemUnchecked(input.itemId, input.itemStatus, input.itemContent)
    await this.appendEventUnchecked(input.runId, input.eventType, {
      ...input.eventData,
      itemId: item.id,
      toolCallId: input.toolCallId,
      item: structuredClone(item),
    })
    checkpoint.revision = input.expectedCheckpointRevision + 1
    checkpoint.phase = input.nextPhase
    checkpoint.state = structuredClone(input.nextCheckpointState)
    checkpoint.updatedAt = new Date().toISOString()
    run.status = "queued"
    delete run.wakeAt
    this.runLeaseExpiresAt.delete(input.runId)
    run.rowVersion += 1
    const turn = this.turns.get(run.turnId)
    if (turn) turn.status = "queued"
    await this.appendEventUnchecked(input.runId, "run.queued", { state: "queued", rowVersion: run.rowVersion })
    return true
  }
  async listRunningRuns(now?: Date) {
    const referenceMs = (now ?? new Date()).getTime()
    return [...this.runs.values()]
      .filter((run) => {
        if (run.status !== "running") return false
        const leaseExpiresAt = this.runLeaseExpiresAt.get(run.id)
        return leaseExpiresAt === undefined || leaseExpiresAt <= referenceMs
      })
      .map(run => ({ id: run.id, rowVersion: run.rowVersion }))
  }
  async interruptOrphanedRun(runId: string, expectedRunVersion: number, options?: InterruptRunOptions): Promise<boolean> {
    const run = this.runs.get(runId)
    if (!run || run.status !== "running" || run.rowVersion !== expectedRunVersion) return false
    if (options?.ignoreActiveLease !== true) {
      const leaseExpiresAt = this.runLeaseExpiresAt.get(runId)
      if (leaseExpiresAt !== undefined && leaseExpiresAt > Date.now()) return false
    }
    await this.updateRun(runId, "running", "interrupted", {
      completedAt: new Date().toISOString(),
      errorCode: "ai.agent_restarted",
      expectedRowVersion: expectedRunVersion,
    })
    return true
  }
  async recoverOrphanedRun(runId: string, expectedRunVersion: number, maxRecoveries: number) {
    const run = this.runs.get(runId)
    if (!run || run.status !== "running" || run.rowVersion !== expectedRunVersion) return "skipped" as const
    const leaseExpiresAt = this.runLeaseExpiresAt.get(runId)
    if (leaseExpiresAt !== undefined && leaseExpiresAt > Date.now()) return "skipped" as const
    const checkpoint = this.checkpoints.get(runId)
    if (!checkpoint || checkpoint.recoveries >= maxRecoveries) {
      const interrupted = await this.interruptOrphanedRun(runId, expectedRunVersion)
      return interrupted ? "interrupted" as const : "skipped" as const
    }
    checkpoint.recoveries += 1
    checkpoint.revision += 1
    checkpoint.updatedAt = new Date().toISOString()
    run.status = "queued"
    this.runLeaseExpiresAt.delete(runId)
    run.rowVersion += 1
    const turn = this.turns.get(run.turnId)
    if (turn) turn.status = "queued"
    await this.appendEventUnchecked(runId, "run.requeued", {
      state: "queued", rowVersion: run.rowVersion, reason: "ai.agent_restarted", recoveries: checkpoint.recoveries,
    })
    return "requeued" as const
  }
  async loadRunCheckpoint(runId: string) {
    const value = this.checkpoints.get(runId)
    return value ? structuredClone(value) : undefined
  }
  async saveRunCheckpoint(runId: string, advance: RunCheckpointAdvance) {
    this.checkRunningRunVersion(runId, advance.expectedRunVersion)
    const current = this.checkpoints.get(runId)
    const revision = advance.expectedRevision + 1
    if (current && current.revision === revision) {
      return structuredClone(current)
    }
    if (current ? current.revision !== advance.expectedRevision : advance.expectedRevision !== 0)
      throw new RunCheckpointConflictError(runId, advance.expectedRevision, current?.revision)
    const now = new Date().toISOString()
    const value: RunCheckpoint = {
      runId,
      revision,
      phase: advance.phase,
      modelStep: advance.modelStep,
      toolCallCount: advance.toolCallCount,
      recoveries: current?.recoveries ?? 0,
      state: structuredClone(advance.state),
      createdAt: current?.createdAt ?? now,
      updatedAt: now,
    }
    this.checkpoints.set(runId, value)
    return structuredClone(value)
  }
  async countActiveUserRuns(userId: string) {
    let count = 0
    for (const run of this.runs.values()) {
      if (run.ownerUserId === userId && run.status === "running") {
        count += 1
      }
    }
    return count
  }

  async recordReportedModelUsage(input: RecordReportedModelUsageInput): Promise<{ id: string, attempt: number }> {
    const run = this.runs.get(input.runId)
    if (!run && !input.model) throw new Error("ai.run_not_found")
    if (run && run.ownerUserId !== input.ownerUserId) throw new Error("ai.run_not_found")
    const attempt = this.modelUsages.filter(item => item.runId === input.runId && item.operation === input.operation).length + 1
    const id = createId("aiuse")
    this.modelUsages.push({
      id,
      runId: input.runId,
      ownerUserId: input.ownerUserId,
      operation: input.operation,
      attempt,
      usage: input.usage,
    })
    const model = input.model ?? run?.model
    if (input.operation === "assistant" && run && model) {
      this.contextUsageByConversation.set(run.conversationId, {
        status: "reported",
        runId: run.id,
        modelId: model.id,
        usedTokens: input.usage.totalTokens,
        maxContextTokensSnapshot: model.maxContextTokens,
        recordedAt: new Date().toISOString(),
      })
    }
    return { id, attempt }
  }

  async getExecutionInput(runId: string) {
    const run = this.runs.get(runId)
    const turn = run ? this.turns.get(run.turnId) : undefined
    const conversation = run ? this.conversations.get(run.conversationId) : undefined
    const contextUsage = run ? this.contextUsageByConversation.get(run.conversationId) : undefined
    const history = run && turn
      ? this.conversationHistory(run.conversationId, -1, turn.turnIndex, 8, true)
      : []
    return run && turn && conversation
      ? {
          turnId: turn.id,
          conversationId: run.conversationId,
          turnIndex: turn.turnIndex,
          input: turn.input,
          pageContext: run.pageContext,
          toolCatalogDigest: run.toolCatalogDigest,
          selectedOperationIds: [...run.selectedOperationIds],
          ...(contextUsage ? { lastTotalTokens: contextUsage.usedTokens } : {}),
          ...(run.model ? { model: run.model } : {}),
          toolInteractions: this.items
            .filter(item => item.runId === runId && item.id !== `${turn.id}:input` && ["reasoning_summary", "progress", "tool_call", "tool_result", "user_message"].includes(item.type))
            .sort((a, b) => a.timelineIndex - b.timelineIndex)
            .map(item => ({ itemId: item.id, type: item.type as ConversationToolInteraction["type"], status: item.status, content: item.content })),
          history,
          conversation: { title: conversation.title, titleSource: conversation.titleSource },
        }
      : undefined
  }

  private conversationHistory(
    conversationId: string,
    afterTurnIndex: number,
    beforeTurnIndex: number,
    limit: number,
    fromEnd = false,
  ): ConversationHistoryEntry[] {
    const ordered = [...this.turns.values()]
      .filter(item => item.conversationId === conversationId
        && item.turnIndex > afterTurnIndex
        && item.turnIndex < beforeTurnIndex)
      .sort((a, b) => a.turnIndex - b.turnIndex)
    const bounded = fromEnd ? ordered.slice(-limit) : ordered.slice(0, limit)
    return bounded
      .map((item) => {
        const selectedRun = this.runs.get(item.selectedRunId)
        const supplementalInputs = this.items
          .filter(candidate => candidate.runId === item.selectedRunId && candidate.type === "user_message" && candidate.id !== `${item.id}:input`)
          .sort((a, b) => a.timelineIndex - b.timelineIndex)
          .map(candidate => timelineText(candidate.content))
          .filter(Boolean)
        const user = supplementalInputs.length > 0 ? `${item.input}\n\n${supplementalInputs.join("\n\n")}` : item.input
        const assistant = this.items
          .filter(candidate => candidate.runId === item.selectedRunId && candidate.type === "assistant_message")
          .sort((a, b) => a.timelineIndex - b.timelineIndex)
          .map(candidate => timelineText(candidate.content))
          .filter(Boolean)
          .join("\n")
        const reasoning = this.items
          .filter(candidate => candidate.runId === item.selectedRunId && candidate.type === "reasoning_summary")
          .sort((a, b) => a.timelineIndex - b.timelineIndex)
          .map(candidate => (typeof candidate.content.summary === "string" ? candidate.content.summary : ""))
          .filter(Boolean)
          .join("\n")
        const toolInteractions = this.items
          .filter(candidate => candidate.runId === item.selectedRunId && ["tool_call", "tool_result"].includes(candidate.type))
          .sort((a, b) => a.timelineIndex - b.timelineIndex)
          .map(candidate => ({ type: candidate.type, status: candidate.status, content: structuredClone(candidate.content) }))
        return {
          turnIndex: item.turnIndex,
          user,
          assistant,
          ...(reasoning ? { reasoning } : {}),
          pageContext: structuredClone(selectedRun?.pageContext ?? {}),
          ...(toolInteractions.length ? { toolInteractions } : {}),
        }
      })
  }
  async getConversationSummary(conversationId: string) {
    return this.summaries.get(conversationId)
  }
  async listConversationHistory(conversationId: string, afterTurnIndex: number, beforeTurnIndex: number, limit: number) {
    return this.conversationHistory(conversationId, afterTurnIndex, beforeTurnIndex, Math.max(0, limit))
  }
  async saveConversationSummary(input: Omit<ConversationSummary, "createdAt" | "updatedAt">) {
    const previous = this.summaries.get(input.conversationId)
    if (previous && previous.coveredThroughTurnIndex >= input.coveredThroughTurnIndex) return previous
    const now = new Date().toISOString()
    const value: ConversationSummary = { ...input, createdAt: previous?.createdAt ?? now, updatedAt: now }
    this.summaries.set(input.conversationId, value)
    return value
  }
  async appendRunInput(runId: string, text: string) {
    const run = this.runs.get(runId)
    const turn = run ? this.turns.get(run.turnId) : undefined
    if (!run || !turn) throw new Error("ai.run_not_found")
    return this.appendItem({
      runId,
      turnId: turn.id,
      type: "user_message",
      status: "completed",
      content: { parts: [{ type: "text", text }] },
    })
  }

  async updateRun(runId: string, from: Run["status"], to: Run["status"], fields: UpdateRunOptions = {}) {
    const run = this.runs.get(runId)
    if (!run || run.status !== from || (fields.expectedRowVersion !== undefined && run.rowVersion !== fields.expectedRowVersion)) {
      throw new RunStateConflictError(runId, from, to, run?.status, run?.rowVersion)
    }
    const { clearWakeAt, expectedRowVersion, eventData, ...restFields } = fields
    void expectedRowVersion
    Object.assign(run, restFields, { status: to, rowVersion: run.rowVersion + 1 })
    if (clearWakeAt || to !== "waiting_external") delete run.wakeAt
    if (to === "running") this.runLeaseExpiresAt.set(runId, Date.now() + 60_000)
    else this.runLeaseExpiresAt.delete(runId)
    const turn = this.turns.get(run.turnId)
    if (turn) turn.status = to
    await this.appendEventUnchecked(runId, `run.${to}`, {
      state: to,
      rowVersion: run.rowVersion,
      ...(restFields.errorCode ? { errorCode: restFields.errorCode } : {}),
      ...(eventData ?? {}),
    })
    return run
  }

  async appendItem(value: Omit<TimelineItem, "id" | "timelineIndex" | "revision" | "createdAt"> & { id?: string }) {
    this.checkRunningRunVersion(value.runId)
    return this.appendItemUnchecked(value)
  }

  private async appendItemUnchecked(value: Omit<TimelineItem, "id" | "timelineIndex" | "revision" | "createdAt"> & { id?: string }) {
    const timelineIndex = this.items.filter(item => item.runId === value.runId).length
    const item: TimelineItem = { ...value, id: value.id ?? createId("aiitm"), timelineIndex, revision: 1, createdAt: new Date().toISOString() }
    this.items.push(item)
    if (value.type === "user_message" || value.type === "assistant_message") {
      const run = this.runs.get(value.runId)
      const conversation = run ? this.conversations.get(run.conversationId) : undefined
      if (conversation)
        this.conversations.set(conversation.id, { ...conversation, updatedAt: item.createdAt })
    }
    return item
  }

  async updateItem(itemId: string, status: TimelineItem["status"], content: Record<string, unknown>) {
    const item = this.items.find(candidate => candidate.id === itemId)
    if (!item) throw new Error("ai.item_not_found")
    this.checkRunningRunVersion(item.runId)
    return this.updateItemUnchecked(itemId, status, content)
  }

  private async updateItemUnchecked(itemId: string, status: TimelineItem["status"], content: Record<string, unknown>) {
    const item = this.items.find(candidate => candidate.id === itemId)
    if (!item) throw new Error("ai.item_not_found")
    item.status = status
    item.content = content
    item.revision += 1
    return item
  }

  async appendItemWithEvent(
    value: Omit<TimelineItem, "id" | "timelineIndex" | "revision" | "createdAt"> & { id?: string },
    eventType: string,
    eventData: Record<string, unknown> = {},
  ) {
    this.checkRunningRunVersion(value.runId)
    const item = await this.appendItemUnchecked(value)
    const event = await this.appendEventUnchecked(item.runId, eventType, { ...eventData, item: structuredClone(item) })
    return { item, event }
  }

  async updateItemWithEvent(
    itemId: string,
    status: TimelineItem["status"],
    content: Record<string, unknown>,
    eventType: string,
    eventData: Record<string, unknown> = {},
  ) {
    const existing = this.items.find(candidate => candidate.id === itemId)
    if (!existing) throw new Error("ai.item_not_found")
    this.checkRunningRunVersion(existing.runId)
    const item = await this.updateItemUnchecked(itemId, status, content)
    const event = await this.appendEventUnchecked(item.runId, eventType, { ...eventData, item: structuredClone(item) })
    return { item, event }
  }

  async completeToolItemWithEvent(
    itemId: string,
    status: TimelineItem["status"],
    content: Record<string, unknown>,
    resultValue: Omit<TimelineItem, "id" | "timelineIndex" | "revision" | "createdAt"> & { id?: string },
    eventType: string,
    eventData: Record<string, unknown> = {},
  ) {
    this.checkRunningRunVersion(resultValue.runId)
    const item = await this.updateItemUnchecked(itemId, status, content)
    const resultItem = await this.appendItemUnchecked(resultValue)
    if (resultItem.runId !== item.runId || resultItem.turnId !== item.turnId)
      throw new Error("ai.tool_result_binding_mismatch")
    const event = await this.appendEventUnchecked(item.runId, eventType, {
      ...eventData,
      item: structuredClone(item),
      resultItem: structuredClone(resultItem),
    })
    return { item, resultItem, event }
  }

  async finalizeStreamingItems(runId: string, status: Exclude<TimelineItem["status"], "streaming">, expectedRowVersion?: number) {
    if (expectedRowVersion !== undefined) {
      const run = this.runs.get(runId)
      if (!run || run.rowVersion !== expectedRowVersion) return
    }
    const terminalToolStatuses = new Set(["succeeded", "failed", "rejected", "canceled", "skipped"])
    const items = this.items.filter(item => item.runId === runId && item.status === "streaming")
    for (const item of items) {
      item.status = item.type === "tool_call" ? "failed" : status
      if (item.type === "tool_call" && (typeof item.content.status !== "string" || !terminalToolStatuses.has(item.content.status))) {
        item.content = { ...item.content, status: "failed" }
      }
      item.revision += 1
      await this.appendEventUnchecked(runId, "item.finalized", { item: structuredClone(item) })
    }
  }

  async appendEvent(runId: string, type: string, data: Record<string, unknown>) {
    this.checkRunningRunVersion(runId)
    return this.appendEventUnchecked(runId, type, data)
  }

  private async appendEventUnchecked(runId: string, type: string, data: Record<string, unknown>) {
    const sequence = Math.max(
      this.streamHighWatermarks.get(runId) ?? 0,
      ...this.events.filter(event => event.runId === runId).map(event => event.sequence),
    ) + 1
    const event: RunEvent = { id: createId("aievt"), runId, sequence, type, data, createdAt: new Date().toISOString() }
    this.events.push(event)
    return event
  }

  async getEvents(ownerUserId: string, runId: string, after: number) {
    if (!await this.getRun(ownerUserId, runId)) return []
    return this.events.filter(event => event.runId === runId && event.sequence > after)
  }

  async getTimeline(ownerUserId: string, conversationId: string, options: TimelinePageOptions = {}) {
    const conversation = await this.getConversation(ownerUserId, conversationId)
    if (!conversation) return undefined
    const contextUsage = this.contextUsageByConversation.get(conversationId)
    const limit = Math.max(1, Math.min(100, Math.trunc(options.limit ?? 30)))
    const recentTurns = [...this.turns.values()]
      .filter(turn => turn.conversationId === conversationId
        && (options.beforeTurnIndex === undefined || turn.turnIndex < options.beforeTurnIndex))
      .sort((a, b) => b.turnIndex - a.turnIndex)
      .slice(0, limit + 1)
    const hasOlder = recentTurns.length > limit
    const boundedTurns = recentTurns.slice(0, limit).reverse()
    const pageTurns = boundedTurns.map((turn) => {
      const storedRun = this.runs.get(turn.selectedRunId)
      const usages = storedRun
        ? this.modelUsages.filter(item => item.runId === storedRun.id && item.operation === "assistant")
        : []
      const latestAssistant = usages.at(-1)
      const run = storedRun
        ? {
            ...storedRun,
            ...(latestAssistant && storedRun.model ? {
              latestPromptTokens: latestAssistant.usage.inputTokens,
              latestUsageModelId: storedRun.model.id,
              latestUsageMaxContextTokensSnapshot: storedRun.model.maxContextTokens,
            } : {}),
          }
        : undefined
      return {
        id: turn.id,
        turnIndex: turn.turnIndex,
        status: turn.status,
        input: turn.input,
        createdAt: turn.createdAt,
        ...(run ? { run } : {}),
        items: this.items.filter(item => item.runId === run?.id).sort((a, b) => a.timelineIndex - b.timelineIndex),
      }
    })
    const eventCursors = pageTurns.flatMap((turn) => {
      if (!turn.run) return []
      const runId = turn.run.id
      return [{
        runId,
        after: this.events
          .filter(event => event.runId === runId)
          .reduce((maximum, event) => Math.max(maximum, event.sequence), 0),
      }]
    })
    return {
      conversation,
      ...(contextUsage ? { contextUsage } : {}),
      turns: pageTurns,
      eventCursors,
      pageInfo: {
        hasOlder,
        ...(hasOlder && pageTurns[0] ? { oldestTurnIndex: pageTurns[0].turnIndex } : {}),
      },
    }
  }
}

function cloneExecutionSnapshot(snapshot: RunExecutionSnapshot): RunExecutionSnapshot {
  return Object.freeze({
    runtimeSettings: runtimeSettingsSnapshot(snapshot.runtimeSettings),
    ...(snapshot.toolCatalogDigest ? { toolCatalogDigest: snapshot.toolCatalogDigest } : {}),
  })
}

function timelineText(content: Record<string, unknown>) {
  if (!Array.isArray(content.parts)) return ""
  return content.parts
    .map((part: unknown) => {
      if (!part || typeof part !== "object") return ""
      const value = part as Record<string, unknown>
      return typeof value.text === "string" ? value.text : ""
    })
    .join("")
}
