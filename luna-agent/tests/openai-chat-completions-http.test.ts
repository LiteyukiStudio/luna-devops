import { createServer, type IncomingMessage, type ServerResponse } from "node:http"
import { afterAll, beforeAll, describe, expect, it, vi } from "vitest"
import type { ContextCompiler } from "../src/context/compiler.js"
import { ModelRuntime, type AssistantModelInput } from "../src/model-runtime.js"
import { OpenAIChatCompletionsProvider } from "../src/provider/openai-chat-completions.js"
import { ProviderRequestError } from "../src/provider/provider-error.js"
import { initializeTelemetry, shutdownTelemetry } from "../src/telemetry.js"

describe("OpenAI Chat Completions real HTTP contract", () => {
  const server = createServer((request, response) => {
    void handleRequest(request, response).catch(() => response.destroy())
  })
  let baseUrl = ""

  beforeAll(async () => {
    if (process.env.OTEL_SMOKE === "true") initializeTelemetry()
    await new Promise<void>((resolve, reject) => {
      server.once("error", reject)
      server.listen(0, "127.0.0.1", resolve)
    })
    const address = server.address()
    if (!address || typeof address === "string") throw new Error("test_server_address_invalid")
    baseUrl = `http://127.0.0.1:${address.port}/v1`
  })

  afterAll(async () => {
    await new Promise<void>((resolve, reject) => server.close(error => error ? reject(error) : resolve()))
    if (process.env.OTEL_SMOKE === "true") await shutdownTelemetry()
  })

  it("reads the final usage-only SSE chunk from /v1/chat/completions", async () => {
    const events = []
    for await (const event of provider(baseUrl).stream(modelRequest("success-stream"))) events.push(event)

    expect(events.at(-1)).toMatchObject({
      type: "completed",
      usage: {
        status: "reported",
        value: {
          inputTokens: 11,
          outputTokens: 4,
          totalTokens: 15,
          cacheReadInputTokens: 5,
          cacheWriteInputTokens: 1,
          reasoningOutputTokens: 2,
        },
      },
    })
  })

  it.each([
    ["missing-usage", { status: "unavailable", reason: "missing_usage" }],
    ["invalid-usage", { status: "unavailable", reason: "invalid_usage" }],
  ])("preserves %s as unavailable", async (content, usage) => {
    await expect(provider(baseUrl).complete(modelRequest(content))).resolves.toMatchObject({ usage })
  })

  it("marks an indeterminate connection outcome as unavailable instead of retrying", async () => {
    let error: unknown
    try { await provider(baseUrl).complete(modelRequest("unknown-outcome")) }
    catch (caught) { error = caught }

    expect(error).toBeInstanceOf(ProviderRequestError)
    expect(error).toMatchObject({ message: "ai.provider_unavailable", options: { requestOutcome: "unknown" } })
  })

  it("lets ModelRuntime compress and retry once after a structured context rejection", async () => {
    contextAttempts = 0
    const compile = vi.fn()
      .mockResolvedValueOnce(compiled())
      .mockResolvedValueOnce(compiled({ summarizedThroughTurnIndex: 0, sourceTurnCount: 1, trigger: "context_error" }))
    const events = []
    for await (const event of new ModelRuntime(
      provider(baseUrl),
      [],
      { compile, setOptions: vi.fn() } as unknown as ContextCompiler,
    ).stream(runtimeInput())) events.push(event)

    expect(contextAttempts).toBe(2)
    expect(compile).toHaveBeenCalledTimes(2)
    expect(events.map(event => event.type)).toEqual(["context.compacted", "message_delta", "completed"])
  })
})

let contextAttempts = 0

async function handleRequest(request: IncomingMessage, response: ServerResponse): Promise<void> {
  if (request.method !== "POST" || request.url !== "/v1/chat/completions") {
    response.writeHead(404).end()
    return
  }
  const chunks: Uint8Array[] = []
  for await (const chunk of request) {
    chunks.push(typeof chunk === "string" ? new TextEncoder().encode(chunk) : chunk as Uint8Array)
  }
  const body = JSON.parse(Buffer.concat(chunks).toString("utf8")) as { messages?: Array<{ content?: string }>, stream?: boolean }
  const content = body.messages?.at(-1)?.content ?? ""
  if (content === "unknown-outcome") {
    request.socket.destroy()
    return
  }
  if (content === "context-retry") {
    contextAttempts += 1
    if (contextAttempts === 1) {
      response.writeHead(400, { "content-type": "application/json" })
      response.end(JSON.stringify({ error: { code: "context_length_exceeded", type: "invalid_request_error", param: "messages" } }))
      return
    }
  }
  if (body.stream) {
    response.writeHead(200, { "content-type": "text/event-stream", "x-request-id": "req_http_stream" })
    response.write(`data: ${JSON.stringify({ id: "chatcmpl_http", model: "model-http", choices: [{ delta: { content: "done" }, finish_reason: "stop" }] })}\n\n`)
    response.write(`data: ${JSON.stringify({
      id: "chatcmpl_http",
      model: "model-http",
      choices: [],
      usage: {
        prompt_tokens: 11,
        completion_tokens: 4,
        total_tokens: 15,
        prompt_tokens_details: { cached_tokens: 5, cache_write_tokens: 1 },
        completion_tokens_details: { reasoning_tokens: 2 },
      },
    })}\n\n`)
    response.end("data: [DONE]\n\n")
    return
  }
  const usage = content === "missing-usage"
    ? {}
    : content === "invalid-usage"
      ? { usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 3 } }
      : { usage: { prompt_tokens: 2, completion_tokens: 1, total_tokens: 3 } }
  response.writeHead(200, { "content-type": "application/json" })
  response.end(JSON.stringify({ id: "chatcmpl_http", model: "model-http", choices: [{ message: { content: "done" } }], ...usage }))
}

function provider(baseUrl: string): OpenAIChatCompletionsProvider {
  return new OpenAIChatCompletionsProvider({ baseUrl, apiKey: "test-key", channelAffinityEnabled: true, promptCacheKeyEnabled: false, model: "model-http", timeoutMs: 5_000 })
}

function modelRequest(content: string) {
  return { messages: [{ role: "user" as const, content }], maxOutputTokens: 32 }
}

function compiled(compaction?: { summarizedThroughTurnIndex: number, sourceTurnCount: number, trigger: "context_error" }) {
  return {
    messages: [{ role: "system" as const, content: "system" }, { role: "user" as const, content: "context-retry" }],
    recentTurnCount: compaction ? 0 : 1,
    compressionOutcome: compaction ? "compressed" as const : "not_needed" as const,
    ...(compaction ? { summarizedThroughTurnIndex: compaction.summarizedThroughTurnIndex, compaction } : {}),
  }
}

function runtimeInput(): AssistantModelInput {
  return {
    runId: "airun_http", ownerUserId: "usr_http", conversationId: "aicnv_http", input: "context-retry",
    pageContext: {}, history: [{ turnIndex: 0, user: "old", assistant: "answer" }],
    conversation: { title: "HTTP", titleSource: "user", turnIndex: 1 }, promptVersion: "system-v4",
    reasoningSummary: "", answer: "", toolCalls: [], continuationMessages: [], loadedOperationIds: [], toolCatalogDigest: "sha256:http",
  }
}
