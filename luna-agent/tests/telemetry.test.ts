import { describe, expect, it } from "vitest"
import { initializeTelemetry, isHealthCheckPath, normalizeTraceContext, sanitizeTelemetryURL, stableErrorCode } from "../src/telemetry.js"

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

  it("persists only bounded W3C trace context fields", () => {
    expect(normalizeTraceContext({
      TraceParent: " 00-0123456789abcdef0123456789abcdef-0123456789abcdef-01 ",
      tracestate: "vendor=value",
      baggage: "user-email=secret@example.com",
      authorization: "Bearer secret",
    })).toEqual({
      traceparent: "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01",
      tracestate: "vendor=value",
    })
  })

  it("classifies only machine health probes as telemetry noise", () => {
    expect(isHealthCheckPath("/healthz")).toBe(true)
    expect(isHealthCheckPath("http://agent:8091/internal/health/ready?probe=1")).toBe(true)
    expect(isHealthCheckPath("/internal/health/live")).toBe(true)
    expect(isHealthCheckPath("/internal/v1/provider/health")).toBe(false)
    expect(isHealthCheckPath("/api/v1/registries/reg_1/test")).toBe(false)
  })
})
