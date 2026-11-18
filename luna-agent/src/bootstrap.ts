import { loadBundledDocsKnowledge } from "./knowledge/docs-knowledge.js"
import { listDocsTool, readDocTool, searchDocsTool } from "./tools/docs-tools.js"
import { BffHmacRequestVerifier, DevelopmentRequestVerifier } from "./auth.js"
import type { RuntimeConfig } from "./config.js"
import { RunExecutor } from "./executor/index.js"
import { ContextCompiler } from "./context/compiler.js"
import { ModelRuntime } from "./model-runtime.js"
import { PayloadCipher } from "./payload-cipher.js"
import { deriveInternalKeys } from "./internal-secret.js"
import { PostgresRepository } from "./persistence/postgres.js"
import { RemoteConfigSnapshot } from "./provider/config-client.js"
import { ManagedProvider } from "./provider/managed.js"
import { UsageRecordingModelProvider } from "./provider/usage-recorder.js"
import { buildServer } from "./server.js"
import { agentMetrics, configureAgentTelemetry, configureAIContentCapture, internalSpanOptions, shutdownTelemetry, telemetryLog, withSpan } from "./telemetry.js"
import { ToolCatalogRegistry } from "./tools/catalog-registry.js"
import { HttpLunaApiToolClient } from "./tools/luna-api-client.js"
import { ToolOrchestrator } from "./tools/orchestrator.js"
import { PostgresToolCallStore } from "./tools/postgres-store.js"
import { businessCardTools } from "./tools/business-card-tools.js"
import { navigateToRouteTool } from "./tools/ui-route.js"
import { searchToolsTool } from "./tools/tool-search.js"
import { getToolDetailsTool } from "./tools/tool-details.js"
import { loadSkillTool } from "./tools/skill-loader.js"
import { HttpLunaApiTaskStatusClient, waitForTaskTool } from "./tools/wait-for-task.js"
import { InMemoryRunStreamBus } from "./run-stream-bus.js"
import { runtimeSettingsFromRemote } from "./runtime-settings.js"

export async function startAgent(config: RuntimeConfig): Promise<void> {
  configureAgentTelemetry(config)
  configureAIContentCapture(config.AI_OBSERVABILITY_CAPTURE_CONTENT)
  if (config.AI_OBSERVABILITY_CAPTURE_CONTENT) {
    telemetryLog("agent.telemetry.content_capture_enabled", "warn", {
      "luna.ai.content_capture": true,
    })
  }
  const internalKeys = deriveInternalKeys(config.AI_INTERNAL_SECRET)
  const trimmedPreviousSecret = config.AI_INTERNAL_SECRET_PREVIOUS?.trim()
  const previousKeys = trimmedPreviousSecret && trimmedPreviousSecret !== config.AI_INTERNAL_SECRET.trim()
    ? deriveInternalKeys(trimmedPreviousSecret)
    : undefined
  const runExecutionSnapshotCipher = new PayloadCipher(
    internalKeys.runExecutionSnapshotEncryptionKey,
    "run-execution-snapshot-v1",
    "ai.run_execution_snapshot_key_unavailable",
    previousKeys?.runExecutionSnapshotEncryptionKey,
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
  )
  // Provider、运行时调度参数和工具目录接受 Luna API 的同一份权威配置。
  // 输入、模型循环、工具、卡片和上下文预算是 Agent 内部固定不变量。
  const initialRemoteConfig = await remoteConfig.initialize()
  const rawProvider = new ManagedProvider(remoteConfig)
  const provider = new UsageRecordingModelProvider(rawProvider, repository)
  const requestVerifier = config.AUTH_MODE === "bff-hmac"
    ? new BffHmacRequestVerifier(
        internalKeys.serviceToken,
        internalKeys.actorSigningKey,
        previousKeys
          ? { serviceToken: previousKeys.serviceToken, actorSigningKey: previousKeys.actorSigningKey }
          : undefined,
      )
    : new DevelopmentRequestVerifier()
  const toolArgumentsCipher = new PayloadCipher(
    internalKeys.toolArgumentsEncryptionKey,
    "tool-arguments-v1",
    "ai.tool_arguments_key_unavailable",
    previousKeys?.toolArgumentsEncryptionKey,
  )
  const catalog = remoteConfig.currentCatalog()
  if (!catalog) throw new Error("ai.provider_config_unavailable")
  const catalogRegistry = new ToolCatalogRegistry(catalog, initialRemoteConfig.version)
  const resolveCatalog = (digest?: string) => digest && catalogRegistry.has(digest)
    ? catalogRegistry.get(digest)
    : catalogRegistry.current()
  const toolStore = new PostgresToolCallStore(repository.pool, repository, toolArgumentsCipher)
  const budgetOverrides = {
    ...(config.AI_RUN_MAX_MODEL_STEPS !== undefined ? { maxModelSteps: config.AI_RUN_MAX_MODEL_STEPS } : {}),
    ...(config.AI_RUN_MAX_TOOL_CALLS !== undefined ? { runMaxToolCalls: config.AI_RUN_MAX_TOOL_CALLS } : {}),
  }
  const runtime = runtimeSettingsFromRemote(initialRemoteConfig.runtime, budgetOverrides)
  const tools = new ToolOrchestrator(async (runId) => {
    const state = await repository.getRunToolState(runId)
    if (!state) throw new Error("ai.run_not_found")
    return resolveCatalog(state.toolCatalogDigest)
  }, new HttpLunaApiToolClient(
    config.LUNA_API_BASE_URL,
    internalKeys.callbackServiceToken,
    () => {
      const current = remoteConfig.current()
      if (!current) throw new Error("ai.provider_config_unavailable")
      return current.runtime.maxRequestRetries
    },
  ), toolStore)
  tools.setRunMaxToolCalls(runtime.runMaxToolCalls)
  const contextCompiler = new ContextCompiler(repository, provider, {
    recentTurnCount: runtime.contextRecentTurnCount,
    maxCompressionTurnsPerCompile: runtime.contextMaxCompressionTurnsPerCompile,
    summaryMaxOutputTokens: runtime.contextSummaryMaxOutputTokens,
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
          const selectedCatalog = resolveCatalog(toolCatalogDigest)
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
            loadSkillTool,
            searchDocsTool,
            listDocsTool,
            readDocTool,
            ...businessCardTools,
            navigateToRouteTool,
            waitForTaskTool,
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
      const selectedCatalog = resolveCatalog(toolCatalogDigest)
      return {
        ...selectedCatalog.search(input),
        loadedOperationIds: [], missingOperationIds: [], catalogDigest: selectedCatalog.digest,
        duplicate: false, cacheHit: false,
      }
    },
    details: (operationIds, toolCatalogDigest) => {
      const selectedCatalog = resolveCatalog(toolCatalogDigest)
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
  const executor = new RunExecutor({
    repository,
    modelRuntime,
    tools,
    runtimeConfig: remoteConfig,
    initialRuntimeSettings: runtime,
    catalogRegistry,
    streamBus,
    docsKnowledge: loadBundledDocsKnowledge(config.AI_DOCS_BASE_URL),
    taskStatusClient: new HttpLunaApiTaskStatusClient(config.LUNA_API_BASE_URL, internalKeys.callbackServiceToken),
  })
  const server = buildServer({
    config, repository, requestVerifier,
    cancelRun: runId => executor.cancel(runId),
    cancelExecutingRun: runId => executor.cancelExecuting(runId),
    notifyRunQueued: () => executor.notifyWorkAvailable(),
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
