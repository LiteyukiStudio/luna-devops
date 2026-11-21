import type { ToolOperation } from "./catalog.js"
import { trace } from "@opentelemetry/api"
import { agentMetrics, clientSpanOptions, telemetryLog, withSpan } from "../telemetry.js"
import { isRetryableHTTPStatus, parseRetryAfter, waitForRetry } from "../retry.js"

export type ToolPreviewRequest = {
  runId: string
  toolCallId: string
  operation: ToolOperation
  arguments: Record<string, unknown>
  signal?: AbortSignal
}

export type ToolExecutionRequest = {
  runId: string
  toolCallId: string
  idempotencyKey: string
  operation: ToolOperation
  arguments: Record<string, unknown>
  previewLock?: string
  signal?: AbortSignal
}

export type HeadlessL0ToolExecutionRequest = {
  runId: string
  toolCallId: string
  actorUserId: string
  projectId?: string
  operation: ToolOperation
  arguments: Record<string, unknown>
  signal?: AbortSignal
}

export type ToolExecutionResult = { status: number, body: unknown, requestId?: string }

export interface LunaApiToolClient {
  preview?(request: ToolPreviewRequest): Promise<ToolExecutionResult>
  execute(request: ToolExecutionRequest): Promise<ToolExecutionResult>
  executeHeadlessL0?(request: HeadlessL0ToolExecutionRequest): Promise<ToolExecutionResult>
}

export class UnknownToolOutcomeError extends Error {
  override readonly name = "UnknownToolOutcomeError"

  constructor(readonly original: unknown) {
    super("ai.tool_outcome_unknown", { cause: original })
  }
}

export class HttpLunaApiToolClient implements LunaApiToolClient {
  constructor(
    private readonly baseUrl: string,
    private readonly serviceToken: string,
    private readonly retryCount: number | (() => number) = 5,
  ) {}

  async preview(request: ToolPreviewRequest): Promise<ToolExecutionResult> {
    const startedAt = performance.now()
    return withSpan("luna_api.tool.preview", clientSpanOptions({
      "server.address": new URL(this.baseUrl).hostname,
      "luna.run.id": request.runId,
      "luna.tool.name": request.operation.operationId,
      "luna.tool_call.id": request.toolCallId,
    }), async (span) => {
      const url = new URL("/internal/v1/ai/tools/preview", this.baseUrl)
      const init: RequestInit = {
        method: "POST",
        headers: {
          "authorization": `Bearer ${this.serviceToken}`,
          "x-luna-ai-run-id": request.runId,
          "x-luna-ai-tool-call-id": request.toolCallId,
          "content-type": "application/json",
        },
        ...(request.signal ? { signal: request.signal } : {}),
        body: JSON.stringify({
          toolName: request.operation.operationId,
          arguments: request.arguments,
        }),
      }
      let response: Response
      try {
        response = await this.fetchWithRetry(url, init, "tool_preview", request.signal, true)
      }
      catch (error) {
        if (request.signal?.aborted) throw error
        throw new UnknownToolOutcomeError(error)
      }
      const requestId = response.headers.get("x-request-id")
      span.setAttribute("http.response.status_code", response.status)
      let body: unknown
      try {
        body = await responseJSON(response)
      }
      catch (error) {
        agentMetrics.externalRequests.add(1, { target: "luna_api", operation: "tool_preview", outcome: "invalid_response" })
        throw new UnknownToolOutcomeError(error)
      }
      agentMetrics.externalRequests.add(1, { target: "luna_api", operation: "tool_preview", outcome: response.ok ? "success" : String(response.status) })
      span.setAttribute("luna.external.duration_ms", performance.now() - startedAt)
      return { status: response.status, body, ...(requestId ? { requestId } : {}) }
    })
  }

  async execute(request: ToolExecutionRequest): Promise<ToolExecutionResult> {
    const startedAt = performance.now()
    return withSpan("luna_api.tool.execute", clientSpanOptions({
      "server.address": new URL(this.baseUrl).hostname,
      "luna.run.id": request.runId,
      "luna.tool.name": request.operation.operationId,
      "luna.tool_call.id": request.toolCallId,
    }), async (span) => {
      const url = new URL("/internal/v1/ai/tools/execute", this.baseUrl)
      const init: RequestInit = {
        method: "POST",
        headers: {
          "authorization": `Bearer ${this.serviceToken}`,
          "x-luna-ai-run-id": request.runId,
          "x-luna-ai-tool-call-id": request.toolCallId,
          "idempotency-key": request.idempotencyKey,
          ...(request.previewLock ? { "x-luna-ai-preview-lock": request.previewLock } : {}),
          "content-type": "application/json",
        },
        ...(request.signal ? { signal: request.signal } : {}),
        body: JSON.stringify({
          toolName: request.operation.operationId,
          arguments: request.arguments,
        }),
      }
      let response: Response
      try {
        response = await this.fetchWithRetry(url, init, "tool_execute", request.signal, request.operation.idempotent)
      }
      catch (error) {
        if (request.signal?.aborted) throw error
        throw new UnknownToolOutcomeError(error)
      }
      const requestId = response.headers.get("x-request-id")
      span.setAttribute("http.response.status_code", response.status)
      let body: unknown
      try {
        body = await responseJSON(response)
      }
      catch (error) {
        agentMetrics.externalRequests.add(1, { target: "luna_api", operation: "tool_execute", outcome: "invalid_response" })
        throw new UnknownToolOutcomeError(error)
      }
      agentMetrics.externalRequests.add(1, { target: "luna_api", operation: "tool_execute", outcome: response.ok ? "success" : String(response.status) })
      span.setAttribute("luna.external.duration_ms", performance.now() - startedAt)
      return { status: response.status, body, ...(requestId ? { requestId } : {}) }
    })
  }

  async executeHeadlessL0(request: HeadlessL0ToolExecutionRequest): Promise<ToolExecutionResult> {
    const startedAt = performance.now()
    return withSpan("luna_api.tool.execute_headless_l0", clientSpanOptions({
      "server.address": new URL(this.baseUrl).hostname,
      "luna.run.id": request.runId,
      "luna.tool.name": request.operation.operationId,
      "luna.tool_call.id": request.toolCallId,
    }), async (span) => {
      const url = new URL("/internal/v1/ai/tools/execute-headless", this.baseUrl)
      const init: RequestInit = {
        method: "POST",
        headers: {
          "authorization": `Bearer ${this.serviceToken}`,
          "x-luna-ai-run-id": request.runId,
          "x-luna-ai-tool-call-id": request.toolCallId,
          "content-type": "application/json",
        },
        ...(request.signal ? { signal: request.signal } : {}),
        body: JSON.stringify({
          actorUserId: request.actorUserId,
          ...(request.projectId ? { projectId: request.projectId } : {}),
          toolName: request.operation.operationId,
          arguments: request.arguments,
        }),
      }
      let response: Response
      try {
        response = await this.fetchWithRetry(url, init, "tool_execute_headless_l0", request.signal, true)
      }
      catch (error) {
        if (request.signal?.aborted) throw error
        throw new UnknownToolOutcomeError(error)
      }
      const requestId = response.headers.get("x-request-id")
      span.setAttribute("http.response.status_code", response.status)
      let body: unknown
      try {
        body = await responseJSON(response)
      }
      catch (error) {
        agentMetrics.externalRequests.add(1, { target: "luna_api", operation: "tool_execute_headless_l0", outcome: "invalid_response" })
        throw new UnknownToolOutcomeError(error)
      }
      agentMetrics.externalRequests.add(1, { target: "luna_api", operation: "tool_execute_headless_l0", outcome: response.ok ? "success" : String(response.status) })
      span.setAttribute("luna.external.duration_ms", performance.now() - startedAt)
      return { status: response.status, body, ...(requestId ? { requestId } : {}) }
    })
  }

  private async fetchWithRetry(url: URL, init: RequestInit, operation: string, signal: AbortSignal | undefined, retryableOperation: boolean): Promise<Response> {
    const maxRetries = Math.max(0, Math.min(10, typeof this.retryCount === "function" ? this.retryCount() : this.retryCount))
    for (let retry = 0; ; retry += 1) {
      try {
        const response = await fetch(url, init)
        if (!retryableOperation || !isRetryableHTTPStatus(response.status) || retry >= maxRetries)
          return response
        agentMetrics.externalRequests.add(1, { target: "luna_api", operation, outcome: String(response.status) })
        const retryAfterMs = parseRetryAfter(response.headers)
        await response.body?.cancel().catch(() => undefined)
        await this.scheduleRetry(operation, retry + 1, maxRetries, signal, retryAfterMs, String(response.status))
      }
      catch (error) {
        if (signal?.aborted) throw error
        const outcome = isFetchTimeoutError(error) ? "timeout" : "failed"
        agentMetrics.externalRequests.add(1, { target: "luna_api", operation, outcome })
        if (!retryableOperation || retry >= maxRetries) throw error
        await this.scheduleRetry(operation, retry + 1, maxRetries, signal, undefined, outcome)
      }
    }
  }

  private async scheduleRetry(operation: string, attempt: number, maxRetries: number, signal: AbortSignal | undefined, retryAfterMs: number | undefined, reason: string): Promise<void> {
    trace.getActiveSpan()?.addEvent("luna_api.request.retry_scheduled", {
      operation,
      "retry.attempt": attempt,
      "retry.max_retries": maxRetries,
      "retry.reason": reason,
    })
    telemetryLog("agent.luna_api.retry_scheduled", "warn", {
      operation,
      "retry.attempt": attempt,
      "retry.max_retries": maxRetries,
      "retry.reason": reason,
    })
    await waitForRetry(attempt, { maxRetries, ...(signal ? { signal } : {}), ...(retryAfterMs !== undefined ? { retryAfterMs } : {}) })
  }
}

function isFetchTimeoutError(error: unknown): boolean {
  return error instanceof Error && (error.name === "TimeoutError" || error.name === "AbortError")
}

async function responseJSON(response: Response): Promise<unknown> {
  const text = await response.text()
  if (response.status === 204) return undefined
  if (text.trim() === "") {
    if (response.ok) throw new Error("ai.tool_response_invalid_json")
    return undefined
  }
  try { return JSON.parse(text) }
  catch {
    if (response.ok) throw new Error("ai.tool_response_invalid_json")
    return undefined
  }
}

export class DeterministicLunaApiClient implements LunaApiToolClient {
  readonly calls: ToolExecutionRequest[] = []
  readonly previews: ToolPreviewRequest[] = []
  readonly headlessCalls: HeadlessL0ToolExecutionRequest[] = []
  constructor(
    private readonly handler: (request: ToolExecutionRequest) => ToolExecutionResult | Promise<ToolExecutionResult>,
    private readonly previewHandler?: (request: ToolPreviewRequest) => ToolExecutionResult | Promise<ToolExecutionResult>,
  ) {}

  async preview(request: ToolPreviewRequest): Promise<ToolExecutionResult> {
    this.previews.push(request)
    if (this.previewHandler) return this.previewHandler(request)
    return {
      status: 200,
      body: {
        approvalPreview: { summary: `Preview ${request.operation.operationId}` },
        previewLock: `ailock_test_${request.toolCallId}`,
      },
    }
  }

  async execute(request: ToolExecutionRequest): Promise<ToolExecutionResult> {
    this.calls.push(request)
    return this.handler(request)
  }

  async executeHeadlessL0(request: HeadlessL0ToolExecutionRequest): Promise<ToolExecutionResult> {
    this.headlessCalls.push(request)
    return this.handler({
      runId: request.runId,
      toolCallId: request.toolCallId,
      idempotencyKey: `headless:${request.toolCallId}`,
      operation: request.operation,
      arguments: request.arguments,
      ...(request.signal ? { signal: request.signal } : {}),
    })
  }
}
