import { describe, expect, it } from "vitest"
import type { RunCheckpoint, TimelineItem } from "../src/domain.js"
import type { RunExecutor } from "../src/executor/index.js"
import { checkpointState, maxRunRecoveries, resumePlan } from "../src/executor/checkpoint.js"
import { ToolStepRunner, type ToolStepState } from "../src/executor/tool-step.js"
import { ModelRuntime } from "../src/model-runtime.js"
import { RunCheckpointConflictError } from "../src/persistence/repository.js"
import type { ModelMessage, ModelProvider, ModelToolCall } from "../src/provider/provider.js"
import { ToolCatalog } from "../src/tools/catalog.js"
import { DeterministicLunaApiClient } from "../src/tools/luna-api-client.js"
import { MemoryToolCallStore, ProjectingToolCallStore, ToolInterruption, ToolOrchestrator, type ToolCallRecord } from "../src/tools/orchestrator.js"
import { searchToolsTool } from "../src/tools/tool-search.js"
import { testRegistry } from "./support/model-tool-registry.js"
import { testExecutor } from "./support/run-executor.js"
import { TestRepository } from "./support/test-repository.js"
import { testToolOperation } from "./support/tool-catalog.js"

const reportedUsage = { status: "reported" as const, value: { inputTokens: 3, outputTokens: 2, totalTokens: 5 } }

type ScriptedStep = {
  text?: string
  reasoning?: string
  toolCalls?: ModelToolCall[]
}

function scriptedProvider(steps: ScriptedStep[]) {
  const state = { modelCalls: 0 }
  const messages: ModelMessage[][] = []
  const provider: ModelProvider = {
    async *stream(request) {
      const step = steps[Math.min(state.modelCalls, steps.length - 1)] ?? {}
      state.modelCalls += 1
      messages.push(request.messages)
      if (step.reasoning) yield { type: "reasoning_summary_delta", delta: step.reasoning }
      if (step.text) yield { type: "message_delta", delta: step.text }
      if (step.toolCalls?.length) yield { type: "tool_call_delta" }
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
  return { provider, modelCalls: () => state.modelCalls, messages }
}

async function queuedRun(owner: string, key: string, input = "检查项目部署状态") {
  const repository = new TestRepository()
  const conversation = await repository.createConversation(owner, key)
  const created = await repository.createTurn(owner, {
    conversationId: conversation.id,
    input,
    pageContext: {},
    idempotencyKey: key,
    actorSessionId: `ses_${key}`,
  })
  return { repository, conversation, runId: created.run.id }
}

async function timelineItems(repository: TestRepository, owner: string, conversationId: string): Promise<TimelineItem[]> {
  const timeline = await repository.getTimeline(owner, conversationId)
  return timeline?.turns[0]?.items ?? []
}

function assistantTexts(items: TimelineItem[]): string[] {
  return items
    .filter(item => item.type === "assistant_message")
    .map(item => (item.content as { parts?: Array<{ text?: string }> }).parts?.[0]?.text ?? "")
}

function platformTool(
  repository: TestRepository,
  catalog: ToolCatalog,
  result: ConstructorParameters<typeof DeterministicLunaApiClient>[0],
) {
  const store = repository.toolCallStore
  const client = new DeterministicLunaApiClient(result)
  return {
    store,
    client,
    tools: new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(store, repository)),
  }
}

/** 模拟重启：同一份事实存储和同一个平台客户端，但进程内守卫计数重新开始。 */
function restartedTool(repository: TestRepository, catalog: ToolCatalog, client: DeterministicLunaApiClient) {
  return new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
}

/** 与 Executor.start() 的启动对账等价，但不依赖轮询计时器。 */
function reconcile(executor: RunExecutor): Promise<boolean> {
  return (executor as unknown as { recoverOrphanedRuns(): Promise<boolean> }).recoverOrphanedRuns()
}

/** 模拟进程退出后重启：先做启动对账，再让新 Executor 领取 Run。 */
async function restart(executor: RunExecutor): Promise<boolean> {
  await reconcile(executor)
  return executor.runOnce()
}

async function claimOrThrow(repository: TestRepository) {
  const claimed = await repository.claimNextQueuedRun()
  if (!claimed) throw new Error("missing claimed run")
  return claimed
}

const requiredProjectInput = {
  type: "object" as const,
  properties: { projectId: { type: "string" } },
  required: ["projectId"],
  additionalProperties: false as const,
}

function checkpoint(phase: RunCheckpoint["phase"], state: RunCheckpoint["state"] = {}): RunCheckpoint {
  return {
    runId: "airun_checkpoint",
    revision: 3,
    phase,
    modelStep: 2,
    toolCallCount: 1,
    recoveries: 0,
    state,
    createdAt: new Date(0).toISOString(),
    updatedAt: new Date(0).toISOString(),
  }
}

describe("checkpoint 恢复计划", () => {
  it("按持久化相位给出续跑动作", () => {
    expect(resumePlan(checkpoint("model"))).toEqual({ kind: "model" })
    expect(resumePlan(checkpoint("awaiting_input"))).toEqual({ kind: "model" })
    expect(resumePlan(checkpoint("awaiting_approval", { pendingToolCallId: "aitool_1" })))
      .toEqual({ kind: "await_approval", toolCallId: "aitool_1" })
    expect(resumePlan(checkpoint("awaiting_approval"))).toEqual({ kind: "settle_tool_batch" })
    expect(resumePlan(checkpoint("finalizing", { finalOutput: "已完成检查。" })))
      .toEqual({ kind: "finalize", finalOutput: "已完成检查。" })
    expect(resumePlan(checkpoint("finalizing"))).toEqual({ kind: "finalize" })
    expect(resumePlan(checkpoint("tool_execution"))).toEqual({ kind: "settle_tool_batch" })
  })

  it("组装状态时省略空计数，保留空字符串形式的最终答复", () => {
    expect(checkpointState({ protocolRepairCount: 0, argumentRepairAttempts: new Map() })).toEqual({})
    expect(checkpointState({
      protocolRepairCount: 2,
      argumentRepairAttempts: new Map([["getProject", 1]]),
      finalOutput: "",
    })).toEqual({ protocolRepairCount: 2, argumentRepairAttempts: { getProject: 1 }, finalOutput: "" })
  })
})

describe("checkpoint 乐观锁", () => {
  it("同一 revision 重放幂等，旧 revision 不能覆盖新状态", async () => {
    const { repository, runId } = await queuedRun("usr_checkpoint_lock", "checkpoint-lock")
    const first = await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0, phase: "model", modelStep: 0, toolCallCount: 0, state: {},
    })
    expect(first.revision).toBe(1)
    const replay = await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0, phase: "model", modelStep: 0, toolCallCount: 0, state: {},
    })
    expect(replay.revision).toBe(1)
    const advanced = await repository.saveRunCheckpoint(runId, {
      expectedRevision: 1, phase: "tool_execution", modelStep: 0, toolCallCount: 0, state: {},
    })
    expect(advanced.revision).toBe(2)
    await expect(repository.saveRunCheckpoint(runId, {
      expectedRevision: 0, phase: "model", modelStep: 9, toolCallCount: 9, state: {},
    })).rejects.toBeInstanceOf(RunCheckpointConflictError)
    expect(await repository.loadRunCheckpoint(runId)).toMatchObject({ revision: 2, phase: "tool_execution" })
  })
})

describe("Phase 2：checkpoint 恢复边界", () => {
  it("模型调用前退出：重启后从同一步继续，只调用一次模型", async () => {
    const owner = "usr_cp_before_model"
    const { repository, conversation, runId } = await queuedRun(owner, "cp-before-model")
    await claimOrThrow(repository)
    await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0, phase: "model", modelStep: 0, toolCallCount: 0, state: {},
    })

    const { provider, modelCalls } = scriptedProvider([{ text: "已完成检查。" }])
    expect(await restart(testExecutor(repository, new ModelRuntime(provider, testRegistry())))).toBe(true)

    expect(modelCalls()).toBe(1)
    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "completed" })
    expect(assistantTexts(await timelineItems(repository, owner, conversation.id))).toEqual(["已完成检查。"])
  })

  it("工具完成后退出：重启复用工具结果，不重复执行工具", async () => {
    const owner = "usr_cp_after_tool"
    const { repository, conversation, runId } = await queuedRun(owner, "cp-after-tool", "查看项目 prj_1")
    const catalog = ToolCatalog.load([testToolOperation("getProject")])
    const { tools, client } = platformTool(repository, catalog, () => ({ status: 200, body: { project: { id: "prj_1" } } }))
    await claimOrThrow(repository)
    const executed = await tools.propose({ runId, operationId: "getProject", arguments: {} })
    expect(executed.status).toBe("succeeded")
    await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0,
      phase: "model",
      modelStep: 1,
      toolCallCount: 1,
      state: {
        loopGuard: {
          proposed: 1,
          occurrences: [{ operationId: "getProject", argumentsHash: executed.argumentsHash, count: 1 }],
          nonRetryableCalls: [],
        },
      },
    })

    const { provider, modelCalls, messages } = scriptedProvider([{ text: "项目 prj_1 运行正常。" }])
    const executor = testExecutor(
      repository,
      new ModelRuntime(provider, testRegistry({ resolve: () => catalog.modelTools(["getProject"]) })),
      { catalog, tools },
    )
    expect(await restart(executor)).toBe(true)

    expect(client.calls).toHaveLength(1)
    expect(modelCalls()).toBe(1)
    expect(messages[0]?.some(message => message.role === "tool" && message.content.includes("prj_1"))).toBe(true)
    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "completed" })
    expect(assistantTexts(await timelineItems(repository, owner, conversation.id))).toEqual(["项目 prj_1 运行正常。"])
  })

  it("等待审批时退出：重启不重复执行，审批完成后续跑到终态", async () => {
    const owner = "usr_cp_approval"
    const { repository, runId } = await queuedRun(owner, "cp-approval", "重启应用 app_1")
    const catalog = ToolCatalog.load([testToolOperation("restartApplication", {
      effect: "read",
      method: "POST",
      path: "/api/v1/applications/:id/restart",
      requiresApproval: true,
      requestBody: true,
      inputSchema: {
        type: "object",
        properties: { applicationId: { type: "string" } },
        required: ["applicationId"],
        additionalProperties: false,
      },
    })])
    const { tools, client } = platformTool(repository, catalog, () => ({ status: 200, body: { status: "restarted" } }))
    await claimOrThrow(repository)
    const pending = await tools.propose({ runId, operationId: "restartApplication", arguments: { applicationId: "app_1" } })
    expect(pending.status).toBe("awaiting_approval")
    await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0,
      phase: "awaiting_approval",
      modelStep: 0,
      toolCallCount: 0,
      state: { pendingToolCallId: pending.id },
    })
    await repository.updateRun(runId, "running", "waiting_approval")

    const { provider, modelCalls, messages } = scriptedProvider([{ text: "应用已重启。" }])
    const executor = testExecutor(
      repository,
      new ModelRuntime(provider, testRegistry({ resolve: () => catalog.modelTools(["restartApplication"]) })),
      { catalog, tools },
    )
    // 等待审批的 Run 不是孤儿 Run，启动对账不应动它。
    expect(await restart(executor)).toBe(false)
    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "waiting_approval" })

    await tools.resolveApproval(pending.id, "approve")
    await repository.updateRun(runId, "waiting_approval", "queued")
    expect(await executor.runOnce()).toBe(true)

    expect(client.calls).toHaveLength(1)
    expect(modelCalls()).toBe(1)
    // 审批结果只出现一次：恢复不能把同一工具调用重复折叠进续跑上下文。
    expect(messages[0]?.filter(message => message.role === "tool")).toHaveLength(1)
    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "completed" })
  })

  it("终态提交前退出：重启不再调用模型，重建未提交的最终答复", async () => {
    const owner = "usr_cp_finalize"
    const { repository, conversation, runId } = await queuedRun(owner, "cp-finalize")
    await claimOrThrow(repository)
    await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0,
      phase: "finalizing",
      modelStep: 1,
      toolCallCount: 0,
      state: { finalOutput: "部署状态正常。" },
    })

    const { provider, modelCalls } = scriptedProvider([{ text: "不应被调用。" }])
    expect(await restart(testExecutor(repository, new ModelRuntime(provider, testRegistry())))).toBe(true)

    expect(modelCalls()).toBe(0)
    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "completed" })
    expect(assistantTexts(await timelineItems(repository, owner, conversation.id))).toEqual(["部署状态正常。"])
    expect((await repository.getEvents(owner, runId, 0)).map(event => event.type)).toContain("run.completed")
    // 恢复推进不能丢掉未提交的答复，否则再次崩溃会得到没有内容的 completed。
    expect(await repository.loadRunCheckpoint(runId)).toMatchObject({
      phase: "finalizing",
      state: { finalOutput: "部署状态正常。" },
    })
  })

  it("终态提交前退出但没有未提交消息时，只补提交终态", async () => {
    const owner = "usr_cp_finalize_bare"
    const { repository, conversation, runId } = await queuedRun(owner, "cp-finalize-bare")
    await claimOrThrow(repository)
    await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0, phase: "finalizing", modelStep: 1, toolCallCount: 0, state: {},
    })

    const { provider, modelCalls } = scriptedProvider([{ text: "不应被调用。" }])
    await restart(testExecutor(repository, new ModelRuntime(provider, testRegistry())))

    expect(modelCalls()).toBe(0)
    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "completed" })
    expect(assistantTexts(await timelineItems(repository, owner, conversation.id))).toEqual([])
  })

  it("工具成功但 checkpoint 仍停在 tool_execution：重启复用结果，不重复执行工具", async () => {
    const owner = "usr_cp_tool_settled"
    const { repository, conversation, runId } = await queuedRun(owner, "cp-tool-settled", "查看项目 prj_1")
    const catalog = ToolCatalog.load([testToolOperation("getProject")])
    const { tools, client } = platformTool(repository, catalog, () => ({ status: 200, body: { project: { id: "prj_1" } } }))
    await claimOrThrow(repository)
    // 真实窗口：工具批次已落地并产生副作用，进程在写下一个 checkpoint 之前退出。
    const executed = await tools.propose({ runId, operationId: "getProject", arguments: {} })
    expect(executed.status).toBe("succeeded")
    await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0, phase: "tool_execution", modelStep: 1, toolCallCount: 1, state: {},
    })

    const { provider, modelCalls, messages } = scriptedProvider([{ text: "项目 prj_1 运行正常。" }])
    const executor = testExecutor(
      repository,
      new ModelRuntime(provider, testRegistry({ resolve: () => catalog.modelTools(["getProject"]) })),
      { catalog, tools },
    )
    expect(await restart(executor)).toBe(true)

    expect(client.calls).toHaveLength(1)
    expect(modelCalls()).toBe(1)
    expect(messages[0]?.some(message => message.role === "tool" && message.content.includes("prj_1"))).toBe(true)
    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "completed" })
    expect(assistantTexts(await timelineItems(repository, owner, conversation.id))).toEqual(["项目 prj_1 运行正常。"])
  })

  it("工具事实已落地但投影缺失：按事实补齐结果，不重复执行工具", async () => {
    const owner = "usr_cp_tool_unprojected"
    const { repository, runId } = await queuedRun(owner, "cp-tool-unprojected")
    const catalog = ToolCatalog.load([testToolOperation("getProject")])
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: { project: { id: "prj_9" } } }))
    // 不挂投影存储：模拟事实已提交、Timeline 条目未写入时退出。
    const store = repository.toolCallStore
    const tools = new ToolOrchestrator(catalog, client, store)
    await claimOrThrow(repository)
    const executed = await tools.propose({ runId, operationId: "getProject", arguments: {} })
    expect(executed.status).toBe("succeeded")
    await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0, phase: "tool_execution", modelStep: 1, toolCallCount: 1, state: {},
    })

    const { provider, modelCalls, messages } = scriptedProvider([{ text: "项目 prj_9 运行正常。" }])
    const executor = testExecutor(
      repository,
      new ModelRuntime(provider, testRegistry({ resolve: () => catalog.modelTools(["getProject"]) })),
      { catalog, tools },
    )
    expect(await restart(executor)).toBe(true)

    expect(client.calls).toHaveLength(1)
    expect(modelCalls()).toBe(1)
    expect(messages[0]?.some(message => message.role === "tool" && message.content.includes("prj_9"))).toBe(true)
    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "completed" })
  })


  it("审批事实已落地但终态投影缺失：新进程恢复计数并持久补齐结果", async () => {
    const owner = "usr_cp_approval_unprojected"
    const { repository, runId } = await queuedRun(owner, "cp-approval-unprojected")
    const catalog = ToolCatalog.load([testToolOperation("restartApplication", { requiresApproval: true, effect: "read" })])
    const { tools, client } = platformTool(repository, catalog, () => ({ status: 200, body: { restarted: "app_1" } }))
    await claimOrThrow(repository)
    const pending = await tools.propose({ runId, operationId: "restartApplication", arguments: {} })
    await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0, phase: "awaiting_approval", modelStep: 1, toolCallCount: 0,
      state: { pendingToolCallId: pending.id },
    })
    await repository.updateRun(runId, "running", "waiting_approval")
    await new ToolOrchestrator(catalog, client, repository.toolCallStore).resolveApproval(pending.id, "approve")
    await repository.updateRun(runId, "waiting_approval", "queued")
    const { provider, messages } = scriptedProvider([{ text: "已重启。" }])
    await testExecutor(repository, new ModelRuntime(provider, testRegistry()), {
      catalog, tools: restartedTool(repository, catalog, client),
    }).runOnce()

    expect(client.calls).toHaveLength(1)
    expect(messages[0]?.filter(message => message.role === "tool")).toHaveLength(1)
    expect(messages[0]?.some(message => message.role === "tool" && message.content.includes("app_1"))).toBe(true)
    expect(await repository.loadRunCheckpoint(runId)).toMatchObject({ toolCallCount: 1 })
    const interactions = (await repository.getExecutionInput(runId))?.toolInteractions
    expect(interactions?.find(item => item.type === "tool_call")?.content.status).toBe("succeeded")
    expect(interactions?.find(item => item.type === "tool_result")?.content.result).toEqual({ restarted: "app_1" })
    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "completed" })
  })

  it("核对后再次退出：下一次从 model 恢复仍能读到补齐的持久结果", async () => {
    const owner = "usr_cp_projection_restart"
    const { repository, runId } = await queuedRun(owner, "cp-projection-restart")
    const catalog = ToolCatalog.load([testToolOperation("getProject")])
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: { id: "prj_recovered" } }))
    await claimOrThrow(repository)
    const settled = await new ToolOrchestrator(catalog, client, repository.toolCallStore)
      .propose({ runId, operationId: "getProject", arguments: {} })
    await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0, phase: "tool_execution", modelStep: 1, toolCallCount: 0, state: {},
    })
    const toolStep = new ToolStepRunner(
      repository,
      new ModelRuntime(scriptedProvider([]).provider, testRegistry()),
      restartedTool(repository, catalog, client),
    )
    const state: ToolStepState = {
      continuationMessages: [],
      selectedOperationIds: [],
      executedOperationIds: new Set(),
      internalReadCallCounts: new Map(),
      conversation: { title: "recover", titleSource: "default", turnIndex: 0 },
      assistantRenamed: false,
      cardRepairAttempts: 0,
      cardRepairExhausted: false,
      providerArgumentRepairAttempts: new Map(),
      searchedToolQueries: new Map(),
      detailedToolRequests: new Map(),
    }
    await toolStep.recoverInterruptedBatch(runId, state)
    const eventCount = (await repository.getEvents(owner, runId, 0)).length
    await toolStep.recoverInterruptedBatch(runId, state)
    expect((await repository.getEvents(owner, runId, 0)).length).toBe(eventCount)
    await repository.saveRunCheckpoint(runId, {
      expectedRevision: 1, phase: "model", modelStep: 1, toolCallCount: 1, state: {},
    })
    const { provider, messages } = scriptedProvider([{ text: "已恢复。" }])
    await restart(testExecutor(repository, new ModelRuntime(provider, testRegistry()), {
      catalog, tools: restartedTool(repository, catalog, client),
    }))
    expect(messages[0]?.filter(message => message.role === "tool" && message.toolCallId === settled.id)).toHaveLength(1)
    expect(messages[0]?.some(message => message.role === "tool" && message.content.includes("prj_recovered"))).toBe(true)
    expect(client.calls).toHaveLength(1)
    expect((await repository.getExecutionInput(runId))?.toolInteractions.filter(item => item.type === "tool_result")).toHaveLength(1)
  })

  it("工具批次未产生任何事实时回到模型，不把批次当成已完成", async () => {
    const owner = "usr_cp_tool_unstarted"
    const { repository, runId } = await queuedRun(owner, "cp-tool-unstarted")
    const catalog = ToolCatalog.load([testToolOperation("getProject")])
    const { tools, client } = platformTool(repository, catalog, () => ({ status: 200, body: {} }))
    await claimOrThrow(repository)
    await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0, phase: "tool_execution", modelStep: 1, toolCallCount: 0, state: {},
    })

    const { provider, modelCalls } = scriptedProvider([{ text: "没有可用结果，已跳过。" }])
    await restart(testExecutor(
      repository,
      new ModelRuntime(provider, testRegistry({ resolve: () => catalog.modelTools(["getProject"]) })),
      { catalog, tools },
    ))

    expect(client.calls).toHaveLength(0)
    expect(modelCalls()).toBe(1)
    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "completed" })
  })

  it("工具调用仍在执行中退出：结果未知，fail closed", async () => {
    const owner = "usr_cp_tool_running"
    const { repository, runId } = await queuedRun(owner, "cp-tool-running")
    const catalog = ToolCatalog.load([testToolOperation("getProject")])
    const { tools, client, store } = platformTool(repository, catalog, () => ({ status: 200, body: {} }))
    await claimOrThrow(repository)
    await store.insert({
      id: "aitool_running", idempotencyKey: "aitool_running", runId, operationId: "getProject", status: "running",
      arguments: {}, argumentsHash: "sha256:args", attempt: 1, rowVersion: 1, executionCount: 1,
    })
    await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0, phase: "tool_execution", modelStep: 1, toolCallCount: 0, state: {},
    })

    const { provider, modelCalls } = scriptedProvider([{ text: "不应被调用。" }])
    await restart(testExecutor(repository, new ModelRuntime(provider, testRegistry()), { catalog, tools }))

    expect(await repository.getRun(owner, runId)).toMatchObject({
      status: "failed",
      errorCode: "ai.tool_outcome_unknown",
    })
    expect(client.calls).toHaveLength(0)
    expect(modelCalls()).toBe(0)
  })

  it("崩溃批次已落地的调用计入循环保护计数，工具预算不放宽", async () => {
    const owner = "usr_cp_guard_raise"
    const { repository, runId } = await queuedRun(owner, "cp-guard-raise")
    const catalog = ToolCatalog.load([testToolOperation("getProject"), testToolOperation("listDeployments")])
    const { client } = platformTool(repository, catalog, () => ({ status: 200, body: { ok: true } }))
    const crashedProcess = restartedTool(repository, catalog, client)
    await claimOrThrow(repository)
    // 批次执行两个工具后进程退出：checkpoint 的 toolCallCount 还停在批次开始时的 0。
    const first = await crashedProcess.propose({ runId, operationId: "getProject", arguments: {} })
    const second = await crashedProcess.propose({ runId, operationId: "listDeployments", arguments: {} })
    expect([first.status, second.status]).toEqual(["succeeded", "succeeded"])
    await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0, phase: "tool_execution", modelStep: 1, toolCallCount: 0, state: {},
    })

    const { provider, modelCalls } = scriptedProvider([{ text: "两项检查都已完成。" }])
    const executor = testExecutor(
      repository,
      new ModelRuntime(provider, testRegistry({
        resolve: () => catalog.modelTools(["getProject", "listDeployments"]),
      })),
      { catalog, tools: restartedTool(repository, catalog, client) },
    )
    expect(await restart(executor)).toBe(true)

    expect(modelCalls()).toBe(1)
    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "completed" })
    // 恢复后的 checkpoint 必须反映事实表里已执行的两次调用。
    expect((await repository.loadRunCheckpoint(runId))?.toolCallCount).toBe(2)
    expect(client.calls).toHaveLength(2)
  })

  it("崩溃批次已落地的同参数 GET 仍重新请求实时平台状态", async () => {
    const owner = "usr_cp_guard_repeat"
    const { repository, runId } = await queuedRun(owner, "cp-guard-repeat")
    const catalog = ToolCatalog.load([testToolOperation("getProject", { inputSchema: requiredProjectInput })])
    const { client } = platformTool(repository, catalog, () => ({ status: 200, body: { ok: true } }))
    const crashedProcess = restartedTool(repository, catalog, client)
    await claimOrThrow(repository)
    expect((await crashedProcess.propose({ runId, operationId: "getProject", arguments: { projectId: "prj_1" } })).status).toBe("succeeded")
    await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0, phase: "tool_execution", modelStep: 1, toolCallCount: 1, state: {},
    })

    // 普通读取不属于幂等副作用，重启后再次调用仍返回平台当前事实。
    const { provider, modelCalls } = scriptedProvider([
      { toolCalls: [{ id: "repeat-1", operationId: "getProject", arguments: { projectId: "prj_1" } }] },
      { text: "已停止重复调用。" },
    ])
    const executor = testExecutor(
      repository,
      new ModelRuntime(provider, testRegistry({ resolve: () => catalog.modelTools(["getProject"]) })),
      { catalog, tools: restartedTool(repository, catalog, client) },
    )
    expect(await restart(executor)).toBe(true)

    expect(modelCalls()).toBe(2)
    expect(client.calls).toHaveLength(2)
    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "completed" })
  })

  it.each(["tool_execution", "awaiting_approval"] as const)("%s 核对工具事实失败时 fail closed，checkpoint 不被推进", async (phase) => {
    const owner = "usr_cp_facts_unavailable"
    const { repository, runId } = await queuedRun(owner, "cp-facts-unavailable")
    class UnavailableFactStore extends MemoryToolCallStore {
      override async listByRun(): Promise<ToolCallRecord[]> { throw new Error("ai.tool_persistence_failed") }
    }
    const catalog = ToolCatalog.load([testToolOperation("getProject")])
    const store = new UnavailableFactStore()
    const tools = new ToolOrchestrator(catalog, new DeterministicLunaApiClient(() => ({ status: 200, body: {} })), store)
    await claimOrThrow(repository)
    await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0, phase, modelStep: 1, toolCallCount: 0, state: { pendingToolCallId: "aitool_pending" },
    })

    const { provider, modelCalls } = scriptedProvider([{ text: "不应被调用。" }])
    await restart(testExecutor(repository, new ModelRuntime(provider, testRegistry()), { catalog, tools }))

    expect(await repository.getRun(owner, runId)).toMatchObject({
      status: "failed",
      errorCode: "ai.tool_persistence_failed",
    })
    expect(modelCalls()).toBe(0)
    // 控制流位置保持原样，供人工核对，不被猜测推进。
    expect(await repository.loadRunCheckpoint(runId)).toMatchObject({ phase, modelStep: 1 })
  })


  it("补写投影失败时关闭 Run 并保留批次 checkpoint", async () => {
    const owner = "usr_cp_projection_failed"
    const { repository, runId } = await queuedRun(owner, "cp-projection-failed")
    const catalog = ToolCatalog.load([testToolOperation("getProject")])
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: { ok: true } }))
    await claimOrThrow(repository)
    await new ToolOrchestrator(catalog, client, repository.toolCallStore)
      .propose({ runId, operationId: "getProject", arguments: {} })
    await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0, phase: "tool_execution", modelStep: 1, toolCallCount: 0, state: {},
    })
    repository.completeToolItemWithEvent = () => Promise.reject(new Error("connection closed"))
    const { provider, modelCalls } = scriptedProvider([{ text: "不应被调用。" }])
    await restart(testExecutor(repository, new ModelRuntime(provider, testRegistry()), {
      catalog, tools: restartedTool(repository, catalog, client),
    }))
    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "failed", errorCode: "ai.tool_persistence_failed" })
    expect(await repository.loadRunCheckpoint(runId)).toMatchObject({ phase: "tool_execution", toolCallCount: 0 })
    expect(modelCalls()).toBe(0)
    expect(client.calls).toHaveLength(1)
  })

  it("工具批次停在等待审批：重启回到等待审批而不是失败", async () => {
    const owner = "usr_cp_tool_awaiting"
    const { repository, runId } = await queuedRun(owner, "cp-tool-awaiting", "重启应用 app_1")
    const catalog = ToolCatalog.load([testToolOperation("restartApplication", {
      effect: "read",
      method: "POST",
      path: "/api/v1/applications/:id/restart",
      requiresApproval: true,
      requestBody: true,
      inputSchema: {
        type: "object",
        properties: { applicationId: { type: "string" } },
        required: ["applicationId"],
        additionalProperties: false,
      },
    })])
    const { tools, client } = platformTool(repository, catalog, () => ({ status: 200, body: { status: "restarted" } }))
    await claimOrThrow(repository)
    const pending = await tools.propose({ runId, operationId: "restartApplication", arguments: { applicationId: "app_1" } })
    expect(pending.status).toBe("awaiting_approval")
    // 真实窗口：审批等待已落库，但 checkpoint 还停在 tool_execution。
    await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0, phase: "tool_execution", modelStep: 1, toolCallCount: 0, state: {},
    })

    const { provider, modelCalls } = scriptedProvider([{ text: "应用已重启。" }])
    const executor = testExecutor(
      repository,
      new ModelRuntime(provider, testRegistry({ resolve: () => catalog.modelTools(["restartApplication"]) })),
      { catalog, tools },
    )
    expect(await restart(executor)).toBe(true)

    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "waiting_approval" })
    expect(client.calls).toHaveLength(0)

    await tools.resolveApproval(pending.id, "approve")
    await repository.updateRun(runId, "waiting_approval", "queued")
    expect(await executor.runOnce()).toBe(true)

    expect(client.calls).toHaveLength(1)
    expect(modelCalls()).toBe(1)
    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "completed" })
  })

  it("等待用户输入时写入 checkpoint，追加补充消息不篡改首轮 turns.input，重新排队后继续到完成", async () => {
    const owner = "usr_cp_waiting_input"
    const { repository, runId } = await queuedRun(owner, "cp-waiting-input", "请检查项目")
    const catalog = ToolCatalog.load([testToolOperation("getProject", { inputSchema: requiredProjectInput })])
    const { tools, client } = platformTool(repository, catalog, () => ({ status: 200, body: { id: "prj_1" } }))
    const originalPropose = tools.propose.bind(tools)
    let interceptedOnce = false
    tools.propose = async (input, signal) => {
      if (!interceptedOnce) {
        interceptedOnce = true
        throw new ToolInterruption("waiting_input", ["projectId"])
      }
      return originalPropose(input, signal)
    }
    const { provider, modelCalls, messages } = scriptedProvider([
      { toolCalls: [{ id: "need-input", operationId: "getProject", arguments: { projectId: "placeholder" } }] },
      { toolCalls: [{ id: "filled", operationId: "getProject", arguments: { projectId: "prj_1" } }] },
      { text: "已查询项目。" },
    ])
    const runtime = new ModelRuntime(provider, testRegistry({ resolve: () => catalog.modelTools(["getProject"]) }))
    await testExecutor(repository, runtime, { catalog, tools }).runOnce()

    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "waiting_input" })
    expect(await repository.loadRunCheckpoint(runId)).toMatchObject({
      phase: "awaiting_input",
      modelStep: 1,
    })

    await repository.appendRunInput(runId, "项目为 prj_1")
    await repository.updateRun(runId, "waiting_input", "queued")
    expect((await repository.getExecutionInput(runId))?.input).toBe("请检查项目")

    await testExecutor(repository, runtime, { catalog, tools }).runOnce()

    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "completed" })
    expect(client.calls).toHaveLength(1)
    expect(modelCalls()).toBe(3)
    expect(messages[1]?.some(msg => msg.role === "user" && msg.content === "项目为 prj_1")).toBe(true)
  })

  it("恢复时完整保留内部工具结果，按模型步合并并行调用且严格隔离每步 reasoning", async () => {
    const owner = "usr_cp_internal_and_parallel"
    const { repository, runId } = await queuedRun(owner, "cp-internal-parallel", "查找项目并重启应用")
    const catalog = ToolCatalog.load([
      testToolOperation("getProject", { inputSchema: requiredProjectInput }),
      testToolOperation("listDeployments", { inputSchema: requiredProjectInput }),
      testToolOperation("restartApplication", {
        effect: "read",
        method: "POST",
        path: "/api/v1/applications/:id/restart",
        requiresApproval: true,
        requestBody: true,
        inputSchema: {
          type: "object",
          properties: { applicationId: { type: "string" } },
          required: ["applicationId"],
          additionalProperties: false,
        },
      }),
    ])
    const { tools, client } = platformTool(repository, catalog, request => ({
      status: 200,
      body: { ok: true, op: request.operation.operationId },
    }))
    const { provider, messages } = scriptedProvider([
      {
        reasoning: "第一步思考：先检索工具并并行读取项目和部署。",
        toolCalls: [
          { id: "call_search", operationId: "search_tools", arguments: { query: "项目" } },
          { id: "call_proj", operationId: "getProject", arguments: { projectId: "prj_1" } },
          { id: "call_deploys", operationId: "listDeployments", arguments: { projectId: "prj_1" } },
        ],
      },
      {
        reasoning: "第二步思考：发起需要审批的应用重启。",
        toolCalls: [
          { id: "call_restart", operationId: "restartApplication", arguments: { applicationId: "app_1" } },
        ],
      },
      { text: "已完成全部读取与重启。" },
    ])
    const runtime = new ModelRuntime(provider, testRegistry({
      resolve: () => [...catalog.modelTools(["getProject", "listDeployments", "restartApplication"]), searchToolsTool],
      search: input => ({
        ...catalog.search(input),
        loadedOperationIds: ["getProject"],
        missingOperationIds: [],
        catalogDigest: catalog.digest,
        duplicate: false,
        cacheHit: false,
      }),
    }))
    const executor = testExecutor(repository, runtime, { catalog, tools })

    await executor.runOnce()
    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "waiting_approval" })

    const pending = [...repository.toolCallStore.records.values()].find(call => call.status === "awaiting_approval")
    if (!pending) throw new Error("missing pending approval")
    await tools.resolveApproval(pending.id, "approve")
    await repository.updateRun(runId, "waiting_approval", "queued")
    await executor.runOnce()

    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "completed" })
    expect(client.calls).toHaveLength(3)

    // 检查审批恢复后第 3 次模型调用收到的完整上下文：
    const resumedRequest = messages[2] ?? []
    const assistantToolMsgs = resumedRequest.filter(
      (msg): msg is Extract<ModelMessage, { role: "assistant" }> => msg.role === "assistant" && Boolean(msg.toolCalls?.length),
    )
    expect(assistantToolMsgs).toHaveLength(2)
    expect(assistantToolMsgs[0]?.reasoning).toBe("第一步思考：先检索工具并并行读取项目和部署。")
    expect(assistantToolMsgs[0]?.toolCalls?.map(c => c.id)).toEqual(["call_search", "call_proj", "call_deploys"])
    expect(assistantToolMsgs[1]?.reasoning).toBe("第二步思考：发起需要审批的应用重启。")
    expect(assistantToolMsgs[1]?.toolCalls?.map(c => c.id)).toEqual(["call_restart"])

    const toolMsgs = resumedRequest.filter((msg): msg is Extract<ModelMessage, { role: "tool" }> => msg.role === "tool")
    expect(toolMsgs.map(m => m.toolCallId)).toEqual(["call_search", "call_proj", "call_deploys", "call_restart"])
  })

  it("同批次 [toolA, toolB_approval, toolC] 在 toolB 暂停时持久化剩余队列，恢复后自动执行完 toolC", async () => {
    const owner = "usr_cp_mid_batch_pause"
    const { repository, runId } = await queuedRun(owner, "cp-mid-batch-pause", "读取、重启并拉取部署列表")
    const catalog = ToolCatalog.load([
      testToolOperation("getProject", { inputSchema: requiredProjectInput }),
      testToolOperation("restartApplication", {
        effect: "read",
        method: "POST",
        path: "/api/v1/applications/:id/restart",
        requiresApproval: true,
        requestBody: true,
        inputSchema: {
          type: "object",
          properties: { applicationId: { type: "string" } },
          required: ["applicationId"],
          additionalProperties: false,
        },
      }),
      testToolOperation("listDeployments", { inputSchema: requiredProjectInput }),
    ])
    const { tools, client } = platformTool(repository, catalog, request => ({
      status: 200,
      body: { executed: request.operation.operationId },
    }))
    const { provider, modelCalls, messages } = scriptedProvider([
      {
        reasoning: "单步并行下发三个工具。",
        toolCalls: [
          { id: "step_a", operationId: "getProject", arguments: { projectId: "prj_1" } },
          { id: "step_b", operationId: "restartApplication", arguments: { applicationId: "app_1" } },
          { id: "step_c", operationId: "listDeployments", arguments: { projectId: "prj_1" } },
        ],
      },
      { text: "三个工具均已顺序执行完成。" },
    ])
    const runtime = new ModelRuntime(provider, testRegistry({
      resolve: () => catalog.modelTools(["getProject", "restartApplication", "listDeployments"]),
    }))
    const executor = testExecutor(repository, runtime, { catalog, tools })

    await executor.runOnce()
    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "waiting_approval" })
    expect(client.calls.map(c => c.operation.operationId)).toEqual(["getProject"])
    const pausedCheckpoint = await repository.loadRunCheckpoint(runId)
    expect(pausedCheckpoint?.state.pendingBatchCalls).toEqual([
      { id: "step_c", operationId: "listDeployments", arguments: { projectId: "prj_1" } },
    ])

    const pending = [...repository.toolCallStore.records.values()].find(call => call.status === "awaiting_approval")
    if (!pending) throw new Error("missing pending approval")
    await tools.resolveApproval(pending.id, "approve")
    await repository.updateRun(runId, "waiting_approval", "queued")

    await executor.runOnce()
    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "completed" })
    expect(modelCalls()).toBe(2)
    expect(client.calls.map(c => c.operation.operationId)).toEqual(["getProject", "restartApplication", "listDeployments"])

    const finalStepMessages = messages[1] ?? []
    const assistantBatch = finalStepMessages.find(
      (msg): msg is Extract<ModelMessage, { role: "assistant" }> => msg.role === "assistant" && Boolean(msg.toolCalls?.length),
    )
    expect(assistantBatch?.toolCalls?.map(c => c.id)).toEqual(["step_a", "step_b", "step_c"])
    expect(finalStepMessages.filter(m => m.role === "tool").map(m => m.toolCallId)).toEqual(["step_a", "step_b", "step_c"])
  })

  it("清理崩溃遗留的未发出孤儿 proposed（executionCount === 0）记录，不毒化后续幂等重试", async () => {
    const owner = "usr_cp_orphan_proposed"
    const { repository, runId } = await queuedRun(owner, "cp-orphan-proposed", "触发构建")
    const catalog = ToolCatalog.load([
      testToolOperation("triggerBuildRun", {
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
    ])
    const { client, store } = platformTool(repository, catalog, () => ({ status: 201, body: { id: "bldr_real", status: "queued" } }))
    await claimOrThrow(repository)
    // 模拟崩溃前刚 insert 到 ai.tool_calls 但尚未发往 Go API（executionCount === 0）
    await store.insert({
      id: "aitool_orphan_proposed",
      idempotencyKey: "aiidem_orphan",
      runId,
      operationId: "triggerBuildRun",
      status: "proposed",
      arguments: { projectId: "prj_1", body: { applicationId: "app_1" } },
      argumentsHash: "sha256:any",
      attempt: 1,
      rowVersion: 1,
      executionCount: 0,
    })
    await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0,
      phase: "tool_execution",
      modelStep: 0,
      toolCallCount: 0,
      state: {},
    })

    const { provider } = scriptedProvider([
      { toolCalls: [{ id: "retry_trigger", operationId: "triggerBuildRun", arguments: { projectId: "prj_1", body: { applicationId: "app_1" } } }] },
      { text: "已真实触发构建 bldr_real。" },
    ])
    const executor = testExecutor(
      repository,
      new ModelRuntime(provider, testRegistry({ resolve: () => catalog.modelTools(["triggerBuildRun"]) })),
      { catalog, tools: restartedTool(repository, catalog, client) },
    )
    expect(await restart(executor)).toBe(true)

    expect(store.records.get("aitool_orphan_proposed")?.status).toBe("skipped")
    expect(client.calls).toHaveLength(1)
    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "completed" })
  })

  it("恢复次数达到上限后按中断终止，不再续跑", async () => {
    const owner = "usr_cp_recovery_limit"
    const { repository, runId } = await queuedRun(owner, "cp-recovery-limit")
    await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0, phase: "model", modelStep: 0, toolCallCount: 0, state: {},
    })
    const { provider, modelCalls } = scriptedProvider([{ text: "不应被调用。" }])
    const executor = testExecutor(repository, new ModelRuntime(provider, testRegistry()))

    for (let attempt = 0; attempt < maxRunRecoveries; attempt += 1) {
      await claimOrThrow(repository)
      expect(await reconcile(executor)).toBe(true)
      expect(await repository.getRun(owner, runId)).toMatchObject({ status: "queued" })
    }
    await claimOrThrow(repository)
    await reconcile(executor)

    expect(await repository.getRun(owner, runId)).toMatchObject({
      status: "interrupted",
      errorCode: "ai.agent_restarted",
    })
    expect((await repository.loadRunCheckpoint(runId))?.recoveries).toBe(maxRunRecoveries)
    expect(modelCalls()).toBe(0)
  })

  it("[P0-1] 真实 PG 下 ToolCallRecord.modelCallId 为 undefined 时，recoverInterruptedBatch 从 projected item 回退读取且绝不重复注入消息", async () => {
    const owner = "usr_p0_1_pg_model_call_id"
    const { repository, runId } = await queuedRun(owner, "p0-1-pg-model-call-id")
    const catalog = ToolCatalog.load([testToolOperation("getProject")])
    const { tools, client, store } = platformTool(repository, catalog, () => ({ status: 200, body: { id: "prj_pg" } }))
    const claimed = await claimOrThrow(repository)

    const proposed = await tools.propose({
      runId,
      modelCallId: "call_pg_0",
      batchId: "batch_0",
      operationId: "getProject",
      arguments: {},
    })
    expect(proposed.status).toBe("succeeded")
    // 模拟真实 PostgreSQL：ai.tool_calls 无 model_call_id / batch_id 列，listByRun 返回的对象上这两个属性恒为 undefined
    const rawRecord = store.records.get(proposed.id)!
    delete rawRecord.modelCallId
    delete rawRecord.batchId

    const executionInput = await repository.getExecutionInput(runId)
    if (!executionInput) throw new Error("missing executionInput")
    const checkpointVal = await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0, phase: "tool_execution", modelStep: 1, toolCallCount: 1, state: {},
    })
    const toolStep = new ToolStepRunner(
      repository,
      new ModelRuntime(scriptedProvider([]).provider, testRegistry()),
      restartedTool(repository, catalog, client),
    )
    const state = await toolStep.initialState(claimed, executionInput, checkpointVal)
    expect(state.continuationMessages.filter(m => m.role === "tool")).toHaveLength(1)

    await toolStep.recoverInterruptedBatch(runId, state)
    const toolMessages = state.continuationMessages.filter(m => m.role === "tool")
    expect(toolMessages).toHaveLength(1)
    expect(toolMessages[0]).toMatchObject({ role: "tool", toolCallId: "call_pg_0" })
  })

  it("[P0-3] pendingBatchCalls 续跑途中部分工具落库后崩溃，重启恢复不重复执行已完成工具且不产生重复 toolCallId", async () => {
    const owner = "usr_p0_3_pending_batch_crash"
    const { repository, runId } = await queuedRun(owner, "p0-3-pending-batch-crash")
    const catalog = ToolCatalog.load([
      testToolOperation("getProject"),
      testToolOperation("listApplications"),
    ])
    let failSecondTool = true
    const client = new DeterministicLunaApiClient((req) => {
      if (req.operation.operationId === "listApplications" && failSecondTool) {
        throw new Error("simulated crash during toolC")
      }
      return { status: 200, body: { ok: req.operation.operationId } }
    })
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    await claimOrThrow(repository)

    // 先完成批次内 toolB（call_0_1），模拟其在进程崩溃前已完整持久化到 ai.tool_calls 与 ai.items
    const settledB = await tools.propose({
      runId,
      modelCallId: "call_0_1",
      batchId: "batch_0",
      operationId: "getProject",
      arguments: {},
    })
    // 同时模拟真实 PG 无 modelCallId 列
    delete repository.toolCallStore.records.get(settledB.id)!.modelCallId
    expect(failSecondTool).toBe(true)
    failSecondTool = false

    await repository.saveRunCheckpoint(runId, {
      expectedRevision: 0,
      phase: "tool_execution",
      modelStep: 1,
      toolCallCount: 1,
      state: {
        pendingBatchId: "batch_0",
        pendingBatchCalls: [
          { id: "call_0_1", operationId: "getProject", arguments: {} },
          { id: "call_0_2", operationId: "listApplications", arguments: {} },
        ],
      },
    })

    const { provider, messages } = scriptedProvider([{ text: "续跑全部完成。" }])
    const executor = testExecutor(
      repository,
      new ModelRuntime(provider, testRegistry({ resolve: () => catalog.modelTools(["getProject", "listApplications"]) })),
      { catalog, tools: restartedTool(repository, catalog, client) },
    )
    expect(await restart(executor)).toBe(true)
    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "completed" })
    // getProject 绝不能在恢复时被重复调用，全程只有初始 1 次 getProject + 续跑 1 次 listApplications
    expect(client.calls.map(c => c.operation.operationId)).toEqual(["getProject", "listApplications"])
    const sentToolIds = messages[0]?.flatMap(m => m.role === "tool" ? [m.toolCallId] : []) ?? []
    expect(sentToolIds).toEqual(["call_0_1", "call_0_2"])
  })

  it("[P2-2] 多步模型循环每次调用 streamModel 前均校验可用余额，且无工具模型步的 progress 不会串入后续工具步", async () => {
    const owner = "usr_p2_2_balance_loop"
    const repository = new TestRepository()
    repository.setWalletBalance(owner, 10)
    const conversation = await repository.createConversation(owner, "balance-loop", "aimod_test")
    const created = await repository.createTurn(owner, {
      conversationId: conversation.id,
      modelId: "aimod_test",
      modelSnapshot: {
        id: "aimod_test",
        name: "Test Model",
        maxContextTokens: 128_000,
        maxOutputTokens: 8_192,
        inputCreditsPerMillion: "1",
        outputCreditsPerMillion: "2",
        cachedInputCreditsPerMillion: "0.5",
      },
      input: "多步任务",
      pageContext: {},
      idempotencyKey: "p2-2-balance",
      actorSessionId: "ses_p2_2",
    })
    const catalog = ToolCatalog.load([testToolOperation("getProject")])
    const client = new DeterministicLunaApiClient(() => {
      // 第一步工具执行期间模拟上一轮计费结算将钱包扣至 0
      repository.setWalletBalance(owner, 0)
      return { status: 200, body: { id: "prj_1" } }
    })
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    const { provider, modelCalls } = scriptedProvider([
      { toolCalls: [{ id: "call_step0", operationId: "getProject", arguments: {} }] },
      { text: "不应到达第 2 步模型调用。" },
    ])
    const executor = testExecutor(
      repository,
      new ModelRuntime(provider, testRegistry({ resolve: () => catalog.modelTools(["getProject"]) })),
      { catalog, tools },
    )
    await executor.runOnce()
    expect(modelCalls()).toBe(1)
    expect(await repository.getRun(owner, created.run.id)).toMatchObject({
      status: "failed",
      errorCode: "ai.wallet_balance_insufficient",
    })
  })
})
