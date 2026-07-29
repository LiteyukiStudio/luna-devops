import { afterEach, describe, expect, it, vi } from "vitest"
import { ProviderConfigClient } from "../src/provider/config-client.js"

afterEach(() => vi.unstubAllGlobals())

describe("ProviderConfigClient", () => {
  it("uses only the callback service identity and parses no-store configuration", async () => {
    const fetchMock = vi.fn(async (_url: URL, init: RequestInit) => {
      expect(init.headers).toMatchObject({ authorization: "Bearer callback-token-value" })
      return new Response(JSON.stringify({
        version: "cfg-1",
        provider: { type: "openai-compatible", baseUrl: "https://provider.example/v1/", defaultModel: "model-a", modelPricing: [], apiKey: "secret", configured: true },
      }), { status: 200, headers: { "content-type": "application/json", "cache-control": "no-store" } })
    })
    vi.stubGlobal("fetch", fetchMock)
    const config = await new ProviderConfigClient("https://luna-api.internal", "callback-token-value").get()
    expect(config.version).toBe("cfg-1")
    expect(fetchMock).toHaveBeenCalledOnce()
  })
})
