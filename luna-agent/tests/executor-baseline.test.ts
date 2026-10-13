import { afterEach, describe, expect, it, vi } from "vitest"
import type { TimelineItem } from "../src/domain.js"
import { ModelRuntime } from "../src/model-runtime.js"
import type { ModelProvider, ModelToolCall } from "../src/provider/provider.js"
import { defaultRuntimeSettings } from "../src/runtime-settings.js"
import { ToolCatalog } from "../src/tools/catalog.js"
import { DeterministicLunaApiClient } from "../src/tools/luna-api-client.js"
import { ProjectingToolCallStore, ToolOrchestrator } from "../src/tools/orchestrator.js"
import { InMemoryRunStreamBus } from "../src/run-stream-bus.js"
import { testRemoteConfigSource, testRemoteProviderConfig } from "./support/remote-config.js"
import { TestRepository } from "./support/test-repository.js"
import { testRegistry } from "./support/model-tool-registry.js"
import { testExecutor } from "./support/run-executor.js"
import { testToolOperation } from "./support/tool-catalog.js"

const reportedUsage = { status: "reported" as const, value: { inputTokens: 3, outputTokens: 2, totalTokens: 5 } }

const presentationCardInput = {
  schemaVersion: 1,
  title: "部署结果",
  mode: "presentation",
  template: "result",
  cards: [{
    id: "result",
    presentation: { variant: "receipt", title: "部署成功" },
    blocks: [{ id: "summary", type: "callout", tone: "success", content: "应用已通过健康检查。" }],
  }],
}

const inputCardInput = {
  schemaVersion: 1,
  title: "配置应用",
  mode: "interactive",
  placement: "turn_end",
  template: "form",
  cards: [{
    id: "configuration",
    presentation: { variant: "form", title: "应用配置" },
    form: { sections: [{ id: "main", fields: [{ id: "name", type: "text", label: "应用名称", required: true }] }] },
    actions: [{ id: "submit", type: "send_message", label: "继续", message: "创建应用 {{name}}" }],
  }],
}

type ScriptedStep = {
  text?: string
  toolCalls?: ModelToolCall[]
}

function scriptedProvider(steps: ScriptedStep[]) {
  const state = { modelCalls: 0 }
  const provider: ModelProvider = {
    async *stream() {
      const step = steps[Math.min(state.modelCalls, steps.length - 1)] ?? {}
      state.modelCalls += 1
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
  return { provider, modelCalls: () => state.modelCalls }
}

function hangingProvider() {
  const provider: ModelProvider = {
    async *stream(request) {
      await new Promise<void>((_resolve, reject) => {
        const abort = () => reject(request.signal?.reason instanceof Error ? request.signal.reason : new Error("ai.run_canceled"))
        if (request.signal?.aborted) abort()
        request.signal?.addEventListener("abort", abort, { once: true })
      })
      yield { type: "completed", usage: reportedUsage }
    },
    async complete() { return { text: "", toolCalls: [], usage: reportedUsage } },
    capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
    health: async () => ({ ok: true }),
  }
  return provider
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

function platformTool(repository: TestRepository, catalog: ToolCatalog, result: () => { status: number, body: unknown }) {
  const store = repository.toolCallStore
  const client = new DeterministicLunaApiClient(result)
  return {
    store,
    client,
    tools: new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(store, repository)),
  }
}

const planText = "我先梳理一下执行步骤：1) 查询项目空间；2) 检查部署配置的当前状态。"

describe("RunExecutor 当前行为基线（Phase 0 特征化测试）", () => {
  afterEach(() => vi.useRealTimers())

  it("普通问答一次模型调用完成", async () => {
    const { repository, conversation, runId } = await queuedRun("usr_answer", "answer", "你好")
    const { provider, modelCalls } = scriptedProvider([{ text: "你好。" }])
    await testExecutor(repository, new ModelRuntime(provider, testRegistry())).runOnce()
    expect((await repository.getRun("usr_answer", runId))?.status).toBe("completed")
    expect(modelCalls()).toBe(1)
    expect(assistantTexts(await timelineItems(repository, "usr_answer", conversation.id))).toEqual(["你好。"])
  })

  it("工具过程文字和最终回答都持久化为 assistant_message", async () => {
    const owner = "usr_progress"
    const { repository, conversation, runId } = await queuedRun(owner, "progress")
    const catalog = ToolCatalog.load([testToolOperation("getProject")])
    const { tools } = platformTool(repository, catalog, () => ({ status: 200, body: {} }))
    const { provider, modelCalls } = scriptedProvider([
      { text: "正在查询。", toolCalls: [{ id: "progress-1", operationId: "getProject", arguments: {} }] },
      { text: "查询完成。" },
    ])
    await testExecutor(repository, new ModelRuntime(provider, testRegistry({ resolve: () => catalog.modelTools(["getProject"]) })), { catalog, tools }).runOnce()
    expect((await repository.getRun(owner, runId))?.status).toBe("completed")
    expect(modelCalls()).toBe(2)
    expect(assistantTexts(await timelineItems(repository, owner, conversation.id))).toEqual(["正在查询。", "查询完成。"])
  })

  it("缺必填参数先等待输入，重新排队后完成同一 Run", async () => {
    const owner = "usr_input_resume"
    const { repository, runId } = await queuedRun(owner, "input-resume")
    const catalog = ToolCatalog.load([testToolOperation("getProject", {
      inputSchema: { type: "object", properties: { projectId: { type: "string" } }, required: ["projectId"], additionalProperties: false },
    })])
    const { tools, client } = platformTool(repository, catalog, () => ({ status: 200, body: { id: "prj_1" } }))
    const { provider, modelCalls } = scriptedProvider([
      { toolCalls: [{ id: "missing", operationId: "getProject", arguments: {} }] },
      { toolCalls: [{ id: "filled", operationId: "getProject", arguments: { projectId: "prj_1" } }] },
      { text: "已查询项目。" },
    ])
    const runtime = new ModelRuntime(provider, testRegistry({ resolve: () => catalog.modelTools(["getProject"]) }))
    await testExecutor(repository, runtime, { catalog, tools }).runOnce()
    expect((await repository.getRun(owner, runId))?.status).toBe("waiting_input")
    expect(client.calls).toHaveLength(0)
    expect((await repository.getEvents(owner, runId, 0)).some(event => event.type === "run.input_required")).toBe(true)
    await repository.appendRunInput(runId, "项目为 prj_1")
    await repository.updateRun(runId, "waiting_input", "queued")
    await testExecutor(repository, runtime, { catalog, tools }).runOnce()
    expect((await repository.getRun(owner, runId))?.status).toBe("completed")
    expect(modelCalls()).toBe(3)
    expect(client.calls).toHaveLength(1)
  })

  it("用户并发配额超限在模型调用前失败", async () => {
    const owner = "usr_quota"
    const { repository, conversation, runId } = await queuedRun(owner, "quota")
    await repository.createTurn(owner, { conversationId: conversation.id, input: "第二个任务", pageContext: {}, idempotencyKey: "quota-2" })
    const { provider, modelCalls } = scriptedProvider([{ text: "不应调用。" }])
    const remote = testRemoteConfigSource(testRemoteProviderConfig({ runtime: { userConcurrentRuns: 1 } }))
    await testExecutor(repository, new ModelRuntime(provider, testRegistry()), { runtimeConfig: remote.source }).runOnce()
    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "failed", errorCode: "ai.quota.user_concurrent_runs_exceeded" })
    expect(modelCalls()).toBe(0)
  })
  it("用例 2 当前行为：首步计划文字没有 tool call 时直接成为最终答复并完成 Run", async () => {
    const owner = "usr_baseline_plan_first"
    const { repository, conversation, runId } = await queuedRun(owner, "baseline-plan-first-step")
    const { provider, modelCalls } = scriptedProvider([{ text: planText }])

    await testExecutor(repository, new ModelRuntime(provider, testRegistry())).runOnce()

    expect((await repository.getRun(owner, runId))?.status).toBe("completed")
    expect(modelCalls()).toBe(1)
    const items = await timelineItems(repository, owner, conversation.id)
    expect(assistantTexts(items)).toEqual([planText])
    expect(items.filter(item => item.type === "tool_call")).toHaveLength(0)
  })

  it("用例 3 当前行为：平台工具成功后只输出计划文字也会完成 Run", async () => {
    const owner = "usr_baseline_plan_after_tool"
    const { repository, conversation, runId } = await queuedRun(owner, "baseline-plan-after-tool")
    const catalog = ToolCatalog.load([testToolOperation("getProject")])
    const { client, store, tools } = platformTool(repository, catalog, () => ({ status: 200, body: { project: { id: "prj_1" } } }))
    const { provider, modelCalls } = scriptedProvider([
      { toolCalls: [{ id: "call-1", operationId: "getProject", arguments: {} }] },
      { text: planText },
    ])
    const executor = testExecutor(
      repository,
      new ModelRuntime(provider, testRegistry({ resolve: () => catalog.modelTools(["getProject"]) })),
      { catalog, tools },
    )

    await executor.runOnce()

    expect((await repository.getRun(owner, runId))?.status).toBe("completed")
    expect(modelCalls()).toBe(2)
    expect(client.calls).toHaveLength(1)
    expect([...store.records.values()][0]).toMatchObject({ status: "succeeded" })
    expect(assistantTexts(await timelineItems(repository, owner, conversation.id))).toEqual([planText])
  })

  it("用例 4a 当前行为：空回答在模型步内重试，后续文本仍能完成 Run", async () => {
    const owner = "usr_baseline_empty_then_answer"
    const { repository, conversation, runId } = await queuedRun(owner, "baseline-empty-then-answer")
    const { provider, modelCalls } = scriptedProvider([{}, { text: "部署当前正常。" }])

    await testExecutor(repository, new ModelRuntime(provider, testRegistry())).runOnce()

    expect((await repository.getRun(owner, runId))?.status).toBe("completed")
    expect(modelCalls()).toBe(2)
    expect(assistantTexts(await timelineItems(repository, owner, conversation.id))).toEqual(["部署当前正常。"])
  })

  it("用例 4b 当前行为：连续空回答没有独立预算，耗尽模型步数后报 ai.final_response_missing", async () => {
    const owner = "usr_baseline_empty_exhausted"
    const { repository, runId } = await queuedRun(owner, "baseline-empty-exhausted")
    const { provider, modelCalls } = scriptedProvider([{}])

    await testExecutor(repository, new ModelRuntime(provider, testRegistry())).runOnce()

    const run = await repository.getRun(owner, runId)
    expect(run?.status).toBe("failed")
    expect(run?.errorCode).toBe("ai.final_response_missing")
    expect(modelCalls()).toBe(defaultRuntimeSettings.maxModelSteps)
  })

  it("用例 5 当前行为：模型步数耗尽时报 ai.limit_exceeded", async () => {
    const owner = "usr_baseline_steps_exhausted"
    const { repository, runId } = await queuedRun(owner, "baseline-steps-exhausted")
    const { provider, modelCalls } = scriptedProvider([
      { toolCalls: [{ id: "search-1", operationId: "search_tools", arguments: { query: "project" } }] },
    ])

    await testExecutor(repository, new ModelRuntime(provider, testRegistry())).runOnce()

    const run = await repository.getRun(owner, runId)
    expect(run?.status).toBe("failed")
    expect(run?.errorCode).toBe("ai.limit_exceeded")
    expect(modelCalls()).toBe(defaultRuntimeSettings.maxModelSteps)
  })

  it("用例 8 当前行为：present_card 之后回到模型而不是结束 Run", async () => {
    const owner = "usr_baseline_present_card"
    const { repository, runId } = await queuedRun(owner, "baseline-present-card")
    const { provider, modelCalls } = scriptedProvider([
      { toolCalls: [{ id: "card-1", operationId: "present_card", arguments: presentationCardInput }] },
      { text: "以上是最近一次部署的结果。" },
    ])

    await testExecutor(repository, new ModelRuntime(provider, testRegistry())).runOnce()

    expect((await repository.getRun(owner, runId))?.status).toBe("completed")
    expect(modelCalls()).toBe(2)
  })

  it.each([
    { operationId: "request_input", arguments: inputCardInput },
    { operationId: "request_choice", arguments: {
      schemaVersion: 1, title: "选择项目", mode: "interactive", template: "candidates",
      cards: [
        { id: "alpha", presentation: { variant: "resource", title: "Alpha" }, actions: [{ id: "select", type: "send_message", label: "选择 Alpha", message: "选择 Alpha" }] },
        { id: "beta", presentation: { variant: "resource", title: "Beta" }, actions: [{ id: "select", type: "send_message", label: "选择 Beta", message: "选择 Beta" }] },
      ],
    } },
  ])("用例 9 当前行为：$operationId 直接完成 Run 并结束模型循环", async (call) => {
    const owner = "usr_baseline_request_input"
    const { repository, runId } = await queuedRun(owner, "baseline-request-input")
    const { provider, modelCalls } = scriptedProvider([
      { toolCalls: [{ id: "input-1", operationId: call.operationId, arguments: call.arguments }] },
    ])

    await testExecutor(repository, new ModelRuntime(provider, testRegistry())).runOnce()

    expect((await repository.getRun(owner, runId))?.status).toBe("completed")
    expect(modelCalls()).toBe(1)
  })

  it.each([false, true])("用例 10 当前行为：导航同一步附带文字=%s，非空文本完成 Run", async (withText) => {
    const owner = "usr_baseline_navigate"
    const { repository, runId } = await queuedRun(owner, "baseline-navigate")
    const { provider, modelCalls } = scriptedProvider([
      { ...(withText ? { text: "已生成打开控制台的入口。" } : {}), toolCalls: [{ id: "nav-1", operationId: "navigate_to_route", arguments: { routeName: "dashboard" } }] },
      { text: "已生成打开控制台的入口。" },
    ])

    await testExecutor(repository, new ModelRuntime(provider, testRegistry())).runOnce()

    expect((await repository.getRun(owner, runId))?.status).toBe("completed")
    expect(modelCalls()).toBe(withText ? 1 : 2)
  })

  it("用例 18 当前行为：Run 超时把 Run 置为 failed 并记录 ai.run_timeout", async () => {
    vi.useFakeTimers()
    const owner = "usr_baseline_timeout"
    const { repository, runId } = await queuedRun(owner, "baseline-run-timeout")
    const executor = testExecutor(repository, new ModelRuntime(hangingProvider(), testRegistry()), {
      runtimeConfig: testRemoteConfigSource(testRemoteProviderConfig({ runtime: { runTimeoutMs: 30_000 } })).source,
    })

    const running = executor.runOnce()
    await vi.advanceTimersByTimeAsync(30_000)
    await running

    const run = await repository.getRun(owner, runId)
    expect(run?.status).toBe("failed")
    expect(run?.errorCode).toBe("ai.run_timeout")
  })

  it("异步场景当前行为：平台受理提交即视为工具成功，没有外部操作状态与权威回读", async () => {
    const owner = "usr_baseline_async_submit"
    const { repository, conversation, runId } = await queuedRun(owner, "baseline-async-submit", "帮我构建应用 app_1")
    const catalog = ToolCatalog.load([testToolOperation("createBuild", {
      name: "创建构建",
      summary: "提交应用构建。",
      method: "POST",
      path: "/api/v1/builds",
      requestBody: true,
      inputSchema: {
        type: "object",
        properties: { applicationId: { type: "string" } },
        required: ["applicationId"],
        additionalProperties: false,
      },
    })])
    const { client, store, tools } = platformTool(repository, catalog, () => ({
      status: 202,
      body: { operationId: "bld_1", status: "queued" },
    }))
    const { provider } = scriptedProvider([
      { toolCalls: [{ id: "build-1", operationId: "createBuild", arguments: { applicationId: "app_1" } }] },
      { text: "构建已提交，正在排队。" },
    ])
    const executor = testExecutor(
      repository,
      new ModelRuntime(provider, testRegistry({ resolve: () => catalog.modelTools(["createBuild"]) })),
      { catalog, tools },
    )

    await executor.runOnce()

    expect((await repository.getRun(owner, runId))?.status).toBe("completed")
    expect(client.calls).toHaveLength(1)
    expect([...store.records.values()][0]).toMatchObject({ status: "succeeded" })
    const toolResult = (await timelineItems(repository, owner, conversation.id)).find(item => item.type === "tool_result")
    expect(toolResult?.content).toMatchObject({ result: { operationId: "bld_1", status: "queued" } })
  })

  it.each(["模型调用前", "工具完成后", "终态提交前"])("用例 15 当前行为：%s 丢弃进程状态后重启置为 interrupted", async (boundary) => {
    vi.useFakeTimers()
    const owner = "usr_baseline_restart"
    const { repository, conversation, runId } = await queuedRun(owner, "baseline-restart")
    const claimed = await repository.claimNextQueuedRun()
    if (!claimed) throw new Error("missing claimed run")
    const turnId = (await repository.getTimeline(owner, conversation.id))!.turns[0]!.id
    if (boundary === "工具完成后") {
      const toolCallItem = await repository.appendItem({
        runId,
        turnId,
        type: "tool_call",
        status: "completed",
        content: { toolCallId: "aitool_restart", operationId: "getProject", status: "succeeded", arguments: {}, result: { project: { id: "prj_1" } } },
      })
      await repository.appendItem({
        runId,
        turnId,
        type: "tool_result",
        status: "completed",
        content: { relatedItemId: toolCallItem.id, result: { project: { id: "prj_1" } } },
      })
    }
    if (boundary === "终态提交前") {
      const oldBus = new InMemoryRunStreamBus(repository)
      const session = await oldBus.open(runId, turnId, claimed.rowVersion)
      await session.appendItemWithEvent({ id: "uncommitted", runId, turnId, type: "assistant_message", status: "completed", content: { parts: [{ type: "text", text: "尚未提交。" }] } }, "message.completed")
      expect(await oldBus.read(runId, 0)).toHaveLength(1)
    }
    const { provider, modelCalls } = scriptedProvider([{ text: "重启后不应继续执行。" }])
    const executor = testExecutor(repository, new ModelRuntime(provider, testRegistry()))

    executor.start()
    await vi.advanceTimersByTimeAsync(0)
    await executor.stop()

    const run = await repository.getRun(owner, runId)
    expect(run?.status).toBe("interrupted")
    expect(run?.errorCode).toBe("ai.agent_restarted")
    expect(await repository.claimNextQueuedRun()).toBeUndefined()
    expect(modelCalls()).toBe(0)
    expect((await repository.getEvents(owner, runId, 0)).at(-1)?.type).toBe("run.interrupted")
    expect((await repository.getExecutionInput(runId))?.toolInteractions.map(item => item.type)).toEqual(boundary === "工具完成后" ? ["tool_call", "tool_result"] : [])
    expect(assistantTexts(await timelineItems(repository, owner, conversation.id))).toEqual([])
  })
})
