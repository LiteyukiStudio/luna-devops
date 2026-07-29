import { createHash } from "node:crypto"
import { createId } from "../id.js"
import { redact } from "../redaction.js"
import type { Repository } from "../persistence/repository.js"
import { validateArguments, type ToolCatalog } from "./catalog.js"
import type { LunaApiToolClient, ToolExecutionResult } from "./luna-api-client.js"
import { ToolPolicy } from "./policy.js"

export type ToolCallStatus = "proposed" | "awaiting_approval" | "awaiting_mfa" | "running" | "succeeded" | "failed" | "canceled" | "skipped"
export type ToolCallRecord = {
  id: string; runId: string; operationId: string; status: ToolCallStatus; arguments: Record<string, unknown>
  argumentsHash: string; attempt: number; rowVersion: number; approvalExpiresAt?: number; mfaPurpose?: string
  result?: unknown; errorCode?: string
}
export type ToolEvent = { type: string, toolCallId: string, data: Record<string, unknown> }

export interface ToolCallStore {
  insert(value: ToolCallRecord): Promise<void>
  get(id: string): Promise<ToolCallRecord | undefined>
  update(id: string, expected: ToolCallStatus, patch: Partial<ToolCallRecord>): Promise<ToolCallRecord>
  emit(event: ToolEvent): Promise<void>
  countForRun(runId: string): Promise<number>
}
export interface ToolResultVerifier {
  verify(operationId: string, result: ToolExecutionResult): Promise<{ ok: boolean, code?: string }>
}
export class AcceptingResultVerifier implements ToolResultVerifier {
  async verify(_operationId: string, result: ToolExecutionResult) {
    return { ok: result.status >= 200 && result.status < 300 }
  }
}
export class ToolInterruption extends Error {
  constructor(readonly state: "waiting_input", readonly fields: string[]) {
    super("ai.input_required")
  }
}

export class ToolOrchestrator {
  constructor(
    private readonly catalog: ToolCatalog,
    private readonly client: LunaApiToolClient,
    private readonly store: ToolCallStore,
    private readonly policy = new ToolPolicy(),
    private readonly maxToolCalls = 12,
    private readonly verifier: ToolResultVerifier = new AcceptingResultVerifier(),
    private readonly grantResolver: (runId: string) => Promise<string> = async () => { throw new Error("ai.run_grant_unavailable") },
  ) {}

  async propose(input: { runId: string, operationId: string, arguments: unknown }): Promise<ToolCallRecord> {
    if (await this.store.countForRun(input.runId) >= this.maxToolCalls) throw new Error("ai.limit_exceeded")
    const operation = this.catalog.get(input.operationId)
    let args: Record<string, unknown>
    try {
      args = validateArguments(operation.inputSchema, input.arguments)
    } catch {
      const value = input.arguments && typeof input.arguments === "object" ? input.arguments as Record<string, unknown> : {}
      throw new ToolInterruption("waiting_input", operation.inputSchema.required.filter(field => value[field] === undefined))
    }
    const argumentsHash = `sha256:${createHash("sha256").update(JSON.stringify(args)).digest("hex")}`
    const record: ToolCallRecord = {
      id: createId("aitool"), runId: input.runId, operationId: input.operationId,
      status: "proposed", arguments: redact(args), argumentsHash, attempt: 1, rowVersion: 1,
    }
    await this.store.insert(record)
    await this.store.emit({ type: "tool_call.proposed", toolCallId: record.id, data: { operationId: record.operationId, arguments: record.arguments, argumentsHash } })
    return this.advance(record, { approved: false })
  }

  async approve(id: string, argumentsHash: string, expectedVersion: number): Promise<ToolCallRecord> {
    const call = await this.require(id)
    if (call.status !== "awaiting_approval" || call.argumentsHash !== argumentsHash || call.rowVersion !== expectedVersion || (call.approvalExpiresAt ?? 0) < Date.now()) {
      throw new Error("ai.approval_expired")
    }
    return this.advance(call, { approved: true })
  }

  async resumeMfa(id: string, purpose: string, expectedVersion: number): Promise<ToolCallRecord> {
    const call = await this.require(id)
    if (call.status !== "awaiting_mfa" || call.mfaPurpose !== purpose || call.rowVersion !== expectedVersion) throw new Error("ai.run_not_resumable")
    return this.execute(call)
  }

  async retryFailed(id: string): Promise<ToolCallRecord> {
    const previous = await this.require(id)
    if (previous.status !== "failed") throw new Error("ai.tool_call_not_retryable")
    if (await this.store.countForRun(previous.runId) >= this.maxToolCalls) throw new Error("ai.limit_exceeded")
    const retry: ToolCallRecord = {
      id: createId("aitool"), runId: previous.runId, operationId: previous.operationId,
      status: "proposed", arguments: previous.arguments, argumentsHash: previous.argumentsHash,
      attempt: previous.attempt + 1, rowVersion: 1,
    }
    await this.store.insert(retry)
    await this.store.emit({ type: "tool_call.retry_proposed", toolCallId: retry.id, data: { previousToolCallId: previous.id, attempt: retry.attempt, argumentsHash: retry.argumentsHash } })
    return this.advance(retry, { approved: false })
  }

  private async advance(call: ToolCallRecord, state: { approved: boolean }): Promise<ToolCallRecord> {
    const operation = this.catalog.get(call.operationId)
    const decision = this.policy.evaluate(operation, state)
    if (decision.action === "wait_approval") {
      return this.transition(call, "awaiting_approval", { approvalExpiresAt: Date.now() + 30 * 60_000 }, "tool_call.awaiting_approval")
    }
    if (decision.action === "wait_mfa") {
      return this.transition(call, "awaiting_mfa", { mfaPurpose: decision.purpose }, "tool_call.awaiting_mfa")
    }
    return this.execute(call)
  }

  private async execute(call: ToolCallRecord): Promise<ToolCallRecord> {
    const operation = this.catalog.get(call.operationId)
    const running = await this.transition(call, "running", {}, "tool_call.running")
    const result = await this.client.execute({
      runId: call.runId, toolCallId: call.id, operation, arguments: call.arguments,
      argumentsHash: call.argumentsHash, runActorGrant: await this.grantResolver(call.runId),
    })
    return this.finish(running, result)
  }

  private async finish(call: ToolCallRecord, result: ToolExecutionResult): Promise<ToolCallRecord> {
    const code = extractCode(result.body)
    if (result.status === 401 || result.status === 403) return this.transition(call, "failed", { errorCode: code ?? "ai.tool_forbidden", result: redact(result.body) }, "tool_call.failed")
    if (result.status === 428 && code === "mfa_required") {
      const purpose = (result.body as Record<string, unknown>).purpose
      return this.transition(call, "awaiting_mfa", { mfaPurpose: typeof purpose === "string" ? purpose : "" }, "tool_call.awaiting_mfa")
    }
    if (result.status < 200 || result.status >= 300) return this.transition(call, "failed", { errorCode: code ?? "ai.tool_failed", result: redact(result.body) }, "tool_call.failed")
    const verification = await this.verifier.verify(call.operationId, result)
    if (!verification.ok) return this.transition(call, "failed", { errorCode: verification.code ?? "verification_inconclusive", result: redact(result.body) }, "tool_call.failed")
    return this.transition(call, "succeeded", { result: redact(result.body) }, "tool_call.succeeded")
  }

  private async transition(call: ToolCallRecord, status: ToolCallStatus, patch: Partial<ToolCallRecord>, event: string) {
    const next = await this.store.update(call.id, call.status, { ...patch, status, rowVersion: call.rowVersion + 1 })
    await this.store.emit({ type: event, toolCallId: call.id, data: redact({ status, result: next.result, errorCode: next.errorCode, mfaPurpose: next.mfaPurpose }) })
    return next
  }
  private async require(id: string) {
    const call = await this.store.get(id)
    if (!call) throw new Error("ai.tool_call_not_found")
    return call
  }
}

function extractCode(body: unknown): string | undefined {
  if (!body || typeof body !== "object") return undefined
  const object = body as Record<string, unknown>
  return typeof object.code === "string" ? object.code : typeof (object.error as Record<string, unknown> | undefined)?.code === "string" ? (object.error as { code: string }).code : undefined
}

export class MemoryToolCallStore implements ToolCallStore {
  readonly records = new Map<string, ToolCallRecord>()
  readonly events: ToolEvent[] = []
  async insert(value: ToolCallRecord) { this.records.set(value.id, value) }
  async get(id: string) { return this.records.get(id) }
  async update(id: string, expected: ToolCallStatus, patch: Partial<ToolCallRecord>) {
    const value = this.records.get(id)
    if (!value || value.status !== expected) throw new Error("ai.tool_call_state_conflict")
    const next = { ...value, ...patch }
    this.records.set(id, next)
    return next
  }
  async emit(event: ToolEvent) { this.events.push(event) }
  async countForRun(runId: string) { return [...this.records.values()].filter(item => item.runId === runId).length }
}

export class ProjectingToolCallStore implements ToolCallStore {
  constructor(private readonly inner: ToolCallStore, private readonly repository: Repository) {}
  insert(value: ToolCallRecord) { return this.inner.insert(value) }
  get(id: string) { return this.inner.get(id) }
  update(id: string, expected: ToolCallStatus, patch: Partial<ToolCallRecord>) { return this.inner.update(id, expected, patch) }
  countForRun(runId: string) { return this.inner.countForRun(runId) }
  async emit(event: ToolEvent) {
    await this.inner.emit(event)
    const call = await this.inner.get(event.toolCallId)
    if (!call) return
    await this.repository.appendEvent(call.runId, event.type, { toolCallId: call.id, ...event.data })
    const execution = await this.repository.getExecutionInput(call.runId)
    if (!execution) return
    await this.repository.appendItem({
      runId: call.runId, turnId: execution.turnId,
      type: event.type === "tool_call.succeeded" || event.type === "tool_call.failed" ? "tool_result" : "tool_call",
      status: event.type === "tool_call.failed" ? "failed" : "completed",
      content: redact({ toolCallId: call.id, operationId: call.operationId, status: call.status, arguments: call.arguments, result: call.result, errorCode: call.errorCode }),
    })
  }
}
