import type { Config } from "../config.js"
import { DeterministicProvider } from "./deterministic.js"
import { ManagedProvider } from "./managed.js"
import { OpenAICompatibleProvider } from "./openai-compatible.js"
import type { ProviderConfigClient } from "./config-client.js"

export function createRuntimeProvider(config: Config, managedConfig?: ProviderConfigClient) {
  if (managedConfig) return new ManagedProvider(managedConfig, config.PROVIDER_CONFIG_TTL_MS)
  if (config.PROVIDER_BASE_URL && config.PROVIDER_API_KEY && config.PROVIDER_MODEL) {
    return new OpenAICompatibleProvider({
      baseUrl: config.PROVIDER_BASE_URL,
      apiKey: config.PROVIDER_API_KEY,
      model: config.PROVIDER_MODEL,
      timeoutMs: config.PROVIDER_TIMEOUT_MS,
    })
  }
  return new DeterministicProvider()
}
