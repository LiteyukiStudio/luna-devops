import { readFileSync, readdirSync } from "node:fs"
import { afterEach, describe, expect, it, vi } from "vitest"
import { ZodError } from "zod"
import { loadConfig, loadProcessRuntimeStartupConfig, loadRuntimeConfig, loadRuntimeStartupConfig, loadTelemetryConfig } from "../src/config.js"

function rootEnvExample(): NodeJS.ProcessEnv {
  const values: NodeJS.ProcessEnv = {}
  for (const sourceLine of readFileSync(new URL("../../.env.example", import.meta.url), "utf8").split("\n")) {
    const line = sourceLine.trim()
    if (!line || line.startsWith("#")) continue
    const separator = line.indexOf("=")
    if (separator > 0) values[line.slice(0, separator)] = line.slice(separator + 1)
  }
  return values
}

function sourceTypeScriptFiles(directory: URL): URL[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const child = new URL(`${entry.name}${entry.isDirectory() ? "/" : ""}`, directory)
    return entry.isDirectory() ? sourceTypeScriptFiles(child) : entry.name.endsWith(".ts") ? [child] : []
  })
}

afterEach(() => vi.unstubAllEnvs())

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
    })).toThrow("LUNA_API_BASE_URL")
  })
  it("validates every dependency required by the real startup path", () => {
    vi.stubEnv("NODE_ENV", "development")
    vi.stubEnv("DATABASE_URL", "")
    vi.stubEnv("AI_INTERNAL_SECRET", "")
    vi.stubEnv("LUNA_API_BASE_URL", "")
    expect(() => loadRuntimeConfig()).toThrow("DATABASE_URL")

    vi.stubEnv("DATABASE_URL", "postgres://localhost/luna")
    expect(() => loadRuntimeConfig()).toThrow("AI_INTERNAL_SECRET")

    vi.stubEnv("AI_INTERNAL_SECRET", "x".repeat(32))
    expect(() => loadRuntimeConfig()).toThrow("LUNA_API_BASE_URL")
  })
  it("keeps validated telemetry available when business startup configuration fails", () => {
    const startup = loadRuntimeStartupConfig({
      NODE_ENV: "production",
      LOG_LEVEL: "warn",
      OTEL_EXPORTER_OTLP_ENDPOINT: "https://otel.internal:4318",
    })
    expect(startup.ok).toBe(false)
    expect(startup.telemetry).toMatchObject({
      LOG_LEVEL: "warn",
      OTEL_EXPORTER_OTLP_ENDPOINT: "https://otel.internal:4318",
    })
  })
  it("removes every implicit OTel SDK channel after taking the process snapshot", () => {
    const originalOTEL = Object.fromEntries(Object.entries(process.env).filter(([name]) => name.startsWith("OTEL_")))
    const completeRuntime = {
      NODE_ENV: "production",
      DATABASE_URL: "postgres://localhost/luna",
      AUTH_MODE: "bff-hmac",
      AI_INTERNAL_SECRET: "x".repeat(32),
      LUNA_API_BASE_URL: "http://localhost:8080",
      OTEL_EXPORTER_OTLP_ENDPOINT: "https://otel.internal:4318",
      OTEL_EXPORTER_OTLP_HEADERS: "snapshot=expected",
      OTEL_SERVICE_VERSION: "snapshot-version",
    }
    const implicitNames = [
      "OTEL_EXPORTER_OTLP_TRACES_HEADERS",
      "OTEL_EXPORTER_OTLP_METRICS_TIMEOUT",
      "OTEL_EXPORTER_OTLP_LOGS_COMPRESSION",
      "OTEL_EXPORTER_OTLP_CERTIFICATE",
      "OTEL_SDK_DISABLED",
      "OTEL_PROPAGATORS",
      "OTEL_TRACES_SAMPLER",
      "OTEL_BSP_MAX_QUEUE_SIZE",
      "OTEL_METRIC_EXPORT_INTERVAL",
    ]
    try {
      for (const [name, value] of Object.entries(completeRuntime)) vi.stubEnv(name, value)
      for (const name of implicitNames) vi.stubEnv(name, "must-not-be-read")

      const startup = loadProcessRuntimeStartupConfig()
      expect(startup.ok).toBe(true)
      expect(startup.telemetry).toMatchObject({
        OTEL_EXPORTER_OTLP_ENDPOINT: completeRuntime.OTEL_EXPORTER_OTLP_ENDPOINT,
        OTEL_EXPORTER_OTLP_HEADERS: completeRuntime.OTEL_EXPORTER_OTLP_HEADERS,
        OTEL_SERVICE_VERSION: completeRuntime.OTEL_SERVICE_VERSION,
      })
      expect(Object.keys(process.env).filter(name => name.startsWith("OTEL_"))).toEqual([])
    }
    finally {
      for (const name of Object.keys(process.env)) {
        if (name.startsWith("OTEL_")) delete process.env[name]
      }
      for (const [name, value] of Object.entries(originalOTEL)) process.env[name] = value
    }
  })
  it("loads a complete local Agent runtime from the root environment example", () => {
    for (const [name, value] of Object.entries(rootEnvExample())) vi.stubEnv(name, value ?? "")
    vi.stubEnv("NODE_ENV", "development")
    vi.stubEnv("AI_INTERNAL_SECRET", "x".repeat(32))

    const config = loadRuntimeConfig()
    expect(config.AUTH_MODE).toBe("bff-hmac")
    expect(config.LUNA_API_BASE_URL).toBe("http://localhost:8080")
    expect(config.DATABASE_URL).toContain("localhost:5432")
  })
  it("accepts only HTTP(S) Luna API and OpenTelemetry endpoints", () => {
    expect(() => loadConfig({ NODE_ENV: "test", LUNA_API_BASE_URL: "not-a-url" })).toThrow(ZodError)
    expect(() => loadConfig({ NODE_ENV: "test", LUNA_API_BASE_URL: "ftp://luna-api.internal" })).toThrow("http or https")
    expect(() => loadConfig({ NODE_ENV: "test", LUNA_API_BASE_URL: "http://user:secret@luna-api.internal" })).toThrow("credentials")
    expect(() => loadConfig({ NODE_ENV: "test", LUNA_API_BASE_URL: "https://luna-api.internal/base" })).toThrow("without a path")
    expect(() => loadTelemetryConfig({ OTEL_EXPORTER_OTLP_ENDPOINT: "https://collector.internal?token=secret" })).toThrow("query parameters")
    expect(() => loadTelemetryConfig({ OTEL_EXPORTER_OTLP_ENDPOINT: "https://collector.internal/#secret" })).toThrow("fragments")
    expect(() => loadTelemetryConfig({ OTEL_EXPORTER_OTLP_ENDPOINT: "grpc://otel.internal:4317" })).toThrow("http or https")
    expect(loadConfig({ NODE_ENV: "test", LUNA_API_BASE_URL: "https://luna-api.internal" }).LUNA_API_BASE_URL).toBe("https://luna-api.internal")
    expect(loadTelemetryConfig({ OTEL_EXPORTER_OTLP_ENDPOINT: "http://otel.internal:4318" }).OTEL_EXPORTER_OTLP_ENDPOINT).toBe("http://otel.internal:4318")
  })
  it("validates PostgreSQL URLs and rejects line breaks in OTLP key-value lists", () => {
    expect(() => loadConfig({ NODE_ENV: "test", DATABASE_URL: "mysql://db/app", AI_INTERNAL_SECRET: "a".repeat(32) })).toThrow("postgres")
    expect(() => loadTelemetryConfig({ OTEL_EXPORTER_OTLP_HEADERS: "authorization=secret%0D%0Ax-leak:value" })).toThrow("key=value")
    expect(() => loadRuntimeConfig({
      NODE_ENV: "development",
      DATABASE_URL: "postgres://localhost/luna",
      AI_INTERNAL_SECRET: `x${" ".repeat(31)}`,
      LUNA_API_BASE_URL: "http://localhost:8080",
    })).toThrow()
    expect(loadRuntimeConfig({
      NODE_ENV: "development",
      DATABASE_URL: "postgres://localhost/luna",
      AI_INTERNAL_SECRET: "猫".repeat(11),
      LUNA_API_BASE_URL: "http://localhost:8080",
    }).AI_INTERNAL_SECRET).toBe("猫".repeat(11))
  })
  it("keeps deployment environment access inside the configuration adapter", () => {
    const violations: string[] = []
    for (const sourceFile of sourceTypeScriptFiles(new URL("../src/", import.meta.url))) {
      if (sourceFile.pathname.endsWith("/config.ts")) continue
      readFileSync(sourceFile, "utf8").split("\n").forEach((line, index) => {
        const runtimeDetection = sourceFile.pathname.endsWith("/telemetry.ts")
          && line.includes("process.env.KUBERNETES_SERVICE_HOST")
          && line.includes("process.env.container")
        if (!runtimeDetection && (line.includes("process.env") || line.includes("Bun.env") || line.includes("Deno.env"))) {
          violations.push(`${sourceFile.pathname}:${index + 1}`)
        }
      })
    }
    expect(violations).toEqual([])
  })
})
