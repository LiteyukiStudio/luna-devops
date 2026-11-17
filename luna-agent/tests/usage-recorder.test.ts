import { describe, expect, it, vi } from "vitest"
import type { Repository } from "../src/persistence/repository.js"
import { UsageRecordingModelProvider } from "../src/provider/usage-recorder.js"
import type { ModelProvider } from "../src/provider/provider.js"

const reported = { status: "reported" as const, value: { inputTokens: 11, outputTokens: 7, totalTokens: 18 } }
const budget = { runId: "airun_test", ownerUserId: "usr_test", operation: "assistant" as const }
const pricing = {
  id: "aimod_test",
  name: "model-test",
  maxContextTokens: 1_000_000,
  maxOutputTokens: 37,
  inputCreditsPerMillion: "2",
  outputCreditsPerMillion: "8",
  cachedInputCreditsPerMillion: "0.5",
}

describe("UsageRecordingModelProvider", () => {
  it("clamps maxOutputTokens to model snapshot and persists reported usage without pre-locking wallet", async () => {
    const recordReportedModelUsage = vi.fn(async () => ({ id: "aiuse_1", attempt: 1 }))
    const complete = vi.fn(async () => ({ text: "ok", usage: reported, providerRequestId: "req_1", responseId: "chatcmpl_1" }))
    const provider = new UsageRecordingModelProvider(modelProvider({ complete }), repository({ recordReportedModelUsage }))

    const result = await provider.complete({
      messages: [{ role: "user", content: "hello" }],
      maxOutputTokens: 100,
      modelPricing: pricing,
      budget,
    })

    expect(complete).toHaveBeenCalledWith(expect.objectContaining({ maxOutputTokens: 37 }))
    expect(recordReportedModelUsage).toHaveBeenCalledWith(expect.objectContaining({
      ...budget,
      usage: reported.value,
      callType: "complete",
      model: pricing,
      providerRequestId: "req_1",
      responseId: "chatcmpl_1",
    }))
    expect(result.usageId).toBe("aiuse_1")
  })

  it("throws ai.provider_usage_unavailable immediately when complete or stream lacks reported usage", async () => {
    const recordReportedModelUsage = vi.fn(async () => ({ id: "aiuse_1", attempt: 1 }))
    const unavailableComplete = new UsageRecordingModelProvider(modelProvider({
      complete: async () => ({ text: "ok", usage: { status: "unavailable", reason: "missing_usage" } }),
    }), repository({ recordReportedModelUsage }))

    await expect(unavailableComplete.complete({ messages: [{ role: "user", content: "hello" }], maxOutputTokens: 10, budget }))
      .rejects.toThrow("ai.provider_usage_unavailable")
    expect(recordReportedModelUsage).not.toHaveBeenCalled()

    const unavailableStream = new UsageRecordingModelProvider(modelProvider({
      async *stream() {
        yield { type: "message_delta", delta: "partial" }
        yield { type: "completed", usage: { status: "unavailable", reason: "stream_ended_without_usage" } }
      },
    }), repository({ recordReportedModelUsage }))

    const consume = async () => {
      for await (const _event of unavailableStream.stream({ messages: [{ role: "user", content: "hello" }], maxOutputTokens: 10, budget })) void _event
    }
    await expect(consume()).rejects.toThrow("ai.provider_usage_unavailable")
    expect(recordReportedModelUsage).not.toHaveBeenCalled()
  })

  it("records every explicit model attempt independently", async () => {
    const recordReportedModelUsage = vi.fn()
      .mockResolvedValueOnce({ id: "aiuse_1", attempt: 1 })
      .mockResolvedValueOnce({ id: "aiuse_2", attempt: 2 })
    const provider = new UsageRecordingModelProvider(modelProvider({}), repository({ recordReportedModelUsage }))

    await provider.complete({ messages: [{ role: "user", content: "first" }], maxOutputTokens: 10, budget })
    await provider.complete({ messages: [{ role: "user", content: "second" }], maxOutputTokens: 10, budget })

    expect(recordReportedModelUsage).toHaveBeenCalledTimes(2)
  })
})

function repository(overrides: Partial<Pick<Repository, "recordReportedModelUsage">>): Pick<Repository, "recordReportedModelUsage"> {
  return {
    recordReportedModelUsage: async () => ({ id: "aiuse_1", attempt: 1 }),
    ...overrides,
  }
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
