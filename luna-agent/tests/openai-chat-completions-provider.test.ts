import { afterEach, describe, expect, it, vi } from "vitest"
import { DeepSeekChatCompletionsProvider } from "../src/provider/deepseek-chat-completions.js"
import { modelRequestSpanAttributes, modelUsageSpanAttributes, OpenAIChatCompletionsProvider, parseUsage } from "../src/provider/openai-chat-completions.js"
import { ProviderRequestError } from "../src/provider/provider-error.js"

const options = { baseUrl: "https://provider.example/v1", apiKey: "secret", channelAffinityEnabled: true, promptCacheKeyEnabled: false, model: "model-a", timeoutMs: 5_000 }
const request = { messages: [{ role: "user" as const, content: "hello" }], maxOutputTokens: 100 }

afterEach(() => vi.unstubAllGlobals())

function streamResponse(frames: string[], trailingSeparator = true) {
  const payload = frames.map(frame => `data: ${frame}\n\n`).join("").replace(/\n\n$/, trailingSeparator ? "\n\n" : "")
  const bytes = new TextEncoder().encode(payload)
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(bytes.slice(0, Math.min(37, bytes.length)))
      controller.enqueue(bytes.slice(Math.min(37, bytes.length)))
      controller.close()
    },
  })
  return new Response(body, { status: 200, headers: { "content-type": "text/event-stream", "x-request-id": "req_provider" } })
}

async function collect(provider: OpenAIChatCompletionsProvider) {
  const events = []
  for await (const event of provider.stream(request)) events.push(event)
  return events
}

describe("OpenAIChatCompletionsProvider official usage", () => {
  it("sends a stable pseudonymous affinity key only for conversation-bound requests when enabled", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({
      id: "chatcmpl_affinity", model: "model-a", choices: [{ message: { content: "done" } }],
      usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 },
    }), { status: 200 }))
    vi.stubGlobal("fetch", fetchMock)
    const provider = new OpenAIChatCompletionsProvider(options)

    await provider.complete({ ...request, conversationId: "aicnv_private_value" })
    await provider.complete({ ...request, conversationId: "aicnv_private_value" })
    await provider.complete({ ...request, conversationId: "aicnv_other_value" })

    const keys = fetchMock.mock.calls.map(([, init]) => new Headers(init?.headers).get("x-luna-affinity-key"))
    expect(keys[0]).toMatch(/^[a-f0-9]{64}$/)
    expect(keys[1]).toBe(keys[0])
    expect(keys[2]).not.toBe(keys[0])
    expect(keys.join(" ")).not.toContain("aicnv_private_value")

    await new OpenAIChatCompletionsProvider({ ...options, channelAffinityEnabled: false })
      .complete({ ...request, conversationId: "aicnv_private_value" })
    await provider.complete(request)
    expect(new Headers(fetchMock.mock.calls[3]?.[1]?.headers).has("x-luna-affinity-key")).toBe(false)
    expect(new Headers(fetchMock.mock.calls[4]?.[1]?.headers).has("x-luna-affinity-key")).toBe(false)
  })

  it("sends a distinct stable prompt cache key only for opted-in conversation-bound requests", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({
      id: "chatcmpl_prompt_cache", model: "model-a", choices: [{ message: { content: "done" } }],
      usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 },
    }), { status: 200 }))
    vi.stubGlobal("fetch", fetchMock)
    const provider = new OpenAIChatCompletionsProvider({ ...options, promptCacheKeyEnabled: true })
    const assistantRequest = {
      ...request,
      conversationId: "aicnv_private_value",
      budget: { runId: "airun_private", ownerUserId: "usr_private", operation: "assistant" as const },
    }

    await provider.complete(assistantRequest)
    await provider.complete(assistantRequest)
    await provider.complete({ ...assistantRequest, conversationId: "aicnv_other_value" })
    await new OpenAIChatCompletionsProvider(options).complete(assistantRequest)
    await provider.complete(request)
    await provider.complete({ ...assistantRequest, budget: { ...assistantRequest.budget, operation: "summary" } })

    const bodies = fetchMock.mock.calls.map(([, init]) => {
      const raw = init?.body
      if (typeof raw !== "string") throw new TypeError("expected JSON request body")
      return JSON.parse(raw) as Record<string, unknown>
    })
    const keys = bodies.map(body => body.prompt_cache_key)
    expect(keys[0]).toMatch(/^[a-f0-9]{64}$/)
    expect(keys[1]).toBe(keys[0])
    expect(keys[2]).not.toBe(keys[0])
    expect(String(keys[0])).not.toContain("aicnv_private_value")
    expect(keys[3]).toBeUndefined()
    expect(keys[4]).toBeUndefined()
    expect(keys[5]).toBeUndefined()
    const affinity = new Headers(fetchMock.mock.calls[0]?.[1]?.headers).get("x-luna-affinity-key")
    expect(keys[0]).not.toBe(affinity)
  })

  it("never sends prompt_cache_key through the DeepSeek adapter", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({
      id: "deepseek_prompt_cache", model: "deepseek-chat", choices: [{ message: { content: "done" } }],
      usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 },
    }), { status: 200 }))
    vi.stubGlobal("fetch", fetchMock)

    await new DeepSeekChatCompletionsProvider({ ...options, promptCacheKeyEnabled: true })
      .complete({
        ...request,
        conversationId: "aicnv_private_value",
        budget: { runId: "airun_private", ownerUserId: "usr_private", operation: "assistant" },
      })

    expect(requestBody(fetchMock)).not.toHaveProperty("prompt_cache_key")
  })

  it.each(["assistant", "summary", "title"] as const)("records the low-cardinality %s model-request purpose", (operation) => {
    expect(modelRequestSpanAttributes({
      ...request,
      budget: { runId: "airun_private", ownerUserId: "usr_private", operation },
    })).toMatchObject({ "luna.gen_ai.request.purpose": operation })
  })

  it("omits a model-request purpose for the unbudgeted health probe", () => {
    expect(modelRequestSpanAttributes(request)).not.toHaveProperty("luna.gen_ai.request.purpose")
  })

  it("accepts a complete non-streaming official usage payload", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      id: "chatcmpl_1", model: "model-a", choices: [{ message: { content: "done" }, finish_reason: "stop" }],
      usage: { prompt_tokens: 12, completion_tokens: 8, total_tokens: 20 },
    }), { status: 200, headers: { "x-request-id": "req_1" } })))

    await expect(new OpenAIChatCompletionsProvider(options).complete(request)).resolves.toMatchObject({
      text: "done", providerRequestId: "req_1", responseId: "chatcmpl_1", responseModel: "model-a", finishReason: "stop",
      usage: { status: "reported", value: { inputTokens: 12, outputTokens: 8, totalTokens: 20 } },
    })
  })

  it("reads an empty-choices final usage-only stream chunk", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () => streamResponse([
      JSON.stringify({ id: "chatcmpl_2", model: "model-a", choices: [{ delta: { content: "done" }, finish_reason: "stop" }] }),
      JSON.stringify({ id: "chatcmpl_2", model: "model-a", choices: [], usage: { prompt_tokens: 20, completion_tokens: 5, total_tokens: 25 } }),
      "[DONE]",
    ]))
    vi.stubGlobal("fetch", fetchMock)

    const events = await collect(new OpenAIChatCompletionsProvider(options))

    expect(events.at(-1)).toMatchObject({
      type: "completed", providerRequestId: "req_provider", responseId: "chatcmpl_2", responseModel: "model-a", finishReason: "stop",
      usage: { status: "reported", value: { inputTokens: 20, outputTokens: 5, totalTokens: 25 } },
    })
    const body = requestBody(fetchMock)
    expect(body).toMatchObject({ stream: true, stream_options: { include_usage: true }, max_completion_tokens: 100 })
    expect(body).not.toHaveProperty("max_tokens")
  })

  it("preserves official cached prompt and reasoning details", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      id: "chatcmpl_3", model: "model-a", choices: [{ message: { content: "done" } }],
      usage: {
        prompt_tokens: 100, completion_tokens: 20, total_tokens: 120,
        prompt_tokens_details: { cached_tokens: 70, cache_write_tokens: 10 },
        completion_tokens_details: { reasoning_tokens: 15 },
      },
    }), { status: 200 })))

    const result = await new OpenAIChatCompletionsProvider(options).complete(request)
    expect(result.usage).toEqual({ status: "reported", value: {
      inputTokens: 100, outputTokens: 20, totalTokens: 120,
      cacheReadInputTokens: 70, cacheWriteInputTokens: 10, reasoningOutputTokens: 15,
    } })
  })

  it("uses current GenAI usage span attributes and preserves the unavailable reason", () => {
    const attributes = modelUsageSpanAttributes({
      status: "reported",
      value: {
        inputTokens: 100,
        outputTokens: 20,
        totalTokens: 120,
        cacheReadInputTokens: 70,
        cacheWriteInputTokens: 10,
        reasoningOutputTokens: 15,
      },
    })
    expect(attributes).toEqual({
      "gen_ai.usage.input_tokens": 100,
      "gen_ai.usage.output_tokens": 20,
      "gen_ai.usage.cache_read.input_tokens": 70,
      "gen_ai.usage.cache_write.input_tokens": 10,
      "gen_ai.usage.reasoning.output_tokens": 15,
    })
    expect(attributes).not.toHaveProperty("gen_ai.usage.cache_creation.input_tokens")
    expect(modelUsageSpanAttributes({ status: "unavailable", reason: "missing_usage" })).toEqual({
      "luna.gen_ai.usage.status": "unavailable",
      "luna.gen_ai.usage.unavailable_reason": "missing_usage",
    })
    expect(modelUsageSpanAttributes({ status: "unavailable", reason: "request_failed" })).toEqual({
      "luna.gen_ai.usage.status": "unavailable",
      "luna.gen_ai.usage.unavailable_reason": "request_failed",
    })
  })

  it.each([
    ["missing", undefined, "missing_usage"],
    ["null", null, "missing_usage"],
    ["empty", {}, "invalid_usage"],
    ["negative", { prompt_tokens: -1, completion_tokens: 1, total_tokens: 0 }, "invalid_usage"],
    ["fractional", { prompt_tokens: 1.5, completion_tokens: 1, total_tokens: 2.5 }, "invalid_usage"],
    ["unsafe", { prompt_tokens: Number.MAX_SAFE_INTEGER + 1, completion_tokens: 0, total_tokens: Number.MAX_SAFE_INTEGER + 1 }, "invalid_usage"],
  ])("rejects %s usage by throwing ai.provider_usage_unavailable", async (_name, usage, reason) => {
    expect(parseUsage(usage, "missing_usage")).toEqual({ status: "unavailable", reason })
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      id: "chatcmpl_invalid", model: "model-a", choices: [{ message: { content: "done" } }], ...(usage === undefined ? {} : { usage }),
    }), { status: 200 })))

    await expect(new OpenAIChatCompletionsProvider(options).complete(request))
      .rejects.toMatchObject({ message: "ai.provider_usage_unavailable" })
  })

  it("throws ai.provider_usage_unavailable when a stream ends without usage", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => streamResponse([
      JSON.stringify({ id: "chatcmpl_4", model: "model-a", choices: [{ delta: { content: "done" } }] }),
      "[DONE]",
    ])))
    await expect(collect(new OpenAIChatCompletionsProvider(options)))
      .rejects.toMatchObject({ message: "ai.provider_usage_unavailable" })
  })

  it("preserves valid reported usage when a gateway emits a trailing usage:null chunk and tolerates nullish fields", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => streamResponse([
      JSON.stringify({
        id: null, model: null, service_tier: null, system_fingerprint: null,
        choices: [{ delta: { content: "done", tool_calls: null }, finish_reason: "stop" }],
        usage: null,
      }),
      JSON.stringify({
        id: "chatcmpl_valid", model: "model-a", choices: null,
        usage: { prompt_tokens: 14, completion_tokens: 6, total_tokens: 20 },
      }),
      JSON.stringify({ id: "chatcmpl_valid", model: "model-a", choices: [], usage: null }),
      "[DONE]",
    ])))
    const events = await collect(new OpenAIChatCompletionsProvider(options))
    expect(events.at(-1)).toMatchObject({
      type: "completed",
      usage: { status: "reported", value: { inputTokens: 14, outputTokens: 6, totalTokens: 20 } },
    })
  })

  it("normalizes Claude-proxy unmerged cached tokens and Gemini-proxy thinking total_tokens without false errors", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      id: "claude_proxy", model: "claude-sonnet", choices: [{ message: { content: "ok" } }],
      usage: {
        prompt_tokens: 50,
        completion_tokens: 30,
        total_tokens: 80,
        prompt_tokens_details: { cached_tokens: 200, cache_write_tokens: 40 },
      },
    }), { status: 200 })))
    const claudeResult = await new OpenAIChatCompletionsProvider(options).complete(request)
    expect(claudeResult.usage).toEqual({
      status: "reported",
      value: {
        inputTokens: 290,
        outputTokens: 30,
        totalTokens: 320,
        cacheReadInputTokens: 200,
        cacheWriteInputTokens: 40,
      },
    })

    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      id: "gemini_proxy", model: "gemini-2.5-pro", choices: [{ message: { content: "ok" } }],
      usage: {
        prompt_tokens: 100,
        completion_tokens: 40,
        total_tokens: 220,
        completion_tokens_details: { reasoning_tokens: 80 },
      },
    }), { status: 200 })))
    const geminiResult = await new OpenAIChatCompletionsProvider(options).complete(request)
    expect(geminiResult.usage).toEqual({
      status: "reported",
      value: {
        inputTokens: 100,
        outputTokens: 40,
        totalTokens: 220,
        reasoningOutputTokens: 80,
      },
    })
  })

  it("maps top-level SSE error frames without choices before chunk schema validation", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => streamResponse([
      JSON.stringify({ error: { code: "rate_limit_exceeded", type: "rate_limit_error", message: "too many requests" } }),
      "[DONE]",
    ])))
    await expect(collect(new OpenAIChatCompletionsProvider(options)))
      .rejects.toMatchObject({ message: "ai.provider_rate_limited" })
  })

  it("parses the final SSE frame even without a trailing blank line", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => streamResponse([
      JSON.stringify({ id: "chatcmpl_5", model: "model-a", choices: [], usage: { prompt_tokens: 4, completion_tokens: 2, total_tokens: 6 } }),
    ], false)))
    expect((await collect(new OpenAIChatCompletionsProvider(options))).at(-1)).toMatchObject({
      usage: { status: "reported", value: { inputTokens: 4, outputTokens: 2, totalTokens: 6 } },
    })
  })

  it("maps a structured context-length error without message matching", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      error: { code: "context_length_exceeded", type: "invalid_request_error", param: "messages", message: "arbitrary text" },
    }), { status: 400, headers: { "x-request-id": "req_context" } })))

    let error: unknown
    try { await new OpenAIChatCompletionsProvider(options).complete(request) }
    catch (caught) { error = caught }
    expect(error).toBeInstanceOf(ProviderRequestError)
    expect(error).toMatchObject({ message: "ai.provider_context_length_exceeded", options: {
      providerCode: "context_length_exceeded", providerType: "invalid_request_error", providerParam: "messages", providerRequestId: "req_context", requestOutcome: "rejected",
    } })
  })

  it("does not send or interpret vendor-private fields in the official provider", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({
      id: "chatcmpl_6", model: "model-a", choices: [{ message: { content: "done", reasoning_content: "private" } }],
      usage: { prompt_tokens: 3, completion_tokens: 1, total_tokens: 4, prompt_cache_hit_tokens: 3 },
    }), { status: 200 }))
    vi.stubGlobal("fetch", fetchMock)
    const result = await new OpenAIChatCompletionsProvider(options).complete({
      ...request,
      messages: [
        { role: "user", content: "hello" },
        { role: "assistant", content: "working", reasoning: "private chain of thought" },
      ],
    })

    const body = requestBody(fetchMock)
    const messages = body.messages as Array<Record<string, unknown>>
    expect(JSON.stringify(body)).not.toContain("reasoning_content")
    expect(messages.every(message => !("reasoning" in message))).toBe(true)
    expect(result).not.toHaveProperty("reasoningSummary")
    expect(result.usage).toEqual({ status: "reported", value: { inputTokens: 3, outputTokens: 1, totalTokens: 4 } })
  })
  it("retries transient 502 and 429 before first chunk on complete and stream, respecting Retry-After, and never retries after first delta", async () => {
    let completeAttempts = 0
    vi.stubGlobal("fetch", vi.fn(async () => {
      completeAttempts += 1
      if (completeAttempts === 1) {
        return new Response(JSON.stringify({ error: { message: "bad gateway" } }), { status: 502 })
      }
      if (completeAttempts === 2) {
        return new Response(JSON.stringify({ error: { message: "rate limited" } }), {
          status: 429,
          headers: { "retry-after-ms": "1" },
        })
      }
      return new Response(JSON.stringify({
        id: "resp_retry_ok",
        model: "gpt-4o",
        choices: [{ message: { content: "recovered" }, finish_reason: "stop" }],
        usage: { prompt_tokens: 4, completion_tokens: 2, total_tokens: 6 },
      }), { status: 200 })
    }))

    const retriedComplete = await new OpenAIChatCompletionsProvider({
      ...options,
      maxRequestRetries: 2,
    }).complete(request)
    expect(completeAttempts).toBe(3)
    expect(retriedComplete.text).toBe("recovered")

    // Stream retries transient 502 before first delta
    let streamPreDeltaAttempts = 0
    vi.stubGlobal("fetch", vi.fn(async () => {
      streamPreDeltaAttempts += 1
      if (streamPreDeltaAttempts === 1) {
        return new Response(JSON.stringify({ error: { message: "bad gateway" } }), {
          status: 502,
          headers: { "retry-after-ms": "1" },
        })
      }
      return streamResponse([
        JSON.stringify({ id: "s_ok", model: "gpt-4o", choices: [{ delta: { content: "hello" } }] }),
        JSON.stringify({ id: "s_ok", model: "gpt-4o", choices: [], usage: { prompt_tokens: 3, completion_tokens: 1, total_tokens: 4 } }),
        "[DONE]",
      ])
    }))
    const streamOkEvents = await collect(new OpenAIChatCompletionsProvider({ ...options, maxRequestRetries: 2 }))
    expect(streamPreDeltaAttempts).toBe(2)
    expect(streamOkEvents.find(event => event.type === "message_delta")).toMatchObject({ delta: "hello" })

    // Stream MUST NOT retry after yielding the first delta when mid-stream breaks
    let midStreamAttempts = 0
    vi.stubGlobal("fetch", vi.fn(async () => {
      midStreamAttempts += 1
      const encoder = new TextEncoder()
      let step = 0
      const brokenBody = new ReadableStream<Uint8Array>({
        pull(controller) {
          step += 1
          if (step === 1) {
            controller.enqueue(encoder.encode(`data: ${JSON.stringify({
              id: "s_mid",
              model: "gpt-4o",
              choices: [{ delta: { content: "partial" } }],
            })}\n\n`))
            return
          }
          controller.error(new Error("socket reset mid-stream"))
        },
      })
      return new Response(brokenBody, { status: 200, headers: { "content-type": "text/event-stream" } })
    }))
    await expect(collect(new OpenAIChatCompletionsProvider({ ...options, maxRequestRetries: 3 })))
      .rejects.toThrow("ai.provider_unavailable")
    expect(midStreamAttempts).toBe(1)
  })

  it("never retries HTTP 429 insufficient_quota while retrying HTTP 429 rate_limit_exceeded and pre-first-delta SSE rate_limit_exceeded", async () => {
    let quotaAttempts = 0
    vi.stubGlobal("fetch", vi.fn(async () => {
      quotaAttempts += 1
      return new Response(JSON.stringify({
        error: { code: "insufficient_quota", type: "insufficient_quota", message: "You exceeded your current quota" },
      }), { status: 429, headers: { "content-type": "application/json", "retry-after": "0" } })
    }))
    await expect(new OpenAIChatCompletionsProvider({ ...options, maxRequestRetries: 3 }).complete(request))
      .rejects.toThrow("ai.provider_quota_exhausted")
    expect(quotaAttempts).toBe(1)

    let rateLimitAttempts = 0
    vi.stubGlobal("fetch", vi.fn(async () => {
      rateLimitAttempts += 1
      if (rateLimitAttempts === 1) {
        return new Response(JSON.stringify({
          error: { code: "rate_limit_exceeded", message: "Too many requests" },
        }), { status: 429, headers: { "content-type": "application/json", "retry-after": "0" } })
      }
      return new Response(JSON.stringify({
        id: "resp_rl_ok", model: "gpt-test", choices: [{ message: { content: "ok" } }],
        usage: { prompt_tokens: 2, completion_tokens: 1, total_tokens: 3 },
      }), { status: 200, headers: { "content-type": "application/json" } })
    }))
    const recovered = await new OpenAIChatCompletionsProvider({ ...options, maxRequestRetries: 2 }).complete(request)
    expect(recovered.text).toBe("ok")
    expect(rateLimitAttempts).toBe(2)

    let sseRateLimitAttempts = 0
    vi.stubGlobal("fetch", vi.fn(async () => {
      sseRateLimitAttempts += 1
      if (sseRateLimitAttempts === 1) {
        return streamResponse([
          JSON.stringify({ error: { code: "rate_limit_exceeded", message: "Rate limit in SSE" } }),
        ])
      }
      return streamResponse([
        JSON.stringify({ id: "sse_rl_ok", model: "gpt-test", choices: [{ delta: { content: "stream-ok" }, finish_reason: "stop" }] }),
        JSON.stringify({ id: "sse_rl_ok", model: "gpt-test", choices: [], usage: { prompt_tokens: 2, completion_tokens: 1, total_tokens: 3 } }),
        "[DONE]",
      ])
    }))
    const streamEvents = await collect(new OpenAIChatCompletionsProvider({ ...options, maxRequestRetries: 2 }))
    expect(sseRateLimitAttempts).toBe(2)
    expect(streamEvents.find(event => event.type === "message_delta")).toMatchObject({ delta: "stream-ok" })
  })
})

describe("DeepSeekChatCompletionsProvider adapter", () => {
  it("omits reasoning_content on historical turns (< lastUserIndex) and preserves reasoning_content on current turn tool chain (> lastUserIndex)", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({
      id: "deepseek_1", model: "deepseek-chat", choices: [{ message: { content: "done" } }],
      usage: { prompt_tokens: 2, completion_tokens: 1, total_tokens: 3 },
    }), { status: 200 }))
    vi.stubGlobal("fetch", fetchMock)
    await new DeepSeekChatCompletionsProvider({ ...options, baseUrl: "https://api.deepseek.com/v1", model: "deepseek-chat" }).complete({
      ...request,
      messages: [
        { role: "system", content: "system prompt" },
        { role: "user", content: "turn 1 question" },
        { role: "assistant", content: "turn 1 tool step", toolCalls: [{ id: "call_hist", operationId: "getBuild", arguments: {} }], reasoning: "历史轮思考" },
        { role: "tool", toolCallId: "call_hist", content: "{}" },
        { role: "assistant", content: "turn 1 final answer" },
        { role: "user", content: "turn 2 question" },
        { role: "assistant", content: "working", toolCalls: [{ id: "call_1", operationId: "getBuild", arguments: {} }], reasoning: "先读取构建记录" },
        { role: "tool", toolCallId: "call_1", content: "{}" },
        { role: "assistant", content: "", toolCalls: [{ id: "call_2", operationId: "listProjects", arguments: {} }] },
        { role: "tool", toolCallId: "call_2", content: "{}" },
      ],
    })
    const body = requestBody(fetchMock)
    expect(body).toMatchObject({ max_tokens: 100 })
    expect(body).not.toHaveProperty("max_completion_tokens")
    const assistantMessages = (body.messages as Array<Record<string, unknown>>).filter(message => message.role === "assistant")
    // Historical turn assistant messages (< lastUserIndex) must omit reasoning_content completely
    expect(assistantMessages[0]).not.toHaveProperty("reasoning_content")
    expect(assistantMessages[1]).not.toHaveProperty("reasoning_content")
    expect(JSON.stringify(body)).not.toContain("上一轮推理内容")
    // Current turn assistant messages (> lastUserIndex) carry real reasoning or single-space fallback so omitempty gateways never strip it
    expect(assistantMessages[2]).toMatchObject({ reasoning_content: "先读取构建记录" })
    expect(assistantMessages[3]).toMatchObject({ reasoning_content: " " })
  })

  it("maps non-streaming prompt_cache_hit_tokens into the normalized cache-read field", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      id: "deepseek_usage", model: "deepseek-chat", choices: [{ message: { content: "done" } }],
      usage: { prompt_tokens: 10, completion_tokens: 2, total_tokens: 12, prompt_cache_hit_tokens: 7 },
    }), { status: 200 })))

    const result = await new DeepSeekChatCompletionsProvider({ ...options, baseUrl: "https://api.deepseek.com/v1", model: "deepseek-chat" }).complete(request)

    expect(result.usage).toEqual({
      status: "reported",
      value: { inputTokens: 10, outputTokens: 2, totalTokens: 12, cacheReadInputTokens: 7 },
    })
  })

  it("maps streaming prompt_cache_hit_tokens into the normalized cache-read field", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => streamResponse([
      JSON.stringify({ id: "deepseek_stream", model: "deepseek-chat", choices: [{ delta: { content: "done" } }] }),
      JSON.stringify({
        id: "deepseek_stream", model: "deepseek-chat", choices: [],
        usage: { prompt_tokens: 9, completion_tokens: 3, total_tokens: 12, prompt_cache_hit_tokens: 6 },
      }),
      "[DONE]",
    ])))

    const events = await collect(new DeepSeekChatCompletionsProvider({ ...options, baseUrl: "https://api.deepseek.com/v1", model: "deepseek-chat" }))

    expect(events.at(-1)).toMatchObject({
      type: "completed",
      usage: { status: "reported", value: { inputTokens: 9, outputTokens: 3, totalTokens: 12, cacheReadInputTokens: 6 } },
    })
  })
})

function requestBody(fetchMock: ReturnType<typeof vi.fn<typeof fetch>>): Record<string, unknown> {
  const raw = fetchMock.mock.calls[0]?.[1]?.body
  if (typeof raw !== "string") throw new TypeError("expected JSON request body")
  return JSON.parse(raw) as Record<string, unknown>
}
