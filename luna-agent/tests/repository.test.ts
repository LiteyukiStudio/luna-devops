import { afterEach, describe, expect, it, vi } from "vitest"
import { MemoryRepository } from "../src/persistence/memory.js"

describe("conversation repository", () => {
  afterEach(() => vi.useRealTimers())

  it("updates conversation activity when a user or assistant message is appended", async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date("2026-08-03T10:00:00.000Z"))
    const repository = new MemoryRepository()
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
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "诊断")
    const request = { conversationId: conversation.id, input: "检查构建", pageContext: {}, idempotencyKey: "request-123" }
    const first = await repository.createTurn("usr_a", request)
    const second = await repository.createTurn("usr_a", request)
    expect(second.run.id).toBe(first.run.id)
    expect((await repository.getTimeline("usr_a", conversation.id))?.turns).toHaveLength(1)
  })
  it("persists queue trace context without making telemetry part of idempotency", async () => {
    const repository = new MemoryRepository()
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
    expect((await repository.claimRun("agent-a", 30))?.traceContext).toEqual(request.traceContext)
  })
  it("isolates conversations by owner", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "private")
    expect(await repository.getConversation("usr_b", conversation.id)).toBeUndefined()
  })
  it("reuses only empty conversations and protects manually renamed titles", async () => {
    const repository = new MemoryRepository()
    const empty = await repository.createConversation("usr_a", "新会话")
    expect(empty.titleSource).toBe("default")
    expect((await repository.findEmptyConversation("usr_a"))?.id).toBe(empty.id)
    expect((await repository.renameConversationByAssistant(empty.id, "自动标题"))?.titleSource).toBe("assistant")
    await repository.renameConversation("usr_a", empty.id, "手动标题")
    expect((await repository.getConversation("usr_a", empty.id))?.titleSource).toBe("user")
    expect(await repository.renameConversationByAssistant(empty.id, "另一个自动标题")).toBeUndefined()
    expect((await repository.getConversation("usr_a", empty.id))?.title).toBe("手动标题")
    await repository.createTurn("usr_a", { conversationId: empty.id, input: "hello", pageContext: {}, idempotencyKey: "occupied" })
    expect(await repository.findEmptyConversation("usr_a")).toBeUndefined()
  })
  it("allows only one worker to claim a queued run", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "lease")
    await repository.createTurn("usr_a", { conversationId: conversation.id, input: "hello", pageContext: {}, idempotencyKey: "request-lease" })
    const [a, b] = await Promise.all([repository.claimRun("worker-a", 30), repository.claimRun("worker-b", 30)])
    expect([a, b].filter(Boolean)).toHaveLength(1)
  })
  it("returns a bounded recent user and assistant history for the next turn", async () => {
    const repository = new MemoryRepository()
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
    const repository = new MemoryRepository()
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
