import { afterAll, beforeAll, describe, expect, it } from "vitest"
import type { CreatedTurn, RunExecutionSnapshot } from "../src/domain.js"
import { PayloadCipher } from "../src/payload-cipher.js"
import { PostgresRepository } from "../src/persistence/postgres.js"
import { RunStateConflictError } from "../src/persistence/repository.js"
import type { RemoteProviderConfig } from "../src/provider/config-client.js"
import { defaultRuntimeSettings, type RemoteRuntimeSettings } from "../src/runtime-settings.js"

/**
 * 真实 PostgreSQL 集成测试：验证 Drizzle 持久层的事务、并发与状态机语义。
 * 仅在 AGENT_TEST_DATABASE_URL 指向专用临时库时运行（脚本开始会清空 ai schema 数据），
 * 默认跳过，不依赖 Mock。
 */
const databaseUrl = process.env.AGENT_TEST_DATABASE_URL
const suite = databaseUrl ? describe : describe.skip
const runExecutionSnapshotKey = Buffer.alloc(32, 17)
const runExecutionSnapshotCipher = () => new PayloadCipher(
  runExecutionSnapshotKey,
  "run-execution-snapshot-v1",
  "ai.run_execution_snapshot_key_unavailable",
)

suite("PostgresRepository (Drizzle) integration", () => {
  let repository: PostgresRepository
  const suffix = Date.now().toString(36)
  const owner = `usr_it_${suffix}`
  const key = (name: string) => `it-${name}-${suffix}`

  beforeAll(async () => {
    repository = new PostgresRepository(databaseUrl!, {}, runExecutionSnapshotCipher())
    await repository.pool.query(`
      truncate ai.tool_calls, ai.run_events, ai.items,
               ai.idempotency_keys, ai.conversation_summaries, ai.runs, ai.turns, ai.conversations
      cascade
    `)
  })

  afterAll(async () => {
    await repository.close()
  })

  it("creates, queries, renames and paginates conversations", async () => {
    const c1 = await repository.createConversation(owner, "新会话", undefined, undefined, "aimod_fast")
    const c2 = await repository.createConversation(owner, "诊断构建", "prj_1", undefined, "aimod_deep")
    expect(c1.titleSource).toBe("default")
    expect(c2.titleSource).toBe("user")
    expect(c1.modelId).toBe("aimod_fast")
    expect(c2.modelId).toBe("aimod_deep")
    expect((await repository.findEmptyConversation(owner, "prj_1"))?.id).toBe(c2.id)
    const page = await repository.listConversations(owner, 1, 1)
    expect(page.total).toBe(2)
    expect(page.items).toHaveLength(1)
    const filtered = await repository.listConversations(owner, 1, 20, { search: "诊断", sortOrder: "asc" })
    expect(filtered.items.map(item => item.id)).toEqual([c2.id])
    expect(await repository.renameConversation(owner, c2.id, "手动标题")).toBeDefined()
    expect((await repository.updateConversation(owner, c1.id, { modelId: "aimod_balanced" }))?.modelId).toBe("aimod_balanced")
    expect((await repository.getConversation(owner, c2.id))?.modelId).toBe("aimod_deep")
    expect(await repository.renameConversationByAssistant(c2.id, "自动标题")).toBeUndefined()
    expect(await repository.getConversation("other", c2.id)).toBeUndefined()
  })

  it("persists a turn with idempotency and rejects conflicting reuse atomically", async () => {
    const conversation = await repository.createConversation(owner, "幂等")
    const request = { conversationId: conversation.id, input: "检查发布", pageContext: { pathname: "/x" }, idempotencyKey: key("idem"), actorSessionId: key("session") }
    const first = await repository.createTurn(owner, request)
    const second = await repository.createTurn(owner, request)
    expect(second.run.id).toBe(first.run.id)
    await expect(repository.createTurn(owner, { ...request, input: "不同内容" })).rejects.toThrow("idempotency_conflict")
    const timeline = await repository.getTimeline(owner, conversation.id)
    expect(timeline?.turns).toHaveLength(1)
    expect(timeline?.turns[0]?.items[0]?.type).toBe("user_message")
  })

  it("atomically transitions a queued Run to running only once under concurrency", async () => {
    while (await repository.claimNextQueuedRun()) { /* drain earlier queued fixtures */ }
    const conversation = await repository.createConversation(owner, "租约")
    const created = await repository.createTurn(owner, { conversationId: conversation.id, input: "竞争", pageContext: {}, idempotencyKey: key("lease"), actorSessionId: key("session") })
    const results = await Promise.all([
      repository.claimNextQueuedRun(),
      repository.claimNextQueuedRun(),
      repository.claimNextQueuedRun(),
    ])
    const claimed = results.filter(Boolean)
    expect(claimed).toHaveLength(1)
    expect(claimed[0]?.id).toBe(created.run.id)
    expect(claimed[0]?.status).toBe("running")
  })

  it("encrypts and restores the first-claim execution snapshot after a repository restart", async () => {
    while (await repository.claimNextQueuedRun()) { /* drain earlier queued fixtures */ }
    const conversation = await repository.createConversation(owner, "配置快照")
    const created = await repository.createTurn(owner, {
      conversationId: conversation.id, input: "等待批准", pageContext: {},
      idempotencyKey: key("execution-snapshot"), actorSessionId: key("session"),
    })
    const oldSnapshot = executionSnapshot("cfg-old", "old-provider-secret", 512, 2)
    const newSnapshot = executionSnapshot("cfg-new", "new-provider-secret", 1_024, 5)

    const firstClaim = await repository.claimNextQueuedRun(oldSnapshot)
    expect(firstClaim?.id).toBe(created.run.id)
    expect(firstClaim?.executionSnapshot?.providerConfig?.version).toBe("cfg-old")
    expect(firstClaim?.executionSnapshot?.runtimeSettings.assistantMaxOutputTokens).toBe(512)
    expect(await repository.getRun(owner, created.run.id)).not.toHaveProperty("executionSnapshot")
    const stored = await repository.pool.query<{ execution_snapshot_ciphertext: string }>(
      "select execution_snapshot_ciphertext from ai.runs where id = $1",
      [created.run.id],
    )
    expect(stored.rows[0]?.execution_snapshot_ciphertext).not.toContain("old-provider-secret")
    expect(stored.rows[0]?.execution_snapshot_ciphertext).not.toContain("https://cfg-old.example/v1/")

    await repository.updateRun(created.run.id, "running", "waiting_approval")
    await repository.updateRun(created.run.id, "waiting_approval", "queued")
    const restarted = new PostgresRepository(databaseUrl!, {}, runExecutionSnapshotCipher())
    try {
      const resumed = await restarted.claimNextQueuedRun(newSnapshot)
      expect(resumed?.id).toBe(created.run.id)
      expect(resumed?.executionSnapshot?.providerConfig?.version).toBe("cfg-old")
      expect(resumed?.executionSnapshot?.providerConfig?.provider.apiKey).toBe("old-provider-secret")
      expect(resumed?.executionSnapshot?.runtimeSettings.assistantMaxOutputTokens).toBe(512)
      expect(resumed?.executionSnapshot?.runtimeSettings.maxCardRepairAttempts).toBe(2)
      expect(resumed?.executionSnapshot?.runtimeSettings.runMaxToolCalls).toBe(32)
      if (!resumed) throw new Error("expected resumed Run")
      await restarted.updateRun(resumed.id, "running", "completed", { completedAt: new Date().toISOString() })

      const next = await restarted.createTurn(owner, {
        conversationId: conversation.id, input: "新任务", pageContext: {},
        idempotencyKey: key("execution-snapshot-next"), actorSessionId: key("session"),
      })
      const nextClaim = await restarted.claimNextQueuedRun(newSnapshot)
      expect(nextClaim?.id).toBe(next.run.id)
      expect(nextClaim?.executionSnapshot?.providerConfig?.version).toBe("cfg-new")
      expect(nextClaim?.executionSnapshot?.runtimeSettings.assistantMaxOutputTokens).toBe(1_024)
      expect(nextClaim?.executionSnapshot?.runtimeSettings.maxCardRepairAttempts).toBe(5)
      expect(nextClaim?.executionSnapshot?.runtimeSettings.runMaxToolCalls).toBe(64)
    }
    finally {
      await restarted.close()
    }
  })

  it("transitions run state atomically and reports authoritative actualStatus on conflict", async () => {
    const conversation = await repository.createConversation(owner, "状态机")
    const created = await repository.createTurn(owner, { conversationId: conversation.id, input: "状态", pageContext: {}, idempotencyKey: key("state"), actorSessionId: key("session") })
    const running = await repository.claimNextQueuedRun()
    expect(running?.id).toBe(created.run.id)
    if (!running) throw new Error("expected queued Run")
    expect(running.status).toBe("running")
    expect(running.rowVersion).toBe(2)
    const conflict = await repository.updateRun(created.run.id, "queued", "failed").catch((error: unknown) => error)
    expect(conflict).toBeInstanceOf(RunStateConflictError)
    expect((conflict as RunStateConflictError).expectedStatus).toBe("queued")
    expect((conflict as RunStateConflictError).targetStatus).toBe("failed")
    expect((conflict as RunStateConflictError).actualStatus).toBe("running")
  })

  it("keeps cancel authoritative when racing a completion write-back", async () => {
    const conversation = await repository.createConversation(owner, "取消竞态")
    const created = await repository.createTurn(owner, { conversationId: conversation.id, input: "取消我", pageContext: {}, idempotencyKey: key("cancel"), actorSessionId: key("session") })
    await repository.updateRun(created.run.id, "queued", "running", { startedAt: new Date().toISOString() })
    const canceled = await repository.cancelRun(owner, created.run.id)
    expect(canceled?.status).toBe("canceled")
    const conflict = await repository.updateRun(created.run.id, "running", "completed", { completedAt: new Date().toISOString() }).catch((error: unknown) => error)
    expect(conflict).toBeInstanceOf(RunStateConflictError)
    expect((conflict as RunStateConflictError).actualStatus).toBe("canceled")
    const events = await repository.getEvents(owner, created.run.id, 0)
    expect(events.map(event => event.type)).toContain("run.canceled")
    expect(events.map(event => event.type)).not.toContain("run.completed")
  })

  it("cancels pending ToolCalls and their timeline items in the Run transaction", async () => {
    const conversation = await repository.createConversation(owner, "审批取消")
    const created = await repository.createTurn(owner, {
      conversationId: conversation.id,
      input: "取消待审批操作",
      pageContext: {},
      idempotencyKey: key("cancel-approval"),
      actorSessionId: key("session"),
    })
    await repository.updateRun(created.run.id, "queued", "waiting_approval")
    const pendingId = `aitool_pending_${suffix}`
    const completedId = `aitool_completed_${suffix}`
    await repository.pool.query(`
      insert into ai.tool_calls (
        id, run_id, operation_id, status, input_mode, arguments, arguments_hash,
        attempt, row_version, result, error_code
      ) values
        ($1, $2, 'updateThing', 'awaiting_approval', 'model', '{}'::jsonb, 'sha256:pending', 1, 2, null, null),
        ($3, $2, 'readThing', 'succeeded', 'model', '{}'::jsonb, 'sha256:completed', 1, 4, '{"ok":true}'::jsonb, null)
    `, [pendingId, created.run.id, completedId])
    await repository.appendItem({
      id: `${pendingId}:item`,
      runId: created.run.id,
      turnId: created.turn.id,
      type: "tool_call",
      status: "streaming",
      content: { toolCallId: pendingId, operationId: "updateThing", status: "awaiting_approval" },
    })
    await repository.appendItem({
      id: `${completedId}:item`,
      runId: created.run.id,
      turnId: created.turn.id,
      type: "tool_call",
      status: "completed",
      content: { toolCallId: completedId, operationId: "readThing", status: "succeeded", result: { ok: true } },
    })

    const canceled = await repository.cancelRun(owner, created.run.id)

    expect(canceled?.status).toBe("canceled")
    const calls = await repository.pool.query<{ id: string, status: string, row_version: number, error_code: string | null, result: unknown }>(
      `select id, status, row_version, error_code, result from ai.tool_calls where id = any($1::text[]) order by id`,
      [[pendingId, completedId]],
    )
    expect(calls.rows.find(row => row.id === pendingId)).toMatchObject({
      status: "canceled",
      row_version: 3,
      error_code: "ai.run_canceled",
      result: { code: "ai.run_canceled", retryable: false },
    })
    expect(calls.rows.find(row => row.id === completedId)).toMatchObject({
      status: "succeeded",
      row_version: 4,
      error_code: null,
      result: { ok: true },
    })
    const timeline = await repository.getTimeline(owner, conversation.id)
    const pendingItem = timeline?.turns[0]?.items.find(item => item.id === `${pendingId}:item`)
    const completedItem = timeline?.turns[0]?.items.find(item => item.id === `${completedId}:item`)
    expect(pendingItem).toMatchObject({
      status: "completed",
      revision: 2,
      content: { status: "canceled", errorCode: "ai.run_canceled", result: { code: "ai.run_canceled", retryable: false } },
    })
    expect(completedItem).toMatchObject({ status: "completed", revision: 1, content: { status: "succeeded", result: { ok: true } } })
    const events = await repository.getEvents(owner, created.run.id, 0)
    expect(events.findIndex(event => event.type === "item.finalized")).toBeLessThan(events.findIndex(event => event.type === "run.canceled"))
  })

  it("rolls back the whole transaction when any step fails", async () => {
    const conversation = await repository.createConversation(owner, "回滚")
    const created = await repository.createTurn(owner, {
      conversationId: conversation.id, input: "x", pageContext: {}, idempotencyKey: key("rollback"), actorSessionId: key("session"),
    })
    // createTurn 在不属于该用户的会话上整体回滚：不产生 turn/run/幂等键/事件
    await expect(repository.createTurn("other-user", {
      conversationId: conversation.id, input: "x", pageContext: {}, idempotencyKey: key("rollback-2"), actorSessionId: key("session-other"),
    })).rejects.toThrow("ai.conversation_not_found")
    // 幂等冲突也在写入任何 turn/run 前抛出
    await expect(repository.createTurn(owner, {
      conversationId: conversation.id, input: "changed", pageContext: {}, idempotencyKey: key("rollback"), actorSessionId: key("session"),
    })).rejects.toThrow("idempotency_conflict")
    const timeline = await repository.getTimeline(owner, conversation.id)
    expect(timeline?.turns).toHaveLength(1)
    expect(timeline?.turns[0]?.id).toBe(created.turn.id)
  })

  it("keeps timeline and event sequences stable and roundtrips jsonb", async () => {
    const conversation = await repository.createConversation(owner, "顺序")
    const created = await repository.createTurn(owner, {
      conversationId: conversation.id, input: "顺序", pageContext: { nested: { a: [1, 2] } }, idempotencyKey: key("order"), actorSessionId: key("session"),
    })
    const first = await repository.appendItem({
      runId: created.run.id, turnId: created.turn.id, type: "assistant_message", status: "streaming",
      content: { parts: [{ type: "text", text: "p1" }] },
    })
    const second = await repository.appendItem({
      runId: created.run.id, turnId: created.turn.id, type: "tool_call", status: "streaming",
      content: { toolCallId: "t1" },
    })
    expect(second.timelineIndex).toBe(first.timelineIndex + 1)
    await repository.finalizeStreamingItems(created.run.id, "completed")
    const timeline = await repository.getTimeline(owner, conversation.id)
    const finalized = timeline?.turns[0]?.items.find(item => item.id === first.id)
    expect(finalized?.status).toBe("completed")
    expect(finalized?.revision).toBe(2)
    const events = await repository.getEvents(owner, created.run.id, 0)
    const sequences = events.map(event => event.sequence)
    expect([...sequences].sort((a, b) => a - b)).toEqual(sequences)
    expect(events.some(event => event.type === "item.finalized")).toBe(true)
    const executionInput = await repository.getExecutionInput(created.run.id)
    expect(executionInput?.pageContext).toEqual({ nested: { a: [1, 2] } })
    expect(executionInput?.toolInteractions).toHaveLength(1)

    const next = await repository.createTurn(owner, {
      conversationId: conversation.id, input: "继续", pageContext: {},
      idempotencyKey: key("order-next"), actorSessionId: key("session"),
    })
    const prior = (await repository.getExecutionInput(next.run.id))?.history.find(item => item.turnIndex === created.turn.turnIndex)
    expect(prior?.pageContext).toEqual({ nested: { a: [1, 2] } })
    expect(prior?.toolInteractions?.[0]).not.toHaveProperty("createdAt")
  })

  it("persists a sparse terminal stream batch and advances the authoritative high-watermark", async () => {
    while (await repository.claimNextQueuedRun()) { /* drain earlier queued fixtures */ }
    const conversation = await repository.createConversation(owner, "稀疏流终态")
    const created = await repository.createTurn(owner, {
      conversationId: conversation.id, input: "stream", pageContext: {},
      idempotencyKey: key("sparse-stream"), actorSessionId: key("session"),
    })
    const running = await repository.claimNextQueuedRun()
    expect(running?.id).toBe(created.run.id)
    const expected = (await repository.getRunStreamPosition(created.run.id))!
    const createdAt = new Date().toISOString()
    const batch = {
      runId: created.run.id,
      expected,
      eventHighWatermark: expected.nextEventSequence + 4,
      expectedRunVersion: expected.runVersion,
      items: [{
        id: `aiitm_sparse_${suffix}`, runId: created.run.id, turnId: created.turn.id,
        timelineIndex: expected.nextItemPosition, revision: 4, type: "assistant_message" as const,
        status: "completed" as const, content: { parts: [{ type: "text", text: "final" }] }, createdAt,
      }],
      events: [
        { id: `aievt_sparse_message_${suffix}`, runId: created.run.id, sequence: expected.nextEventSequence + 2, type: "message.completed", data: {}, createdAt },
        { id: `aievt_sparse_model_${suffix}`, runId: created.run.id, sequence: expected.nextEventSequence + 4, type: "model.completed", data: {}, createdAt },
      ],
    }
    await repository.persistRunStreamBatch(batch)
    await repository.persistRunStreamBatch(batch)
    const position = await repository.getRunStreamPosition(created.run.id)
    expect(position?.nextEventSequence).toBe(batch.eventHighWatermark + 1)
    const sparse = await repository.getEvents(owner, created.run.id, expected.nextEventSequence - 1)
    expect(sparse.map(event => event.sequence)).toEqual([
      expected.nextEventSequence + 2,
      expected.nextEventSequence + 4,
    ])
    const terminal = await repository.appendEvent(created.run.id, "run.completed", { state: "completed" })
    expect(terminal.sequence).toBe(batch.eventHighWatermark + 1)
    const timeline = await repository.getTimeline(owner, conversation.id)
    expect(timeline?.eventCursors).toEqual([{ runId: created.run.id, after: terminal.sequence }])
    expect(timeline?.turns[0]?.items.some(item => item.id === batch.items[0]!.id)).toBe(true)
  })

  it("pages complete timeline turns and keeps item snapshots aligned with event cursors", async () => {
    const conversation = await repository.createConversation(owner, "分页一致性")
    const createdTurns: CreatedTurn[] = []
    for (let turnIndex = 0; turnIndex < 13; turnIndex += 1) {
      createdTurns.push(await repository.createTurn(owner, {
        conversationId: conversation.id,
        input: `turn-${turnIndex}`,
        pageContext: {},
        idempotencyKey: key(`timeline-page-${turnIndex}`),
        actorSessionId: key("session"),
      }))
    }

    const latest = await repository.getTimeline(owner, conversation.id, { limit: 5 })
    expect(latest?.turns.map(turn => turn.turnIndex)).toEqual([8, 9, 10, 11, 12])
    expect(latest?.pageInfo).toEqual({ hasOlder: true, oldestTurnIndex: 8 })
    const older = await repository.getTimeline(owner, conversation.id, { beforeTurnIndex: 8, limit: 5 })
    expect(older?.turns.map(turn => turn.turnIndex)).toEqual([3, 4, 5, 6, 7])

    const active = createdTurns.at(-1)
    if (!active) throw new Error("expected a created turn")
    for (let attempt = 0; attempt < 8; attempt += 1) {
      const mutationPromise = repository.appendItemWithEvent({
        runId: active.run.id,
        turnId: active.turn.id,
        type: "assistant_message",
        status: "completed",
        content: { parts: [{ type: "text", text: `message-${attempt}` }] },
      }, "item.completed")
      const snapshotPromise = repository.getTimeline(owner, conversation.id, { limit: 1 })
      const [mutation, snapshot] = await Promise.all([mutationPromise, snapshotPromise])
      const cursor = snapshot?.eventCursors.find(item => item.runId === active.run.id)?.after ?? 0
      const containsItem = snapshot?.turns[0]?.items.some(item => item.id === mutation.item.id) ?? false
      if (cursor >= mutation.event.sequence) expect(containsItem).toBe(true)
    }
  })

  it("advances conversation summaries monotonically", async () => {
    const conversation = await repository.createConversation(owner, "摘要")
    const content = { userGoals: ["g"], constraints: [], confirmedResources: [], completedActions: [], failures: [], pendingWork: [], durableFacts: [] }
    const saved = await repository.saveConversationSummary({
      conversationId: conversation.id, coveredThroughTurnIndex: 1, compressionVersion: 1, sourceTurnCount: 2, content,
    })
    const stale = await repository.saveConversationSummary({
      conversationId: conversation.id, coveredThroughTurnIndex: 0, compressionVersion: 1, sourceTurnCount: 1, content,
    })
    expect(stale.coveredThroughTurnIndex).toBe(1)
    expect(stale.createdAt).toBe(saved.createdAt)
    expect((await repository.getConversationSummary(conversation.id))?.sourceTurnCount).toBe(2)
  })

  it("appends additional input to a queued run turn", async () => {
    const conversation = await repository.createConversation(owner, "补充输入")
    const created = await repository.createTurn(owner, { conversationId: conversation.id, input: "原始", pageContext: {}, idempotencyKey: key("append"), actorSessionId: key("session") })
    await repository.appendRunInput(created.run.id, "补充")
    expect((await repository.getExecutionInput(created.run.id))?.input).toBe("原始\n补充")
    await expect(repository.appendRunInput("airun_missing", "x")).rejects.toThrow("ai.run_not_found")
  })

})

function executionSnapshot(version: string, apiKey: string, assistantMaxOutputTokens: number, maxCardRepairAttempts: number): RunExecutionSnapshot {
  return {
    runtimeSettings: {
      ...defaultRuntimeSettings,
      assistantMaxOutputTokens,
      maxCardRepairAttempts,
      runMaxToolCalls: maxCardRepairAttempts === 2 ? 32 : 64,
    },
    providerConfig: remoteProviderConfig(version, apiKey, assistantMaxOutputTokens, maxCardRepairAttempts),
  }
}

function remoteProviderConfig(version: string, apiKey: string, assistantMaxOutputTokens: number, maxCardRepairAttempts: number): RemoteProviderConfig {
  const runtime: RemoteRuntimeSettings = {
    providerTimeoutMs: defaultRuntimeSettings.providerTimeoutMs,
    maxRequestRetries: defaultRuntimeSettings.maxRequestRetries,
    runTimeoutMs: defaultRuntimeSettings.runTimeoutMs,
    agentConcurrentRuns: defaultRuntimeSettings.agentConcurrentRuns,
    userConcurrentRuns: defaultRuntimeSettings.userConcurrentRuns,
    assistantMaxOutputTokens,
    maxModelSteps: defaultRuntimeSettings.maxModelSteps,
    runMaxToolCalls: maxCardRepairAttempts === 2 ? 32 : 64,
    maxInputBytes: defaultRuntimeSettings.maxInputBytes,
    maxCardRepairAttempts,
    contextMaxUncompressedTurnCount: defaultRuntimeSettings.contextMaxUncompressedTurnCount,
    contextMaxCompressionTurnsPerCompile: defaultRuntimeSettings.contextMaxCompressionTurnsPerCompile,
    contextSummaryMaxOutputTokens: defaultRuntimeSettings.contextSummaryMaxOutputTokens,
  }
  return {
    version,
    provider: {
      baseUrl: `https://${version}.example/v1/`,
      apiKey,
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
