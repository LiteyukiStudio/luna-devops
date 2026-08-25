import type { Attributes, Span, SpanOptions } from "@opentelemetry/api"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

const telemetryState = vi.hoisted(() => ({
  spans: [] as Array<{ attributes: Record<string, unknown>, ended: boolean }>,
}))

vi.mock("../src/telemetry.js", async (importOriginal) => {
  const actual = await importOriginal<Record<string, unknown>>()
  const createSpan = (): Span => {
    const captured = { attributes: {} as Record<string, unknown>, ended: false }
    telemetryState.spans.push(captured)
    return {
      setAttribute(name: string, value: unknown) {
        captured.attributes[name] = value
        return this
      },
      setAttributes(attributes: Attributes) {
        Object.assign(captured.attributes, attributes)
        return this
      },
      setStatus() { return this },
      updateName() { return this },
      addEvent() { return this },
      addLink() { return this },
      addLinks() { return this },
      recordException() {},
      end() { captured.ended = true },
      isRecording() { return true },
      spanContext() {
        return { traceId: "0".repeat(32), spanId: "0".repeat(16), traceFlags: 0 }
      },
    }
  }
  return {
    ...actual,
    telemetryLog: () => undefined,
    withSpan: async <T>(
      _name: string,
      _options: SpanOptions,
      operation: (span: Span) => Promise<T>,
    ): Promise<T> => {
      const span = createSpan()
      try { return await operation(span) }
      finally { span.end() }
    },
    withSpanStream: async function* <T>(
      _name: string,
      _options: SpanOptions,
      operation: (span: Span) => AsyncIterable<T>,
    ): AsyncIterable<T> {
      const span = createSpan()
      try { yield* operation(span) }
      finally { span.end() }
    },
  }
})

const { OpenAIChatCompletionsProvider } = await import("../src/provider/openai-chat-completions.js")

const providerOptions = {
  baseUrl: "https://provider.example/v1",
  apiKey: "secret",
  channelAffinityEnabled: true,
  promptCacheKeyEnabled: false,
  model: "model-a",
  timeoutMs: 5_000,
}
const request = {
  messages: [{ role: "user" as const, content: "hello" }],
  maxOutputTokens: 100,
  budget: { runId: "airun_span_failure", ownerUserId: "usr_span_failure", operation: "assistant" as const },
}

beforeEach(() => {
  telemetryState.spans.length = 0
})

afterEach(() => vi.unstubAllGlobals())

describe("OpenAI Chat Completions failure usage spans", () => {
  it("marks a rejected complete attempt as unavailable before ending the span", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      error: { code: "context_length_exceeded", message: "too long" },
    }), { status: 400 })))

    await expect(new OpenAIChatCompletionsProvider(providerOptions).complete(request)).rejects.toThrow()

    expect(telemetryState.spans.at(-1)).toMatchObject({
      ended: true,
      attributes: {
        "luna.gen_ai.usage.status": "unavailable",
        "luna.gen_ai.usage.unavailable_reason": "request_failed",
      },
    })
  })

  it("marks a malformed stream attempt as unavailable before ending the span", async () => {
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(new TextEncoder().encode("data: {malformed}\n\n"))
        controller.close()
      },
    })
    vi.stubGlobal("fetch", vi.fn(async () => new Response(body, {
      status: 200,
      headers: { "content-type": "text/event-stream" },
    })))

    const consume = async () => {
      await new OpenAIChatCompletionsProvider(providerOptions).stream(request)[Symbol.asyncIterator]().next()
    }
    await expect(consume()).rejects.toThrow()

    expect(telemetryState.spans.at(-1)).toMatchObject({
      ended: true,
      attributes: {
        "luna.gen_ai.usage.status": "unavailable",
        "luna.gen_ai.usage.unavailable_reason": "request_failed",
      },
    })
  })
})
