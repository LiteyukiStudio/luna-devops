import { describe, expect, it } from "vitest"
import { loadConfig } from "../src/config.js"
import { RunExecutor } from "../src/executor.js"
import { GraphVersionRegistry } from "../src/graph/registry.js"
import { MemoryRepository } from "../src/persistence/memory.js"
import { DeterministicProvider } from "../src/provider/deterministic.js"
import { ToolCatalog } from "../src/tools/catalog.js"
import { DeterministicLunaApiClient } from "../src/tools/luna-api-client.js"
import { MemoryToolCallStore, ProjectingToolCallStore, ToolOrchestrator } from "../src/tools/orchestrator.js"

describe("provider to tool to subsequent model invocation", () => {
  it("executes a deterministic read tool and completes the same durable run", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "tool loop")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: 'tool:getBuildRun {"buildId":"build_a"}',
      pageContext: {}, idempotencyKey: "tool-loop-request", runActorGrantCiphertext: "encrypted-test-grant",
    })
    const catalog = ToolCatalog.load([{
      operationId: "getBuildRun", method: "GET", path: "/api/v1/builds", category: "build",
      risk: "read", requiredScopes: ["build:read"], approval: "never", idempotent: true, timeoutMs: 5000,
      inputSchema: { type: "object", properties: { buildId: { type: "string" } }, required: ["buildId"], additionalProperties: false },
    }])
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: { id: "build_a", status: "failed" } }))
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(new MemoryToolCallStore(), repository), undefined, 12, undefined, async () => "opaque-grant")
    const config = loadConfig({ NODE_ENV: "test", INSTANCE_ID: "test-worker" })
    const executor = new RunExecutor(repository, new GraphVersionRegistry(new DeterministicProvider()), config, tools)
    expect(await executor.runOnce()).toBe(true)
    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("completed")
    const timeline = await repository.getTimeline("usr_a", conversation.id)
    expect(timeline?.turns[0]?.items.some(item => item.type === "tool_result")).toBe(true)
    expect(timeline?.turns[0]?.items.some(item => item.type === "assistant_message")).toBe(true)
    expect(client.calls[0]?.runActorGrant).toBe("opaque-grant")
  })
  it("persists approval and MFA interruptions, then resumes the same run", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "restart")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: 'tool:restartRelease {"releaseId":"rel_a"}',
      pageContext: {}, idempotencyKey: "approval-loop", runActorGrantCiphertext: "encrypted",
    })
    const catalog = ToolCatalog.load([{
      operationId: "restartRelease", method: "POST", path: "/api/v1/releases/restart", category: "deployment",
      risk: "destructive", requiredScopes: ["deployment:write"], approval: "always", stepUpPurpose: "deployment_restart",
      idempotent: true, timeoutMs: 5000,
      inputSchema: { type: "object", properties: { releaseId: { type: "string" } }, required: ["releaseId"], additionalProperties: false },
    }])
    const store = new MemoryToolCallStore()
    const tools = new ToolOrchestrator(catalog, new DeterministicLunaApiClient(() => ({ status: 200, body: { restarted: true } })), new ProjectingToolCallStore(store, repository), undefined, 12, undefined, async () => "grant")
    const executor = new RunExecutor(repository, new GraphVersionRegistry(new DeterministicProvider()), loadConfig({ NODE_ENV: "test", INSTANCE_ID: "approval-worker" }), tools)
    await executor.runOnce()
    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("waiting_approval")
    const pending = [...store.records.values()][0]!
    const mfa = await tools.approve(pending.id, pending.argumentsHash, pending.rowVersion)
    await repository.updateRun(created.run.id, "waiting_approval", "waiting_mfa")
    expect(mfa.status).toBe("awaiting_mfa")
    await tools.resumeMfa(mfa.id, "deployment_restart", mfa.rowVersion)
    await repository.updateRun(created.run.id, "waiting_mfa", "queued")
    await executor.runOnce()
    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("completed")
  })

  it("moves missing arguments to waiting_input and resumes with supplied input", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "input")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: "tool:getBuildRun {}", pageContext: {}, idempotencyKey: "input-loop",
    })
    const catalog = ToolCatalog.load([{
      operationId: "getBuildRun", method: "GET", path: "/api/v1/builds", category: "build",
      risk: "read", requiredScopes: ["build:read"], approval: "never", idempotent: true, timeoutMs: 5000,
      inputSchema: { type: "object", properties: { buildId: { type: "string" } }, required: ["buildId"], additionalProperties: false },
    }])
    const tools = new ToolOrchestrator(catalog, new DeterministicLunaApiClient(() => ({ status: 200, body: { id: "build_a" } })), new ProjectingToolCallStore(new MemoryToolCallStore(), repository), undefined, 12, undefined, async () => "grant")
    const executor = new RunExecutor(repository, new GraphVersionRegistry(new DeterministicProvider()), loadConfig({ NODE_ENV: "test", INSTANCE_ID: "input-worker" }), tools)
    await executor.runOnce()
    const waiting = await repository.getRun("usr_a", created.run.id)
    expect(waiting?.status).toBe("waiting_input")
    await repository.appendRunInput(created.run.id, 'tool:getBuildRun {"buildId":"build_a"}')
    await repository.updateRun(created.run.id, "waiting_input", "queued")
    await executor.runOnce()
    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("completed")
  })
})
