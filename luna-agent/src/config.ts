import { z } from "zod"

const schema = z.object({
  NODE_ENV: z.enum(["development", "test", "production"]).default("development"),
  HOST: z.string().default("127.0.0.1"),
  PORT: z.coerce.number().int().min(1).max(65535).default(8091),
  DATABASE_URL: z.string().optional(),
  INSTANCE_ID: z.string().min(1).max(128).default(`agent-${process.pid}`),
  AUTH_MODE: z.enum(["development", "bff-hmac", "jwt"]).default("development"),
  API_SERVICE_TOKEN: z.string().min(32).optional(),
  ACTOR_CONTEXT_SIGNING_KEY: z.string().min(32).optional(),
  RUN_GRANT_ENCRYPTION_KEY_BASE64: z.string().optional(),
  API_SERVICE_JWT_PUBLIC_KEY: z.string().optional(),
  ACTOR_CONTEXT_PUBLIC_KEY: z.string().optional(),
  PROVIDER_TYPE: z.enum(["deterministic", "openai-compatible"]).default("deterministic"),
  PROVIDER_BASE_URL: z.string().url().optional(),
  PROVIDER_API_KEY: z.string().min(1).optional(),
  PROVIDER_MODEL: z.string().min(1).optional(),
  PROVIDER_TIMEOUT_MS: z.coerce.number().int().min(1000).max(120000).default(30000),
  PROVIDER_CONFIG_TTL_MS: z.coerce.number().int().min(1000).max(600000).default(300000),
  RUN_POLL_MS: z.coerce.number().int().min(50).max(10000).default(500),
  RUN_LEASE_SECONDS: z.coerce.number().int().min(5).max(300).default(30),
  RUN_MAX_WALL_MS: z.coerce.number().int().min(1000).max(900000).default(300000),
  MAX_INPUT_BYTES: z.coerce.number().int().min(1024).max(1048576).default(48000),
  MAX_CONCURRENT_RUNS: z.coerce.number().int().min(1).max(10).default(2),
  LUNA_API_BASE_URL: z.string().url().optional(),
  AI_AGENT_CALLBACK_SERVICE_TOKEN: z.string().min(16).optional(),
  TOOL_CATALOG_JSON: z.string().optional(),
})

export type Config = z.infer<typeof schema>

export function loadConfig(input: NodeJS.ProcessEnv = process.env): Config {
  const config = schema.parse(input)
  if (config.NODE_ENV === "production" && !config.DATABASE_URL) throw new Error("DATABASE_URL is required in production")
  if (config.NODE_ENV === "production" && config.AUTH_MODE === "development") throw new Error("A production authentication mode is required")
  if (config.AUTH_MODE === "bff-hmac" && (!config.API_SERVICE_TOKEN || !config.ACTOR_CONTEXT_SIGNING_KEY)) {
    throw new Error("BFF HMAC authentication requires independent service token and Actor Context signing key")
  }
  if (config.AUTH_MODE === "jwt" && (!config.API_SERVICE_JWT_PUBLIC_KEY || !config.ACTOR_CONTEXT_PUBLIC_KEY)) {
    throw new Error("API service and Actor Context public keys are required for JWT authentication")
  }
  if (config.PROVIDER_TYPE === "openai-compatible" && (!config.PROVIDER_BASE_URL || !config.PROVIDER_API_KEY || !config.PROVIDER_MODEL)) {
    throw new Error("OpenAI-compatible provider requires base URL, API key, and model")
  }
  if ((config.NODE_ENV === "production" || config.DATABASE_URL) && !config.RUN_GRANT_ENCRYPTION_KEY_BASE64) {
    throw new Error("RUN_GRANT_ENCRYPTION_KEY_BASE64 is required with durable storage")
  }
  if (config.NODE_ENV === "production" && (!config.LUNA_API_BASE_URL || !config.AI_AGENT_CALLBACK_SERVICE_TOKEN)) {
    throw new Error("Production model execution requires Luna API provider configuration")
  }
  return config
}
