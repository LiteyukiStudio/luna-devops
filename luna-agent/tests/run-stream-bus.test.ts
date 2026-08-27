import { afterEach, describe, expect, it, vi } from "vitest"
import { InMemoryRunStreamBus } from "../src/run-stream-bus.js"
import { TestRepository } from "./support/test-repository.js"

async function fixture() {
  const repository = new TestRepository()
  const conversation = await repository.createConversation("user-stream", "stream", undefined, "default")
  const created = await repository.createTurn("user-stream", {
    conversationId: conversation.id,
    input: "hello",
    pageContext: {},
    idempotencyKey: "stream-test-key",
  })
  const run = await repository.claimNextQueuedRun()
  if (!run || run.id !== created.run.id) throw new Error("stream fixture claim failed")
  return { repository, conversation, turn: created.turn, run }
}

afterEach(() => vi.useRealTimers())

describe("run stream terminal persistence", () => {
  it("keeps deltas live-only and persists one final item plus durable checkpoints", async () => {
    const { repository, run, turn } = await fixture()
    const bus = new InMemoryRunStreamBus(repository)
    const baseline = (await repository.getRunStreamPosition(run.id))!
    const stream = await bus.open(run.id, turn.id)
    await stream.appendEvent("model.started", {})
    const itemId = "aiitm_stream_final"
    await stream.appendItemWithEvent({
      id: itemId, runId: run.id, turnId: turn.id,
      type: "assistant_message", status: "streaming", content: { parts: [{ type: "text", text: "a" }] },
    }, "content.delta", { itemId, delta: "a" })
    await stream.updateItemWithEvent(itemId, "streaming", { parts: [{ type: "text", text: "ab" }] }, "content.delta", { itemId, delta: "b" })
    await stream.updateItemWithEvent(itemId, "completed", { parts: [{ type: "text", text: "ab" }] }, "message.completed", { itemId })
    await stream.appendEvent("model.completed", { usage: { status: "unavailable", reason: "not_reported" } })

    expect(await repository.getEvents("user-stream", run.id, baseline.nextEventSequence - 1)).toEqual([])
    expect((await bus.read(run.id, baseline.nextEventSequence - 1)).map(event => event.type)).toEqual([
      "model.started", "content.delta", "content.delta", "message.completed", "model.completed",
    ])

    await stream.commit()
    const durable = await repository.getEvents("user-stream", run.id, baseline.nextEventSequence - 1)
    expect(durable.map(event => event.type)).toEqual(["message.completed", "model.completed"])
    expect(durable[0]!.sequence).toBeGreaterThan(baseline.nextEventSequence)
    const timeline = await repository.getTimeline("user-stream", run.conversationId)
    expect(timeline?.turns[0]?.items.filter(item => item.type === "assistant_message")).toEqual([
      expect.objectContaining({ id: itemId, status: "completed", content: { parts: [{ type: "text", text: "ab" }] } }),
    ])
    const next = await repository.appendEvent(run.id, "run.completed", { state: "completed" })
    expect(next.sequence).toBeGreaterThan(durable.at(-1)!.sequence)
  })

  it("replays strictly after the refresh cursor and makes terminal commit idempotent", async () => {
    const { repository, run, turn } = await fixture()
    const bus = new InMemoryRunStreamBus(repository)
    const stream = await bus.open(run.id, turn.id)
    const first = await stream.appendEvent("model.started", {})
    const second = await stream.appendEvent("model.completed", {})
    expect((await bus.read(run.id, first.sequence)).map(event => event.sequence)).toEqual([second.sequence])
    await stream.commit()
    await stream.commit()
    expect((await repository.getEvents("user-stream", run.id, 0)).filter(event => event.id === second.id)).toHaveLength(1)
  })

  it("retries the same completed terminal intent after two transient persistence failures", async () => {
    const { repository, run, turn } = await fixture()
    const persisted = repository.persistRunStreamBatch.bind(repository)
    const terminalStates: string[] = []
    let attempts = 0
    repository.persistRunStreamBatch = async (batch) => {
      attempts += 1
      if (batch.terminal) terminalStates.push(batch.terminal.to)
      if (attempts < 3) throw new Error("temporary database failure")
      await persisted(batch)
    }
    const stream = await new InMemoryRunStreamBus(repository).open(run.id, turn.id, run.rowVersion)

    await stream.commitTerminal("completed")

    expect(attempts).toBe(3)
    expect(terminalStates).toEqual(["completed", "completed", "completed"])
    expect((await repository.getRun("user-stream", run.id))?.status).toBe("completed")
  })

  it("retries an unknown commit result idempotently without duplicating the completed terminal", async () => {
    const { repository, run, turn } = await fixture()
    const persisted = repository.persistRunStreamBatch.bind(repository)
    let attempts = 0
    repository.persistRunStreamBatch = async (batch) => {
      attempts += 1
      await persisted(batch)
      if (attempts === 1) throw new Error("connection lost after commit")
    }
    const stream = await new InMemoryRunStreamBus(repository).open(run.id, turn.id, run.rowVersion)

    await stream.commitTerminal("completed")

    expect(attempts).toBe(2)
    expect((await repository.getEvents("user-stream", run.id, 0)).filter(event => event.type === "run.completed")).toHaveLength(1)
    expect((await repository.getRun("user-stream", run.id))?.status).toBe("completed")
  })

  it("keeps the completed intent fixed when terminal persistence retries are exhausted", async () => {
    const { repository, run, turn } = await fixture()
    const terminalStates: string[] = []
    repository.persistRunStreamBatch = async (batch) => {
      if (batch.terminal) terminalStates.push(batch.terminal.to)
      throw new Error("database unavailable")
    }
    const stream = await new InMemoryRunStreamBus(repository).open(run.id, turn.id, run.rowVersion)

    await expect(stream.commitTerminal("completed")).rejects.toMatchObject({
      message: "ai.terminal_persistence_failed",
      terminalState: "completed",
    })
    await expect(stream.commitTerminal("failed")).rejects.toThrow("ai.terminal_intent_conflict")
    expect(terminalStates).toEqual(["completed", "completed", "completed"])
    expect((await repository.getRun("user-stream", run.id))?.status).toBe("running")
  })

  it("advances the durable high-water mark when a live-only model start is the entire batch", async () => {
    const { repository, run, turn } = await fixture()
    const bus = new InMemoryRunStreamBus(repository)
    const baseline = (await repository.getRunStreamPosition(run.id))!
    const stream = await bus.open(run.id, turn.id)
    const started = await stream.appendEvent("model.started", {})

    await stream.commit()

    const position = await repository.getRunStreamPosition(run.id)
    expect(position?.nextEventSequence).toBe(started.sequence + 1)
    const failed = await repository.appendEvent(run.id, "run.failed", { state: "failed" })
    expect(failed.sequence).toBe(started.sequence + 1)
    expect(failed.sequence).toBeGreaterThanOrEqual(baseline.nextEventSequence)
  })

  it("keeps long live output linear and persists only the final item", async () => {
    const { repository, run, turn } = await fixture()
    const bus = new InMemoryRunStreamBus(repository)
    const stream = await bus.open(run.id, turn.id, run.rowVersion)
    const itemId = "aiitm_long_linear"
    await stream.appendItemWithEvent({
      id: itemId, runId: run.id, turnId: turn.id,
      type: "assistant_message", status: "streaming", content: { parts: [{ type: "text", text: "x" }] },
    }, "content.delta", { delta: "x", partIndex: 0 })
    let answer = "x"
    for (let index = 1; index < 10_000; index += 1) {
      answer += "x"
      await stream.updateItemWithEvent(itemId, "streaming", {
        parts: [{ type: "text", text: answer }],
      }, "content.delta", { delta: "x", partIndex: 0 })
    }
    const live = await bus.read(run.id, 0, 20_000)
    expect(live).toHaveLength(10_000)
    expect(live.every(event => event.data.item === undefined && event.data.delta === "x")).toBe(true)
    expect(Buffer.byteLength(JSON.stringify(live))).toBeLessThan(3_000_000)
    await stream.commit()
    const timeline = await repository.getTimeline("user-stream", run.conversationId)
    expect(timeline?.turns[0]?.items.find(item => item.id === itemId)?.content).toEqual({
      parts: [{ type: "text", text: answer }],
    })
  })

  it("atomically persists the final answer and title before the unique terminal event", async () => {
    const { repository, run, turn, conversation } = await fixture()
    const bus = new InMemoryRunStreamBus(repository)
    const stream = await bus.open(run.id, turn.id, run.rowVersion)
    const itemId = "aiitm_atomic_terminal"
    await stream.appendItemWithEvent({
      id: itemId, runId: run.id, turnId: turn.id,
      type: "assistant_message", status: "streaming", content: { parts: [{ type: "text", text: "done" }] },
    }, "content.delta", { delta: "done", partIndex: 0 })
    await stream.updateItemWithEvent(itemId, "completed", {
      parts: [{ type: "text", text: "done" }],
    }, "message.completed", { itemId, partIndex: 0 })
    await stream.appendEvent("model.completed", {})

    await stream.commitTerminal("completed", undefined, "Atomic title")

    expect((await repository.getRun("user-stream", run.id))?.status).toBe("completed")
    expect((await repository.getConversation("user-stream", conversation.id))?.title).toBe("Atomic title")
    const events = await repository.getEvents("user-stream", run.id, 0)
    expect(events.at(-1)?.type).toBe("run.completed")
    expect(events.filter(event => event.type === "run.completed")).toHaveLength(1)
    expect(events.findIndex(event => event.type === "conversation.title.updated")).toBeLessThan(events.length - 1)
  })

  it("does not allocate a PostgreSQL event sequence for a user rename during a live model session", async () => {
    const { repository, run, turn, conversation } = await fixture()
    const bus = new InMemoryRunStreamBus(repository)
    const stream = await bus.open(run.id, turn.id, run.rowVersion)
    const first = await stream.appendEvent("model.started", {})
    await repository.updateConversation("user-stream", conversation.id, { title: "User title" })
    const second = await stream.appendEvent("content.delta", { itemId: "aiitm_live", delta: "x", timelineIndex: 1 })

    expect(second.sequence).toBe(first.sequence + 1)
    expect((await repository.getEvents("user-stream", run.id, 0)).some(event => event.type === "conversation.title.updated")).toBe(false)
    expect((await repository.getConversation("user-stream", conversation.id))?.title).toBe("User title")
  })

  it("keeps an assistant rename between model checkpoints ahead of the next live sequence", async () => {
    const { repository, run, turn, conversation } = await fixture()
    const bus = new InMemoryRunStreamBus(repository)
    const firstSession = await bus.open(run.id, turn.id, run.rowVersion)
    await firstSession.appendEvent("model.started", {})
    await firstSession.appendEvent("model.completed", {})
    await firstSession.commit()
    await repository.renameConversationByAssistant(conversation.id, "Assistant title", run.id)
    const renameEvent = (await repository.getEvents("user-stream", run.id, 0)).find(event => event.type === "conversation.title.updated")!

    const nextSession = await bus.open(run.id, turn.id, run.rowVersion)
    const nextStarted = await nextSession.appendEvent("model.started", {})

    expect(nextStarted.sequence).toBe(renameEvent.sequence + 1)
  })

  it("rejects an oversized live event without persisting partial state", async () => {
    const { repository, run, turn } = await fixture()
    const bus = new InMemoryRunStreamBus(repository)
    const stream = await bus.open(run.id, turn.id, run.rowVersion)
    await expect(stream.appendEvent("content.delta", { delta: "x".repeat(300 * 1024) }))
      .rejects.toThrow("ai.stream_buffer_overflow")
    expect(await bus.read(run.id, 0)).toEqual([])
  })

  it("releases retained events and the process byte count after the cleanup grace period", async () => {
    vi.useFakeTimers()
    const { repository, run, turn } = await fixture()
    const bus = new InMemoryRunStreamBus(repository)
    const stream = await bus.open(run.id, turn.id, run.rowVersion)
    await stream.appendEvent("content.delta", { delta: "retained" })
    expect((bus as unknown as { totalBytes: number }).totalBytes).toBeGreaterThan(0)

    await bus.cleanup(run.id)
    await vi.advanceTimersByTimeAsync(5 * 60 * 1_000)

    expect(await bus.read(run.id, 0)).toEqual([])
    expect((bus as unknown as { totalBytes: number }).totalBytes).toBe(0)
  })
})
