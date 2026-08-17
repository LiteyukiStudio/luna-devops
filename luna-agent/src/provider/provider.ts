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
export type ModelToolRegistry = {
  resolve: (pageContext: Record<string, unknown>, userInput: string, loadedOperationIds: string[]) => ModelToolDefinition[]
  search: (query: string, pageContext: Record<string, unknown>, limit: number) => ModelToolSearchResult
}
export type ModelToolResolver = ModelToolDefinition[]
  | ((pageContext: Record<string, unknown>, userInput: string, loadedOperationIds: string[]) => ModelToolDefinition[])
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
}
export type ModelEvent =
  | { type: "reasoning_summary_delta", delta: string }
  | { type: "message_delta", delta: string }
  | { type: "tool_call_delta" }
  | { type: "completed", usage: { inputTokens: number, outputTokens: number, cachedInputTokens?: number, cachedOutputTokens?: number }, toolCalls?: ModelToolCall[] }
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
export type ModelResponse = { text: string, reasoningSummary?: string, toolCalls?: ModelToolCall[], usage: { inputTokens: number, outputTokens: number, cachedInputTokens?: number, cachedOutputTokens?: number } }
export type ModelCapabilities = { streaming: boolean, toolCalling: boolean, structuredOutput: boolean }

export interface ModelProvider {
  stream(request: ModelRequest): AsyncIterable<ModelEvent>
  complete(request: ModelRequest): Promise<ModelResponse>
  capabilities(): ModelCapabilities
  health(): Promise<{ ok: boolean, requestId?: string }>
}
import type { AIModelSnapshot } from "../domain.js"
