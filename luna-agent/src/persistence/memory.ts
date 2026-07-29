import { createHash } from "node:crypto"
import type { Conversation, CreatedTurn, CreateTurn, Run, RunEvent, TimelineItem, Turn } from "../domain.js"
import { createId } from "../id.js"
import type { Repository } from "./repository.js"

type StoredRun = Run & { ownerUserId: string, leaseOwner?: string, leaseExpiresAt?: number, runActorGrantCiphertext?: string }

export class MemoryRepository implements Repository {
  private readonly conversations = new Map<string, Conversation>()
  private readonly turns = new Map<string, Turn>()
  private readonly runs = new Map<string, StoredRun>()
  private readonly items: TimelineItem[] = []
  private readonly events: RunEvent[] = []
  private readonly idempotency = new Map<string, { hash: string, created: CreatedTurn }>()

  async health(): Promise<boolean> { return true }

  async createConversation(ownerUserId: string, title: string, projectId?: string): Promise<Conversation> {
    const now = new Date().toISOString()
    const value: Conversation = {
      id: createId("aicnv"), ownerUserId, title, status: "active", createdAt: now, updatedAt: now,
      ...(projectId ? { projectId } : {}),
    }
    this.conversations.set(value.id, value)
    return value
  }

  async listConversations(ownerUserId: string, page: number, pageSize: number) {
    const all = [...this.conversations.values()].filter(item => item.ownerUserId === ownerUserId)
      .sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
    return { items: all.slice((page - 1) * pageSize, page * pageSize), total: all.length }
  }

  async getConversation(ownerUserId: string, id: string) {
    const value = this.conversations.get(id)
    return value?.ownerUserId === ownerUserId ? value : undefined
  }

  async renameConversation(ownerUserId: string, id: string, title: string) {
    const value = await this.getConversation(ownerUserId, id)
    if (!value) return undefined
    const next = { ...value, title, updatedAt: new Date().toISOString() }
    this.conversations.set(id, next)
    return next
  }

  async deleteConversation(ownerUserId: string, id: string) {
    if (!await this.getConversation(ownerUserId, id)) return false
    this.conversations.delete(id)
    const turnIds = [...this.turns.values()].filter(t => t.conversationId === id).map(t => t.id)
    for (const turnId of turnIds) this.turns.delete(turnId)
    for (const [runId, run] of this.runs) if (run.conversationId === id) this.runs.delete(runId)
    return true
  }

  async createTurn(ownerUserId: string, input: CreateTurn): Promise<CreatedTurn> {
    if (!await this.getConversation(ownerUserId, input.conversationId)) throw new Error("ai.conversation_not_found")
    const hash = createHash("sha256").update(JSON.stringify(input)).digest("hex")
    const key = `${ownerUserId}:${input.idempotencyKey}`
    const previous = this.idempotency.get(key)
    if (previous) {
      if (previous.hash !== hash) throw new Error("idempotency_conflict")
      return previous.created
    }
    const now = new Date().toISOString()
    const turnIndex = [...this.turns.values()].filter(t => t.conversationId === input.conversationId).length
    const runId = input.preallocatedRunId ?? createId("airun")
    const turn: Turn = {
      id: createId("aitrn"), conversationId: input.conversationId, turnIndex, status: "queued",
      input: input.input, selectedRunId: runId, createdAt: now,
    }
    const run: StoredRun = {
      id: runId, conversationId: input.conversationId, turnId: turn.id, runIndex: 0,
      status: "queued", rowVersion: 1, graphVersion: "assistant-v1", promptVersion: "system-v1",
      toolCatalogDigest: "sha256:p0-readonly", pageContext: input.pageContext, createdAt: now, ownerUserId,
      ...(input.runActorGrantCiphertext ? { runActorGrantCiphertext: input.runActorGrantCiphertext } : {}),
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
    const run = await this.getRun(ownerUserId, id)
    if (!run || ["completed", "failed", "canceled", "expired"].includes(run.status)) return run
    return this.updateRun(id, run.status, "canceled", { completedAt: new Date().toISOString() })
  }

  async claimRun(instanceId: string, leaseSeconds: number) {
    const now = Date.now()
    const run = [...this.runs.values()].find(item => item.status === "queued" && (!item.leaseExpiresAt || item.leaseExpiresAt <= now))
    if (!run) return undefined
    run.leaseOwner = instanceId
    run.leaseExpiresAt = now + leaseSeconds * 1000
    return run
  }
  async getExecutionInput(runId: string) {
    const run = this.runs.get(runId)
    const turn = run ? this.turns.get(run.turnId) : undefined
    return run && turn ? { turnId: turn.id, input: turn.input, pageContext: run.pageContext, toolResults: this.items.filter(item => item.runId === runId && item.type === "tool_result").map(item => item.content) } : undefined
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
    if (!run || run.leaseOwner !== instanceId) return false
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
    if (!run || run.status !== from) throw new Error("ai.run_state_conflict")
    Object.assign(run, fields, { status: to, rowVersion: run.rowVersion + 1 })
    const turn = this.turns.get(run.turnId)
    if (turn) turn.status = to
    await this.appendEvent(runId, `run.${to}`, { state: to, rowVersion: run.rowVersion })
    return run
  }

  async appendItem(value: Omit<TimelineItem, "id" | "timelineIndex" | "createdAt">) {
    const timelineIndex = this.items.filter(item => item.runId === value.runId).length
    const item: TimelineItem = { ...value, id: createId("aiitm"), timelineIndex, createdAt: new Date().toISOString() }
    this.items.push(item)
    return item
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

  async getTimeline(ownerUserId: string, conversationId: string) {
    const conversation = await this.getConversation(ownerUserId, conversationId)
    if (!conversation) return undefined
    const turns = [...this.turns.values()].filter(turn => turn.conversationId === conversationId)
      .sort((a, b) => a.turnIndex - b.turnIndex)
      .map(turn => {
        const run = this.runs.get(turn.selectedRunId)
        return { id: turn.id, turnIndex: turn.turnIndex, status: turn.status, input: turn.input, ...(run ? { run } : {}), items: this.items.filter(i => i.runId === run?.id).sort((a, b) => a.timelineIndex - b.timelineIndex) }
      })
    return { conversation, turns }
  }
}
