import type { Config } from "../config.js"
import { DeterministicProvider } from "./deterministic.js"
import { ManagedProvider } from "./managed.js"
import { OpenAICompatibleProvider } from "./openai-compatible.js"
import type { ProviderConfigClient } from "./config-client.js"
import { agentRuntimeInternals, defaultRuntimeSettings } from "../runtime-settings.js"

export function createRuntimeProvider(config: Config, managedConfig?: ProviderConfigClient) {
  if (managedConfig) return new ManagedProvider(managedConfig, agentRuntimeInternals.configRefreshMs)
  if (config.PROVIDER_BASE_URL && config.PROVIDER_API_KEY && config.PROVIDER_MODEL) {
    return new OpenAICompatibleProvider({
      baseUrl: config.PROVIDER_BASE_URL,
      apiKey: config.PROVIDER_API_KEY,
      model: config.PROVIDER_MODEL,
      timeoutMs: defaultRuntimeSettings.providerTimeoutMs,
      maxRetries: defaultRuntimeSettings.maxRequestRetries,
    })
  }
  return new DeterministicProvider()
}
