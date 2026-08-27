import { BffHmacRequestVerifier, DevelopmentRequestVerifier } from "./auth.js"
import { loadConfig } from "./config.js"
import { RunExecutor } from "./executor.js"
import { ContextCompiler } from "./context/compiler.js"
import { ModelRuntime } from "./model-runtime.js"
import { PayloadCipher } from "./payload-cipher.js"
import { deriveInternalKeys } from "./internal-secret.js"
import { PostgresRepository } from "./persistence/postgres.js"
import { RemoteConfigSnapshot } from "./provider/config-client.js"
import { createRuntimeProvider } from "./provider/runtime.js"
import { BudgetedModelProvider } from "./provider/budgeted.js"
import { buildServer } from "./server.js"
import { agentMetrics, configureAIContentCapture, internalSpanOptions, shutdownTelemetry, telemetryLog, withSpan } from "./telemetry.js"
import { ToolCatalog } from "./tools/catalog.js"
import { ToolCatalogRegistry } from "./tools/catalog-registry.js"
import { HttpLunaApiToolClient } from "./tools/luna-api-client.js"
import { ToolOrchestrator } from "./tools/orchestrator.js"
import { PostgresToolCallStore } from "./tools/postgres-store.js"
import { businessCardTools } from "./tools/business-card-tools.js"
import { navigateToRouteTool } from "./tools/ui-route.js"
import { searchToolsTool } from "./tools/tool-search.js"
import { getToolDetailsTool } from "./tools/tool-details.js"
import { InMemoryRunStreamBus } from "./run-stream-bus.js"
import { runtimeSettingsFromRemote } from "./runtime-settings.js"

export async function startAgent(): Promise<void> {
  const config = loadConfig()
  configureAIContentCapture(config.AI_OBSERVABILITY_CAPTURE_CONTENT)
  if (config.AI_OBSERVABILITY_CAPTURE_CONTENT) {
    telemetryLog("agent.telemetry.content_capture_enabled", "warn", {
      "luna.ai.content_capture": true,
    })
  }
  const internalKeys = config.AI_INTERNAL_SECRET ? deriveInternalKeys(config.AI_INTERNAL_SECRET) : undefined
  if (!config.DATABASE_URL) throw new Error("ai.persistence_database_url_required")
  if (!config.LUNA_API_BASE_URL || !internalKeys) throw new Error("ai.provider_config_required")
  const runExecutionSnapshotCipher = new PayloadCipher(
    internalKeys.runExecutionSnapshotEncryptionKey,
    "run-execution-snapshot-v1",
    "ai.run_execution_snapshot_key_unavailable",
  )
  const repository = new PostgresRepository(config.DATABASE_URL, {
    maxConnections: config.AI_DATABASE_MAX_CONNECTIONS,
    connectionTimeoutMs: config.AI_DATABASE_CONNECTION_TIMEOUT_MS,
    statementTimeoutMs: config.AI_DATABASE_STATEMENT_TIMEOUT_MS,
  }, runExecutionSnapshotCipher)
  await repository.assertReady()
  const streamBus = new InMemoryRunStreamBus(repository)
  const remoteConfig = new RemoteConfigSnapshot(
    config.LUNA_API_BASE_URL,
    internalKeys.callbackServiceToken,
    candidate => { ToolCatalog.load(candidate.toolCatalog) },
  )
  // Provider、运行时调度参数和工具目录接受 Luna API 的同一份权威配置。
  // 输入、模型循环、工具、卡片和上下文预算是 Agent 内部固定不变量。
  const initialRemoteConfig = await remoteConfig.initialize()
  const rawProvider = createRuntimeProvider(config, remoteConfig)
  const provider = new BudgetedModelProvider(rawProvider, repository)
  const requestVerifier = config.AUTH_MODE === "bff-hmac"
    ? new BffHmacRequestVerifier(internalKeys.serviceToken, internalKeys.actorSigningKey)
    : new DevelopmentRequestVerifier()
  const toolArgumentsCipher = new PayloadCipher(
    internalKeys.toolArgumentsEncryptionKey,
    "tool-arguments-v1",
    "ai.tool_arguments_key_unavailable",
  )
  const catalog = ToolCatalog.load(initialRemoteConfig.toolCatalog)
  const catalogRegistry = new ToolCatalogRegistry(catalog, initialRemoteConfig.version)
  const toolStore = new PostgresToolCallStore(repository.pool, repository, toolArgumentsCipher)
  const runtime = runtimeSettingsFromRemote(initialRemoteConfig.runtime)
  const tools = new ToolOrchestrator(async (runId) => {
    const state = await repository.getRunToolState(runId)
    if (!state) throw new Error("ai.run_not_found")
    return catalogRegistry.get(state.toolCatalogDigest)
  }, new HttpLunaApiToolClient(
    config.LUNA_API_BASE_URL,
    internalKeys.callbackServiceToken,
    () => remoteConfig.current()?.runtime.maxRequestRetries ?? runtime.maxRequestRetries,
  ), toolStore)
  tools.setRunMaxToolCalls(runtime.runMaxToolCalls)
  const contextCompiler = new ContextCompiler(repository, provider, {
    recentTurnCount: runtime.contextRecentTurnCount,
    maxUncompressedTurnCount: runtime.contextMaxUncompressedTurnCount,
    maxCompressionTurnsPerCompile: runtime.contextMaxCompressionTurnsPerCompile,
    summaryMaxOutputTokens: runtime.contextSummaryMaxOutputTokens,
    maxHistoryPayloadBytes: runtime.contextMaxHistoryPayloadBytes,
    maxSummaryPayloadBytes: runtime.contextMaxSummaryPayloadBytes,
    maxContinuationPayloadBytes: runtime.contextMaxContinuationPayloadBytes,
  })
  const modelRuntime = new ModelRuntime(provider, {
    resolve: async (_pageContext, _userInput, loadedOperationIds, _signal, toolCatalogDigest) => {
      const startedAt = performance.now()
      let outcome = "succeeded"
      let strategy = "base_only"
      let candidateCount = 0
      let matchCount = 0
      try {
        return await withSpan("agent.tools.retrieve", internalSpanOptions({
          "luna.agent.tool_retrieval.trigger": "automatic",
        }), async span => {
          const selectedCatalog = toolCatalogDigest ? catalogRegistry.get(toolCatalogDigest) : catalogRegistry.current()
          const retrievedTools = selectedCatalog.modelTools(loadedOperationIds)
          strategy = "explicit_details"
          candidateCount = loadedOperationIds.length
          matchCount = retrievedTools.length
          span.setAttribute("luna.agent.tool_retrieval.outcome", outcome)
          span.setAttribute("luna.agent.tool_retrieval.strategy", strategy)
          span.setAttribute("luna.agent.tool_retrieval.match_count", matchCount)
          return [
            ...retrievedTools,
            searchToolsTool,
            getToolDetailsTool,
            ...businessCardTools,
            navigateToRouteTool,
          ]
        })
      }
      catch (error) {
        outcome = "failed"
        throw error
      }
      finally {
        const attributes = { outcome, strategy }
        agentMetrics.toolRetrievals.add(1, attributes)
        agentMetrics.toolRetrievalCandidates.record(candidateCount, attributes)
        agentMetrics.toolRetrievalLoaded.record(matchCount, attributes)
        agentMetrics.toolRetrievalDuration.record((performance.now() - startedAt) / 1000, attributes)
      }
    },
    search: (input, _pageContext, _signal, toolCatalogDigest) => {
      const selectedCatalog = toolCatalogDigest ? catalogRegistry.get(toolCatalogDigest) : catalogRegistry.current()
      return {
        ...selectedCatalog.search(input),
        loadedOperationIds: [], missingOperationIds: [], catalogDigest: selectedCatalog.digest,
        duplicate: false, cacheHit: false,
      }
    },
    details: (operationIds, toolCatalogDigest) => {
      const selectedCatalog = toolCatalogDigest ? catalogRegistry.get(toolCatalogDigest) : catalogRegistry.current()
      const result = selectedCatalog.semanticDetails(operationIds)
      return {
        items: result.items,
        loadedOperationIds: result.items.map(item => item.operationId),
        alreadySelectedOperationIds: [],
        missingOperationIds: result.missingOperationIds,
        catalogDigest: selectedCatalog.digest,
        duplicate: false,
        cacheHit: false,
      }
    },
  }, contextCompiler)
  const executor = new RunExecutor(repository, modelRuntime, config, tools, remoteConfig, runtime, catalogRegistry, streamBus)
  const server = buildServer({
    config, repository, requestVerifier,
    cancelRun: runId => executor.cancel(runId),
    toolCatalogDigest: () => catalogRegistry.digest(),
    tools,
    remoteConfig,
    streamBus,
  })

  remoteConfig.start()
  executor.start()
  await server.listen({ host: config.HOST, port: config.PORT })
  telemetryLog("agent.started", "info", { "server.address": config.HOST, "server.port": config.PORT })

  let stopping = false
  async function shutdown() {
    if (stopping) return
    stopping = true
    telemetryLog("agent.stopping", "info")
    remoteConfig.stop()
    await executor.stop()
    await server.close()
    if (repository instanceof PostgresRepository) await repository.close()
    await shutdownTelemetry()
  }
  process.once("SIGTERM", () => { void shutdown() })
  process.once("SIGINT", () => { void shutdown() })
}
