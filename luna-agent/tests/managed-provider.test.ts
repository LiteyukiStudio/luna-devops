import { afterEach, describe, expect, it, vi } from "vitest"
import { ManagedProvider } from "../src/provider/managed.js"
import type { ModelProvider } from "../src/provider/provider.js"

afterEach(() => {
  vi.useRealTimers()
})

describe("ManagedProvider", () => {
  it("uses a short-lived in-memory configuration and applies updates to real completions", async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date("2026-07-29T00:00:00Z"))
    let version = 1
    const resolver = {
      get: vi.fn(async () => ({
        version: `cfg-${version}`,
        provider: {
          baseUrl: "https://provider.example/v1/",
          model: `model-${version}`,
          apiKey: `secret-${version}`,
          configured: true,
        },
      })),
    }
    const provider = new ManagedProvider(resolver, 1000, config => fakeProvider(config.provider.model))
    const request = { messages: [{ role: "user" as const, content: "hello" }], maxOutputTokens: 10 }
    expect((await provider.complete(request)).text).toBe("model-1")
    version = 2
    expect((await provider.complete(request)).text).toBe("model-1")
    expect(resolver.get).toHaveBeenCalledOnce()
    vi.advanceTimersByTime(1001)
    expect((await provider.complete(request)).text).toBe("model-2")
    expect(provider.currentVersion()).toBe("cfg-2")
    expect(resolver.get).toHaveBeenCalledTimes(2)
  })

  it("coalesces concurrent refreshes without persisting provider secrets", async () => {
    const resolver = {
      get: vi.fn(async () => ({
        version: "cfg-1",
        provider: {
          baseUrl: "https://provider.example/v1/",
          model: "model-a", apiKey: "secret-value", configured: true,
        },
      })),
    }
    const provider = new ManagedProvider(resolver, 1000, config => fakeProvider(config.provider.model))
    await Promise.all([provider.health(), provider.health(), provider.health()])
    expect(resolver.get).toHaveBeenCalledOnce()
    expect(JSON.stringify(provider)).not.toContain("secret-value")
  })
})

function fakeProvider(model: string): ModelProvider {
  return {
    capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
    health: async () => ({ ok: true, requestId: `request-${model}` }),
    complete: async () => ({ text: model, usage: { inputTokens: 1, outputTokens: 1 } }),
    async *stream() {
      yield { type: "message_delta" as const, delta: model }
      yield { type: "completed" as const, usage: { inputTokens: 1, outputTokens: 1 } }
    },
  }
}
