export type ModelMessage =
  | { role: "system" | "user", content: string }
  | { role: "assistant", content: string, toolCalls?: ModelToolCall[] }
  | { role: "tool", toolCallId: string, content: string }
export type ModelToolDefinition = {
  operationId: string
  description: string
  inputSchema: Record<string, unknown>
}
export type ModelToolSearchResult = {
  query: string
  items: Array<{
    operationId: string
    name: string
    summary: string
    category: string
    tags: string[]
    aliases: { zh: string[], en: string[] }
    purpose: { zh: string, en: string }
    avoidWhen: { zh: string, en: string }
    preconditions: { zh: string[], en: string[] }
    successEvidence: { zh: string, en: string }
    requiresApproval: boolean
  }>
  page: number
  pageSize: number
  total: number
  totalPages: number
  loadedOperationIds: string[]
  missingOperationIds: string[]
  catalogDigest: string
  duplicate: boolean
  cacheHit: boolean
}
export type ModelToolDetailsResult = {
  items: Array<object & { operationId: string }>
  loadedOperationIds: string[]
  alreadySelectedOperationIds: string[]
  missingOperationIds: string[]
  catalogDigest: string
  duplicate: boolean
  cacheHit: boolean
}
type Awaitable<T> = T | Promise<T>
export type ModelToolRegistry = {
  resolve: (
    pageContext: Record<string, unknown>,
    userInput: string,
    loadedOperationIds: string[],
    signal?: AbortSignal,
    toolCatalogDigest?: string,
  ) => Awaitable<ModelToolDefinition[]>
  search: (
    input: { query?: string, page?: number, pageSize?: number },
    pageContext: Record<string, unknown>,
    signal?: AbortSignal,
    toolCatalogDigest?: string,
  ) => Awaitable<ModelToolSearchResult>
  details: (operationIds: string[], toolCatalogDigest?: string) => Awaitable<ModelToolDetailsResult>
}
export type ModelToolResolver = ModelToolDefinition[]
  | ((
    pageContext: Record<string, unknown>,
    userInput: string,
    loadedOperationIds: string[],
    signal?: AbortSignal,
    toolCatalogDigest?: string,
  ) => Awaitable<ModelToolDefinition[]>)
  | ModelToolRegistry
export type ModelToolChoice = "auto" | "required" | { operationId: string }
export type ModelRequest = {
  messages: ModelMessage[]
  tools?: ModelToolDefinition[]
  toolChoice?: ModelToolChoice
  maxOutputTokens: number
  signal?: AbortSignal
  thinking?: { type: string }
  modelId?: string
  modelName?: string
  modelPricing?: AIModelSnapshot
  budget?: { runId: string, ownerUserId: string, operation: "assistant" | "summary" | "title" }
  conversationId?: string
  conversationCompacted?: boolean
}
export type ModelEvent =
  | { type: "reasoning_summary_delta", delta: string }
  | { type: "message_delta", delta: string }
  | { type: "tool_call_delta" }
  | {
      type: "completed"
      usage: ModelUsage
      creditHoldId?: string
      reconciliationRequired?: boolean
      toolCalls?: ModelToolCall[]
      finishReason?: string
      providerRequestId?: string
      responseId?: string
      responseModel?: string
    }
export type OfficialModelUsage = {
  inputTokens: number
  outputTokens: number
  totalTokens: number
  cacheReadInputTokens?: number
  cacheWriteInputTokens?: number
  reasoningOutputTokens?: number
}
export type ModelUsage =
  | { status: "reported", value: OfficialModelUsage }
  | { status: "unavailable", reason: UsageUnavailableReason }
export type UsageUnavailableReason =
  | "missing_usage"
  | "invalid_usage"
  | "stream_ended_without_usage"
export type ModelToolArgumentError = {
  code: "invalid_json"
  message: string
}
export type ModelToolCall = {
  id?: string
  operationId: string
  arguments: Record<string, unknown>
  argumentError?: ModelToolArgumentError
}
export type ModelResponse = {
  text: string
  reasoningSummary?: string
  toolCalls?: ModelToolCall[]
  finishReason?: string
  creditHoldId?: string
  reconciliationRequired?: boolean
  providerRequestId?: string
  responseId?: string
  responseModel?: string
  serviceTier?: string
  systemFingerprint?: string
  usage: ModelUsage
}
export type ModelCapabilities = { streaming: boolean, toolCalling: boolean, structuredOutput: boolean }

export interface ModelProvider {
  stream(request: ModelRequest): AsyncIterable<ModelEvent>
  complete(request: ModelRequest): Promise<ModelResponse>
  capabilities(): ModelCapabilities
  health(): Promise<{ ok: boolean, requestId?: string }>
}
import type { AIModelSnapshot } from "../domain.js"
