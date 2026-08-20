import type {
  Conversation,
  ConversationHistoryEntry,
  ConversationSummary,
  ConversationTitleSource,
  CreatedTurn,
  CreateTurn,
  Run,
  RunEvent,
  TimelineItem,
  Turn,
  UIActionAcknowledgement,
  UIActionDelivery,
} from "../domain.js"
import { createId } from "../id.js"
import {
  RunStateConflictError,
  type ConversationListOptions,
  type Repository,
  type TimelinePageOptions,
  type ModelBudgetOperation,
  type ModelBudgetUsage,
} from "./repository.js"
import { createTurnRequestHash } from "./create-turn-hash.js"

type StoredRun = Run & { ownerUserId: string, leaseOwner?: string, leaseExpiresAt?: number, runActorGrantCiphertext?: string }

export class MemoryRepository implements Repository {
  private readonly conversations = new Map<string, Conversation>()
  private readonly turns = new Map<string, Turn>()
  private readonly runs = new Map<string, StoredRun>()
  private readonly items: TimelineItem[] = []
  private readonly events: RunEvent[] = []
  private readonly uiActions = new Map<string, UIActionDelivery>()
  private readonly summaries = new Map<string, ConversationSummary>()
  private readonly idempotency = new Map<string, { hash: string, created: CreatedTurn }>()
  private readonly modelReservations = new Map<string, {
    runId: string
    operation: ModelBudgetOperation
    inputTokens: number
    tokens: number
    state: "reserved" | "confirmed" | "released"
    maxOutputTokens: number
  }>()

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
    return next
  }

  async renameConversationByAssistant(id: string, title: string) {
    const value = this.conversations.get(id)
    if (!value || value.titleSource === "user") return undefined
    const next: Conversation = { ...value, title, titleSource: "assistant", updatedAt: new Date().toISOString() }
    this.conversations.set(id, next)
    return next
  }

  async deleteConversation(ownerUserId: string, id: string) {
    if (!await this.getConversation(ownerUserId, id)) return false
    this.conversations.delete(id)
    this.summaries.delete(id)
    const turnIds = [...this.turns.values()].filter(t => t.conversationId === id).map(t => t.id)
    for (const turnId of turnIds) this.turns.delete(turnId)
    for (const [runId, run] of this.runs) if (run.conversationId === id) this.runs.delete(runId)
    return true
  }

  async createTurn(ownerUserId: string, input: CreateTurn): Promise<CreatedTurn> {
    if (!await this.getConversation(ownerUserId, input.conversationId)) throw new Error("ai.conversation_not_found")
    const hash = createTurnRequestHash(input)
    const key = `${ownerUserId}:${input.idempotencyKey}`
    const previous = this.idempotency.get(key)
    if (previous) {
      if (previous.hash !== hash) throw new Error("idempotency_conflict")
      return previous.created
    }
    const now = new Date().toISOString()
    if (input.modelId) {
      const conversation = this.conversations.get(input.conversationId)!
      this.conversations.set(input.conversationId, { ...conversation, modelId: input.modelId, updatedAt: now })
    }
    const turnIndex = [...this.turns.values()].filter(t => t.conversationId === input.conversationId).length
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
      ...(input.traceContext ? { traceContext: input.traceContext } : {}),
      clientInstanceId: input.clientInstanceId ?? "memory-client-instance", createdAt: now, ownerUserId,
      ...(input.runActorGrantCiphertext ? { runActorGrantCiphertext: input.runActorGrantCiphertext } : {}),
      ...(input.modelSnapshot ? { model: input.modelSnapshot } : {}),
      budget: input.runBudgetSnapshot ?? { totalTokens: 2_000_000, totalCredits: "10000" },
    }
    this.turns.set(turn.id, turn)
    this.runs.set(run.id, run)
    const created = { turn, run }
    this.idempotency.set(key, { hash, created })
    await this.appendItem({ runId, turnId: turn.id, type: "user_message", status: "completed", content: { parts: [{ type: "text", text: input.input }] } })
    await this.appendEvent(runId, "run.queued", { state: "queued" })
    return created
  }

  async getRun(ownerUserId: string, id: string) {
    const value = this.runs.get(id)
    return value?.ownerUserId === ownerUserId ? value : undefined
  }

  async cancelRun(ownerUserId: string, id: string) {
    const run = this.runs.get(id)
    if (run?.ownerUserId !== ownerUserId) return undefined
    if (!run || ["completed", "failed", "canceled", "expired"].includes(run.status)) return run
    run.status = "canceled"
    run.completedAt = new Date().toISOString()
    run.rowVersion += 1
    const turn = this.turns.get(run.turnId)
    if (turn) turn.status = "canceled"
    await this.appendEvent(id, "run.canceled", { state: "canceled", rowVersion: run.rowVersion })
    return run
  }

  async claimRun(instanceId: string, leaseSeconds: number) {
    const now = Date.now()
    const run = [...this.runs.values()].find(item => item.status === "queued" && (!item.leaseExpiresAt || item.leaseExpiresAt <= now))
    if (!run) return undefined
    run.leaseOwner = instanceId
    run.leaseExpiresAt = now + leaseSeconds * 1000
    return run
  }
  async countActiveUserRuns(userId: string) {
    let count = 0
    for (const run of this.runs.values()) {
      if (run.ownerUserId === userId && (run.status === "queued" || run.status === "running")) {
        count += 1
      }
    }
    return count
  }

  async reserveModelBudget(input: {
    id: string
    runId: string
    ownerUserId: string
    operation: ModelBudgetOperation
    estimatedInputTokens: number
    requestedOutputTokens: number
    leaseSeconds: number
  }) {
    const run = this.runs.get(input.runId)
    if (!run || run.ownerUserId !== input.ownerUserId) throw new Error("ai.run_not_found")
    const used = [...this.modelReservations.values()]
      .filter(item => item.runId === input.runId && item.state !== "released")
      .reduce((total, item) => total + item.tokens, 0)
    const contextCapacity = (run.model?.maxContextTokens ?? 524_288) - input.estimatedInputTokens
    if (contextCapacity < 1) throw new Error("ai.model_context_insufficient")
    const tokenCapacity = (run.budget?.totalTokens ?? 2_000_000) - used - input.estimatedInputTokens
    if (tokenCapacity < 1) throw new Error("ai.run_token_budget_exhausted")
    const maxOutputTokens = Math.min(input.requestedOutputTokens, run.model?.maxOutputTokens ?? 65_536, contextCapacity, tokenCapacity)
    this.modelReservations.set(input.id, {
      runId: input.runId,
      operation: input.operation,
      inputTokens: input.estimatedInputTokens,
      tokens: input.estimatedInputTokens + maxOutputTokens,
      state: "reserved",
      maxOutputTokens,
    })
    return { id: input.id, maxOutputTokens }
  }

  async confirmModelBudget(reservationId: string, usage?: ModelBudgetUsage): Promise<void> {
    const item = this.modelReservations.get(reservationId)
    if (!item || item.state !== "reserved") return
    if (usage?.reported) {
      item.inputTokens = usage.inputTokens
      item.tokens = usage.inputTokens + usage.outputTokens
    }
    item.state = "confirmed"
  }

  async releaseModelBudget(reservationId: string): Promise<void> {
    const item = this.modelReservations.get(reservationId)
    if (item?.state === "reserved") item.state = "released"
  }
  async getExecutionInput(runId: string) {
    const run = this.runs.get(runId)
    const turn = run ? this.turns.get(run.turnId) : undefined
    const conversation = run ? this.conversations.get(run.conversationId) : undefined
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
          ...(run.model ? { model: run.model } : {}),
          toolInteractions: this.items
            .filter(item => item.runId === runId && (item.type === "tool_call" || item.type === "tool_result"))
            .map(item => ({ itemId: item.id, type: item.type as "tool_call" | "tool_result", status: item.status, content: item.content })),
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
        const assistant = this.items
          .filter(candidate => candidate.runId === item.selectedRunId && candidate.type === "assistant_message")
          .sort((a, b) => a.timelineIndex - b.timelineIndex)
          .map(candidate => timelineText(candidate.content))
          .filter(Boolean)
          .join("\n")
        const toolInteractions = this.items
          .filter(candidate => candidate.runId === item.selectedRunId && ["tool_call", "tool_result"].includes(candidate.type))
          .sort((a, b) => a.timelineIndex - b.timelineIndex)
          .map(candidate => ({ type: candidate.type, status: candidate.status, content: structuredClone(candidate.content) }))
        return {
          turnIndex: item.turnIndex,
          user: item.input,
          assistant,
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
  async getRunActorGrantCiphertext(runId: string) { return this.runs.get(runId)?.runActorGrantCiphertext }
  async appendRunInput(runId: string, text: string) {
    const run = this.runs.get(runId)
    const turn = run ? this.turns.get(run.turnId) : undefined
    if (!turn) throw new Error("ai.run_not_found")
    turn.input = `${turn.input}\n${text}`
  }

  async renewLease(runId: string, instanceId: string, leaseSeconds: number) {
    const run = this.runs.get(runId)
    if (!run || run.leaseOwner !== instanceId || !["queued", "running"].includes(run.status)) return false
    run.leaseExpiresAt = Date.now() + leaseSeconds * 1000
    return true
  }

  async releaseLease(runId: string, instanceId: string) {
    const run = this.runs.get(runId)
    if (run?.leaseOwner === instanceId) {
      delete run.leaseOwner
      delete run.leaseExpiresAt
    }
  }

  async updateRun(runId: string, from: Run["status"], to: Run["status"], fields: Partial<Run> = {}) {
    const run = this.runs.get(runId)
    if (!run || run.status !== from) throw new RunStateConflictError(runId, from, to, run?.status)
    Object.assign(run, fields, { status: to, rowVersion: run.rowVersion + 1 })
    const turn = this.turns.get(run.turnId)
    if (turn) turn.status = to
    await this.appendEvent(runId, `run.${to}`, {
      state: to,
      rowVersion: run.rowVersion,
      ...(fields.errorCode ? { errorCode: fields.errorCode } : {}),
    })
    return run
  }

  async appendItem(value: Omit<TimelineItem, "id" | "timelineIndex" | "revision" | "createdAt"> & { id?: string }) {
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
    const item = await this.appendItem(value)
    const event = await this.appendEvent(item.runId, eventType, { ...eventData, item: structuredClone(item) })
    return { item, event }
  }

  async updateItemWithEvent(
    itemId: string,
    status: TimelineItem["status"],
    content: Record<string, unknown>,
    eventType: string,
    eventData: Record<string, unknown> = {},
  ) {
    const item = await this.updateItem(itemId, status, content)
    const event = await this.appendEvent(item.runId, eventType, { ...eventData, item: structuredClone(item) })
    return { item, event }
  }

  async finalizeStreamingItems(runId: string, status: Exclude<TimelineItem["status"], "streaming">) {
    const items = this.items.filter(item => item.runId === runId && item.status === "streaming")
    for (const item of items) {
      item.status = status
      item.revision += 1
      await this.appendEvent(runId, "item.finalized", { item: structuredClone(item) })
    }
  }

  async appendEvent(runId: string, type: string, data: Record<string, unknown>) {
    const sequence = this.events.filter(event => event.runId === runId).length + 1
    const event: RunEvent = { id: createId("aievt"), runId, sequence, type, data, createdAt: new Date().toISOString() }
    this.events.push(event)
    return event
  }

  async getEvents(ownerUserId: string, runId: string, after: number) {
    if (!await this.getRun(ownerUserId, runId)) return []
    return this.events.filter(event => event.runId === runId && event.sequence > after)
  }

  async createUIAction(runId: string, toolCallId: string, action: Record<string, unknown>, expiresAt: string) {
    const existing = [...this.uiActions.values()].find(item => item.toolCallId === toolCallId)
    if (existing) return existing
    const run = this.runs.get(runId)
    if (!run?.clientInstanceId) throw new Error("ai.client_instance_unavailable")
    const now = new Date().toISOString()
    const value: UIActionDelivery = {
      id: createId("aiuia"), runId, toolCallId, clientInstanceId: run.clientInstanceId,
      action, status: "pending", attempts: 1, expiresAt, createdAt: now, updatedAt: now,
    }
    this.uiActions.set(value.id, value)
    return value
  }

  async listPendingUIActions(ownerUserId: string, clientInstanceId: string) {
    const now = Date.now()
    for (const action of this.uiActions.values()) {
      if (action.status === "pending" && Date.parse(action.expiresAt) <= now) {
        action.status = "expired"
        action.updatedAt = new Date().toISOString()
      }
    }
    return [...this.uiActions.values()]
      .filter(action => action.status === "pending"
        && action.clientInstanceId === clientInstanceId
        && this.runs.get(action.runId)?.ownerUserId === ownerUserId)
      .sort((left, right) => left.createdAt.localeCompare(right.createdAt))
  }

  async acknowledgeUIAction(ownerUserId: string, clientInstanceId: string, actionId: string, acknowledgement: UIActionAcknowledgement) {
    const action = this.uiActions.get(actionId)
    if (!action
      || action.clientInstanceId !== clientInstanceId
      || this.runs.get(action.runId)?.ownerUserId !== ownerUserId)
      return undefined
    if (action.status !== "pending") return action
    action.status = acknowledgement.status
    action.acknowledgedAt = new Date().toISOString()
    action.updatedAt = action.acknowledgedAt
    if (acknowledgement.actualPath) action.actualPath = acknowledgement.actualPath
    if (acknowledgement.errorCode) action.errorCode = acknowledgement.errorCode
    return action
  }

  async getTimeline(ownerUserId: string, conversationId: string, options: TimelinePageOptions = {}) {
    const conversation = await this.getConversation(ownerUserId, conversationId)
    if (!conversation) return undefined
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
      const reservations = storedRun
        ? [...this.modelReservations.values()].filter(item => item.runId === storedRun.id && item.state !== "released")
        : []
      const latestAssistant = reservations.findLast(item => item.operation === "assistant" && item.state === "confirmed")
      const run = storedRun
        ? {
            ...storedRun,
            usedTokens: reservations.reduce((total, item) => total + item.tokens, 0),
            ...(latestAssistant ? { latestInputTokens: latestAssistant.inputTokens } : {}),
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
      turns: pageTurns,
      eventCursors,
      pageInfo: {
        hasOlder,
        ...(hasOlder && pageTurns[0] ? { oldestTurnIndex: pageTurns[0].turnIndex } : {}),
      },
    }
  }
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
