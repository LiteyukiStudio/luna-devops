import { describe, expect, it, vi } from "vitest"
import type { Repository } from "../src/persistence/repository.js"
import { BudgetedModelProvider } from "../src/provider/budgeted.js"
import { ProviderRequestError } from "../src/provider/provider-error.js"
import type { ModelProvider } from "../src/provider/provider.js"

const reported = { status: "reported" as const, value: { promptTokens: 11, completionTokens: 7, totalTokens: 18 } }
const budget = { runId: "airun_test", ownerUserId: "usr_test", operation: "assistant" as const }

describe("BudgetedModelProvider", () => {
  it("creates one credit hold and persists only Provider-reported usage", async () => {
    const createModelCreditHold = vi.fn(async () => ({ id: "aihold_1", attempt: 1, maxOutputTokens: 37 }))
    const recordReportedModelUsage = vi.fn(async (holdId: string) => ({ reconciliationRequired: holdId === "" }))
    const complete = vi.fn(async () => ({ text: "ok", usage: reported, providerRequestId: "req_1", responseId: "chatcmpl_1" }))
    const provider = new BudgetedModelProvider(modelProvider({ complete }), repository({ createModelCreditHold, recordReportedModelUsage }))

    const result = await provider.complete({ messages: [{ role: "user", content: "hello" }], maxOutputTokens: 100, budget })

    expect(createModelCreditHold).toHaveBeenCalledWith(expect.objectContaining({ ...budget, requestedOutputTokens: 100 }))
    expect(complete).toHaveBeenCalledWith(expect.objectContaining({ maxOutputTokens: 37 }))
    expect(recordReportedModelUsage).toHaveBeenCalledWith("aihold_1", reported.value, expect.objectContaining({
      callType: "complete", providerRequestId: "req_1", responseId: "chatcmpl_1",
    }))
    expect(result.creditHoldId).toBe("aihold_1")
  })

  it("marks missing usage for reconciliation without inventing a usage row", async () => {
    const recordReportedModelUsage = vi.fn(async () => ({ reconciliationRequired: false }))
    const markModelUsageUnavailable = vi.fn(async () => undefined)
    const provider = new BudgetedModelProvider(modelProvider({
      complete: async () => ({ text: "ok", usage: { status: "unavailable", reason: "missing_usage" } }),
    }), repository({ recordReportedModelUsage, markModelUsageUnavailable }))

    const result = await provider.complete({ messages: [{ role: "user", content: "hello" }], maxOutputTokens: 10, budget })

    expect(recordReportedModelUsage).not.toHaveBeenCalled()
    expect(markModelUsageUnavailable).toHaveBeenCalledWith("aihold_1", "missing_usage", expect.objectContaining({ callType: "complete" }))
    expect(result).toMatchObject({ creditHoldId: "aihold_1", reconciliationRequired: true })
  })

  it("releases a hold for a structured Provider rejection", async () => {
    const releaseModelCreditHold = vi.fn(async () => undefined)
    const markModelUsageUnavailable = vi.fn(async () => undefined)
    const provider = new BudgetedModelProvider(modelProvider({
      complete: async () => { throw new ProviderRequestError("ai.provider_context_length_exceeded", { stage: "response_headers", requestOutcome: "rejected", providerRequestId: "req_context" }) },
    }), repository({ releaseModelCreditHold, markModelUsageUnavailable }))

    await expect(provider.complete({ messages: [{ role: "user", content: "hello" }], maxOutputTokens: 10, budget }))
      .rejects.toThrow("ai.provider_context_length_exceeded")
    expect(releaseModelCreditHold).toHaveBeenCalledWith("aihold_1")
    expect(markModelUsageUnavailable).not.toHaveBeenCalled()
  })

  it("keeps an unknown dispatch outcome in reconciliation", async () => {
    const releaseModelCreditHold = vi.fn(async () => undefined)
    const markModelUsageUnavailable = vi.fn(async () => undefined)
    const provider = new BudgetedModelProvider(modelProvider({
      complete: async () => { throw new ProviderRequestError("ai.provider_timeout", { stage: "dispatch", requestOutcome: "unknown" }) },
    }), repository({ releaseModelCreditHold, markModelUsageUnavailable }))

    await expect(provider.complete({ messages: [{ role: "user", content: "hello" }], maxOutputTokens: 10, budget })).rejects.toThrow("ai.provider_timeout")
    expect(releaseModelCreditHold).not.toHaveBeenCalled()
    expect(markModelUsageUnavailable).toHaveBeenCalledWith("aihold_1", "request_outcome_unknown", expect.objectContaining({ failureStage: "dispatch" }))
  })

  it("creates a separate hold for every explicit model attempt", async () => {
    const createModelCreditHold = vi.fn()
      .mockResolvedValueOnce({ id: "aihold_1", attempt: 1, maxOutputTokens: 10 })
      .mockResolvedValueOnce({ id: "aihold_2", attempt: 2, maxOutputTokens: 10 })
    const recordReportedModelUsage = vi.fn(async (holdId: string) => ({ reconciliationRequired: holdId === "" }))
    const provider = new BudgetedModelProvider(modelProvider({}), repository({ createModelCreditHold, recordReportedModelUsage }))

    await provider.complete({ messages: [{ role: "user", content: "first" }], maxOutputTokens: 10, budget })
    await provider.complete({ messages: [{ role: "user", content: "second" }], maxOutputTokens: 10, budget })

    expect(createModelCreditHold).toHaveBeenCalledTimes(2)
    expect(recordReportedModelUsage.mock.calls.map(call => call[0])).toEqual(["aihold_1", "aihold_2"])
  })

  it("does not emit prompts, wallet values, or identifiers in hold telemetry", async () => {
    const write = vi.spyOn(process.stdout, "write").mockImplementation(() => true)
    try {
      const provider = new BudgetedModelProvider(modelProvider({}), repository({}))
      await provider.complete({
        messages: [{ role: "user", content: "prompt-secret-7f4f" }], maxOutputTokens: 10,
        budget: { runId: "airun-sensitive-7f4f", ownerUserId: "usr-sensitive-7f4f", operation: "assistant" },
      })
      const telemetry = write.mock.calls.map(call => String(call[0])).join("\n")
      expect(telemetry).not.toContain("prompt-secret-7f4f")
      expect(telemetry).not.toContain("airun-sensitive-7f4f")
      expect(telemetry).not.toContain("usr-sensitive-7f4f")
    }
    finally { write.mockRestore() }
  })
})

function repository(overrides: Partial<Repository>): Repository {
  return {
    createModelCreditHold: async () => ({ id: "aihold_1", attempt: 1, maxOutputTokens: 10 }),
    recordReportedModelUsage: async () => ({ reconciliationRequired: false }),
    markModelUsageUnavailable: async () => undefined,
    releaseModelCreditHold: async () => undefined,
    ...overrides,
  } as unknown as Repository
}

function modelProvider(overrides: Partial<ModelProvider>): ModelProvider {
  return {
    capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
    health: async () => ({ ok: true }),
    complete: async () => ({ text: "", usage: reported }),
    async *stream() { yield { type: "completed", usage: reported } },
    ...overrides,
  }
}
