import { describe, expect, it } from "vitest"
import { ModelRuntime } from "../src/model-runtime.js"
import type { ModelProvider, ModelToolCall } from "../src/provider/provider.js"
import { ToolCatalog } from "../src/tools/catalog.js"
import { DeterministicLunaApiClient } from "../src/tools/luna-api-client.js"
import { ProjectingToolCallStore, ToolOrchestrator } from "../src/tools/orchestrator.js"
import { defaultRuntimeSettings } from "../src/runtime-settings.js"
import { TestRepository } from "./support/test-repository.js"
import { testRegistry } from "./support/model-tool-registry.js"
import { testExecutor } from "./support/run-executor.js"
import { testToolOperation } from "./support/tool-catalog.js"

const reportedUsage = { status: "reported" as const, value: { inputTokens: 3, outputTokens: 2, totalTokens: 5 } }

function provider(steps: Array<{ text?: string, toolCalls?: ModelToolCall[] }>) {
  let index = 0
  const value: ModelProvider = {
    async *stream() {
      const step = steps[Math.min(index, steps.length - 1)] ?? {}
      index += 1
      if (step.text) yield { type: "message_delta", delta: step.text }
      yield {
        type: "completed",
        usage: reportedUsage,
        ...(step.toolCalls?.length ? { toolCalls: step.toolCalls } : {}),
      }
    },
    async complete() { return { text: "", toolCalls: [], usage: reportedUsage } },
    capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
    health: async () => ({ ok: true }),
  }
  return { value, calls: () => index }
}

function buildCatalog() {
  return ToolCatalog.load([
    testToolOperation("triggerBuildRun", {
      name: "触发构建",
      summary: "提交构建任务。",
      method: "POST",
      path: "/api/v1/projects/{projectId}/build-runs/trigger",
      effect: "async",
      verification: [{ operationId: "getBuildRun", family: "build_run" }],
      parameters: [{ inputName: "projectId", wireName: "projectId", in: "path", required: true }],
      requestBody: true,
      requestRequired: true,
      inputSchema: {
        type: "object",
        properties: { projectId: { type: "string" }, body: { type: "object" } },
        required: ["projectId", "body"],
        additionalProperties: false,
      },
    }),
    testToolOperation("getBuildRun", {
      name: "读取构建",
      summary: "读取构建状态。",
      method: "GET",
      path: "/api/v1/projects/{projectId}/build-runs/{runId}",
      effect: "read",
      parameters: [
        { inputName: "projectId", wireName: "projectId", in: "path", required: true },
        { inputName: "runId", wireName: "runId", in: "path", required: true },
      ],
      inputSchema: {
        type: "object",
        properties: { projectId: { type: "string" }, runId: { type: "string" } },
        required: ["projectId", "runId"],
        additionalProperties: false,
      },
    }),
    testToolOperation("cancelBuildRun", {
      method: "POST",
      path: "/api/v1/projects/{projectId}/build-runs/{runId}/cancel",
      effect: "write",
      verification: [{ operationId: "getBuildRun", family: "build_run", expect: "canceled" }],
      parameters: [
        { inputName: "projectId", wireName: "projectId", in: "path", required: true },
        { inputName: "runId", wireName: "runId", in: "path", required: true },
      ],
      inputSchema: {
        type: "object",
        properties: { projectId: { type: "string" }, runId: { type: "string" } },
        required: ["projectId", "runId"],
        additionalProperties: false,
      },
    }),
    testToolOperation("deleteBuildRun", {
      method: "DELETE",
      path: "/api/v1/projects/{projectId}/build-runs/{runId}",
      effect: "write",
      verification: [{ operationId: "getBuildRun", family: "build_run", expect: "absent" }],
      parameters: [
        { inputName: "projectId", wireName: "projectId", in: "path", required: true },
        { inputName: "runId", wireName: "runId", in: "path", required: true },
      ],
      inputSchema: {
        type: "object",
        properties: { projectId: { type: "string" }, runId: { type: "string" } },
        required: ["projectId", "runId"],
        additionalProperties: false,
      },
    }),
  ])
}

function unverifiedWriteCatalog() {
  return ToolCatalog.load([
    testToolOperation("updateThing", {
      name: "更新资源",
      summary: "更新普通平台资源。",
      method: "POST",
      path: "/api/v1/things",
      effect: "write",
      requestBody: true,
      requestRequired: true,
      inputSchema: {
        type: "object",
        properties: { body: { type: "object" } },
        required: ["body"],
        additionalProperties: false,
      },
    }),
  ])
}

function failedWriteCatalog() {
  return ToolCatalog.load([
    testToolOperation("updateThing", {
      name: "更新资源",
      summary: "更新普通平台资源。",
      method: "POST",
      path: "/api/v1/things",
      effect: "write",
      verification: [{ operationId: "getThing", family: "resource", idFrom: "result.id", readIdArgument: "thingId" }],
      requestBody: true,
      requestRequired: true,
      inputSchema: {
        type: "object",
        properties: { body: { type: "object" } },
        required: ["body"],
        additionalProperties: false,
      },
    }),
    testToolOperation("getThing", {
      method: "GET",
      path: "/api/v1/things/{thingId}",
      effect: "read",
      parameters: [{ inputName: "thingId", wireName: "thingId", in: "path", required: true }],
      inputSchema: {
        type: "object",
        properties: { thingId: { type: "string" } },
        required: ["thingId"],
        additionalProperties: false,
      },
    }),
  ])
}

function applicationWriteCatalog() {
  return ToolCatalog.load([
    testToolOperation("createApplication", {
      method: "POST",
      path: "/api/v1/projects/{projectId}/applications",
      effect: "write",
      verification: [{ operationId: "getApplication", family: "application" }],
      parameters: [{ inputName: "projectId", wireName: "projectId", in: "path", required: true }],
      requestBody: true,
      requestRequired: true,
      inputSchema: {
        type: "object",
        properties: { projectId: { type: "string" }, body: { type: "object" } },
        required: ["projectId", "body"],
        additionalProperties: false,
      },
    }),
    testToolOperation("getApplication", {
      method: "GET",
      path: "/api/v1/projects/{projectId}/applications/{applicationId}",
      effect: "read",
      parameters: [
        { inputName: "projectId", wireName: "projectId", in: "path", required: true },
        { inputName: "applicationId", wireName: "applicationId", in: "path", required: true },
      ],
      inputSchema: {
        type: "object",
        properties: { projectId: { type: "string" }, applicationId: { type: "string" } },
        required: ["projectId", "applicationId"],
        additionalProperties: false,
      },
    }),
    testToolOperation("deleteApplication", {
      method: "DELETE",
      path: "/api/v1/projects/{projectId}/applications/{applicationId}",
      effect: "write",
      verification: [{ operationId: "getApplication", family: "application", expect: "absent" }],
      parameters: [
        { inputName: "projectId", wireName: "projectId", in: "path", required: true },
        { inputName: "applicationId", wireName: "applicationId", in: "path", required: true },
      ],
      inputSchema: {
        type: "object",
        properties: { projectId: { type: "string" }, applicationId: { type: "string" } },
        required: ["projectId", "applicationId"],
        additionalProperties: false,
      },
    }),
  ])
}

function deploymentTargetWriteCatalog() {
  return ToolCatalog.load([
    testToolOperation("updateDeploymentTarget", {
      method: "PUT",
      path: "/api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets/{targetId}",
      effect: "write",
      verification: [{ operationId: "listDeploymentTargets", family: "deployment_target" }],
      parameters: [
        { inputName: "projectId", wireName: "projectId", in: "path", required: true },
        { inputName: "applicationId", wireName: "applicationId", in: "path", required: true },
        { inputName: "targetId", wireName: "targetId", in: "path", required: true },
      ],
      requestBody: true,
      requestRequired: true,
      inputSchema: {
        type: "object",
        properties: {
          projectId: { type: "string" },
          applicationId: { type: "string" },
          targetId: { type: "string" },
          body: { type: "object" },
        },
        required: ["projectId", "applicationId", "targetId", "body"],
        additionalProperties: false,
      },
    }),
    testToolOperation("listDeploymentTargets", {
      method: "GET",
      path: "/api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets",
      effect: "read",
      parameters: [
        { inputName: "projectId", wireName: "projectId", in: "path", required: true },
        { inputName: "applicationId", wireName: "applicationId", in: "path", required: true },
        { inputName: "stage", wireName: "stage", in: "query", required: false },
        { inputName: "page", wireName: "page", in: "query", required: false },
        { inputName: "pageSize", wireName: "pageSize", in: "query", required: false },
      ],
      inputSchema: {
        type: "object",
        properties: {
          projectId: { type: "string" },
          applicationId: { type: "string" },
          stage: { type: "string" },
          page: { type: "integer" },
          pageSize: { type: "integer" },
        },
        required: ["projectId", "applicationId"],
        additionalProperties: false,
      },
    }),
    testToolOperation("deleteDeploymentTarget", {
      method: "DELETE",
      path: "/api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets/{targetId}",
      effect: "write",
      verification: [{ operationId: "listDeploymentTargets", family: "deployment_target", expect: "absent" }],
      parameters: [
        { inputName: "projectId", wireName: "projectId", in: "path", required: true },
        { inputName: "applicationId", wireName: "applicationId", in: "path", required: true },
        { inputName: "targetId", wireName: "targetId", in: "path", required: true },
      ],
      inputSchema: {
        type: "object",
        properties: {
          projectId: { type: "string" },
          applicationId: { type: "string" },
          targetId: { type: "string" },
        },
        required: ["projectId", "applicationId", "targetId"],
        additionalProperties: false,
      },
    }),
  ])
}

function appTemplateInstallCatalog() {
  return ToolCatalog.load([
    testToolOperation("installAppTemplate", {
      method: "POST",
      path: "/api/v1/projects/{projectId}/app-templates/{templateId}/install",
      effect: "write",
      verification: [
        { operationId: "getRelease", family: "release", whenResultHas: "release" },
        { operationId: "getApplication", family: "application" },
      ],
      parameters: [
        { inputName: "projectId", wireName: "projectId", in: "path", required: true },
        { inputName: "templateId", wireName: "templateId", in: "path", required: true },
      ],
      requestBody: true,
      requestRequired: true,
      inputSchema: {
        type: "object",
        properties: {
          projectId: { type: "string" },
          templateId: { type: "string" },
          body: { type: "object" },
        },
        required: ["projectId", "templateId", "body"],
        additionalProperties: false,
      },
    }),
    testToolOperation("getApplication", {
      method: "GET",
      path: "/api/v1/projects/{projectId}/applications/{applicationId}",
      effect: "read",
      parameters: [
        { inputName: "projectId", wireName: "projectId", in: "path", required: true },
        { inputName: "applicationId", wireName: "applicationId", in: "path", required: true },
      ],
      inputSchema: {
        type: "object",
        properties: { projectId: { type: "string" }, applicationId: { type: "string" } },
        required: ["projectId", "applicationId"],
        additionalProperties: false,
      },
    }),
    testToolOperation("getRelease", {
      method: "GET",
      path: "/api/v1/projects/{projectId}/releases/{releaseId}",
      effect: "read",
      parameters: [
        { inputName: "projectId", wireName: "projectId", in: "path", required: true },
        { inputName: "releaseId", wireName: "releaseId", in: "path", required: true },
      ],
      inputSchema: {
        type: "object",
        properties: { projectId: { type: "string" }, releaseId: { type: "string" } },
        required: ["projectId", "releaseId"],
        additionalProperties: false,
      },
    }),
  ])
}

function runtimeSecretCatalog() {
  return ToolCatalog.load([
    testToolOperation("updateDeploymentTargetRuntimeSecrets", {
      method: "PUT",
      path: "/api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets/{targetId}/runtime-secrets",
      effect: "write",
      requiresApproval: true,
      verification: [{ operationId: "getDeploymentTargetRuntimeSecretsSummary", family: "runtime_secrets" }],
      parameters: [
        { inputName: "projectId", wireName: "projectId", in: "path", required: true },
        { inputName: "applicationId", wireName: "applicationId", in: "path", required: true },
        { inputName: "targetId", wireName: "targetId", in: "path", required: true },
      ],
      requestBody: true,
      requestRequired: true,
      inputSchema: {
        type: "object",
        properties: {
          projectId: { type: "string" },
          applicationId: { type: "string" },
          targetId: { type: "string" },
          body: { type: "object" },
        },
        required: ["projectId", "applicationId", "targetId", "body"],
        additionalProperties: false,
      },
    }),
    testToolOperation("getDeploymentTargetRuntimeSecretsSummary", {
      method: "GET",
      path: "/api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets/{targetId}/runtime-secrets",
      effect: "read",
      verificationOnly: true,
      parameters: [
        { inputName: "projectId", wireName: "projectId", in: "path", required: true },
        { inputName: "applicationId", wireName: "applicationId", in: "path", required: true },
        { inputName: "targetId", wireName: "targetId", in: "path", required: true },
      ],
      inputSchema: {
        type: "object",
        properties: {
          projectId: { type: "string" },
          applicationId: { type: "string" },
          targetId: { type: "string" },
        },
        required: ["projectId", "applicationId", "targetId"],
        additionalProperties: false,
      },
    }),
  ])
}

function resourceVerificationCatalog() {
  return ToolCatalog.load([
    testToolOperation("createThing", {
      method: "POST",
      path: "/api/v1/projects/{projectId}/things",
      effect: "write",
      verification: [{
        operationId: "getThing",
        family: "resource",
        idFrom: "result.id",
        readIdArgument: "thingId",
        statusField: "status",
        successStatuses: ["ready"],
        pendingStatuses: ["provisioning"],
        failureStatuses: ["error"],
      }],
      parameters: [{ inputName: "projectId", wireName: "projectId", in: "path", required: true }],
      requestBody: true,
      requestRequired: true,
      inputSchema: {
        type: "object",
        properties: { projectId: { type: "string" }, body: { type: "object" } },
        required: ["projectId", "body"],
        additionalProperties: false,
      },
    }),
    testToolOperation("getThing", {
      method: "GET",
      path: "/api/v1/projects/{projectId}/things/{thingId}",
      effect: "read",
      parameters: [
        { inputName: "projectId", wireName: "projectId", in: "path", required: true },
        { inputName: "thingId", wireName: "thingId", in: "path", required: true },
      ],
      inputSchema: {
        type: "object",
        properties: { projectId: { type: "string" }, thingId: { type: "string" } },
        required: ["projectId", "thingId"],
        additionalProperties: false,
      },
    }),
  ])
}

async function queuedRun(owner: string, key: string) {
  const repository = new TestRepository()
  const conversation = await repository.createConversation(owner, key)
  const created = await repository.createTurn(owner, {
    conversationId: conversation.id,
    input: "构建应用并等待完成",
    pageContext: { projectId: "prj_1" },
    idempotencyKey: key,
    actorSessionId: `ses_${key}`,
  })
  return { repository, conversation, runId: created.run.id }
}

describe("Phase 4 tool verification", () => {
  it("reads a generic resource through pending and success states before finalizing", async () => {
    const { repository, runId } = await queuedRun("usr_verify_resource", "verify-resource")
    const catalog = resourceVerificationCatalog()
    let reads = 0
    const client = new DeterministicLunaApiClient((request) => {
      if (request.operation.operationId === "createThing")
        return { status: 201, body: { id: "thing_1", projectId: "prj_1", status: "provisioning" } }
      reads += 1
      return {
        status: 200,
        body: { id: "thing_1", projectId: "prj_1", status: reads === 1 ? "provisioning" : "ready" },
      }
    })
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    const model = provider([
      { toolCalls: [{ id: "create-thing", operationId: "createThing", arguments: {
        projectId: "prj_1",
        body: { name: "Thing" },
      } }] },
      { text: "资源已创建。" },
    ])
    const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry({
      resolve: (_page, _input, operationIds) => catalog.modelTools(operationIds),
    })), { catalog, tools, verificationPollIntervalMs: 0 })

    await executor.runOnce()

    expect(await repository.getRun("usr_verify_resource", runId)).toMatchObject({ status: "completed" })
    expect(client.calls.map(call => call.operation.operationId)).toEqual(["createThing", "getThing", "getThing"])
    expect(client.calls[1]?.arguments).toEqual({ projectId: "prj_1", thingId: "thing_1" })
  })

  it("reads an asynchronous build to its authoritative terminal state before allowing a final answer", async () => {
    const { repository, runId } = await queuedRun("usr_verify_build", "verify-build")
    const checkpointAdvances: Array<{ phase: string, modelStep: number }> = []
    const saveCheckpoint = repository.saveRunCheckpoint.bind(repository)
    repository.saveRunCheckpoint = async (valueRunId, advance) => {
      checkpointAdvances.push({ phase: advance.phase, modelStep: advance.modelStep })
      return saveCheckpoint(valueRunId, advance)
    }
    const catalog = buildCatalog()
    let buildReads = 0
    const client = new DeterministicLunaApiClient((request) => {
      if (request.operation.operationId === "triggerBuildRun")
        return { status: 201, body: { id: "bldr_1", status: "queued" } }
      buildReads += 1
      return {
        status: 200,
        body: { id: "bldr_1", status: buildReads === 1 ? "running" : "succeeded" },
      }
    })
    const tools = new ToolOrchestrator(
      catalog,
      client,
      new ProjectingToolCallStore(repository.toolCallStore, repository),
    )
    const model = provider([
      { toolCalls: [{ id: "build-call", operationId: "triggerBuildRun", arguments: { projectId: "prj_1", body: { applicationId: "app_1" } } }] },
      { text: "构建已完成。" },
    ])
    const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry({
      resolve: (_page, _input, operationIds) => catalog.modelTools(operationIds),
    })), { catalog, tools, verificationPollIntervalMs: 0 })

    await executor.runOnce()

    expect(await repository.getRun("usr_verify_build", runId)).toMatchObject({ status: "completed" })
    expect(client.calls.map(call => call.operation.operationId)).toEqual([
      "triggerBuildRun",
      "getBuildRun",
      "getBuildRun",
    ])
    expect(model.calls()).toBe(2)
    const events = await repository.getEvents("usr_verify_build", runId, 0)
    expect(events.map(event => event.type)).toContain("run.waiting_external")
    const checkpoint = await repository.loadRunCheckpoint(runId)
    expect(checkpoint?.phase).toBe("finalizing")
    expect(checkpoint?.state.pendingVerifications).toBeUndefined()
    const verificationAdvances = checkpointAdvances
      .filter(advance => advance.phase === "verification" || advance.phase === "waiting_external")
    expect(verificationAdvances.length).toBeGreaterThan(0)
    expect(verificationAdvances.every(advance => advance.modelStep === 1)).toBe(true)
  })

  it("fails the Run when the authoritative readback reaches a failed external state", async () => {
    const { repository, runId } = await queuedRun("usr_verify_failed", "verify-failed")
    const catalog = buildCatalog()
    const client = new DeterministicLunaApiClient((request) => request.operation.operationId === "triggerBuildRun"
      ? { status: 201, body: { id: "bldr_failed", status: "queued" } }
      : { status: 200, body: { id: "bldr_failed", status: "failed" } })
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    const model = provider([
      { toolCalls: [{ id: "build-failed-call", operationId: "triggerBuildRun", arguments: { projectId: "prj_1", body: { applicationId: "app_1" } } }] },
      { text: "不应完成。" },
    ])
    const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry({
      resolve: (_page, _input, operationIds) => catalog.modelTools(operationIds),
    })), { catalog, tools, verificationPollIntervalMs: 0 })

    await executor.runOnce()

    expect(await repository.getRun("usr_verify_failed", runId)).toMatchObject({
      status: "failed",
      errorCode: "ai.tool_execution_failed",
    })
    expect(model.calls()).toBe(1)
  })

  it("applies the same verification gate to direct tool actions", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_verify_direct", "verify-direct")
    const created = await repository.createTurn("usr_verify_direct", {
      conversationId: conversation.id,
      input: "触发构建",
      pageContext: { projectId: "prj_1", __lunaDirectToolAction: true },
      idempotencyKey: "verify-direct",
      actorSessionId: "ses_verify_direct",
    })
    const catalog = buildCatalog()
    const client = new DeterministicLunaApiClient(request => request.operation.operationId === "triggerBuildRun"
      ? { status: 201, body: { id: "bldr_direct", status: "queued" } }
      : { status: 200, body: { id: "bldr_direct", status: "succeeded" } })
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    await tools.propose({
      runId: created.run.id,
      operationId: "triggerBuildRun",
      arguments: { projectId: "prj_1", body: { applicationId: "app_1" } },
      inputMode: "direct",
    })
    const model = provider([{ text: "不应调用模型。" }])
    const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry()), {
      catalog,
      tools,
      verificationPollIntervalMs: 0,
    })

    await executor.runOnce()

    expect(await repository.getRun("usr_verify_direct", created.run.id)).toMatchObject({ status: "completed" })
    expect(client.calls.map(call => call.operation.operationId)).toEqual(["triggerBuildRun", "getBuildRun"])
    expect(model.calls()).toBe(0)
  })

  it("resumes verification after an approved write without executing the write again", async () => {
    const { repository, runId } = await queuedRun("usr_verify_approval", "verify-approval")
    const catalog = ToolCatalog.load([
      {
        ...buildCatalog().get("triggerBuildRun"),
        requiresApproval: true,
      },
      buildCatalog().get("getBuildRun"),
    ])
    let buildReads = 0
    const client = new DeterministicLunaApiClient((request) => {
      if (request.operation.operationId === "triggerBuildRun")
        return { status: 201, body: { id: "bldr_approved", status: "queued" } }
      buildReads += 1
      return { status: 200, body: { id: "bldr_approved", status: buildReads === 1 ? "running" : "succeeded" } }
    })
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    const model = provider([
      { toolCalls: [{ id: "approved-build", operationId: "triggerBuildRun", arguments: { projectId: "prj_1", body: { applicationId: "app_1" } } }] },
      { text: "构建已完成。" },
    ])
    const runtime = new ModelRuntime(model.value, testRegistry({
      resolve: (_page, _input, operationIds) => catalog.modelTools(operationIds),
    }))
    const first = testExecutor(repository, runtime, { catalog, tools, verificationPollIntervalMs: 0 })

    await first.runOnce()
    expect(await repository.getRun("usr_verify_approval", runId)).toMatchObject({ status: "waiting_approval" })
    const pending = [...repository.toolCallStore.records.values()].find(call => call.status === "awaiting_approval")
    if (!pending) throw new Error("missing pending approval")
    await tools.resolveApproval(pending.id, "approve")
    await repository.updateRun(runId, "waiting_approval", "queued")

    const second = testExecutor(repository, runtime, { catalog, tools, verificationPollIntervalMs: 0 })
    await second.runOnce()

    expect(await repository.getRun("usr_verify_approval", runId)).toMatchObject({ status: "completed" })
    expect(client.calls.filter(call => call.operation.operationId === "triggerBuildRun")).toHaveLength(1)
    expect(client.calls.filter(call => call.operation.operationId === "getBuildRun").length).toBeGreaterThanOrEqual(2)
    expect(model.calls()).toBe(2)
  })

  it("recovers a waiting-external checkpoint without resubmitting the asynchronous write", async () => {
    const { repository, runId } = await queuedRun("usr_verify_restart", "verify-restart")
    const catalog = buildCatalog()
    let buildReads = 0
    const client = new DeterministicLunaApiClient((request) => {
      if (request.operation.operationId === "triggerBuildRun")
        return { status: 201, body: { id: "bldr_restart", status: "queued" } }
      buildReads += 1
      return { status: 200, body: { id: "bldr_restart", status: "succeeded" } }
    })
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    const claimed = await repository.claimNextQueuedRun()
    if (!claimed) throw new Error("missing claimed run")
    const write = await tools.propose({
      runId,
      operationId: "triggerBuildRun",
      arguments: { projectId: "prj_1", body: { applicationId: "app_1" } },
    })
    const outcome = await tools.outcome(write.id)
    if (outcome.kind !== "running") throw new Error("missing verification")
    await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0,
      phase: "waiting_external",
      modelStep: 1,
      toolCallCount: 1,
      state: { pendingVerifications: [outcome.verification] },
    })
    await repository.recoverOrphanedRun(runId, claimed.rowVersion, 3)
    const model = provider([{ text: "构建已完成。" }])
    const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry()), {
      catalog,
      tools,
      verificationPollIntervalMs: 0,
    })

    await executor.runOnce()

    expect(await repository.getRun("usr_verify_restart", runId)).toMatchObject({ status: "completed" })
    expect(client.calls.filter(call => call.operation.operationId === "triggerBuildRun")).toHaveLength(1)
    expect(buildReads).toBe(1)
    expect(model.calls()).toBe(1)
  })

  it("does not grant an extra model step when a run resumes during verification", async () => {
    const { repository, runId } = await queuedRun("usr_verify_budget_restart", "verify-budget-restart")
    const catalog = buildCatalog()
    const client = new DeterministicLunaApiClient((request) => request.operation.operationId === "triggerBuildRun"
      ? { status: 201, body: { id: "bldr_budget", status: "queued" } }
      : { status: 200, body: { id: "bldr_budget", status: "succeeded" } })
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    const claimed = await repository.claimNextQueuedRun()
    if (!claimed) throw new Error("missing claimed run")
    const write = await tools.propose({
      runId,
      operationId: "triggerBuildRun",
      arguments: { projectId: "prj_1", body: { applicationId: "app_1" } },
    })
    const outcome = await tools.outcome(write.id)
    if (outcome.kind !== "running") throw new Error("missing verification")
    await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0,
      phase: "waiting_external",
      modelStep: defaultRuntimeSettings.maxModelSteps,
      toolCallCount: 1,
      state: { pendingVerifications: [outcome.verification] },
    })
    await repository.recoverOrphanedRun(runId, claimed.rowVersion, 3)
    const model = provider([{ text: "不应获得额外模型预算。" }])
    const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry()), {
      catalog,
      tools,
      verificationPollIntervalMs: 0,
    })

    await executor.runOnce()

    expect(await repository.getRun("usr_verify_budget_restart", runId)).toMatchObject({
      status: "failed",
      errorCode: "ai.limit_exceeded",
    })
    expect(model.calls()).toBe(0)
    expect(client.calls.filter(call => call.operation.operationId === "triggerBuildRun")).toHaveLength(1)
  })

  it("verifies a created application through its authoritative readback", async () => {
    const { repository, runId } = await queuedRun("usr_verify_application", "verify-application")
    const catalog = applicationWriteCatalog()
    const client = new DeterministicLunaApiClient((request) => request.operation.operationId === "createApplication"
      ? { status: 201, body: { id: "app_verified", projectId: "prj_1", name: "Web" } }
      : { status: 200, body: { id: "app_verified", projectId: "prj_1", name: "Web" } })
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    const model = provider([
      { toolCalls: [{ id: "create-app", operationId: "createApplication", arguments: {
        projectId: "prj_1",
        body: { identifier: "web", name: "Web" },
      } }] },
      { text: "应用已创建。" },
    ])
    const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry({
      resolve: (_page, _input, operationIds) => catalog.modelTools(operationIds),
    })), { catalog, tools, verificationPollIntervalMs: 0 })

    await executor.runOnce()

    expect(await repository.getRun("usr_verify_application", runId)).toMatchObject({ status: "completed" })
    expect(client.calls.map(call => call.operation.operationId)).toEqual(["createApplication", "getApplication"])
    expect((await repository.loadRunCheckpoint(runId))?.state.completionBlockers).toBeUndefined()
  })

  it("verifies an updated deployment target through its authoritative list readback", async () => {
    const { repository, runId } = await queuedRun("usr_verify_deployment", "verify-deployment")
    const catalog = deploymentTargetWriteCatalog()
    const target = {
      id: "dplt_verified",
      projectId: "prj_1",
      applicationId: "app_1",
      name: "production",
      replicas: 2,
      deleteStatus: "active",
    }
    const client = new DeterministicLunaApiClient((request) => request.operation.operationId === "updateDeploymentTarget"
      ? { status: 200, body: target }
      : { status: 200, body: { items: [target], total: 1, page: 1, pageSize: 100, totalPages: 1 } })
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    const model = provider([
      { toolCalls: [{ id: "update-deployment", operationId: "updateDeploymentTarget", arguments: {
        projectId: "prj_1",
        applicationId: "app_1",
        targetId: "dplt_verified",
        body: { name: "production", stage: "prod", replicas: 2 },
      } }] },
      { text: "部署配置已更新。" },
    ])
    const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry({
      resolve: (_page, _input, operationIds) => catalog.modelTools(operationIds),
    })), { catalog, tools, verificationPollIntervalMs: 0 })

    await executor.runOnce()

    expect(await repository.getRun("usr_verify_deployment", runId)).toMatchObject({ status: "completed" })
    expect(client.calls.map(call => call.operation.operationId)).toEqual(["updateDeploymentTarget", "listDeploymentTargets"])
    expect(client.calls[1]?.arguments).toMatchObject({
      projectId: "prj_1",
      applicationId: "app_1",
      stage: "prod",
      pageSize: 100,
    })
  })

  it("completes an application deletion only after the target no longer exists", async () => {
    const { repository, runId } = await queuedRun("usr_verify_application_delete", "verify-application-delete")
    const catalog = applicationWriteCatalog()
    let reads = 0
    const client = new DeterministicLunaApiClient((request) => {
      if (request.operation.operationId === "deleteApplication")
        return { status: 202, body: { id: "app_delete", projectId: "prj_1", deleteStatus: "deleting" } }
      reads += 1
      return reads === 1
        ? { status: 200, body: { id: "app_delete", projectId: "prj_1", deleteStatus: "deleting" } }
        : { status: 404, body: { code: "resource.not_found" } }
    })
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    const model = provider([
      { toolCalls: [{ id: "delete-application", operationId: "deleteApplication", arguments: {
        projectId: "prj_1",
        applicationId: "app_delete",
      } }] },
      { text: "应用已删除。" },
    ])
    const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry({
      resolve: (_page, _input, operationIds) => catalog.modelTools(operationIds),
    })), { catalog, tools, verificationPollIntervalMs: 0 })

    await executor.runOnce()

    expect(await repository.getRun("usr_verify_application_delete", runId)).toMatchObject({ status: "completed" })
    expect(client.calls.map(call => call.operation.operationId)).toEqual([
      "deleteApplication",
      "getApplication",
      "getApplication",
    ])
  })

  it("does not recreate a completed present verification when recovering after a later deletion", async () => {
    const { repository, runId } = await queuedRun("usr_verify_history", "verify-history")
    const base = applicationWriteCatalog()
    const catalog = ToolCatalog.load(base.all().map(operation => operation.operationId === "deleteApplication"
      ? { ...operation, requiresApproval: true }
      : operation))
    let applicationReads = 0
    const client = new DeterministicLunaApiClient((request) => {
      if (request.operation.operationId === "createApplication")
        return { status: 201, body: { id: "app_history", projectId: "prj_1", name: "History" } }
      if (request.operation.operationId === "deleteApplication")
        return { status: 202, body: { id: "app_history", projectId: "prj_1", deleteStatus: "deleting" } }
      applicationReads += 1
      return applicationReads === 1
        ? { status: 200, body: { id: "app_history", projectId: "prj_1", name: "History" } }
        : { status: 404, body: { code: "resource.not_found" } }
    })
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    const claimed = await repository.claimNextQueuedRun()
    if (!claimed) throw new Error("missing claimed run")
    const created = await tools.propose({
      runId,
      operationId: "createApplication",
      arguments: { projectId: "prj_1", body: { identifier: "history", name: "History" } },
    })
    const createdOutcome = await tools.outcome(created.id)
    if (createdOutcome.kind !== "succeeded" || !createdOutcome.verification) throw new Error("missing create verification")
    await tools.propose({
      runId,
      operationId: createdOutcome.verification.operationId,
      arguments: createdOutcome.verification.arguments,
      inputMode: "verification",
      verificationForToolCallId: created.id,
    })
    const deletion = await tools.propose({
      runId,
      operationId: "deleteApplication",
      arguments: { projectId: "prj_1", applicationId: "app_history" },
    })
    expect(deletion.status).toBe("awaiting_approval")
    await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0,
      phase: "awaiting_approval",
      modelStep: 1,
      toolCallCount: 1,
      state: { pendingToolCallId: deletion.id },
    })
    await repository.updateRun(runId, "running", "waiting_approval")
    await tools.resolveApproval(deletion.id, "approve")
    await repository.updateRun(runId, "waiting_approval", "queued")

    const model = provider([{ text: "应用已创建并删除。" }])
    const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry()), {
      catalog,
      tools,
      verificationPollIntervalMs: 0,
    })

    await executor.runOnce()

    expect(await repository.getRun("usr_verify_history", runId)).toMatchObject({ status: "completed" })
    expect(client.calls.map(call => call.operation.operationId)).toEqual([
      "createApplication",
      "getApplication",
      "deleteApplication",
      "getApplication",
    ])
  })

  it("completes a deployment target deletion only after the target leaves the authoritative list", async () => {
    const { repository, runId } = await queuedRun("usr_verify_deployment_delete", "verify-deployment-delete")
    const catalog = deploymentTargetWriteCatalog()
    let reads = 0
    const client = new DeterministicLunaApiClient((request) => {
      if (request.operation.operationId === "deleteDeploymentTarget")
        return { status: 204, body: undefined }
      reads += 1
      return {
        status: 200,
        body: {
          items: reads === 1
            ? [{ id: "dplt_delete", projectId: "prj_1", applicationId: "app_1", deleteStatus: "deleting" }]
            : [],
          total: reads === 1 ? 1 : 0,
          page: 1,
          pageSize: 100,
          totalPages: reads === 1 ? 1 : 0,
        },
      }
    })
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    const model = provider([
      { toolCalls: [{ id: "delete-target", operationId: "deleteDeploymentTarget", arguments: {
        projectId: "prj_1",
        applicationId: "app_1",
        targetId: "dplt_delete",
      } }] },
      { text: "部署目标已删除。" },
    ])
    const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry({
      resolve: (_page, _input, operationIds) => catalog.modelTools(operationIds),
    })), { catalog, tools, verificationPollIntervalMs: 0 })

    await executor.runOnce()

    expect(await repository.getRun("usr_verify_deployment_delete", runId)).toMatchObject({ status: "completed" })
    expect(client.calls.map(call => call.operation.operationId)).toEqual([
      "deleteDeploymentTarget",
      "listDeploymentTargets",
      "listDeploymentTargets",
    ])
  })

  it("completes a build cancellation only after the build reaches canceled", async () => {
    const { repository, runId } = await queuedRun("usr_verify_build_cancel", "verify-build-cancel")
    const catalog = buildCatalog()
    const client = new DeterministicLunaApiClient((request) => request.operation.operationId === "cancelBuildRun"
      ? { status: 200, body: { id: "bldr_cancel", status: "canceled" } }
      : { status: 200, body: { id: "bldr_cancel", status: "canceled" } })
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    const model = provider([
      { toolCalls: [{ id: "cancel-build", operationId: "cancelBuildRun", arguments: {
        projectId: "prj_1",
        runId: "bldr_cancel",
      } }] },
      { text: "构建已取消。" },
    ])
    const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry({
      resolve: (_page, _input, operationIds) => catalog.modelTools(operationIds),
    })), { catalog, tools, verificationPollIntervalMs: 0 })

    await executor.runOnce()

    expect(await repository.getRun("usr_verify_build_cancel", runId)).toMatchObject({ status: "completed" })
    expect(client.calls.map(call => call.operation.operationId)).toEqual(["cancelBuildRun", "getBuildRun"])
  })

  it("completes a build record deletion only after getBuildRun reports absence", async () => {
    const { repository, runId } = await queuedRun("usr_verify_build_delete", "verify-build-delete")
    const catalog = buildCatalog()
    const client = new DeterministicLunaApiClient((request) => request.operation.operationId === "deleteBuildRun"
      ? { status: 204, body: undefined }
      : { status: 404, body: { code: "resource.not_found" } })
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    const model = provider([
      { toolCalls: [{ id: "delete-build", operationId: "deleteBuildRun", arguments: {
        projectId: "prj_1",
        runId: "bldr_delete",
      } }] },
      { text: "构建记录已删除。" },
    ])
    const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry({
      resolve: (_page, _input, operationIds) => catalog.modelTools(operationIds),
    })), { catalog, tools, verificationPollIntervalMs: 0 })

    await executor.runOnce()

    expect(await repository.getRun("usr_verify_build_delete", runId)).toMatchObject({ status: "completed" })
    expect(client.calls.map(call => call.operation.operationId)).toEqual(["deleteBuildRun", "getBuildRun"])
  })

  it("verifies a template installation without a release through the installed application", async () => {
    const { repository, runId } = await queuedRun("usr_verify_template", "verify-template")
    const catalog = appTemplateInstallCatalog()
    const client = new DeterministicLunaApiClient((request) => {
      if (request.operation.operationId === "installAppTemplate")
        return {
          status: 201,
          body: {
            installation: { id: "atpl_1" },
            application: { id: "app_template", projectId: "prj_1" },
            deploymentTarget: { id: "dplt_template", projectId: "prj_1", applicationId: "app_template" },
          },
        }
      return { status: 200, body: { id: "app_template", projectId: "prj_1" } }
    })
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    const model = provider([
      { toolCalls: [{ id: "install-template", operationId: "installAppTemplate", arguments: {
        projectId: "prj_1",
        templateId: "postgres",
        body: { applicationName: "Postgres", applicationIdentifier: "postgres", deploymentName: "default", stage: "dev", clusterId: "cls_1", installNow: false },
      } }] },
      { text: "模板已安装，未创建发布。" },
    ])
    const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry({
      resolve: (_page, _input, operationIds) => catalog.modelTools(operationIds),
    })), { catalog, tools, verificationPollIntervalMs: 0 })

    await executor.runOnce()

    expect(await repository.getRun("usr_verify_template", runId)).toMatchObject({ status: "completed" })
    expect(client.calls.map(call => call.operation.operationId)).toEqual(["installAppTemplate", "getApplication"])
  })

  it("verifies runtime secret updates through a verification-only summary read", async () => {
    const { repository, runId } = await queuedRun("usr_verify_runtime_secrets", "verify-runtime-secrets")
    const catalog = runtimeSecretCatalog()
    const client = new DeterministicLunaApiClient((request) => request.operation.operationId === "updateDeploymentTargetRuntimeSecrets"
      ? {
          status: 200,
          body: {
            configuredKeys: ["TOKEN"],
            generatedKeys: [],
            clearedKeys: ["OLD_TOKEN"],
            environmentVariables: [],
          },
        }
      : {
          status: 200,
          body: {
            environmentVariables: [
              { key: "PUBLIC_MODE", valueMode: "public", value: "safe", configured: true },
              { key: "TOKEN", valueMode: "secret", configured: true },
            ],
          },
        })
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    const model = provider([
      { toolCalls: [{ id: "update-secrets", operationId: "updateDeploymentTargetRuntimeSecrets", arguments: {
        projectId: "prj_1",
        applicationId: "app_1",
        targetId: "dplt_1",
        body: {
          items: [
            { key: "TOKEN", valueMode: "secret", operation: "set", value: "secret-value" },
            { key: "OLD_TOKEN", valueMode: "secret", operation: "clear" },
          ],
        },
      } }] },
      { text: "运行密钥已更新。" },
    ])
    const runtime = new ModelRuntime(model.value, testRegistry({
      resolve: (_page, _input, operationIds) => catalog.modelTools(operationIds),
    }))
    const first = testExecutor(repository, runtime, { catalog, tools, verificationPollIntervalMs: 0 })

    await first.runOnce()
    expect(await repository.getRun("usr_verify_runtime_secrets", runId)).toMatchObject({ status: "waiting_approval" })
    const pending = [...repository.toolCallStore.records.values()].find(call => call.status === "awaiting_approval")
    if (!pending) throw new Error("missing pending approval")
    await tools.resolveApproval(pending.id, "approve")
    await repository.updateRun(runId, "waiting_approval", "queued")

    const second = testExecutor(repository, runtime, { catalog, tools, verificationPollIntervalMs: 0 })
    await second.runOnce()

    expect(await repository.getRun("usr_verify_runtime_secrets", runId)).toMatchObject({ status: "completed" })
    expect(client.calls.map(call => call.operation.operationId)).toEqual([
      "updateDeploymentTargetRuntimeSecrets",
      "getDeploymentTargetRuntimeSecretsSummary",
    ])
    expect(catalog.modelTools(["getDeploymentTargetRuntimeSecretsSummary"])).toEqual([])
  })

  it("rejects a write without authoritative verification before exposing any side effect", async () => {
    const { repository, runId } = await queuedRun("usr_write_unverified", "write-unverified")
    const catalog = unverifiedWriteCatalog()
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: { updated: true } }))
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    const model = provider([
      { toolCalls: [{ id: "update-unverified", operationId: "updateThing", arguments: { body: { name: "next" } } }] },
      { text: "操作已完成。" },
    ])
    const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry({
      resolve: (_page, _input, operationIds) => catalog.modelTools(operationIds),
    })), { catalog, tools, verificationPollIntervalMs: 0 })

    await executor.runOnce()

    expect(await repository.getRun("usr_write_unverified", runId)).toMatchObject({
      status: "failed",
      errorCode: "ai.tool_not_available",
    })
    expect(client.calls).toHaveLength(0)
    expect(model.calls()).toBe(1)
  })

  it("blocks completion after a failed write unless the logical call is retried to authoritative success", async () => {
    const { repository, runId } = await queuedRun("usr_write_failed_gate", "write-failed-gate")
    const catalog = failedWriteCatalog()
    const client = new DeterministicLunaApiClient(() => ({
      status: 500,
      body: { code: "service.unavailable", retryable: true },
    }))
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    const model = provider([
      { toolCalls: [{ id: "update-failed", operationId: "updateThing", arguments: { body: { name: "next" } } }] },
      { text: "操作已完成。" },
    ])
    const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry({
      resolve: (_page, _input, operationIds) => catalog.modelTools(operationIds),
    })), { catalog, tools, verificationPollIntervalMs: 0 })

    await executor.runOnce()

    expect(await repository.getRun("usr_write_failed_gate", runId)).toMatchObject({
      status: "failed",
      errorCode: "ai.verification_inconclusive",
    })
    expect([...repository.toolCallStore.records.values()][0]).toMatchObject({
      operationId: "updateThing",
      status: "failed",
      errorCode: "service.unavailable",
    })
  })

  it("reuses one logical idempotency key when the model retries an identical asynchronous write", async () => {
    const { repository, runId } = await queuedRun("usr_write_model_retry", "write-model-retry")
    const catalog = buildCatalog()
    let triggerAttempts = 0
    const client = new DeterministicLunaApiClient((request) => {
      if (request.operation.operationId === "triggerBuildRun") {
        triggerAttempts += 1
        return triggerAttempts === 1
          ? { status: 500, body: { code: "service.unavailable", retryable: true } }
          : { status: 201, body: { id: "bldr_retry", status: "queued" } }
      }
      return { status: 200, body: { id: "bldr_retry", status: "succeeded" } }
    })
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    const model = provider([
      { toolCalls: [{ id: "build-retry-1", operationId: "triggerBuildRun", arguments: { projectId: "prj_1", body: { applicationId: "app_1" } } }] },
      { toolCalls: [{ id: "build-retry-2", operationId: "triggerBuildRun", arguments: { projectId: "prj_1", body: { applicationId: "app_1" } } }] },
      { text: "构建已完成。" },
    ])
    const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry({
      resolve: (_page, _input, operationIds) => catalog.modelTools(operationIds),
    })), { catalog, tools, verificationPollIntervalMs: 0 })

    await executor.runOnce()

    expect(await repository.getRun("usr_write_model_retry", runId)).toMatchObject({ status: "completed" })
    const triggerCalls = client.calls.filter(call => call.operation.operationId === "triggerBuildRun")
    expect(triggerCalls).toHaveLength(2)
    expect(triggerCalls[1]?.idempotencyKey).toBe(triggerCalls[0]?.idempotencyKey)
  })

  it("restores completion blockers from checkpoint before permitting finalization", async () => {
    const { repository, runId } = await queuedRun("usr_write_blocker_restart", "write-blocker-restart")
    const claimed = await repository.claimNextQueuedRun()
    if (!claimed) throw new Error("missing claimed run")
    await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0,
      phase: "model",
      modelStep: 1,
      toolCallCount: 1,
      state: {
        completionBlockers: [{
          toolCallId: "aitool_persisted",
          idempotencyKey: "aiidem_persisted",
          operationId: "updateThing",
          reason: "verification_required",
        }],
      },
    })
    await repository.recoverOrphanedRun(runId, claimed.rowVersion, 3)
    const model = provider([{ text: "操作已完成。" }])
    const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry()))

    await executor.runOnce()

    expect(await repository.getRun("usr_write_blocker_restart", runId)).toMatchObject({
      status: "failed",
      errorCode: "ai.verification_inconclusive",
    })
  })

  it("persists an async verification plan when a later tool in the same batch waits for input", async () => {
    const { repository, runId } = await queuedRun("usr_batch_waiting_input", "batch-waiting-input")
    const catalog = ToolCatalog.load([
      buildCatalog().get("triggerBuildRun"),
      buildCatalog().get("getBuildRun"),
      testToolOperation("getProject", {
        method: "GET",
        path: "/api/v1/projects/{projectId}",
        effect: "read",
        parameters: [{ inputName: "projectId", wireName: "projectId", in: "path", required: true }],
        inputSchema: {
          type: "object",
          properties: { projectId: { type: "string" } },
          required: ["projectId"],
          additionalProperties: false,
        },
      }),
    ])
    const client = new DeterministicLunaApiClient((request) => {
      if (request.operation.operationId === "triggerBuildRun")
        return { status: 201, body: { id: "bldr_batch", status: "queued" } }
      return { status: 200, body: { id: "bldr_batch", status: "succeeded" } }
    })
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    const model = provider([
      { toolCalls: [
        { id: "batch-build", operationId: "triggerBuildRun", arguments: { projectId: "prj_1", body: { applicationId: "app_1" } } },
        { id: "batch-project", operationId: "getProject", arguments: {} },
      ] },
      { text: "构建已完成。" },
    ])
    const runtime = new ModelRuntime(model.value, testRegistry({
      resolve: (_page, _input, operationIds) => catalog.modelTools(operationIds),
    }))
    const first = testExecutor(repository, runtime, { catalog, tools, verificationPollIntervalMs: 0 })

    await first.runOnce()

    expect(await repository.getRun("usr_batch_waiting_input", runId)).toMatchObject({ status: "waiting_input" })
    expect((await repository.loadRunCheckpoint(runId))?.state.pendingVerifications).toHaveLength(1)
    expect(client.calls.map(call => call.operation.operationId)).toEqual(["triggerBuildRun"])

    await repository.appendRunInput(runId, "prj_1")
    await repository.updateRun(runId, "waiting_input", "queued")
    const second = testExecutor(repository, runtime, { catalog, tools, verificationPollIntervalMs: 0 })
    await second.runOnce()

    expect(await repository.getRun("usr_batch_waiting_input", runId)).toMatchObject({ status: "completed" })
    expect(client.calls.map(call => call.operation.operationId)).toEqual(["triggerBuildRun", "getBuildRun"])
    expect((await repository.loadRunCheckpoint(runId))?.state.pendingVerifications).toBeUndefined()
  })
})
