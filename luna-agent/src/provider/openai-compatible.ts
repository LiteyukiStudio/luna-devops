import type { ModelCapabilities, ModelEvent, ModelProvider, ModelRequest, ModelResponse, ModelToolCall } from "./provider.js"

type Options = { baseUrl: string, apiKey: string, model: string, timeoutMs: number }

export class OpenAICompatibleProvider implements ModelProvider {
  constructor(private readonly options: Options) {}

  capabilities(): ModelCapabilities {
    return { streaming: true, toolCalling: true, structuredOutput: true }
  }

  async health(): Promise<{ ok: boolean, requestId?: string }> {
    const response = await this.request([{ role: "user", content: "只回复 OK。" }], 4)
    return { ok: response.text.length > 0, ...(response.requestId ? { requestId: response.requestId } : {}) }
  }

  async complete(request: ModelRequest): Promise<ModelResponse> {
    const response = await this.request(request.messages, request.maxOutputTokens, request.signal, request.tools, request.toolChoice)
    return {
      text: response.text,
      usage: response.usage,
      ...(response.reasoningSummary ? { reasoningSummary: response.reasoningSummary } : {}),
      ...(response.toolCalls.length ? { toolCalls: response.toolCalls } : {}),
    }
  }

  async *stream(request: ModelRequest): AsyncIterable<ModelEvent> {
    const response = await this.fetchCompletion(request.messages, request.maxOutputTokens, true, request.signal, request.tools, request.toolChoice)
    if (!response.body) throw new Error("ai.provider_empty_stream")
    const decoder = new TextDecoder()
    let buffer = ""
    let usage = { inputTokens: 0, outputTokens: 0 }
    const toolFragments = new Map<number, { operationId: string, arguments: string }>()
    const reader = (response.body as ReadableStream<Uint8Array>).getReader()
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const frames = buffer.split(/\r?\n\r?\n/)
      buffer = frames.pop() ?? ""
      for (const frame of frames) {
        const data = frame.split(/\r?\n/).filter(line => line.startsWith("data:")).map(line => line.slice(5).trimStart()).join("\n")
        if (!data || data === "[DONE]") continue
        const payload = JSON.parse(data) as StreamChunk
        if (payload.error) throw new Error(providerPayloadError(payload.error))
        const delta = payload.choices?.[0]?.delta
        const reasoning = textValue(delta?.reasoning_summary)
        const content = contentText(delta?.content)
        if (reasoning) yield { type: "reasoning_summary_delta", delta: reasoning }
        if (content) yield { type: "message_delta", delta: content }
        for (const fragment of delta?.tool_calls ?? []) {
          const current = toolFragments.get(fragment.index) ?? { operationId: "", arguments: "" }
          current.operationId += fragment.function?.name ?? ""
          current.arguments += fragment.function?.arguments ?? ""
          toolFragments.set(fragment.index, current)
        }
        if (payload.usage) {
          usage = {
            inputTokens: payload.usage.prompt_tokens ?? usage.inputTokens,
            outputTokens: payload.usage.completion_tokens ?? usage.outputTokens,
          }
        }
      }
    }
    const toolCalls = parseToolCalls(toolFragments)
    yield { type: "completed", usage, ...(toolCalls.length ? { toolCalls } : {}) }
  }

  private async request(messages: ModelRequest["messages"], maxTokens: number, signal?: AbortSignal, tools?: ModelRequest["tools"], toolChoice?: ModelRequest["toolChoice"]) {
    const response = await this.fetchCompletion(messages, maxTokens, false, signal, tools, toolChoice)
    const body = await response.json() as CompletionBody
    const message = body.choices?.[0]?.message
    return {
      text: contentText(message?.content),
      reasoningSummary: textValue(message?.reasoning_summary),
      toolCalls: (message?.tool_calls ?? []).map(call => ({
        operationId: call.function?.name ?? "",
        arguments: parseArguments(call.function?.arguments ?? ""),
      })).filter(call => call.operationId),
      usage: { inputTokens: body.usage?.prompt_tokens ?? 0, outputTokens: body.usage?.completion_tokens ?? 0 },
      requestId: response.headers.get("x-request-id") ?? undefined,
    }
  }

  private async fetchCompletion(messages: ModelRequest["messages"], maxTokens: number, stream: boolean, signal?: AbortSignal, tools?: ModelRequest["tools"], toolChoice?: ModelRequest["toolChoice"]) {
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), this.options.timeoutMs)
    const abort = () => controller.abort(signal?.reason)
    signal?.addEventListener("abort", abort, { once: true })
    try {
      const response = await fetch(new URL("chat/completions", ensureTrailingSlash(this.options.baseUrl)), {
        method: "POST",
        headers: { authorization: `Bearer ${this.options.apiKey}`, "content-type": "application/json" },
        body: JSON.stringify({
          model: this.options.model,
          messages,
          max_tokens: maxTokens,
          stream,
          ...(tools?.length
            ? {
                tools: tools.map(tool => ({
                  type: "function",
                  function: {
                    name: tool.operationId,
                    description: tool.description,
                    parameters: tool.inputSchema,
                  },
                })),
                tool_choice: providerToolChoice(toolChoice),
              }
            : {}),
        }),
        signal: controller.signal,
      })
      if (!response.ok) throw new Error(providerHTTPError(response.status))
      return response
    } finally {
      clearTimeout(timer)
      signal?.removeEventListener("abort", abort)
    }
  }
}

function providerToolChoice(choice: ModelRequest["toolChoice"]) {
  if (choice && typeof choice === "object") {
    return { type: "function", function: { name: choice.operationId } }
  }
  return choice ?? "auto"
}

function providerHTTPError(status: number): string {
  if (status === 401 || status === 403) return "ai.provider_auth_failed"
  if (status === 402) return "ai.provider_quota_exhausted"
  if (status === 408 || status === 504) return "ai.provider_timeout"
  if (status === 429) return "ai.provider_rate_limited"
  if (status >= 500) return "ai.provider_unavailable"
  return "ai.provider_request_failed"
}

function providerPayloadError(value: unknown): string {
  if (!value || typeof value !== "object") return "ai.provider_stream_failed"
  const error = value as { code?: unknown, message?: unknown, type?: unknown }
  const fingerprint = [error.code, error.type, error.message]
    .filter(part => typeof part === "string")
    .join(" ")
    .toLowerCase()
  if (/insufficient.*(?:balance|quota|credit)|quota|billing/.test(fingerprint)) return "ai.provider_quota_exhausted"
  if (/unauthorized|forbidden|authentication|invalid.*(?:key|token)/.test(fingerprint)) return "ai.provider_auth_failed"
  if (/rate.?limit|too many requests/.test(fingerprint)) return "ai.provider_rate_limited"
  if (/timeout|timed out/.test(fingerprint)) return "ai.provider_timeout"
  if (/unavailable|overloaded/.test(fingerprint)) return "ai.provider_unavailable"
  return "ai.provider_stream_failed"
}

type ToolCallShape = { index: number, function?: { name?: string, arguments?: string } }
type MessageShape = { content?: unknown, reasoning_summary?: unknown, tool_calls?: Array<{ function?: { name?: string, arguments?: string } }> }
type CompletionBody = { choices?: Array<{ message?: MessageShape }>, usage?: { prompt_tokens?: number, completion_tokens?: number } }
type StreamChunk = {
  choices?: Array<{ delta?: { content?: unknown, reasoning_summary?: unknown, tool_calls?: ToolCallShape[] } }>
  usage?: { prompt_tokens?: number, completion_tokens?: number }
  error?: unknown
}

function textValue(value: unknown): string {
  return typeof value === "string" ? value : ""
}

function contentText(value: unknown): string {
  if (typeof value === "string") return value
  if (!Array.isArray(value)) return ""
  return value.map((part: unknown) => {
    if (!part || typeof part !== "object" || !("text" in part)) return ""
    return textValue((part as { text?: unknown }).text)
  }).join("")
}

function parseArguments(value: string): Record<string, unknown> {
  if (!value.trim()) return {}
  try {
    const parsed = JSON.parse(value) as unknown
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) return parsed as Record<string, unknown>
  } catch {
    throw new Error("ai.provider_invalid_tool_arguments")
  }
  throw new Error("ai.provider_invalid_tool_arguments")
}

function parseToolCalls(fragments: Map<number, { operationId: string, arguments: string }>): ModelToolCall[] {
  return [...fragments.entries()].sort(([a], [b]) => a - b).map(([, call]) => ({
    operationId: call.operationId,
    arguments: parseArguments(call.arguments),
  })).filter(call => call.operationId)
}

function ensureTrailingSlash(value: string): string {
  return value.endsWith("/") ? value : `${value}/`
}
