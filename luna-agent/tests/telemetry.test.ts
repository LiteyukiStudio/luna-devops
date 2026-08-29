import { describe, expect, it, vi } from "vitest"
import { Writable } from "node:stream"
import { SpanStatusCode, type Span } from "@opentelemetry/api"
import { captureTraceContext, createAgentLogger, errorDiagnostic, genAIClientTokenUsageMetric, initializeTelemetry, internalSpanOptions, isDatabaseSpanCaptureEnabled, isExpectedCancellation, isHealthCheckPath, normalizeTraceContext, recordSpanError, resolveAgentLogColor, resolveAgentLogFormat, resolveAgentLogLevel, sanitizeTelemetryURL, stableErrorCode, stableFastifyLifecycleSpanName, telemetryLog, withSpan } from "../src/telemetry.js"
import { loadTelemetryConfig } from "../src/config.js"

describe("agent telemetry", () => {
  it("keeps noisy database spans opt-in", () => {
    expect(isDatabaseSpanCaptureEnabled()).toBe(false)
    expect(isDatabaseSpanCaptureEnabled(false)).toBe(false)
    expect(isDatabaseSpanCaptureEnabled(true)).toBe(true)
  })

  it("removes credentials, query strings, and fragments from telemetry URLs", () => {
    expect(sanitizeTelemetryURL("https://user:password@example.com/path?token=secret#fragment"))
      .toBe("https://example.com/path")
  })

  it("stays disabled when no OTLP endpoint is configured", () => {
    expect(() => initializeTelemetry(loadTelemetryConfig({}))).not.toThrow()
  })

  it("uses the official GenAI client token usage histogram contract", () => {
    expect(genAIClientTokenUsageMetric).toMatchObject({
      name: "gen_ai.client.token.usage",
      description: "Number of input and output tokens used.",
      unit: "{token}",
    })
    expect(genAIClientTokenUsageMetric.explicitBucketBoundaries).toEqual([
      1, 4, 16, 64, 256, 1_024, 4_096, 16_384, 65_536, 262_144, 1_048_576, 4_194_304, 16_777_216, 67_108_864,
    ])
  })

  it("rejects unsupported OTLP endpoint protocols", () => {
    expect(() => initializeTelemetry(loadTelemetryConfig({ OTEL_EXPORTER_OTLP_ENDPOINT: "file:///tmp/telemetry" }))).toThrow(/http or https/)
  })

  it("only exposes stable error codes", () => {
    expect(stableErrorCode(new Error("ai.provider_timeout"))).toBe("ai.provider_timeout")
    expect(stableErrorCode(new Error("request contained secret abc"))).toBe("ai.internal_error")
  })

  it("matches Go log format, color, and level resolution", () => {
    expect(resolveAgentLogFormat("console", false, true)).toBe("console")
    expect(resolveAgentLogFormat("json", true, false)).toBe("json")
    expect(resolveAgentLogFormat("auto", true, false)).toBe("console")
    expect(resolveAgentLogFormat(undefined, false, false)).toBe("json")
    expect(resolveAgentLogFormat("auto", true, true)).toBe("json")
    expect(resolveAgentLogColor("always", false, false)).toBe(true)
    expect(resolveAgentLogColor("always", true, true)).toBe(false)
    expect(resolveAgentLogLevel("debug")).toBe("debug")
    expect(resolveAgentLogLevel("invalid")).toBe("info")
  })

  it("keeps JSON output ANSI-free, structured, redacted, and level-filtered", () => {
    let output = ""
    const destination = new Writable({
      write(chunk, _encoding, callback) {
        output += String(chunk)
        callback()
      },
    })
    const logger = createAgentLogger({
      color: "always",
      destination,
      format: "json",
      isTTY: true,
      level: "warn",
    })
    logger.info({ "event.name": "agent.hidden" }, "hidden")
    logger.error({
      "event.name": "agent.start_failed",
      ...errorDiagnostic(
        new Error("connect PostgreSQL at postgres.internal:5432; token=must-not-leak"),
        "agent.startup.failed",
      ),
    }, "Agent startup failed")
    expect(output).not.toContain("hidden")
    expect(output).not.toContain("\u001B")
    expect(output).not.toContain("must-not-leak")
    expect(output).toContain("postgres.internal:5432")
    expect(output).toContain("[REDACTED]")
    expect(JSON.parse(output)).toMatchObject({
      "event.name": "agent.start_failed",
      "error.code": "agent.startup.failed",
      "error.type": "Error",
    })
  })

  it("renders readable colored console output without changing structured fields", async () => {
    const noColor = process.env.NO_COLOR
    delete process.env.NO_COLOR
    let output = ""
    const destination = new Writable({
      write(chunk, _encoding, callback) {
        output += String(chunk)
        callback()
      },
    })
    try {
      const logger = createAgentLogger({
        color: "always",
        destination,
        format: "console",
        isTTY: true,
        level: "debug",
      })
      logger.error({
        "event.name": "agent.start_failed",
        "operation": "agent.startup",
        "outcome": "failed",
        ...errorDiagnostic(new Error("dial tcp postgres.internal:5432: connection refused"), "agent.startup.failed"),
      }, "Agent startup failed\nretry later")
      await new Promise(resolve => setImmediate(resolve))

      expect(output).toContain("Agent startup failed")
      expect(output).toContain("\\nretry later")
      expect(output).toContain("agent.startup.failed")
      expect(output).toContain("postgres.internal:5432")
      expect(output).toContain("\u001B[")
      expect(output.trimEnd().split("\n")).toHaveLength(1)
    }
    finally {
      if (noColor === undefined) delete process.env.NO_COLOR
      else process.env.NO_COLOR = noColor
    }
  })

  it("keeps the complete cause chain and runtime stack while redacting credentials", () => {
    const cause = new Error("dial tcp postgres.internal:5432: connection refused; password=must-not-leak")
    const diagnostic = errorDiagnostic(new Error("dependency.postgres.unavailable", { cause }), "agent.startup.failed")
    expect(diagnostic["error.message"]).toContain("dependency.postgres.unavailable: dial tcp postgres.internal:5432")
    expect(diagnostic["error.message"]).toContain("[REDACTED]")
    expect(diagnostic["error.message"]).not.toContain("must-not-leak")
    expect(diagnostic["exception.stacktrace"]).toContain("dependency.postgres.unavailable")
  })

  it("marks failed spans with a stable code without recording sensitive error text", () => {
    const setStatus = vi.fn()
    const setAttribute = vi.fn()
    recordSpanError({ setStatus, setAttribute } as unknown as Span, new Error("provider leaked secret sk-test-value"))
    expect(setStatus).toHaveBeenCalledWith({ code: SpanStatusCode.ERROR, message: "ai.internal_error" })
    expect(setAttribute).toHaveBeenCalledWith("error.code", "ai.internal_error")
    expect(JSON.stringify([setStatus.mock.calls, setAttribute.mock.calls])).not.toContain("sk-test-value")
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

  it("falls back to the inbound W3C header when telemetry is disabled locally", () => {
    expect(captureTraceContext({
      traceparent: "00-abcdefabcdefabcdefabcdefabcdefab-0123456789abcdef-01",
      authorization: "Bearer secret",
    })).toEqual({
      traceparent: "00-abcdefabcdefabcdefabcdefabcdefab-0123456789abcdef-01",
    })
  })

  it("classifies only machine health probes as telemetry noise", () => {
    expect(isHealthCheckPath("/healthz")).toBe(true)
    expect(isHealthCheckPath("http://agent:8091/internal/health/ready?probe=1")).toBe(true)
    expect(isHealthCheckPath("/internal/health/live")).toBe(true)
    expect(isHealthCheckPath("/api/v1/registries/reg_1/test")).toBe(false)
  })

  it("keeps Fastify handler span names bounded to method and route templates", () => {
    expect(stableFastifyLifecycleSpanName("handler", "POST", "/internal/v1/conversations/:conversationId/turns?ignored=true"))
      .toBe("fastify.handler POST /internal/v1/conversations/:conversationId/turns")
    expect(stableFastifyLifecycleSpanName("async request => userInput", "post", "/route"))
      .toBe("fastify.handler UNKNOWN /route")
  })

  it("correlates nested logs with the active AI conversation, turn, and run", async () => {
    const write = vi.spyOn(process.stderr, "write").mockImplementation(() => true)
    try {
      await withSpan("invoke_agent Luna Agent", internalSpanOptions({
        "gen_ai.operation.name": "invoke_agent",
        "gen_ai.conversation.id": "aicnv_test",
        "luna.turn.id": "aitrn_test",
        "luna.run.id": "airun_test",
      }), async () => {
        await withSpan("agent.response.process", internalSpanOptions(), async () => {
          telemetryLog("agent.model.completed", "info")
        })
      })

      const raw = write.mock.calls.at(-1)?.[0]
      expect(typeof raw).toBe("string")
      expect(JSON.parse(String(raw))).toMatchObject({
        "gen_ai.conversation.id": "aicnv_test",
        "luna.turn.id": "aitrn_test",
        "luna.run.id": "airun_test",
        "resource.type": "agent_run",
        "resource.id": "airun_test",
      })
    }
    finally {
      write.mockRestore()
    }
  })
})
