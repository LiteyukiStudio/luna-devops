import { RunExecutor, type RunExecutorDependencies } from "../../src/executor/index.js"
import type { ModelRuntime } from "../../src/model-runtime.js"
import type { Repository } from "../../src/persistence/repository.js"
import { InMemoryRunStreamBus } from "../../src/run-stream-bus.js"
import { defaultRuntimeSettings } from "../../src/runtime-settings.js"
import { ToolCatalogRegistry } from "../../src/tools/catalog-registry.js"
import type { ToolCatalog } from "../../src/tools/catalog.js"
import { DeterministicLunaApiClient } from "../../src/tools/luna-api-client.js"
import { MemoryToolCallStore, ToolOrchestrator } from "../../src/tools/orchestrator.js"
import { testRemoteConfigSource, testRemoteProviderConfig } from "./remote-config.js"

type TestExecutorDependencies = Partial<Omit<RunExecutorDependencies, "repository" | "modelRuntime">> & {
  catalog?: ToolCatalog
}

export function testExecutor(
  repository: Repository,
  modelRuntime: ModelRuntime,
  input: TestExecutorDependencies = {},
): RunExecutor {
  const remote = testRemoteConfigSource(input.catalog
    ? testRemoteProviderConfig({ toolCatalog: input.catalog.all() })
    : testRemoteProviderConfig())
  const runtimeConfig = input.runtimeConfig ?? remote.source
  const catalog = input.catalog ?? runtimeConfig.currentCatalog()
  const config = runtimeConfig.current()
  if (!catalog || !config) throw new Error("test.remote_config_missing")
  return new RunExecutor({
    repository,
    modelRuntime,
    runtimeConfig,
    catalogRegistry: input.catalogRegistry ?? new ToolCatalogRegistry(catalog, config.version),
    tools: input.tools ?? new ToolOrchestrator(
      catalog,
      new DeterministicLunaApiClient(() => ({ status: 200, body: {} })),
      new MemoryToolCallStore(),
    ),
    initialRuntimeSettings: input.initialRuntimeSettings ?? defaultRuntimeSettings,
    streamBus: input.streamBus ?? new InMemoryRunStreamBus(repository),
  })
}
