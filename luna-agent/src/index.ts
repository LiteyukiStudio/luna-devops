import { randomBytes } from "node:crypto"
import { BffHmacAuthenticator, DevelopmentAuthenticator, JwtAuthenticator } from "./auth.js"
import { loadConfig } from "./config.js"
import { RunExecutor } from "./executor.js"
import { GraphVersionRegistry } from "./graph/registry.js"
import { RunGrantCipher } from "./grant-cipher.js"
import { MemoryRepository } from "./persistence/memory.js"
import { PostgresRepository } from "./persistence/postgres.js"
import { ProviderConfigClient } from "./provider/config-client.js"
import { createRuntimeProvider } from "./provider/runtime.js"
import { buildServer } from "./server.js"
import { ToolCatalog } from "./tools/catalog.js"
import { HttpLunaApiToolClient } from "./tools/luna-api-client.js"
import { MemoryToolCallStore, ProjectingToolCallStore, ToolOrchestrator } from "./tools/orchestrator.js"
import { PostgresToolCallStore } from "./tools/postgres-store.js"
import { platformOperations } from "./tools/generated/platform.js"
import { createOptionsTool } from "./tools/ui-options.js"
import { navigateToRouteTool } from "./tools/ui-route.js"

const config = loadConfig()
const repository = config.DATABASE_URL ? new PostgresRepository(config.DATABASE_URL) : new MemoryRepository()
const providerConfigClient = config.LUNA_API_BASE_URL && config.AI_AGENT_CALLBACK_SERVICE_TOKEN
  ? new ProviderConfigClient(config.LUNA_API_BASE_URL, config.AI_AGENT_CALLBACK_SERVICE_TOKEN)
  : undefined
const provider = createRuntimeProvider(config, providerConfigClient)
const authenticator = config.AUTH_MODE === "jwt"
  ? await JwtAuthenticator.create(config.API_SERVICE_JWT_PUBLIC_KEY!, config.ACTOR_CONTEXT_PUBLIC_KEY!)
  : config.AUTH_MODE === "bff-hmac"
    ? new BffHmacAuthenticator(config.API_SERVICE_TOKEN!, config.ACTOR_CONTEXT_SIGNING_KEY!)
    : new DevelopmentAuthenticator()
const grantCipher = new RunGrantCipher(config.RUN_GRANT_ENCRYPTION_KEY_BASE64 ? Buffer.from(config.RUN_GRANT_ENCRYPTION_KEY_BASE64, "base64") : randomBytes(32))
const catalog = ToolCatalog.load(config.TOOL_CATALOG_JSON ? JSON.parse(config.TOOL_CATALOG_JSON) as unknown : platformOperations)
const toolStore = repository instanceof PostgresRepository ? new PostgresToolCallStore(repository.pool, repository) : new ProjectingToolCallStore(new MemoryToolCallStore(), repository)
const tools = catalog && config.LUNA_API_BASE_URL && config.AI_AGENT_CALLBACK_SERVICE_TOKEN
  ? new ToolOrchestrator(catalog, new HttpLunaApiToolClient(config.LUNA_API_BASE_URL, config.AI_AGENT_CALLBACK_SERVICE_TOKEN), toolStore, undefined, 12, undefined, async runId => {
      const encrypted = await repository.getRunActorGrantCiphertext(runId)
      if (!encrypted) throw new Error("ai.run_grant_unavailable")
      return grantCipher.decrypt(encrypted)
    })
  : undefined
const graphs = new GraphVersionRegistry(provider, pageContext => [
  ...(tools
    ? catalog.modelTools(typeof pageContext.projectId === "string" ? { projectId: pageContext.projectId } : {})
    : []),
  createOptionsTool,
  navigateToRouteTool,
])
const executor = new RunExecutor(repository, graphs, config, tools)
const server = buildServer({
  config, repository, authenticator, provider, graphVersions: graphs.versions(), grantCipher,
  cancelRun: runId => { executor.cancel(runId) },
  toolCatalogDigest: catalog.digest,
  ...(tools ? { tools } : {}),
  ...(providerConfigClient ? { providerConfigClient } : {}),
})

executor.start()
await server.listen({ host: config.HOST, port: config.PORT })

async function shutdown() {
  await executor.stop()
  await server.close()
  if (repository instanceof PostgresRepository) await repository.close()
}
process.once("SIGTERM", () => { void shutdown() })
process.once("SIGINT", () => { void shutdown() })
