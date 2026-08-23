import { describe, expect, it, vi } from "vitest"
import { ContextCompiler } from "../src/context/compiler.js"
import type { AIModelSnapshot, ConversationHistoryEntry, ConversationSummary } from "../src/domain.js"
import type { Repository } from "../src/persistence/repository.js"
import { ProviderRequestError } from "../src/provider/provider-error.js"

const model: AIModelSnapshot = {
  id: "aimdl_400k", name: "400K", maxContextTokens: 400_000, maxOutputTokens: 8_000,
  inputCreditsPerMillion: "1", outputCreditsPerMillion: "2", cachedInputCreditsPerMillion: "0.5",
}
const summaryJSON = JSON.stringify({
  userGoals: ["完成部署"], constraints: [], confirmedResources: [], completedActions: [], failures: [], pendingWork: [], durableFacts: [],
})
const options = {
  compressionTriggerRatio: 0.8,
  recentTurnCount: 1,
  maxUncompressedTurnCount: 4,
  maxCompressionTurnsPerCompile: 8,
  summaryMaxOutputTokens: 1_024,
  maxHistoryPayloadBytes: 64 * 1_024,
  maxSummaryPayloadBytes: 2 * 1_024,
  maxContinuationPayloadBytes: 32 * 1_024,
}

describe("ContextCompiler authoritative compression", () => {
  it("triggers from the previous official prompt_tokens ratio", async () => {
    const history = entries(3)
    const repository = memoryRepository({ history, latestUsage: { modelId: "aimdl_400k", promptTokens: 320_000, maxContextTokensSnapshot: 400_000 } })
    const complete = vi.fn(async () => ({ text: summaryJSON, usage: reported(500, 100) }))

    const result = await new ContextCompiler(repository, { complete }, options).compile(compileInput(history))

    expect(complete).toHaveBeenCalledOnce()
    expect(result.compaction).toEqual({ summarizedThroughTurnIndex: 1, sourceTurnCount: 2, trigger: "provider_usage", priorPromptTokens: 320_000 })
    expect(result.messages.some(message => message.content.includes("历史会话结构化摘要"))).toBe(true)
  })

  it("does not reject a new 400K-model request before calling the Provider", async () => {
    const repository = memoryRepository({ history: [] })
    const complete = vi.fn(async () => ({ text: summaryJSON, usage: reported(1, 1) }))
    const current = "新输入".repeat(200_000)

    const result = await new ContextCompiler(repository, { complete }, options).compile(compileInput([], { currentUserMessage: { role: "user", content: current } }))

    expect(complete).not.toHaveBeenCalled()
    expect(result.compressionOutcome).toBe("not_needed")
    expect(result.messages.at(-1)).toEqual({ role: "user", content: current })
  })

  it("uses explicit turn backlog instead of a local Token preflight", async () => {
    const history = entries(6)
    const repository = memoryRepository({ history })
    const complete = vi.fn(async () => ({ text: summaryJSON, usage: reported(20, 5) }))

    const result = await new ContextCompiler(repository, { complete }, options).compile(compileInput(history))

    expect(result.compaction?.trigger).toBe("turn_backlog")
    expect(result.compaction?.priorPromptTokens).toBeUndefined()
  })

  it("bisects a summary batch only after a structured context error", async () => {
    const history = entries(4)
    const repository = memoryRepository({ history })
    const complete = vi.fn()
      .mockRejectedValueOnce(contextError())
      .mockResolvedValue({ text: summaryJSON, usage: reported(20, 5) })

    const result = await new ContextCompiler(repository, { complete }, options).compile(compileInput(history, { forceCompressionTrigger: "context_error" }))

    expect(complete).toHaveBeenCalledTimes(3)
    expect(result.compaction).toMatchObject({ trigger: "context_error", sourceTurnCount: 4, summarizedThroughTurnIndex: 3 })
  })

  it("segments a single oversized turn on a byte boundary after Provider rejection", async () => {
    const history: ConversationHistoryEntry[] = [{ turnIndex: 0, user: "用户".repeat(4_000), assistant: "助手".repeat(4_000) }]
    const repository = memoryRepository({ history })
    const complete = vi.fn()
      .mockRejectedValueOnce(contextError())
      .mockResolvedValue({ text: summaryJSON, usage: reported(20, 5) })

    const result = await new ContextCompiler(repository, { complete }, options).compile(compileInput(history, { forceCompressionTrigger: "context_error" }))

    expect(complete.mock.calls.length).toBeGreaterThan(2)
    expect(result.compaction).toMatchObject({ trigger: "context_error", sourceTurnCount: 1 })
  })

  it("does not hide authentication and availability failures behind fallback", async () => {
    const history = entries(2)
    const repository = memoryRepository({ history })
    const complete = vi.fn(async () => { throw new ProviderRequestError("ai.provider_auth_failed", { stage: "response_headers", requestOutcome: "rejected" }) })

    await expect(new ContextCompiler(repository, { complete }, options).compile(compileInput(history, { forceCompressionTrigger: "context_error" })))
      .rejects.toThrow("ai.provider_auth_failed")
    expect(complete).toHaveBeenCalledOnce()
  })

  it("preserves an official usage contract failure instead of accepting the summary", async () => {
    const history = entries(2)
    const repository = memoryRepository({ history })
    const complete = vi.fn(async () => ({
      text: summaryJSON,
      usage: { status: "unavailable" as const, reason: "missing_usage" as const },
    }))

    await expect(new ContextCompiler(repository, { complete }, options).compile(compileInput(history, { forceCompressionTrigger: "context_error" })))
      .rejects.toThrow("ai.provider_usage_unavailable")
    expect(complete).toHaveBeenCalledOnce()
  })
})

function compileInput(history: ConversationHistoryEntry[], overrides: Record<string, unknown> = {}) {
  return {
    conversationId: "aicnv_test", beforeTurnIndex: history.length,
    systemMessage: { role: "system" as const, content: "系统提示" },
    currentUserMessage: { role: "user" as const, content: "继续" },
    history, continuationMessages: [], tools: [], model,
    ...overrides,
  }
}

function entries(count: number): ConversationHistoryEntry[] {
  return Array.from({ length: count }, (_, turnIndex) => ({ turnIndex, user: `用户 ${turnIndex}`, assistant: `助手 ${turnIndex}` }))
}

function reported(promptTokens: number, completionTokens: number) {
  return { status: "reported" as const, value: { promptTokens, completionTokens, totalTokens: promptTokens + completionTokens } }
}

function contextError() {
  return new ProviderRequestError("ai.provider_context_length_exceeded", { stage: "response_headers", requestOutcome: "rejected" })
}

function memoryRepository(input: {
  history: ConversationHistoryEntry[]
  latestUsage?: { modelId: string, promptTokens: number, maxContextTokensSnapshot: number }
}) {
  let summary: ConversationSummary | undefined
  return {
    getConversationSummary: async () => summary,
    getLatestReportedModelUsage: async () => input.latestUsage,
    listConversationHistory: async (_conversationId: string, after: number, before: number, limit: number) =>
      input.history.filter(entry => entry.turnIndex > after && entry.turnIndex < before).slice(0, limit),
    saveConversationSummary: async (value: Omit<ConversationSummary, "createdAt" | "updatedAt">) => {
      const now = new Date().toISOString()
      summary = { ...value, createdAt: now, updatedAt: now }
      return summary
    },
  } as Pick<Repository, "getConversationSummary" | "getLatestReportedModelUsage" | "listConversationHistory" | "saveConversationSummary">
}
