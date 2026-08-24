import { z } from "zod"
import { DiagnosticError } from "./diagnostic-error.js"

function optionalValue<T extends z.ZodType>(schema: T) {
  return z.preprocess(value => typeof value === "string" && value.trim() === "" ? undefined : value, schema.optional())
}

const schema = z.object({
  NODE_ENV: z.enum(["development", "test", "production"]).default("development"),
  HOST: z.string().default("127.0.0.1"),
  PORT: z.coerce.number().int().min(1).max(65535).default(8091),
  LOG_FORMAT: z.enum(["auto", "console", "json"]).default("auto"),
  LOG_COLOR: z.enum(["auto", "always", "never"]).default("auto"),
  LOG_LEVEL: z.enum(["debug", "info", "warn", "error"]).default("info"),
  DATABASE_URL: optionalValue(z.string()),
  REDIS_ADDR: optionalValue(z.string().url()),
  AI_DATABASE_MAX_CONNECTIONS: z.coerce.number().int().min(1).max(100).default(10),
  AI_DATABASE_CONNECTION_TIMEOUT_MS: z.coerce.number().int().min(100).max(30_000).default(5_000),
  AI_DATABASE_STATEMENT_TIMEOUT_MS: z.coerce.number().int().min(1_000).max(120_000).default(15_000),
  INSTANCE_ID: z.string().min(1).max(128).default(`agent-${process.pid}`),
  AUTH_MODE: z.enum(["development", "bff-hmac"]).default("development"),
  AI_INTERNAL_SECRET: optionalValue(z.string().min(32)),
  LUNA_API_BASE_URL: optionalValue(z.string().url()),
  OTEL_EXPORTER_OTLP_ENDPOINT: optionalValue(z.string().url()),
  OTEL_RESOURCE_ATTRIBUTES: optionalValue(z.string()),
  OTEL_EXPORTER_OTLP_HEADERS: optionalValue(z.string()),
  AI_OBSERVABILITY_CAPTURE_CONTENT: z.stringbool().default(false),
  AI_CONTEXT_COMPRESSION_TRIGGER_RATIO: z.coerce.number().min(0.5).max(0.95).default(0.9),
  AI_CONTEXT_RECENT_TURN_COUNT: z.coerce.number().int().min(1).max(32).default(16),
  AI_CONTEXT_MAX_HISTORY_PAYLOAD_K_BYTES: z.coerce.number().int().min(64).max(16_384).default(4096),
  AI_CONTEXT_MAX_SUMMARY_PAYLOAD_K_BYTES: z.coerce.number().int().min(16).max(4096).default(512),
  AI_CONTEXT_MAX_CONTINUATION_PAYLOAD_K_BYTES: z.coerce.number().int().min(16).max(4096).default(1024),
  AI_TOOLS_RESULT_PAYLOAD_K_BYTES: z.coerce.number().int().min(4).max(4096).default(512),
})

export type Config = z.infer<typeof schema>

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
  if (config.NODE_ENV === "production" && !config.REDIS_ADDR) {
    throw new DiagnosticError(
      "ai.stream_redis_url_required",
      "Production streaming requires REDIS_ADDR",
      "configure REDIS_ADDR with a valid Redis connection URI in the Agent deployment and redeploy",
    )
  }
  return config
}
