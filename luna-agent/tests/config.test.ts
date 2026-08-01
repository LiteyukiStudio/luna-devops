import { describe, expect, it } from "vitest"
import { loadConfig } from "../src/config.js"

describe("configuration", () => {
  it("rejects unsafe production defaults", () => {
    expect(() => loadConfig({ NODE_ENV: "production" })).toThrow("DATABASE_URL")
  })
  it("allows tests without a model configuration", () => {
    const config = loadConfig({ NODE_ENV: "test" })
    expect(config.PROVIDER_BASE_URL).toBeUndefined()
    expect(config.PROVIDER_API_KEY).toBeUndefined()
    expect(config.PROVIDER_MODEL).toBeUndefined()
    expect(config.AI_OBSERVABILITY_CAPTURE_CONTENT).toBe(false)
  })
  it("enables sensitive AI content observability only when explicitly requested", () => {
    expect(loadConfig({ NODE_ENV: "test", AI_OBSERVABILITY_CAPTURE_CONTENT: "true" }).AI_OBSERVABILITY_CAPTURE_CONTENT).toBe(true)
    expect(() => loadConfig({ NODE_ENV: "test", AI_OBSERVABILITY_CAPTURE_CONTENT: "sometimes" })).toThrow()
  })
  it("treats empty optional compose values as unset", () => {
    const config = loadConfig({
      NODE_ENV: "test",
      PROVIDER_BASE_URL: "",
      PROVIDER_API_KEY: "   ",
      PROVIDER_MODEL: "",
      LUNA_API_BASE_URL: "",
    })
    expect(config.PROVIDER_BASE_URL).toBeUndefined()
    expect(config.PROVIDER_API_KEY).toBeUndefined()
    expect(config.PROVIDER_MODEL).toBeUndefined()
    expect(config.LUNA_API_BASE_URL).toBeUndefined()
  })
  it("requires all three direct provider values together", () => {
    expect(() => loadConfig({ NODE_ENV: "development", PROVIDER_BASE_URL: "https://api.example.com/v1" }))
      .toThrow("base URL, API key, and model")
    expect(loadConfig({
      NODE_ENV: "development",
      PROVIDER_BASE_URL: "https://api.example.com/v1",
      PROVIDER_API_KEY: "secret",
      PROVIDER_MODEL: "model-1",
    }).PROVIDER_MODEL).toBe("model-1")
  })
})
