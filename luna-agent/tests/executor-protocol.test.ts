import { describe, expect, it } from "vitest"
import type { TimelineItem } from "../src/domain.js"
import { ModelRuntime } from "../src/model-runtime.js"
import type { ModelMessage, ModelProvider, ModelToolCall } from "../src/provider/provider.js"
import { ToolCatalog } from "../src/tools/catalog.js"
import { DeterministicLunaApiClient } from "../src/tools/luna-api-client.js"
import { ProjectingToolCallStore, ToolOrchestrator } from "../src/tools/orchestrator.js"
import { TestRepository } from "./support/test-repository.js"
import { testRegistry } from "./support/model-tool-registry.js"
import { testExecutor } from "./support/run-executor.js"
import { testToolOperation } from "./support/tool-catalog.js"

const reportedUsage = { status: "reported" as const, value: { inputTokens: 3, outputTokens: 2, totalTokens: 5 } }

type ScriptedStep = {
  text?: string
  toolCalls?: ModelToolCall[]
  finishReason?: string
}

function protocolProvider(steps: ScriptedStep[]) {
  const state = { modelCalls: 0, requests: [] as ModelMessage[][] }
  const provider: ModelProvider = {
    async *stream(request) {
      state.requests.push(request.messages)
      const step = steps[Math.min(state.modelCalls, steps.length - 1)] ?? {}
      state.modelCalls += 1
      if (step.text) yield { type: "message_delta", delta: step.text }
      if (step.toolCalls?.length) yield { type: "tool_call_delta" }
      yield {
        type: "completed",
        usage: reportedUsage,
        ...(step.toolCalls?.length ? { toolCalls: step.toolCalls } : {}),
        ...(step.finishReason === undefined ? {} : { finishReason: step.finishReason }),
      }
    },
    async complete() { return { text: "", toolCalls: [], usage: reportedUsage } },
    capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
    health: async () => ({ ok: true }),
  }
  return { provider, modelCalls: () => state.modelCalls, requests: state.requests }
}

async function queuedRun(owner: string, key: string) {
  const repository = new TestRepository()
  const conversation = await repository.createConversation(owner, key)
  const created = await repository.createTurn(owner, {
    conversationId: conversation.id,
    input: "检查项目部署状态",
    pageContext: {},
    idempotencyKey: key,
    actorSessionId: `ses_${key}`,
  })
  return { repository, conversation, runId: created.run.id }
}

function assistantTexts(items: TimelineItem[]): string[] {
  return items
    .filter(item => item.type === "assistant_message")
    .map(item => (item.content as { parts?: Array<{ text?: string }> }).parts?.[0]?.text ?? "")
}

function progressTexts(items: TimelineItem[]): string[] {
  return items
    .filter(item => item.type === "progress")
    .map(item => (item.content as { parts?: Array<{ text?: string }> }).parts?.[0]?.text ?? "")
}

const planText = "我先梳理一下执行步骤，随后再确认部署状态。"

const invalidPresentCard = {
  schemaVersion: 1,
  title: "部署结果",
  mode: "interactive",
  template: "result",
  cards: [{
    id: "result",
    presentation: { variant: "receipt", title: "部署成功" },
    blocks: [{ id: "summary", type: "callout", tone: "success", content: "应用已通过健康检查。" }],
  }],
}

describe("模型协议错误处理", () => {
  it("结束原因宣告工具调用但没有工具调用时，先注入一次有界修复，仍违规才失败且计划文字不冒充最终回答", async () => {
    const owner = "usr_protocol_missing_calls"
    const { repository, conversation, runId } = await queuedRun(owner, "protocol-missing-calls")
    const { provider, modelCalls, requests } = protocolProvider([{ text: planText, finishReason: "tool_calls" }])

    await testExecutor(repository, new ModelRuntime(provider, testRegistry())).runOnce()

    const run = await repository.getRun(owner, runId)
    expect(run?.status).toBe("failed")
    expect(run?.errorCode).toBe("ai.model_protocol_violation")
    expect(modelCalls()).toBe(2)
    expect(requests[1]?.at(-1)).toMatchObject({ role: "system" })
    const items = (await repository.getTimeline(owner, conversation.id))?.turns[0]?.items ?? []
    expect(progressTexts(items)).toHaveLength(2)
    expect(progressTexts(items).every(text => text === planText)).toBe(true)
    expect(assistantTexts(items)).toEqual([])
    expect(items).toContainEqual(expect.objectContaining({ type: "progress", status: "completed" }))
  })

  it("有界修复后模型给出工具调用时继续执行并正常完成", async () => {
    const owner = "usr_protocol_repaired"
    const { repository, runId } = await queuedRun(owner, "protocol-repaired")
    const catalog = ToolCatalog.load([testToolOperation("getProject")])
    const store = repository.toolCallStore
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: { project: { id: "prj_1" } } }))
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(store, repository))
    const { provider, modelCalls } = protocolProvider([
      { text: planText, finishReason: "tool_calls" },
      { toolCalls: [{ id: "call-1", operationId: "getProject", arguments: {} }], finishReason: "tool_calls" },
      { text: "部署配置正常。", finishReason: "stop" },
    ])
    const executor = testExecutor(
      repository,
      new ModelRuntime(provider, testRegistry({ resolve: () => catalog.modelTools(["getProject"]) })),
      { catalog, tools },
    )

    await executor.runOnce()

    expect((await repository.getRun(owner, runId))?.status).toBe("completed")
    expect(modelCalls()).toBe(3)
    expect(client.calls).toHaveLength(1)
  })

  it("工具调用与非工具结束原因同时出现时不执行工具，直接以协议违规失败", async () => {
    const owner = "usr_protocol_invalid_reason"
    const { repository, runId } = await queuedRun(owner, "protocol-invalid-reason")
    const catalog = ToolCatalog.load([testToolOperation("getProject")])
    const store = repository.toolCallStore
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: {} }))
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(store, repository))
    const { provider, modelCalls } = protocolProvider([
      { toolCalls: [{ id: "call-1", operationId: "getProject", arguments: {} }], finishReason: "stop" },
    ])
    const executor = testExecutor(
      repository,
      new ModelRuntime(provider, testRegistry({ resolve: () => catalog.modelTools(["getProject"]) })),
      { catalog, tools },
    )

    await executor.runOnce()

    const run = await repository.getRun(owner, runId)
    expect(run?.status).toBe("failed")
    expect(run?.errorCode).toBe("ai.model_protocol_violation")
    expect(modelCalls()).toBe(1)
    expect(client.calls).toHaveLength(0)
  })

  it("非正常最终结束原因不会把截断文本判为完成", async () => {
    const owner = "usr_protocol_length"
    const { repository, runId } = await queuedRun(owner, "protocol-length")
    const { provider, modelCalls } = protocolProvider([{ text: "未完成的回答", finishReason: "length" }])

    await testExecutor(repository, new ModelRuntime(provider, testRegistry())).runOnce()

    const run = await repository.getRun(owner, runId)
    expect(run?.status).toBe("failed")
    expect(run?.errorCode).toBe("ai.model_protocol_violation")
    expect(modelCalls()).toBe(1)
  })

  it("卡片修复耗尽后，宣告工具调用的纯文本不再把 Run 判为完成", async () => {
    const owner = "usr_protocol_card_repair"
    const { repository, runId } = await queuedRun(owner, "protocol-card-repair")
    const { provider, modelCalls } = protocolProvider([
      { toolCalls: [{ id: "card-1", operationId: "present_card", arguments: invalidPresentCard }], finishReason: "tool_calls" },
      { toolCalls: [{ id: "card-2", operationId: "present_card", arguments: invalidPresentCard }], finishReason: "tool_calls" },
      { text: planText, finishReason: "tool_calls" },
    ])

    await testExecutor(repository, new ModelRuntime(provider, testRegistry())).runOnce()

    const run = await repository.getRun(owner, runId)
    expect(run?.status).toBe("failed")
    expect(run?.errorCode).toBe("ai.model_protocol_violation")
    expect(modelCalls()).toBe(3)
  })
})
