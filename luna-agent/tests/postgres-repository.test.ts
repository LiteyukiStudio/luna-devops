import { afterAll, beforeAll, describe, expect, it } from "vitest"
import type { CreatedTurn } from "../src/domain.js"
import { PostgresRepository } from "../src/persistence/postgres.js"
import { RunStateConflictError } from "../src/persistence/repository.js"

/**
 * 真实 PostgreSQL 集成测试：验证 Drizzle 持久层的事务、并发与状态机语义。
 * 仅在 AGENT_TEST_DATABASE_URL 指向专用临时库时运行（脚本开始会清空 ai schema 数据），
 * 默认跳过，不依赖 Mock。
 */
const databaseUrl = process.env.AGENT_TEST_DATABASE_URL
const suite = databaseUrl ? describe : describe.skip

suite("PostgresRepository (Drizzle) integration", () => {
  let repository: PostgresRepository
  const suffix = Date.now().toString(36)
  const owner = `usr_it_${suffix}`
  const key = (name: string) => `it-${name}-${suffix}`

  beforeAll(async () => {
    repository = new PostgresRepository(databaseUrl!)
    await repository.pool.query(`
      truncate ai.tool_approval_exemptions, ai.ui_actions, ai.tool_calls, ai.run_events, ai.items,
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

  it("delivers, expires and acknowledges UI actions idempotently", async () => {
    const conversation = await repository.createConversation(owner, "卡片")
    const created = await repository.createTurn(owner, {
      conversationId: conversation.id, input: "卡片", pageContext: {}, idempotencyKey: key("ui"), clientInstanceId: "client-1", actorSessionId: key("session"),
    })
    const expiresAt = new Date(Date.now() + 60_000).toISOString()
    const action = await repository.createUIAction(created.run.id, `aitool_${suffix}`, { kind: "navigate" }, expiresAt)
    expect(action.status).toBe("pending")
    const duplicated = await repository.createUIAction(created.run.id, `aitool_${suffix}`, { kind: "navigate" }, expiresAt)
    expect(duplicated.id).toBe(action.id)
    const pending = await repository.listPendingUIActions(owner, "client-1")
    expect(pending).toHaveLength(1)
    expect(pending[0]?.attempts).toBe(2)
    const acked = await repository.acknowledgeUIAction(owner, "client-1", action.id, { status: "succeeded", actualPath: "/apps" })
    expect(acked?.status).toBe("succeeded")
    expect(await repository.listPendingUIActions(owner, "client-1")).toHaveLength(0)
  })

  it("appends additional input to a queued run turn", async () => {
    const conversation = await repository.createConversation(owner, "补充输入")
    const created = await repository.createTurn(owner, { conversationId: conversation.id, input: "原始", pageContext: {}, idempotencyKey: key("append"), actorSessionId: key("session") })
    await repository.appendRunInput(created.run.id, "补充")
    expect((await repository.getExecutionInput(created.run.id))?.input).toBe("原始\n补充")
    await expect(repository.appendRunInput("airun_missing", "x")).rejects.toThrow("ai.run_not_found")
  })

  it("lists and revokes approval exemptions by owner and operation", async () => {
    const conversation = await repository.createConversation(owner, "审批豁免")
    const created = await repository.createTurn(owner, {
      conversationId: conversation.id,
      input: "restart",
      pageContext: {},
      idempotencyKey: key("approval-exemption"),
      actorSessionId: key("session"),
    })
    await repository.grantToolApprovalExemption(created.run.id, "restartRelease", "aitool_approval_exemption")
    const exemptions = await repository.listToolApprovalExemptions(owner)
    expect(exemptions).toHaveLength(1)
    expect(exemptions[0]?.operationId).toBe("restartRelease")
    expect(typeof exemptions[0]?.createdAt).toBe("string")
    expect(await repository.listToolApprovalExemptions("usr_other")).toEqual([])
    expect(await repository.revokeToolApprovalExemption("usr_other", "restartRelease")).toBe(false)
    expect(await repository.revokeToolApprovalExemption(owner, "restartRelease")).toBe(true)
    expect(await repository.listToolApprovalExemptions(owner)).toEqual([])
  })
})
