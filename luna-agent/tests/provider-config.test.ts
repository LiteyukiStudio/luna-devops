import { afterEach, describe, expect, it, vi } from "vitest"
import { defaultRuntimeSettings } from "../src/runtime-settings.js"
import { ProviderConfigClient } from "../src/provider/config-client.js"

afterEach(() => vi.unstubAllGlobals())

const runtimePayload = {
  ...defaultRuntimeSettings,
  providerTimeoutMs: 45_000,
  runTimeoutMs: 420_000,
  agentConcurrentRuns: 3,
  userConcurrentRuns: 10,
  contextInputTokenBudget: 256 * 1024,
}

describe("ProviderConfigClient", () => {
  it("defaults to a log-heavy DevOps context budget", () => {
    expect(defaultRuntimeSettings.contextInputTokenBudget).toBe(1024 * 1024)
    expect(defaultRuntimeSettings.runMaxToolCalls).toBe(256)
  })

  it("uses only the callback service identity and parses no-store configuration", async () => {
    const fetchMock = vi.fn(async (_url: URL, init: RequestInit) => {
      expect(init.headers).toMatchObject({ authorization: "Bearer callback-token-value" })
      return new Response(JSON.stringify({
        version: "cfg-1",
        provider: { baseUrl: "https://provider.example/v1/", model: "model-a", apiKey: "secret", configured: true },
        runtime: runtimePayload,
        toolCatalog: [{ operationId: "listProjects" }],
      }), { status: 200, headers: { "content-type": "application/json", "cache-control": "no-store" } })
    })
    vi.stubGlobal("fetch", fetchMock)
    const config = await new ProviderConfigClient("https://luna-api.internal", "callback-token-value").get()
    expect(config.version).toBe("cfg-1")
    expect(config.runtime).toEqual(runtimePayload)
    expect(config.toolCatalog).toEqual([{ operationId: "listProjects" }])
    expect(fetchMock).toHaveBeenCalledOnce()
  })

  it("accepts model token limits supplied by the Luna API contract", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      version: "cfg-model-limits",
      provider: {
        baseUrl: "https://provider.example/v1/",
        model: "model-a",
        apiKey: "secret",
        configured: true,
        models: [{
          id: "aimod_test",
          name: "model-a",
          maxContextTokens: 524_288,
          maxOutputTokens: 65_536,
          inputCreditsPerMillion: "1.25",
          outputCreditsPerMillion: "2.5",
          cachedInputCreditsPerMillion: "0.5",
          cachedOutputCreditsPerMillion: "0.75",
        }],
      },
      runtime: runtimePayload,
    }), { status: 200 })))

    const config = await new ProviderConfigClient("https://luna-api.internal", "callback-token-value").get()
    expect(config.provider.models).toEqual([expect.objectContaining({
      id: "aimod_test",
      maxContextTokens: 524_288,
      maxOutputTokens: 65_536,
    })])
  })

  it("accepts the expanded context budget up to the model catalog capacity", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      version: "cfg-expanded-context",
      provider: { baseUrl: "", model: "", apiKey: "", configured: false },
      runtime: { ...runtimePayload, contextInputTokenBudget: 2048 * 1024 },
    }), { status: 200 })))

    const config = await new ProviderConfigClient("https://luna-api.internal", "callback-token-value").get()
    expect(config.runtime.contextInputTokenBudget).toBe(2048 * 1024)
  })

  it("normalizes a legacy context budget above the current platform range", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      version: "cfg-context-too-large",
      provider: { baseUrl: "", model: "", apiKey: "", configured: false },
      runtime: { ...runtimePayload, contextInputTokenBudget: 2049 * 1024 },
    }), { status: 200 })))

    const config = await new ProviderConfigClient("https://luna-api.internal", "callback-token-value").get()
    expect(config.runtime.contextInputTokenBudget).toBe(defaultRuntimeSettings.contextInputTokenBudget)
  })

  it("retries transient configuration failures before parsing the response", async () => {
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response("busy", { status: 503, headers: { "retry-after": "0" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        version: "cfg-recovered",
        provider: { baseUrl: "", model: "", apiKey: "", configured: false },
        runtime: runtimePayload,
      }), { status: 200 }))
    vi.stubGlobal("fetch", fetchMock)

    const config = await new ProviderConfigClient("https://luna-api.internal", "callback-token-value").get()
    expect(config.version).toBe("cfg-recovered")
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it("falls back to platform defaults when advanced runtime fields are omitted", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      version: "cfg-partial",
      provider: { baseUrl: "", model: "", apiKey: "", configured: false },
      runtime: {
        providerTimeoutMs: 30_000,
        runTimeoutMs: 300_000,
        agentConcurrentRuns: 2,
        userConcurrentRuns: 10,
        contextInputTokenBudget: 256 * 1024,
      },
    }), { status: 200 })))
    const config = await new ProviderConfigClient("https://luna-api.internal", "callback-token-value").get()
    expect(config.runtime.assistantMaxOutputTokens).toBe(defaultRuntimeSettings.assistantMaxOutputTokens)
    expect(config.runtime.contextCompressionTriggerRatio).toBe(defaultRuntimeSettings.contextCompressionTriggerRatio)
    expect(config.runtime.toolResultPayloadBudget).toBe(defaultRuntimeSettings.toolResultPayloadBudget)
  })

  it("normalizes invalid historical runtime settings while keeping valid fields", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      version: "cfg-unsafe",
      provider: { baseUrl: "", model: "", apiKey: "", configured: false },
      runtime: {
        providerTimeoutMs: 0,
        runTimeoutMs: 1_000,
        agentConcurrentRuns: 101,
        userConcurrentRuns: 0,
        contextInputTokenBudget: 32 * 1024,
        maxRequestRetries: 7,
      },
    }), { status: 200 })))
    const config = await new ProviderConfigClient("https://luna-api.internal", "callback-token-value").get()
    expect(config.runtime).toMatchObject({
      providerTimeoutMs: defaultRuntimeSettings.providerTimeoutMs,
      runTimeoutMs: defaultRuntimeSettings.runTimeoutMs,
      agentConcurrentRuns: defaultRuntimeSettings.agentConcurrentRuns,
      userConcurrentRuns: defaultRuntimeSettings.userConcurrentRuns,
      contextInputTokenBudget: defaultRuntimeSettings.contextInputTokenBudget,
      maxRequestRetries: 7,
    })
  })

  it("accepts the configurable high tool-call guard range", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      version: "cfg-tool-budget",
      provider: { baseUrl: "", model: "", apiKey: "", configured: false },
      runtime: { ...runtimePayload, runMaxToolCalls: 2048 },
    }), { status: 200 })))
    const config = await new ProviderConfigClient("https://luna-api.internal", "callback-token-value").get()
    expect(config.runtime.runMaxToolCalls).toBe(2048)
  })

  it("normalizes the legacy 20-call default during a rolling upgrade", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      version: "cfg-tool-budget-legacy",
      provider: { baseUrl: "", model: "", apiKey: "", configured: false },
      runtime: { ...runtimePayload, runMaxToolCalls: 20 },
    }), { status: 200 })))
    const config = await new ProviderConfigClient("https://luna-api.internal", "callback-token-value").get()
    expect(config.runtime.runMaxToolCalls).toBe(256)
  })

  it("normalizes an out-of-range tool-call guard and reports only its stable field name", async () => {
    const write = vi.spyOn(process.stdout, "write").mockImplementation(() => true)
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      version: "cfg-tool-budget-too-small",
      provider: { baseUrl: "", model: "", apiKey: "", configured: false },
      runtime: { ...runtimePayload, runMaxToolCalls: 31 },
    }), { status: 200 })))
    try {
      const config = await new ProviderConfigClient("https://luna-api.internal", "callback-token-value").get()
      expect(config.runtime.runMaxToolCalls).toBe(defaultRuntimeSettings.runMaxToolCalls)
      const telemetry = write.mock.calls.map(call => String(call[0])).join("\n")
      expect(telemetry).toContain("agent.provider_config.normalized")
      expect(telemetry).toContain("runtime.runMaxToolCalls")
      expect(telemetry).not.toContain("callback-token-value")
      expect(telemetry).not.toContain('"runMaxToolCalls":31')
    }
    finally {
      write.mockRestore()
    }
  })

  it("reports the same normalized configuration only once across refreshes", async () => {
    const write = vi.spyOn(process.stdout, "write").mockImplementation(() => true)
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      version: "cfg-tool-budget-repeated",
      provider: { baseUrl: "", model: "", apiKey: "", configured: false },
      runtime: { ...runtimePayload, runMaxToolCalls: 20 },
    }), { status: 200 })))
    try {
      const client = new ProviderConfigClient("https://luna-api.internal", "callback-token-value")
      await client.get()
      await client.get()
      const normalizedEvents = write.mock.calls
        .map(call => String(call[0]))
        .filter(line => line.includes('"message":"agent.provider_config.normalized"'))
      expect(normalizedEvents).toHaveLength(1)
    }
    finally {
      write.mockRestore()
    }
  })

  it("normalizes inconsistent advanced context settings as a pair", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      version: "cfg-inconsistent",
      provider: { baseUrl: "", model: "", apiKey: "", configured: false },
      runtime: { ...defaultRuntimeSettings, contextCompressionTriggerRatio: 0.5, contextCompressionTargetRatio: 0.6 },
    }), { status: 200 })))
    const config = await new ProviderConfigClient("https://luna-api.internal", "callback-token-value").get()
    expect(config.runtime.contextCompressionTriggerRatio).toBe(defaultRuntimeSettings.contextCompressionTriggerRatio)
    expect(config.runtime.contextCompressionTargetRatio).toBe(defaultRuntimeSettings.contextCompressionTargetRatio)
  })

  it("keeps provider and model catalog structures fail closed", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      version: "cfg-invalid-model-contract",
      provider: {
        baseUrl: "https://provider.example/v1/",
        model: "model-a",
        apiKey: "secret",
        configured: true,
        models: [{ id: "aimod_test", name: "model-a", maxContextTokens: 1 }],
      },
      runtime: runtimePayload,
    }), { status: 200 })))
    await expect(new ProviderConfigClient("https://luna-api.internal", "callback-token-value").get())
      .rejects.toThrow("ai.provider_config_invalid")
  })

  it("maps an unavailable platform configuration endpoint to a stable error", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("unauthorized", { status: 401 })))

    await expect(new ProviderConfigClient("https://luna-api.internal", "callback-token-value").get())
      .rejects.toThrow("ai.provider_config_unavailable")
  })
})
