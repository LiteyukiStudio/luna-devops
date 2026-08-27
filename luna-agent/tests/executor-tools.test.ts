import { afterEach, describe, expect, it, vi } from "vitest"
import { loadConfig } from "../src/config.js"
import { RunExecutor } from "../src/executor.js"
import { ModelRuntime } from "../src/model-runtime.js"
import { TestRepository } from "./support/test-repository.js"
import { DeterministicProvider } from "../src/provider/deterministic.js"
import type { ModelProvider } from "../src/provider/provider.js"
import { ToolCatalog } from "../src/tools/catalog.js"
import { DeterministicLunaApiClient } from "../src/tools/luna-api-client.js"
import { MemoryToolCallStore, ProjectingToolCallStore, ToolOrchestrator } from "../src/tools/orchestrator.js"
import { getToolDetailsTool } from "../src/tools/tool-details.js"
import { searchToolsTool } from "../src/tools/tool-search.js"
import { InMemoryRunStreamBus } from "../src/run-stream-bus.js"
import { defaultRuntimeSettings, type RemoteRuntimeSettings } from "../src/runtime-settings.js"
import { ManagedProvider } from "../src/provider/managed.js"
import type { RemoteConfigSnapshot, RemoteProviderConfig } from "../src/provider/config-client.js"

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe("RunExecutor slim lifecycle", () => {
  it("contains a polling claim failure instead of creating an unhandled rejection", async () => {
    class FailingClaimRepository extends TestRepository {
      override async claimNextQueuedRun(): Promise<never> { throw new Error("dependency.postgres.unavailable") }
    }
    const executor = new RunExecutor(
      new FailingClaimRepository(), new ModelRuntime(new DeterministicProvider()),
      loadConfig({ NODE_ENV: "test" }),
    )
    executor.start()
    await new Promise(resolve => setTimeout(resolve, 10))
    await expect(executor.stop()).resolves.toBeUndefined()
  })

  it("cleans up the live buffer after startup interrupts an orphaned Run", async () => {
    class CleanupBus extends InMemoryRunStreamBus {
      cleanupCalls = 0
      override async cleanup(runId: string) { this.cleanupCalls += 1; await super.cleanup(runId) }
    }
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_reconcile_cleanup", "stale")
    await repository.createTurn("usr_reconcile_cleanup", {
      conversationId: conversation.id, input: "hello", pageContext: {},
      idempotencyKey: "reconcile-cleanup", actorSessionId: "ses_reconcile_cleanup",
    })
    const run = await repository.claimNextQueuedRun()
    if (!run) throw new Error("missing run")
    run.startedAt = new Date(Date.now() - 60_000).toISOString()
    const streamBus = new CleanupBus(repository)
    const executor = new RunExecutor(
      repository, new ModelRuntime(new DeterministicProvider()),
      loadConfig({ NODE_ENV: "test" }),
      undefined, undefined, defaultRuntimeSettings, undefined, streamBus,
    )

    await expect((executor as unknown as { interruptOrphanedRuns(): Promise<boolean> }).interruptOrphanedRuns()).resolves.toBe(true)

    expect((await repository.getRun("usr_reconcile_cleanup", run.id))?.status).toBe("interrupted")
    expect(streamBus.cleanupCalls).toBe(1)
  })

  it("does not claim work until a failed PostgreSQL startup reconciliation succeeds", async () => {
    vi.useFakeTimers()
    class FlakyReconcileRepository extends TestRepository {
      reconcileAttempts = 0
      claimAttempts = 0

      override async listRunningRuns() {
        this.reconcileAttempts += 1
        if (this.reconcileAttempts === 1) throw new Error("database unavailable")
        return super.listRunningRuns()
      }

      override async claimNextQueuedRun() {
        this.claimAttempts += 1
        return super.claimNextQueuedRun()
      }
    }
    const repository = new FlakyReconcileRepository()
    const executor = new RunExecutor(
      repository, new ModelRuntime(new DeterministicProvider()),
      loadConfig({ NODE_ENV: "test" }),
    )

    executor.start()
    await vi.advanceTimersByTimeAsync(0)
    expect(repository.reconcileAttempts).toBe(1)
    expect(repository.claimAttempts).toBe(0)
    await vi.advanceTimersByTimeAsync(499)
    expect(repository.claimAttempts).toBe(0)
    await vi.advanceTimersByTimeAsync(1)
    expect(repository.reconcileAttempts).toBe(2)
    expect(repository.claimAttempts).toBe(1)
    await executor.stop()
  })

  it("retries a failed orphaned Run without skipping the rest of the reconciliation round", async () => {
    vi.useFakeTimers()
    const repository = new TestRepository()
    const firstConversation = await repository.createConversation("usr_reconcile_first", "first")
    const firstCreated = await repository.createTurn("usr_reconcile_first", {
      conversationId: firstConversation.id, input: "first", pageContext: {},
      idempotencyKey: "reconcile-first", actorSessionId: "ses_reconcile_first",
    })
    const firstRun = await repository.claimNextQueuedRun()
    if (!firstRun) throw new Error("missing first run")
    const secondConversation = await repository.createConversation("usr_reconcile_second", "second")
    const secondCreated = await repository.createTurn("usr_reconcile_second", {
      conversationId: secondConversation.id, input: "second", pageContext: {},
      idempotencyKey: "reconcile-second", actorSessionId: "ses_reconcile_second",
    })
    const secondRun = await repository.claimNextQueuedRun()
    if (!secondRun) throw new Error("missing second run")
    const interrupt = repository.interruptOrphanedRun.bind(repository)
    const attempts = new Map<string, number>()
    repository.interruptOrphanedRun = vi.fn(async (runId: string, rowVersion: number) => {
      const attempt = (attempts.get(runId) ?? 0) + 1
      attempts.set(runId, attempt)
      if (runId === firstCreated.run.id && attempt === 1) throw new Error("database unavailable")
      return interrupt(runId, rowVersion)
    })
    const claim = vi.spyOn(repository, "claimNextQueuedRun")
    const executor = new RunExecutor(
      repository, new ModelRuntime(new DeterministicProvider()),
      loadConfig({ NODE_ENV: "test" }),
    )

    executor.start()
    await vi.advanceTimersByTimeAsync(0)
    expect((await repository.getRun("usr_reconcile_first", firstCreated.run.id))?.status).toBe("running")
    expect((await repository.getRun("usr_reconcile_second", secondCreated.run.id))?.status).toBe("interrupted")
    expect(claim).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(500)
    expect((await repository.getRun("usr_reconcile_first", firstCreated.run.id))?.status).toBe("interrupted")
    expect(attempts.get(firstCreated.run.id)).toBe(2)
    expect(attempts.get(secondCreated.run.id)).toBe(1)
    expect(claim).toHaveBeenCalledOnce()
    await executor.stop()
  })

  it("keeps one immutable provider and runtime snapshot across a refresh between model steps", async () => {
    const oldConfig = remoteConfig("cfg-old", "https://old-provider.example/v1/")
    const newConfig = remoteConfig("cfg-new", "https://new-provider.example/v1/")
    let current = oldConfig
    const listeners = new Set<(config: RemoteProviderConfig) => void>()
    const remoteSnapshot = {
      current: () => current,
      subscribe: (listener: (config: RemoteProviderConfig) => void) => {
        listeners.add(listener)
        return () => listeners.delete(listener)
      },
    } as unknown as RemoteConfigSnapshot
    const publish = (config: RemoteProviderConfig) => {
      current = config
      for (const listener of listeners) listener(config)
    }
    const observed: Array<{ runId: string, configVersion: string, baseUrl: string, maxOutputTokens: number }> = []
    const modelAttempts = new Map<string, number>()
    let firstRunId = ""
    let secondRunId = ""
    const managedProvider = new ManagedProvider(remoteSnapshot, config => ({
      async *stream(request) {
        const runId = request.budget?.runId ?? "unknown"
        const attempt = (modelAttempts.get(runId) ?? 0) + 1
        modelAttempts.set(runId, attempt)
        observed.push({ runId, configVersion: config.version, baseUrl: config.provider.baseUrl, maxOutputTokens: request.maxOutputTokens })
        const invalidCard = runId === firstRunId && attempt <= 2 || runId === secondRunId && attempt === 1
        if (invalidCard) {
          yield {
            type: "completed" as const,
            usage: { status: "reported" as const, value: { inputTokens: 1, outputTokens: 1, totalTokens: 2 } },
            toolCalls: [{ id: `card-${runId}-${attempt}`, operationId: "present_card", arguments: {} }],
          }
          return
        }
        yield { type: "message_delta" as const, delta: "done" }
        yield { type: "completed" as const, usage: { status: "reported" as const, value: { inputTokens: 1, outputTokens: 1, totalTokens: 2 } } }
      },
      async complete() {
        return { text: "done", usage: { status: "reported" as const, value: { inputTokens: 1, outputTokens: 1, totalTokens: 2 } } }
      },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }))
    let releaseSecondStep!: () => void
    const secondStepReleased = new Promise<void>(resolve => { releaseSecondStep = resolve })
    let notifySecondStep!: () => void
    const secondStepStarted = new Promise<void>(resolve => { notifySecondStep = resolve })
    let toolResolutionCount = 0
    const runtime = new ModelRuntime(managedProvider, async () => {
      toolResolutionCount += 1
      if (toolResolutionCount === 2) {
        notifySecondStep()
        await secondStepReleased
      }
      return []
    })
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_snapshot", "snapshot")
    const first = await repository.createTurn("usr_snapshot", {
      conversationId: conversation.id, input: "first", pageContext: {},
      idempotencyKey: "runtime-snapshot-first", actorSessionId: "ses_snapshot",
    })
    firstRunId = first.run.id
    const executor = new RunExecutor(
      repository,
      runtime,
      loadConfig({ NODE_ENV: "test" }),
      undefined,
      remoteSnapshot,
      { ...defaultRuntimeSettings, ...oldConfig.runtime },
    )

    const firstExecution = executor.runOnce()
    await secondStepStarted
    publish(newConfig)
    releaseSecondStep()
    await expect(firstExecution).resolves.toBe(true)

    const second = await repository.createTurn("usr_snapshot", {
      conversationId: conversation.id, input: "second", pageContext: {},
      idempotencyKey: "runtime-snapshot-second", actorSessionId: "ses_snapshot",
    })
    secondRunId = second.run.id
    await expect(executor.runOnce()).resolves.toBe(true)
    await executor.stop()

    expect(observed.filter(item => item.runId === firstRunId)).toEqual([
      { runId: firstRunId, configVersion: "cfg-old", baseUrl: "https://old-provider.example/v1/", maxOutputTokens: defaultRuntimeSettings.assistantMaxOutputTokens },
      { runId: firstRunId, configVersion: "cfg-old", baseUrl: "https://old-provider.example/v1/", maxOutputTokens: defaultRuntimeSettings.assistantMaxOutputTokens },
      { runId: firstRunId, configVersion: "cfg-old", baseUrl: "https://old-provider.example/v1/", maxOutputTokens: defaultRuntimeSettings.assistantMaxOutputTokens },
    ])
    expect(observed.filter(item => item.runId === secondRunId)).toEqual([
      { runId: secondRunId, configVersion: "cfg-new", baseUrl: "https://new-provider.example/v1/", maxOutputTokens: defaultRuntimeSettings.assistantMaxOutputTokens },
      { runId: secondRunId, configVersion: "cfg-new", baseUrl: "https://new-provider.example/v1/", maxOutputTokens: defaultRuntimeSettings.assistantMaxOutputTokens },
    ])
    const firstCard = (await repository.getExecutionInput(firstRunId))?.toolInteractions
      .find(item => item.content.operationId === "create_interaction_cards")
    const secondCard = (await repository.getExecutionInput(secondRunId))?.toolInteractions
      .find(item => item.content.operationId === "create_interaction_cards")
    expect(firstCard?.content.result).toMatchObject({ maxAttempts: 2 })
    expect(secondCard?.content.result).toMatchObject({ maxAttempts: 2 })
  })

  it("restores the first-claim configuration after approval while new Runs use the refresh", async () => {
    const oldConfig = remoteConfig("cfg-approval-old", "https://old-approval.example/v1/")
    const newConfig = remoteConfig("cfg-approval-new", "https://new-approval.example/v1/")
    let current = oldConfig
    const listeners = new Set<(config: RemoteProviderConfig) => void>()
    const remoteSnapshot = {
      current: () => current,
      subscribe: (listener: (config: RemoteProviderConfig) => void) => {
        listeners.add(listener)
        return () => listeners.delete(listener)
      },
    } as unknown as RemoteConfigSnapshot
    const publish = (config: RemoteProviderConfig) => {
      current = config
      for (const listener of listeners) listener(config)
    }
    const catalog = ToolCatalog.load([{
      operationId: "restartRelease",
      name: "重启发布",
      summary: "重启指定发布。",
      method: "POST",
      path: "/api/v1/releases/{releaseId}/restart",
      category: "deployment",
      requiredScopes: ["deployment:write"],
      requiresApproval: true,
      idempotent: true,
      parameters: [{ inputName: "releaseId", wireName: "releaseId", in: "path", required: true }],
      inputSchema: { type: "object", properties: { releaseId: { type: "string" } }, required: ["releaseId"], additionalProperties: false },
    }])
    const observed: Array<{ runId: string, configVersion: string, maxOutputTokens: number }> = []
    const attempts = new Map<string, number>()
    let approvalRunId = ""
    const provider = new ManagedProvider(remoteSnapshot, config => ({
      async *stream(request) {
        const runId = request.budget?.runId ?? "unknown"
        const attempt = (attempts.get(runId) ?? 0) + 1
        attempts.set(runId, attempt)
        observed.push({ runId, configVersion: config.version, maxOutputTokens: request.maxOutputTokens })
        if (runId === approvalRunId && attempt === 1) {
          yield {
            type: "completed" as const,
            usage: { status: "reported" as const, value: { inputTokens: 4, outputTokens: 2, totalTokens: 6 } },
            toolCalls: [{ id: "restart-snapshot", operationId: "restartRelease", arguments: { releaseId: "rel_snapshot" } }],
          }
          return
        }
        yield { type: "message_delta" as const, delta: "done" }
        yield { type: "completed" as const, usage: { status: "reported" as const, value: { inputTokens: 3, outputTokens: 1, totalTokens: 4 } } }
      },
      async complete() {
        return { text: "done", usage: { status: "reported" as const, value: { inputTokens: 1, outputTokens: 1, totalTokens: 2 } } }
      },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }))
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_claim_snapshot", "approval snapshot")
    const approvalTurn = await repository.createTurn("usr_claim_snapshot", {
      conversationId: conversation.id, input: "restart", pageContext: {},
      idempotencyKey: "claim-snapshot-approval", actorSessionId: "ses_claim_snapshot",
    })
    approvalRunId = approvalTurn.run.id
    const store = new MemoryToolCallStore()
    const tools = new ToolOrchestrator(
      catalog,
      new DeterministicLunaApiClient(() => ({ status: 200, body: { restarted: true } })),
      new ProjectingToolCallStore(store, repository),
    )
    const runLimit = vi.spyOn(tools, "setRunMaxToolCallsForRun")
    const executor = new RunExecutor(
      repository,
      new ModelRuntime(provider, catalog.modelTools(["restartRelease"])),
      loadConfig({ NODE_ENV: "test" }),
      tools,
      remoteSnapshot,
      { ...defaultRuntimeSettings, ...oldConfig.runtime },
    )

    await expect(executor.runOnce()).resolves.toBe(true)
    expect((await repository.getRun("usr_claim_snapshot", approvalRunId))?.status).toBe("waiting_approval")
    expect(await repository.getRun("usr_claim_snapshot", approvalRunId)).not.toHaveProperty("executionSnapshot")
    publish(newConfig)
    const pending = [...store.records.values()][0]!
    await tools.resolveApproval(pending.id, "approve")
    await repository.updateRun(approvalRunId, "waiting_approval", "queued")
    await expect(executor.runOnce()).resolves.toBe(true)

    const nextTurn = await repository.createTurn("usr_claim_snapshot", {
      conversationId: conversation.id, input: "next", pageContext: {},
      idempotencyKey: "claim-snapshot-next", actorSessionId: "ses_claim_snapshot",
    })
    await expect(executor.runOnce()).resolves.toBe(true)
    await executor.stop()

    expect(observed.filter(item => item.runId === approvalRunId)).toEqual([
      { runId: approvalRunId, configVersion: "cfg-approval-old", maxOutputTokens: defaultRuntimeSettings.assistantMaxOutputTokens },
      { runId: approvalRunId, configVersion: "cfg-approval-old", maxOutputTokens: defaultRuntimeSettings.assistantMaxOutputTokens },
    ])
    expect(observed.filter(item => item.runId === nextTurn.run.id)).toEqual([
      { runId: nextTurn.run.id, configVersion: "cfg-approval-new", maxOutputTokens: defaultRuntimeSettings.assistantMaxOutputTokens },
    ])
    expect(runLimit.mock.calls).toEqual([
      [approvalRunId, defaultRuntimeSettings.runMaxToolCalls],
      [approvalRunId, defaultRuntimeSettings.runMaxToolCalls],
      [nextTurn.run.id, defaultRuntimeSettings.runMaxToolCalls],
    ])
  })

  it("atomically claims and completes a queued Run without a lease heartbeat", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_a", "hello")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id,
      input: "请简要回复",
      pageContext: {},
      idempotencyKey: "run-complete",
      actorSessionId: "ses_a",
    })
    const executor = new RunExecutor(
      repository,
      new ModelRuntime(new DeterministicProvider()),
      loadConfig({ NODE_ENV: "test" }),
    )

    expect(await executor.runOnce()).toBe(true)
    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("completed")
    expect(await executor.runOnce()).toBe(false)
  })

  it("interrupts a Run when terminal persistence exhausts its bounded retries", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_terminal_failure", "terminal")
    const created = await repository.createTurn("usr_terminal_failure", {
      conversationId: conversation.id,
      input: "complete",
      pageContext: {},
      idempotencyKey: "terminal-persistence-failure",
    })
    repository.persistRunStreamBatch = async () => { throw new Error("database unavailable") }
    const executor = new RunExecutor(
      repository,
      new ModelRuntime(new DeterministicProvider()),
      loadConfig({ NODE_ENV: "test" }),
    )

    expect(await executor.runOnce()).toBe(true)
    expect(await repository.getRun("usr_terminal_failure", created.run.id)).toMatchObject({
      status: "interrupted",
      errorCode: "ai.agent_restarted",
    })
  })

  it("publishes normalized official usage fields in model.completed", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_usage_event", "usage")
    const created = await repository.createTurn("usr_usage_event", {
      conversationId: conversation.id,
      input: "usage",
      pageContext: {},
      idempotencyKey: "usage-event",
      actorSessionId: "ses_usage_event",
    })
    const usage = {
      inputTokens: 120,
      outputTokens: 30,
      totalTokens: 150,
      cacheReadInputTokens: 70,
      cacheWriteInputTokens: 10,
      reasoningOutputTokens: 20,
    }
    const provider: ModelProvider = {
      async *stream() {
        yield { type: "message_delta", delta: "done" }
        yield { type: "completed", usage: { status: "reported", value: usage } }
      },
      async complete() { return { text: "done", usage: { status: "reported", value: usage } } },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }

    await new RunExecutor(repository, new ModelRuntime(provider), loadConfig({ NODE_ENV: "test" })).runOnce()

    const completed = (await repository.getEvents("usr_usage_event", created.run.id, 0))
      .find(event => event.type === "model.completed")
    expect(completed?.data.usage).toEqual({ status: "reported", ...usage })
    expect(completed?.data.usage).not.toHaveProperty("promptTokens")
    expect(completed?.data.usage).not.toHaveProperty("completionTokens")
  })

  it("marks an in-flight Run interrupted when the Agent stops", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_a", "stop")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id,
      input: "等待",
      pageContext: {},
      idempotencyKey: "run-stop",
      actorSessionId: "ses_a",
    })
    let notifyStarted!: () => void
    const started = new Promise<void>(resolve => { notifyStarted = resolve })
    const provider: ModelProvider = {
      async *stream(request) {
        notifyStarted()
        await new Promise<void>((_resolve, reject) => {
          const signal = request.signal
          const abortError = () => signal?.reason instanceof Error ? signal.reason : new Error("ai.run_canceled")
          if (signal?.aborted) reject(abortError())
          signal?.addEventListener("abort", () => reject(abortError()), { once: true })
        })
        yield { type: "completed", usage: { status: "reported" as const, value: { inputTokens: 0, outputTokens: 0, totalTokens: 0 + 0 } } }
      },
      async complete() { return { text: "", usage: { status: "reported" as const, value: { inputTokens: 1, outputTokens: 0, totalTokens: 1 + 0 } }, toolCalls: [] } },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }
    const executor = new RunExecutor(repository, new ModelRuntime(provider), loadConfig({ NODE_ENV: "test" }))
    const executing = executor.runOnce()
    await started
    await executor.stop()
    await executing

    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("interrupted")
  })

  it("flushes the partial item before a local cancellation becomes durable", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_cancel", "cancel")
    const created = await repository.createTurn("usr_cancel", {
      conversationId: conversation.id, input: "等待", pageContext: {},
      idempotencyKey: "run-cross-cancel", actorSessionId: "ses_cancel",
    })
    let notifyDelta!: () => void
    const deltaPublished = new Promise<void>(resolve => { notifyDelta = resolve })
    const provider: ModelProvider = {
      async *stream(request) {
        yield { type: "message_delta", delta: "partial" }
        notifyDelta()
        await new Promise<void>((_resolve, reject) => {
          const signal = request.signal
          const abort = () => reject(signal?.reason instanceof Error ? signal.reason : new Error("ai.run_canceled"))
          if (signal?.aborted) abort()
          signal?.addEventListener("abort", abort, { once: true })
        })
      },
      async complete() { return { text: "", usage: { status: "reported" as const, value: { inputTokens: 1, outputTokens: 0, totalTokens: 1 } }, toolCalls: [] } },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }
    const bus = new InMemoryRunStreamBus(repository)
    const executor = new RunExecutor(
      repository, new ModelRuntime(provider), loadConfig({ NODE_ENV: "test" }),
      undefined, undefined, defaultRuntimeSettings, undefined, bus,
    )
    const executing = executor.runOnce()
    await deltaPublished
    expect(await executor.cancel(created.run.id)).toBe(true)
    await executing

    expect((await repository.getRun("usr_cancel", created.run.id))?.status).toBe("canceled")
    const timeline = await repository.getTimeline("usr_cancel", conversation.id)
    expect(timeline?.turns[0]?.items).toContainEqual(expect.objectContaining({
      type: "assistant_message", status: "failed", content: { parts: [{ type: "text", text: "partial" }] },
    }))
    const events = await repository.getEvents("usr_cancel", created.run.id, 0)
    expect(events.at(-1)).toMatchObject({ type: "run.canceled" })
    expect(events.every((event, index) => index === 0 || event.sequence > events[index - 1]!.sequence)).toBe(true)
  })

  it("aborts an in-flight platform tool when the Run is canceled", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_tool_cancel", "cancel tool")
    const created = await repository.createTurn("usr_tool_cancel", {
      conversationId: conversation.id,
      input: "读取项目空间",
      pageContext: {},
      idempotencyKey: "run-tool-cancel",
      actorSessionId: "ses_tool_cancel",
    })
    const catalog = ToolCatalog.load([{
      operationId: "getProject",
      name: "读取项目空间",
      summary: "读取指定项目空间。",
      method: "GET",
      path: "/api/v1/projects/{projectId}",
      category: "project",
      requiredScopes: ["project:read"],
      requiresApproval: false,
      idempotent: true,
      parameters: [{ inputName: "projectId", wireName: "projectId", in: "path", required: true }],
      inputSchema: { type: "object", properties: { projectId: { type: "string" } }, required: ["projectId"], additionalProperties: false },
    }])
    let modelStep = 0
    const provider: ModelProvider = {
      async *stream(request) {
        if (modelStep++ === 0) {
          yield {
            type: "completed",
            usage: { status: "reported" as const, value: { inputTokens: 2, outputTokens: 1, totalTokens: 3 } },
            toolCalls: [{ id: "get-project", operationId: "getProject", arguments: { projectId: "prj_cancel" } }],
          }
          return
        }
        request.signal?.throwIfAborted()
        yield { type: "completed", usage: { status: "reported" as const, value: { inputTokens: 1, outputTokens: 0, totalTokens: 1 } } }
      },
      async complete() { return { text: "", usage: { status: "reported" as const, value: { inputTokens: 1, outputTokens: 0, totalTokens: 1 } } } },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }
    let notifyToolStarted!: () => void
    const toolStarted = new Promise<void>(resolve => { notifyToolStarted = resolve })
    let observedSignal: AbortSignal | undefined
    const client = new DeterministicLunaApiClient(request => new Promise((_resolve, reject) => {
      observedSignal = request.signal
      notifyToolStarted()
      const abort = () => reject(request.signal?.reason instanceof Error ? request.signal.reason : new Error("ai.run_canceled"))
      if (request.signal?.aborted) abort()
      request.signal?.addEventListener("abort", abort, { once: true })
    }))
    const store = new MemoryToolCallStore()
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(store, repository))
    const executor = new RunExecutor(
      repository,
      new ModelRuntime(provider, catalog.modelTools(["getProject"])),
      loadConfig({ NODE_ENV: "test" }),
      tools,
    )

    const executing = executor.runOnce()
    await toolStarted
    expect(await executor.cancel(created.run.id)).toBe(true)
    await executing

    expect(observedSignal?.aborted).toBe(true)
    expect((await repository.getRun("usr_tool_cancel", created.run.id))?.status).toBe("canceled")
    expect([...store.records.values()][0]).toMatchObject({ status: "failed", errorCode: "ai.run_canceled" })
  })

  it("resumes the same Run after one-call approval and preserves the Tool result", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_a", "restart")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id,
      input: "重启发布",
      pageContext: {},
      idempotencyKey: "run-approval",
      actorSessionId: "ses_a",
    })
    const catalog = ToolCatalog.load([{
      operationId: "restartRelease",
      name: "重启发布",
      summary: "重启指定发布。",
      method: "POST",
      path: "/api/v1/releases/{releaseId}/restart",
      category: "deployment",
      requiredScopes: ["deployment:write"],
      requiresApproval: true,
      idempotent: true,
      parameters: [{ inputName: "releaseId", wireName: "releaseId", in: "path", required: true }],
      inputSchema: { type: "object", properties: { releaseId: { type: "string" } }, required: ["releaseId"], additionalProperties: false },
    }])
    let step = 0
    const provider: ModelProvider = {
      async *stream() {
        if (step++ === 0) {
          yield { type: "completed", usage: { status: "reported" as const, value: { inputTokens: 4, outputTokens: 2, totalTokens: 4 + 2 } }, toolCalls: [{ id: "restart", operationId: "restartRelease", arguments: { releaseId: "rel_a" } }] }
          return
        }
        yield { type: "message_delta", delta: "发布已重启。" }
        yield { type: "completed", usage: { status: "reported" as const, value: { inputTokens: 6, outputTokens: 3, totalTokens: 6 + 3 } } }
      },
      async complete() { return { text: "", usage: { status: "reported" as const, value: { inputTokens: 1, outputTokens: 0, totalTokens: 1 + 0 } }, toolCalls: [] } },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }
    const store = new MemoryToolCallStore()
    class WaitingCleanupBus extends InMemoryRunStreamBus {
      cleanupCalls = 0
      override async cleanup(runId: string) { this.cleanupCalls += 1; await super.cleanup(runId) }
    }
    const streamBus = new WaitingCleanupBus(repository)
    const tools = new ToolOrchestrator(
      catalog,
      new DeterministicLunaApiClient(() => ({ status: 200, body: { restarted: true } })),
      new ProjectingToolCallStore(store, repository),
    )
    const executor = new RunExecutor(
      repository,
      new ModelRuntime(provider, catalog.modelTools(["restartRelease"])),
      loadConfig({ NODE_ENV: "test" }),
      tools,
      undefined,
      defaultRuntimeSettings,
      undefined,
      streamBus,
    )

    await executor.runOnce()
    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("waiting_approval")
    expect(streamBus.cleanupCalls).toBe(1)
    expect((await repository.getRunToolState(created.run.id))?.selectedOperationIds).toEqual(["restartRelease"])
    const pending = [...store.records.values()][0]!
    await tools.resolveApproval(pending.id, "approve")
    await repository.updateRun(created.run.id, "waiting_approval", "queued")
    await executor.runOnce()

    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("completed")
    expect(store.records.get(pending.id)).toMatchObject({ status: "succeeded", approvalDecision: "approve" })
    expect((await repository.getExecutionInput(created.run.id))?.toolInteractions.some(item => item.type === "tool_result")).toBe(true)
    const completedEvent = (await repository.getEvents("usr_a", created.run.id, 0))
      .find(event => event.type === "tool.completed")
    expect(completedEvent?.data.resultItem).toMatchObject({
      type: "tool_result",
      status: "completed",
      content: { relatedItemId: `${pending.id}:item`, result: { restarted: true } },
    })
  })

  it("keeps an exact platform tool available for the rest of the Run after get_tool_details", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_a", "details")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id,
      input: "查看项目空间",
      pageContext: {},
      idempotencyKey: "run-details",
      actorSessionId: "ses_a",
    })
    const catalog = ToolCatalog.load([{
      operationId: "getProject",
      name: "读取项目空间",
      summary: "读取指定项目空间。",
      method: "GET",
      path: "/api/v1/projects/{projectId}",
      category: "project",
      requiredScopes: ["project:read"],
      requiresApproval: false,
      idempotent: true,
      parameters: [{ inputName: "projectId", wireName: "projectId", in: "path", required: true }],
      inputSchema: { type: "object", properties: { projectId: { type: "string" } }, required: ["projectId"], additionalProperties: false },
    }])
    const observedTools: string[][] = []
    let step = 0
    const provider: ModelProvider = {
      async *stream(request) {
        observedTools.push((request.tools ?? []).map(tool => tool.operationId))
        if (step++ === 0) {
          yield { type: "completed", usage: { status: "reported" as const, value: { inputTokens: 2, outputTokens: 1, totalTokens: 2 + 1 } }, toolCalls: [{ id: "details", operationId: "get_tool_details", arguments: { operationIds: ["getProject"] } }] }
          return
        }
        if (step === 2) {
          yield { type: "completed", usage: { status: "reported" as const, value: { inputTokens: 3, outputTokens: 1, totalTokens: 3 + 1 } }, toolCalls: [{ id: "get", operationId: "getProject", arguments: { projectId: "prj_a" } }] }
          return
        }
        yield { type: "message_delta", delta: "项目空间已读取。" }
        yield { type: "completed", usage: { status: "reported" as const, value: { inputTokens: 4, outputTokens: 2, totalTokens: 4 + 2 } } }
      },
      async complete() { return { text: "", usage: { status: "reported" as const, value: { inputTokens: 1, outputTokens: 0, totalTokens: 1 + 0 } }, toolCalls: [] } },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }
    const runtime = new ModelRuntime(provider, {
      resolve: (_pageContext, _userInput, operationIds) => [...catalog.modelTools(operationIds), getToolDetailsTool],
      search: input => ({
        ...catalog.search(input), loadedOperationIds: [], missingOperationIds: [], catalogDigest: catalog.digest,
        duplicate: false, cacheHit: false,
      }),
      details: operationIds => {
        const result = catalog.getDetails(operationIds)
        return {
          ...result, loadedOperationIds: result.items.map(item => item.operationId),
          alreadySelectedOperationIds: [], catalogDigest: catalog.digest, duplicate: false, cacheHit: false,
        }
      },
    })
    const tools = new ToolOrchestrator(
      catalog,
      new DeterministicLunaApiClient(() => ({ status: 200, body: { id: "prj_a" } })),
      new ProjectingToolCallStore(new MemoryToolCallStore(), repository),
    )
    const executor = new RunExecutor(repository, runtime, loadConfig({ NODE_ENV: "test" }), tools)

    await executor.runOnce()

    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("completed")
    expect(observedTools[0]).not.toContain("getProject")
    expect(observedTools[1]).toContain("getProject")
    expect(observedTools[2]).toContain("getProject")
    expect((await repository.getRunToolState(created.run.id))?.selectedOperationIds).toEqual(["getProject"])
  })

  it("auto-loads search candidates, keeps them visible, permits different arguments, and replays duplicate search results", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_a", "search")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id,
      input: "分别查看两个项目空间",
      pageContext: {},
      idempotencyKey: "run-search-autoload",
      actorSessionId: "ses_a",
    })
    const catalog = ToolCatalog.load([{
      operationId: "getProject",
      name: "读取项目空间",
      summary: "按 projectId 读取项目空间详情。",
      purpose: { zh: "读取单个项目空间详情。", en: "Get one project." },
      aliases: { zh: ["查看项目空间详情"], en: ["get project"] },
      avoidWhen: { zh: "需要列表时不要使用。", en: "Do not use for lists." },
      preconditions: { zh: ["提供 projectId"], en: ["Provide projectId"] },
      successEvidence: { zh: "返回项目空间。", en: "Returns the project." },
      method: "GET",
      path: "/api/v1/projects/{projectId}",
      category: "projects",
      tags: ["Projects"],
      requiredScopes: ["project:read"],
      requiresApproval: false,
      idempotent: true,
      parameters: [{ inputName: "projectId", wireName: "projectId", in: "path", required: true }],
      inputSchema: { type: "object", properties: { projectId: { type: "string" } }, required: ["projectId"], additionalProperties: false },
      outputSchema: { type: "object", properties: { id: { type: "string" } } },
    }])
    const observedTools: string[][] = []
    const executed: string[] = []
    let step = 0
    const provider: ModelProvider = {
      async *stream(request) {
        observedTools.push((request.tools ?? []).map(tool => tool.operationId))
        if (step === 0 || step === 3) {
          step += 1
          yield { type: "completed", usage: { status: "reported" as const, value: { inputTokens: 2, outputTokens: 1, totalTokens: 2 + 1 } }, toolCalls: [{ id: `search-${step}`, operationId: "search_tools", arguments: { query: "查看项目空间详情" } }] }
          return
        }
        if (step === 1 || step === 2) {
          const projectId = step === 1 ? "prj_a" : "prj_b"
          step += 1
          yield { type: "completed", usage: { status: "reported" as const, value: { inputTokens: 2, outputTokens: 1, totalTokens: 2 + 1 } }, toolCalls: [{ id: `get-${projectId}`, operationId: "getProject", arguments: { projectId } }] }
          return
        }
        yield { type: "message_delta", delta: "两个项目空间都已读取。" }
        yield { type: "completed", usage: { status: "reported" as const, value: { inputTokens: 2, outputTokens: 1, totalTokens: 2 + 1 } } }
      },
      async complete() { return { text: "", usage: { status: "reported" as const, value: { inputTokens: 1, outputTokens: 0, totalTokens: 1 + 0 } }, toolCalls: [] } },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }
    const runtime = new ModelRuntime(provider, {
      resolve: (_pageContext, _userInput, operationIds) => [...catalog.modelTools(operationIds), searchToolsTool],
      search: input => ({
        ...catalog.search(input), loadedOperationIds: [], missingOperationIds: [], catalogDigest: catalog.digest,
        duplicate: false, cacheHit: false,
      }),
      details: operationIds => {
        const result = catalog.semanticDetails(operationIds)
        return {
          ...result, loadedOperationIds: result.items.map(item => item.operationId), alreadySelectedOperationIds: [],
          catalogDigest: catalog.digest, duplicate: false, cacheHit: false,
        }
      },
    })
    const tools = new ToolOrchestrator(
      catalog,
      new DeterministicLunaApiClient(({ arguments: args }) => {
        executed.push(String(args.projectId))
        return { status: 200, body: { id: args.projectId } }
      }),
      new ProjectingToolCallStore(new MemoryToolCallStore(), repository),
    )

    await new RunExecutor(repository, runtime, loadConfig({ NODE_ENV: "test" }), tools).runOnce()

    expect(executed).toEqual(["prj_a", "prj_b"])
    expect(observedTools[0]).not.toContain("getProject")
    expect(observedTools.slice(1).every(tools => tools.includes("getProject"))).toBe(true)
    expect((await repository.getRunToolState(created.run.id))?.selectedOperationIds).toEqual(["getProject"])
    const searchRecords = (await repository.getExecutionInput(created.run.id))!.toolInteractions
      .filter(item => item.content.operationId === "search_tools")
    expect(searchRecords).toHaveLength(2)
    expect(searchRecords[0]?.content.result).toMatchObject({ loadedOperationIds: ["getProject"], duplicate: false, cacheHit: false })
    expect(searchRecords[1]?.content.result).toMatchObject({ loadedOperationIds: ["getProject"], duplicate: true, cacheHit: true })
    expect((searchRecords[1]?.content.result as { items: unknown[] }).items).toHaveLength(1)
  })

  it("replays repeated mixed-order detail requests and reports already selected tools", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_a", "details-cache")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: "确认项目工具", pageContext: {}, idempotencyKey: "details-cache-run", actorSessionId: "ses_a",
    })
    const catalog = ToolCatalog.load([{
      operationId: "getProject", name: "查看项目空间", summary: "读取项目空间", category: "projects", tags: ["Projects"],
      aliases: { zh: ["项目空间详情"], en: ["project details"] }, purpose: { zh: "读取项目空间。", en: "Get project." },
      avoidWhen: { zh: "", en: "" }, preconditions: { zh: [], en: [] }, successEvidence: { zh: "返回项目空间。", en: "Returns project." },
      method: "GET", path: "/api/v1/projects/{projectId}", requiredScopes: ["project:read"], requiresApproval: false, idempotent: true,
      parameters: [{ inputName: "projectId", wireName: "projectId", in: "path", required: true }],
      inputSchema: { type: "object", properties: { projectId: { type: "string" } }, required: ["projectId"], additionalProperties: false },
      outputSchema: { type: "object" },
    }])
    let step = 0
    const provider: ModelProvider = {
      async *stream() {
        if (step++ < 2) {
          yield {
            type: "completed", usage: { status: "reported" as const, value: { inputTokens: 1, outputTokens: 1, totalTokens: 1 + 1 } },
            toolCalls: [{ id: `details-${step}`, operationId: "get_tool_details", arguments: { operationIds: step === 1 ? ["getProject", "missingTool"] : ["missingTool", "getProject", "getProject"] } }],
          }
          return
        }
        yield { type: "message_delta", delta: "详情已确认。" }
        yield { type: "completed", usage: { status: "reported" as const, value: { inputTokens: 1, outputTokens: 1, totalTokens: 1 + 1 } } }
      },
      async complete() { return { text: "", usage: { status: "reported" as const, value: { inputTokens: 1, outputTokens: 0, totalTokens: 1 + 0 } }, toolCalls: [] } },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }), health: async () => ({ ok: true }),
    }
    const runtime = new ModelRuntime(provider, {
      resolve: (_page, _input, operationIds) => [...catalog.modelTools(operationIds), getToolDetailsTool],
      search: input => ({ ...catalog.search(input), loadedOperationIds: [], missingOperationIds: [], catalogDigest: catalog.digest, duplicate: false, cacheHit: false }),
      details: operationIds => {
        const result = catalog.semanticDetails(operationIds)
        return { ...result, loadedOperationIds: result.items.map(item => item.operationId), alreadySelectedOperationIds: [], catalogDigest: catalog.digest, duplicate: false, cacheHit: false }
      },
    })

    await new RunExecutor(repository, runtime, loadConfig({ NODE_ENV: "test" })).runOnce()

    const records = (await repository.getExecutionInput(created.run.id))!.toolInteractions.filter(item => item.content.operationId === "get_tool_details")
    expect(records).toHaveLength(2)
    expect(records[0]?.content.result).toMatchObject({ loadedOperationIds: ["getProject"], missingOperationIds: ["missingTool"], duplicate: false })
    expect(records[1]?.content.result).toMatchObject({ loadedOperationIds: ["getProject"], alreadySelectedOperationIds: ["getProject"], duplicate: true, cacheHit: true })
    expect((records[1]?.content.result as { items: unknown[] }).items).toHaveLength(1)
  })
})

function remoteConfig(
  version: string,
  baseUrl: string,
): RemoteProviderConfig {
  const runtime: RemoteRuntimeSettings = {
    providerTimeoutMs: defaultRuntimeSettings.providerTimeoutMs,
    maxRequestRetries: defaultRuntimeSettings.maxRequestRetries,
    runTimeoutMs: defaultRuntimeSettings.runTimeoutMs,
    agentConcurrentRuns: defaultRuntimeSettings.agentConcurrentRuns,
    userConcurrentRuns: defaultRuntimeSettings.userConcurrentRuns,
  }
  return {
    version,
    provider: {
      baseUrl,
      apiKey: `${version}-secret`,
      providerCompatibility: "openai",
      promptCacheKeyMode: "disabled",
      channelAffinityEnabled: false,
      configured: true,
      models: [{
        id: "aimod_snapshot",
        name: "snapshot-model",
        maxContextTokens: 32_000,
        maxOutputTokens: 8_000,
        inputCreditsPerMillion: "1",
        outputCreditsPerMillion: "2",
        cachedInputCreditsPerMillion: "0.5",
      }],
    },
    runtime,
    toolCatalog: [],
  }
}
