import type { ToolOperation } from "./catalog.js"

export type ToolExecutionRequest = {
  runId: string
  toolCallId: string
  operation: ToolOperation
  arguments: Record<string, unknown>
  argumentsHash: string
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
  constructor(private readonly baseUrl: string, private readonly serviceToken: string) {}
  async execute(request: ToolExecutionRequest): Promise<ToolExecutionResult> {
    const exchange = await fetch(new URL("/internal/v1/ai/delegations/exchange", this.baseUrl), {
      method: "POST",
      headers: { authorization: `Bearer ${this.serviceToken}`, "content-type": "application/json" },
      body: JSON.stringify({
        runActorGrant: request.runActorGrant, runId: request.runId, toolCallId: request.toolCallId,
        operationId: request.operation.operationId, requestedScopes: request.operation.requiredScopes,
        argumentsHash: request.argumentsHash,
        approvalGranted: request.approvalGranted,
        ...(request.mfaPurpose ? { mfaPurpose: request.mfaPurpose } : {}),
        ...(request.stepUpAssertionId ? { stepUpAssertionId: request.stepUpAssertionId } : {}),
      }),
      ...(request.signal ? { signal: request.signal } : {}),
    })
    if (!exchange.ok) {
      const requestId = exchange.headers.get("x-request-id")
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
    init.body = JSON.stringify({ arguments: request.arguments })
    const response = await fetch(url, init)
    const requestId = response.headers.get("x-request-id")
    return { status: response.status, body: await safeJson(response), ...(requestId ? { requestId } : {}) }
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
