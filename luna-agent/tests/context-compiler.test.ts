import { describe, expect, it, vi } from "vitest"
import { ContextCompiler, type CompileContextInput } from "../src/context/compiler.js"
import type { ConversationHistoryEntry } from "../src/domain.js"
import { MemoryRepository } from "../src/persistence/memory.js"
import type { ModelProvider } from "../src/provider/provider.js"

describe("ContextCompiler", () => {
  it("persists an incremental structured summary and reuses its coverage cursor", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "长会话")
    for (let index = 0; index < 10; index += 1) {
      const created = await repository.createTurn("usr_a", {
        conversationId: conversation.id,
        input: `用户消息 ${index}`,
        pageContext: {},
        idempotencyKey: `context-${index}`,
      })
      await repository.appendItem({
        runId: created.run.id,
        turnId: created.turn.id,
        type: "assistant_message",
        status: "completed",
        content: { parts: [{ type: "text", text: `助手回复 ${index}` }] },
      })
    }
    const current = await repository.createTurn("usr_a", {
      conversationId: conversation.id,
      input: "继续",
      pageContext: {},
      idempotencyKey: "context-current",
    })
    const execution = await repository.getExecutionInput(current.run.id)
    const provider = summaryProvider()
    const compiler = new ContextCompiler(repository, provider, {
      inputTokenBudget: 8_000,
      compressionTriggerRatio: 0.8,
      compressionTargetRatio: 0.5,
      recentTurnCount: 4,
      maxRecentTurnCount: 8,
      maxUncompressedTurnCount: 8,
      maxCompressionTurnsPerCompile: 96,
      summaryInputTokenBudget: 4_000,
      summaryMaxOutputTokens: 500,
      historicalToolTokenBudget: 1_000,
    })

    const first = await compiler.compile(compileInput(conversation.id, execution!.turnIndex, execution!.history))
    expect(first.compressionOutcome).toBe("compressed")
    expect(first.summarizedThroughTurnIndex).toBe(5)
    expect(first.recentTurnCount).toBe(4)
    expect(first.messages.some(message => message.content.includes("历史会话结构化摘要"))).toBe(true)
    expect(await repository.getConversationSummary(conversation.id)).toMatchObject({
      coveredThroughTurnIndex: 5,
      compressionVersion: 1,
      sourceTurnCount: 6,
    })

    const second = await compiler.compile(compileInput(conversation.id, execution!.turnIndex, execution!.history))
    expect(second.compressionOutcome).toBe("reused")
    expect(provider.complete).toHaveBeenCalledOnce()
  })

  it("falls back to recent authoritative turns when summary generation fails", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "fallback")
    const history: ConversationHistoryEntry[] = Array.from({ length: 8 }, (_, turnIndex) => ({
      turnIndex,
      user: `user-${turnIndex}`,
      assistant: `assistant-${turnIndex}`,
    }))
    vi.spyOn(repository, "listConversationHistory").mockResolvedValue(history.slice(0, 4))
    const provider = summaryProvider(new Error("ai.provider_request_failed"))
    const compiler = new ContextCompiler(repository, provider, {
      inputTokenBudget: 8_000,
      compressionTriggerRatio: 0.8,
      compressionTargetRatio: 0.5,
      recentTurnCount: 4,
      maxRecentTurnCount: 6,
      maxUncompressedTurnCount: 6,
      maxCompressionTurnsPerCompile: 96,
      summaryInputTokenBudget: 4_000,
      summaryMaxOutputTokens: 500,
      historicalToolTokenBudget: 1_000,
    })

    const result = await compiler.compile(compileInput(conversation.id, 8, history.slice(-4)))

    expect(result.compressionOutcome).toBe("fallback")
    expect(result.messages.some(message => message.content.includes("user-7"))).toBe(true)
    expect(await repository.getConversationSummary(conversation.id)).toBeUndefined()
  })

  it("keeps current tool calls and results paired while compiling", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "tools")
    const compiler = new ContextCompiler(repository, summaryProvider(), {
      inputTokenBudget: 8_000,
      compressionTriggerRatio: 0.8,
      compressionTargetRatio: 0.5,
      recentTurnCount: 4,
      maxRecentTurnCount: 8,
      maxUncompressedTurnCount: 24,
      maxCompressionTurnsPerCompile: 96,
      summaryInputTokenBudget: 4_000,
      summaryMaxOutputTokens: 500,
      historicalToolTokenBudget: 1_000,
    })
    const input = compileInput(conversation.id, 0, [])
    input.continuationMessages = [
      { role: "assistant", content: "正在查询", toolCalls: [{ id: "call-1", operationId: "listProjects", arguments: {} }] },
      { role: "tool", toolCallId: "call-1", content: JSON.stringify({ status: "succeeded", result: [] }) },
    ]

    const result = await compiler.compile(input)

    expect(result.messages.slice(-2)).toEqual(input.continuationMessages)
  })

  it("incrementally catches up a large pre-existing conversation without silently covering the gap", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "large-history")
    const history: ConversationHistoryEntry[] = Array.from({ length: 250 }, (_, turnIndex) => ({
      turnIndex,
      user: `user-${turnIndex}`,
      assistant: `assistant-${turnIndex}`,
    }))
    vi.spyOn(repository, "listConversationHistory").mockImplementation(async (_conversationId, after, before, limit) =>
      history.filter(entry => entry.turnIndex > after && entry.turnIndex < before).slice(0, limit))
    const compiler = new ContextCompiler(repository, summaryProvider(), {
      inputTokenBudget: 8_000,
      compressionTriggerRatio: 0.8,
      compressionTargetRatio: 0.5,
      recentTurnCount: 4,
      maxRecentTurnCount: 8,
      maxUncompressedTurnCount: 8,
      maxCompressionTurnsPerCompile: 48,
      summaryInputTokenBudget: 4_000,
      summaryMaxOutputTokens: 500,
      historicalToolTokenBudget: 1_000,
    })

    const first = await compiler.compile(compileInput(conversation.id, 250, history.slice(-8)))
    expect(first.compressionOutcome).toBe("catching_up")
    expect(first.summarizedThroughTurnIndex).toBe(47)
    expect(first.messages.some(message => message.content.includes("第 48 至 241 轮尚未进入摘要"))).toBe(true)

    const second = await compiler.compile(compileInput(conversation.id, 250, history.slice(-8)))
    expect(second.compressionOutcome).toBe("catching_up")
    expect(second.summarizedThroughTurnIndex).toBe(95)
    expect(await repository.getConversationSummary(conversation.id)).toMatchObject({ sourceTurnCount: 96 })
  })

  it("bounds oversized continuation payloads while retaining complete tool call and result pairs", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "large-tools")
    const compiler = new ContextCompiler(repository, summaryProvider(), {
      inputTokenBudget: 600,
      compressionTriggerRatio: 0.8,
      compressionTargetRatio: 0.5,
      recentTurnCount: 4,
      maxRecentTurnCount: 8,
      maxUncompressedTurnCount: 24,
      maxCompressionTurnsPerCompile: 96,
      summaryInputTokenBudget: 300,
      summaryMaxOutputTokens: 500,
      historicalToolTokenBudget: 200,
    })
    const input = compileInput(conversation.id, 0, [])
    input.continuationMessages = ["call-1", "call-2"].flatMap((id): CompileContextInput["continuationMessages"] => [
      {
        role: "assistant",
        content: "准备调用".repeat(2_000),
        toolCalls: [{ id, operationId: "inspectResource", arguments: { payload: "参数".repeat(8_000) } }],
      },
      { role: "tool", toolCallId: id, content: JSON.stringify({ result: "结果".repeat(12_000) }) },
    ])

    const result = await compiler.compile(input)

    expect(result.estimatedInputTokens).toBeLessThanOrEqual(600)
    const continuation = result.messages.filter(message => message.role === "assistant" || message.role === "tool")
    expect(continuation).toHaveLength(4)
    for (let index = 0; index < continuation.length; index += 2) {
      const call = continuation[index]
      const toolResult = continuation[index + 1]
      expect(call?.role).toBe("assistant")
      expect(toolResult?.role).toBe("tool")
      if (call?.role === "assistant" && toolResult?.role === "tool") {
        expect(call.toolCalls?.[0]?.id).toBe(toolResult.toolCallId)
      }
    }
  })

  it("does not summarize a short conversation below the token high watermark", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "short-history")
    const history: ConversationHistoryEntry[] = Array.from({ length: 10 }, (_, turnIndex) => ({
      turnIndex,
      user: `user-${turnIndex}`,
      assistant: `assistant-${turnIndex}`,
    }))
    vi.spyOn(repository, "listConversationHistory").mockResolvedValue(history)
    const provider = summaryProvider()
    const compiler = new ContextCompiler(repository, provider, {
      inputTokenBudget: 8_000,
      compressionTriggerRatio: 0.8,
      compressionTargetRatio: 0.5,
      recentTurnCount: 4,
      maxRecentTurnCount: 12,
      maxUncompressedTurnCount: 24,
      maxCompressionTurnsPerCompile: 96,
      summaryInputTokenBudget: 4_000,
      summaryMaxOutputTokens: 500,
      historicalToolTokenBudget: 1_000,
    })

    const result = await compiler.compile(compileInput(conversation.id, 10, history.slice(-8)))

    expect(result.compressionOutcome).toBe("not_needed")
    expect(provider.complete).not.toHaveBeenCalled()
    expect(result.estimatedInputTokens).toBeLessThanOrEqual(8_000)
  })

  it("summarizes oversized recent turns by token pressure instead of dropping them silently", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "token-pressure")
    const history: ConversationHistoryEntry[] = Array.from({ length: 4 }, (_, turnIndex) => ({
      turnIndex,
      user: `user-${turnIndex}-${"输入".repeat(4_000)}`,
      assistant: `assistant-${turnIndex}-${"输出".repeat(8_000)}`,
      toolInteractions: [{ result: "工具结果".repeat(5_000) }],
    }))
    vi.spyOn(repository, "listConversationHistory").mockResolvedValue(history)
    const provider = summaryProvider()
    const compiler = new ContextCompiler(repository, provider, {
      inputTokenBudget: 4_000,
      compressionTriggerRatio: 0.8,
      compressionTargetRatio: 0.5,
      recentTurnCount: 4,
      maxRecentTurnCount: 8,
      maxUncompressedTurnCount: 24,
      maxCompressionTurnsPerCompile: 96,
      summaryInputTokenBudget: 4_000,
      summaryMaxOutputTokens: 500,
      historicalToolTokenBudget: 500,
    })

    const result = await compiler.compile(compileInput(conversation.id, 4, history))

    expect(result.compressionOutcome).toBe("compressed")
    expect(result.summarizedThroughTurnIndex).toBeGreaterThanOrEqual(0)
    expect(provider.complete).toHaveBeenCalled()
    expect(result.estimatedInputTokens).toBeLessThanOrEqual(4_000)
  })

  it("continues compressing a legacy backlog across runs until only recent verbatim turns remain", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "legacy-backlog")
    const history: ConversationHistoryEntry[] = Array.from({ length: 300 }, (_, turnIndex) => ({
      turnIndex,
      user: `user-${turnIndex}`,
      assistant: `assistant-${turnIndex}`,
    }))
    vi.spyOn(repository, "listConversationHistory").mockImplementation(async (_conversationId, after, before, limit) =>
      history.filter(entry => entry.turnIndex > after && entry.turnIndex < before).slice(0, limit))
    const compiler = new ContextCompiler(repository, summaryProvider(), {
      inputTokenBudget: 8_000,
      compressionTriggerRatio: 0.8,
      compressionTargetRatio: 0.5,
      recentTurnCount: 4,
      maxRecentTurnCount: 8,
      maxUncompressedTurnCount: 24,
      maxCompressionTurnsPerCompile: 48,
      summaryInputTokenBudget: 4_000,
      summaryMaxOutputTokens: 500,
      historicalToolTokenBudget: 1_000,
    })

    let result = await compiler.compile(compileInput(conversation.id, 300, history.slice(-8)))
    for (let attempt = 0; attempt < 8 && (result.summarizedThroughTurnIndex ?? -1) < 295; attempt += 1) {
      expect(result.estimatedInputTokens).toBeLessThanOrEqual(8_000)
      result = await compiler.compile(compileInput(conversation.id, 300, history.slice(-8)))
    }

    expect(result.summarizedThroughTurnIndex).toBe(295)
    expect(result.compressionOutcome).toBe("compressed")
    expect(result.messages.some(message => message.content.includes("压缩正在增量追赶"))).toBe(false)
    expect(result.estimatedInputTokens).toBeLessThanOrEqual(8_000)
  })

  it("applies a validated context budget update without recreating the compiler", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "runtime-budget")
    const compiler = new ContextCompiler(repository, summaryProvider(), {
      inputTokenBudget: 1,
      compressionTriggerRatio: 0.8,
      compressionTargetRatio: 0.5,
      recentTurnCount: 4,
      maxRecentTurnCount: 8,
      maxUncompressedTurnCount: 24,
      maxCompressionTurnsPerCompile: 96,
      summaryInputTokenBudget: 4_000,
      summaryMaxOutputTokens: 500,
      historicalToolTokenBudget: 1_000,
    })

    await expect(compiler.compile(compileInput(conversation.id, 0, [])))
      .rejects.toThrow("ai.context_base_budget_exhausted")
    compiler.setInputTokenBudget(8_000)

    const result = await compiler.compile(compileInput(conversation.id, 0, []))
    expect(result.estimatedInputTokens).toBeLessThanOrEqual(8_000)
  })
})

function compileInput(conversationId: string, beforeTurnIndex: number, history: ConversationHistoryEntry[]): CompileContextInput {
  return {
    conversationId,
    beforeTurnIndex,
    systemMessage: { role: "system" as const, content: "系统提示" },
    currentUserMessage: { role: "user" as const, content: "当前问题" },
    history,
    continuationMessages: [],
    tools: [],
  }
}

function summaryProvider(failure?: Error) {
  return {
    complete: vi.fn(async () => {
      if (failure) throw failure
      return {
        text: JSON.stringify({
          userGoals: ["完成诊断"],
          constraints: [],
          confirmedResources: [],
          completedActions: [],
          failures: [],
          pendingWork: ["继续检查"],
          durableFacts: [],
        }),
        usage: { inputTokens: 100, outputTokens: 30 },
      }
    }),
  } satisfies Pick<ModelProvider, "complete">
}
