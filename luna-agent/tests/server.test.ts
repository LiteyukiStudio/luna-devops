import { describe, expect, it } from "vitest"
import { DevelopmentAuthenticator } from "../src/auth.js"
import { loadConfig } from "../src/config.js"
import { MemoryRepository } from "../src/persistence/memory.js"
import { DeterministicProvider } from "../src/provider/deterministic.js"
import { buildServer } from "../src/server.js"
import { RunGrantCipher } from "../src/grant-cipher.js"
import type { AICapabilities, AIEvent, AITimeline, AITurnCreated } from "../../web/src/api/ai-types.js"
import { presentTimeline } from "../src/timeline-presenter.js"

function fixture() {
  const repository = new MemoryRepository()
  const provider = new DeterministicProvider()
  const app = buildServer({ config: loadConfig({ NODE_ENV: "test" }), repository, provider, authenticator: new DevelopmentAuthenticator(), graphVersions: ["assistant-v1"], grantCipher: new RunGrantCipher(Buffer.alloc(32, 1)) })
  return { app, repository }
}

describe("internal API", () => {
  it("requires an authenticated actor", async () => {
    const { app } = fixture()
    const response = await app.inject({ method: "GET", url: "/internal/v1/conversations" })
    expect(response.statusCode).toBe(401)
    await app.close()
  })
  it("creates a conversation and a durable turn", async () => {
    const { app } = fixture()
    const headers = { "x-luna-dev-user": "usr_test" }
    const conversation = await app.inject({ method: "POST", url: "/internal/v1/conversations", headers, payload: { title: "构建诊断" } })
    expect(conversation.statusCode).toBe(201)
    const id = conversation.json<{ id: string }>().id
    const turn = await app.inject({
      method: "POST", url: `/internal/v1/conversations/${id}/turns`,
      headers: { ...headers, "idempotency-key": "browser-request-1" },
      payload: { input: { parts: [{ type: "text", text: "为什么失败？" }] }, pageContext: { routeName: "application.builds" } },
    })
    expect(turn.statusCode).toBe(202)
    expect(turn.json()).toMatchObject({ state: "queued" })
    await app.close()
  })
  it("returns the complete fail-closed Web capabilities contract", async () => {
    const { app } = fixture()
    const response = await app.inject({
      method: "GET",
      url: "/internal/v1/capabilities",
      headers: { "x-luna-dev-user": "usr_test" },
    })
    expect(response.statusCode).toBe(200)
    const capabilities: AICapabilities = response.json<AICapabilities>()
    expect(capabilities).toMatchObject({
      available: true,
      reasonCode: null,
      features: {
        streaming: true,
        approvals: false,
        stepUpMFA: false,
        uiActions: true,
        longTermMemory: false,
      },
      limits: {
        maxInputBytes: 48000,
        maxConcurrentRuns: 2,
      },
    })
    await app.close()
  })
  it("presents a created turn as the strict Web timeline contract", async () => {
    const { app, repository } = fixture()
    const headers = { "x-luna-dev-user": "usr_timeline" }
    const conversation = await app.inject({ method: "POST", url: "/internal/v1/conversations", headers, payload: { title: "Timeline" } })
    const conversationId = conversation.json<{ id: string }>().id
    const created = await app.inject({
      method: "POST",
      url: `/internal/v1/conversations/${conversationId}/turns`,
      headers: { ...headers, "idempotency-key": "timeline-request-1" },
      payload: { input: { parts: [{ type: "text", text: "检查构建状态" }] }, pageContext: {} },
    })
    const turnCreated: AITurnCreated = created.json<AITurnCreated>()
    const runId = turnCreated.runId
    expect(turnCreated.eventsUrl).toBe(`/api/v1/ai/runs/${runId}/events`)
    const response = await app.inject({ method: "GET", url: `/internal/v1/conversations/${conversationId}/timeline`, headers })
    const timeline: AITimeline = response.json<AITimeline>()
    expect(response.statusCode).toBe(200)
    expect(timeline).toMatchObject({
      conversation: { id: conversationId, title: "Timeline", status: "active" },
      turns: [{
        turnIndex: 0,
        input: { type: "user_message", parts: [{ partIndex: 0, type: "text", text: "检查构建状态" }] },
        selectedRun: { id: runId, runIndex: 0, status: "queued", expectedVersion: 1, items: [] },
      }],
      eventCursors: [{ runId, after: 1 }],
    })
    expect(Array.isArray(timeline.eventCursors)).toBe(true)
    const directlyTyped: AITimeline | undefined = await presentTimeline(repository, "usr_timeline", conversationId)
    expect(directlyTyped?.turns[0]?.selectedRun?.id).toBe(runId)
    const eventsResponse = await app.inject({ method: "GET", url: `/internal/v1/runs/${runId}/events?after=0&stream=false`, headers })
    const events = eventsResponse.json<{ items: AIEvent[], cursor: number }>()
    expect(events).toMatchObject({
      cursor: 1,
      items: [{
        version: 1,
        eventSequence: 1,
        type: "run.queued",
        conversationId,
        turnId: turnCreated.turnId,
        runId,
        payload: { state: "queued" },
      }],
    })
    await app.close()
  })
  it("uses SSE when EventSource negotiates text/event-stream without a query flag", async () => {
    const { app } = fixture()
    const headers = { "x-luna-dev-user": "usr_sse" }
    const conversation = await app.inject({ method: "POST", url: "/internal/v1/conversations", headers, payload: { title: "SSE" } })
    const conversationId = conversation.json<{ id: string }>().id
    const created = await app.inject({
      method: "POST",
      url: `/internal/v1/conversations/${conversationId}/turns`,
      headers: { ...headers, "idempotency-key": "sse-request-1" },
      payload: { input: { parts: [{ type: "text", text: "hello" }] }, pageContext: {} },
    })
    const runId = created.json<AITurnCreated>().runId
    await app.inject({ method: "POST", url: `/internal/v1/runs/${runId}/cancel`, headers })
    const response = await app.inject({
      method: "GET",
      url: `/internal/v1/runs/${runId}/events`,
      headers: { ...headers, accept: "text/event-stream" },
    })
    expect(response.statusCode).toBe(200)
    expect(response.headers["content-type"]).toContain("text/event-stream")
    expect(response.body).toContain("event: run.queued")
    expect(response.body).toContain("\"version\":1")
    expect(response.body).not.toContain("\"items\"")
    await app.close()
  })
})
