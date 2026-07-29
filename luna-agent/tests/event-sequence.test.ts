import { describe, expect, it } from "vitest"
import { normalizeEventSequence } from "../src/event-sequence.js"
import { MemoryRepository } from "../src/persistence/memory.js"
import { presentEvent, presentTimeline } from "../src/timeline-presenter.js"

describe("PostgreSQL bigint event sequence normalization", () => {
  it("converts a pg-shaped string sequence for Timeline cursors and AIEvent", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "cursor")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: "hello", pageContext: {}, idempotencyKey: "cursor-request",
    })
    const originalGetEvents = repository.getEvents.bind(repository)
    repository.getEvents = async (...args) => (await originalGetEvents(...args)).map(event => ({
      ...event,
      sequence: String(event.sequence) as unknown as number,
    }))
    const timeline = await presentTimeline(repository, "usr_a", conversation.id)
    expect(timeline?.eventCursors).toEqual([{ runId: created.run.id, after: 1 }])
    const raw = (await repository.getEvents("usr_a", created.run.id, 0))[0]!
    const event = await presentEvent(repository, "usr_a", raw)
    expect(event?.eventSequence).toBe(1)
    expect(typeof event?.eventSequence).toBe("number")
  })

  it("fails closed for unsafe or malformed bigint values", () => {
    expect(normalizeEventSequence("6")).toBe(6)
    expect(() => normalizeEventSequence("9007199254740992")).toThrow("ai.event_sequence_invalid")
    expect(() => normalizeEventSequence("-1")).toThrow("ai.event_sequence_invalid")
    expect(() => normalizeEventSequence("1.5")).toThrow("ai.event_sequence_invalid")
  })
})
