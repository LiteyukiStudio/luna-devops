import { describe, expect, it, vi } from "vitest"
import { initializeTelemetry, internalSpanOptions, isDatabaseSpanCaptureEnabled, isExpectedCancellation, isHealthCheckPath, normalizeTraceContext, sanitizeTelemetryURL, stableErrorCode, telemetryLog, withSpan } from "../src/telemetry.js"

describe("agent telemetry", () => {
  it("keeps noisy database spans opt-in", () => {
    expect(isDatabaseSpanCaptureEnabled(undefined)).toBe(false)
    expect(isDatabaseSpanCaptureEnabled("false")).toBe(false)
    expect(isDatabaseSpanCaptureEnabled("TRUE")).toBe(true)
  })

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

  it("separates expected cancellation from operational failures", () => {
    expect(isExpectedCancellation(new Error("ai.run_canceled"))).toBe(true)
    expect(isExpectedCancellation(new Error("ai.agent_stopping"))).toBe(true)
    expect(isExpectedCancellation(new Error("ai.run_timeout"))).toBe(false)
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

  it("correlates nested logs with the active AI conversation, turn, and run", async () => {
    const write = vi.spyOn(process.stdout, "write").mockImplementation(() => true)
    try {
      await withSpan("agent.run.execute", internalSpanOptions({
        "gen_ai.conversation.id": "aicnv_test",
        "luna.turn.id": "aitrn_test",
        "luna.run.id": "airun_test",
      }), async () => {
        await withSpan("agent.model.stream", internalSpanOptions(), async () => {
          telemetryLog("agent.model.completed", "info")
        })
      })

      const raw = write.mock.calls.at(-1)?.[0]
      expect(typeof raw).toBe("string")
      expect(JSON.parse(String(raw))).toMatchObject({
        "gen_ai.conversation.id": "aicnv_test",
        "luna.turn.id": "aitrn_test",
        "luna.run.id": "airun_test",
      })
    }
    finally {
      write.mockRestore()
    }
  })
})
