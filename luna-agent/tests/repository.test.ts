import { afterEach, describe, expect, it, vi } from "vitest"
import { TestRepository } from "./support/test-repository.js"

describe("conversation repository", () => {
  afterEach(() => vi.useRealTimers())

  it("keeps the preferred model on the conversation and updates it with each explicit turn model", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_models", "models", undefined, "user", "aimod_fast")
    expect(conversation.modelId).toBe("aimod_fast")

    await repository.updateConversation("usr_models", conversation.id, { modelId: "aimod_deep" })
    expect((await repository.getConversation("usr_models", conversation.id))?.modelId).toBe("aimod_deep")

    await repository.createTurn("usr_models", {
      conversationId: conversation.id,
      input: "use balanced",
      pageContext: {},
      idempotencyKey: "model-preference-turn",
      modelId: "aimod_balanced",
    })
    expect((await repository.getConversation("usr_models", conversation.id))?.modelId).toBe("aimod_balanced")
  })

  it("updates conversation activity when a user or assistant message is appended", async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date("2026-08-03T10:00:00.000Z"))
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_a", "activity")

    vi.setSystemTime(new Date("2026-08-03T10:05:00.000Z"))
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: "继续诊断", pageContext: {}, idempotencyKey: "activity-turn",
    })
    expect((await repository.getConversation("usr_a", conversation.id))?.updatedAt).toBe("2026-08-03T10:05:00.000Z")

    vi.setSystemTime(new Date("2026-08-03T10:06:00.000Z"))
    await repository.appendItem({
      runId: created.run.id,
      turnId: created.turn.id,
      type: "assistant_message",
      status: "completed",
      content: { parts: [{ type: "text", text: "诊断完成" }] },
    })
    expect((await repository.getConversation("usr_a", conversation.id))?.updatedAt).toBe("2026-08-03T10:06:00.000Z")
  })

  it("persists a turn and returns the same run for an idempotent retry", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_a", "诊断")
    const request = { conversationId: conversation.id, input: "检查构建", pageContext: {}, idempotencyKey: "request-123" }
    const first = await repository.createTurn("usr_a", request)
    const second = await repository.createTurn("usr_a", request)
    expect(second.run.id).toBe(first.run.id)
    const events = await repository.getEvents("usr_a", first.run.id, 0)
    expect(events.map(event => event.type)).toEqual(["run.input_received", "run.queued"])
    expect(events[0]?.data.item).toMatchObject({ type: "user_message", content: { parts: [{ type: "text", text: "检查构建" }] } })
    expect((await repository.getTimeline("usr_a", conversation.id))?.turns).toHaveLength(1)
  })
  it("persists queue trace context without making telemetry part of idempotency", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_a", "trace")
    const request = {
      conversationId: conversation.id,
      input: "检查发布",
      pageContext: {},
      idempotencyKey: "request-trace",
      traceContext: { traceparent: "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01" },
    }
    const first = await repository.createTurn("usr_a", request)
    const second = await repository.createTurn("usr_a", {
      ...request,
      traceContext: { traceparent: "00-abcdef0123456789abcdef0123456789-abcdef0123456789-01" },
    })

    expect(second.run.id).toBe(first.run.id)
    expect((await repository.claimNextQueuedRun())?.traceContext).toEqual(request.traceContext)
  })
  it("isolates conversations by owner", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_a", "private")
    expect(await repository.getConversation("usr_b", conversation.id)).toBeUndefined()
  })
  it("filters the conversation directory and respects stable ascending or descending activity order", async () => {
    vi.useFakeTimers()
    const repository = new TestRepository()
    vi.setSystemTime(new Date("2026-08-15T01:00:00.000Z"))
    const older = await repository.createConversation("usr_a", "Build failure")
    vi.setSystemTime(new Date("2026-08-15T02:00:00.000Z"))
    const newer = await repository.createConversation("usr_a", "BUILD release")
    await repository.createConversation("usr_a", "unrelated")
    await repository.createConversation("usr_b", "build from another owner")

    const ascending = await repository.listConversations("usr_a", 1, 20, { search: "build", sortOrder: "asc" })
    const descending = await repository.listConversations("usr_a", 1, 20, { search: "BUILD", sortOrder: "desc" })

    expect(ascending.total).toBe(2)
    expect(ascending.items.map(item => item.id)).toEqual([older.id, newer.id])
    expect(descending.items.map(item => item.id)).toEqual([newer.id, older.id])
  })
  it("returns recent complete turns and pages older turns with an exclusive stable boundary", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_a", "long history")
    for (let turnIndex = 0; turnIndex < 35; turnIndex += 1) {
      await repository.createTurn("usr_a", {
        conversationId: conversation.id,
        input: `turn-${turnIndex}`,
        pageContext: {},
        idempotencyKey: `history-page-${turnIndex}`,
      })
    }

    const latest = await repository.getTimeline("usr_a", conversation.id)
    expect(latest?.turns.map(turn => turn.turnIndex)).toEqual(Array.from({ length: 30 }, (_, index) => index + 5))
    expect(latest?.eventCursors).toHaveLength(30)
    expect(latest?.pageInfo).toEqual({ hasOlder: true, oldestTurnIndex: 5 })

    await repository.createTurn("usr_a", {
      conversationId: conversation.id,
      input: "concurrent-newer-turn",
      pageContext: {},
      idempotencyKey: "history-page-concurrent",
    })
    const older = await repository.getTimeline("usr_a", conversation.id, { beforeTurnIndex: 5, limit: 30 })
    expect(older?.turns.map(turn => turn.turnIndex)).toEqual([0, 1, 2, 3, 4])
    expect(older?.pageInfo).toEqual({ hasOlder: false })
  })
  it("reuses only empty conversations and protects manually renamed titles", async () => {
    const repository = new TestRepository()
    const empty = await repository.createConversation("usr_a", "新会话")
    expect(empty.titleSource).toBe("default")
    expect((await repository.findEmptyConversation("usr_a"))?.id).toBe(empty.id)
    const created = await repository.createTurn("usr_a", { conversationId: empty.id, input: "hello", pageContext: {}, idempotencyKey: "occupied" })
    expect((await repository.renameConversationByAssistant(empty.id, "自动标题", created.run.id))?.titleSource).toBe("assistant")
    await repository.renameConversation("usr_a", empty.id, "手动标题")
    expect((await repository.getConversation("usr_a", empty.id))?.titleSource).toBe("user")
    expect(await repository.renameConversationByAssistant(empty.id, "另一个自动标题")).toBeUndefined()
    expect((await repository.getConversation("usr_a", empty.id))?.title).toBe("手动标题")
    const titleEvents = (await repository.getEvents("usr_a", created.run.id, 0))
      .filter(event => event.type === "conversation.title.updated")
    expect(titleEvents.map(event => event.data)).toEqual([
      { title: "自动标题", titleSource: "assistant", locked: false },
      { title: "手动标题", titleSource: "user", locked: true },
    ])
    expect(await repository.findEmptyConversation("usr_a")).toBeUndefined()
  })
  it("atomically starts a queued Run only once", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_a", "lease")
    await repository.createTurn("usr_a", { conversationId: conversation.id, input: "hello", pageContext: {}, idempotencyKey: "request-lease" })
    const [a, b] = await Promise.all([repository.claimNextQueuedRun(), repository.claimNextQueuedRun()])
    expect([a, b].filter(Boolean)).toHaveLength(1)
  })
  it("returns a bounded recent user and assistant history for the next turn", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_a", "history")
    const first = await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: "先检查构建", pageContext: {}, idempotencyKey: "history-1",
    })
    await repository.appendItem({
      runId: first.run.id,
      turnId: first.turn.id,
      type: "assistant_message",
      status: "completed",
      content: { parts: [{ type: "text", text: "构建失败在测试阶段。" }] },
    })
    const second = await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: "那发布呢", pageContext: {}, idempotencyKey: "history-2",
    })

    expect((await repository.getExecutionInput(second.run.id))?.history).toEqual([{
      turnIndex: 0,
      user: "先检查构建",
      assistant: "构建失败在测试阶段。",
    }])
  })

  it("publishes an authoritative revision when a streaming item is finalized", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_a", "finalize")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: "hello", pageContext: {}, idempotencyKey: "finalize-stream",
    })
    const item = await repository.appendItem({
      runId: created.run.id,
      turnId: created.turn.id,
      type: "assistant_message",
      status: "streaming",
      content: { parts: [{ type: "text", text: "partial" }] },
    })

    await repository.finalizeStreamingItems(created.run.id, "failed")

    const timeline = await repository.getTimeline("usr_a", conversation.id)
    const finalized = timeline?.turns[0]?.items.find(candidate => candidate.id === item.id)
    const event = (await repository.getEvents("usr_a", created.run.id, 0)).at(-1)
    expect(finalized).toMatchObject({ status: "failed", revision: 2 })
    expect(event).toMatchObject({
      type: "item.finalized",
      data: { item: { id: item.id, status: "failed", revision: 2 } },
    })
  })
})
