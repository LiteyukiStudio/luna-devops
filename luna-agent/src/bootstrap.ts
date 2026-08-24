import { BffHmacRequestVerifier, DevelopmentRequestVerifier } from "./auth.js"
import { loadConfig } from "./config.js"
import { RunExecutor } from "./executor.js"
import { ContextCompiler } from "./context/compiler.js"
import { ModelRuntime } from "./model-runtime.js"
import { PayloadCipher } from "./payload-cipher.js"
import { deriveInternalKeys } from "./internal-secret.js"
import { PostgresRepository } from "./persistence/postgres.js"
import { ProviderConfigClient } from "./provider/config-client.js"
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
import { RedisRunStreamBus } from "./run-stream-bus.js"

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
  if (!config.REDIS_ADDR) throw new Error("ai.stream_redis_url_required")
  if (!config.LUNA_API_BASE_URL || !internalKeys) throw new Error("ai.provider_config_required")
  const repository = new PostgresRepository(config.DATABASE_URL, {
    maxConnections: config.AI_DATABASE_MAX_CONNECTIONS,
    connectionTimeoutMs: config.AI_DATABASE_CONNECTION_TIMEOUT_MS,
    statementTimeoutMs: config.AI_DATABASE_STATEMENT_TIMEOUT_MS,
  })
  await repository.assertReady()
  const streamBus = new RedisRunStreamBus(config.REDIS_ADDR, repository)
  await streamBus.connect()
  const providerConfigClient = new ProviderConfigClient(config.LUNA_API_BASE_URL, internalKeys.callbackServiceToken)
  // Provider、平台运行参数和工具目录接受 Luna API 的同一份权威配置；
  // 少量实例级上下文策略在下方由 Agent 环境变量覆盖。
  const initialRemoteConfig = await providerConfigClient.getCandidate()
  const rawProvider = createRuntimeProvider(config, providerConfigClient)
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
  providerConfigClient.commit(initialRemoteConfig)
  const toolStore = new PostgresToolCallStore(repository.pool, repository, toolArgumentsCipher)
  // 上下文收敛策略属于 Agent 进程内的无状态策略：默认无需配置，只有显式环境变量才覆盖。
  const runtime = {
    ...initialRemoteConfig.runtime,
    contextCompressionTriggerRatio: config.AI_CONTEXT_COMPRESSION_TRIGGER_RATIO,
    contextRecentTurnCount: config.AI_CONTEXT_RECENT_TURN_COUNT,
    contextMaxHistoryPayloadBytes: config.AI_CONTEXT_MAX_HISTORY_PAYLOAD_K_BYTES * 1024,
    contextMaxSummaryPayloadBytes: config.AI_CONTEXT_MAX_SUMMARY_PAYLOAD_K_BYTES * 1024,
    contextMaxContinuationPayloadBytes: config.AI_CONTEXT_MAX_CONTINUATION_PAYLOAD_K_BYTES * 1024,
    toolResultPayloadBudget: config.AI_TOOLS_RESULT_PAYLOAD_K_BYTES * 1024,
  }
  const tools = new ToolOrchestrator(async (runId) => {
    const state = await repository.getRunToolState(runId)
    if (!state) throw new Error("ai.run_not_found")
    return catalogRegistry.get(state.toolCatalogDigest)
  }, new HttpLunaApiToolClient(
    config.LUNA_API_BASE_URL,
    internalKeys.callbackServiceToken,
    () => providerConfigClient.current()?.runtime.maxRequestRetries ?? runtime.maxRequestRetries,
  ), toolStore, undefined, repository)
  tools.setRunMaxToolCalls(runtime.runMaxToolCalls)
  const contextCompiler = new ContextCompiler(repository, provider, {
    compressionTriggerRatio: runtime.contextCompressionTriggerRatio,
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
  const executor = new RunExecutor(repository, modelRuntime, config, tools, providerConfigClient, runtime, catalogRegistry, streamBus)
  const server = buildServer({
    config, repository, requestVerifier, provider,
    cancelRun: runId => { executor.cancel(runId) },
    toolCatalogDigest: () => catalogRegistry.digest(),
    tools,
    providerConfigClient,
    streamBus,
  })

  executor.start()
  await server.listen({ host: config.HOST, port: config.PORT })
  telemetryLog("agent.started", "info", { "server.address": config.HOST, "server.port": config.PORT })

  let stopping = false
  async function shutdown() {
    if (stopping) return
    stopping = true
    telemetryLog("agent.stopping", "info")
    await executor.stop()
    await server.close()
    await streamBus.close()
    if (repository instanceof PostgresRepository) await repository.close()
    await shutdownTelemetry()
  }
  process.once("SIGTERM", () => { void shutdown() })
  process.once("SIGINT", () => { void shutdown() })
}
