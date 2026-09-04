import { describe, expect, it, vi } from "vitest"
import type { ContextCompiler } from "../src/context/compiler.js"
import { ModelRuntime, type AssistantModelInput } from "../src/model-runtime.js"
import { ProviderRequestError } from "../src/provider/provider-error.js"
import type { ModelProvider } from "../src/provider/provider.js"
import { testRegistry } from "./support/model-tool-registry.js"

const reported = { status: "reported" as const, value: { inputTokens: 20, outputTokens: 5, totalTokens: 25 } }

describe("ModelRuntime structured context retry", () => {
  it("compresses after a structured context rejection and performs a second independent Provider attempt", async () => {
    const compile = vi.fn()
      .mockResolvedValueOnce(compiled())
      .mockResolvedValueOnce(compiled({ summarizedThroughTurnIndex: 0, sourceTurnCount: 1, trigger: "context_error" }))
    let attempt = 0
    const provider = modelProvider(async function* () {
      attempt += 1
      if (attempt === 1) throw contextError()
      yield { type: "completed", usage: reported }
    })

    const events: Array<{ type: string }> = []
    for await (const event of new ModelRuntime(provider, testRegistry(), { compile, setOptions: vi.fn() } as unknown as ContextCompiler).stream(input())) events.push(event)

    expect(attempt).toBe(2)
    expect(compile).toHaveBeenCalledTimes(2)
    expect(events.map(event => event.type)).toEqual(["context.compacted", "completed"])
  })

  it("returns model_context_insufficient without repeating an unchanged oversized current message", async () => {
    const compile = vi.fn().mockResolvedValue(compiled())
    let attempts = 0
    const provider = modelProvider(async function* () {
      attempts += 1
      yield { type: "message_delta", delta: "" }
      throw contextError()
    })
    const consume = async () => {
      for await (const _event of new ModelRuntime(provider, testRegistry(), { compile, setOptions: vi.fn() } as unknown as ContextCompiler).stream(input())) void _event
    }

    await expect(consume()).rejects.toThrow("ai.model_context_insufficient")
    expect(attempts).toBe(1)
    expect(compile).toHaveBeenCalledTimes(2)
  })
})

function compiled(compaction?: { summarizedThroughTurnIndex: number, sourceTurnCount: number, trigger: "context_error" }) {
  return {
    messages: [{ role: "system" as const, content: "system" }, { role: "user" as const, content: "current" }],
    recentTurnCount: compaction ? 0 : 1,
    compressionOutcome: compaction ? "compressed" as const : "not_needed" as const,
    ...(compaction ? { summarizedThroughTurnIndex: compaction.summarizedThroughTurnIndex, compaction } : {}),
  }
}

function modelProvider(stream: ModelProvider["stream"]): ModelProvider {
  return {
    stream,
    complete: async () => ({ text: "", usage: reported }),
    capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
    health: async () => ({ ok: true }),
  }
}

function contextError() {
  return new ProviderRequestError("ai.provider_context_length_exceeded", { stage: "response_headers", requestOutcome: "rejected" })
}

function input(): AssistantModelInput {
  return {
    runId: "airun_test", ownerUserId: "usr_test", conversationId: "aicnv_test", input: "current",
    pageContext: {}, history: [{ turnIndex: 0, user: "old", assistant: "answer" }],
    conversation: { title: "test", titleSource: "user", turnIndex: 1 }, promptVersion: "system-v4",
    reasoningSummary: "", answer: "", toolCalls: [], continuationMessages: [], loadedOperationIds: [], toolCatalogDigest: "sha256:test",
  }
}
