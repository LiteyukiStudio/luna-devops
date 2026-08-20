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
  matches: Array<{ operationId: string, category: string, description: string }>
  loadedOperationIds: string[]
  totalMatches: number
}
export type ModelToolRetrievalState = {
  resourceContext: string[]
  completedOperations: string[]
  stableOutcomes: string[]
  pendingState?: "user_input" | "approval" | "mfa" | "async_terminal_check"
  stableErrorCodes: string[]
}
type Awaitable<T> = T | Promise<T>
export type ModelToolRegistry = {
  resolve: (
    pageContext: Record<string, unknown>,
    userInput: string,
    loadedOperationIds: string[],
    retrievalState?: ModelToolRetrievalState,
    signal?: AbortSignal,
  ) => Awaitable<ModelToolDefinition[]>
  search: (
    query: string,
    pageContext: Record<string, unknown>,
    limit: number,
    retrievalState?: ModelToolRetrievalState,
    signal?: AbortSignal,
  ) => Awaitable<ModelToolSearchResult>
}
export type ModelToolResolver = ModelToolDefinition[]
  | ((
    pageContext: Record<string, unknown>,
    userInput: string,
    loadedOperationIds: string[],
    retrievalState?: ModelToolRetrievalState,
    signal?: AbortSignal,
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
  budget?: { runId: string, ownerUserId: string, operation: "assistant" | "summary" | "title" | "next_steps" }
  conversationId?: string
  conversationCompacted?: boolean
}
export type ModelEvent =
  | { type: "reasoning_summary_delta", delta: string }
  | { type: "message_delta", delta: string }
  | { type: "tool_call_delta" }
  | { type: "completed", usage: ModelUsage, reservationId?: string, toolCalls?: ModelToolCall[], finishReason?: string }
export type ModelUsage = {
  inputTokens: number
  outputTokens: number
  cachedInputTokens?: number
  cachedOutputTokens?: number
  reasoningOutputTokens?: number
  reported?: boolean
}
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
export type ModelResponse = { text: string, reasoningSummary?: string, toolCalls?: ModelToolCall[], finishReason?: string, reservationId?: string, requestId?: string, serviceTier?: string, systemFingerprint?: string, usage: ModelUsage }
export type ModelCapabilities = { streaming: boolean, toolCalling: boolean, structuredOutput: boolean }

export interface ModelProvider {
  stream(request: ModelRequest): AsyncIterable<ModelEvent>
  complete(request: ModelRequest): Promise<ModelResponse>
  capabilities(): ModelCapabilities
  health(): Promise<{ ok: boolean, requestId?: string }>
}
import type { AIModelSnapshot } from "../domain.js"
