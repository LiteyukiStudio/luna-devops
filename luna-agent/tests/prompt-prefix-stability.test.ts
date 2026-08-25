import { describe, expect, it } from "vitest"
import { boundContinuationMessages, boundHistoryMessages } from "../src/context/model-messages.js"
import type { ConversationHistoryEntry } from "../src/domain.js"
import { ModelRuntime, type AssistantModelInput } from "../src/model-runtime.js"
import type { ModelMessage, ModelProvider, ModelRequest } from "../src/provider/provider.js"

const reported = { status: "reported" as const, value: { inputTokens: 20, outputTokens: 5, totalTokens: 25 } }

describe("Provider prompt prefix stability", () => {
  it("replays a prior current user message byte-for-byte from persisted history", async () => {
    const requests: ModelRequest[] = []
    const runtime = new ModelRuntime(capturingProvider(requests))
    await runtime.complete(runtimeInput({
      input: "检查这个构建",
      pageContext: { z: 1, nested: { b: 2, a: 1 } },
      conversation: { title: "初始标题", titleSource: "default", turnIndex: 0 },
    }))
    await runtime.complete(runtimeInput({
      input: "继续",
      pageContext: {},
      history: [{
        turnIndex: 0,
        user: "检查这个构建",
        assistant: "构建失败在测试阶段。",
        pageContext: { nested: { a: 1, b: 2 }, z: 1 },
      }],
      conversation: { title: "已经变化的标题", titleSource: "assistant", turnIndex: 1 },
    }))

    const firstCurrent = requests[0]!.messages.find(message => message.role === "user")
    const replayed = requests[1]!.messages.find(message => message.role === "user" && message.content.includes("第 0 轮"))
    expect(requests[1]!.messages.slice(0, requests[0]!.messages.length)).toEqual(requests[0]!.messages)
    expect(Buffer.from(JSON.stringify(replayed))).toEqual(Buffer.from(JSON.stringify(firstCurrent)))
    expect(firstCurrent?.content).not.toContain("初始标题")
    expect(replayed?.content).not.toContain("已经变化的标题")
    expect(firstCurrent?.content).toContain('{"nested":{"a":1,"b":2},"z":1}')
  })

  it("keeps Run workflow references frozen and Provider tool order independent from LRU order", async () => {
    const requests: ModelRequest[] = []
    const runtime = new ModelRuntime(capturingProvider(requests), (_pageContext, _userInput, operationIds) =>
      operationIds.map(operationId => ({ operationId, description: operationId, inputSchema: { type: "object" } })))
    const base = runtimeInput({ input: "继续", conversation: { title: "用户标题", titleSource: "user", turnIndex: 0 } })

    await runtime.complete({ ...base, loadedOperationIds: [] })
    await runtime.complete({ ...base, loadedOperationIds: ["createApplication", "fetchWebPage"] })
    await runtime.complete({ ...base, loadedOperationIds: ["zetaTool", "alphaTool"] })
    await runtime.complete({ ...base, loadedOperationIds: ["alphaTool", "zetaTool"] })

    const systemMessages = (request: ModelRequest) => request.messages.filter(message => message.role === "system")
    expect(systemMessages(requests[1]!)).toEqual(systemMessages(requests[0]!))
    expect(requests[2]!.tools?.map(tool => tool.operationId)).toEqual(["alphaTool", "zetaTool"])
    expect(requests[3]!.tools).toEqual(requests[2]!.tools)
  })

  it("does not redistribute old history or continuation item bytes when a new item is appended", () => {
    const firstHistory: ConversationHistoryEntry = {
      turnIndex: 0,
      user: "用户".repeat(6_000),
      assistant: "助手".repeat(8_000),
      pageContext: { routeName: "build.detail" },
    }
    const historyBefore = boundHistoryMessages([firstHistory], 64 * 1024)
    const historyAfter = boundHistoryMessages([
      firstHistory,
      { turnIndex: 1, user: "继续", assistant: "收到", pageContext: {} },
    ], 64 * 1024)
    expect(serialized(historyAfter.slice(0, historyBefore.length))).toBe(serialized(historyBefore))

    const firstContinuation: ModelMessage = { role: "assistant", content: "a".repeat(28_000) }
    const continuationBefore = boundContinuationMessages([firstContinuation], 64 * 1024)
    const continuationAfter = boundContinuationMessages([
      firstContinuation,
      { role: "assistant", content: "b".repeat(4_000) },
    ], 64 * 1024)
    expect(serialized(continuationAfter.slice(0, continuationBefore.length))).toBe(serialized(continuationBefore))
  })

  it("keeps the overall continuation payload bounded while retaining complete newest exchanges", () => {
    const bounded = boundContinuationMessages([
      { role: "assistant", content: "old".repeat(4_000), toolCalls: [{ id: "old", operationId: "old_tool", arguments: {} }] },
      { role: "tool", toolCallId: "old", content: "old-result".repeat(2_000) },
      { role: "assistant", content: "new".repeat(4_000), toolCalls: [{ id: "new", operationId: "new_tool", arguments: {} }] },
      { role: "tool", toolCallId: "new", content: "new-result".repeat(2_000) },
    ], 1024)

    expect(payloadBytes(bounded)).toBeLessThanOrEqual(1024)
    expect(bounded[0]).toMatchObject({ role: "assistant", toolCalls: [{ id: "new" }] })
    expect(bounded[1]).toMatchObject({ role: "tool", toolCallId: "new" })
  })
})

function capturingProvider(requests: ModelRequest[]): ModelProvider {
  return {
    async *stream(request) {
      requests.push(request)
      yield { type: "completed", usage: reported }
    },
    async complete(request) {
      requests.push(request)
      return { text: "ok", usage: reported }
    },
    capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
    health: async () => ({ ok: true }),
  }
}

function runtimeInput(overrides: Partial<AssistantModelInput> = {}): AssistantModelInput {
  return {
    runId: "airun_prefix",
    ownerUserId: "usr_prefix",
    conversationId: "aicnv_prefix",
    input: "你好",
    pageContext: {},
    history: [],
    conversation: { title: "标题", titleSource: "user", turnIndex: 0 },
    promptVersion: "system-v4",
    reasoningSummary: "",
    answer: "",
    toolCalls: [],
    continuationMessages: [],
    loadedOperationIds: [],
    toolCatalogDigest: "sha256:prefix",
    ...overrides,
  }
}

function serialized(messages: ModelMessage[]): string {
  return JSON.stringify(messages)
}

function payloadBytes(messages: ModelMessage[]): number {
  return messages.reduce((total, message) => total
    + Buffer.byteLength(message.content, "utf8")
    + (message.role === "assistant" && message.toolCalls ? Buffer.byteLength(JSON.stringify(message.toolCalls), "utf8") : 0), 0)
}
