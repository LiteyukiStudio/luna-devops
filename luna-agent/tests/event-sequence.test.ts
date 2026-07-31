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

  it("projects bounded non-sensitive tool results consistently for snapshots and events", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "tool results")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: "list applications", pageContext: {}, idempotencyKey: "tool-result",
    })
    const result = {
      summaryKey: "ai.tool.result.completed",
      requestId: "req_visible",
      result: {
        data: {
          items: [{ id: "app-1", name: "PostgreSQL", apiToken: "must-not-leak" }],
          total: 1,
        },
      },
    }
    await repository.appendItem({
      runId: created.run.id,
      turnId: created.turn.id,
      type: "tool_call",
      status: "completed",
      content: {
        toolCallId: "tool-1",
        operationId: "listApplications",
        status: "succeeded",
        arguments: {},
        result,
      },
    })
    const rawEvent = await repository.appendEvent(created.run.id, "tool.completed", {
      itemId: "item-1",
      toolCallId: "tool-1",
      operationId: "listApplications",
      result,
    })

    const timeline = await presentTimeline(repository, "usr_a", conversation.id)
    const toolItem = timeline?.turns[0]?.selectedRun?.items.find(item => "toolCall" in item)
    if (!toolItem || !("toolCall" in toolItem)) throw new Error("expected projected tool call")
    const toolResult = toolItem.toolCall.result
    expect(toolResult).toMatchObject({
      requestId: "req_visible",
      data: { data: { items: [{ id: "app-1", name: "PostgreSQL" }], total: 1 } },
    })
    expect(JSON.stringify(toolResult)).not.toContain("must-not-leak")

    const event = await presentEvent(repository, "usr_a", rawEvent)
    expect(event?.payload.result).toEqual(toolResult)
    expect(JSON.stringify(event?.payload.result)).not.toContain("must-not-leak")
  })
})
