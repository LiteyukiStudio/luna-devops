import type { Config } from "../config.js"
import { ManagedProvider } from "./managed.js"
import type { RemoteConfigSnapshot } from "./config-client.js"

export function createRuntimeProvider(_config: Config, managedConfig?: RemoteConfigSnapshot) {
  if (!managedConfig) throw new Error("ai.provider_config_required")
  return new ManagedProvider(managedConfig)
}
