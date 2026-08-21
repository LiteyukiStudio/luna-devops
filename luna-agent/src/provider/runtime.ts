import type { Config } from "../config.js"
import { ManagedProvider } from "./managed.js"
import type { ProviderConfigClient } from "./config-client.js"
import { agentRuntimeInternals } from "../runtime-settings.js"

export function createRuntimeProvider(_config: Config, managedConfig?: ProviderConfigClient) {
  if (!managedConfig) throw new Error("ai.provider_config_required")
  return new ManagedProvider(managedConfig, agentRuntimeInternals.configRefreshMs)
}
