import type { ModelCapabilities, ModelEvent, ModelProvider, ModelRequest, ModelResponse, ModelToolCall } from "./provider.js"
import { agentMetrics, clientSpanOptions, isAIContentCaptureEnabled, recordAIContent, telemetryLog, withSpan } from "../telemetry.js"
import { trace } from "@opentelemetry/api"
import { isRetryableHTTPStatus, parseRetryAfter, waitForRetry } from "../retry.js"

type Options = { baseUrl: string, apiKey: string, model: string, timeoutMs: number, maxRetries?: number }

export class OpenAICompatibleProvider implements ModelProvider {
  constructor(private readonly options: Options) {}

  capabilities(): ModelCapabilities {
    return { streaming: true, toolCalling: true, structuredOutput: true }
  }

  async health(): Promise<{ ok: boolean, requestId?: string }> {
    return withSpan("gen_ai.chat.health", clientSpanOptions(), async span => {
      const response = await this.request([{ role: "user", content: "只回复 OK。" }], 32)
      span.setAttribute("gen_ai.usage.input_tokens", response.usage.inputTokens)
      span.setAttribute("gen_ai.usage.output_tokens", response.usage.outputTokens)
      const ok = Boolean(response.text.trim() || response.reasoningSummary.trim() || response.usage.outputTokens > 0)
      return { ok, ...(response.requestId ? { requestId: response.requestId } : {}) }
    })
  }

  async complete(request: ModelRequest): Promise<ModelResponse> {
    return withSpan("gen_ai.chat.complete", clientSpanOptions(), async span => {
      const response = await this.request(request.messages, request.maxOutputTokens, request.signal, request.tools, request.toolChoice, request.thinking)
      span.setAttribute("gen_ai.usage.input_tokens", response.usage.inputTokens)
      span.setAttribute("gen_ai.usage.output_tokens", response.usage.outputTokens)
      span.setAttribute("luna.tool_call.count", response.toolCalls.length)
      return {
        text: response.text,
        usage: response.usage,
        ...(response.reasoningSummary ? { reasoningSummary: response.reasoningSummary } : {}),
        ...(response.toolCalls.length ? { toolCalls: response.toolCalls } : {}),
      }
    })
  }

  async *stream(request: ModelRequest): AsyncIterable<ModelEvent> {
    let retry = 0
    while (true) {
      let outputStarted = false
      try {
        for await (const event of this.streamAttempt(request)) {
          if (event.type !== "completed") outputStarted = true
          yield event
        }
        return
      }
      catch (error) {
        if (outputStarted || retry >= this.maxRetries || !isRetryableProviderError(error, true))
          throw error
        retry += 1
        await this.scheduleRetry(error, retry, "chat_stream", request.signal)
      }
    }
  }

  private async *streamAttempt(request: ModelRequest): AsyncIterable<ModelEvent> {
    const response = await this.fetchCompletionOnce(request.messages, request.maxOutputTokens, true, request.signal, request.tools, request.toolChoice, request.thinking)
    if (!response.body) throw new Error("ai.provider_empty_stream")
    const decoder = new TextDecoder()
    let buffer = ""
    let usage: { inputTokens: number, outputTokens: number, cachedInputTokens?: number, cachedOutputTokens?: number } = { inputTokens: 0, outputTokens: 0 }
    let responseText = ""
    let reasoningText = ""
    let toolCallDeltaEmitted = false
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
        const activeSpan = trace.getActiveSpan()
        if (payload.id) activeSpan?.setAttribute("gen_ai.response.id", payload.id)
        if (payload.model) activeSpan?.setAttribute("gen_ai.response.model", payload.model)
        const delta = payload.choices?.[0]?.delta
        const reasoning = extractReasoningText(delta)
        const content = contentText(delta?.content)
        if (reasoning) {
          reasoningText += reasoning
          yield { type: "reasoning_summary_delta", delta: reasoning }
        }
        if (content) {
          responseText += content
          yield { type: "message_delta", delta: content }
        }
        for (const fragment of delta?.tool_calls ?? []) {
          if (!toolCallDeltaEmitted) {
            toolCallDeltaEmitted = true
            yield { type: "tool_call_delta" }
          }
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
            ...(payload.usage.prompt_tokens_details?.cached_tokens !== undefined ? { cachedInputTokens: payload.usage.prompt_tokens_details.cached_tokens } : usage.cachedInputTokens !== undefined ? { cachedInputTokens: usage.cachedInputTokens } : {}),
            ...(payload.usage.completion_tokens_details?.cached_tokens !== undefined ? { cachedOutputTokens: payload.usage.completion_tokens_details.cached_tokens } : usage.cachedOutputTokens !== undefined ? { cachedOutputTokens: usage.cachedOutputTokens } : {}),
          }
        }
      }
    }
    const toolCalls = parseToolCalls(toolFragments)
    const activeSpan = trace.getActiveSpan()
    if (activeSpan) {
      recordAIContent(activeSpan, "gen_ai.content.output", "gen_ai.output.messages", {
        text: responseText,
        reasoningSummary: reasoningText,
        toolCalls,
        usage,
      })
    }
    yield { type: "completed", usage, ...(toolCalls.length ? { toolCalls } : {}) }
  }

  private async request(messages: ModelRequest["messages"], maxTokens: number, signal?: AbortSignal, tools?: ModelRequest["tools"], toolChoice?: ModelRequest["toolChoice"], thinking?: ModelRequest["thinking"]) {
    const response = await this.fetchCompletion(messages, maxTokens, false, signal, tools, toolChoice, thinking)
    const body = await response.json() as CompletionBody
    const activeSpan = trace.getActiveSpan()
    if (body.id) activeSpan?.setAttribute("gen_ai.response.id", body.id)
    if (body.model) activeSpan?.setAttribute("gen_ai.response.model", body.model)
    const message = body.choices?.[0]?.message
    const result = {
      text: contentText(message?.content),
      reasoningSummary: extractReasoningText(message),
      toolCalls: (message?.tool_calls ?? []).map((call) => {
        const parsed = parseArguments(call.function?.arguments ?? "")
        return {
          ...(call.id ? { id: call.id } : {}),
          operationId: call.function?.name ?? "",
          ...parsed,
        }
      }).filter(call => call.operationId),
      usage: {
        inputTokens: body.usage?.prompt_tokens ?? 0,
        outputTokens: body.usage?.completion_tokens ?? 0,
        ...(body.usage?.prompt_tokens_details?.cached_tokens !== undefined ? { cachedInputTokens: body.usage.prompt_tokens_details.cached_tokens } : {}),
        ...(body.usage?.completion_tokens_details?.cached_tokens !== undefined ? { cachedOutputTokens: body.usage.completion_tokens_details.cached_tokens } : {}),
      },
      requestId: response.headers.get("x-request-id") ?? undefined,
    }
    if (activeSpan) recordAIContent(activeSpan, "gen_ai.content.output", "gen_ai.output.messages", result)
    return result
  }

  private async fetchCompletion(messages: ModelRequest["messages"], maxTokens: number, stream: boolean, signal?: AbortSignal, tools?: ModelRequest["tools"], toolChoice?: ModelRequest["toolChoice"], thinking?: ModelRequest["thinking"]) {
    let retry = 0
    while (true) {
      try {
        return await this.fetchCompletionOnce(messages, maxTokens, stream, signal, tools, toolChoice, thinking)
      }
      catch (error) {
        if (retry >= this.maxRetries || !isRetryableProviderError(error, false))
          throw error
        retry += 1
        await this.scheduleRetry(error, retry, stream ? "chat_stream" : "chat_complete", signal)
      }
    }
  }

  private async fetchCompletionOnce(messages: ModelRequest["messages"], maxTokens: number, stream: boolean, signal?: AbortSignal, tools?: ModelRequest["tools"], toolChoice?: ModelRequest["toolChoice"], thinking?: ModelRequest["thinking"]) {
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), this.options.timeoutMs)
    const abort = () => controller.abort(signal?.reason)
    signal?.addEventListener("abort", abort, { once: true })
    const activeSpan = trace.getActiveSpan()
    activeSpan?.setAttributes({
      "gen_ai.provider.name": "openai_compatible",
      "gen_ai.request.model": this.options.model,
      "gen_ai.request.max_tokens": maxTokens,
      "gen_ai.request.streaming": stream,
      "server.address": new URL(this.options.baseUrl).hostname,
    })
    if (activeSpan) {
      recordAIContent(activeSpan, "gen_ai.content.input", "gen_ai.input.messages", {
        messages,
        tools,
        toolChoice,
        maxOutputTokens: maxTokens,
        streaming: stream,
      })
    }
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
            ...(thinking ? { thinking } : {}),
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
        if (activeSpan) {
          recordAIContent(activeSpan, "gen_ai.content.error", "gen_ai.response.error_body", contentError(error))
        }
        const transportError = providerTransportError(error, signal)
        agentMetrics.externalRequests.add(1, {
          target: "model_provider",
          operation: stream ? "chat_stream" : "chat_complete",
          outcome: transportError.message,
        })
        telemetryLog("agent.provider.request_failed", "warn", {
          "error.code": transportError.message,
        })
        throw transportError
      }
      if (!response.ok) {
        const errorCode = providerHTTPError(response.status)
        if (activeSpan && isAIContentCaptureEnabled()) {
          const responseBody = await response.clone().text().catch(() => "")
          recordAIContent(activeSpan, "gen_ai.content.error", "gen_ai.response.error_body", {
            status: response.status,
            body: responseBody,
          })
        }
        agentMetrics.externalRequests.add(1, {
          target: "model_provider",
          operation: stream ? "chat_stream" : "chat_complete",
          outcome: errorCode,
        })
        telemetryLog("agent.provider.request_failed", "warn", {
          "http.response.status_code": response.status,
          "error.code": errorCode,
        })
        throw new ProviderRequestError(errorCode, response.status, parseRetryAfter(response.headers))
      }
      activeSpan?.setAttribute("http.response.status_code", response.status)
      const providerRequestId = response.headers.get("x-request-id")
      if (providerRequestId) activeSpan?.setAttribute("server.request.id", providerRequestId)
      agentMetrics.externalRequests.add(1, {
        target: "model_provider",
        operation: stream ? "chat_stream" : "chat_complete",
        outcome: "success",
      })
      return response
    } finally {
      clearTimeout(timer)
      signal?.removeEventListener("abort", abort)
    }
  }

  private get maxRetries(): number {
    return Math.max(0, Math.min(10, this.options.maxRetries ?? 5))
  }

  private async scheduleRetry(error: unknown, attempt: number, operation: string, signal?: AbortSignal): Promise<void> {
    const retryAfterMs = error instanceof ProviderRequestError ? error.retryAfterMs : undefined
    trace.getActiveSpan()?.addEvent("gen_ai.request.retry_scheduled", {
      "retry.attempt": attempt,
      "retry.max_retries": this.maxRetries,
      "error.code": errorCode(error),
    })
    telemetryLog("agent.provider.retry_scheduled", "warn", {
      "retry.attempt": attempt,
      "retry.max_retries": this.maxRetries,
      "error.code": errorCode(error),
      operation,
    })
    await waitForRetry(attempt, { maxRetries: this.maxRetries, ...(signal ? { signal } : {}), ...(retryAfterMs !== undefined ? { retryAfterMs } : {}) })
  }
}

class ProviderRequestError extends Error {
  constructor(message: string, readonly status?: number, readonly retryAfterMs?: number) {
    super(message)
    this.name = "ProviderRequestError"
  }
}

function errorCode(error: unknown): string {
  return error instanceof Error ? error.message : "ai.provider_unknown"
}

function isRetryableProviderError(error: unknown, streamBody: boolean): boolean {
  if (error instanceof ProviderRequestError)
    return error.status === undefined || isRetryableHTTPStatus(error.status, true)
  if (!(error instanceof Error)) return false
  return error.message === "ai.provider_unavailable"
    || error.message === "ai.provider_timeout"
    || error.message === "ai.provider_rate_limited"
    || (streamBody && error.message === "ai.provider_stream_failed")
}

function contentError(error: unknown): Record<string, unknown> {
  return error instanceof Error
    ? { errorType: error.name, errorMessage: error.message, cause: error.cause }
    : { errorType: "UnknownError", errorMessage: String(error) }
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
type ReasoningShape = { reasoning_summary?: unknown, reasoning_content?: unknown }
type MessageShape = ReasoningShape & { content?: unknown, tool_calls?: Array<{ id?: string, function?: { name?: string, arguments?: string } }> }
type CompletionUsage = {
  prompt_tokens?: number
  completion_tokens?: number
  prompt_tokens_details?: { cached_tokens?: number }
  completion_tokens_details?: { cached_tokens?: number }
}
type CompletionBody = { id?: string, model?: string, choices?: Array<{ message?: MessageShape }>, usage?: CompletionUsage }
type StreamChunk = {
  id?: string
  model?: string
  choices?: Array<{ delta?: ReasoningShape & { content?: unknown, tool_calls?: ToolCallShape[] } }>
  usage?: CompletionUsage
  error?: unknown
}

function textValue(value: unknown): string {
  return typeof value === "string" ? value : ""
}

function extractReasoningText(value: ReasoningShape | undefined): string {
  return textValue(value?.reasoning_summary) || textValue(value?.reasoning_content)
}

function contentText(value: unknown): string {
  if (typeof value === "string") return value
  if (!Array.isArray(value)) return ""
  return value.map((part: unknown) => {
    if (!part || typeof part !== "object" || !("text" in part)) return ""
    return textValue((part as { text?: unknown }).text)
  }).join("")
}

function parseArguments(value: string): Pick<ModelToolCall, "arguments" | "argumentError"> {
  if (!value.trim()) return { arguments: {} }
  try {
    const parsed = JSON.parse(value) as unknown
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) return { arguments: parsed as Record<string, unknown> }
  } catch {
    const repaired = trimTrailingObjectClosures(value)
    if (repaired) {
      try {
        const parsed = JSON.parse(repaired) as unknown
        if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
          telemetryLog("agent.provider.tool_arguments_repaired", "warn", {
            "arguments.original_length": value.length,
            "arguments.repaired_length": repaired.length,
          })
          return { arguments: parsed as Record<string, unknown> }
        }
      } catch {
        // Fall through to the stable provider error below.
      }
    }
    telemetryLog("agent.provider.invalid_tool_arguments", "warn", {
      "error.code": "ai.provider_invalid_tool_arguments",
      "arguments.length": value.length,
      "arguments.starts_with_object": value.trimStart().startsWith("{"),
      "arguments.ends_with_object": value.trimEnd().endsWith("}"),
      "arguments.open_braces": [...value].filter(character => character === "{").length,
      "arguments.close_braces": [...value].filter(character => character === "}").length,
    })
    return {
      arguments: {},
      argumentError: {
        code: "invalid_json",
        message: "工具参数不是完整的 JSON 对象。",
      },
    }
  }
  return {
    arguments: {},
    argumentError: {
      code: "invalid_json",
      message: "工具参数必须是 JSON 对象。",
    },
  }
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
  return [...fragments.entries()].sort(([a], [b]) => a - b).map(([, call]) => {
    const parsed = parseArguments(call.arguments)
    return {
      ...(call.id ? { id: call.id } : {}),
      operationId: call.operationId,
      ...parsed,
    }
  }).filter(call => call.operationId)
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
