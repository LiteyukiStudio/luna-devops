import { describe, expect, it } from "vitest"
import { loadConfig } from "../src/config.js"
import { DeterministicProvider } from "../src/provider/deterministic.js"
import { ManagedProvider } from "../src/provider/managed.js"
import { OpenAICompatibleProvider } from "../src/provider/openai-compatible.js"
import { ProviderConfigClient } from "../src/provider/config-client.js"
import { createRuntimeProvider } from "../src/provider/runtime.js"

describe("runtime Provider selection", () => {
  it("always prefers the platform-managed configuration", () => {
    const config = loadConfig({
      NODE_ENV: "development",
      PROVIDER_BASE_URL: "https://direct.example/v1",
      PROVIDER_API_KEY: "direct-secret",
      PROVIDER_MODEL: "direct-model",
    })
    const managed = new ProviderConfigClient("https://luna-api.internal", "callback-token-value")
    expect(createRuntimeProvider(config, managed)).toBeInstanceOf(ManagedProvider)
  })

  it("uses the three direct values for standalone development", () => {
    const config = loadConfig({
      NODE_ENV: "development",
      PROVIDER_BASE_URL: "https://direct.example/v1",
      PROVIDER_API_KEY: "direct-secret",
      PROVIDER_MODEL: "direct-model",
    })
    expect(createRuntimeProvider(config)).toBeInstanceOf(OpenAICompatibleProvider)
  })

  it("keeps the deterministic Provider only for an unconfigured non-production process", () => {
    expect(createRuntimeProvider(loadConfig({ NODE_ENV: "test" }))).toBeInstanceOf(DeterministicProvider)
  })
})
