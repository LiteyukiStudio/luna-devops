import { z } from "zod"

function optionalValue<T extends z.ZodType>(schema: T) {
  return z.preprocess(value => typeof value === "string" && value.trim() === "" ? undefined : value, schema.optional())
}

const schema = z.object({
  NODE_ENV: z.enum(["development", "test", "production"]).default("development"),
  HOST: z.string().default("127.0.0.1"),
  PORT: z.coerce.number().int().min(1).max(65535).default(8091),
  DATABASE_URL: optionalValue(z.string()),
  INSTANCE_ID: z.string().min(1).max(128).default(`agent-${process.pid}`),
  AUTH_MODE: z.enum(["development", "bff-hmac"]).default("development"),
  AI_INTERNAL_SECRET: optionalValue(z.string().min(32)),
  LUNA_API_BASE_URL: optionalValue(z.string().url()),
  OTEL_EXPORTER_OTLP_ENDPOINT: optionalValue(z.string().url()),
  OTEL_RESOURCE_ATTRIBUTES: optionalValue(z.string()),
  OTEL_EXPORTER_OTLP_HEADERS: optionalValue(z.string()),
  AI_OBSERVABILITY_CAPTURE_CONTENT: z.stringbool().default(false),
  AI_CONTEXT_COMPRESSION_TRIGGER_RATIO: z.coerce.number().min(0.5).max(0.95).default(0.9),
  AI_CONTEXT_COMPRESSION_TARGET_RATIO: z.coerce.number().min(0.1).max(0.8).default(0.7),
  AI_CONTEXT_RECENT_TURN_COUNT: z.coerce.number().int().min(1).max(32).default(16),
  AI_CONTEXT_MAX_RECENT_TURN_COUNT: z.coerce.number().int().min(2).max(64).default(32),
  AI_CONTEXT_HISTORICAL_TOOL_K_TOKENS: z.coerce.number().int().min(1).max(256).default(64),
  AI_TOOLS_RESULT_PAYLOAD_K_BYTES: z.coerce.number().int().min(4).max(4096).default(512),
}).superRefine((value, context) => {
  if (value.AI_CONTEXT_COMPRESSION_TRIGGER_RATIO <= value.AI_CONTEXT_COMPRESSION_TARGET_RATIO) {
    context.addIssue({ code: "custom", path: ["AI_CONTEXT_COMPRESSION_TRIGGER_RATIO"], message: "must exceed AI_CONTEXT_COMPRESSION_TARGET_RATIO" })
  }
  if (value.AI_CONTEXT_RECENT_TURN_COUNT > value.AI_CONTEXT_MAX_RECENT_TURN_COUNT) {
    context.addIssue({ code: "custom", path: ["AI_CONTEXT_RECENT_TURN_COUNT"], message: "must not exceed AI_CONTEXT_MAX_RECENT_TURN_COUNT" })
  }
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
  return config
}
