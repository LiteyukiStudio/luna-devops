import { describe, expect, it } from "vitest"
import { loadConfig } from "../src/config.js"
import { ManagedProvider } from "../src/provider/managed.js"
import { ProviderConfigClient } from "../src/provider/config-client.js"
import { createRuntimeProvider } from "../src/provider/runtime.js"

describe("runtime Provider selection", () => {
  it("uses only the platform-managed configuration", () => {
    const config = loadConfig({ NODE_ENV: "development" })
    const managed = new ProviderConfigClient("https://luna-api.internal", "callback-token-value")
    expect(createRuntimeProvider(config, managed)).toBeInstanceOf(ManagedProvider)
  })

  it("fails closed when the managed configuration client is absent", () => {
    expect(() => createRuntimeProvider(loadConfig({ NODE_ENV: "test" }))).toThrow("ai.provider_config_required")
  })
})
