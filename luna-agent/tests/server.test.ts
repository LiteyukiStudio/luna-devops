import { describe, expect, it } from "vitest"
import { DevelopmentAuthenticator } from "../src/auth.js"
import { loadConfig } from "../src/config.js"
import { MemoryRepository } from "../src/persistence/memory.js"
import { DeterministicProvider } from "../src/provider/deterministic.js"
import { buildServer } from "../src/server.js"
import { PayloadCipher } from "../src/payload-cipher.js"
import type { AICapabilities, AIEvent, AITimeline, AITurnCreated } from "../../web/src/api/ai-types.js"
import { presentTimeline } from "../src/timeline-presenter.js"

function fixture() {
  const repository = new MemoryRepository()
  const provider = new DeterministicProvider()
  const app = buildServer({ config: loadConfig({ NODE_ENV: "test" }), repository, provider, authenticator: new DevelopmentAuthenticator(), graphVersions: ["assistant-v1"], grantCipher: new PayloadCipher(Buffer.alloc(32, 1)) })
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
      payload: { input: { parts: [{ type: "text", text: "为什么失败？" }] }, pageContext: { routeName: "application.builds" }, clientInstanceId: "browser-client-instance-1" },
    })
    expect(turn.statusCode).toBe(202)
    expect(turn.json()).toMatchObject({ state: "queued", turnIndex: 0 })
    await app.close()
  })
  it("never exposes persisted trace propagation state in run responses", async () => {
    const { app, repository } = fixture()
    const conversation = await repository.createConversation("usr_trace_response", "trace")
    const { run } = await repository.createTurn("usr_trace_response", {
      conversationId: conversation.id,
      input: "检查链路",
      pageContext: {},
      idempotencyKey: "trace-response-1",
      traceContext: { traceparent: "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01" },
    })

    const response = await app.inject({
      method: "GET",
      url: `/internal/v1/runs/${run.id}`,
      headers: { "x-luna-dev-user": "usr_trace_response" },
    })

    expect(response.statusCode).toBe(200)
    expect(response.json()).not.toHaveProperty("traceContext")
    await app.close()
  })
  it("marks browser title edits as user-owned and permanently blocks assistant renames", async () => {
    const { app, repository } = fixture()
    const headers = { "x-luna-dev-user": "usr_title_owner" }
    const created = await app.inject({ method: "POST", url: "/internal/v1/conversations", headers, payload: {} })
    const conversationId = created.json<{ id: string }>().id
    expect(created.json<{ titleSource: string }>().titleSource).toBe("default")

    const renamed = await app.inject({
      method: "PATCH",
      url: `/internal/v1/conversations/${conversationId}`,
      headers,
      payload: { title: "我的固定标题" },
    })

    expect(renamed.statusCode).toBe(200)
    expect(renamed.json()).toMatchObject({ title: "我的固定标题", titleSource: "user" })
    expect(await repository.renameConversationByAssistant(conversationId, "AI 不应覆盖")).toBeUndefined()
    expect((await repository.getConversation("usr_title_owner", conversationId))?.title).toBe("我的固定标题")
    await app.close()
  })
  it("keeps a durable cancellation successful when the local abort hook fails", async () => {
    const repository = new MemoryRepository()
    const provider = new DeterministicProvider()
    const app = buildServer({
      config: loadConfig({ NODE_ENV: "test" }),
      repository,
      provider,
      authenticator: new DevelopmentAuthenticator(),
      graphVersions: ["assistant-v1"],
      grantCipher: new PayloadCipher(Buffer.alloc(32, 1)),
      cancelRun: () => { throw new Error("local abort failed") },
    })
    const headers = { "x-luna-dev-user": "usr_cancel" }
    const conversation = await app.inject({ method: "POST", url: "/internal/v1/conversations", headers, payload: { title: "Cancel" } })
    const conversationId = conversation.json<{ id: string }>().id
    const created = await app.inject({
      method: "POST",
      url: `/internal/v1/conversations/${conversationId}/turns`,
      headers: { ...headers, "idempotency-key": "cancel-request-1" },
      payload: { input: { parts: [{ type: "text", text: "stop" }] }, pageContext: {}, clientInstanceId: "browser-client-instance-2" },
    })
    const runId = created.json<AITurnCreated>().runId
    const canceled = await app.inject({ method: "POST", url: `/internal/v1/runs/${runId}/cancel`, headers })
    expect(canceled.statusCode).toBe(200)
    expect(canceled.json()).toMatchObject({ id: runId, status: "canceled" })
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
      payload: { input: { parts: [{ type: "text", text: "检查构建状态" }] }, pageContext: {}, clientInstanceId: "browser-client-instance-3" },
    })
    const turnCreated: AITurnCreated = created.json<AITurnCreated>()
    const runId = turnCreated.runId
    expect(turnCreated.turnIndex).toBe(0)
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
    const directlyTyped = await presentTimeline(repository, "usr_timeline", conversationId) as AITimeline | undefined
    expect(directlyTyped?.turns[0]?.selectedRun?.id).toBe(runId)
    const eventsResponse = await app.inject({ method: "GET", url: `/internal/v1/runs/${runId}/events?after=0&stream=false`, headers })
    const events = eventsResponse.json<{ items: AIEvent[], cursor: number }>()
    expect(events).toMatchObject({
      cursor: 1,
      items: [{
        version: 2,
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
      payload: { input: { parts: [{ type: "text", text: "hello" }] }, pageContext: {}, clientInstanceId: "browser-client-instance-4" },
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
    expect(response.body).toContain("\"version\":2")
    expect(response.body).not.toContain("\"items\"")
    await app.close()
  })
  it("replays and acknowledges UI actions only for their bound browser client", async () => {
    const { app, repository } = fixture()
    const headers = { "x-luna-dev-user": "usr_ui_action" }
    const clientInstanceId = "browser-client-instance-5"
    const conversation = await app.inject({ method: "POST", url: "/internal/v1/conversations", headers, payload: { title: "Navigation" } })
    const conversationId = conversation.json<{ id: string }>().id
    const created = await app.inject({
      method: "POST",
      url: `/internal/v1/conversations/${conversationId}/turns`,
      headers: { ...headers, "idempotency-key": "ui-action-request-1" },
      payload: { input: { parts: [{ type: "text", text: "打开项目空间" }] }, pageContext: {}, clientInstanceId },
    })
    const runId = created.json<AITurnCreated>().runId
    const delivery = await repository.createUIAction(runId, "aitool_navigation", {
      version: 1,
      type: "navigate",
      activation: "automatic",
      repeatable: false,
      payload: { routeName: "projects", params: {}, query: {} },
    }, new Date(Date.now() + 60_000).toISOString())

    const wrongClient = await app.inject({
      method: "GET",
      url: "/internal/v1/ui-actions/pending?clientInstanceId=another-browser-client",
      headers,
    })
    expect(wrongClient.json()).toEqual({ items: [] })

    const pending = await app.inject({
      method: "GET",
      url: `/internal/v1/ui-actions/pending?clientInstanceId=${clientInstanceId}`,
      headers,
    })
    expect(pending.json()).toMatchObject({ items: [{ actionId: delivery.id, runId, toolCallId: "aitool_navigation" }] })

    const rejectedAck = await app.inject({
      method: "POST",
      url: `/internal/v1/ui-actions/${delivery.id}/ack`,
      headers,
      payload: { clientInstanceId: "another-browser-client", status: "succeeded", actualPath: "/projects" },
    })
    expect(rejectedAck.statusCode).toBe(404)

    const acknowledged = await app.inject({
      method: "POST",
      url: `/internal/v1/ui-actions/${delivery.id}/ack`,
      headers,
      payload: { clientInstanceId, status: "succeeded", actualPath: "/projects" },
    })
    expect(acknowledged.statusCode).toBe(202)
    expect(acknowledged.json()).toMatchObject({ actionId: delivery.id, status: "succeeded" })

    const empty = await app.inject({
      method: "GET",
      url: `/internal/v1/ui-actions/pending?clientInstanceId=${clientInstanceId}`,
      headers,
    })
    expect(empty.json()).toEqual({ items: [] })
    await app.close()
  })
})
