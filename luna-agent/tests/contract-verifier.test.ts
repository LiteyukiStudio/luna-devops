import { describe, expect, it } from "vitest"
import { ToolCatalog, type ToolOperation } from "../src/tools/catalog.js"
import { DeterministicLunaApiClient } from "../src/tools/luna-api-client.js"
import { MemoryToolCallStore, ToolOrchestrator } from "../src/tools/orchestrator.js"

const responseContract: NonNullable<ToolOperation["contract"]> = {
  allowed: true,
  resourceTypes: ["release"],
  action: "verify",
  sideEffect: "none",
  idempotent: true,
  replaySafe: true,
  risk: "low",
  approval: "never",
  intents: ["验收发布"],
  useWhen: ["已有发布 ID，需要读取权威状态"],
  avoidWhen: ["没有发布 ID"],
  prerequisites: ["已有 projectId 和 releaseId"],
  parameterSummary: ["projectId 与 releaseId"],
  successEvidence: ["返回当前发布状态"],
  commonErrorCodes: [],
  predecessors: ["createRelease"],
  followups: [],
  verification: { mode: "response", successCodes: [200] },
}

const createContract: NonNullable<ToolOperation["contract"]> = {
  allowed: true,
  resourceTypes: ["release"],
  action: "execute",
  sideEffect: "external-write",
  idempotent: false,
  replaySafe: false,
  risk: "medium",
  approval: "never",
  intents: ["创建发布"],
  useWhen: ["已经确认部署目标与镜像"],
  avoidWhen: ["只是查看发布状态"],
  prerequisites: ["已有 projectId 与部署目标"],
  parameterSummary: ["projectId 与发布配置"],
  successEvidence: ["getRelease 回读到 succeeded"],
  commonErrorCodes: [],
  predecessors: [],
  followups: ["getRelease"],
  verification: {
    mode: "async-readback",
    operationId: "getRelease",
    idSource: "/id",
    argumentBindings: { projectId: "/projectId", releaseId: "/id" },
    completion: {
      mode: "state",
      path: "/status",
      pendingStates: ["pending", "running"],
      successStates: ["succeeded"],
      failureStates: ["failed", "canceled"],
    },
  },
}

function operation(
  operationId: string,
  method: ToolOperation["method"],
  contract: ToolOperation["contract"],
  required: string[],
): ToolOperation {
  return {
    operationId,
    method,
    path: operationId === "createRelease"
      ? "/api/v1/projects/{projectId}/releases"
      : "/api/v1/projects/{projectId}/releases/{releaseId}",
    category: "releases",
    description: `${operationId} 测试工具`,
    risk: method === "GET" ? "read" : "write",
    requiredScopes: ["deployment:read"],
    approval: "never",
    idempotent: method === "GET",
    timeoutMs: 30_000,
    inputSchema: {
      type: "object",
      properties: Object.fromEntries(required.map(key => [key, { type: "string", minLength: 1 }])),
      required,
      additionalProperties: false,
    },
    contract,
  }
}

function catalog() {
  return ToolCatalog.load([
    operation("createRelease", "POST", createContract, ["projectId"]),
    operation("getRelease", "GET", responseContract, ["projectId", "releaseId"]),
  ])
}

function readVerification(result: unknown) {
  return (result as { lunaVerification?: Record<string, unknown> }).lunaVerification
}

function delegated(operationId: string, result: Record<string, unknown>, verified: boolean) {
  return { operationId, accepted: true, verified, result, verificationRequired: !verified }
}

describe("contract result verifier", () => {
  it("performs the declared authoritative readback and reports a pending async state", async () => {
    const client = new DeterministicLunaApiClient(request => request.operation.operationId === "createRelease"
      ? { status: 201, body: delegated("createRelease", { id: "release-1", projectId: "project-1", status: "pending" }, false) }
      : { status: 200, body: delegated("getRelease", { id: "release-1", projectId: "project-1", status: "running" }, true) })
    const store = new MemoryToolCallStore()
    const orchestrator = new ToolOrchestrator(catalog(), client, store, undefined, undefined, async () => "grant")

    const call = await orchestrator.propose({ runId: "run-1", operationId: "createRelease", arguments: { projectId: "project-1" } })

    expect(call.status).toBe("succeeded")
    expect(client.calls.map(item => item.operation.operationId)).toEqual(["createRelease", "getRelease"])
    expect(client.calls[1]?.arguments).toEqual({ projectId: "project-1", releaseId: "release-1" })
    expect(readVerification(call.result)).toMatchObject({ operationId: "getRelease", state: "running", status: "pending" })
    expect([...store.records.values()].some(item => item.operationId === "getRelease" && item.status === "succeeded")).toBe(true)
  })

  it("fails the write call when the authoritative readback reaches a failure terminal state", async () => {
    const client = new DeterministicLunaApiClient(request => request.operation.operationId === "createRelease"
      ? { status: 201, body: delegated("createRelease", { id: "release-2", projectId: "project-1" }, false) }
      : { status: 200, body: delegated("getRelease", { id: "release-2", status: "failed" }, true) })
    const orchestrator = new ToolOrchestrator(catalog(), client, new MemoryToolCallStore(), undefined, undefined, async () => "grant")

    const call = await orchestrator.propose({ runId: "run-2", operationId: "createRelease", arguments: { projectId: "project-1" } })

    expect(call).toMatchObject({ status: "failed", errorCode: "ai.tool_verification_terminal_failure" })
    expect(readVerification(call.result)).toMatchObject({ state: "failed", status: "failed" })
  })

  it("enforces response success codes instead of accepting every 2xx", async () => {
    const client = new DeterministicLunaApiClient(() => ({ status: 201, body: delegated("getRelease", { status: "succeeded" }, true) }))
    const orchestrator = new ToolOrchestrator(catalog(), client, new MemoryToolCallStore(), undefined, undefined, async () => "grant")

    const call = await orchestrator.propose({
      runId: "run-3",
      operationId: "getRelease",
      arguments: { projectId: "project-1", releaseId: "release-3" },
    })

    expect(call).toMatchObject({ status: "failed", errorCode: "ai.tool_response_status_unexpected" })
    expect(client.calls).toHaveLength(1)
  })

  it("fails closed when the write response omits the declared authoritative id", async () => {
    const client = new DeterministicLunaApiClient(() => ({ status: 201, body: { projectId: "project-1", status: "pending" } }))
    const orchestrator = new ToolOrchestrator(catalog(), client, new MemoryToolCallStore(), undefined, undefined, async () => "grant")

    const call = await orchestrator.propose({ runId: "run-4", operationId: "createRelease", arguments: { projectId: "project-1" } })

    expect(call).toMatchObject({ status: "failed", errorCode: "ai.tool_verification_id_missing" })
    expect(client.calls.map(item => item.operation.operationId)).toEqual(["createRelease"])
  })
})
