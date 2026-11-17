import { describe, expect, it } from "vitest"
import { DevelopmentRequestVerifier, type RequestVerifier } from "../src/auth.js"
import { loadConfig } from "../src/config.js"
import { ModelRuntime } from "../src/model-runtime.js"
import { createTurnRequestHash } from "../src/persistence/create-turn-hash.js"
import { DeterministicProvider } from "../src/provider/deterministic.js"
import type { ModelMessage, ModelProvider, ModelRequest, ModelToolCall } from "../src/provider/provider.js"
import { defaultRuntimeSettings } from "../src/runtime-settings.js"
import { presentTimeline } from "../src/timeline-presenter.js"
import { ToolCatalog } from "../src/tools/catalog.js"
import { DeterministicLunaApiClient } from "../src/tools/luna-api-client.js"
import { ProjectingToolCallStore, ToolOrchestrator } from "../src/tools/orchestrator.js"
import { testRegistry } from "./support/model-tool-registry.js"
import { testRemoteConfigSource, testRemoteProviderConfig } from "./support/remote-config.js"
import { testExecutor } from "./support/run-executor.js"
import { buildTestServer } from "./support/server.js"
import { TestRepository } from "./support/test-repository.js"
import { testToolOperation } from "./support/tool-catalog.js"

const reportedUsage = {
  status: "reported" as const,
  value: { inputTokens: 12, outputTokens: 8, totalTokens: 20 },
}

class RunScopedVerifier implements RequestVerifier {
  private readonly inner = new DevelopmentRequestVerifier()
  async verify(headers: Record<string, string | string[] | undefined>) {
    const actor = await this.inner.verify(headers)
    const runId = Array.isArray(headers["x-luna-dev-run"]) ? headers["x-luna-dev-run"][0] : headers["x-luna-dev-run"]
    return runId ? { ...actor, runId } : actor
  }
}

function scriptedProvider(
  steps: Array<{
    text?: string
    toolCalls?: ModelToolCall[]
    finishReason?: string
  }>,
) {
  const requests: ModelRequest[] = []
  let callIndex = 0
  const provider: ModelProvider = {
    async *stream(request) {
      requests.push(request)
      const step = steps[Math.min(callIndex, steps.length - 1)] ?? {}
      callIndex += 1
      if (step.text) yield { type: "message_delta", delta: step.text }
      if (step.toolCalls?.length) yield { type: "tool_call_delta" }
      yield {
        type: "completed",
        usage: reportedUsage,
        ...(step.toolCalls?.length ? { toolCalls: step.toolCalls } : {}),
        finishReason: step.finishReason ?? (step.toolCalls?.length ? "tool_calls" : "stop"),
      }
    },
    async complete(request) {
      requests.push(request)
      return { text: "会话标题", toolCalls: [], usage: reportedUsage }
    },
    capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
    health: async () => ({ ok: true }),
  }
  return { provider, requests, modelCalls: () => callIndex }
}

function toolMessagesFromRequest(messages: ModelMessage[]) {
  return messages
    .filter((msg): msg is Extract<ModelMessage, { role: "tool" }> => msg.role === "tool")
    .map((msg) => {
      const newlineIndex = msg.content.indexOf("\n")
      const jsonText = newlineIndex >= 0 ? msg.content.slice(newlineIndex + 1) : msg.content
      return {
        toolCallId: msg.toolCallId,
        payload: JSON.parse(jsonText) as Record<string, unknown>,
      }
    })
}

describe("Step 3 基础架构改造回归验证", () => {
  it("1.2.1 & 1.2.2：cardRepairExhausted 仅阻断继续调用卡片工具，不误杀非卡片工具并跨 Checkpoint 持久化", async () => {
    const owner = "usr_card_exhausted_non_card"
    const repository = new TestRepository()
    const conversation = await repository.createConversation(owner, "初始会话", undefined, "default")
    const turn = await repository.createTurn(owner, {
      conversationId: conversation.id,
      input: "查看项目并更新标题",
      pageContext: {},
      idempotencyKey: "card-exhausted-non-card",
    })
    const catalog = ToolCatalog.load([testToolOperation("getProject")])
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: { id: "prj_1", name: "核心业务" } }))
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))

    const { provider, requests } = scriptedProvider([
      // 第 1 次非法卡片 -> attempt 1 (retryable: true)
      { toolCalls: [{ id: "c1", operationId: "present_card", arguments: { title: "缺少必填字段" } }] },
      // 第 2 次非法卡片 -> attempt 2 === maxCardRepairAttempts (cardRepairExhausted = true)
      { toolCalls: [{ id: "c2", operationId: "present_card", arguments: { title: "依然非法" } }] },
      // 卡片修复耗尽后，模型继续调用非卡片工具（rename_conversation、navigate_to_route、getProject）及再次尝试 present_card
      {
        toolCalls: [
          { id: "c3", operationId: "present_card", arguments: { title: "第三次尝试" } },
          { id: "r1", operationId: "rename_conversation", arguments: { title: "项目排查结果" } },
          { id: "n1", operationId: "navigate_to_route", arguments: { routeName: "dashboard" } },
          { id: "p1", operationId: "getProject", arguments: {} },
        ],
      },
      { text: "卡片已降级为纯文本：核心业务项目运行正常。" },
    ])

    const executor = testExecutor(
      repository,
      new ModelRuntime(provider, testRegistry({ resolve: () => catalog.modelTools(["getProject"]) })),
      { catalog, tools },
    )
    await executor.runOnce()

    const run = await repository.getRun(owner, turn.run.id)
    expect(run?.status).toBe("completed")
    expect(client.calls).toHaveLength(1)
    expect((await repository.getConversation(owner, conversation.id))?.title).toBe("项目排查结果")

    const checkpoint = await repository.loadRunCheckpoint(turn.run.id)
    expect(checkpoint?.state.cardRepairExhausted).toBe(true)

    const finalStepToolResults = toolMessagesFromRequest(requests[3]!.messages)
    expect(finalStepToolResults.find(item => item.toolCallId === "c3")?.payload).toMatchObject({
      status: "failed",
      errorCode: "ai.interaction_card_schema_invalid",
      retryable: false,
    })
    expect(finalStepToolResults.find(item => item.toolCallId === "p1")?.payload).toMatchObject({
      status: "succeeded",
    })
  })

  it("1.2.3 & 1.2.4：同批次混合 interactive + presentation 保持暂停语义；与平台工具同批返回明确中文 guidance", async () => {
    const owner = "usr_mixed_cards"
    const repository = new TestRepository()
    const conversation = await repository.createConversation(owner, "混合卡片")
    const firstTurn = await repository.createTurn(owner, {
      conversationId: conversation.id,
      input: "先选环境并看摘要",
      pageContext: {},
      idempotencyKey: "mixed-cards-pause",
    })

    const mixedBatch = scriptedProvider([
      {
        toolCalls: [
          {
            id: "choice_1",
            operationId: "request_choice",
            arguments: {
              title: "选择环境",
              cards: [
                { id: "prod", presentation: { title: "生产环境" }, actions: [{ id: "pick_prod", type: "send_message", label: "选生产", message: "使用生产环境" }] },
              ],
            },
          },
          {
            id: "present_1",
            operationId: "present_card",
            arguments: {
              title: "当前摘要",
              template: "result",
              cards: [{ id: "summary", presentation: { variant: "summary", title: "摘要" }, blocks: [{ id: "m1", type: "markdown", content: "请选择目标环境。" }] }],
            },
          },
        ],
      },
      { text: "如果未暂停就会错误走到第二步" },
    ])
    await testExecutor(repository, new ModelRuntime(mixedBatch.provider, testRegistry())).runOnce()
    expect((await repository.getRun(owner, firstTurn.run.id))?.status).toBe("completed")
    // 必须保留 interactive 暂停语义（在第 1 步直接结束等待用户选择，不进入第 2 步模型调用）
    expect(mixedBatch.modelCalls()).toBe(1)

    // 同批含平台工具时，卡片工具返回 deferred_until_platform_results 与清晰中文 guidance
    await repository.createTurn(owner, {
      conversationId: conversation.id,
      input: "查询并展示",
      pageContext: {},
      idempotencyKey: "deferred-card-guidance",
    })
    const catalog = ToolCatalog.load([testToolOperation("getProject")])
    const tools = new ToolOrchestrator(
      catalog,
      new DeterministicLunaApiClient(() => ({ status: 200, body: { id: "prj_1" } })),
      new ProjectingToolCallStore(repository.toolCallStore, repository),
    )
    const deferred = scriptedProvider([
      {
        toolCalls: [
          { id: "p_call", operationId: "getProject", arguments: {} },
          {
            id: "c_defer",
            operationId: "present_card",
            arguments: {
              title: "延后卡片",
              template: "result",
              cards: [{ id: "r1", presentation: { variant: "summary", title: "结果" } }],
            },
          },
        ],
      },
      { text: "查询完成。" },
    ])
    await testExecutor(
      repository,
      new ModelRuntime(deferred.provider, testRegistry({ resolve: () => catalog.modelTools(["getProject"]) })),
      { catalog, tools },
    ).runOnce()

    const deferredMsg = toolMessagesFromRequest(deferred.requests[1]!.messages).find(item => item.toolCallId === "c_defer")
    expect(deferredMsg?.payload.status).toBe("deferred_until_platform_results")
    expect(typeof deferredMsg?.payload.guidance === "string" ? deferredMsg.payload.guidance : "").toContain("当前批次包含待执行或待审批的平台工具，交互卡片已延后")
  })

  it("1.2.5 & 1.2.6：未完成 tool_call 流式占位项强制收敛为 failed；内部工具 .safeParse 返回可恢复结构化错误", async () => {
    const owner = "usr_internal_safeparse"
    const repository = new TestRepository()
    const conversation = await repository.createConversation(owner, "内部工具容错")
    const turn = await repository.createTurn(owner, {
      conversationId: conversation.id,
      input: "测试内部工具容错",
      pageContext: {},
      idempotencyKey: "internal-safeparse",
    })

    // 模拟残留 streaming 状态的 tool_call 与 assistant_message
    const streamingMsg = await repository.appendItem({
      runId: turn.run.id,
      turnId: turn.turn.id,
      type: "assistant_message",
      status: "streaming",
      content: { parts: [{ type: "text", text: "部分输出" }] },
    })
    const streamingTool = await repository.appendItem({
      runId: turn.run.id,
      turnId: turn.turn.id,
      type: "tool_call",
      status: "streaming",
      content: { toolCallId: "dangling_tool", operationId: "present_card", status: "running" },
    })
    await repository.finalizeStreamingItems(turn.run.id, "completed")
    const finalizedItems = (await repository.getTimeline(owner, conversation.id))?.turns[0]?.items ?? []
    expect(finalizedItems.find(item => item.id === streamingMsg.id)?.status).toBe("completed")
    const finalizedToolItem = finalizedItems.find(item => item.id === streamingTool.id)
    expect(finalizedToolItem?.status).toBe("failed")
    expect(finalizedToolItem?.content.status).toBe("failed")
    await repository.updateRun(turn.run.id, "queued", "completed", { completedAt: new Date().toISOString() })

    // 验证 navigate_to_route 与 rename_conversation 收到非法参数时不炸毁 Run
    const secondTurn = await repository.createTurn(owner, {
      conversationId: conversation.id,
      input: "传错内部工具参数",
      pageContext: {},
      idempotencyKey: "internal-safeparse-2",
    })
    const { provider, requests } = scriptedProvider([
      {
        toolCalls: [
          { id: "bad_nav", operationId: "navigate_to_route", arguments: { routeName: "nonexistent.route" } },
          { id: "bad_rename", operationId: "rename_conversation", arguments: { title: "" } },
        ],
      },
      { text: "已忽略非法内部参数并正常回复。" },
    ])
    await testExecutor(repository, new ModelRuntime(provider, testRegistry())).runOnce()
    expect((await repository.getRun(owner, secondTurn.run.id))?.status).toBe("completed")
    const recoveredResults = toolMessagesFromRequest(requests[1]!.messages)
    expect(recoveredResults.find(item => item.toolCallId === "bad_nav")?.payload).toMatchObject({
      status: "failed",
      errorCode: "ai.tool_arguments_invalid",
      retryable: true,
    })
    expect(recoveredResults.find(item => item.toolCallId === "bad_rename")?.payload).toMatchObject({
      status: "failed",
      errorCode: "ai.tool_arguments_invalid",
      retryable: true,
    })
  })

  it("2.2：createTurnRequestHash 剔除 BFF 动态注入字段保持幂等一致，并发相同幂等键不抛 500", async () => {
    const firstHash = createTurnRequestHash({
      conversationId: "aicnv_1",
      input: "部署后端服务",
      modelId: "aimod_pro",
      pageContext: {
        schemaVersion: 1,
        routeName: "application.detail",
        projectId: "prj_1",
        locale: "zh-CN",
        client: { timestamp: "2026-11-17T10:00:00.000Z", utcOffsetMinutes: 480, locale: "zh-CN" },
        server: { requestTimestamp: "2026-11-17T10:00:00Z", locale: "zh-CN", projectContextPresent: true },
      },
      idempotencyKey: "idem_same_1",
      preallocatedRunId: "airun_attempt_1",
      actorSessionId: "sess_1",
      toolCatalogDigest: "sha256:v1",
      traceContext: { traceparent: "00-11111111111111111111111111111111-1111111111111111-01" },
      modelSnapshot: {
        id: "aimod_pro",
        name: "Pro v1",
        maxContextTokens: 128000,
        maxOutputTokens: 8192,
        inputCreditsPerMillion: "1.0",
        outputCreditsPerMillion: "2.0",
        cachedInputCreditsPerMillion: "0.2",
      },
    })
    const retryHash = createTurnRequestHash({
      conversationId: "aicnv_1",
      input: "部署后端服务",
      modelId: "aimod_pro",
      pageContext: {
        projectId: "prj_1",
        routeName: "application.detail",
        schemaVersion: 1,
        locale: "en-US",
        server: { requestTimestamp: "2026-11-17T10:00:09Z", locale: "en-US", projectContextPresent: true },
      },
      idempotencyKey: "idem_same_1",
      preallocatedRunId: "airun_attempt_2",
      actorSessionId: "sess_2",
      toolCatalogDigest: "sha256:v2",
      traceContext: { traceparent: "00-22222222222222222222222222222222-2222222222222222-01" },
      modelSnapshot: {
        id: "aimod_pro",
        name: "Pro Renamed",
        maxContextTokens: 256000,
        maxOutputTokens: 16384,
        inputCreditsPerMillion: "1.5",
        outputCreditsPerMillion: "3.0",
        cachedInputCreditsPerMillion: "0.3",
      },
    })
    expect(retryHash).toBe(firstHash)

    const repository = new TestRepository()
    const owner = "usr_idem_concurrent"
    const conversation = await repository.createConversation(owner, "并发幂等")
    const [firstCreated, secondCreated] = await Promise.all([
      repository.createTurn(owner, {
        conversationId: conversation.id,
        input: "同一意图并发提交",
        pageContext: { routeName: "dashboard", server: { requestTimestamp: "t1" } },
        idempotencyKey: "concurrent-idem-key",
        preallocatedRunId: "airun_conc_1",
        actorSessionId: "s1",
      }),
      repository.createTurn(owner, {
        conversationId: conversation.id,
        input: "同一意图并发提交",
        pageContext: { routeName: "dashboard", server: { requestTimestamp: "t2" } },
        idempotencyKey: "concurrent-idem-key",
        preallocatedRunId: "airun_conc_2",
        actorSessionId: "s2",
      }),
    ])
    expect(secondCreated.turn.id).toBe(firstCreated.turn.id)
    expect(secondCreated.run.id).toBe(firstCreated.run.id)

    await expect(repository.createTurn(owner, {
      conversationId: conversation.id,
      input: "篡改后的不同意图",
      pageContext: { routeName: "dashboard" },
      idempotencyKey: "concurrent-idem-key",
    })).rejects.toThrow("idempotency_conflict")
  })

  it("2.3.3：Direct Tool Action 被用户拒绝（rejected）时优雅收敛为 completed 且时间线保留 rejected 状态", async () => {
    const owner = "usr_direct_reject"
    const repository = new TestRepository()
    const conversation = await repository.createConversation(owner, "直接工具拒绝")
    const catalog = ToolCatalog.load([
      {
        ...testToolOperation("deleteApplication"),
        method: "DELETE",
        requiresApproval: true,
        effect: "write",
        verification: [{ operationId: "getApplication", family: "application", expect: "absent" }],
      },
      testToolOperation("getApplication"),
    ])
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: {} }))
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    const remote = testRemoteConfigSource(testRemoteProviderConfig({ toolCatalog: catalog.all() }))
    const executor = testExecutor(
      repository,
      new ModelRuntime(new DeterministicProvider(), testRegistry()),
      { catalog, tools, runtimeConfig: remote.source },
    )
    const app = buildTestServer({
      config: loadConfig({ NODE_ENV: "test" }),
      repository,
      requestVerifier: new RunScopedVerifier(),
      tools,
      remoteConfig: remote.source,
      toolCatalogDigest: () => catalog.digest,
    })

    const runId = "airun_direct_reject_1"
    const actionRes = await app.inject({
      method: "POST",
      url: `/internal/v1/conversations/${conversation.id}/tool-actions`,
      headers: {
        "x-luna-dev-user": owner,
        "x-luna-dev-run": runId,
        "idempotency-key": "direct-reject-action-1",
      },
      payload: {
        operationId: "deleteApplication",
        arguments: {},
        message: "删除应用",
        runId,
      },
    })
    expect(actionRes.statusCode).toBe(202)
    const createdBody = actionRes.json<{ state: string, toolCallId: string }>()
    expect(createdBody.state).toBe("waiting_approval")
    expect(client.calls).toHaveLength(0)

    const rejectRes = await app.inject({
      method: "POST",
      url: `/internal/v1/runs/${runId}/approvals/${createdBody.toolCallId}/decision`,
      headers: { "x-luna-dev-user": owner },
      payload: { decision: "reject" },
    })
    expect(rejectRes.statusCode).toBe(202)
    expect(rejectRes.json()).toEqual({ runId, state: "queued" })

    await executor.runOnce()

    const finalRun = await repository.getRun(owner, runId)
    expect(finalRun?.status).toBe("completed")
    expect(finalRun?.errorCode).toBeUndefined()
    expect(client.calls).toHaveLength(0)

    const timeline = await presentTimeline(repository, owner, conversation.id)
    const toolItem = timeline?.turns[0]?.selectedRun?.items.find(item => item.type === "tool_call") as {
      status?: string
      toolCall?: { id?: string, status?: string }
    } | undefined
    expect(toolItem?.status).toBe("completed")
    expect(toolItem?.toolCall).toMatchObject({
      id: createdBody.toolCallId,
      status: "rejected",
    })
    await app.close()
  })

  it("3.1.2 & 3.2.2 & 3.2.3：跨 Turn 继承 selectedOperationIds、保护已执行工具免于淘汰、同工具 >=5 次软预警与 maxModelSteps 优雅收尾步", async () => {
    const owner = "usr_step3_combo"
    const repository = new TestRepository()
    const conversation = await repository.createConversation(owner, "跨轮工具继承")

    // 构造 20 个可检索只读工具
    const operations = Array.from({ length: 20 }, (_, idx) => testToolOperation(`readTool${String(idx + 1).padStart(2, "0")}`))
    const catalog = ToolCatalog.load(operations)
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: { items: [] } }))
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))

    // Turn 1：加载并实际执行 readTool01，验证满槽淘汰时已执行的 readTool01 不会被挤出
    const turn1 = await repository.createTurn(owner, {
      conversationId: conversation.id,
      input: "第一轮执行工具",
      pageContext: {},
      idempotencyKey: "combo-turn-1",
    })
    await repository.touchRunSelectedOperations(turn1.run.id, ["readTool01", "readTool02", "readTool03"], 4)
    await tools.propose({ runId: turn1.run.id, operationId: "readTool01", arguments: {} })
    const afterOverflow = await repository.touchRunSelectedOperations(
      turn1.run.id,
      ["readTool04", "readTool05", "readTool06"],
      4,
    )
    expect(afterOverflow.selectedOperationIds).toContain("readTool01")
    expect(afterOverflow.evictedOperationIds).not.toContain("readTool01")
    expect(afterOverflow.evictedOperationIds).toEqual(["readTool02", "readTool03"])
    await repository.updateRun(turn1.run.id, "queued", "completed", { completedAt: new Date().toISOString() })

    // Turn 2：新建 Turn 自动继承上一轮有效 Run 的 selectedOperationIds
    const turn2 = await repository.createTurn(owner, {
      conversationId: conversation.id,
      input: "第二轮连续探索并触发软预警与优雅收尾",
      pageContext: {},
      idempotencyKey: "combo-turn-2",
    })
    expect(turn2.run.selectedOperationIds).toEqual(afterOverflow.selectedOperationIds)

    // 在 Turn 2 中设置 maxModelSteps = 6：前 5 步以不同参数连续调用 search_tools（第 5 次触发软预警），第 6 步触发无工具优雅收尾总结步
    const requests: ModelRequest[] = []
    let stepIndex = 0
    const provider: ModelProvider = {
      async *stream(request) {
        requests.push(request)
        stepIndex += 1
        if ((request.tools?.length ?? 0) === 0) {
          yield { type: "message_delta", delta: "总结：已完成 5 次目录探索，尚未修改任何资源，建议明确目标应用名称后继续。" }
          yield { type: "completed", usage: reportedUsage, finishReason: "stop" }
          return
        }
        yield { type: "tool_call_delta" }
        yield {
          type: "completed",
          usage: reportedUsage,
          toolCalls: [{
            id: `spin_${stepIndex}`,
            operationId: "search_tools",
            arguments: { query: `variant-${stepIndex}` },
          }],
        }
      },
      async complete() { return { text: "总结标题", toolCalls: [], usage: reportedUsage } },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }

    const executor = testExecutor(
      repository,
      new ModelRuntime(provider, testRegistry({
        resolve: (_ctx, _input, loaded) => catalog.modelTools(loaded),
        search: input => ({
          ...catalog.search(input),
          loadedOperationIds: [],
          missingOperationIds: [],
          catalogDigest: catalog.digest,
          duplicate: false,
          cacheHit: false,
        }),
      })),
      {
        catalog,
        tools,
        initialRuntimeSettings: {
          ...defaultRuntimeSettings,
          maxModelSteps: 6,
        },
      },
    )
    await executor.runOnce()

    const finishedRun = await repository.getRun(owner, turn2.run.id)
    expect(finishedRun?.status).toBe("completed")
    expect(requests).toHaveLength(6)
    // 首步模型请求已直接继承上一轮的平台工具定义
    expect(requests[0]!.tools?.map(tool => tool.operationId)).toContain("readTool01")
    // 最后一步（优雅收尾总结步）不挂载任何工具（tools: []）且注入收尾提示词
    expect(requests[5]!.tools).toEqual([])
    const finalPromptMessages = requests[5]!.messages
    expect(finalPromptMessages.at(-1)?.role).toBe("user")
    expect(finalPromptMessages.at(-1)?.content).toContain("已达到单轮工具探索步数上限，本步不再提供任何工具")
    // 第 5 次调用 search_tools 的返回结果注入变参空转软预警
    const fifthSearchResult = toolMessagesFromRequest(finalPromptMessages).find(item => item.toolCallId === "spin_5")
    expect(fifthSearchResult?.payload.guidance).toContain("你已在当前回合多次调用 search_tools，请停止盲目变换参数重试")
  })
})
