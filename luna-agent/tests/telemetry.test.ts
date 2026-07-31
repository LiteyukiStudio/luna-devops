import { describe, expect, it } from "vitest"
import { initializeTelemetry, sanitizeTelemetryURL, stableErrorCode } from "../src/telemetry.js"

describe("agent telemetry", () => {
  it("removes credentials, query strings, and fragments from telemetry URLs", () => {
    expect(sanitizeTelemetryURL("https://user:password@example.com/path?token=secret#fragment"))
      .toBe("https://example.com/path")
  })

  it("stays disabled when no OTLP endpoint is configured", () => {
    expect(() => initializeTelemetry(undefined)).not.toThrow()
  })

  it("rejects unsupported OTLP endpoint protocols", () => {
    expect(() => initializeTelemetry("file:///tmp/telemetry")).toThrow(/http or https/)
  })

  it("only exposes stable error codes", () => {
    expect(stableErrorCode(new Error("ai.provider_timeout"))).toBe("ai.provider_timeout")
    expect(stableErrorCode(new Error("request contained secret abc"))).toBe("ai.internal_error")
  })
})
