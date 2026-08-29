import { describe, expect, it } from "vitest"
import { loadConfig, loadTelemetryConfig } from "../src/config.js"

describe("configuration", () => {
  it("rejects unsafe production defaults", () => {
    expect(() => loadConfig({ NODE_ENV: "production" })).toThrow("DATABASE_URL")
  })
  it("loads early telemetry settings without bypassing later runtime validation", () => {
    expect(loadTelemetryConfig({ NODE_ENV: "production", LOG_LEVEL: "warn" }).LOG_LEVEL).toBe("warn")
    expect(() => loadConfig({ NODE_ENV: "production", LOG_LEVEL: "warn" })).toThrow("DATABASE_URL")
  })
  it("allows unit tests to construct configuration without starting the managed runtime", () => {
    const config = loadConfig({ NODE_ENV: "test" })
    expect(config.AI_OBSERVABILITY_CAPTURE_CONTENT).toBe(false)
    expect(config.AI_OBSERVABILITY_CAPTURE_DATABASE_SPANS).toBe(false)
  })
  it("enables sensitive AI content observability only when explicitly requested", () => {
    expect(loadConfig({ NODE_ENV: "test", AI_OBSERVABILITY_CAPTURE_CONTENT: "true" }).AI_OBSERVABILITY_CAPTURE_CONTENT).toBe(true)
    expect(() => loadConfig({ NODE_ENV: "test", AI_OBSERVABILITY_CAPTURE_CONTENT: "sometimes" })).toThrow()
  })
  it("validates Agent logging and OpenTelemetry environment fields centrally", () => {
    const config = loadConfig({
      NODE_ENV: "test",
      NO_COLOR: "",
      OTEL_EXPORTER_OTLP_HEADERS: "authorization=Bearer%20token",
      OTEL_RESOURCE_ATTRIBUTES: "deployment.environment.name=test",
      OTEL_SERVICE_VERSION: "version-1",
      AI_OBSERVABILITY_CAPTURE_DATABASE_SPANS: "true",
    })
    expect(config.NO_COLOR).toBe("")
    expect(config.OTEL_SERVICE_VERSION).toBe("version-1")
    expect(config.AI_OBSERVABILITY_CAPTURE_DATABASE_SPANS).toBe(true)
    expect(() => loadConfig({ NODE_ENV: "test", AI_OBSERVABILITY_CAPTURE_DATABASE_SPANS: "sometimes" })).toThrow()
    expect(() => loadConfig({ NODE_ENV: "test", OTEL_RESOURCE_ATTRIBUTES: "missing-value" })).toThrow()
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
