import { describe, expect, it } from "vitest"
import { ToolCatalog } from "../src/tools/catalog.js"
import { DeterministicLunaApiClient } from "../src/tools/luna-api-client.js"
import { MemoryToolCallStore, ToolOrchestrator, type ToolInterruption } from "../src/tools/orchestrator.js"

const catalog = ToolCatalog.load([
  {
    operationId: "getBuildRun", method: "GET", path: "/api/v1/builds", category: "build",
    risk: "read", requiredScopes: ["build:read"], approval: "never", idempotent: true, timeoutMs: 5000,
    inputSchema: { type: "object", properties: { buildId: { type: "string", maxLength: 64 } }, required: ["buildId"], additionalProperties: false },
  },
  {
    operationId: "restartRelease", method: "POST", path: "/api/v1/releases/restart", category: "deployment",
    risk: "destructive", requiredScopes: ["deployment:write"], approval: "always", stepUpPurpose: "deployment_restart",
    idempotent: true, timeoutMs: 5000,
    inputSchema: { type: "object", properties: { releaseId: { type: "string" } }, required: ["releaseId"], additionalProperties: false },
  },
])
const resolveGrant = async () => "opaque-test-run-grant"

describe("tool catalog and orchestration", () => {
  it("rejects undeclared arguments and asks for required input", async () => {
    const orchestrator = new ToolOrchestrator(catalog, new DeterministicLunaApiClient(() => ({ status: 200, body: {} })), new MemoryToolCallStore())
    await expect(orchestrator.propose({ runId: "airun_test", operationId: "getBuildRun", arguments: {} }))
      .rejects.toMatchObject({ state: "waiting_input", fields: ["buildId"] } satisfies Partial<ToolInterruption>)
  })
  it("executes a read tool only through the Luna API client", async () => {
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: { id: "build_a", status: "failed", token: "must-hide" } }))
    const store = new MemoryToolCallStore()
    const result = await new ToolOrchestrator(catalog, client, store, undefined, undefined, resolveGrant).propose({ runId: "airun_test", operationId: "getBuildRun", arguments: { buildId: "build_a" } })
    expect(result.status).toBe("succeeded")
    expect(result.result).toMatchObject({ token: "[REDACTED]" })
    expect(client.calls).toHaveLength(1)
    expect(client.calls[0]?.approvalGranted).toBe(false)
    expect(store.events.map(event => event.type)).toEqual(["tool.started", "tool_call.running", "tool_call.succeeded"])
    expect(store.events.at(-1)?.data.durationMs).toEqual(expect.any(Number))
  })
  it("binds approval to arguments and requires MFA separately", async () => {
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: { restarted: true } }))
    const orchestrator = new ToolOrchestrator(catalog, client, new MemoryToolCallStore(), undefined, undefined, resolveGrant)
    const pending = await orchestrator.propose({ runId: "airun_test", operationId: "restartRelease", arguments: { releaseId: "rel_a" } })
    expect(pending.status).toBe("awaiting_approval")
    const mfa = await orchestrator.approve(pending.id, pending.argumentsHash, pending.rowVersion)
    expect(mfa).toMatchObject({ status: "awaiting_mfa", mfaPurpose: "deployment_restart" })
    const completed = await orchestrator.resumeMfa(mfa.id, "deployment_restart", mfa.rowVersion, "mfa_assertion_1")
    expect(completed.status).toBe("succeeded")
    expect(client.calls[0]).toMatchObject({ approvalGranted: true, mfaPurpose: "deployment_restart", stepUpAssertionId: "mfa_assertion_1" })
  })
  it("executes the approved original arguments while only emitting a redacted projection", async () => {
    const sensitiveCatalog = ToolCatalog.load([{
      operationId: "installTemplate", method: "POST", path: "/api/v1/templates/install", category: "application",
      risk: "sensitive", requiredScopes: ["application:write"], approval: "always", idempotent: true, timeoutMs: 5000,
      inputSchema: {
        type: "object",
        properties: {
          projectId: { type: "string" },
          body: { type: "object", additionalProperties: true },
        },
        required: ["projectId", "body"],
        additionalProperties: false,
      },
    }])
    const sensitiveClient = new DeterministicLunaApiClient(() => ({ status: 200, body: { installed: true } }))
    const sensitiveStore = new MemoryToolCallStore()
    const sensitive = new ToolOrchestrator(sensitiveCatalog, sensitiveClient, sensitiveStore, undefined, undefined, resolveGrant)
    const approval = await sensitive.propose({
      runId: "airun_secret",
      operationId: "installTemplate",
      arguments: { projectId: "prj_1", body: { username: "app", password: "generated-secret" } },
    })
    await sensitive.approve(approval.id, approval.argumentsHash, approval.rowVersion)

    expect(sensitiveClient.calls[0]?.arguments).toEqual({
      projectId: "prj_1",
      body: { username: "app", password: "generated-secret" },
    })
    expect(sensitiveStore.events[0]?.data.arguments).toEqual({
      projectId: "prj_1",
      body: { username: "app", password: "[REDACTED]" },
    })
  })
  it("does not impose a per-run tool-call ceiling", async () => {
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: {} }))
    const orchestrator = new ToolOrchestrator(catalog, client, new MemoryToolCallStore(), undefined, undefined, resolveGrant)
    for (let index = 0; index < 65; index += 1) {
      await expect(orchestrator.propose({ runId: "airun_test", operationId: "getBuildRun", arguments: { buildId: `build_${index}` } }))
        .resolves.toMatchObject({ status: "succeeded" })
    }
    expect(client.calls).toHaveLength(65)
  })
  it("fails when final-state verification is inconclusive", async () => {
    const client = new DeterministicLunaApiClient(() => ({ status: 202, body: { accepted: true } }))
    const verifier = { verify: async () => ({ ok: false, code: "verification_inconclusive" }) }
    const orchestrator = new ToolOrchestrator(catalog, client, new MemoryToolCallStore(), undefined, verifier, resolveGrant)
    const result = await orchestrator.propose({ runId: "airun_test", operationId: "getBuildRun", arguments: { buildId: "a" } })
    expect(result).toMatchObject({ status: "failed", errorCode: "verification_inconclusive" })
  })
  it("retains the Luna API request id for a failed tool without exposing internal details", async () => {
    const client = new DeterministicLunaApiClient(() => ({
      status: 503,
      body: { code: "ai.tool_storage_unavailable", error: "Service unavailable" },
      requestId: "req_tool_failure",
    }))
    const result = await new ToolOrchestrator(catalog, client, new MemoryToolCallStore(), undefined, undefined, resolveGrant)
      .propose({ runId: "airun_test", operationId: "getBuildRun", arguments: { buildId: "a" } })

    expect(result).toMatchObject({
      status: "failed",
      errorCode: "ai.tool_storage_unavailable",
      result: {
        code: "ai.tool_storage_unavailable",
        requestId: "req_tool_failure",
      },
    })
    expect(JSON.stringify(result.result)).not.toContain("driver")
  })
  it("creates an immutable new attempt when a failed read is retried", async () => {
    let attempts = 0
    const client = new DeterministicLunaApiClient(() => ++attempts === 1
      ? { status: 503, body: { code: "provider_unavailable" } }
      : { status: 200, body: { ok: true } })
    const store = new MemoryToolCallStore()
    const orchestrator = new ToolOrchestrator(catalog, client, store, undefined, undefined, resolveGrant)
    const failed = await orchestrator.propose({ runId: "airun_retry", operationId: "getBuildRun", arguments: { buildId: "a" } })
    const retried = await orchestrator.retryFailed(failed.id)
    expect(retried).toMatchObject({ status: "succeeded", attempt: 2, argumentsHash: failed.argumentsHash })
    expect(retried.id).not.toBe(failed.id)
    expect(store.records.get(failed.id)?.status).toBe("failed")
  })
})

describe("tool catalog validation", () => {
  it("allows handler-driven MFA for a high-risk operation while keeping approval mandatory", () => {
    expect(ToolCatalog.load([{
      operationId: "deleteThing", method: "DELETE", path: "/api/v1/things", category: "thing",
      risk: "destructive", requiredScopes: [], approval: "always", idempotent: true, timeoutMs: 1000,
      inputSchema: { type: "object", properties: {}, required: [], additionalProperties: false },
    }]).get("deleteThing")).toMatchObject({ approval: "always" })
  })
  it("rejects a high-risk operation that disables approval", () => {
    expect(() => ToolCatalog.load([{
      operationId: "deleteThing", method: "DELETE", path: "/api/v1/things", category: "thing",
      risk: "destructive", requiredScopes: [], approval: "never", stepUpPurpose: "thing_delete",
      idempotent: true, timeoutMs: 1000,
      inputSchema: { type: "object", properties: {}, required: [], additionalProperties: false },
    }])).toThrow()
  })
})
