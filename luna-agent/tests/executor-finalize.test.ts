import { trace } from "@opentelemetry/api"
import { describe, expect, it } from "vitest"
import { RunFinalizer } from "../src/executor/finalize.js"
import { RunLoop } from "../src/executor/run-loop.js"
import { ModelRuntime } from "../src/model-runtime.js"
import { RunStateConflictError } from "../src/persistence/repository.js"
import type { ModelProvider } from "../src/provider/provider.js"
import { InMemoryRunStreamBus } from "../src/run-stream-bus.js"
import { defaultRuntimeSettings } from "../src/runtime-settings.js"
import { DeterministicLunaApiClient } from "../src/tools/luna-api-client.js"
import { MemoryToolCallStore, ToolOrchestrator } from "../src/tools/orchestrator.js"
import { testRegistry } from "./support/model-tool-registry.js"
import { testRemoteConfigSource } from "./support/remote-config.js"
import { TestRepository } from "./support/test-repository.js"

const reportedUsage = { status: "reported" as const, value: { inputTokens: 1, outputTokens: 1, totalTokens: 2 } }

async function claimedRun(owner: string, key: string) {
  const repository = new TestRepository()
  const conversation = await repository.createConversation(owner, "新会话")
  await repository.createTurn(owner, {
    conversationId: conversation.id,
    input: "检查部署",
    pageContext: {},
    idempotencyKey: key,
    actorSessionId: `ses_${key}`,
  })
  const run = await repository.claimNextQueuedRun()
  if (!run) throw new Error("missing claimed run")
  const finalizer = new RunFinalizer(
    repository,
    new InMemoryRunStreamBus(repository),
    new ModelRuntime({
      async *stream() {},
      async complete() { return { text: "", toolCalls: [], usage: reportedUsage } },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }, testRegistry()),
  )
  return { repository, run, finalizer }
}

describe("run finalizer terminal races", () => {
  it("returns the actual terminal status and skips title work when completion loses to cancellation", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_finalize_race", "新会话")
    await repository.createTurn("usr_finalize_race", {
      conversationId: conversation.id,
      input: "检查部署",
      pageContext: {},
      idempotencyKey: "finalize-race",
      actorSessionId: "ses_finalize_race",
    })
    const run = await repository.claimNextQueuedRun()
    if (!run) throw new Error("missing claimed run")
    const executionInput = await repository.getExecutionInput(run.id)
    if (!executionInput) throw new Error("missing execution input")
    let titleCalls = 0
    const provider: ModelProvider = {
      async *stream() {},
      async complete() {
        titleCalls += 1
        return { text: "不应生成", toolCalls: [], usage: reportedUsage }
      },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }
    repository.finalizeStreamingItems = async () => {
      throw new RunStateConflictError(run.id, "running", "completed", "canceled")
    }
    const finalizer = new RunFinalizer(
      repository,
      new InMemoryRunStreamBus(repository),
      new ModelRuntime(provider, testRegistry()),
    )
    const span = trace.getTracer("test").startSpan("finalize")
    const outcome = await finalizer.complete({
      run,
      executionInput,
      finalAnswer: "完成",
      recoveredFinal: false,
      assistantRenamed: false,
      signal: new AbortController().signal,
      span,
    }).finally(() => span.end())

    expect(outcome).toBe("canceled")
    expect(titleCalls).toBe(0)
    expect((await repository.getEvents("usr_finalize_race", run.id, 0)).map(event => event.type))
      .not.toContain("conversation.title.updated")
    expect((await repository.getEvents("usr_finalize_race", run.id, 0)).map(event => event.type))
      .not.toContain("run.postprocess.completed")
  })

  it("returns the actual terminal status when failure loses to cancellation", async () => {
    const { repository, run, finalizer } = await claimedRun("usr_finalize_fail_race", "fail-race")
    repository.finalizeStreamingItems = async () => {
      throw new RunStateConflictError(run.id, "running", "failed", "canceled")
    }

    await expect(finalizer.fail(run, new Error("ai.provider_unavailable"))).resolves.toBe("canceled")
  })

  it("returns the actual terminal status when cancellation loses to completion", async () => {
    const { repository, run, finalizer } = await claimedRun("usr_finalize_cancel_race", "cancel-race")
    repository.finalizeStreamingItems = async () => {
      throw new RunStateConflictError(run.id, "running", "canceled", "completed")
    }

    await expect(finalizer.cancel(run, new Error("ai.run_canceled"))).resolves.toBe("completed")
  })

  it("returns the actual terminal status when interruption loses to failure", async () => {
    const { repository, run, finalizer } = await claimedRun("usr_finalize_interrupt_race", "interrupt-race")
    repository.finalizeStreamingItems = async () => {
      throw new RunStateConflictError(run.id, "running", "interrupted", "failed")
    }

    await expect(finalizer.interrupt(run)).resolves.toBe("failed")
  })

  it("returns canceled from RunLoop when the model stream is canceled", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_run_loop_cancel", "cancel")
    await repository.createTurn("usr_run_loop_cancel", {
      conversationId: conversation.id,
      input: "检查部署",
      pageContext: {},
      idempotencyKey: "run-loop-cancel",
      actorSessionId: "ses_run_loop_cancel",
    })
    const run = await repository.claimNextQueuedRun()
    if (!run) throw new Error("missing claimed run")
    const remote = testRemoteConfigSource()
    const providerConfig = remote.source.current()
    const catalog = remote.source.currentCatalog()
    if (!providerConfig || !catalog) throw new Error("missing test remote config")
    const modelRuntime = new ModelRuntime({
      async *stream() {
        yield { type: "message_delta", delta: "" }
        throw new Error("ai.run_canceled")
      },
      async complete() { return { text: "", toolCalls: [], usage: reportedUsage } },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }, testRegistry())
    const runLoop = new RunLoop(
      repository,
      modelRuntime,
      new ToolOrchestrator(catalog, new DeterministicLunaApiClient(() => ({ status: 200, body: {} })), new MemoryToolCallStore()),
      new InMemoryRunStreamBus(repository),
    )
    const span = trace.getTracer("test").startSpan("run-loop-cancel")
    const outcome = await runLoop.execute({
      run,
      runtimeSettings: defaultRuntimeSettings,
      providerConfig,
      toolCatalogDigest: catalog.digest,
      signal: new AbortController().signal,
      span,
    }).finally(() => span.end())

    expect(outcome).toBe("canceled")
    expect(await repository.getRun("usr_run_loop_cancel", run.id)).toMatchObject({ status: "canceled" })
  })
})
