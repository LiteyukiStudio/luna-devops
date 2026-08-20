import { randomBytes } from "node:crypto"
import { BffHmacAuthenticator, DevelopmentAuthenticator } from "./auth.js"
import { loadConfig } from "./config.js"
import { RunExecutor } from "./executor.js"
import { ContextCompiler } from "./context/compiler.js"
import { ModelRuntime } from "./model-runtime.js"
import { PayloadCipher } from "./payload-cipher.js"
import { deriveInternalKeys } from "./internal-secret.js"
import { MemoryRepository } from "./persistence/memory.js"
import { PostgresRepository } from "./persistence/postgres.js"
import { ProviderConfigClient } from "./provider/config-client.js"
import { createRuntimeProvider } from "./provider/runtime.js"
import { BudgetedModelProvider } from "./provider/budgeted.js"
import { buildServer } from "./server.js"
import { agentMetrics, configureAIContentCapture, internalSpanOptions, shutdownTelemetry, telemetryLog, withSpan } from "./telemetry.js"
import { defaultRuntimeSettings } from "./runtime-settings.js"
import { ToolCatalog, type RetrievalContext } from "./tools/catalog.js"
import { HttpLunaApiToolClient } from "./tools/luna-api-client.js"
import { MemoryToolCallStore, ProjectingToolCallStore, ToolOrchestrator } from "./tools/orchestrator.js"
import { PostgresToolCallStore } from "./tools/postgres-store.js"
import { platformOperations } from "./tools/generated/platform.js"
import { businessCardTools } from "./tools/business-card-tools.js"
import { navigateToRouteTool } from "./tools/ui-route.js"
import { searchToolsTool } from "./tools/tool-search.js"
import { browseToolsTool } from "./tools/tool-directory.js"

export async function startAgent(): Promise<void> {
  const config = loadConfig()
  configureAIContentCapture(config.AI_OBSERVABILITY_CAPTURE_CONTENT)
  if (config.AI_OBSERVABILITY_CAPTURE_CONTENT) {
    telemetryLog("agent.telemetry.content_capture_enabled", "warn", {
      "luna.ai.content_capture": true,
    })
  }
  const internalKeys = config.AI_INTERNAL_SECRET ? deriveInternalKeys(config.AI_INTERNAL_SECRET) : undefined
  const repository = config.DATABASE_URL ? new PostgresRepository(config.DATABASE_URL) : new MemoryRepository()
  const providerConfigClient = config.LUNA_API_BASE_URL && internalKeys
    ? new ProviderConfigClient(config.LUNA_API_BASE_URL, internalKeys.callbackServiceToken)
    : undefined
  // 托管模式的工具契约只能由 Luna API 权威下发。首次拉取失败时直接阻止启动，
  // 避免使用无契约 fallback 后在配置恢复时仍永久缺失平台工具。
  const initialRemoteConfig = providerConfigClient ? await providerConfigClient.get() : undefined
  const rawProvider = createRuntimeProvider(config, providerConfigClient)
  const provider = new BudgetedModelProvider(rawProvider, repository)
  const authenticator = config.AUTH_MODE === "bff-hmac"
    ? new BffHmacAuthenticator(internalKeys!.serviceToken, internalKeys!.actorSigningKey)
    : new DevelopmentAuthenticator()
  const grantCipher = new PayloadCipher(
    internalKeys?.runGrantEncryptionKey ?? randomBytes(32),
    "run-grant-v1",
    "ai.run_grant_key_unavailable",
  )
  const toolArgumentsCipher = new PayloadCipher(
    internalKeys?.toolArgumentsEncryptionKey ?? randomBytes(32),
    "tool-arguments-v1",
    "ai.tool_arguments_key_unavailable",
  )
  const remoteOperationIds = new Set(
    (initialRemoteConfig?.toolCatalog ?? [])
      .map(item => typeof item === "object" && item && "operationId" in item ? String(item.operationId) : ""),
  )
  const catalog = ToolCatalog.load(config.TOOL_CATALOG_JSON
    ? JSON.parse(config.TOOL_CATALOG_JSON) as unknown
    : [
        ...(initialRemoteConfig?.toolCatalog ?? []),
        ...platformOperations.filter(operation => !remoteOperationIds.has(operation.operationId)),
      ])
  const toolStore = repository instanceof PostgresRepository
    ? new PostgresToolCallStore(repository.pool, repository, toolArgumentsCipher)
    : new ProjectingToolCallStore(new MemoryToolCallStore(), repository)
  const runtime = initialRemoteConfig?.runtime ?? defaultRuntimeSettings
  const tools = catalog && config.LUNA_API_BASE_URL && internalKeys
    ? new ToolOrchestrator(catalog, new HttpLunaApiToolClient(
        config.LUNA_API_BASE_URL,
        internalKeys.callbackServiceToken,
        () => providerConfigClient?.current()?.runtime.maxRequestRetries ?? runtime.maxRequestRetries,
      ), toolStore, undefined, undefined, async runId => {
        const encrypted = await repository.getRunActorGrantCiphertext(runId)
        if (!encrypted) throw new Error("ai.run_grant_unavailable")
        return grantCipher.decrypt(encrypted)
      }, undefined, async runId => {
        const authorization = await repository.getRunConversationAuthorization(runId)
        return authorization ? grantCipher.decrypt(authorization.grantCiphertext) : undefined
      })
    : undefined
  tools?.setRunMaxToolCalls(runtime.runMaxToolCalls)
  const contextCompiler = new ContextCompiler(repository, provider, {
    inputTokenBudget: runtime.contextInputTokenBudget,
    compressionTriggerRatio: runtime.contextCompressionTriggerRatio,
    compressionTargetRatio: runtime.contextCompressionTargetRatio,
    recentTurnCount: runtime.contextRecentTurnCount,
    maxRecentTurnCount: runtime.contextMaxRecentTurnCount,
    maxUncompressedTurnCount: runtime.contextMaxUncompressedTurnCount,
    maxCompressionTurnsPerCompile: runtime.contextMaxCompressionTurnsPerCompile,
    summaryInputTokenBudget: runtime.contextSummaryInputTokenBudget,
    summaryMaxOutputTokens: runtime.contextSummaryMaxOutputTokens,
    historicalToolTokenBudget: runtime.contextHistoricalToolTokenBudget,
  })
  const modelRuntime = new ModelRuntime(provider, {
    resolve: async (pageContext, userInput, loadedOperationIds, retrievalState, signal) => {
      const startedAt = performance.now()
      let outcome = "succeeded"
      let strategy = "base_only"
      let candidateCount = 0
      let matchCount = 0
      try {
        return await withSpan("agent.tools.retrieve", internalSpanOptions({
          "luna.agent.tool_retrieval.trigger": "automatic",
        }), async span => {
          const retrieval = tools
            ? await catalog.resolveDetailedAsync(toolRetrievalContext(pageContext, retrievalState), userInput, loadedOperationIds, signal)
            : undefined
          const retrievedTools = retrieval?.tools ?? []
          outcome = retrieval?.retrieval.outcome ?? outcome
          strategy = retrieval?.retrieval.strategy ?? strategy
          candidateCount = retrieval?.retrieval.totalMatches ?? 0
          matchCount = retrievedTools.length
          if (!tools) outcome = "unavailable"
          const platformTools = tools
            ? config.TOOL_RETRIEVAL_MODE === "dynamic" ? retrievedTools : catalog.allowedModelTools()
            : []
          if (tools && retrievedTools.length === 0) {
            outcome = "unavailable"
            telemetryLog("agent.tool_retrieval.unavailable", "warn", {
              "error.code": "ai.tool_retrieval_unavailable",
            })
          }
          span.setAttribute("luna.agent.tool_retrieval.outcome", outcome)
          span.setAttribute("luna.agent.tool_retrieval.strategy", strategy)
          span.setAttribute("luna.agent.tool_retrieval.match_count", matchCount)
          span.setAttribute("luna.agent.tool_retrieval.mode", config.TOOL_RETRIEVAL_MODE)
          return [
            ...platformTools,
            ...(tools ? [browseToolsTool, searchToolsTool] : []),
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
        const attributes = { outcome, mode: config.TOOL_RETRIEVAL_MODE, strategy }
        agentMetrics.toolRetrievals.add(1, attributes)
        agentMetrics.toolRetrievalCandidates.record(candidateCount, attributes)
        agentMetrics.toolRetrievalLoaded.record(matchCount, attributes)
        agentMetrics.toolRetrievalDuration.record((performance.now() - startedAt) / 1000, attributes)
      }
    },
    search: (query, pageContext, limit, retrievalState, signal) => catalog.searchAsync(
      query,
      toolRetrievalContext(pageContext, retrievalState),
      limit,
      signal,
    ),
    browse: request => catalog.browse(request),
  }, contextCompiler)
  const executor = new RunExecutor(repository, modelRuntime, config, tools, providerConfigClient)
  const server = buildServer({
    config, repository, authenticator, provider, grantCipher,
    cancelRun: runId => { executor.cancel(runId) },
    toolCatalogDigest: catalog.digest,
    ...(tools ? { tools } : {}),
    ...(providerConfigClient ? { providerConfigClient } : {}),
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
    if (repository instanceof PostgresRepository) await repository.close()
    await shutdownTelemetry()
  }
  process.once("SIGTERM", () => { void shutdown() })
  process.once("SIGINT", () => { void shutdown() })
}

function toolRetrievalContext(
  pageContext: Record<string, unknown>,
  state?: {
    resourceContext: string[]
    completedOperations: string[]
    stableOutcomes: string[]
    pendingState?: "user_input" | "approval" | "mfa" | "async_terminal_check"
    stableErrorCodes: string[]
  },
): RetrievalContext {
  return {
    ...(typeof pageContext.projectId === "string" ? { projectId: pageContext.projectId } : {}),
    ...(typeof pageContext.pathname === "string" ? { pathname: pageContext.pathname } : {}),
    ...(typeof pageContext.routeName === "string" ? { routeName: pageContext.routeName } : {}),
    ...(state?.resourceContext.length ? { resourceTypes: state.resourceContext } : {}),
    ...(state?.completedOperations.length ? { completedOperations: state.completedOperations } : {}),
    ...(state?.stableOutcomes.length ? { stableOutcomes: state.stableOutcomes } : {}),
    ...(state?.pendingState ? { pendingState: state.pendingState } : {}),
    ...(state?.stableErrorCodes.length ? { stableErrorCodes: state.stableErrorCodes } : {}),
  }
}
