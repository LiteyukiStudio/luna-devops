import { describe, expect, it } from "vitest"
import { loadConfig } from "../src/config.js"

describe("configuration", () => {
  it("rejects unsafe production defaults", () => {
    expect(() => loadConfig({ NODE_ENV: "production" })).toThrow("DATABASE_URL")
  })
  it("allows unit tests to construct configuration without starting the managed runtime", () => {
    const config = loadConfig({ NODE_ENV: "test" })
    expect(config.AI_OBSERVABILITY_CAPTURE_CONTENT).toBe(false)
  })
  it("enables sensitive AI content observability only when explicitly requested", () => {
    expect(loadConfig({ NODE_ENV: "test", AI_OBSERVABILITY_CAPTURE_CONTENT: "true" }).AI_OBSERVABILITY_CAPTURE_CONTENT).toBe(true)
    expect(() => loadConfig({ NODE_ENV: "test", AI_OBSERVABILITY_CAPTURE_CONTENT: "sometimes" })).toThrow()
  })
  it("treats empty optional compose values as unset", () => {
    const config = loadConfig({
      NODE_ENV: "test",
      LUNA_API_BASE_URL: "",
    })
    expect(config.LUNA_API_BASE_URL).toBeUndefined()
  })
  it("requires the Luna API configuration path in production", () => {
    expect(() => loadConfig({
      NODE_ENV: "production",
      DATABASE_URL: "postgres://localhost/luna",
      AUTH_MODE: "bff-hmac",
      AI_INTERNAL_SECRET: "x".repeat(32),
    })).toThrow("Luna API")
  })
})
