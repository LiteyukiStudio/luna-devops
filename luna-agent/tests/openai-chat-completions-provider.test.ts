import { afterEach, describe, expect, it, vi } from "vitest"
import { DeepSeekChatCompletionsProvider } from "../src/provider/deepseek-chat-completions.js"
import { modelUsageSpanAttributes, OpenAIChatCompletionsProvider } from "../src/provider/openai-chat-completions.js"
import { ProviderRequestError } from "../src/provider/provider-error.js"

const options = { baseUrl: "https://provider.example/v1", apiKey: "secret", channelAffinityEnabled: true, model: "model-a", timeoutMs: 5_000 }
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
  })

  it.each([
    ["missing", undefined, "missing_usage"],
    ["empty", {}, "invalid_usage"],
    ["mismatched total", { prompt_tokens: 2, completion_tokens: 3, total_tokens: 6 }, "invalid_usage"],
    ["negative", { prompt_tokens: -1, completion_tokens: 1, total_tokens: 0 }, "invalid_usage"],
    ["fractional", { prompt_tokens: 1.5, completion_tokens: 1, total_tokens: 2.5 }, "invalid_usage"],
    ["unsafe", { prompt_tokens: Number.MAX_SAFE_INTEGER + 1, completion_tokens: 0, total_tokens: Number.MAX_SAFE_INTEGER + 1 }, "invalid_usage"],
    ["cached overflow", { prompt_tokens: 2, completion_tokens: 1, total_tokens: 3, prompt_tokens_details: { cached_tokens: 3 } }, "invalid_usage"],
    ["reasoning overflow", { prompt_tokens: 2, completion_tokens: 1, total_tokens: 3, completion_tokens_details: { reasoning_tokens: 2 } }, "invalid_usage"],
  ])("marks %s usage unavailable", async (_name, usage, reason) => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      id: "chatcmpl_invalid", model: "model-a", choices: [{ message: { content: "done" } }], ...(usage === undefined ? {} : { usage }),
    }), { status: 200 })))

    const result = await new OpenAIChatCompletionsProvider(options).complete(request)
    expect(result.usage).toEqual({ status: "unavailable", reason })
  })

  it("returns unavailable when a stream ends without usage", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => streamResponse([
      JSON.stringify({ id: "chatcmpl_4", model: "model-a", choices: [{ delta: { content: "done" } }] }),
      "[DONE]",
    ])))
    expect((await collect(new OpenAIChatCompletionsProvider(options))).at(-1)).toMatchObject({
      type: "completed", usage: { status: "unavailable", reason: "stream_ended_without_usage" },
    })
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
    const result = await new OpenAIChatCompletionsProvider(options).complete({ ...request, thinking: { type: "enabled" } })

    const body = requestBody(fetchMock)
    expect(body).not.toHaveProperty("thinking")
    expect(JSON.stringify(body)).not.toContain("reasoning_content")
    expect(result).not.toHaveProperty("reasoningSummary")
    expect(result.usage).toEqual({ status: "reported", value: { inputTokens: 3, outputTokens: 1, totalTokens: 4 } })
  })
})

describe("DeepSeekChatCompletionsProvider adapter", () => {
  it("contains the explicit max_tokens and reasoning_content compatibility", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({
      id: "deepseek_1", model: "deepseek-chat", choices: [{ message: { content: "done" } }],
      usage: { prompt_tokens: 2, completion_tokens: 1, total_tokens: 3 },
    }), { status: 200 }))
    vi.stubGlobal("fetch", fetchMock)
    await new DeepSeekChatCompletionsProvider({ ...options, baseUrl: "https://api.deepseek.com/v1", model: "deepseek-chat" }).complete({
      ...request,
      messages: [{ role: "assistant", content: "working", toolCalls: [{ id: "call_1", operationId: "getBuild", arguments: {} }] }],
      thinking: { type: "enabled" },
    })
    const body = requestBody(fetchMock)
    expect(body).toMatchObject({ max_tokens: 100, thinking: { type: "enabled" } })
    expect(body).not.toHaveProperty("max_completion_tokens")
    expect(JSON.stringify(body)).toContain("reasoning_content")
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
