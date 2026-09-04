import { DevelopmentRequestVerifier } from "../../src/auth.js"
import { loadConfig } from "../../src/config.js"
import { InMemoryRunStreamBus } from "../../src/run-stream-bus.js"
import { buildServer, type BuildServerDependencies } from "../../src/server.js"
import { DeterministicLunaApiClient } from "../../src/tools/luna-api-client.js"
import { MemoryToolCallStore, ToolOrchestrator } from "../../src/tools/orchestrator.js"
import { testRemoteConfigSource } from "./remote-config.js"

type TestServerDependencies = Pick<BuildServerDependencies, "repository">
  & Partial<Omit<BuildServerDependencies, "repository">>

export function buildTestServer(input: TestServerDependencies) {
  const remote = testRemoteConfigSource()
  const catalog = remote.source.currentCatalog()!
  const tools = input.tools ?? new ToolOrchestrator(
    catalog,
    new DeterministicLunaApiClient(() => ({ status: 200, body: {} })),
    new MemoryToolCallStore(),
  )
  const streamBus = input.streamBus ?? new InMemoryRunStreamBus(input.repository)
  return buildServer({
    config: input.config ?? loadConfig({ NODE_ENV: "test" }),
    repository: input.repository,
    requestVerifier: input.requestVerifier ?? new DevelopmentRequestVerifier(),
    tools,
    remoteConfig: input.remoteConfig ?? remote.source,
    cancelRun: input.cancelRun ?? (runId => tools.cancelRun(runId)),
    toolCatalogDigest: input.toolCatalogDigest ?? (() => catalog.digest),
    streamBus,
    ...(input.streamHubLimits ? { streamHubLimits: input.streamHubLimits } : {}),
  })
}
