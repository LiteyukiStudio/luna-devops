import { describe, expect, it } from "vitest"
import { ModelRuntime } from "../src/model-runtime.js"
import type { ModelProvider } from "../src/provider/provider.js"
import { InMemoryRunStreamBus } from "../src/run-stream-bus.js"
import { testRegistry } from "./support/model-tool-registry.js"
import { testExecutor } from "./support/run-executor.js"
import { buildTestServer } from "./support/server.js"
import { TestRepository } from "./support/test-repository.js"

const reportedUsage = { status: "reported" as const, value: { inputTokens: 1, outputTokens: 0, totalTokens: 1 } }

function streamingUntilAbort(deltas: string[]) {
  let notifyStreaming!: () => void
  const streaming = new Promise<void>(resolve => { notifyStreaming = resolve })
  const provider: ModelProvider = {
    async *stream(request) {
      for (const delta of deltas) yield { type: "message_delta", delta }
      notifyStreaming()
      await new Promise<void>((_resolve, reject) => {
        const signal = request.signal
        const abort = () => reject(signal?.reason instanceof Error ? signal.reason : new Error("ai.run_canceled"))
        if (signal?.aborted) abort()
        signal?.addEventListener("abort", abort, { once: true })
      })
    },
    async complete() { return { text: "", usage: reportedUsage, toolCalls: [] } },
    capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
    health: async () => ({ ok: true }),
  }
  return { provider, streaming }
}

describe("流式输出中途取消 Run", () => {
  it("取消接口产生的 run.canceled 序号大于客户端已收到的实时事件，且部分正文被保留", async () => {
    const owner = "usr_cancel_sequence"
    const repository = new TestRepository()
    const bus = new InMemoryRunStreamBus(repository)
    const { provider, streaming } = streamingUntilAbort(["第一段", "第二段", "第三段"])
    const executor = testExecutor(repository, new ModelRuntime(provider, testRegistry()), { streamBus: bus })
    const app = buildTestServer({ repository, streamBus: bus, cancelRun: runId => executor.cancel(runId), cancelExecutingRun: runId => executor.cancelExecuting(runId) })
    const conversation = await repository.createConversation(owner, "取消序号")
    const created = await repository.createTurn(owner, {
      conversationId: conversation.id,
      input: "写一段长文",
      pageContext: {},
      idempotencyKey: "cancel-sequence",
      actorSessionId: "ses_cancel_sequence",
    })
    const runId = created.run.id

    const executing = executor.runOnce()
    await streaming
    // 客户端此时已经通过实时总线收到的最大序号。
    const deliveredBeforeCancel = Math.max(...(await bus.read(runId, 0, 10_000)).map(event => event.sequence))

    const canceled = await app.inject({
      method: "POST",
      url: `/internal/v1/runs/${runId}/cancel`,
      headers: { "x-luna-dev-user": owner },
    })
    await executing

    expect(canceled.statusCode).toBe(202)
    expect((await repository.getRun(owner, runId))?.status).toBe("canceled")
    const durable = await repository.getEvents(owner, runId, 0)
    const runCanceled = durable.filter(event => event.type === "run.canceled")
    expect(runCanceled).toHaveLength(1)
    // 客户端按序号去重：不大于已收到序号的终态事件会被当作重复丢弃。
    expect(runCanceled[0]!.sequence).toBeGreaterThan(deliveredBeforeCancel)
    const sequences = durable.map(event => event.sequence)
    expect(new Set(sequences).size).toBe(sequences.length)
    const timeline = await repository.getTimeline(owner, conversation.id)
    expect(timeline?.turns[0]?.items).toContainEqual(expect.objectContaining({
      type: "progress",
      status: "failed",
      content: { parts: [{ type: "text", text: "第一段第二段第三段" }] },
    }))
    await app.close()
  })
})
