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
    const toolFragments = new Map<number, { id?: string, operationId: string, arguments: string }>()
    const reader = (response.body as ReadableStream<Uint8Array>).getReader()
    while (true) {
      const chunk = await reader.read().catch((error: unknown) => {
        throw providerTransportError(error, request.signal)
      })
      const { done, value } = chunk
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const frames = buffer.split(/\r?\n\r?\n/)
      buffer = frames.pop() ?? ""
      for (const frame of frames) {
        const data = frame.split(/\r?\n/).filter(line => line.startsWith("data:")).map(line => line.slice(5).trimStart()).join("\n")
        if (!data || data === "[DONE]") continue
        let payload: StreamChunk
        try {
          payload = JSON.parse(data) as StreamChunk
        } catch {
          throw new Error("ai.provider_stream_failed")
        }
        if (payload.error) throw new Error(providerPayloadError(payload.error))
        const delta = payload.choices?.[0]?.delta
        const reasoning = textValue(delta?.reasoning_summary)
        const content = contentText(delta?.content)
        if (reasoning) yield { type: "reasoning_summary_delta", delta: reasoning }
        if (content) yield { type: "message_delta", delta: content }
        for (const fragment of delta?.tool_calls ?? []) {
          const current = toolFragments.get(fragment.index) ?? { operationId: "", arguments: "" }
          if (!current.id && fragment.id) current.id = fragment.id
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
        ...(call.id ? { id: call.id } : {}),
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
      let response: Response
      try {
        response = await fetch(new URL("chat/completions", ensureTrailingSlash(this.options.baseUrl)), {
          method: "POST",
          headers: { authorization: `Bearer ${this.options.apiKey}`, "content-type": "application/json" },
          body: JSON.stringify({
            model: this.options.model,
            messages: providerMessages(messages),
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
      } catch (error) {
        throw providerTransportError(error, signal)
      }
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

function providerTransportError(error: unknown, signal?: AbortSignal): Error {
  if (error instanceof Error && error.message.startsWith("ai."))
    return error
  if (signal?.aborted)
    return signal.reason instanceof Error ? signal.reason : new Error("ai.run_canceled")
  if (error instanceof Error && error.name === "AbortError")
    return new Error("ai.provider_timeout")
  return new Error("ai.provider_unavailable")
}

type ToolCallShape = { index: number, id?: string, function?: { name?: string, arguments?: string } }
type MessageShape = { content?: unknown, reasoning_summary?: unknown, tool_calls?: Array<{ id?: string, function?: { name?: string, arguments?: string } }> }
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
    const repaired = trimTrailingObjectClosures(value)
    if (repaired) {
      try {
        const parsed = JSON.parse(repaired) as unknown
        if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
          console.warn(JSON.stringify({
            event: "provider_tool_arguments_trailing_closure_repaired",
            originalLength: value.length,
            repairedLength: repaired.length,
          }))
          return parsed as Record<string, unknown>
        }
      } catch {
        // Fall through to the stable provider error below.
      }
    }
    console.warn(JSON.stringify({
      event: "provider_invalid_tool_arguments",
      length: value.length,
      startsWithObject: value.trimStart().startsWith("{"),
      endsWithObject: value.trimEnd().endsWith("}"),
      openBraces: [...value].filter(character => character === "{").length,
      closeBraces: [...value].filter(character => character === "}").length,
    }))
    throw new Error("ai.provider_invalid_tool_arguments")
  }
  throw new Error("ai.provider_invalid_tool_arguments")
}

function trimTrailingObjectClosures(value: string): string | undefined {
  const input = value.trim()
  if (!input.startsWith("{"))
    return undefined
  let depth = 0
  let inString = false
  let escaped = false
  for (let index = 0; index < input.length; index += 1) {
    const character = input[index]
    if (inString) {
      if (escaped) {
        escaped = false
        continue
      }
      if (character === "\\") {
        escaped = true
        continue
      }
      if (character === "\"")
        inString = false
      continue
    }
    if (character === "\"") {
      inString = true
      continue
    }
    if (character === "{")
      depth += 1
    else if (character === "}")
      depth -= 1
    if (depth === 0 && character === "}") {
      const trailing = input.slice(index + 1)
      return /^}+$/.test(trailing) ? input.slice(0, index + 1) : undefined
    }
    if (depth < 0)
      return undefined
  }
  return undefined
}

function parseToolCalls(fragments: Map<number, { id?: string, operationId: string, arguments: string }>): ModelToolCall[] {
  return [...fragments.entries()].sort(([a], [b]) => a - b).map(([, call]) => ({
    ...(call.id ? { id: call.id } : {}),
    operationId: call.operationId,
    arguments: parseArguments(call.arguments),
  })).filter(call => call.operationId)
}

function providerMessages(messages: ModelRequest["messages"]) {
  return messages.map((message, messageIndex) => {
    if (message.role === "tool") {
      return { role: "tool", tool_call_id: message.toolCallId, content: message.content }
    }
    if (message.role === "assistant" && message.toolCalls?.length) {
      return {
        role: "assistant",
        content: message.content || null,
        tool_calls: message.toolCalls.map((call, callIndex) => ({
          id: call.id ?? `call_${messageIndex}_${callIndex}`,
          type: "function",
          function: {
            name: call.operationId,
            arguments: JSON.stringify(call.arguments),
          },
        })),
      }
    }
    return message
  })
}

function ensureTrailingSlash(value: string): string {
  return value.endsWith("/") ? value : `${value}/`
}
