import { describe, expect, it, vi } from "vitest"
import { DevelopmentRequestVerifier, type RequestVerifier } from "../src/auth.js"
import { loadConfig } from "../src/config.js"
import { ModelRuntime } from "../src/model-runtime.js"
import { createTurnRequestHash } from "../src/persistence/create-turn-hash.js"
import { DeterministicProvider } from "../src/provider/deterministic.js"
import { OpenAIChatCompletionsProvider } from "../src/provider/openai-chat-completions.js"
import type { ModelMessage, ModelProvider, ModelRequest, ModelToolCall } from "../src/provider/provider.js"
import { defaultRuntimeSettings } from "../src/runtime-settings.js"
import { presentTimeline } from "../src/timeline-presenter.js"
import { businessCardToolInputs, businessCardTools } from "../src/tools/business-card-tools.js"
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

    // 在 Turn 2 中设置 maxModelSteps = 6：前 5 步以不同参数连续调用 search_tools（第 5 次触发软预警），第 6 步触发 toolChoice: "none" 优雅收尾总结步
    const requests: ModelRequest[] = []
    let stepIndex = 0
    const provider: ModelProvider = {
      async *stream(request) {
        requests.push(request)
        stepIndex += 1
        if (request.toolChoice === "none") {
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
    // 最后一步（优雅收尾总结步）保留完整已排序 tools 数组（保住 KV Cache）并显式传递 toolChoice: "none"
    expect(requests[5]!.tools).toEqual(requests[4]!.tools)
    expect(requests[5]!.toolChoice).toBe("none")
    const finalPromptMessages = requests[5]!.messages
    expect(finalPromptMessages.at(-1)?.role).toBe("user")
    expect(finalPromptMessages.at(-1)?.content).toContain("已达到单轮工具探索步数上限，本步不再提供任何工具")
    // 第 5 次调用 search_tools 的返回结果注入变参空转软预警
    const fifthSearchResult = toolMessagesFromRequest(finalPromptMessages).find(item => item.toolCallId === "spin_5")
    expect(fifthSearchResult?.payload.guidance).toContain("你已在当前回合多次调用 search_tools，请停止盲目变换参数重试")
  })

  it("P0-2 真实 Provider 请求体测试：优雅收尾步发送非空 tools + tool_choice: none，且违规夹带 tool_calls 时仍落库为 assistant_message 并进入下一轮 history", async () => {
    const owner = "usr_p0_2_real_provider"
    const repository = new TestRepository()
    const conversation = await repository.createConversation(owner, "已手动锁定的标题", undefined, "user")
    const turn1 = await repository.createTurn(owner, {
      conversationId: conversation.id,
      input: "检查项目状态并总结",
      pageContext: {},
      idempotencyKey: "p0-2-turn-1",
    })
    const catalog = ToolCatalog.load([testToolOperation("getProject")])
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: { id: "prj_1", status: "healthy" } }))
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))

    const capturedBodies: Array<Record<string, unknown>> = []
    let fetchAttempt = 0
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockImplementation(async (_url, init) => {
      fetchAttempt += 1
      const parsedBody = JSON.parse(typeof init?.body === "string" ? init.body : "{}") as Record<string, unknown>
      capturedBodies.push(parsedBody)
      const encoder = new TextEncoder()
      if (fetchAttempt === 1) {
        const ssePayload = [
          `data: ${JSON.stringify({ id: "chatcmpl_1", model: "gpt-test", choices: [{ delta: { tool_calls: [{ index: 0, id: "call_step_1", type: "function", function: { name: "getProject", arguments: "{}" } }] }, finish_reason: null }] })}\n\n`,
          `data: ${JSON.stringify({ id: "chatcmpl_1", model: "gpt-test", choices: [{ delta: {}, finish_reason: "tool_calls" }], usage: { prompt_tokens: 100, completion_tokens: 15, total_tokens: 115 } })}\n\n`,
          "data: [DONE]\n\n",
        ].join("")
        return new Response(new ReadableStream({
          start(controller) {
            controller.enqueue(encoder.encode(ssePayload))
            controller.close()
          },
        }), { status: 200, headers: { "content-type": "text/event-stream" } })
      }
      // 第 2 步（maxModelSteps = 2 的优雅收尾步）：模拟不规范网关在 tool_choice: "none" 下依然违规夹带 tool_calls delta 与 finish_reason: "tool_calls"
      const summaryText = "总结：已确认项目 prj_1 运行健康，尚未做任何变更，建议按需查看部署详情。"
      const sseSummaryPayload = [
        `data: ${JSON.stringify({ id: "chatcmpl_2", model: "gpt-test", choices: [{ delta: { content: summaryText, tool_calls: [{ index: 0, id: "hallucinated_call", type: "function", function: { name: "getProject", arguments: "{}" } }] }, finish_reason: null }] })}\n\n`,
        `data: ${JSON.stringify({ id: "chatcmpl_2", model: "gpt-test", choices: [{ delta: {}, finish_reason: "tool_calls" }], usage: { prompt_tokens: 140, completion_tokens: 28, total_tokens: 168 } })}\n\n`,
        "data: [DONE]\n\n",
      ].join("")
      return new Response(new ReadableStream({
        start(controller) {
          controller.enqueue(encoder.encode(sseSummaryPayload))
          controller.close()
        },
      }), { status: 200, headers: { "content-type": "text/event-stream" } })
    })

    try {
      const realProvider = new OpenAIChatCompletionsProvider({
        baseUrl: "https://provider.example.test/v1",
        apiKey: "sk-test",
        channelAffinityEnabled: true,
        promptCacheKeyEnabled: true,
        model: "gpt-test",
        timeoutMs: 10_000,
      })
      const executor = testExecutor(
        repository,
        new ModelRuntime(realProvider, testRegistry({ resolve: () => catalog.modelTools(["getProject"]) })),
        {
          catalog,
          tools,
          initialRuntimeSettings: {
            ...defaultRuntimeSettings,
            maxModelSteps: 2,
          },
        },
      )
      await executor.runOnce()

      expect(capturedBodies).toHaveLength(2)
      const step1Tools = capturedBodies[0]!.tools as unknown[]
      const step2Tools = capturedBodies[1]!.tools as unknown[]
      expect(Array.isArray(step2Tools) && step2Tools.length > 0).toBe(true)
      expect(step2Tools).toEqual(step1Tools)
      expect(capturedBodies[1]!.tool_choice).toBe("none")

      const run1 = await repository.getRun(owner, turn1.run.id)
      expect(run1?.status).toBe("completed")

      const turn1Items = (await repository.getTimeline(owner, conversation.id))?.turns[0]?.items ?? []
      const assistantItems = turn1Items.filter(item => item.type === "assistant_message")
      expect(assistantItems).toHaveLength(1)
      expect(assistantItems[0]?.status).toBe("completed")

      // 创建下一轮并核对 history 能完整读取到该总结正文（不丢失为 progress）
      const turn2 = await repository.createTurn(owner, {
        conversationId: conversation.id,
        input: "继续下一步",
        pageContext: {},
        idempotencyKey: "p0-2-turn-2",
      })
      const execInput2 = await repository.getExecutionInput(turn2.run.id)
      expect(execInput2?.history).toHaveLength(1)
      expect(execInput2?.history[0]?.assistant).toBe("总结：已确认项目 prj_1 运行健康，尚未做任何变更，建议按需查看部署详情。")
    }
    finally {
      fetchSpy.mockRestore()
    }
  })

  it("P0-2 步数边界测试：第 maxModelSteps-2 步空响应 retry 且第 maxModelSteps-1 步输出工具调用时，单轮总调用次数绝不超过 maxModelSteps", async () => {
    const owner = "usr_p0_2_step_cap"
    const repository = new TestRepository()
    const conversation = await repository.createConversation(owner, "步数上限测试", undefined, "user")
    const turn = await repository.createTurn(owner, {
      conversationId: conversation.id,
      input: "测试步数边界",
      pageContext: {},
      idempotencyKey: "p0-2-step-cap",
    })
    const catalog = ToolCatalog.load([testToolOperation("getProject")])
    const tools = new ToolOrchestrator(
      catalog,
      new DeterministicLunaApiClient(() => ({ status: 200, body: { id: "prj_1" } })),
      new ProjectingToolCallStore(repository.toolCallStore, repository),
    )
    const { provider, modelCalls } = scriptedProvider([
      { toolCalls: [{ id: "s0", operationId: "getProject", arguments: {} }] },
      { toolCalls: [{ id: "s1", operationId: "search_tools", arguments: { query: "deploy" } }] },
      // step 2 (maxModelSteps - 2): empty response -> retry (finalResponseMissing = true)
      {},
      // step 3 (maxModelSteps - 1): outputs tool call again
      { toolCalls: [{ id: "s3", operationId: "search_tools", arguments: { query: "cluster" } }] },
      { text: "不应触发第 5 次调用" },
    ])
    const executor = testExecutor(
      repository,
      new ModelRuntime(provider, testRegistry({ resolve: () => catalog.modelTools(["getProject"]) })),
      {
        catalog,
        tools,
        initialRuntimeSettings: {
          ...defaultRuntimeSettings,
          maxModelSteps: 4,
        },
      },
    )
    await executor.runOnce()

    expect(modelCalls()).toBe(4)
    const finalRun = await repository.getRun(owner, turn.run.id)
    expect(finalRun?.status).toBe("failed")
    expect(finalRun?.errorCode).toBe("ai.limit_exceeded")
  })

  it("P0-1 & P1-1：后台 RunExecutor 并发轮询下 /tool-actions 同幂等键并发提交零抢占零冲突，且审批分裂故障重试可恢复推进", async () => {
    const owner = "usr_p0_1_concurrent"
    const repository = new TestRepository()
    const conversation = await repository.createConversation(owner, "并发工具动作", undefined, "user")
    const catalog = ToolCatalog.load([
      testToolOperation("getProject"),
      {
        ...testToolOperation("deleteApplication"),
        method: "DELETE",
        requiresApproval: true,
        effect: "write",
        verification: [{ operationId: "getProject", family: "resource", expect: "absent" }],
      },
    ])
    const client = new DeterministicLunaApiClient(call => call.operation.operationId === "deleteApplication"
      ? { status: 200, body: { deleted: true } }
      : { status: 200, body: { id: "prj_1" } })
    // 对 deleteApplication 返回 absent（404）以满足删除后校验
    let deleteExecuted = false
    const dynamicClient = new DeterministicLunaApiClient((call) => {
      if (call.operation.operationId === "deleteApplication") {
        deleteExecuted = true
        return { status: 200, body: { deleted: true } }
      }
      return deleteExecuted ? { status: 404, body: { code: "resource.not_found" } } : { status: 200, body: { id: "prj_1" } }
    })
    void client
    const tools = new ToolOrchestrator(catalog, dynamicClient, new ProjectingToolCallStore(repository.toolCallStore, repository))
    const remote = testRemoteConfigSource(testRemoteProviderConfig({ toolCatalog: catalog.all() }))
    const executor = testExecutor(
      repository,
      new ModelRuntime(new DeterministicProvider(), testRegistry()),
      { catalog, tools, runtimeConfig: remote.source },
    )
    executor.start()
    const aggressivePoller = setInterval(() => executor.notifyWorkAvailable(), 1)
    const app = buildTestServer({
      config: loadConfig({ NODE_ENV: "test" }),
      repository,
      requestVerifier: new RunScopedVerifier(),
      tools,
      remoteConfig: remote.source,
      notifyRunQueued: () => executor.notifyWorkAvailable(),
      toolCatalogDigest: () => catalog.digest,
    })

    try {
      // 1) 免审批工具：并发发送相同 Idempotency-Key
      const [freeResA, freeResB] = await Promise.all([
        app.inject({
          method: "POST",
          url: `/internal/v1/conversations/${conversation.id}/tool-actions`,
          headers: {
            "x-luna-dev-user": owner,
            "x-luna-dev-run": "airun_conc_free_a",
            "idempotency-key": "conc-free-tool-key",
          },
          payload: {
            operationId: "getProject",
            arguments: {},
            message: "读取项目",
            runId: "airun_conc_free_a",
          },
        }),
        app.inject({
          method: "POST",
          url: `/internal/v1/conversations/${conversation.id}/tool-actions`,
          headers: {
            "x-luna-dev-user": owner,
            "x-luna-dev-run": "airun_conc_free_b",
            "idempotency-key": "conc-free-tool-key",
          },
          payload: {
            operationId: "getProject",
            arguments: {},
            message: "读取项目",
            runId: "airun_conc_free_b",
          },
        }),
      ])
      expect(freeResA.statusCode).toBe(202)
      expect(freeResB.statusCode).toBe(202)
      expect(freeResA.headers["cache-control"]).toBe("no-store")
      const freeBodyA = freeResA.json<{ runId: string, toolCallId: string, state: string }>()
      const freeBodyB = freeResB.json<{ runId: string, toolCallId: string, state: string }>()
      expect(freeBodyB.runId).toBe(freeBodyA.runId)
      expect(freeBodyB.toolCallId).toBe(freeBodyA.toolCallId)
      expect(await tools.listByRun(freeBodyA.runId)).toHaveLength(1)

      // 等待后台 RunExecutor 完成免审批 Run，确认无 ai.direct_tool_not_ready
      await vi.waitFor(async () => {
        const current = await repository.getRun(owner, freeBodyA.runId)
        expect(current?.status).toBe("completed")
        expect(current?.errorCode).toBeUndefined()
      }, { timeout: 2_000 })

      // 2) 需审批工具：并发发送相同 Idempotency-Key
      const [approvalResA, approvalResB] = await Promise.all([
        app.inject({
          method: "POST",
          url: `/internal/v1/conversations/${conversation.id}/tool-actions`,
          headers: {
            "x-luna-dev-user": owner,
            "x-luna-dev-run": "airun_conc_appr_a",
            "idempotency-key": "conc-approval-tool-key",
          },
          payload: {
            operationId: "deleteApplication",
            arguments: {},
            message: "删除应用",
            runId: "airun_conc_appr_a",
          },
        }),
        app.inject({
          method: "POST",
          url: `/internal/v1/conversations/${conversation.id}/tool-actions`,
          headers: {
            "x-luna-dev-user": owner,
            "x-luna-dev-run": "airun_conc_appr_b",
            "idempotency-key": "conc-approval-tool-key",
          },
          payload: {
            operationId: "deleteApplication",
            arguments: {},
            message: "删除应用",
            runId: "airun_conc_appr_b",
          },
        }),
      ])
      expect(approvalResA.statusCode).toBe(202)
      expect(approvalResB.statusCode).toBe(202)
      const approvalBodyA = approvalResA.json<{ runId: string, toolCallId: string, state: string }>()
      const approvalBodyB = approvalResB.json<{ runId: string, toolCallId: string, state: string }>()
      expect(approvalBodyB.runId).toBe(approvalBodyA.runId)
      expect(approvalBodyB.toolCallId).toBe(approvalBodyA.toolCallId)
      expect(approvalBodyA.state).toBe("waiting_approval")
      expect(approvalBodyB.state).toBe("waiting_approval")
      expect(await tools.listByRun(approvalBodyA.runId)).toHaveLength(1)

      // 3) P1-1 审批分裂故障模拟：仅更新 ai.tool_calls 为 approve，但保留 ai.runs.status === "waiting_approval"
      await tools.recordApprovalDecision(approvalBodyA.toolCallId, "approve")
      expect((await repository.getRun(owner, approvalBodyA.runId))?.status).toBe("waiting_approval")

      // 客户端重试 POST /approvals/:toolCallId/decision，必须推进 Run 进入 queued 并由后台 RunExecutor 执行完成
      const recoveredApproval = await app.inject({
        method: "POST",
        url: `/internal/v1/runs/${approvalBodyA.runId}/approvals/${approvalBodyA.toolCallId}/decision`,
        headers: { "x-luna-dev-user": owner },
        payload: { decision: "approve" },
      })
      expect(recoveredApproval.statusCode).toBe(202)
      expect(["queued", "running", "completed"]).toContain(recoveredApproval.json<{ state: string }>().state)

      await vi.waitFor(async () => {
        const finished = await repository.getRun(owner, approvalBodyA.runId)
        expect(finished?.status).toBe("completed")
      }, { timeout: 2_000 })
      expect(deleteExecuted).toBe(true)
    }
    finally {
      clearInterval(aggressivePoller)
      await executor.stop()
      await app.close()
    }
  })

  it("P1-2 & P1-3 & P2-1：跨卡片工具交替调用共享修复上限、混合批次清理悬空占位、JSON Schema 零 not:{}、以及 proposed/awaiting_approval 残留收敛为 failed", async () => {
    // 1) 断言 3 个卡片工具导出的 JSON Schema 字符串均不含 "not":{}，且轻量卡片不含悬空 sourceRefIds
    for (const tool of businessCardTools) {
      const serialized = JSON.stringify(tool.inputSchema)
      expect(serialized).not.toContain("\"not\":{}")
      if (tool.operationId === "request_input" || tool.operationId === "request_choice") {
        expect(serialized).not.toContain("sourceRefIds")
      }
      else {
        expect(serialized).toContain("sourceRefIds")
      }
    }

    // 验证 request_choice 错误路径映射回 cards.<index>.selection... 且拦截不在 options 中的 defaultValue 与 minItems > maxItems
    const invalidChoice = businessCardToolInputs.request_choice.safeParse({
      title: "选择集群",
      cards: [{
        id: "cluster_card",
        presentation: { title: "运行集群" },
        selection: {
          id: "clusterId",
          label: "集群",
          multiple: true,
          minItems: 3,
          maxItems: 1,
          defaultValue: ["clu_nonexistent"],
          options: [{ value: "clu_1", label: "生产集群" }],
        },
        actions: [{ id: "confirm", type: "send_message", label: "确认", message: "选择 {{clusterId}}" }],
      }],
    })
    expect(invalidChoice.success).toBe(false)
    if (!invalidChoice.success) {
      const paths = invalidChoice.error.issues.map(issue => issue.path.join("."))
      expect(paths.every(p => !p.includes(".form."))).toBe(true)
      expect(paths).toContain("cards.0.selection.maxItems")
      expect(paths).toContain("cards.0.selection.defaultValue.0")
    }

    // 2) 交替调用 present_card -> request_choice -> present_card 共享单轮修复预算（maxCardRepairAttempts = 2）
    const owner = "usr_alternating_cards"
    const repository = new TestRepository()
    const conversation = await repository.createConversation(owner, "交替卡片上限", undefined, "user")
    const turn = await repository.createTurn(owner, {
      conversationId: conversation.id,
      input: "交替卡片测试",
      pageContext: {},
      idempotencyKey: "alternating-card-limit",
    })
    const { provider, requests } = scriptedProvider([
      // 第 1 次失败：present_card (attempt 1/2, retryable: true)
      { toolCalls: [{ id: "alt_1", operationId: "present_card", arguments: { title: "缺字段" } }] },
      // 第 2 次切换工具名失败：request_choice (attempt 2/2 -> 立即触发 cardRepairExhausted!)
      { toolCalls: [{ id: "alt_2", operationId: "request_choice", arguments: { title: "无 cards" } }] },
      // 第 3 次再切回合法的 present_card：因预算已耗尽，必须立即拒绝
      {
        toolCalls: [{
          id: "alt_3",
          operationId: "present_card",
          arguments: {
            title: "合法卡片也应被阻断",
            template: "result",
            cards: [{ id: "c1", presentation: { variant: "summary", title: "摘要" } }],
          },
        }],
      },
      { text: "已按要求降级为纯文本回复。" },
    ])
    await testExecutor(repository, new ModelRuntime(provider, testRegistry())).runOnce()

    const completedRun = await repository.getRun(owner, turn.run.id)
    expect(completedRun?.status).toBe("completed")
    const checkpoint = await repository.loadRunCheckpoint(turn.run.id)
    expect(checkpoint?.state.cardRepairExhausted).toBe(true)
    expect(checkpoint?.state.cardRepairAttempts).toBe(2)

    const step2Results = toolMessagesFromRequest(requests[2]!.messages)
    expect(step2Results.find(item => item.toolCallId === "alt_2")?.payload).toMatchObject({
      retryable: false,
      attempt: 2,
    })
    const step3Results = toolMessagesFromRequest(requests[3]!.messages)
    expect(step3Results.find(item => item.toolCallId === "alt_3")?.payload).toMatchObject({
      status: "failed",
      errorCode: "ai.interaction_card_schema_invalid",
      retryable: false,
    })

    // 3) P1-3：proposed 与 awaiting_approval 的残留 streaming tool_call 收敛后经 presentTimeline 输出均为 failed
    const turnDangling = await repository.createTurn(owner, {
      conversationId: conversation.id,
      input: "残留工具状态收敛",
      pageContext: {},
      idempotencyKey: "dangling-proposed-awaiting",
    })
    await repository.appendItem({
      runId: turnDangling.run.id,
      turnId: turnDangling.turn.id,
      type: "tool_call",
      status: "streaming",
      content: { toolCallId: "tc_proposed", operationId: "getProject", status: "proposed", arguments: {} },
    })
    await repository.appendItem({
      runId: turnDangling.run.id,
      turnId: turnDangling.turn.id,
      type: "tool_call",
      status: "streaming",
      content: { toolCallId: "tc_awaiting", operationId: "deleteApplication", status: "awaiting_approval", arguments: {} },
    })
    await repository.finalizeStreamingItems(turnDangling.run.id, "failed")
    await repository.updateRun(turnDangling.run.id, "queued", "failed", {
      completedAt: new Date().toISOString(),
      errorCode: "ai.run_failed",
    })

    const timeline = await presentTimeline(repository, owner, conversation.id)
    const danglingTurn = timeline?.turns.find(t => t.id === turnDangling.turn.id)
    const toolItems = (danglingTurn?.selectedRun?.items ?? []).filter(item => item.type === "tool_call") as Array<{
      status?: string
      toolCall?: { id?: string, status?: string }
    }>
    expect(toolItems).toHaveLength(2)
    expect(toolItems.find(item => item.toolCall?.id === "tc_proposed")?.toolCall?.status).toBe("failed")
    expect(toolItems.find(item => item.toolCall?.id === "tc_awaiting")?.toolCall?.status).toBe("failed")
  })
})
