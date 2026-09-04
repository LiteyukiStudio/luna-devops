import type { ToolOperation } from "./catalog.js"
import { trace } from "@opentelemetry/api"
import { agentMetrics, clientSpanOptions, telemetryLog, withSpan } from "../telemetry.js"
import { isRetryableHTTPStatus, parseRetryAfter, waitForRetry } from "../retry.js"

export type ToolExecutionRequest = {
  runId: string
  toolCallId: string
  operation: ToolOperation
  arguments: Record<string, unknown>
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
    const transport = buildToolRequest(request.operation, request.arguments, this.baseUrl)
    const init: RequestInit = {
      method: request.operation.method,
      headers: {
        ...transport.headers,
        authorization: `Bearer ${this.serviceToken}`,
        "x-luna-ai-run-id": request.runId, "x-luna-ai-tool-call-id": request.toolCallId,
        "idempotency-key": request.toolCallId,
        ...(transport.body === undefined ? {} : { "content-type": "application/json" }),
      },
      ...(request.signal ? { signal: request.signal } : {}),
      ...(transport.body === undefined ? {} : { body: JSON.stringify(transport.body) }),
    }
    const response = await this.fetchWithRetry(transport.url, init, "tool_execute", request.signal, request.operation.idempotent)
    const requestId = response.headers.get("x-request-id")
    span.setAttribute("http.response.status_code", response.status)
    let body: unknown
    try {
      body = await responseJSON(response)
    }
    catch (error) {
      agentMetrics.externalRequests.add(1, { target: "luna_api", operation: "tool_execute", outcome: "invalid_response" })
      throw error
    }
    agentMetrics.externalRequests.add(1, { target: "luna_api", operation: "tool_execute", outcome: response.ok ? "success" : String(response.status) })
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

type TransportParameter = {
  inputName: string
  wireName: string
  in: "path" | "query" | "header"
  required: boolean
}

type TransportOperation = ToolOperation & {
  parameters?: TransportParameter[]
  requestBody?: boolean
  requestRequired?: boolean
  requestType?: string
}

export function buildToolRequest(operation: ToolOperation, input: Record<string, unknown>, baseUrl: string): { url: URL, body?: Record<string, unknown>, headers?: Record<string, string> } {
  const transport = operation as TransportOperation
  const consumed = new Set<string>()
  const headers: Record<string, string> = {}
  let pathname = transport.path
  const url = new URL(pathname, baseUrl)
  for (const parameter of transport.parameters ?? []) {
    const value = input[parameter.inputName]
    if (value === undefined || value === null) {
      if (parameter.required) throw new Error("ai.tool_arguments_invalid")
      continue
    }
    consumed.add(parameter.inputName)
    if (parameter.in === "path") {
      const marker = `{${parameter.wireName}}`
      if (!pathname.includes(marker)) throw new Error("ai.tool_catalog_invalid")
      pathname = pathname.replaceAll(marker, encodeURIComponent(pathParameterValue(value)))
      continue
    }
    if (parameter.in === "header") {
      const headerName = allowedToolHeader(parameter.wireName)
      headers[headerName] = pathParameterValue(value)
      continue
    }
    appendQueryValue(url.searchParams, parameter.wireName, value)
  }
  if (/\{[^}]+\}/.test(pathname)) throw new Error("ai.tool_arguments_invalid")
  url.pathname = pathname

  const remaining = Object.fromEntries(Object.entries(input).filter(([key]) => !consumed.has(key)))
  if (transport.requestBody === true) {
    const explicitBody = remaining.body
    if (isRecord(explicitBody) && Object.keys(remaining).length === 1)
      return { url, body: explicitBody, ...(Object.keys(headers).length ? { headers } : {}) }
    return { url, body: remaining, ...(Object.keys(headers).length ? { headers } : {}) }
  }
  if ((transport.parameters?.length ?? 0) === 0) {
    for (const [key, value] of Object.entries(input)) appendQueryValue(url.searchParams, key, value)
  }
  return { url, ...(Object.keys(headers).length ? { headers } : {}) }
}

function allowedToolHeader(name: string): string {
  if (name.toLowerCase() === "if-match") return "if-match"
  throw new Error("ai.tool_catalog_invalid")
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function pathParameterValue(value: unknown): string {
  if (typeof value === "string") return value
  if (typeof value === "number" || typeof value === "bigint") return value.toString()
  if (typeof value === "boolean") return value ? "true" : "false"
  throw new Error("ai.tool_arguments_invalid")
}

function appendQueryValue(query: URLSearchParams, name: string, value: unknown): void {
  if (Array.isArray(value)) {
    for (const item of value) appendQueryValue(query, name, item)
    return
  }
  if (typeof value === "object" && value !== null) {
    query.append(name, JSON.stringify(value))
    return
  }
  query.append(name, String(value))
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
  constructor(private readonly handler: (request: ToolExecutionRequest) => ToolExecutionResult | Promise<ToolExecutionResult>) {}
  async execute(request: ToolExecutionRequest): Promise<ToolExecutionResult> {
    this.calls.push(request)
    return this.handler(request)
  }
}
