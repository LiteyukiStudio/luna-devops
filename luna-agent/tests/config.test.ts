import { describe, expect, it } from "vitest"
import { loadConfig } from "../src/config.js"
import { DiagnosticError } from "../src/diagnostic-error.js"
import { errorDiagnostic } from "../src/telemetry.js"

describe("configuration", () => {
  it("rejects unsafe production defaults", () => {
    expect(() => loadConfig({ NODE_ENV: "production" })).toThrow("DATABASE_URL")
  })
  it("allows unit tests to construct configuration without starting the managed runtime", () => {
    const config = loadConfig({ NODE_ENV: "test" })
    expect(config.AI_OBSERVABILITY_CAPTURE_CONTENT).toBe(false)
    expect(config).toMatchObject({
      AI_CONTEXT_COMPRESSION_TRIGGER_RATIO: 0.9,
      AI_CONTEXT_RECENT_TURN_COUNT: 16,
      AI_CONTEXT_MAX_HISTORY_PAYLOAD_K_BYTES: 4096,
      AI_CONTEXT_MAX_SUMMARY_PAYLOAD_K_BYTES: 512,
      AI_CONTEXT_MAX_CONTINUATION_PAYLOAD_K_BYTES: 1024,
      AI_TOOLS_RESULT_PAYLOAD_K_BYTES: 512,
    })
  })
  it("accepts valid Agent-local context strategy overrides", () => {
    expect(loadConfig({
      NODE_ENV: "test",
      AI_CONTEXT_COMPRESSION_TRIGGER_RATIO: "0.85",
      AI_CONTEXT_RECENT_TURN_COUNT: "12",
      AI_CONTEXT_MAX_HISTORY_PAYLOAD_K_BYTES: "2048",
      AI_CONTEXT_MAX_SUMMARY_PAYLOAD_K_BYTES: "256",
      AI_CONTEXT_MAX_CONTINUATION_PAYLOAD_K_BYTES: "512",
      AI_TOOLS_RESULT_PAYLOAD_K_BYTES: "256",
    })).toMatchObject({
      AI_CONTEXT_COMPRESSION_TRIGGER_RATIO: 0.85,
      AI_CONTEXT_RECENT_TURN_COUNT: 12,
      AI_CONTEXT_MAX_HISTORY_PAYLOAD_K_BYTES: 2048,
      AI_CONTEXT_MAX_SUMMARY_PAYLOAD_K_BYTES: 256,
      AI_CONTEXT_MAX_CONTINUATION_PAYLOAD_K_BYTES: 512,
      AI_TOOLS_RESULT_PAYLOAD_K_BYTES: 256,
    })
  })
  it("rejects invalid Agent-local byte limits", () => {
    expect(() => loadConfig({
      NODE_ENV: "test",
      AI_CONTEXT_MAX_HISTORY_PAYLOAD_K_BYTES: "32",
    })).toThrow()
    expect(() => loadConfig({
      NODE_ENV: "test",
      AI_CONTEXT_MAX_SUMMARY_PAYLOAD_K_BYTES: "8192",
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
  it("reports a stable diagnostic when production Redis configuration is missing", () => {
    const input = {
      NODE_ENV: "production",
      DATABASE_URL: "postgres://localhost/luna",
      AUTH_MODE: "bff-hmac",
      AI_INTERNAL_SECRET: "x".repeat(32),
      LUNA_API_BASE_URL: "http://localhost:8080",
    } as const

    let startupError: unknown
    try {
      loadConfig(input)
    }
    catch (error) {
      startupError = error
    }

    expect(startupError).toBeInstanceOf(DiagnosticError)
    expect(errorDiagnostic(startupError, "agent.startup.failed", "generic startup hint")).toMatchObject({
      "error.code": "ai.stream_redis_url_required",
      "error.message": "Production streaming requires REDIS_ADDR",
      "error.hint": "configure REDIS_ADDR with a valid Redis connection URI in the Agent deployment and redeploy",
    })
    expect(errorDiagnostic(
      new Error("Agent bootstrap failed", { cause: startupError }),
      "agent.startup.failed",
      "generic startup hint",
    )).toMatchObject({
      "error.code": "ai.stream_redis_url_required",
      "error.hint": "configure REDIS_ADDR with a valid Redis connection URI in the Agent deployment and redeploy",
    })
    expect(() => loadConfig({ ...input, REDIS_ADDR: " " })).toThrow("Production streaming requires REDIS_ADDR")
    expect(loadConfig({ ...input, REDIS_ADDR: "redis://localhost:6379/0" }).REDIS_ADDR)
      .toBe("redis://localhost:6379/0")
  })
})
