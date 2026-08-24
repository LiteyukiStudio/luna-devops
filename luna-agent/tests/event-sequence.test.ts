import { describe, expect, it } from "vitest"
import { normalizeEventSequence } from "../src/event-sequence.js"
import { TestRepository } from "./support/test-repository.js"
import { presentEvent, presentTimeline } from "../src/timeline-presenter.js"

describe("PostgreSQL bigint event sequence normalization", () => {
  it("converts a pg-shaped string sequence for Timeline cursors and AIEvent", async () => {
    const repository = new TestRepository()
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
    expect(timeline?.eventCursors).toEqual([{ runId: created.run.id, after: 2 }])
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

  it("projects authoritative Provider prompt usage into timeline snapshots", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_usage", "usage")
    const created = await repository.createTurn("usr_usage", {
      conversationId: conversation.id,
      input: "inspect usage",
      pageContext: {},
      idempotencyKey: "usage-request",
      modelId: "aimdl_usage",
      modelSnapshot: {
        id: "aimdl_usage", name: "usage", maxContextTokens: 32_000, maxOutputTokens: 4_000,
        inputCreditsPerMillion: "1", outputCreditsPerMillion: "2", cachedInputCreditsPerMillion: "0.5",
      },
    })
    await repository.createModelCreditHold({
      id: "aihold_assistant_1",
      runId: created.run.id,
      ownerUserId: "usr_usage",
      operation: "assistant",
      requestedOutputTokens: 4_000,
      leaseSeconds: 60,
    })
    await repository.recordReportedModelUsage("aihold_assistant_1", {
      inputTokens: 18_000, outputTokens: 2_000, totalTokens: 20_000,
    }, { callType: "stream" })
    await repository.createModelCreditHold({
      id: "aihold_title_1",
      runId: created.run.id,
      ownerUserId: "usr_usage",
      operation: "title",
      requestedOutputTokens: 200,
      leaseSeconds: 60,
    })
    await repository.recordReportedModelUsage("aihold_title_1", {
      inputTokens: 700, outputTokens: 100, totalTokens: 800,
    }, { callType: "complete" })

    const timeline = await presentTimeline(repository, "usr_usage", conversation.id)
    expect(timeline?.turns[0]?.selectedRun).toMatchObject({
      latestPromptTokens: 18_000,
      latestUsageModelId: "aimdl_usage",
      latestUsageMaxContextTokensSnapshot: 32_000,
    })
    expect(timeline?.contextUsage).toMatchObject({
      status: "reported",
      runId: created.run.id,
      modelId: "aimdl_usage",
      usedTokens: 20_000,
      maxContextTokensSnapshot: 32_000,
    })

    await repository.updateRun(created.run.id, "queued", "running")
    await repository.updateRun(created.run.id, "running", "completed")
    const next = await repository.createTurn("usr_usage", {
      conversationId: conversation.id,
      input: "continue",
      pageContext: {},
      idempotencyKey: "usage-request-next",
      modelId: "aimdl_usage",
      modelSnapshot: created.run.model!,
    })
    const activeTimeline = await presentTimeline(repository, "usr_usage", conversation.id)
    expect(activeTimeline?.turns.at(-1)?.selectedRun).toMatchObject({ id: next.run.id, status: "queued" })
    expect(activeTimeline?.contextUsage).toMatchObject({ runId: created.run.id, usedTokens: 20_000 })
  })

  it("projects bounded non-sensitive tool results consistently for snapshots and events", async () => {
    const repository = new TestRepository()
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
        durationMs: 234,
        traceId: "717690e2661f8337d53fcd3295591b4b",
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
    expect(toolItem.toolCall).toMatchObject({
      durationMs: 234,
      traceId: "717690e2661f8337d53fcd3295591b4b",
    })
    expect(toolResult).toMatchObject({
      requestId: "req_visible",
      data: { data: { items: [{ id: "app-1", name: "PostgreSQL" }], total: 1 } },
    })
    expect(JSON.stringify(toolResult)).not.toContain("must-not-leak")

    const event = await presentEvent(repository, "usr_a", rawEvent)
    expect(event?.payload.result).toEqual(toolResult)
    expect(JSON.stringify(event?.payload.result)).not.toContain("must-not-leak")
  })

  it("preserves a rejected approval decision in the public timeline", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_rejected", "approval")
    const created = await repository.createTurn("usr_rejected", {
      conversationId: conversation.id, input: "do not delete", pageContext: {}, idempotencyKey: "rejected-tool",
    })
    await repository.appendItem({
      runId: created.run.id,
      turnId: created.turn.id,
      type: "tool_call",
      status: "completed",
      content: {
        toolCallId: "tool-rejected",
        operationId: "deleteProject",
        status: "rejected",
        arguments: { projectId: "prj_test" },
        errorCode: "ai.tool_rejected",
        result: { code: "ai.tool_rejected", retryable: false },
      },
    })

    const timeline = await presentTimeline(repository, "usr_rejected", conversation.id)
    const toolItem = timeline?.turns[0]?.selectedRun?.items.find(item => "toolCall" in item && item.toolCall.id === "tool-rejected")
    if (!toolItem || !("toolCall" in toolItem)) throw new Error("expected rejected tool call")
    expect(toolItem.toolCall.status).toBe("rejected")
  })

  it("uses the same authoritative item revision and position for live events and snapshots", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_a", "authoritative timeline")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: "inspect the project", pageContext: {}, idempotencyKey: "authoritative-item",
    })
    const started = await repository.appendItemWithEvent({
      runId: created.run.id,
      turnId: created.turn.id,
      type: "assistant_message",
      status: "streaming",
      content: { parts: [{ type: "text", text: "正在检查" }] },
    }, "content.delta")
    const completed = await repository.updateItemWithEvent(
      started.item.id,
      "completed",
      { parts: [{ type: "text", text: "检查完成" }] },
      "message.completed",
    )

    const startedEvent = await presentEvent(repository, "usr_a", started.event)
    const completedEvent = await presentEvent(repository, "usr_a", completed.event)
    const timeline = await presentTimeline(repository, "usr_a", conversation.id)
    const snapshotItem = timeline?.turns[0]?.selectedRun?.items[0]

    expect(startedEvent).toMatchObject({
      version: 2,
      eventSequence: 3,
      item: { id: started.item.id, timelineIndex: 1, revision: 1, status: "streaming" },
    })
    expect(completedEvent).toMatchObject({
      version: 2,
      eventSequence: 4,
      item: { id: started.item.id, timelineIndex: 1, revision: 2, status: "completed" },
    })
    expect(snapshotItem).toMatchObject({
      id: started.item.id,
      timelineIndex: 1,
      revision: 2,
      status: "completed",
      parts: [{ text: "检查完成" }],
    })
    expect(timeline?.eventCursors).toEqual([{ runId: created.run.id, after: 4 }])
  })
})
