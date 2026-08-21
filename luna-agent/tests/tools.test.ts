import { describe, expect, it } from "vitest"
import { ToolCatalog } from "../src/tools/catalog.js"
import { DeterministicLunaApiClient } from "../src/tools/luna-api-client.js"
import { MemoryToolCallStore, ToolOrchestrator, type ToolInterruption } from "../src/tools/orchestrator.js"

const catalog = ToolCatalog.load([
  {
    operationId: "getBuildRun", name: "读取构建", summary: "读取构建状态。", method: "GET", path: "/api/v1/builds/{buildId}", category: "build",
    requiredScopes: ["build:read"], requiresApproval: false, idempotent: true,
    parameters: [{ inputName: "buildId", wireName: "buildId", in: "path", required: true }],
    inputSchema: { type: "object", properties: { buildId: { type: "string", maxLength: 64 } }, required: ["buildId"], additionalProperties: false },
  },
  {
    operationId: "restartRelease", name: "重启发布", summary: "重启指定发布。", method: "POST", path: "/api/v1/releases/{releaseId}/restart", category: "deployment",
    requiredScopes: ["deployment:write"], requiresApproval: true, idempotent: true,
    parameters: [{ inputName: "releaseId", wireName: "releaseId", in: "path", required: true }],
    inputSchema: { type: "object", properties: { releaseId: { type: "string" } }, required: ["releaseId"], additionalProperties: false },
  },
])

describe("tool catalog and orchestration", () => {
  it("asks for missing required input", async () => {
    const orchestrator = new ToolOrchestrator(catalog, new DeterministicLunaApiClient(() => ({ status: 200, body: {} })), new MemoryToolCallStore())
    await expect(orchestrator.propose({ runId: "airun_test", operationId: "getBuildRun", arguments: {} }))
      .rejects.toMatchObject({ state: "waiting_input", fields: ["buildId"] } satisfies Partial<ToolInterruption>)
  })

  it("executes an ordinary tool directly and redacts the stored result", async () => {
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: { id: "build_a", token: "must-hide" } }))
    const store = new MemoryToolCallStore()
    const result = await new ToolOrchestrator(catalog, client, store).propose({
      runId: "airun_test", operationId: "getBuildRun", arguments: { buildId: "build_a" },
    })
    expect(result).toMatchObject({ status: "succeeded", result: { token: "[REDACTED]" } })
    expect(client.calls).toHaveLength(1)
  })

  it("terminates the ToolCall when the transport rejects before returning an HTTP response", async () => {
    const client = new DeterministicLunaApiClient(() => {
      throw new Error("ai.tool_catalog_invalid")
    })
    const store = new MemoryToolCallStore()
    const result = await new ToolOrchestrator(catalog, client, store).propose({
      runId: "airun_test", operationId: "getBuildRun", arguments: { buildId: "build_a" },
    })

    expect(result).toMatchObject({
      status: "failed",
      errorCode: "ai.tool_catalog_invalid",
      result: { code: "ai.tool_catalog_invalid", retryable: false },
    })
    expect(store.records.get(result.id)?.status).toBe("failed")
  })

  it("persists one-call approval before execution", async () => {
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: { restarted: true } }))
    const store = new MemoryToolCallStore()
    const orchestrator = new ToolOrchestrator(catalog, client, store)
    const pending = await orchestrator.propose({ runId: "airun_test", operationId: "restartRelease", arguments: { releaseId: "rel_a" } })
    expect(pending.status).toBe("awaiting_approval")
    const completed = await orchestrator.resolveApproval(pending.id, "approve")
    expect(completed).toMatchObject({ status: "succeeded", approvalDecision: "approve" })
    expect(client.calls).toHaveLength(1)
  })

  it("rejects only the ToolCall and records no side effect", async () => {
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: {} }))
    const orchestrator = new ToolOrchestrator(catalog, client, new MemoryToolCallStore())
    const pending = await orchestrator.propose({ runId: "airun_test", operationId: "restartRelease", arguments: { releaseId: "rel_a" } })
    const rejected = await orchestrator.resolveApproval(pending.id, "reject")
    expect(rejected).toMatchObject({ status: "rejected", errorCode: "ai.tool_rejected" })
    expect(client.calls).toHaveLength(0)
  })

  it("grants approve_always only for the same Run owner and operation", async () => {
    const exemptions = new Set<string>()
    const repository = {
      hasToolApprovalExemption: async (runId: string, operationId: string) => exemptions.has(`${runId}:${operationId}`),
      grantToolApprovalExemption: async (runId: string, operationId: string) => { exemptions.add(`${runId}:${operationId}`) },
    }
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: {} }))
    const orchestrator = new ToolOrchestrator(catalog, client, new MemoryToolCallStore(), undefined, repository)
    const pending = await orchestrator.propose({ runId: "airun_test", operationId: "restartRelease", arguments: { releaseId: "rel_a" } })
    await orchestrator.resolveApproval(pending.id, "approve_always")
    const later = await orchestrator.propose({ runId: "airun_test", operationId: "restartRelease", arguments: { releaseId: "rel_b" } })
    expect(later).toMatchObject({ status: "succeeded", approvalDecision: "approve_always" })
  })

  it("allows model tools to submit schema-sensitive input through the normal approval flow", async () => {
    const sensitive = ToolCatalog.load([{
      operationId: "updateRuntimeSecret", name: "更新密钥", summary: "更新运行密钥。", method: "PUT", path: "/api/v1/runtime-secrets", category: "deployment",
      requiredScopes: ["deployment:update"], requiresApproval: true, idempotent: true, requestBody: true,
      inputSchema: {
        type: "object",
        properties: { values: { type: "object", writeOnly: true, "x-luna-sensitive": true, additionalProperties: { type: "string" } } },
        required: ["values"], additionalProperties: false,
      },
    }])
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: { configured: ["TOKEN"] } }))
    const orchestrator = new ToolOrchestrator(sensitive, client, new MemoryToolCallStore())
    const pending = await orchestrator.propose({ runId: "airun_test", operationId: "updateRuntimeSecret", arguments: { values: { TOKEN: "secret" } } })
    expect(pending.status).toBe("awaiting_approval")
    const completed = await orchestrator.resolveApproval(pending.id, "approve")
    expect(completed).toMatchObject({ status: "succeeded", result: { configured: ["TOKEN"] } })
    expect(client.calls).toHaveLength(1)
  })
})
