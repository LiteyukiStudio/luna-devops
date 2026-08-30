import { Buffer } from "node:buffer"
import { z } from "zod"

function optionalValue<T extends z.ZodType>(schema: T) {
  return z.preprocess(value => typeof value === "string" && value.trim() === "" ? undefined : value, schema.optional())
}

function validOTELKeyValueList(value: string): boolean {
  return value.split(",").every((entry) => {
    const separator = entry.indexOf("=")
    if (separator <= 0 || entry.slice(0, separator).trim() === "") return false
    try {
      const key = decodeURIComponent(entry.slice(0, separator).trim())
      const itemValue = decodeURIComponent(entry.slice(separator + 1).trim())
      return !/[\r\n]/u.test(key + itemValue)
    }
    catch {
      return false
    }
  })
}

const optionalOTELKeyValueList = optionalValue(z.string().refine(validOTELKeyValueList, {
  message: "must be a comma-separated key=value list",
}))

const httpURL = z.string().trim().url().refine((value) => {
  try {
    const parsed = new URL(value)
    return (parsed.protocol === "http:" || parsed.protocol === "https:")
      && parsed.username === ""
      && parsed.password === ""
      && parsed.search === ""
      && parsed.hash === ""
  }
  catch {
    return false
  }
}, {
  message: "must be an http or https URL without credentials, query parameters, or fragments",
})

const httpOrigin = httpURL.refine((value) => {
  try {
    const pathname = new URL(value).pathname
    return pathname === "" || pathname === "/"
  }
  catch {
    return false
  }
}, { message: "must be an http or https origin without a path" })

const postgresURL = z.string().trim().refine((value) => {
  try {
    const parsed = new URL(value)
    return (parsed.protocol === "postgres:" || parsed.protocol === "postgresql:") && parsed.hostname !== ""
  }
  catch {
    return false
  }
}, { message: "must be an absolute postgres or postgresql URL" })

const schema = z.object({
  NODE_ENV: z.enum(["development", "test", "production"]).default("development"),
  HOST: z.string().trim().min(1).default("127.0.0.1"),
  PORT: z.coerce.number().int().min(1).max(65535).default(8091),
  LOG_FORMAT: z.enum(["auto", "console", "json"]).default("auto"),
  LOG_COLOR: z.enum(["auto", "always", "never"]).default("auto"),
  LOG_LEVEL: z.enum(["debug", "info", "warn", "error"]).default("info"),
  NO_COLOR: z.string().optional(),
  DATABASE_URL: optionalValue(postgresURL),
  AI_DATABASE_MAX_CONNECTIONS: z.coerce.number().int().min(1).max(100).default(10),
  AI_DATABASE_CONNECTION_TIMEOUT_MS: z.coerce.number().int().min(100).max(30_000).default(5_000),
  AI_DATABASE_STATEMENT_TIMEOUT_MS: z.coerce.number().int().min(1_000).max(120_000).default(15_000),
  AUTH_MODE: z.enum(["development", "bff-hmac"]).default("development"),
  AI_INTERNAL_SECRET: optionalValue(z.string().trim().refine(value => Buffer.byteLength(value, "utf8") >= 32, {
    message: "must contain at least 32 UTF-8 bytes",
  })),
  LUNA_API_BASE_URL: optionalValue(httpOrigin),
  OTEL_EXPORTER_OTLP_ENDPOINT: optionalValue(httpURL),
  OTEL_RESOURCE_ATTRIBUTES: optionalOTELKeyValueList,
  OTEL_EXPORTER_OTLP_HEADERS: optionalOTELKeyValueList,
  OTEL_SERVICE_VERSION: optionalValue(z.string()),
  AI_OBSERVABILITY_CAPTURE_CONTENT: z.stringbool().default(false),
  AI_OBSERVABILITY_CAPTURE_DATABASE_SPANS: z.stringbool().default(false),
})

const telemetryConfigSchema = schema.pick({
  LOG_FORMAT: true,
  LOG_COLOR: true,
  LOG_LEVEL: true,
  NO_COLOR: true,
  OTEL_EXPORTER_OTLP_ENDPOINT: true,
  OTEL_RESOURCE_ATTRIBUTES: true,
  OTEL_EXPORTER_OTLP_HEADERS: true,
  OTEL_SERVICE_VERSION: true,
  AI_OBSERVABILITY_CAPTURE_DATABASE_SPANS: true,
})

export type Config = z.infer<typeof schema>
export type RuntimeConfig = Config & {
  DATABASE_URL: string
  AI_INTERNAL_SECRET: string
  LUNA_API_BASE_URL: string
}
export type AgentTelemetryConfig = z.infer<typeof telemetryConfigSchema>
export type RuntimeStartupConfig =
  | { ok: true, telemetry: AgentTelemetryConfig, config: RuntimeConfig }
  | { ok: false, telemetry: AgentTelemetryConfig, error: unknown }

const defaultTelemetryConfig = telemetryConfigSchema.parse({})

export function loadTelemetryConfig(input: NodeJS.ProcessEnv = process.env): AgentTelemetryConfig {
  return telemetryConfigSchema.parse(input)
}

export function loadConfig(input: NodeJS.ProcessEnv = process.env): Config {
  const config = schema.parse(input)
  if (config.NODE_ENV === "production" && !config.DATABASE_URL) throw new Error("DATABASE_URL is required in production")
  if (config.NODE_ENV === "production" && config.AUTH_MODE === "development") throw new Error("A production authentication mode is required")
  if (config.AUTH_MODE === "bff-hmac" && !config.AI_INTERNAL_SECRET) {
    throw new Error("BFF HMAC authentication requires AI_INTERNAL_SECRET")
  }
  if ((config.NODE_ENV === "production" || config.DATABASE_URL) && !config.AI_INTERNAL_SECRET) {
    throw new Error("AI_INTERNAL_SECRET is required with durable storage")
  }
  if (config.NODE_ENV === "production" && !config.LUNA_API_BASE_URL) {
    throw new Error("Production model execution requires LUNA_API_BASE_URL")
  }
  return config
}

export function loadRuntimeConfig(input: NodeJS.ProcessEnv = process.env): RuntimeConfig {
  const config = loadConfig(input)
  if (!config.DATABASE_URL) throw new Error("DATABASE_URL is required to start Agent")
  if (!config.AI_INTERNAL_SECRET) throw new Error("AI_INTERNAL_SECRET is required to start Agent")
  if (!config.LUNA_API_BASE_URL) throw new Error("LUNA_API_BASE_URL is required to start Agent")
  return config as RuntimeConfig
}

// The real entrypoint snapshots process.env once. This keeps early telemetry
// available for reporting business-configuration failures without letting any
// later module re-read deployment settings.
export function loadRuntimeStartupConfig(input: NodeJS.ProcessEnv = process.env): RuntimeStartupConfig {
  const snapshot = { ...input }
  const telemetry = telemetryConfigSchema.safeParse(snapshot)
  if (!telemetry.success) return { ok: false, telemetry: defaultTelemetryConfig, error: telemetry.error }
  try {
    return { ok: true, telemetry: telemetry.data, config: loadRuntimeConfig(snapshot) }
  }
  catch (error) {
    return { ok: false, telemetry: telemetry.data, error }
  }
}

// OpenTelemetry's Node SDK merges dozens of process variables even when URL
// and headers are supplied in code. Snapshot the supported contract first,
// then remove the entire implicit channel before any exporter is constructed.
export function loadProcessRuntimeStartupConfig(): RuntimeStartupConfig {
  const startup = loadRuntimeStartupConfig(process.env)
  for (const key of Object.keys(process.env)) {
    if (key.startsWith("OTEL_")) delete process.env[key]
  }
  return startup
}
