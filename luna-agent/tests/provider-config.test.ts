import { afterEach, describe, expect, it, vi } from "vitest"
import { RemoteConfigSnapshot } from "../src/provider/config-client.js"
import { agentRuntimeInternals, defaultRuntimeSettings, type RemoteRuntimeSettings } from "../src/runtime-settings.js"
import { testToolOperation } from "./support/tool-catalog.js"

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe("RemoteConfigSnapshot", () => {
  it("uses only the callback service identity and accepts the complete authority payload", async () => {
    const fetchMock = vi.fn(async (_url: URL, init: RequestInit) => {
      expect(init.headers).toMatchObject({ authorization: "Bearer callback-token-value" })
      return response(authoritativePayload())
    })
    vi.stubGlobal("fetch", fetchMock)

    const configClient = client()
    const config = await configClient.initialize()
    expect(config.version).toBe("cfg-1")
    expect(config.provider.models[0]).toMatchObject({ id: "aimod_test", name: "model-a" })
    expect(config.runtime).toEqual(authoritativePayload().runtime)
    expect(config.toolCatalog).toEqual([expect.objectContaining({ operationId: "listProjects" })])
    expect(configClient.currentCatalog()?.get("listProjects").operationId).toBe("listProjects")
    expect(fetchMock).toHaveBeenCalledOnce()
  })

  it("publishes one validated refresh to all consumers", async () => {
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(response(authoritativePayload()))
      .mockResolvedValueOnce(response({ ...authoritativePayload(), version: "cfg-2" }))
    vi.stubGlobal("fetch", fetchMock)
    const configClient = client()
    expect((await configClient.initialize()).version).toBe("cfg-1")
    const observed: string[] = []
    configClient.subscribe(config => observed.push(config.version))
    expect((await configClient.refresh()).version).toBe("cfg-2")
    expect(configClient.current()?.version).toBe("cfg-2")
    expect(observed).toEqual(["cfg-2"])
  })

  it("coalesces a slow refresh so an older request cannot overwrite a newer snapshot", async () => {
    const pending = deferred<Response>()
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(response(authoritativePayload()))
      .mockImplementationOnce(() => pending.promise)
      .mockResolvedValueOnce(response({ ...authoritativePayload(), version: "cfg-3" }))
    vi.stubGlobal("fetch", fetchMock)
    const snapshot = client()
    await snapshot.initialize()

    const controller = new AbortController()
    const first = snapshot.refresh(controller.signal)
    const joined = snapshot.refresh()
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2))
    controller.abort(new Error("caller stopped waiting"))
    await expect(first).rejects.toThrow("caller stopped waiting")
    pending.resolve(response({ ...authoritativePayload(), version: "cfg-2" }))

    await expect(joined).resolves.toMatchObject({ version: "cfg-2" })
    expect(snapshot.current()?.version).toBe("cfg-2")
    await expect(snapshot.refresh()).resolves.toMatchObject({ version: "cfg-3" })
    expect(fetchMock).toHaveBeenCalledTimes(3)
  })

  it("bounds a hanging fetch and schedules no overlapping polling refreshes", async () => {
    vi.useFakeTimers()
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(response(authoritativePayload({ maxRequestRetries: 0 })))
      .mockImplementationOnce((_input, init) => new Promise<Response>((_resolve, reject) => {
        const signal = init?.signal
        const rejectOnAbort = () => reject(signal?.reason instanceof Error ? signal.reason : new Error("aborted"))
        if (signal?.aborted) rejectOnAbort()
        else signal?.addEventListener("abort", rejectOnAbort, { once: true })
      }))
      .mockResolvedValueOnce(response({ ...authoritativePayload({ maxRequestRetries: 0 }), version: "cfg-2" }))
    vi.stubGlobal("fetch", fetchMock)
    const snapshot = new RemoteConfigSnapshot(
      "https://luna-api.internal",
      "callback-token-value",
      50,
    )
    await snapshot.initialize()
    snapshot.start()

    await vi.advanceTimersByTimeAsync(50)
    expect(fetchMock).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(agentRuntimeInternals.configFetchTimeoutMs - 1)
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(snapshot.current()?.version).toBe("cfg-1")

    await vi.advanceTimersByTimeAsync(1)
    await vi.advanceTimersByTimeAsync(49)
    expect(fetchMock).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(1)
    expect(fetchMock).toHaveBeenCalledTimes(3)
    expect(snapshot.current()?.version).toBe("cfg-2")
    snapshot.stop()
  })

  it("keeps the last valid snapshot when a semantic candidate is rejected", async () => {
    const invalidOperation = { ...testToolOperation("listProjects") }
    Reflect.deleteProperty(invalidOperation, "requiresApproval")
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(response(authoritativePayload()))
      .mockResolvedValueOnce(response({ ...authoritativePayload(), version: "cfg-2", toolCatalog: [invalidOperation] }))
    vi.stubGlobal("fetch", fetchMock)
    const snapshot = new RemoteConfigSnapshot("https://luna-api.internal", "callback-token-value")
    await snapshot.initialize()
    const oldCatalogDigest = snapshot.currentCatalog()?.digest

    await expect(snapshot.refresh()).rejects.toThrow("ai.provider_config_invalid")
    expect(snapshot.current()?.version).toBe("cfg-1")
    expect(snapshot.currentCatalog()?.digest).toBe(oldCatalogDigest)
  })

  it("keeps serving the last valid snapshot during a short authority outage", async () => {
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(response(authoritativePayload({ maxRequestRetries: 0 })))
      .mockRejectedValueOnce(new Error("connection reset"))
    vi.stubGlobal("fetch", fetchMock)
    const snapshot = client()
    await snapshot.initialize()

    await expect(snapshot.refresh()).rejects.toThrow("ai.provider_config_unavailable")
    expect(snapshot.current()?.version).toBe("cfg-1")
  })

  it("rejects omitted runtime fields instead of merging local defaults", async () => {
    const payload = authoritativePayload()
    const runtime = { ...payload.runtime }
    Reflect.deleteProperty(runtime, "maxRequestRetries")
    vi.stubGlobal("fetch", vi.fn(async () => response({ ...payload, runtime })))
    await expect(client().initialize()).rejects.toThrow("ai.provider_config_invalid")
  })

  it("rejects runtime fields outside the strict OpenAPI contract", async () => {
    const payload = authoritativePayload()
    vi.stubGlobal("fetch", vi.fn(async () => response({
      ...payload,
      runtime: { ...payload.runtime, unexpected: 1 },
    })))
    await expect(client().initialize()).rejects.toThrow("ai.provider_config_invalid")
  })

  it("rejects omitted model and tool catalogs", async () => {
    const payload = authoritativePayload()
    const provider = { ...payload.provider }
    const withoutCatalog = { ...payload }
    Reflect.deleteProperty(provider, "models")
    Reflect.deleteProperty(withoutCatalog, "toolCatalog")
    vi.stubGlobal("fetch", vi.fn(async () => response({ ...withoutCatalog, provider })))
    await expect(client().initialize()).rejects.toThrow("ai.provider_config_invalid")
  })

  it("rejects an omitted channel-affinity policy instead of inventing a local default", async () => {
    const payload = authoritativePayload()
    const provider = { ...payload.provider }
    Reflect.deleteProperty(provider, "channelAffinityEnabled")
    vi.stubGlobal("fetch", vi.fn(async () => response({ ...payload, provider })))
    await expect(client().initialize()).rejects.toThrow("ai.provider_config_invalid")
  })

  it.each(["providerCompatibility", "promptCacheKeyMode"])("rejects an omitted %s policy instead of inventing a local default", async (field) => {
    const payload = authoritativePayload()
    const provider = { ...payload.provider }
    Reflect.deleteProperty(provider, field)
    vi.stubGlobal("fetch", vi.fn(async () => response({ ...payload, provider })))
    await expect(client().initialize()).rejects.toThrow("ai.provider_config_invalid")
  })

  it("rejects invalid limits instead of normalizing them", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => response(authoritativePayload({ maxRequestRetries: 11 }))))
    await expect(client().initialize()).rejects.toThrow("ai.provider_config_invalid")
  })

  it("retries transient configuration failures before parsing the authority payload", async () => {
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response("busy", { status: 503, headers: { "retry-after": "0" } }))
      .mockResolvedValueOnce(response(authoritativePayload({ maxRequestRetries: 1 })))
    vi.stubGlobal("fetch", fetchMock)

    expect((await client().initialize()).version).toBe("cfg-1")
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it("keeps model pricing structures fail closed", async () => {
    const payload = authoritativePayload()
    vi.stubGlobal("fetch", vi.fn(async () => response({
      ...payload,
      provider: { ...payload.provider, models: [{ id: "aimod_test", name: "model-a", maxContextTokens: 1 }] },
    })))
    await expect(client().initialize()).rejects.toThrow("ai.provider_config_invalid")
  })

  it("maps an unavailable authority endpoint to a stable error", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("unauthorized", { status: 401 })))
    await expect(client().initialize()).rejects.toThrow("ai.provider_config_unavailable")
  })
})

function authoritativePayload(runtimeOverrides: Partial<RemoteRuntimeSettings> = {}) {
  const remoteRuntimeDefaults: RemoteRuntimeSettings = {
    providerTimeoutMs: defaultRuntimeSettings.providerTimeoutMs,
    maxRequestRetries: defaultRuntimeSettings.maxRequestRetries,
    runTimeoutMs: defaultRuntimeSettings.runTimeoutMs,
    agentConcurrentRuns: defaultRuntimeSettings.agentConcurrentRuns,
    userConcurrentRuns: defaultRuntimeSettings.userConcurrentRuns,
  }
  return {
    version: "cfg-1",
    provider: {
      baseUrl: "https://provider.example/v1/",
      apiKey: "secret",
      providerCompatibility: "auto" as const,
      promptCacheKeyMode: "auto" as const,
      channelAffinityEnabled: true,
      configured: true,
      models: [{
        id: "aimod_test",
        name: "model-a",
        maxContextTokens: 524_288,
        maxOutputTokens: 65_536,
        inputCreditsPerMillion: "1.25",
        outputCreditsPerMillion: "2.5",
        cachedInputCreditsPerMillion: "0.5",
      }],
    },
    runtime: { ...remoteRuntimeDefaults, providerTimeoutMs: 45_000, ...runtimeOverrides },
    toolCatalog: [testToolOperation("listProjects")],
  }
}

function response(payload: unknown): Response {
  return new Response(JSON.stringify(payload), {
    status: 200,
    headers: { "content-type": "application/json", "cache-control": "no-store" },
  })
}

function client(): RemoteConfigSnapshot {
  return new RemoteConfigSnapshot("https://luna-api.internal", "callback-token-value")
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((settle) => { resolve = settle })
  return { promise, resolve }
}
