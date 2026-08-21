import { describe, expect, it } from "vitest"
import { loadConfig } from "../src/config.js"

describe("configuration", () => {
  it("rejects unsafe production defaults", () => {
    expect(() => loadConfig({ NODE_ENV: "production" })).toThrow("DATABASE_URL")
  })
  it("allows unit tests to construct configuration without starting the managed runtime", () => {
    const config = loadConfig({ NODE_ENV: "test" })
    expect(config.AI_OBSERVABILITY_CAPTURE_CONTENT).toBe(false)
    expect(config).toMatchObject({
      AI_CONTEXT_COMPRESSION_TRIGGER_RATIO: 0.9,
      AI_CONTEXT_COMPRESSION_TARGET_RATIO: 0.7,
      AI_CONTEXT_RECENT_TURN_COUNT: 16,
      AI_CONTEXT_MAX_RECENT_TURN_COUNT: 32,
      AI_CONTEXT_HISTORICAL_TOOL_K_TOKENS: 64,
      AI_TOOLS_RESULT_PAYLOAD_K_BYTES: 512,
    })
  })
  it("accepts valid Agent-local context strategy overrides", () => {
    expect(loadConfig({
      NODE_ENV: "test",
      AI_CONTEXT_COMPRESSION_TRIGGER_RATIO: "0.85",
      AI_CONTEXT_COMPRESSION_TARGET_RATIO: "0.6",
      AI_CONTEXT_RECENT_TURN_COUNT: "12",
      AI_CONTEXT_MAX_RECENT_TURN_COUNT: "24",
      AI_CONTEXT_HISTORICAL_TOOL_K_TOKENS: "32",
      AI_TOOLS_RESULT_PAYLOAD_K_BYTES: "256",
    })).toMatchObject({
      AI_CONTEXT_COMPRESSION_TRIGGER_RATIO: 0.85,
      AI_CONTEXT_COMPRESSION_TARGET_RATIO: 0.6,
      AI_CONTEXT_RECENT_TURN_COUNT: 12,
      AI_CONTEXT_MAX_RECENT_TURN_COUNT: 24,
      AI_CONTEXT_HISTORICAL_TOOL_K_TOKENS: 32,
      AI_TOOLS_RESULT_PAYLOAD_K_BYTES: 256,
    })
  })
  it("rejects inconsistent Agent-local context strategy overrides", () => {
    expect(() => loadConfig({
      NODE_ENV: "test",
      AI_CONTEXT_COMPRESSION_TRIGGER_RATIO: "0.6",
      AI_CONTEXT_COMPRESSION_TARGET_RATIO: "0.7",
    })).toThrow()
    expect(() => loadConfig({
      NODE_ENV: "test",
      AI_CONTEXT_RECENT_TURN_COUNT: "24",
      AI_CONTEXT_MAX_RECENT_TURN_COUNT: "12",
    })).toThrow()
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
