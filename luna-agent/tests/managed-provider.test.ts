import { afterEach, describe, expect, it, vi } from "vitest"
import { ManagedProvider } from "../src/provider/managed.js"
import { defaultRuntimeSettings } from "../src/runtime-settings.js"
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
        runtime: { ...defaultRuntimeSettings, providerTimeoutMs: 30_000, runTimeoutMs: 300_000, agentConcurrentRuns: 2, userConcurrentRuns: 10, contextInputTokenBudget: 256 * 1024 },
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
        runtime: { ...defaultRuntimeSettings, providerTimeoutMs: 30_000, runTimeoutMs: 300_000, agentConcurrentRuns: 2, userConcurrentRuns: 10, contextInputTokenBudget: 256 * 1024 },
      })),
    }
    const provider = new ManagedProvider(resolver, 1000, config => fakeProvider(config.provider.model))
    await Promise.all([provider.health(), provider.health(), provider.health()])
    expect(resolver.get).toHaveBeenCalledOnce()
    expect(JSON.stringify(provider)).not.toContain("secret-value")
  })

  it("keeps concurrent first requests on their selected models", async () => {
    let resolveConfig!: (config: Awaited<ReturnType<typeof configForModels>>) => void
    const configPromise = new Promise<Awaited<ReturnType<typeof configForModels>>>(resolve => { resolveConfig = resolve })
    const resolver = { get: vi.fn(() => configPromise) }
    const provider = new ManagedProvider(resolver, 1000, config => fakeProvider(config.provider.model))
    const request = (modelId: string) => provider.complete({ modelId, messages: [], maxOutputTokens: 10 })

    const first = request("model-a-id")
    const second = request("model-b-id")
    resolveConfig(configForModels())

    expect((await first).text).toBe("model-a")
    expect((await second).text).toBe("model-b")
  })
})

function configForModels() {
  return {
    version: "cfg-1",
    provider: {
      baseUrl: "https://provider.example/v1/",
      model: "model-a",
      apiKey: "secret-value",
      configured: true,
      models: [
        { id: "model-a-id", name: "model-a", maxContextTokens: 524_288, maxOutputTokens: 65_536, inputCreditsPerMillion: "1", outputCreditsPerMillion: "1", cachedInputCreditsPerMillion: "0", cachedOutputCreditsPerMillion: "0" },
        { id: "model-b-id", name: "model-b", maxContextTokens: 524_288, maxOutputTokens: 65_536, inputCreditsPerMillion: "1", outputCreditsPerMillion: "1", cachedInputCreditsPerMillion: "0", cachedOutputCreditsPerMillion: "0" },
      ],
    },
    runtime: { ...defaultRuntimeSettings, providerTimeoutMs: 30_000, runTimeoutMs: 300_000, agentConcurrentRuns: 2, userConcurrentRuns: 10, contextInputTokenBudget: 256 * 1024 },
  }
}

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
