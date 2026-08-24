import type { Attributes, Span } from "@opentelemetry/api"
import { createHash } from "node:crypto"
import { trace } from "@opentelemetry/api"
import { z } from "zod"
import { genAIClientTokenUsageAttributes, genAIInputMessages, genAIModelSpan, genAIOutputMessages, genAIToolDefinitions } from "../genai-semconv.js"
import { agentMetrics, clientSpanOptions, errorDiagnostic, recordAIContent, telemetryLog, withSpan, withSpanStream } from "../telemetry.js"
import { mapProviderError, parseProviderErrorBody, ProviderRequestError } from "./provider-error.js"
import type {
  ModelCapabilities,
  ModelEvent,
  ModelProvider,
  ModelRequest,
  ModelResponse,
  ModelToolCall,
  ModelUsage,
  OfficialModelUsage,
} from "./provider.js"

export type OpenAIChatCompletionsOptions = {
  baseUrl: string
  apiKey: string
  channelAffinityEnabled: boolean
  model: string
  timeoutMs: number
}

const nonNegativeSafeInteger = z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER)
const requestToolCallSchema = z.object({
  id: z.string(),
  type: z.literal("function"),
  function: z.object({ name: z.string().min(1), arguments: z.string() }).strict(),
}).strict()
const requestMessageSchema = z.discriminatedUnion("role", [
  z.object({ role: z.literal("system"), content: z.string() }).strict(),
  z.object({ role: z.literal("user"), content: z.string() }).strict(),
  z.object({ role: z.literal("assistant"), content: z.string().nullable(), tool_calls: z.array(requestToolCallSchema).optional() }).strict(),
  z.object({ role: z.literal("tool"), tool_call_id: z.string().min(1), content: z.string() }).strict(),
])
const requestSchema = z.object({
  model: z.string().min(1),
  messages: z.array(requestMessageSchema).min(1),
  max_completion_tokens: nonNegativeSafeInteger.refine(value => value > 0),
  stream: z.boolean(),
  stream_options: z.object({ include_usage: z.literal(true) }).strict().optional(),
  tools: z.array(z.object({
    type: z.literal("function"),
    function: z.object({
      name: z.string().min(1),
      description: z.string(),
      parameters: z.record(z.string(), z.unknown()),
    }).strict(),
  }).strict()).optional(),
  tool_choice: z.union([
    z.enum(["auto", "none", "required"]),
    z.object({ type: z.literal("function"), function: z.object({ name: z.string().min(1) }).strict() }).strict(),
  ]).optional(),
}).strict().superRefine((request, context) => {
  if (request.stream && request.stream_options?.include_usage !== true)
    context.addIssue({ code: "custom", message: "stream_usage_required", path: ["stream_options"] })
  if (!request.stream && request.stream_options !== undefined)
    context.addIssue({ code: "custom", message: "stream_options_without_stream", path: ["stream_options"] })
  if ((request.tools === undefined) !== (request.tool_choice === undefined))
    context.addIssue({ code: "custom", message: "tools_and_choice_must_coexist", path: ["tools"] })
})
const usageSchema = z.object({
  prompt_tokens: nonNegativeSafeInteger,
  completion_tokens: nonNegativeSafeInteger,
  total_tokens: nonNegativeSafeInteger,
  prompt_tokens_details: z.object({
    cached_tokens: nonNegativeSafeInteger.optional(),
    cache_write_tokens: nonNegativeSafeInteger.optional(),
  }).passthrough().optional(),
  completion_tokens_details: z.object({
    reasoning_tokens: nonNegativeSafeInteger.optional(),
  }).passthrough().optional(),
}).passthrough().superRefine((usage, context) => {
  if (usage.total_tokens !== usage.prompt_tokens + usage.completion_tokens) {
    context.addIssue({ code: "custom", message: "total_tokens_mismatch", path: ["total_tokens"] })
  }
  if ((usage.prompt_tokens_details?.cached_tokens ?? 0) > usage.prompt_tokens) {
    context.addIssue({ code: "custom", message: "cached_tokens_exceed_prompt", path: ["prompt_tokens_details", "cached_tokens"] })
  }
  if ((usage.prompt_tokens_details?.cache_write_tokens ?? 0) > usage.prompt_tokens) {
    context.addIssue({ code: "custom", message: "cache_write_tokens_exceed_prompt", path: ["prompt_tokens_details", "cache_write_tokens"] })
  }
  if ((usage.prompt_tokens_details?.cached_tokens ?? 0) + (usage.prompt_tokens_details?.cache_write_tokens ?? 0) > usage.prompt_tokens) {
    context.addIssue({ code: "custom", message: "prompt_detail_tokens_exceed_prompt", path: ["prompt_tokens_details"] })
  }
  if ((usage.completion_tokens_details?.reasoning_tokens ?? 0) > usage.completion_tokens) {
    context.addIssue({ code: "custom", message: "reasoning_tokens_exceed_completion", path: ["completion_tokens_details", "reasoning_tokens"] })
  }
})

const toolCallSchema = z.object({
  id: z.string().optional(),
  function: z.object({ name: z.string().optional(), arguments: z.string().optional() }).passthrough().optional(),
}).passthrough()
const messageSchema = z.object({
  content: z.unknown().optional(),
  tool_calls: z.array(toolCallSchema).optional(),
}).passthrough()
const completionSchema = z.object({
  id: z.string(),
  model: z.string(),
  choices: z.array(z.object({ message: messageSchema.optional(), finish_reason: z.string().nullable().optional() }).passthrough()),
  usage: z.unknown().optional(),
  service_tier: z.string().optional(),
  system_fingerprint: z.string().optional(),
}).passthrough()
const streamChunkSchema = z.object({
  id: z.string().optional(),
  model: z.string().optional(),
  choices: z.array(z.object({
    delta: z.object({
      content: z.unknown().optional(),
      tool_calls: z.array(z.object({
        index: nonNegativeSafeInteger,
        id: z.string().optional(),
        function: z.object({ name: z.string().optional(), arguments: z.string().optional() }).passthrough().optional(),
      }).passthrough()).optional(),
    }).passthrough().optional(),
    finish_reason: z.string().nullable().optional(),
  }).passthrough()),
  usage: z.unknown().optional(),
  service_tier: z.string().optional(),
  system_fingerprint: z.string().optional(),
  error: z.unknown().optional(),
}).passthrough()

export class OpenAIChatCompletionsProvider implements ModelProvider {
  constructor(protected readonly options: OpenAIChatCompletionsOptions) {}

  capabilities(): ModelCapabilities {
    return { streaming: true, toolCalling: true, structuredOutput: true }
  }

  async health(): Promise<{ ok: boolean, requestId?: string }> {
    const response = await this.complete({ messages: [{ role: "user", content: "只回复 OK。" }], maxOutputTokens: 32 })
    return {
      ok: Boolean(response.text.trim() || response.usage.status === "reported" && response.usage.value.outputTokens > 0),
      ...(response.providerRequestId ? { requestId: response.providerRequestId } : {}),
    }
  }

  async complete(request: ModelRequest): Promise<ModelResponse> {
    const modelSpan = genAIModelSpan(this.options.baseUrl, this.providerName(), this.options.model, request.maxOutputTokens, false)
    return withSpan(modelSpan.name, clientSpanOptions({ ...modelSpan.attributes, ...requestIdentity(request) }), async span => {
      this.recordRequest(span, request)
      const response = await this.fetchCompletion(request, false)
      const providerRequestId = response.headers.get("x-request-id") ?? undefined
      const raw = await response.json().catch(() => {
        throw new ProviderRequestError("ai.provider_response_invalid", {
          stage: "response_body", requestOutcome: "unknown", ...(providerRequestId ? { providerRequestId } : {}),
        })
      })
      const parsed = completionSchema.safeParse(raw)
      if (!parsed.success) throw this.schemaError("ai.provider_response_invalid", "response_body", providerRequestId)
      const body = parsed.data
      const message = body.choices[0]?.message
      const toolCalls = (message?.tool_calls ?? []).map(parseToolCall).filter(call => call.operationId)
      const usage = parseUsage(this.adaptUsagePayload(body.usage), "missing_usage")
      const finishReason = body.choices[0]?.finish_reason ?? undefined
      this.recordResponse(span, usage, finishReason, toolCalls.length, body.id, body.model)
      const text = contentText(message?.content)
      recordAIContent(span, "luna.gen_ai.content.output", "gen_ai.output.messages", genAIOutputMessages({ text, toolCalls, ...(finishReason ? { finishReason } : {}) }))
      return {
        text,
        usage,
        ...(toolCalls.length ? { toolCalls } : {}),
        ...(finishReason ? { finishReason } : {}),
        ...(providerRequestId ? { providerRequestId } : {}),
        responseId: body.id,
        responseModel: body.model,
        ...(body.service_tier ? { serviceTier: body.service_tier } : {}),
        ...(body.system_fingerprint ? { systemFingerprint: body.system_fingerprint } : {}),
      }
    })
  }

  async *stream(request: ModelRequest): AsyncIterable<ModelEvent> {
    const modelSpan = genAIModelSpan(this.options.baseUrl, this.providerName(), this.options.model, request.maxOutputTokens, true)
    yield* withSpanStream(modelSpan.name, clientSpanOptions({ ...modelSpan.attributes, ...requestIdentity(request) }), span => this.streamAttempt(request, span))
  }

  protected buildRequestBody(request: ModelRequest, stream: boolean): Record<string, unknown> {
    const parsed = requestSchema.safeParse({
      model: this.options.model,
      messages: providerMessages(request.messages),
      max_completion_tokens: request.maxOutputTokens,
      stream,
      ...(stream ? { stream_options: { include_usage: true } } : {}),
      ...(request.tools?.length ? {
        tools: request.tools.map(tool => ({
          type: "function",
          function: { name: tool.operationId, description: tool.description, parameters: tool.inputSchema },
        })),
        tool_choice: providerToolChoice(request.toolChoice),
      } : {}),
    })
    if (!parsed.success) {
      throw new ProviderRequestError("ai.provider_request_invalid", {
        stage: "dispatch",
        requestOutcome: "not_dispatched",
      })
    }
    return parsed.data
  }

  protected reasoningDelta(_delta: Record<string, unknown> | undefined): string {
    void _delta
    return ""
  }

  protected providerName(): string {
    return "openai"
  }

  protected adaptUsagePayload(raw: unknown): unknown {
    return raw
  }

  private async *streamAttempt(request: ModelRequest, span: Span): AsyncIterable<ModelEvent> {
    this.recordRequest(span, request)
    const response = await this.fetchCompletion(request, true)
    const providerRequestId = response.headers.get("x-request-id") ?? undefined
    if (!response.body) throw this.schemaError("ai.provider_empty_stream", "stream", providerRequestId)
    let usage: ModelUsage = { status: "unavailable", reason: "stream_ended_without_usage" }
    let usageSeen = false
    let finishReason: string | undefined
    let responseId: string | undefined
    let responseModel: string | undefined
    let responseText = ""
    let reasoningText = ""
    let toolDeltaEmitted = false
    const toolFragments = new Map<number, { id?: string, operationId: string, arguments: string }>()
    for await (const data of readSSEData(response.body, request.signal)) {
      if (!data || data === "[DONE]") continue
      let raw: unknown
      try { raw = JSON.parse(data) }
      catch { throw this.schemaError("ai.provider_stream_invalid", "stream", providerRequestId) }
      const parsed = streamChunkSchema.safeParse(raw)
      if (!parsed.success) throw this.schemaError("ai.provider_stream_invalid", "stream", providerRequestId)
      const chunk = parsed.data
      if (chunk.error !== undefined) throw providerPayloadError(chunk.error, providerRequestId, responseId, responseModel)
      responseId = chunk.id ?? responseId
      responseModel = chunk.model ?? responseModel
      const choice = chunk.choices[0]
      const delta = choice?.delta as Record<string, unknown> | undefined
      finishReason = choice?.finish_reason ?? finishReason
      const reasoning = this.reasoningDelta(delta)
      const content = contentText(delta?.content)
      if (reasoning) { reasoningText += reasoning; yield { type: "reasoning_summary_delta", delta: reasoning } }
      if (content) { responseText += content; yield { type: "message_delta", delta: content } }
      for (const fragment of choice?.delta?.tool_calls ?? []) {
        if (!toolDeltaEmitted) { toolDeltaEmitted = true; yield { type: "tool_call_delta" } }
        const current = toolFragments.get(fragment.index) ?? { operationId: "", arguments: "" }
        if (!current.id && fragment.id) current.id = fragment.id
        current.operationId += fragment.function?.name ?? ""
        current.arguments += fragment.function?.arguments ?? ""
        toolFragments.set(fragment.index, current)
      }
      if (chunk.usage !== undefined) {
        usageSeen = true
        usage = parseUsage(this.adaptUsagePayload(chunk.usage), "invalid_usage")
      }
    }
    if (!usageSeen) usage = { status: "unavailable", reason: "stream_ended_without_usage" }
    const toolCalls = parseToolFragments(toolFragments)
    this.recordResponse(span, usage, finishReason, toolCalls.length, responseId, responseModel)
    recordAIContent(span, "luna.gen_ai.content.output", "gen_ai.output.messages", genAIOutputMessages({
      text: responseText, ...(reasoningText ? { reasoningSummary: reasoningText } : {}), ...(toolCalls.length ? { toolCalls } : {}), ...(finishReason ? { finishReason } : {}),
    }))
    yield {
      type: "completed", usage,
      ...(toolCalls.length ? { toolCalls } : {}),
      ...(finishReason ? { finishReason } : {}),
      ...(providerRequestId ? { providerRequestId } : {}),
      ...(responseId ? { responseId } : {}),
      ...(responseModel ? { responseModel } : {}),
    }
  }

  private async fetchCompletion(request: ModelRequest, stream: boolean): Promise<Response> {
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), this.options.timeoutMs)
    const abort = () => controller.abort(request.signal?.reason)
    request.signal?.addEventListener("abort", abort, { once: true })
    try {
      let response: Response
      try {
        response = await fetch(new URL("chat/completions", ensureTrailingSlash(this.options.baseUrl)), {
          method: "POST",
          headers: this.requestHeaders(request),
          body: JSON.stringify(this.buildRequestBody(request, stream)),
          signal: controller.signal,
        })
      }
      catch (error) {
        const mapped = transportError(error, request.signal)
        telemetryLog("agent.provider.request_failed", "warn", { operation: stream ? "agent.provider.chat_stream" : "agent.provider.chat_complete", outcome: "failed", ...errorDiagnostic(mapped, mapped.message) })
        throw mapped
      }
      const providerRequestId = response.headers.get("x-request-id") ?? undefined
      if (!response.ok) {
        const raw = await response.clone().json().catch(() => undefined)
        const detail = parseProviderErrorBody(raw)
        const code = mapProviderError(response.status, detail)
        const error = new ProviderRequestError(code, {
          status: response.status, stage: "response_headers", requestOutcome: "rejected",
          ...(detail.code ? { providerCode: detail.code } : {}),
          ...(detail.type ? { providerType: detail.type } : {}),
          ...(detail.param ? { providerParam: detail.param } : {}),
          ...(providerRequestId ? { providerRequestId } : {}),
        })
        telemetryLog("agent.provider.request_failed", "warn", {
          operation: stream ? "agent.provider.chat_stream" : "agent.provider.chat_complete", outcome: "failed",
          "http.response.status_code": response.status, "provider.error.code": detail.code ?? "unknown", ...errorDiagnostic(error, code),
        })
        throw error
      }
      agentMetrics.externalRequests.add(1, { target: "model_provider", operation: stream ? "chat_stream" : "chat_complete", outcome: "success" })
      trace.getActiveSpan()?.setAttribute("http.response.status_code", response.status)
      if (providerRequestId) trace.getActiveSpan()?.setAttribute("server.request.id", providerRequestId)
      return response
    }
    finally {
      clearTimeout(timer)
      request.signal?.removeEventListener("abort", abort)
    }
  }

  private schemaError(code: string, stage: "response_body" | "stream", providerRequestId?: string): ProviderRequestError {
    return new ProviderRequestError(code, { stage, requestOutcome: "unknown", ...(providerRequestId ? { providerRequestId } : {}) })
  }

  private recordRequest(span: Span, request: ModelRequest): void {
    span.setAttribute("luna.gen_ai.channel_affinity.applied", Boolean(this.channelAffinityKey(request)))
    recordAIContent(span, "luna.gen_ai.content.input", "gen_ai.input.messages", genAIInputMessages(request.messages))
    const system = request.messages.filter(message => message.role === "system")
    if (system.length) recordAIContent(span, "luna.gen_ai.content.system_instructions", "gen_ai.system_instructions", genAIInputMessages(system))
    if (request.tools?.length) recordAIContent(span, "luna.gen_ai.content.tools", "gen_ai.tool.definitions", genAIToolDefinitions(request.tools))
  }

  private requestHeaders(request: ModelRequest): Record<string, string> {
    const affinityKey = this.channelAffinityKey(request)
    return {
      authorization: `Bearer ${this.options.apiKey}`,
      "content-type": "application/json",
      ...(affinityKey ? { "X-Luna-Affinity-Key": affinityKey } : {}),
    }
  }

  private channelAffinityKey(request: ModelRequest): string | undefined {
    if (!this.options.channelAffinityEnabled || !request.conversationId) return undefined
    return createHash("sha256")
      .update(`luna.devops.channel-affinity.v1\0${request.conversationId}`)
      .digest("hex")
  }

  private recordResponse(span: Span, usage: ModelUsage, finishReason: string | undefined, toolCallCount: number, responseId?: string, responseModel?: string): void {
    span.setAttributes(modelUsageSpanAttributes(usage))
    if (usage.status === "reported") {
      agentMetrics.modelTokenUsage.record(
        usage.value.inputTokens,
        genAIClientTokenUsageAttributes(this.providerName(), this.options.model, "input", responseModel),
      )
      agentMetrics.modelTokenUsage.record(
        usage.value.outputTokens,
        genAIClientTokenUsageAttributes(this.providerName(), this.options.model, "output", responseModel),
      )
    }
    if (finishReason) span.setAttribute("gen_ai.response.finish_reasons", [finishReason])
    if (responseId) span.setAttribute("gen_ai.response.id", responseId)
    if (responseModel) span.setAttribute("gen_ai.response.model", responseModel)
    span.setAttribute("luna.tool_call.count", toolCallCount)
  }
}

export function modelUsageSpanAttributes(usage: ModelUsage): Attributes {
  if (usage.status === "unavailable") {
    return {
      "luna.gen_ai.usage.status": usage.status,
      "luna.gen_ai.usage.unavailable_reason": usage.reason,
    }
  }
  return {
    "gen_ai.usage.input_tokens": usage.value.inputTokens,
    "gen_ai.usage.output_tokens": usage.value.outputTokens,
    ...(usage.value.cacheReadInputTokens !== undefined ? { "gen_ai.usage.cache_read.input_tokens": usage.value.cacheReadInputTokens } : {}),
    ...(usage.value.cacheWriteInputTokens !== undefined ? { "gen_ai.usage.cache_write.input_tokens": usage.value.cacheWriteInputTokens } : {}),
    ...(usage.value.reasoningOutputTokens !== undefined ? { "gen_ai.usage.reasoning.output_tokens": usage.value.reasoningOutputTokens } : {}),
  }
}

function parseUsage(raw: unknown, absentReason: "missing_usage" | "invalid_usage"): ModelUsage {
  if (raw === undefined) return { status: "unavailable", reason: absentReason }
  const parsed = usageSchema.safeParse(raw)
  if (!parsed.success) return { status: "unavailable", reason: "invalid_usage" }
  const usage = parsed.data
  const value: OfficialModelUsage = {
    inputTokens: usage.prompt_tokens,
    outputTokens: usage.completion_tokens,
    totalTokens: usage.total_tokens,
    ...(usage.prompt_tokens_details?.cached_tokens !== undefined ? { cacheReadInputTokens: usage.prompt_tokens_details.cached_tokens } : {}),
    ...(usage.prompt_tokens_details?.cache_write_tokens !== undefined ? { cacheWriteInputTokens: usage.prompt_tokens_details.cache_write_tokens } : {}),
    ...(usage.completion_tokens_details?.reasoning_tokens !== undefined ? { reasoningOutputTokens: usage.completion_tokens_details.reasoning_tokens } : {}),
  }
  return { status: "reported", value }
}

async function* readSSEData(body: ReadableStream<Uint8Array>, signal?: AbortSignal): AsyncIterable<string> {
  const reader = body.getReader()
  const decoder = new TextDecoder()
  let buffer = ""
  while (true) {
    const result = await reader.read().catch((error) => { throw transportError(error, signal) })
    if (result.done) break
    buffer += decoder.decode(result.value, { stream: true })
    const frames = buffer.split(/\r?\n\r?\n/)
    buffer = frames.pop() ?? ""
    for (const frame of frames) yield sseFrameData(frame)
  }
  buffer += decoder.decode()
  if (buffer.trim()) yield sseFrameData(buffer)
}

function sseFrameData(frame: string): string {
  return frame.split(/\r?\n/).filter(line => line.startsWith("data:")).map(line => line.slice(5).trimStart()).join("\n")
}

function providerPayloadError(value: unknown, providerRequestId?: string, responseId?: string, responseModel?: string): ProviderRequestError {
  const detail = parseProviderErrorBody({ error: value })
  return new ProviderRequestError(mapProviderError(500, detail), {
    stage: "stream", requestOutcome: "unknown",
    ...(detail.code ? { providerCode: detail.code } : {}), ...(detail.type ? { providerType: detail.type } : {}), ...(detail.param ? { providerParam: detail.param } : {}),
    ...(providerRequestId ? { providerRequestId } : {}), ...(responseId ? { responseId } : {}), ...(responseModel ? { responseModel } : {}),
  })
}

function transportError(error: unknown, signal?: AbortSignal): ProviderRequestError {
  if (error instanceof ProviderRequestError) return error
  if (signal?.aborted) return new ProviderRequestError("ai.run_canceled", { stage: "dispatch", requestOutcome: "not_dispatched" })
  if (error instanceof Error && error.name === "AbortError") return new ProviderRequestError("ai.provider_timeout", { stage: "dispatch", requestOutcome: "unknown" })
  return new ProviderRequestError("ai.provider_unavailable", { stage: "dispatch", requestOutcome: "unknown" })
}

function requestIdentity(request: ModelRequest): Record<string, string | boolean> {
  return {
    ...(request.conversationId ? { "gen_ai.conversation.id": request.conversationId } : {}),
    ...(request.conversationCompacted !== undefined ? { "gen_ai.conversation.compacted": request.conversationCompacted } : {}),
  }
}

function providerToolChoice(choice: ModelRequest["toolChoice"]): unknown {
  return choice && typeof choice === "object" ? { type: "function", function: { name: choice.operationId } } : choice ?? "auto"
}

function providerMessages(messages: ModelRequest["messages"]): unknown[] {
  return messages.map((message, messageIndex) => {
    if (message.role === "tool") return { role: "tool", tool_call_id: message.toolCallId, content: message.content }
    if (message.role === "assistant" && message.toolCalls?.length) {
      return {
        role: "assistant", content: message.content || null,
        tool_calls: message.toolCalls.map((call, callIndex) => ({
          id: call.id ?? `call_${messageIndex}_${callIndex}`, type: "function",
          function: { name: call.operationId, arguments: JSON.stringify(call.arguments) },
        })),
      }
    }
    return message
  })
}

function parseToolCall(call: z.infer<typeof toolCallSchema>): ModelToolCall {
  return { ...(call.id ? { id: call.id } : {}), operationId: call.function?.name ?? "", ...parseArguments(call.function?.arguments ?? "") }
}

function parseToolFragments(fragments: Map<number, { id?: string, operationId: string, arguments: string }>): ModelToolCall[] {
  return [...fragments.entries()].sort(([left], [right]) => left - right).map(([, call]) => ({
    ...(call.id ? { id: call.id } : {}), operationId: call.operationId, ...parseArguments(call.arguments),
  })).filter(call => call.operationId)
}

function parseArguments(value: string): Pick<ModelToolCall, "arguments" | "argumentError"> {
  if (!value.trim()) return { arguments: {} }
  try {
    const parsed: unknown = JSON.parse(value)
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) return { arguments: parsed as Record<string, unknown> }
  }
  catch {
    return { arguments: {}, argumentError: { code: "invalid_json", message: "工具参数不是完整的 JSON 对象。" } }
  }
  return { arguments: {}, argumentError: { code: "invalid_json", message: "工具参数不是完整的 JSON 对象。" } }
}

function contentText(value: unknown): string {
  if (typeof value === "string") return value
  if (!Array.isArray(value)) return ""
  return value.map((part: unknown) => {
    if (!part || typeof part !== "object" || Array.isArray(part)) return ""
    const record = part as Record<string, unknown>
    return typeof record.text === "string" ? record.text : ""
  }).join("")
}

function ensureTrailingSlash(value: string): string {
  return value.endsWith("/") ? value : `${value}/`
}
