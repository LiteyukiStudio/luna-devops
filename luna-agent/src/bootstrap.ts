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
import { buildServer } from "./server.js"
import { configureAIContentCapture, shutdownTelemetry, telemetryLog } from "./telemetry.js"
import { defaultRuntimeSettings } from "./runtime-settings.js"
import { ToolCatalog } from "./tools/catalog.js"
import { HttpLunaApiToolClient } from "./tools/luna-api-client.js"
import { MemoryToolCallStore, ProjectingToolCallStore, ToolOrchestrator } from "./tools/orchestrator.js"
import { PostgresToolCallStore } from "./tools/postgres-store.js"
import { platformOperations } from "./tools/generated/platform.js"
import { createInteractionCardsTool } from "./tools/ui-cards.js"
import { createOptionsTool } from "./tools/ui-options.js"
import { navigateToRouteTool } from "./tools/ui-route.js"
import { searchToolsTool } from "./tools/tool-search.js"

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
  const initialRemoteConfig = providerConfigClient
    ? await providerConfigClient.get()
    : undefined
  const provider = createRuntimeProvider(config, providerConfigClient)
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
      })
    : undefined
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
    resolve: (pageContext, userInput, loadedOperationIds) => [
    ...(tools
      ? catalog.resolve({
          ...(typeof pageContext.projectId === "string" ? { projectId: pageContext.projectId } : {}),
          ...(typeof pageContext.pathname === "string" ? { pathname: pageContext.pathname } : {}),
          ...(typeof pageContext.routeName === "string" ? { routeName: pageContext.routeName } : {}),
        }, userInput, loadedOperationIds)
      : []),
    ...(tools ? [searchToolsTool] : []),
    createOptionsTool,
    createInteractionCardsTool,
    navigateToRouteTool,
    ],
    search: (query, pageContext, limit) => catalog.search(query, {
      ...(typeof pageContext.projectId === "string" ? { projectId: pageContext.projectId } : {}),
      ...(typeof pageContext.pathname === "string" ? { pathname: pageContext.pathname } : {}),
      ...(typeof pageContext.routeName === "string" ? { routeName: pageContext.routeName } : {}),
    }, limit),
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
