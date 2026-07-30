import { afterEach, describe, expect, it, vi } from "vitest"
import { ProviderConfigClient } from "../src/provider/config-client.js"

afterEach(() => vi.unstubAllGlobals())

describe("ProviderConfigClient", () => {
  it("uses only the callback service identity and parses no-store configuration", async () => {
    const fetchMock = vi.fn(async (_url: URL, init: RequestInit) => {
      expect(init.headers).toMatchObject({ authorization: "Bearer callback-token-value" })
      return new Response(JSON.stringify({
        version: "cfg-1",
        provider: { baseUrl: "https://provider.example/v1/", model: "model-a", apiKey: "secret", configured: true },
        runtime: { providerTimeoutMs: 45_000, runTimeoutMs: 420_000, agentConcurrentRuns: 3 },
      }), { status: 200, headers: { "content-type": "application/json", "cache-control": "no-store" } })
    })
    vi.stubGlobal("fetch", fetchMock)
    const config = await new ProviderConfigClient("https://luna-api.internal", "callback-token-value").get()
    expect(config.version).toBe("cfg-1")
    expect(config.runtime).toEqual({ providerTimeoutMs: 45_000, runTimeoutMs: 420_000, agentConcurrentRuns: 3 })
    expect(fetchMock).toHaveBeenCalledOnce()
  })

  it("rejects runtime settings outside the platform contract", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      version: "cfg-unsafe",
      provider: { baseUrl: "", model: "", apiKey: "", configured: false },
      runtime: { providerTimeoutMs: 0, runTimeoutMs: 1_000, agentConcurrentRuns: 100 },
    }), { status: 200 })))
    await expect(new ProviderConfigClient("https://luna-api.internal", "callback-token-value").get())
      .rejects.toThrow()
  })
})
