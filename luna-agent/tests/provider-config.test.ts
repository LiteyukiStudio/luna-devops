import { afterEach, describe, expect, it, vi } from "vitest"
import { ProviderConfigClient } from "../src/provider/config-client.js"
import { defaultRuntimeSettings } from "../src/runtime-settings.js"

afterEach(() => vi.unstubAllGlobals())

describe("ProviderConfigClient", () => {
  it("uses only the callback service identity and accepts the complete authority payload", async () => {
    const fetchMock = vi.fn(async (_url: URL, init: RequestInit) => {
      expect(init.headers).toMatchObject({ authorization: "Bearer callback-token-value" })
      return response(authoritativePayload())
    })
    vi.stubGlobal("fetch", fetchMock)

    const config = await client().get()
    expect(config.version).toBe("cfg-1")
    expect(config.provider.models[0]).toMatchObject({ id: "aimod_test", name: "model-a" })
    expect(config.runtime).toEqual(authoritativePayload().runtime)
    expect(config.toolCatalog).toEqual([{ operationId: "listProjects" }])
    expect(fetchMock).toHaveBeenCalledOnce()
  })

  it("accepts the configured upper context and tool-call limits", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => response(authoritativePayload({
      contextInputTokenBudget: 2048 * 1024,
      runMaxToolCalls: 2048,
    }))))
    const config = await client().get()
    expect(config.runtime.contextInputTokenBudget).toBe(2048 * 1024)
    expect(config.runtime.runMaxToolCalls).toBe(2048)
  })

  it("rejects omitted runtime fields instead of merging local defaults", async () => {
    const payload = authoritativePayload()
    const runtime = { ...payload.runtime }
    Reflect.deleteProperty(runtime, "assistantMaxOutputTokens")
    vi.stubGlobal("fetch", vi.fn(async () => response({ ...payload, runtime })))
    await expect(client().get()).rejects.toThrow("ai.provider_config_invalid")
  })

  it("rejects omitted model and tool catalogs", async () => {
    const payload = authoritativePayload()
    const provider = { ...payload.provider }
    const withoutCatalog = { ...payload }
    Reflect.deleteProperty(provider, "models")
    Reflect.deleteProperty(withoutCatalog, "toolCatalog")
    vi.stubGlobal("fetch", vi.fn(async () => response({ ...withoutCatalog, provider })))
    await expect(client().get()).rejects.toThrow("ai.provider_config_invalid")
  })

  it("rejects invalid limits instead of normalizing them", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => response(authoritativePayload({ runMaxToolCalls: 20 }))))
    await expect(client().get()).rejects.toThrow("ai.provider_config_invalid")
  })

  it("rejects inconsistent context settings instead of replacing the pair", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => response(authoritativePayload({
      contextCompressionTriggerRatio: 0.5,
      contextCompressionTargetRatio: 0.6,
    }))))
    await expect(client().get()).rejects.toThrow("ai.provider_config_invalid")
  })

  it("retries transient configuration failures before parsing the authority payload", async () => {
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response("busy", { status: 503, headers: { "retry-after": "0" } }))
      .mockResolvedValueOnce(response(authoritativePayload({ maxRequestRetries: 1 })))
    vi.stubGlobal("fetch", fetchMock)

    expect((await client().get()).version).toBe("cfg-1")
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it("keeps model pricing structures fail closed", async () => {
    const payload = authoritativePayload()
    vi.stubGlobal("fetch", vi.fn(async () => response({
      ...payload,
      provider: { ...payload.provider, models: [{ id: "aimod_test", name: "model-a", maxContextTokens: 1 }] },
    })))
    await expect(client().get()).rejects.toThrow("ai.provider_config_invalid")
  })

  it("maps an unavailable authority endpoint to a stable error", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("unauthorized", { status: 401 })))
    await expect(client().get()).rejects.toThrow("ai.provider_config_unavailable")
  })
})

function authoritativePayload(runtimeOverrides: Partial<typeof defaultRuntimeSettings> = {}) {
  return {
    version: "cfg-1",
    provider: {
      baseUrl: "https://provider.example/v1/",
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
    runtime: { ...defaultRuntimeSettings, providerTimeoutMs: 45_000, ...runtimeOverrides },
    toolCatalog: [{ operationId: "listProjects" }],
  }
}

function response(payload: unknown): Response {
  return new Response(JSON.stringify(payload), {
    status: 200,
    headers: { "content-type": "application/json", "cache-control": "no-store" },
  })
}

function client(): ProviderConfigClient {
  return new ProviderConfigClient("https://luna-api.internal", "callback-token-value")
}
