import { describe, expect, it } from "vitest"
import { InMemoryRunStreamBus } from "../src/run-stream-bus.js"
import { RunStreamHubManager } from "../src/run-stream-hub.js"
import { TestRepository } from "./support/test-repository.js"

async function fixture() {
  class CountingRepository extends TestRepository {
    runReads = 0
    eventReads = 0
    override async getRun(ownerUserId: string, runId: string) {
      this.runReads += 1
      return super.getRun(ownerUserId, runId)
    }
    override async getEvents(ownerUserId: string, runId: string, after: number) {
      this.eventReads += 1
      return super.getEvents(ownerUserId, runId, after)
    }
  }
  class CountingBus extends InMemoryRunStreamBus {
    readerOpens = 0
    override async openReader(runId: string) {
      this.readerOpens += 1
      return super.openReader(runId)
    }
  }
  const repository = new CountingRepository()
  const conversation = await repository.createConversation("hub-user", "hub")
  const created = await repository.createTurn("hub-user", {
    conversationId: conversation.id,
    input: "hello",
    pageContext: {},
    idempotencyKey: "hub-test-key",
  })
  const run = await repository.claimNextQueuedRun()
  if (!run) throw new Error("missing claimed run")
  const bus = new CountingBus(repository)
  return { repository, bus, run, turn: created.turn }
}

describe("per-Run SSE hub", () => {
  it("fans one live reader and one PG watcher out to the four supported subscribers", async () => {
    const { repository, bus, run, turn } = await fixture()
    const manager = new RunStreamHubManager(repository, bus)
    const after = (await repository.getRunStreamPosition(run.id))!.nextEventSequence - 1
    const subscriptions = await Promise.all(Array.from({ length: 4 }, () => manager.subscribe(run.id, "hub-user", after)))
    repository.runReads = 0
    repository.eventReads = 0
    expect(bus.readerOpens).toBe(1)

    const stream = await bus.open(run.id, turn.id, run.rowVersion)
    const first = await stream.appendEvent("model.started", {})
    const updates = await Promise.all(subscriptions.map(subscription => subscription.next(new AbortController().signal)))
    expect(updates.every(update => update.events.some(event => event.id === first.id))).toBe(true)
    // TestRepository.getEvents 会做一次额外所有权回读；仍为单 hub 常数，不随订阅者增长。
    expect(repository.runReads).toBeLessThanOrEqual(2)
    expect(repository.eventReads).toBeLessThanOrEqual(1)

    subscriptions[0]!.close()
    for (const subscription of subscriptions.slice(1)) subscription.advance(first.sequence)
    const second = await stream.appendEvent("content.delta", { itemId: "aiitm_hub", delta: "x", timelineIndex: 0 })
    const remaining = await Promise.all(subscriptions.slice(1).map(subscription => nextContaining(subscription, second.id)))
    expect(remaining.every(update => update.events.some(event => event.id === second.id))).toBe(true)
    for (const subscription of subscriptions.slice(1)) subscription.close()
    await manager.close()
  })

  it("disconnects only a slow subscriber whose bounded pending queue overflows", async () => {
    const { repository, bus, run, turn } = await fixture()
    const manager = new RunStreamHubManager(repository, bus, {
      perRun: 4, perInstance: 4, pendingEvents: 2, pendingBytes: 1024 * 1024,
    })
    const slow = await manager.subscribe(run.id, "hub-user", 0)
    const fast = await manager.subscribe(run.id, "hub-user", 0)
    const stream = await bus.open(run.id, turn.id, run.rowVersion)

    for (let index = 0; index < 3; index += 1) {
      const event = await stream.appendEvent("content.delta", { itemId: "aiitm_hub", delta: "x", timelineIndex: 0 })
      const update = await nextContaining(fast, event.id)
      fast.advance(update.events.at(-1)!.sequence)
    }

    const slowUpdate = await slow.next(new AbortController().signal)
    expect(slowUpdate.error?.message).toBe("ai.stream_subscriber_slow")
    expect(slowUpdate.events).toHaveLength(2)
    slow.close()
    fast.close()
    await manager.close()
  })

  it("rejects a stable per-Run subscriber overflow without disturbing accepted subscribers", async () => {
    const { repository, bus, run } = await fixture()
    const manager = new RunStreamHubManager(repository, bus, { perRun: 2, perInstance: 4 })
    const first = await manager.subscribe(run.id, "hub-user", 0)
    const second = await manager.subscribe(run.id, "hub-user", 0)

    await expect(manager.subscribe(run.id, "hub-user", 0)).rejects.toThrow("ai.stream_subscriber_limit")
    expect(bus.readerOpens).toBe(1)

    first.close()
    second.close()
    await manager.close()
  })
})

async function nextContaining(
  subscription: { next(signal: AbortSignal): Promise<{ events: Array<{ id: string, sequence: number }> }> },
  eventId: string,
) {
  const signal = new AbortController().signal
  for (;;) {
    const update = await subscription.next(signal)
    if (update.events.some(event => event.id === eventId)) return update
  }
}
