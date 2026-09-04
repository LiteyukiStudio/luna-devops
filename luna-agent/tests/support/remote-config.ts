import type { RemoteRuntimeSettings } from "../../src/runtime-settings.js"
import { defaultRuntimeSettings } from "../../src/runtime-settings.js"
import {
  immutableRemoteProviderConfig,
  type RemoteConfigListener,
  type RemoteConfigSource,
  type RemoteProviderConfig,
} from "../../src/provider/config-client.js"
import { ToolCatalog } from "../../src/tools/catalog.js"
import { testToolOperation } from "./tool-catalog.js"

export function testRemoteProviderConfig(input: {
  version?: string
  baseUrl?: string
  configured?: boolean
  runtime?: Partial<RemoteRuntimeSettings>
  toolCatalog?: unknown[]
} = {}): RemoteProviderConfig {
  return immutableRemoteProviderConfig({
    version: input.version ?? "cfg-test",
    provider: {
      baseUrl: input.baseUrl ?? "https://provider.example/v1/",
      apiKey: "test-secret",
      providerCompatibility: "openai",
      promptCacheKeyMode: "disabled",
      channelAffinityEnabled: false,
      configured: input.configured ?? true,
      models: [{
        id: "aimod_test",
        name: "model-test",
        maxContextTokens: 32_000,
        maxOutputTokens: 8_000,
        inputCreditsPerMillion: "1",
        outputCreditsPerMillion: "2",
        cachedInputCreditsPerMillion: "0.5",
      }],
    },
    runtime: {
      providerTimeoutMs: defaultRuntimeSettings.providerTimeoutMs,
      maxRequestRetries: defaultRuntimeSettings.maxRequestRetries,
      runTimeoutMs: defaultRuntimeSettings.runTimeoutMs,
      agentConcurrentRuns: defaultRuntimeSettings.agentConcurrentRuns,
      userConcurrentRuns: defaultRuntimeSettings.userConcurrentRuns,
      ...input.runtime,
    },
    toolCatalog: input.toolCatalog ?? [testToolOperation("testOperation")],
  })
}

export function testRemoteConfigSource(initial = testRemoteProviderConfig()) {
  let config = initial
  let catalog = ToolCatalog.load(initial.toolCatalog)
  const listeners = new Set<RemoteConfigListener>()
  const source: RemoteConfigSource = {
    current: () => config,
    currentCatalog: () => catalog,
    subscribe(listener) {
      listeners.add(listener)
      return () => listeners.delete(listener)
    },
  }
  return {
    source,
    publish(next: RemoteProviderConfig, nextCatalog = ToolCatalog.load(next.toolCatalog)) {
      config = next
      catalog = nextCatalog
      for (const listener of listeners) listener(config, catalog)
    },
  }
}
