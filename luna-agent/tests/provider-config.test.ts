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

  it("rejects runtime settings outside the platform contract", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      version: "cfg-unsafe",
      provider: { baseUrl: "", model: "", apiKey: "", configured: false },
      runtime: { providerTimeoutMs: 0, runTimeoutMs: 1_000, agentConcurrentRuns: 101, userConcurrentRuns: 0, contextInputTokenBudget: 32 * 1024 },
    }), { status: 200 })))
    await expect(new ProviderConfigClient("https://luna-api.internal", "callback-token-value").get())
      .rejects.toThrow("ai.provider_config_invalid")
  })

  it("rejects inconsistent advanced context settings", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      version: "cfg-inconsistent",
      provider: { baseUrl: "", model: "", apiKey: "", configured: false },
      runtime: { ...defaultRuntimeSettings, contextCompressionTriggerRatio: 0.5, contextCompressionTargetRatio: 0.6 },
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
