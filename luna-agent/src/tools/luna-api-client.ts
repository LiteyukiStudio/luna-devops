import type { ToolOperation } from "./catalog.js"
import { trace } from "@opentelemetry/api"
import { canonicalJSONStringify } from "../canonical-json.js"
import { agentMetrics, clientSpanOptions, telemetryLog, withSpan } from "../telemetry.js"
import { isRetryableHTTPStatus, parseRetryAfter, waitForRetry } from "../retry.js"

export type ToolExecutionRequest = {
  runId: string
  toolCallId: string
  operation: ToolOperation
  arguments: Record<string, unknown>
  argumentsHash: string
  inputMode?: "model" | "direct"
  runActorGrant: string
  approvalGranted: boolean
  mfaPurpose?: string
  stepUpAssertionId?: string
  signal?: AbortSignal
}
export type ToolExecutionResult = { status: number, body: unknown, requestId?: string }

export interface LunaApiToolClient {
  execute(request: ToolExecutionRequest): Promise<ToolExecutionResult>
}

export class HttpLunaApiToolClient implements LunaApiToolClient {
  constructor(
    private readonly baseUrl: string,
    private readonly serviceToken: string,
    private readonly retryCount: number | (() => number) = 5,
  ) {}
  async execute(request: ToolExecutionRequest): Promise<ToolExecutionResult> {
    const startedAt = performance.now()
    return withSpan("luna_api.tool.execute", clientSpanOptions({
      "server.address": new URL(this.baseUrl).hostname,
      "luna.run.id": request.runId,
      "luna.tool.name": request.operation.operationId,
      "luna.tool_call.id": request.toolCallId,
    }), async span => {
    const exchange = await this.fetchWithRetry(new URL("/internal/v1/ai/delegations/exchange", this.baseUrl), {
      method: "POST",
      headers: { authorization: `Bearer ${this.serviceToken}`, "content-type": "application/json" },
      body: JSON.stringify({
        runActorGrant: request.runActorGrant, runId: request.runId, toolCallId: request.toolCallId,
        operationId: request.operation.operationId, requestedScopes: request.operation.requiredScopes,
        argumentsHash: request.argumentsHash,
        inputMode: request.inputMode ?? "model",
        approvalGranted: request.approvalGranted,
        ...(request.mfaPurpose ? { mfaPurpose: request.mfaPurpose } : {}),
        ...(request.stepUpAssertionId ? { stepUpAssertionId: request.stepUpAssertionId } : {}),
      }),
      ...(request.signal ? { signal: request.signal } : {}),
    }, "delegation_exchange", request.signal, true)
    if (!exchange.ok) {
      const requestId = exchange.headers.get("x-request-id")
      span.setAttribute("http.response.status_code", exchange.status)
      agentMetrics.externalRequests.add(1, { target: "luna_api", operation: "delegation_exchange", outcome: String(exchange.status) })
      telemetryLog("agent.luna_api.delegation_failed", "warn", {
        "tool.name": request.operation.operationId,
        "http.response.status_code": exchange.status,
      })
      return { status: exchange.status, body: await safeJson(exchange), ...(requestId ? { requestId } : {}) }
    }
    const { accessToken } = await exchange.json() as { accessToken: string }
    const url = new URL(`/internal/v1/ai/tools/${encodeURIComponent(request.operation.operationId)}/execute`, this.baseUrl)
    const init: RequestInit = {
      method: "POST",
      headers: {
        authorization: `Bearer ${accessToken}`, "content-type": "application/json",
        "x-luna-ai-run-id": request.runId, "x-luna-ai-tool-call-id": request.toolCallId,
        "idempotency-key": `${request.toolCallId}:${request.argumentsHash}`,
      },
      ...(request.signal ? { signal: request.signal } : {}),
    }
    init.body = JSON.stringify({ argumentsCanonical: canonicalJSONStringify(request.arguments) })
    const response = await this.fetchWithRetry(url, init, "tool_execute", request.signal, request.operation.idempotent)
    const requestId = response.headers.get("x-request-id")
    span.setAttribute("http.response.status_code", response.status)
    agentMetrics.externalRequests.add(1, {
      target: "luna_api",
      operation: "tool_execute",
      outcome: response.ok ? "success" : String(response.status),
    })
    span.setAttribute("luna.external.duration_ms", performance.now() - startedAt)
    return { status: response.status, body: await safeJson(response), ...(requestId ? { requestId } : {}) }
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
        await this.scheduleRetry(operation, retry + 1, maxRetries, signal, parseRetryAfter(response.headers), String(response.status))
      }
      catch (error) {
        if (signal?.aborted || !retryableOperation || retry >= maxRetries) throw error
        agentMetrics.externalRequests.add(1, { target: "luna_api", operation, outcome: "network_error" })
        await this.scheduleRetry(operation, retry + 1, maxRetries, signal, undefined, "network_error")
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

async function safeJson(response: Response): Promise<unknown> {
  const text = await response.text()
  try { return JSON.parse(text) } catch { return { code: "invalid_json_response" } }
}

export class DeterministicLunaApiClient implements LunaApiToolClient {
  readonly calls: ToolExecutionRequest[] = []
  constructor(private readonly handler: (request: ToolExecutionRequest) => ToolExecutionResult | Promise<ToolExecutionResult>) {}
  async execute(request: ToolExecutionRequest): Promise<ToolExecutionResult> {
    this.calls.push(request)
    return this.handler(request)
  }
}
