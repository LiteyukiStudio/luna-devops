export type ModelMessage = { role: "system" | "user" | "assistant", content: string }
export type ModelRequest = { messages: ModelMessage[], maxOutputTokens: number, signal?: AbortSignal }
export type ModelEvent =
  | { type: "reasoning_summary_delta", delta: string }
  | { type: "message_delta", delta: string }
  | { type: "completed", usage: { inputTokens: number, outputTokens: number } }
export type ModelToolCall = { operationId: string, arguments: Record<string, unknown> }
export type ModelResponse = { text: string, reasoningSummary?: string, toolCalls?: ModelToolCall[], usage: { inputTokens: number, outputTokens: number } }
export type ModelCapabilities = { streaming: boolean, toolCalling: boolean, structuredOutput: boolean }

export interface ModelProvider {
  stream(request: ModelRequest): AsyncIterable<ModelEvent>
  complete(request: ModelRequest): Promise<ModelResponse>
  capabilities(): ModelCapabilities
  health(): Promise<{ ok: boolean, requestId?: string }>
}
