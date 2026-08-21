import { describe, expect, it, vi } from "vitest"
import type { Repository } from "../src/persistence/repository.js"
import { BudgetedModelProvider, estimateRequestInputTokens } from "../src/provider/budgeted.js"
import type { ModelProvider } from "../src/provider/provider.js"

describe("BudgetedModelProvider", () => {
  it("sends only the database-approved output limit and confirms usage", async () => {
    const reserveModelBudget = vi.fn(async () => ({ id: "aibgt_test", maxOutputTokens: 37 }))
    const confirmModelBudget = vi.fn(async () => undefined)
    const complete = vi.fn(async () => ({ text: "ok", usage: { inputTokens: 11, outputTokens: 7, reported: true } }))
    const provider = new BudgetedModelProvider(modelProvider({ complete }), {
      reserveModelBudget,
      confirmModelBudget,
      releaseModelBudget: vi.fn(async () => undefined),
    } as unknown as Repository)
    const result = await provider.complete({
      messages: [{ role: "user", content: "hello" }],
      maxOutputTokens: 100,
      budget: { runId: "airun_test", ownerUserId: "usr_test", operation: "assistant" },
    })
    expect(complete).toHaveBeenCalledWith(expect.objectContaining({ maxOutputTokens: 37 }))
    expect(confirmModelBudget).toHaveBeenCalledWith("aibgt_test", expect.objectContaining({ inputTokens: 11, outputTokens: 7, reported: true }))
    expect(result.reservationId).toBe("aibgt_test")
  })

  it("uses UTF-8 bytes as the final tokenizer-independent input upper bound", () => {
    const content = "a".repeat(10_000)
    const estimate = estimateRequestInputTokens({ messages: [{ role: "user", content }], tools: [] })
    expect(estimate).toBeGreaterThanOrEqual(Buffer.byteLength(JSON.stringify([{ role: "user", content }]), "utf8"))
  })

  it("conservatively confirms the full reservation when provider usage is unknown", async () => {
    const confirmModelBudget = vi.fn(async () => undefined)
    const provider = new BudgetedModelProvider(modelProvider({
      complete: async () => ({ text: "ok", usage: { inputTokens: 0, outputTokens: 0, reported: false } }),
    }), {
      reserveModelBudget: async () => ({ id: "aibgt_unknown", maxOutputTokens: 10 }),
      confirmModelBudget,
      releaseModelBudget: async () => undefined,
    } as unknown as Repository)
    await provider.complete({
      messages: [{ role: "user", content: "hello" }], maxOutputTokens: 10,
      budget: { runId: "airun_test", ownerUserId: "usr_test", operation: "title" },
    })
    expect(confirmModelBudget).toHaveBeenCalledWith("aibgt_unknown", expect.objectContaining({ reported: false }))
  })

  it.each(["ai.not_configured", "ai.model_not_available", "ai.provider_config_unavailable"])(
    "releases a reservation for the definite pre-dispatch failure %s",
    async (code) => {
      const releaseModelBudget = vi.fn(async () => undefined)
      const confirmModelBudget = vi.fn(async () => undefined)
      const provider = new BudgetedModelProvider(modelProvider({
        complete: async () => { throw new Error(code) },
      }), {
        reserveModelBudget: async () => ({ id: "aibgt_pre_dispatch", maxOutputTokens: 10 }),
        confirmModelBudget,
        releaseModelBudget,
      } as unknown as Repository)

      await expect(provider.complete({
        messages: [{ role: "user", content: "secret prompt" }], maxOutputTokens: 10,
        budget: { runId: "airun_test", ownerUserId: "usr_test", operation: "assistant" },
      })).rejects.toThrow(code)
      expect(releaseModelBudget).toHaveBeenCalledWith("aibgt_pre_dispatch")
      expect(confirmModelBudget).not.toHaveBeenCalled()
    },
  )

  it("does not invoke the provider when the authoritative reservation rejects", async () => {
    const complete = vi.fn(async () => ({ text: "unexpected", usage: { inputTokens: 1, outputTokens: 1 } }))
    const provider = new BudgetedModelProvider(modelProvider({ complete }), {
      reserveModelBudget: async () => { throw new Error("ai.run_token_budget_exhausted") },
      confirmModelBudget: vi.fn(async () => undefined),
      releaseModelBudget: vi.fn(async () => undefined),
    } as unknown as Repository)

    await expect(provider.complete({
      messages: [{ role: "user", content: "hello" }], maxOutputTokens: 10,
      budget: { runId: "airun_test", ownerUserId: "usr_test", operation: "assistant" },
    })).rejects.toThrow("ai.run_token_budget_exhausted")
    expect(complete).not.toHaveBeenCalled()
  })

  it("keeps successful provider output but confirms the full hold for invalid cached usage", async () => {
    const confirmModelBudget = vi.fn(async (_id: string, usage?: { reported: boolean }) => {
      if (usage) throw new Error("ai.provider_usage_invalid")
    })
    const provider = new BudgetedModelProvider(modelProvider({
      complete: async () => ({
        text: "provider result",
        usage: { inputTokens: 2, outputTokens: 3, cachedInputTokens: 4, reported: true },
      }),
    }), {
      reserveModelBudget: async () => ({ id: "aibgt_bad_usage", maxOutputTokens: 10 }),
      confirmModelBudget,
      releaseModelBudget: vi.fn(async () => undefined),
    } as unknown as Repository)

    await expect(provider.complete({
      messages: [{ role: "user", content: "hello" }], maxOutputTokens: 10,
      budget: { runId: "airun_test", ownerUserId: "usr_test", operation: "assistant" },
    })).resolves.toMatchObject({ text: "provider result", reservationId: "aibgt_bad_usage" })
    expect(confirmModelBudget).toHaveBeenNthCalledWith(2, "aibgt_bad_usage")
  })

  it.each(["assistant", "summary", "title"] as const)("forwards the %s operation to the single reservation entry point", async (operation) => {
    const reserveModelBudget = vi.fn(async () => ({ id: `aibgt_${operation}`, maxOutputTokens: 10 }))
    const provider = new BudgetedModelProvider(modelProvider({}), {
      reserveModelBudget,
      confirmModelBudget: vi.fn(async () => undefined),
      releaseModelBudget: vi.fn(async () => undefined),
    } as unknown as Repository)
    await provider.complete({
      messages: [{ role: "user", content: "hello" }], maxOutputTokens: 10,
      budget: { runId: "airun_shared", ownerUserId: "usr_shared", operation },
    })
    expect(reserveModelBudget).toHaveBeenCalledWith(expect.objectContaining({ runId: "airun_shared", ownerUserId: "usr_shared", operation }))
  })

  it("does not emit prompts, wallet values, or identifiers in budget telemetry", async () => {
    const write = vi.spyOn(process.stdout, "write").mockImplementation(() => true)
    try {
      const provider = new BudgetedModelProvider(modelProvider({}), {
        reserveModelBudget: async () => ({ id: "aibgt_sensitive", maxOutputTokens: 10 }),
        confirmModelBudget: vi.fn(async () => undefined),
        releaseModelBudget: vi.fn(async () => undefined),
      } as unknown as Repository)
      await provider.complete({
        messages: [{ role: "user", content: "prompt-secret-7f4f" }], maxOutputTokens: 10,
        budget: { runId: "airun-sensitive-7f4f", ownerUserId: "usr-sensitive-7f4f", operation: "assistant" },
      })
      const telemetry = write.mock.calls.map(call => String(call[0])).join("\n")
      expect(telemetry).not.toContain("prompt-secret-7f4f")
      expect(telemetry).not.toContain("airun-sensitive-7f4f")
      expect(telemetry).not.toContain("usr-sensitive-7f4f")
    }
    finally {
      write.mockRestore()
    }
  })
})

function modelProvider(overrides: Partial<ModelProvider>): ModelProvider {
  return {
    capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
    health: async () => ({ ok: true }),
    complete: async () => ({ text: "", usage: { inputTokens: 1, outputTokens: 1 } }),
    async *stream() { yield { type: "completed", usage: { inputTokens: 1, outputTokens: 1 } } },
    ...overrides,
  }
}
