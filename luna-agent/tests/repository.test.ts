import { describe, expect, it } from "vitest"
import { MemoryRepository } from "../src/persistence/memory.js"

describe("conversation repository", () => {
  it("persists a turn and returns the same run for an idempotent retry", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "诊断")
    const request = { conversationId: conversation.id, input: "检查构建", pageContext: {}, idempotencyKey: "request-123" }
    const first = await repository.createTurn("usr_a", request)
    const second = await repository.createTurn("usr_a", request)
    expect(second.run.id).toBe(first.run.id)
    expect((await repository.getTimeline("usr_a", conversation.id))?.turns).toHaveLength(1)
  })
  it("isolates conversations by owner", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "private")
    expect(await repository.getConversation("usr_b", conversation.id)).toBeUndefined()
  })
  it("allows only one worker to claim a queued run", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "lease")
    await repository.createTurn("usr_a", { conversationId: conversation.id, input: "hello", pageContext: {}, idempotencyKey: "request-lease" })
    const [a, b] = await Promise.all([repository.claimRun("worker-a", 30), repository.claimRun("worker-b", 30)])
    expect([a, b].filter(Boolean)).toHaveLength(1)
  })
})
