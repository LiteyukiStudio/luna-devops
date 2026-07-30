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
  PROVIDER_BASE_URL: optionalValue(z.string().url()),
  PROVIDER_API_KEY: optionalValue(z.string().min(1)),
  PROVIDER_MODEL: optionalValue(z.string().min(1)),
  LUNA_API_BASE_URL: optionalValue(z.string().url()),
  TOOL_CATALOG_JSON: optionalValue(z.string()),
})

export type Config = z.infer<typeof schema>

export function loadConfig(input: NodeJS.ProcessEnv = process.env): Config {
  const config = schema.parse(input)
  if (config.NODE_ENV === "production" && !config.DATABASE_URL) throw new Error("DATABASE_URL is required in production")
  if (config.NODE_ENV === "production" && config.AUTH_MODE === "development") throw new Error("A production authentication mode is required")
  if (config.AUTH_MODE === "bff-hmac" && !config.AI_INTERNAL_SECRET) {
    throw new Error("BFF HMAC authentication requires AI_INTERNAL_SECRET")
  }
  const directProviderValues = [config.PROVIDER_BASE_URL, config.PROVIDER_API_KEY, config.PROVIDER_MODEL]
  if (directProviderValues.some(Boolean) && !directProviderValues.every(Boolean)) {
    throw new Error("Direct provider configuration requires base URL, API key, and model")
  }
  if ((config.NODE_ENV === "production" || config.DATABASE_URL) && !config.AI_INTERNAL_SECRET) {
    throw new Error("AI_INTERNAL_SECRET is required with durable storage")
  }
  if (config.NODE_ENV === "production" && !config.LUNA_API_BASE_URL) {
    throw new Error("Production model execution requires Luna API provider configuration")
  }
  return config
}
