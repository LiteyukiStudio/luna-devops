import { afterEach, describe, expect, it, vi } from "vitest"
import { createConfiguredProvider, ManagedProvider } from "../src/provider/managed.js"
import { DeepSeekChatCompletionsProvider } from "../src/provider/deepseek-chat-completions.js"
import { OpenAIChatCompletionsProvider } from "../src/provider/openai-chat-completions.js"
import { defaultRuntimeSettings } from "../src/runtime-settings.js"
import type { ModelProvider } from "../src/provider/provider.js"

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe("ManagedProvider", () => {
  it("selects a provider adapter whose telemetry dimension matches the endpoint", () => {
    expect(createConfiguredProvider(configForModels(), "model-a")).toBeInstanceOf(OpenAIChatCompletionsProvider)
    expect(createConfiguredProvider({
      ...configForModels(),
      provider: { ...configForModels().provider, baseUrl: "https://api.deepseek.com/v1" },
    }, "deepseek-chat")).toBeInstanceOf(DeepSeekChatCompletionsProvider)
    expect(createConfiguredProvider({
      ...configForModels(),
      provider: { ...configForModels().provider, baseUrl: "https://notdeepseek.com/v1" },
    }, "model-a")).toBeInstanceOf(OpenAIChatCompletionsProvider)
    expect(createConfiguredProvider({
      ...configForModels(),
      provider: { ...configForModels().provider, baseUrl: "https://gateway.example/v1", providerCompatibility: "deepseek" },
    }, "deepseek-chat")).toBeInstanceOf(DeepSeekChatCompletionsProvider)
    expect(createConfiguredProvider({
      ...configForModels(),
      provider: { ...configForModels().provider, baseUrl: "https://api.deepseek.com/v1", providerCompatibility: "openai" },
    }, "deepseek-chat")).toBeInstanceOf(OpenAIChatCompletionsProvider)
  })

  it.each([
    ["official OpenAI auto mode", "https://api.openai.com/v1", "auto", "auto", true],
    ["unknown gateway auto mode", "https://gateway.example/v1", "auto", "auto", false],
    ["confirmed compatible gateway", "https://gateway.example/v1", "openai", "enabled", true],
    ["DeepSeek adapter defense", "https://gateway.example/v1", "deepseek", "enabled", false],
  ] as const)("gates prompt_cache_key for %s", async (_name, baseUrl, providerCompatibility, promptCacheKeyMode, expected) => {
    const fetchMock = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({
      id: "chatcmpl_gate", model: "model-a", choices: [{ message: { content: "done" } }],
      usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 },
    }), { status: 200 }))
    vi.stubGlobal("fetch", fetchMock)
    const config = configForModels()
    const provider = createConfiguredProvider({
      ...config,
      provider: { ...config.provider, baseUrl, providerCompatibility, promptCacheKeyMode },
    }, "model-a")

    await provider.complete({
      messages: [{ role: "user", content: "hello" }],
      maxOutputTokens: 10,
      conversationId: "aicnv_test",
      budget: { runId: "airun_test", ownerUserId: "usr_test", operation: "assistant" },
    })

    const raw = fetchMock.mock.calls[0]?.[1]?.body
    if (typeof raw !== "string") throw new TypeError("expected JSON request body")
    const body = JSON.parse(raw) as Record<string, unknown>
    expect(Object.hasOwn(body, "prompt_cache_key")).toBe(expected)
  })

  it("uses a short-lived authoritative configuration and applies updates", async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date("2026-07-29T00:00:00Z"))
    let version = 1
    const resolver = { get: vi.fn(async () => configForVersion(version)) }
    const provider = new ManagedProvider(resolver, 1000, (_config, modelName) => fakeProvider(modelName))
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
    const resolver = { get: vi.fn(async () => configForVersion(1)) }
    const provider = new ManagedProvider(resolver, 1000, (_config, modelName) => fakeProvider(modelName))
    await Promise.all([provider.health(), provider.health(), provider.health()])
    expect(resolver.get).toHaveBeenCalledOnce()
    expect(JSON.stringify(provider)).not.toContain("secret-value")
  })

  it("keeps concurrent first requests on their selected models", async () => {
    let resolveConfig!: (config: ReturnType<typeof configForModels>) => void
    const configPromise = new Promise<ReturnType<typeof configForModels>>(resolve => { resolveConfig = resolve })
    const resolver = { get: vi.fn(() => configPromise) }
    const provider = new ManagedProvider(resolver, 1000, (_config, modelName) => fakeProvider(modelName))
    const request = (modelId: string) => provider.complete({ modelId, messages: [], maxOutputTokens: 10 })

    const first = request("model-a-id")
    const second = request("model-b-id")
    resolveConfig(configForModels())

    expect((await first).text).toBe("model-a")
    expect((await second).text).toBe("model-b")
  })

  it("rejects an unknown model instead of inventing a zero-price fallback", async () => {
    const provider = new ManagedProvider({ get: vi.fn(async () => configForModels()) }, 1000)
    await expect(provider.complete({ modelId: "missing", messages: [], maxOutputTokens: 10 }))
      .rejects.toThrow("ai.model_not_available")
  })
})

function configForVersion(version: number) {
  return {
    ...configForModels(),
    version: `cfg-${version}`,
    provider: {
      ...configForModels().provider,
      apiKey: `secret-${version}`,
      models: [{ ...configForModels().provider.models[0]!, id: `model-${version}-id`, name: `model-${version}` }],
    },
  }
}

function configForModels() {
  return {
    version: "cfg-1",
    provider: {
      baseUrl: "https://provider.example/v1/",
      apiKey: "secret-value",
      providerCompatibility: "auto" as const,
      promptCacheKeyMode: "auto" as const,
      channelAffinityEnabled: true,
      configured: true,
      models: [
        modelSnapshot("model-a-id", "model-a"),
        modelSnapshot("model-b-id", "model-b"),
      ],
    },
    runtime: { ...defaultRuntimeSettings },
    toolCatalog: [{ operationId: "listProjects" }],
  }
}

function modelSnapshot(id: string, name: string) {
  return {
    id,
    name,
    maxContextTokens: 524_288,
    maxOutputTokens: 65_536,
    inputCreditsPerMillion: "1",
    outputCreditsPerMillion: "1",
    cachedInputCreditsPerMillion: "0",
  }
}

function fakeProvider(model: string): ModelProvider {
  return {
    capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
    health: async () => ({ ok: true, requestId: `request-${model}` }),
    complete: async () => ({ text: model, usage: { status: "reported" as const, value: { inputTokens: 1, outputTokens: 1, totalTokens: 1 + 1 } } }),
    async *stream() {
      yield { type: "message_delta" as const, delta: model }
      yield { type: "completed" as const, usage: { status: "reported" as const, value: { inputTokens: 1, outputTokens: 1, totalTokens: 1 + 1 } } }
    },
  }
}
