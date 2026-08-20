import { describe, expect, it } from "vitest"
import { ToolCatalog } from "../src/tools/catalog.js"
import { DeterministicLunaApiClient } from "../src/tools/luna-api-client.js"
import { MemoryToolCallStore, SensitiveInputRejected, ToolOrchestrator, type ToolInterruption } from "../src/tools/orchestrator.js"
import { responseToolContract } from "./tool-contract-fixtures.js"

const catalog = ToolCatalog.load([
  {
    operationId: "getBuildRun", method: "GET", path: "/api/v1/builds", category: "build",
    risk: "read", requiredScopes: ["build:read"], approval: "never", idempotent: true, timeoutMs: 5000,
    contract: responseToolContract({ resourceTypes: ["build-run"] }),
    inputSchema: { type: "object", properties: { buildId: { type: "string", maxLength: 64 } }, required: ["buildId"], additionalProperties: false },
  },
  {
    operationId: "restartRelease", method: "POST", path: "/api/v1/releases/restart", category: "deployment",
    risk: "destructive", requiredScopes: ["deployment:write"], approval: "always", stepUpPurpose: "deployment_restart",
    idempotent: true, timeoutMs: 5000,
    contract: responseToolContract({
      resourceTypes: ["release"], action: "execute", sideEffect: "platform-write", replaySafe: false,
      risk: "high", approval: "always", mfaPurpose: "deployment_restart", avoidWhen: ["未取得明确目标时"], prerequisites: ["必须取得真实 releaseId"],
    }),
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
  it("requires a Direct Tool Action for sensitive arguments", async () => {
    const sensitiveCatalog = ToolCatalog.load([{
      operationId: "updateRuntimeSecret", method: "PUT", path: "/api/v1/runtime-secrets", category: "deployment",
      risk: "sensitive", requiredScopes: ["deployment:update"], approval: "always", stepUpPurpose: "secret_update", idempotent: true, timeoutMs: 5000,
      contract: responseToolContract({
        resourceTypes: ["runtime-secret"], action: "update", sideEffect: "platform-write", replaySafe: false,
        risk: "high", approval: "always", mfaPurpose: "secret_update", avoidWhen: ["没有安全表单输入时"], prerequisites: ["必须由用户安全提交"],
      }),
      inputSchema: {
        type: "object",
        properties: { values: { type: "object", writeOnly: true, "x-luna-sensitive": true, additionalProperties: { type: "string" } } },
        required: ["values"], additionalProperties: false,
      },
    }])
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: { accepted: true } }))
    const store = new MemoryToolCallStore()
    const orchestrator = new ToolOrchestrator(sensitiveCatalog, client, store, undefined, undefined, resolveGrant)
    await expect(orchestrator.propose({ runId: "airun_test", operationId: "updateRuntimeSecret", arguments: { values: { TOKEN: "secret-value" } } }))
      .rejects.toBeInstanceOf(SensitiveInputRejected)
    await expect(orchestrator.propose({ runId: "airun_test", operationId: "updateRuntimeSecret", arguments: { values: { TOKEN: "secret-value" } }, inputMode: "direct" }))
      .resolves.toMatchObject({ status: "awaiting_approval", inputMode: "direct" })
    expect(client.calls).toHaveLength(0)
    expect(store.records.size).toBe(1)
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
    const client = new DeterministicLunaApiClient(request => request.stepUpAssertionId
      ? { status: 200, body: { restarted: true } }
      : { status: 428, body: { code: "mfa_required", purpose: "deployment_restart" } })
    const orchestrator = new ToolOrchestrator(catalog, client, new MemoryToolCallStore(), undefined, undefined, resolveGrant)
    const pending = await orchestrator.propose({ runId: "airun_test", operationId: "restartRelease", arguments: { releaseId: "rel_a" } })
    expect(pending.status).toBe("awaiting_approval")
    const mfa = await orchestrator.approve(pending.id, pending.argumentsHash, pending.rowVersion)
    expect(mfa).toMatchObject({ status: "awaiting_mfa", mfaPurpose: "deployment_restart" })
    const completed = await orchestrator.resumeMfa(mfa.id, "deployment_restart", mfa.rowVersion, "mfa_assertion_1")
    expect(completed.status).toBe("succeeded")
    expect(client.calls).toHaveLength(2)
    expect(client.calls[0]).toMatchObject({ approvalGranted: true })
    expect(client.calls[1]).toMatchObject({ approvalGranted: true, mfaPurpose: "deployment_restart", stepUpAssertionId: "mfa_assertion_1" })
  })
  it("executes the approved original arguments while only emitting a redacted projection", async () => {
    const sensitiveCatalog = ToolCatalog.load([{
      operationId: "installTemplate", method: "POST", path: "/api/v1/templates/install", category: "application",
      risk: "sensitive", requiredScopes: ["application:write"], approval: "always", idempotent: true, timeoutMs: 5000,
      contract: responseToolContract({
        resourceTypes: ["application"], action: "create", sideEffect: "platform-write", replaySafe: false,
        risk: "high", approval: "always", avoidWhen: ["参数不完整时"], prerequisites: ["必须取得真实 projectId"],
      }),
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
  it("reuses a conversation authorization for later approval-required tools", async () => {
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: { restarted: true } }))
    const orchestrator = new ToolOrchestrator(
      catalog,
      client,
      new MemoryToolCallStore(),
      undefined,
      undefined,
      resolveGrant,
      undefined,
      async () => "conversation-authorization-grant",
    )
    const completed = await orchestrator.propose({
      runId: "airun_later",
      operationId: "restartRelease",
      arguments: { releaseId: "rel_later" },
    })
    expect(completed.status).toBe("succeeded")
    expect(client.calls[0]).toMatchObject({
      approvalGranted: false,
      conversationAuthorizationGrant: "conversation-authorization-grant",
    })
  })
  it("reuses a conversation authorization for MFA-only tools", async () => {
    const mfaOnlyCatalog = ToolCatalog.load([{
      operationId: "viewProtectedLog", method: "GET", path: "/api/v1/logs/protected", category: "logs",
      risk: "read", requiredScopes: ["logs:read"], approval: "never", stepUpPurpose: "secret_view",
      idempotent: true, timeoutMs: 5000,
      contract: responseToolContract({ resourceTypes: ["log"], mfaPurpose: "secret_view" }),
      inputSchema: { type: "object", properties: {}, required: [], additionalProperties: false },
    }])
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: { protected: true } }))
    const orchestrator = new ToolOrchestrator(
      mfaOnlyCatalog,
      client,
      new MemoryToolCallStore(),
      undefined,
      undefined,
      resolveGrant,
      undefined,
      async () => "conversation-authorization-grant",
    )
    const completed = await orchestrator.propose({ runId: "airun_mfa_only", operationId: "viewProtectedLog", arguments: {} })
    expect(completed.status).toBe("succeeded")
    expect(client.calls[0]).toMatchObject({
      approvalGranted: false,
      conversationAuthorizationGrant: "conversation-authorization-grant",
    })
  })
  it("enforces the configured high per-run tool-call ceiling", async () => {
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: {} }))
    const orchestrator = new ToolOrchestrator(catalog, client, new MemoryToolCallStore(), undefined, undefined, resolveGrant)
    orchestrator.setRunMaxToolCalls(32)
    for (let index = 0; index < 32; index += 1) {
      await expect(orchestrator.propose({ runId: "airun_test", operationId: "getBuildRun", arguments: { buildId: `build_${index}` } }))
        .resolves.toMatchObject({ status: "succeeded" })
    }
    await expect(orchestrator.propose({ runId: "airun_test", operationId: "getBuildRun", arguments: { buildId: "overflow" } }))
      .rejects.toMatchObject({ code: "ai.run_tool_call_budget_exceeded", retryable: false })
    expect(client.calls).toHaveLength(32)
    expect(orchestrator.toolLoopSnapshot("airun_test")).toEqual({ proposed: 33, executed: 32, maxToolCalls: 32 })
  })
  it("returns invalid present values to the model as structured repairable failures", async () => {
    const constrainedCatalog = ToolCatalog.load([{
      operationId: "listRuntimeClusterResources", method: "GET", path: "/api/v1/runtime/resources", category: "runtime",
      risk: "read", requiredScopes: ["runtime:read"], approval: "never", idempotent: true, timeoutMs: 5000,
      contract: responseToolContract({ resourceTypes: ["runtime-resource"] }),
      inputSchema: {
        type: "object",
        properties: { resourceCategory: { type: "string", enum: ["namespaces", "workloads", "services", "configs", "storage"] } },
        required: ["resourceCategory"],
        additionalProperties: false,
      },
    }])
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: {} }))
    const orchestrator = new ToolOrchestrator(constrainedCatalog, client, new MemoryToolCallStore(), undefined, undefined, resolveGrant)
    const failed = await orchestrator.propose({ runId: "airun_invalid", operationId: "listRuntimeClusterResources", arguments: { resourceCategory: "Deployment" } })
    expect(failed).toMatchObject({
      status: "failed",
      errorCode: "ai.tool_arguments_invalid",
      result: {
        code: "ai.tool_arguments_invalid",
        retryable: true,
        issues: [{
          path: "/resourceCategory",
          code: "enum",
          allowedValues: ["namespaces", "workloads", "services", "configs", "storage"],
        }],
      },
    })
    expect(client.calls).toHaveLength(0)
    await expect(orchestrator.propose({ runId: "airun_invalid", operationId: "listRuntimeClusterResources", arguments: { resourceCategory: "Deployment" } }))
      .rejects.toMatchObject({ code: "ai.tool_deterministic_failure_repeated", retryable: false })
    await expect(orchestrator.propose({ runId: "airun_invalid", operationId: "listRuntimeClusterResources", arguments: { resourceCategory: "workloads" } }))
      .resolves.toMatchObject({ status: "succeeded" })
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
