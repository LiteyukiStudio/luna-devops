import { z } from "zod"

function optionalValue<T extends z.ZodType>(schema: T) {
  return z.preprocess(value => typeof value === "string" && value.trim() === "" ? undefined : value, schema.optional())
}

function validOTELKeyValueList(value: string): boolean {
  return value.split(",").every((entry) => {
    const separator = entry.indexOf("=")
    if (separator <= 0 || entry.slice(0, separator).trim() === "") return false
    try {
      decodeURIComponent(entry.slice(0, separator).trim())
      decodeURIComponent(entry.slice(separator + 1).trim())
      return true
    }
    catch {
      return false
    }
  })
}

const optionalOTELKeyValueList = optionalValue(z.string().refine(validOTELKeyValueList, {
  message: "must be a comma-separated key=value list",
}))

const schema = z.object({
  NODE_ENV: z.enum(["development", "test", "production"]).default("development"),
  HOST: z.string().default("127.0.0.1"),
  PORT: z.coerce.number().int().min(1).max(65535).default(8091),
  LOG_FORMAT: z.enum(["auto", "console", "json"]).default("auto"),
  LOG_COLOR: z.enum(["auto", "always", "never"]).default("auto"),
  LOG_LEVEL: z.enum(["debug", "info", "warn", "error"]).default("info"),
  NO_COLOR: z.string().optional(),
  DATABASE_URL: optionalValue(z.string()),
  AI_DATABASE_MAX_CONNECTIONS: z.coerce.number().int().min(1).max(100).default(10),
  AI_DATABASE_CONNECTION_TIMEOUT_MS: z.coerce.number().int().min(100).max(30_000).default(5_000),
  AI_DATABASE_STATEMENT_TIMEOUT_MS: z.coerce.number().int().min(1_000).max(120_000).default(15_000),
  AUTH_MODE: z.enum(["development", "bff-hmac"]).default("development"),
  AI_INTERNAL_SECRET: optionalValue(z.string().min(32)),
  LUNA_API_BASE_URL: optionalValue(z.string().url()),
  OTEL_EXPORTER_OTLP_ENDPOINT: optionalValue(z.string().url()),
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
export type AgentTelemetryConfig = z.infer<typeof telemetryConfigSchema>

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
    throw new Error("Production model execution requires Luna API provider configuration")
  }
  return config
}
