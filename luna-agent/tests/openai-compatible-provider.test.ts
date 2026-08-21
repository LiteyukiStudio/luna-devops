import { afterEach, describe, expect, it, vi } from "vitest"
import { OpenAICompatibleProvider } from "../src/provider/openai-compatible.js"

afterEach(() => vi.unstubAllGlobals())

describe("OpenAICompatibleProvider streaming", () => {
  it("normalizes reasoning_content from compatible reasoning providers", async () => {
    const frames = `data: ${JSON.stringify({ choices: [{ delta: { reasoning_content: "分析中" } }] })}\n\ndata: [DONE]\n\n`
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(new TextEncoder().encode(frames))
        controller.close()
      },
    })
    vi.stubGlobal("fetch", vi.fn(async () => new Response(body, {
      status: 200,
      headers: { "content-type": "text/event-stream" },
    })))
    const provider = new OpenAICompatibleProvider({ baseUrl: "https://provider.example/v1", apiKey: "secret", model: "model-a", timeoutMs: 5000 })

    const events = []
    for await (const event of provider.stream({
      messages: [{ role: "user", content: "hello" }],
      maxOutputTokens: 100,
    }))
      events.push(event)

    expect(events).toEqual([
      { type: "reasoning_summary_delta", delta: "分析中" },
      { type: "completed", usage: { inputTokens: 0, outputTokens: 0, reported: false } },
    ])
  })

  it("accepts a successful reasoning-only health response", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({
      choices: [{ message: { content: "", reasoning_content: "正在判断" } }],
      usage: { prompt_tokens: 8, completion_tokens: 4 },
    }), { status: 200, headers: { "content-type": "application/json", "x-request-id": "provider-health" } }))
    vi.stubGlobal("fetch", fetchMock)
    const provider = new OpenAICompatibleProvider({ baseUrl: "https://provider.example/v1", apiKey: "secret", model: "model-a", timeoutMs: 5000 })

    await expect(provider.health()).resolves.toEqual({ ok: true, requestId: "provider-health" })
    const requestBody = fetchMock.mock.calls[0]?.[1]?.body
    expect(JSON.parse(typeof requestBody === "string" ? requestBody : "{}")).toMatchObject({ max_tokens: 32 })

    await expect(provider.complete({
      messages: [{ role: "user", content: "hello" }],
      maxOutputTokens: 100,
    })).resolves.toMatchObject({ reasoningSummary: "正在判断" })
  })

  it("normalizes reasoning, content, usage, and fragmented tool calls from SSE", async () => {
    const frames = [
      { choices: [{ delta: { reasoning_summary: "检查上下文" } }] },
      { choices: [{ delta: { content: "正在" } }] },
      { choices: [{ delta: { content: "诊断" } }] },
      { choices: [{ delta: { tool_calls: [{ index: 0, function: { name: "getBuild", arguments: "{\"id\":" } }] } }] },
      { choices: [{ delta: { tool_calls: [{ index: 0, function: { arguments: "\"build_a\"}" } }] } }], usage: { prompt_tokens: 12, completion_tokens: 8 } },
    ].map(frame => `data: ${JSON.stringify(frame)}\n\n`).join("") + "data: [DONE]\n\n"
    const bytes = new TextEncoder().encode(frames)
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(bytes.slice(0, 37))
        controller.enqueue(bytes.slice(37))
        controller.close()
      },
    })
    const fetchMock = vi.fn<typeof fetch>(async () => new Response(body, { status: 200, headers: { "content-type": "text/event-stream" } }))
    vi.stubGlobal("fetch", fetchMock)
    const provider = new OpenAICompatibleProvider({ baseUrl: "https://provider.example/v1", apiKey: "secret", model: "model-a", timeoutMs: 5000 })
    const events = []
    for await (const event of provider.stream({
      messages: [{ role: "user", content: "hello" }],
      tools: [{ operationId: "getBuild", description: "Get a build", inputSchema: { type: "object", properties: { id: { type: "string" } }, required: ["id"] } }],
      maxOutputTokens: 100,
    }))
      events.push(event)

    expect(events).toEqual([
      { type: "reasoning_summary_delta", delta: "检查上下文" },
      { type: "message_delta", delta: "正在" },
      { type: "message_delta", delta: "诊断" },
      { type: "tool_call_delta" },
      { type: "completed", usage: { inputTokens: 12, outputTokens: 8, reported: true }, toolCalls: [{ operationId: "getBuild", arguments: { id: "build_a" } }] },
    ])
    const requestBody = fetchMock.mock.calls[0]?.[1]?.body
    expect(JSON.parse(typeof requestBody === "string" ? requestBody : "{}")).toMatchObject({
      stream: true,
      tool_choice: "auto",
      tools: [{ type: "function", function: { name: "getBuild", description: "Get a build" } }],
    })
  })

  it("normalizes DeepSeek cache-hit usage from streaming responses", async () => {
    const frames = `data: ${JSON.stringify({
      choices: [{ delta: { content: "完成" } }],
      usage: { prompt_tokens: 100, completion_tokens: 5, prompt_cache_hit_tokens: 80, prompt_cache_miss_tokens: 20 },
    })}\n\ndata: [DONE]\n\n`
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(new TextEncoder().encode(frames))
        controller.close()
      },
    })
    vi.stubGlobal("fetch", vi.fn(async () => new Response(body, { status: 200, headers: { "content-type": "text/event-stream" } })))
    const provider = new OpenAICompatibleProvider({ baseUrl: "https://api.deepseek.com/v1", apiKey: "secret", model: "deepseek-chat", timeoutMs: 5000 })
    const events = []

    for await (const event of provider.stream({ messages: [{ role: "user", content: "hello" }], maxOutputTokens: 100 }))
      events.push(event)

    expect(events.at(-1)).toEqual({
      type: "completed",
      usage: { inputTokens: 100, outputTokens: 5, cachedInputTokens: 80, reported: true },
    })
  })

  it("normalizes DeepSeek cache-hit usage from non-streaming responses", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      choices: [{ message: { content: "完成" } }],
      usage: { prompt_tokens: 120, completion_tokens: 6, prompt_cache_hit_tokens: 90, prompt_cache_miss_tokens: 30 },
    }), { status: 200, headers: { "content-type": "application/json" } })))
    const provider = new OpenAICompatibleProvider({ baseUrl: "https://api.deepseek.com/v1", apiKey: "secret", model: "deepseek-chat", timeoutMs: 5000 })

    const result = await provider.complete({ messages: [{ role: "user", content: "hello" }], maxOutputTokens: 100 })

    expect(result.usage).toEqual({ inputTokens: 120, outputTokens: 6, cachedInputTokens: 90, reported: true })
  })

  it("maps upstream quota failures to a stable public error code", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(
      JSON.stringify({ error: { message: "upstream detail must stay private" } }),
      { status: 402, headers: { "content-type": "application/json" } },
    )))
    const provider = new OpenAICompatibleProvider({ baseUrl: "https://provider.example/v1", apiKey: "secret", model: "model-a", timeoutMs: 5000, maxRetries: 0 })

    await expect(provider.complete({
      messages: [{ role: "user", content: "hello" }],
      maxOutputTokens: 100,
    })).rejects.toThrow("ai.provider_quota_exhausted")
  })

  it("maps provider transport and malformed stream failures to stable error codes", async () => {
    const provider = new OpenAICompatibleProvider({ baseUrl: "https://provider.example/v1", apiKey: "secret", model: "model-a", timeoutMs: 5000, maxRetries: 0 })
    vi.stubGlobal("fetch", vi.fn(async () => {
      throw new TypeError("socket closed with upstream detail")
    }))
    await expect(provider.complete({
      messages: [{ role: "user", content: "hello" }],
      maxOutputTokens: 100,
    })).rejects.toThrow("ai.provider_unavailable")

    vi.stubGlobal("fetch", vi.fn(async () => new Response("data: {broken-json}\n\n", {
      status: 200,
      headers: { "content-type": "text/event-stream" },
    })))
    const consume = async () => {
      for await (const event of provider.stream({
        messages: [{ role: "user", content: "hello" }],
        maxOutputTokens: 100,
      })) {
        void event
      }
    }
    await expect(consume()).rejects.toThrow("ai.provider_stream_failed")
  })

  it("can require one named tool for structured UI prediction", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({
      choices: [{ message: { tool_calls: [{ function: { name: "create_options", arguments: "{\"title\":\"Next\",\"options\":[]}" } }] } }],
      usage: { prompt_tokens: 4, completion_tokens: 2 },
    }), { status: 200, headers: { "content-type": "application/json" } }))
    vi.stubGlobal("fetch", fetchMock)
    const provider = new OpenAICompatibleProvider({ baseUrl: "https://provider.example/v1", apiKey: "secret", model: "model-a", timeoutMs: 5000 })

    await provider.complete({
      messages: [{ role: "user", content: "predict" }],
      tools: [{ operationId: "create_options", description: "Predict", inputSchema: { type: "object" } }],
      toolChoice: { operationId: "create_options" },
      maxOutputTokens: 100,
    })

    const requestBody = fetchMock.mock.calls[0]?.[1]?.body
    expect(JSON.parse(typeof requestBody === "string" ? requestBody : "{}")).toMatchObject({
      tool_choice: { type: "function", function: { name: "create_options" } },
    })
  })

  it("repairs only redundant trailing object closures from compatible providers", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      choices: [{ message: { tool_calls: [{ function: { name: "create_interaction_cards", arguments: "{\"schemaVersion\":1}}" } }] } }],
      usage: { prompt_tokens: 4, completion_tokens: 2 },
    }), { status: 200, headers: { "content-type": "application/json" } })))
    const provider = new OpenAICompatibleProvider({ baseUrl: "https://provider.example/v1", apiKey: "secret", model: "model-a", timeoutMs: 5000 })

    const result = await provider.complete({
      messages: [{ role: "user", content: "create a card" }],
      maxOutputTokens: 100,
    })

    expect(result.toolCalls).toEqual([{
      operationId: "create_interaction_cards",
      arguments: { schemaVersion: 1 },
    }])
  })

  it("returns malformed tool arguments to the executor as a recoverable validation result", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      choices: [{ message: { tool_calls: [{ id: "card_call", function: { name: "create_interaction_cards", arguments: "{\"schemaVersion\":1" } }] } }],
      usage: { prompt_tokens: 4, completion_tokens: 2 },
    }), { status: 200, headers: { "content-type": "application/json" } })))
    const provider = new OpenAICompatibleProvider({ baseUrl: "https://provider.example/v1", apiKey: "secret", model: "model-a", timeoutMs: 5000 })

    const result = await provider.complete({
      messages: [{ role: "user", content: "create a card" }],
      maxOutputTokens: 100,
    })

    expect(result.toolCalls).toEqual([{
      id: "card_call",
      operationId: "create_interaction_cards",
      arguments: {},
      argumentError: {
        code: "invalid_json",
        message: "工具参数不是完整的 JSON 对象。",
      },
    }])
  })

  it("serializes assistant tool calls and correlated tool results for the next model step", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({
      choices: [{ message: { content: "完成" } }],
      usage: { prompt_tokens: 8, completion_tokens: 2 },
    }), { status: 200, headers: { "content-type": "application/json" } }))
    vi.stubGlobal("fetch", fetchMock)
    const provider = new OpenAICompatibleProvider({ baseUrl: "https://provider.example/v1", apiKey: "secret", model: "model-a", timeoutMs: 5000 })

    await provider.complete({
      messages: [
        { role: "user", content: "查询项目空间" },
        {
          role: "assistant",
          content: "正在查询。",
          toolCalls: [{ id: "call_projects", operationId: "listProjects", arguments: {} }],
        },
        { role: "tool", toolCallId: "call_projects", content: "{\"items\":[]}" },
      ],
      tools: [{ operationId: "listProjects", description: "查询项目空间", inputSchema: { type: "object" } }],
      maxOutputTokens: 100,
    })

    const requestBody = fetchMock.mock.calls[0]?.[1]?.body
    const parsedRequest = JSON.parse(typeof requestBody === "string" ? requestBody : "{}") as { messages?: unknown }
    expect(parsedRequest.messages).toEqual([
      { role: "user", content: "查询项目空间" },
      {
        role: "assistant",
        content: "正在查询。",
        tool_calls: [{
          id: "call_projects",
          type: "function",
          function: { name: "listProjects", arguments: "{}" },
        }],
      },
      { role: "tool", tool_call_id: "call_projects", content: "{\"items\":[]}" },
    ])
  })

  it("replays compatible reasoning content after a thinking provider rejects a resumed tool call", async () => {
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        error: {
          code: "invalid_request_error",
          type: "invalid_request_error",
          message: "The `reasoning_content` in the thinking mode must be passed back to the API.",
        },
      }), { status: 400, headers: { "content-type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        choices: [{ message: { content: "完成" }, finish_reason: "stop" }],
        usage: { prompt_tokens: 8, completion_tokens: 2 },
      }), { status: 200, headers: { "content-type": "application/json" } }))
    vi.stubGlobal("fetch", fetchMock)
    const provider = new OpenAICompatibleProvider({ baseUrl: "https://provider.example/v1", apiKey: "secret", model: "model-a", timeoutMs: 5000, maxRetries: 0 })

    await expect(provider.complete({
      messages: [
        { role: "user", content: "查询项目空间" },
        { role: "assistant", content: "正在查询。", toolCalls: [{ id: "call_projects", operationId: "listProjects", arguments: {} }] },
        { role: "tool", toolCallId: "call_projects", content: "{\"items\":[]}" },
      ],
      tools: [{ operationId: "listProjects", description: "查询项目空间", inputSchema: { type: "object" } }],
      maxOutputTokens: 100,
    })).resolves.toMatchObject({ text: "完成" })

    expect(fetchMock).toHaveBeenCalledTimes(2)
    const firstRawBody = fetchMock.mock.calls[0]?.[1]?.body
    const replayRawBody = fetchMock.mock.calls[1]?.[1]?.body
    expect(typeof firstRawBody).toBe("string")
    expect(typeof replayRawBody).toBe("string")
    if (typeof firstRawBody !== "string" || typeof replayRawBody !== "string") throw new Error("expected JSON request bodies")
    const firstBody = JSON.parse(firstRawBody) as { messages: Array<Record<string, unknown>> }
    const replayBody = JSON.parse(replayRawBody) as { messages: Array<Record<string, unknown>> }
    expect(firstBody.messages[1]).not.toHaveProperty("reasoning_content")
    expect(replayBody.messages[1]).toMatchObject({
      role: "assistant",
      content: "正在查询。",
      reasoning_content: "正在查询。",
    })
  })

  it("retries a resumed tool stream once with reasoning replay compatibility", async () => {
    const frames = `data: ${JSON.stringify({ choices: [{ delta: { content: "完成" }, finish_reason: "stop" }], usage: { prompt_tokens: 8, completion_tokens: 2 } })}\n\ndata: [DONE]\n\n`
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        error: { message: "The `reasoning_content` in the thinking mode must be passed back to the API." },
      }), { status: 400, headers: { "content-type": "application/json" } }))
      .mockResolvedValueOnce(new Response(frames, { status: 200, headers: { "content-type": "text/event-stream" } }))
    vi.stubGlobal("fetch", fetchMock)
    const provider = new OpenAICompatibleProvider({ baseUrl: "https://provider.example/v1", apiKey: "secret", model: "model-a", timeoutMs: 5000, maxRetries: 0 })

    const events = []
    for await (const event of provider.stream({
      messages: [
        { role: "assistant", content: "", toolCalls: [{ id: "call_projects", operationId: "listProjects", arguments: {} }] },
        { role: "tool", toolCallId: "call_projects", content: "{\"items\":[]}" },
      ],
      tools: [{ operationId: "listProjects", description: "查询项目空间", inputSchema: { type: "object" } }],
      maxOutputTokens: 100,
    })) events.push(event)

    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(events).toEqual([
      { type: "message_delta", delta: "完成" },
      { type: "completed", usage: { inputTokens: 8, outputTokens: 2, reported: true }, finishReason: "stop" },
    ])
    const replayRawBody = fetchMock.mock.calls[1]?.[1]?.body
    expect(typeof replayRawBody).toBe("string")
    if (typeof replayRawBody !== "string") throw new Error("expected JSON request body")
    const replayBody = JSON.parse(replayRawBody) as { messages: Array<Record<string, unknown>> }
    expect(replayBody.messages[0]).toMatchObject({
      reasoning_content: "继续处理此前已完成的工具调用。",
    })
  })

  it("does not retry unrelated provider 400 responses as reasoning compatibility", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({
      error: { message: "tool schema is invalid" },
    }), { status: 400, headers: { "content-type": "application/json" } }))
    vi.stubGlobal("fetch", fetchMock)
    const provider = new OpenAICompatibleProvider({ baseUrl: "https://provider.example/v1", apiKey: "secret", model: "model-a", timeoutMs: 5000, maxRetries: 0 })

    await expect(provider.complete({
      messages: [{ role: "user", content: "hello" }],
      maxOutputTokens: 100,
    })).rejects.toThrow("ai.provider_request_failed")
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it("normalizes a quota error embedded in an otherwise successful SSE response", async () => {
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(new TextEncoder().encode(
          `data: ${JSON.stringify({ error: { message: "Insufficient Balance", code: "invalid_request_error" } })}\n\n`,
        ))
        controller.close()
      },
    })
    vi.stubGlobal("fetch", vi.fn(async () => new Response(body, {
      status: 200,
      headers: { "content-type": "text/event-stream" },
    })))
    const provider = new OpenAICompatibleProvider({ baseUrl: "https://provider.example/v1", apiKey: "secret", model: "model-a", timeoutMs: 5000, maxRetries: 0 })

    const consume = async () => {
      await provider.stream({
        messages: [{ role: "user", content: "hello" }],
        maxOutputTokens: 100,
      })[Symbol.asyncIterator]().next()
    }
    await expect(consume()).rejects.toThrow("ai.provider_quota_exhausted")
  })

  it("retries transient provider failures and honors Retry-After", async () => {
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response("busy", { status: 429, headers: { "retry-after": "0" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        choices: [{ message: { content: "恢复" } }],
        usage: { prompt_tokens: 2, completion_tokens: 1 },
      }), { status: 200, headers: { "content-type": "application/json" } }))
    vi.stubGlobal("fetch", fetchMock)
    const provider = new OpenAICompatibleProvider({ baseUrl: "https://provider.example/v1", apiKey: "secret", model: "model-a", timeoutMs: 5000, maxRetries: 5 })

    await expect(provider.complete({ messages: [{ role: "user", content: "hello" }], maxOutputTokens: 100 }))
      .resolves.toMatchObject({ text: "恢复" })
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it("stops after the configured five retries", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () => new Response("busy", {
      status: 429,
      headers: { "retry-after": "0" },
    }))
    vi.stubGlobal("fetch", fetchMock)
    const provider = new OpenAICompatibleProvider({
      baseUrl: "https://provider.example/v1",
      apiKey: "secret",
      model: "model-a",
      timeoutMs: 5000,
      maxRetries: 5,
    })

    await expect(provider.complete({ messages: [{ role: "user", content: "hello" }], maxOutputTokens: 100 }))
      .rejects.toThrow("ai.provider_rate_limited")
    expect(fetchMock).toHaveBeenCalledTimes(6)
  })

  it("does not replay a stream after visible output has started", async () => {
    let pullCount = 0
    const body = new ReadableStream<Uint8Array>({
      pull(controller) {
        pullCount += 1
        if (pullCount === 1) {
          controller.enqueue(new TextEncoder().encode(`data: ${JSON.stringify({ choices: [{ delta: { content: "部分" } }] })}\n\n`))
          return
        }
        controller.error(new Error("connection reset"))
      },
    })
    const fetchMock = vi.fn<typeof fetch>(async () => new Response(body, { status: 200 }))
    vi.stubGlobal("fetch", fetchMock)
    const provider = new OpenAICompatibleProvider({ baseUrl: "https://provider.example/v1", apiKey: "secret", model: "model-a", timeoutMs: 5000, maxRetries: 5 })
    const events: Array<{ type: string, delta?: string }> = []
    await expect(async () => {
      for await (const event of provider.stream({ messages: [{ role: "user", content: "hello" }], maxOutputTokens: 100 }))
        events.push(event)
    }).rejects.toThrow("ai.provider_unavailable")
    expect(events).toEqual([{ type: "message_delta", delta: "部分" }])
    expect(fetchMock).toHaveBeenCalledOnce()
  })
})
