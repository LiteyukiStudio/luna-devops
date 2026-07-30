import { afterEach, describe, expect, it, vi } from "vitest"
import { OpenAICompatibleProvider } from "../src/provider/openai-compatible.js"

afterEach(() => vi.unstubAllGlobals())

describe("OpenAICompatibleProvider streaming", () => {
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
      { type: "completed", usage: { inputTokens: 12, outputTokens: 8 }, toolCalls: [{ operationId: "getBuild", arguments: { id: "build_a" } }] },
    ])
    const requestBody = fetchMock.mock.calls[0]?.[1]?.body
    expect(JSON.parse(typeof requestBody === "string" ? requestBody : "{}")).toMatchObject({
      stream: true,
      tool_choice: "auto",
      tools: [{ type: "function", function: { name: "getBuild", description: "Get a build" } }],
    })
  })

  it("maps upstream quota failures to a stable public error code", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(
      JSON.stringify({ error: { message: "upstream detail must stay private" } }),
      { status: 402, headers: { "content-type": "application/json" } },
    )))
    const provider = new OpenAICompatibleProvider({ baseUrl: "https://provider.example/v1", apiKey: "secret", model: "model-a", timeoutMs: 5000 })

    await expect(provider.complete({
      messages: [{ role: "user", content: "hello" }],
      maxOutputTokens: 100,
    })).rejects.toThrow("ai.provider_quota_exhausted")
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
    const provider = new OpenAICompatibleProvider({ baseUrl: "https://provider.example/v1", apiKey: "secret", model: "model-a", timeoutMs: 5000 })

    const consume = async () => {
      await provider.stream({
        messages: [{ role: "user", content: "hello" }],
        maxOutputTokens: 100,
      })[Symbol.asyncIterator]().next()
    }
    await expect(consume()).rejects.toThrow("ai.provider_quota_exhausted")
  })
})
