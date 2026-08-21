import { describe, expect, it } from "vitest"
import { TestRepository } from "./support/test-repository.js"
import { ToolCatalog } from "../src/tools/catalog.js"
import { ToolCatalogRegistry } from "../src/tools/catalog-registry.js"
import { DeterministicLunaApiClient } from "../src/tools/luna-api-client.js"
import { MemoryToolCallStore, ToolOrchestrator } from "../src/tools/orchestrator.js"

describe("Run tool selection and catalog snapshots", () => {
  it("uses deterministic LRU ordering with a hard cap", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_a", "lru")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: "load", pageContext: {}, idempotencyKey: "selection-lru", actorSessionId: "ses_a",
    })
    const first = Array.from({ length: 16 }, (_, index) => `getTool${index}`)
    expect((await repository.touchRunSelectedOperations(created.run.id, first, 16)).selectedOperationIds).toEqual(first)
    expect((await repository.touchRunSelectedOperations(created.run.id, ["getTool2"], 16)).selectedOperationIds.at(-1)).toBe("getTool2")

    const overflow = await repository.touchRunSelectedOperations(created.run.id, ["getTool16", "getTool17"], 16)
    expect(overflow.evictedOperationIds).toEqual(["getTool0", "getTool1"])
    expect(overflow.selectedOperationIds).toHaveLength(16)
    expect(overflow.selectedOperationIds.slice(-3)).toEqual(["getTool2", "getTool16", "getTool17"])
  })

  it("builds a new catalog before atomic publication and preserves old snapshots", () => {
    const oldCatalog = ToolCatalog.load([operation("getProject")])
    const registry = new ToolCatalogRegistry(oldCatalog, "cfg-old")
    const refresh = registry.refresh([operation("listProjects")], "cfg-new")

    expect(refresh.changed).toBe(true)
    expect(registry.current().get("listProjects").operationId).toBe("listProjects")
    expect(registry.get(oldCatalog.digest).get("getProject").operationId).toBe("getProject")
    expect(() => registry.refresh([{ invalid: true }], "cfg-invalid")).toThrow()
    expect(registry.current().get("listProjects").operationId).toBe("listProjects")
  })

  it("keeps old and new Runs isolated for search and execution", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_a", "snapshots")
    const oldCatalog = ToolCatalog.load([operation("getProject")])
    const registry = new ToolCatalogRegistry(oldCatalog, "cfg-old")
    const oldRun = await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: "old", pageContext: {}, idempotencyKey: "catalog-old-run",
      actorSessionId: "ses_a", toolCatalogDigest: oldCatalog.digest,
    })
    registry.refresh([operation("listProjects")], "cfg-new")
    const newRun = await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: "new", pageContext: {}, idempotencyKey: "catalog-new-run",
      actorSessionId: "ses_a", toolCatalogDigest: registry.digest(),
    })
    const orchestrator = new ToolOrchestrator(async (runId) => {
      const state = await repository.getRunToolState(runId)
      if (!state) throw new Error("ai.run_not_found")
      return registry.get(state.toolCatalogDigest)
    }, new DeterministicLunaApiClient(() => ({ status: 200, body: { ok: true } })), new MemoryToolCallStore())

    expect(registry.get(oldRun.run.toolCatalogDigest).search({ query: "getProject" }).items[0]?.operationId).toBe("getProject")
    expect(registry.get(newRun.run.toolCatalogDigest).search({ query: "listProjects" }).items[0]?.operationId).toBe("listProjects")
    await expect(orchestrator.propose({ runId: oldRun.run.id, operationId: "getProject", arguments: {} })).resolves.toMatchObject({ status: "succeeded" })
    await expect(orchestrator.propose({ runId: newRun.run.id, operationId: "listProjects", arguments: {} })).resolves.toMatchObject({ status: "succeeded" })
    await expect(orchestrator.propose({ runId: oldRun.run.id, operationId: "listProjects", arguments: {} })).rejects.toThrow("ai.tool_not_available")
  })
})

function operation(operationId: string) {
  return {
    operationId,
    name: operationId,
    summary: operationId,
    category: "projects",
    tags: ["Projects"],
    aliases: { zh: [], en: [] },
    purpose: { zh: "", en: "" },
    avoidWhen: { zh: "", en: "" },
    preconditions: { zh: [], en: [] },
    successEvidence: { zh: "", en: "" },
    requiresApproval: false,
    idempotent: true,
    method: "GET" as const,
    path: `/api/v1/test/${operationId}`,
    requiredScopes: ["project:read"],
    inputSchema: { type: "object" as const, properties: {}, required: [], additionalProperties: false as const },
    outputSchema: { type: "object" },
    sensitivePaths: [], parameters: [], requestBody: false, requestRequired: false, requestType: "",
  }
}
