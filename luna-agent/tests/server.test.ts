import { EventEmitter } from "node:events"
import { describe, expect, it, vi } from "vitest"
import { DevelopmentRequestVerifier } from "../src/auth.js"
import { loadConfig } from "../src/config.js"
import { TestRepository } from "./support/test-repository.js"
import { DeterministicProvider } from "../src/provider/deterministic.js"
import { RemoteConfigSnapshot } from "../src/provider/config-client.js"
import { defaultRuntimeSettings } from "../src/runtime-settings.js"
import { buildServer, writeSSE } from "../src/server.js"
import type { AIEvent, AITimeline, AITurnCreated } from "../../web/src/api/ai-types.js"
import { presentTimeline } from "../src/timeline-presenter.js"
import { InMemoryRunStreamBus } from "../src/run-stream-bus.js"

function fixture() {
  const repository = new TestRepository()
  const provider = new DeterministicProvider()
  const app = buildServer({ config: loadConfig({ NODE_ENV: "test" }), repository, provider, requestVerifier: new DevelopmentRequestVerifier() })
  return { app, repository }
}

describe("internal API", () => {
  it("bounds an SSE write that never drains and destroys the slow connection", async () => {
    vi.useFakeTimers()
    try {
      let destroyed = false
      const stream = Object.assign(new EventEmitter(), {
        write: () => false,
        end: () => undefined,
        destroy: () => { destroyed = true },
      }) as unknown as NodeJS.WritableStream
      const pending = writeSSE(stream, "data: blocked\n\n", new AbortController().signal)

      await vi.advanceTimersByTimeAsync(9_999)
      expect(destroyed).toBe(false)
      await vi.advanceTimersByTimeAsync(1)

      await expect(pending).resolves.toBe(false)
      expect(destroyed).toBe(true)
      expect(stream.listenerCount("drain")).toBe(0)
      expect(stream.listenerCount("close")).toBe(0)
      expect(stream.listenerCount("error")).toBe(0)
    }
    finally { vi.useRealTimers() }
  })

  it("releases the accepted SSE slot when initial replay fails before hijacking", async () => {
    class ReplayFailureBus extends InMemoryRunStreamBus {
      override async read(): Promise<never> { throw new Error("ai.stream_transport_unavailable") }
    }
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_replay_failure", "replay")
    const created = await repository.createTurn("usr_replay_failure", {
      conversationId: conversation.id,
      input: "hello",
      pageContext: {},
      idempotencyKey: "replay-failure-slot",
    })
    const app = buildServer({
      config: loadConfig({ NODE_ENV: "test" }), repository,
      provider: new DeterministicProvider(), requestVerifier: new DevelopmentRequestVerifier(),
      streamBus: new ReplayFailureBus(repository),
      streamHubLimits: { perRun: 1, perInstance: 1 },
    })
    const request = {
      method: "GET" as const,
      url: `/internal/v1/runs/${created.run.id}/events`,
      headers: { "x-luna-dev-user": "usr_replay_failure", accept: "text/event-stream" },
    }

    expect((await app.inject(request)).statusCode).toBe(503)
    // 若第一次 replay 异常泄漏 subscription，此请求会被 per-instance cap 以 429 拒绝。
    expect((await app.inject(request)).statusCode).toBe(503)
    await app.close()
  })

  it("reports readiness dimensions and fails with stable dependency codes", async () => {
    const healthy = fixture()
    const healthyResponse = await healthy.app.inject({ method: "GET", url: "/internal/health/ready" })
    expect(healthyResponse.statusCode).toBe(200)
    expect(healthyResponse.json()).toEqual({
      status: "ready",
      checks: { database: true, schema: true, providerConfigAvailable: true, providerConfigured: true },
    })
    await healthy.app.close()

    class SchemaMismatchRepository extends TestRepository {
      override async readiness() { return { database: true, schema: false } }
    }
    const schemaApp = buildServer({
      config: loadConfig({ NODE_ENV: "test" }),
      repository: new SchemaMismatchRepository(),
      provider: new DeterministicProvider(),
      requestVerifier: new DevelopmentRequestVerifier(),
    })
    const schemaResponse = await schemaApp.inject({ method: "GET", url: "/internal/health/ready" })
    expect(schemaResponse.statusCode).toBe(503)
    expect(schemaResponse.json()).toMatchObject({ errorCode: "ai.database_schema_mismatch" })
    await schemaApp.close()

    const configApp = buildServer({
      config: loadConfig({ NODE_ENV: "test" }),
      repository: new TestRepository(),
      provider: new DeterministicProvider(),
      requestVerifier: new DevelopmentRequestVerifier(),
      remoteConfig: new RemoteConfigSnapshot("https://luna-api.internal", "callback-token-value"),
    })
    const configResponse = await configApp.inject({ method: "GET", url: "/internal/health/ready" })
    expect(configResponse.statusCode).toBe(503)
    expect(configResponse.json()).toMatchObject({ errorCode: "ai.provider_config_unavailable" })
    await configApp.close()

  })

  it("requires an authenticated actor", async () => {
    const { app } = fixture()
    const response = await app.inject({ method: "GET", url: "/internal/v1/conversations" })
    expect(response.statusCode).toBe(401)
    await app.close()
  })
  it("does not expose another user's conversation, timeline, Run, or events", async () => {
    const { app, repository } = fixture()
    const owner = "usr_isolation_owner"
    const intruderHeaders = { "x-luna-dev-user": "usr_isolation_intruder" }
    const conversation = await repository.createConversation(owner, "private conversation")
    const created = await repository.createTurn(owner, {
      conversationId: conversation.id,
      input: "private input",
      pageContext: {},
      idempotencyKey: "isolation-owner-turn",
    })

    for (const url of [
      `/internal/v1/conversations/${conversation.id}`,
      `/internal/v1/conversations/${conversation.id}/timeline`,
      `/internal/v1/runs/${created.run.id}`,
      `/internal/v1/runs/${created.run.id}/events?after=0&stream=false`,
    ]) {
      const response = await app.inject({ method: "GET", url, headers: intruderHeaders })
      expect(response.statusCode, url).toBe(404)
      expect(JSON.stringify(response.json())).not.toContain("private input")
    }
    await app.close()
  })
  it("creates a conversation and a durable turn", async () => {
    const { app, repository } = fixture()
    const headers = { "x-luna-dev-user": "usr_test" }
    const conversation = await app.inject({ method: "POST", url: "/internal/v1/conversations", headers, payload: { title: "构建诊断", modelId: "aimod_test" } })
    expect(conversation.statusCode).toBe(201)
    expect(conversation.json()).toMatchObject({ modelId: "aimod_test" })
    const id = conversation.json<{ id: string }>().id
    const turn = await app.inject({
      method: "POST", url: `/internal/v1/conversations/${id}/turns`,
      headers: {
        ...headers,
        "idempotency-key": "browser-request-1",
        traceparent: "00-abcdefabcdefabcdefabcdefabcdefab-0123456789abcdef-01",
      },
      payload: { modelId: "aimod_test", input: { parts: [{ type: "text", text: "为什么失败？" }] }, pageContext: { routeName: "application.builds" } },
    })
    expect(turn.statusCode).toBe(202)
    expect(turn.json()).toMatchObject({ state: "queued", turnIndex: 0 })
    const runId = turn.json<{ runId: string }>().runId
    expect((await repository.getRun("usr_test", runId))?.traceContext).toEqual({
      traceparent: "00-abcdefabcdefabcdefabcdefabcdefab-0123456789abcdef-01",
    })
    await app.close()
  })
  it("rejects an invalid immutable model snapshot", async () => {
    const { app } = fixture()
    const headers = { "x-luna-dev-user": "usr_snapshot_contract" }
    const conversation = await app.inject({ method: "POST", url: "/internal/v1/conversations", headers, payload: { modelId: "aimod_test" } })
    const id = conversation.json<{ id: string }>().id
    const response = await app.inject({
      method: "POST", url: `/internal/v1/conversations/${id}/turns`,
      headers: { ...headers, "idempotency-key": "invalid-model-snapshot" },
      payload: {
        modelId: "aimod_test",
        modelSnapshot: {
          id: "aimod_test", name: "test", maxContextTokens: 4096, maxOutputTokens: 4096,
          inputCreditsPerMillion: "0", outputCreditsPerMillion: "0",
          cachedInputCreditsPerMillion: "0",
        },
        input: { parts: [{ type: "text", text: "test" }] }, pageContext: {},
      },
    })
    expect(response.statusCode).toBe(400)
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
    const created = await app.inject({ method: "POST", url: "/internal/v1/conversations", headers, payload: { modelId: "aimod_test" } })
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
  it("persists model changes only on the selected conversation", async () => {
    const { app } = fixture()
    const headers = { "x-luna-dev-user": "usr_model_scope" }
    const first = await app.inject({
      method: "POST", url: "/internal/v1/conversations", headers,
      payload: { title: "First", modelId: "aimod_fast" },
    })
    const second = await app.inject({
      method: "POST", url: "/internal/v1/conversations", headers,
      payload: { title: "Second", modelId: "aimod_deep" },
    })
    const firstId = first.json<{ id: string }>().id
    const secondId = second.json<{ id: string }>().id

    const updated = await app.inject({
      method: "PATCH", url: `/internal/v1/conversations/${firstId}`, headers,
      payload: { modelId: "aimod_balanced" },
    })
    const untouched = await app.inject({ method: "GET", url: `/internal/v1/conversations/${secondId}`, headers })

    expect(updated.json()).toMatchObject({ id: firstId, modelId: "aimod_balanced" })
    expect(untouched.json()).toMatchObject({ id: secondId, modelId: "aimod_deep" })
    await app.close()
  })
  it("keeps a durable cancellation successful when the local abort hook fails", async () => {
    const repository = new TestRepository()
    const provider = new DeterministicProvider()
    const app = buildServer({
      config: loadConfig({ NODE_ENV: "test" }),
      repository,
      provider,
      requestVerifier: new DevelopmentRequestVerifier(),
      cancelRun: () => { throw new Error("local abort failed") },
    })
    const headers = { "x-luna-dev-user": "usr_cancel" }
    const conversation = await app.inject({ method: "POST", url: "/internal/v1/conversations", headers, payload: { title: "Cancel", modelId: "aimod_test" } })
    const conversationId = conversation.json<{ id: string }>().id
    const created = await app.inject({
      method: "POST",
      url: `/internal/v1/conversations/${conversationId}/turns`,
      headers: { ...headers, "idempotency-key": "cancel-request-1" },
      payload: { modelId: "aimod_test", input: { parts: [{ type: "text", text: "stop" }] }, pageContext: {} },
    })
    const runId = created.json<AITurnCreated>().runId
    const canceled = await app.inject({ method: "POST", url: `/internal/v1/runs/${runId}/cancel`, headers })
    expect(canceled.statusCode).toBe(202)
    expect(canceled.json()).toMatchObject({ id: runId, status: "canceled" })
    await app.close()
  })
  it("cancels a waiting Run directly without a live executor", async () => {
    const repository = new TestRepository()
    const bus = new InMemoryRunStreamBus(repository)
    const app = buildServer({
      config: loadConfig({ NODE_ENV: "test" }), repository,
      provider: new DeterministicProvider(), requestVerifier: new DevelopmentRequestVerifier(),
      streamBus: bus,
    })
    const ownerUserId = "usr_cancel_waiting"
    const headers = { "x-luna-dev-user": ownerUserId }
    const conversation = await repository.createConversation(ownerUserId, "Waiting")
    const created = await repository.createTurn(ownerUserId, {
      conversationId: conversation.id,
      input: "approve this",
      pageContext: {},
      idempotencyKey: "cancel-waiting-request",
    })
    await repository.updateRun(created.run.id, "queued", "waiting_approval")

    const canceled = await app.inject({ method: "POST", url: `/internal/v1/runs/${created.run.id}/cancel`, headers })

    expect(canceled.statusCode).toBe(202)
    expect(canceled.json()).toMatchObject({ id: created.run.id, status: "canceled" })
    expect((await repository.getEvents(ownerUserId, created.run.id, 0)).at(-1)?.type).toBe("run.canceled")
    await app.close()
  })
  it("returns Agent feature and limit metadata without a redundant health flag", async () => {
    const { app } = fixture()
    const response = await app.inject({
      method: "GET",
      url: "/internal/v1/capabilities",
      headers: { "x-luna-dev-user": "usr_test" },
    })
    expect(response.statusCode).toBe(200)
    const capabilities = response.json<Record<string, unknown>>()
    expect(capabilities).not.toHaveProperty("available")
    expect(capabilities).not.toHaveProperty("enabled")
    expect(capabilities).toMatchObject({
      features: {
        streaming: true,
        approvals: false,
        uiActions: true,
        longTermMemory: false,
      },
      limits: {
        maxInputBytes: defaultRuntimeSettings.maxInputBytes,
        maxConcurrentRuns: 10,
        maxUserConcurrentRuns: 10,
      },
    })
    await app.close()
  })
  it("reports only compatibility dimensions that still select runtime behavior", async () => {
    const { app } = fixture()
    const response = await app.inject({ method: "GET", url: "/internal/v1/health/compatibility" })

    expect(response.statusCode).toBe(200)
    expect(response.json()).toMatchObject({
      component: "luna-agent",
      internalApiVersions: ["v1"],
      aiSchemaMin: 1,
      aiSchemaMax: 1,
      promptVersions: ["system-v4"],
      toolCatalogDigest: "sha256:platform-tools-v1",
    })
    expect(response.json()).not.toHaveProperty("graphVersions")
    await app.close()
  })
  it("presents a created turn as the strict Web timeline contract", async () => {
    const { app, repository } = fixture()
    const headers = { "x-luna-dev-user": "usr_timeline" }
    const conversation = await app.inject({ method: "POST", url: "/internal/v1/conversations", headers, payload: { title: "Timeline", modelId: "aimod_test" } })
    const conversationId = conversation.json<{ id: string }>().id
    const created = await app.inject({
      method: "POST",
      url: `/internal/v1/conversations/${conversationId}/turns`,
      headers: { ...headers, "idempotency-key": "timeline-request-1" },
      payload: { modelId: "aimod_test", input: { parts: [{ type: "text", text: "检查构建状态" }] }, pageContext: {} },
    })
    const turnCreated: AITurnCreated = created.json<AITurnCreated>()
    const runId = turnCreated.runId
    expect(turnCreated.turnIndex).toBe(0)
    expect(turnCreated.eventsUrl).toBe(`/api/v1/ai/runs/${runId}/events`)

    const response = await app.inject({ method: "GET", url: `/internal/v1/conversations/${conversationId}/timeline`, headers })
    const timeline: AITimeline = response.json<AITimeline>()
    expect(response.statusCode).toBe(200)
    expect(response.headers["cache-control"]).toBe("no-store")
    expect(timeline).toMatchObject({
      conversation: { id: conversationId, title: "Timeline", status: "active", modelId: "aimod_test" },
      turns: [{
        turnIndex: 0,
        input: { type: "user_message", parts: [{ partIndex: 0, type: "text", text: "检查构建状态" }] },
        selectedRun: { id: runId, runIndex: 0, status: "queued", expectedVersion: 1, items: [] },
      }],
      eventCursors: [{ runId, after: 2 }],
      pageInfo: { hasOlder: false },
    })
    expect(Array.isArray(timeline.eventCursors)).toBe(true)
    const directlyTyped = await presentTimeline(repository, "usr_timeline", conversationId) as AITimeline | undefined
    expect(directlyTyped?.turns[0]?.selectedRun?.id).toBe(runId)
    const eventsResponse = await app.inject({ method: "GET", url: `/internal/v1/runs/${runId}/events?after=0&stream=false`, headers })
    const events = eventsResponse.json<{ items: AIEvent[], cursor: number }>()
    expect(events).toMatchObject({
      cursor: 2,
      items: [
        {
          version: 2,
          eventSequence: 1,
          type: "run.input_received",
          conversationId,
          turnId: turnCreated.turnId,
          runId,
          item: { id: `${turnCreated.turnId}:input`, type: "user_message", parts: [{ text: "检查构建状态" }] },
          payload: { initial: true, conversationTitle: "Timeline", conversationTitleSource: "user" },
        },
        {
          version: 2,
          eventSequence: 2,
          type: "run.queued",
          conversationId,
          turnId: turnCreated.turnId,
          runId,
          payload: { state: "queued" },
        },
      ],
    })
    await app.close()
  })
  it("paginates complete timeline turns toward older history with an opaque exclusive cursor", async () => {
    const { app, repository } = fixture()
    const ownerUserId = "usr_timeline_pages"
    const headers = { "x-luna-dev-user": ownerUserId }
    const conversation = await repository.createConversation(ownerUserId, "Long timeline")
    for (let turnIndex = 0; turnIndex < 33; turnIndex += 1) {
      await repository.createTurn(ownerUserId, {
        conversationId: conversation.id,
        input: `turn-${turnIndex}`,
        pageContext: {},
        idempotencyKey: `server-timeline-page-${turnIndex}`,
      })
    }

    const latestResponse = await app.inject({
      method: "GET",
      url: `/internal/v1/conversations/${conversation.id}/timeline?limit=10`,
      headers,
    })
    const latest = latestResponse.json<{
      turns: Array<{ turnIndex: number }>
      pageInfo: { hasOlder: boolean, olderCursor?: string }
    }>()
    expect(latestResponse.statusCode).toBe(200)
    expect(latest.turns.map(turn => turn.turnIndex)).toEqual([23, 24, 25, 26, 27, 28, 29, 30, 31, 32])
    expect(latest.pageInfo.hasOlder).toBe(true)
    expect(latest.pageInfo.olderCursor).toEqual(expect.any(String))

    const olderResponse = await app.inject({
      method: "GET",
      url: `/internal/v1/conversations/${conversation.id}/timeline?limit=10&before=${encodeURIComponent(latest.pageInfo.olderCursor!)}`,
      headers,
    })
    const older = olderResponse.json<{ turns: Array<{ turnIndex: number }> }>()
    expect(older.turns.map(turn => turn.turnIndex)).toEqual([13, 14, 15, 16, 17, 18, 19, 20, 21, 22])
    expect(new Set([...latest.turns, ...older.turns].map(turn => turn.turnIndex)).size).toBe(20)

    const invalidResponse = await app.inject({
      method: "GET",
      url: `/internal/v1/conversations/${conversation.id}/timeline?before=not-a-valid-cursor`,
      headers,
    })
    expect(invalidResponse.statusCode).toBe(400)
    expect(invalidResponse.headers["cache-control"]).toBe("no-store")
    expect(invalidResponse.json()).toMatchObject({ error: { code: "ai.timeline_cursor_invalid" } })

    const otherConversation = await repository.createConversation(ownerUserId, "Other timeline")
    const crossConversationResponse = await app.inject({
      method: "GET",
      url: `/internal/v1/conversations/${otherConversation.id}/timeline?before=${encodeURIComponent(latest.pageInfo.olderCursor!)}`,
      headers,
    })
    expect(crossConversationResponse.statusCode).toBe(400)
    expect(crossConversationResponse.json()).toMatchObject({ error: { code: "ai.timeline_cursor_invalid" } })
    await app.close()
  })
  it("applies conversation directory search and validates the requested sort order", async () => {
    const { app, repository } = fixture()
    const ownerUserId = "usr_conversation_search"
    const headers = { "x-luna-dev-user": ownerUserId }
    await repository.createConversation(ownerUserId, "Build diagnostics")
    await repository.createConversation(ownerUserId, "Deployment review")
    await repository.createConversation("usr_other", "Build from another owner")

    const response = await app.inject({
      method: "GET",
      url: "/internal/v1/conversations?search=build&sortBy=updatedAt&sortOrder=asc&page=1&pageSize=20",
      headers,
    })
    expect(response.statusCode).toBe(200)
    expect(response.json()).toMatchObject({
      total: 1,
      items: [{ title: "Build diagnostics" }],
      sortBy: "updatedAt",
      sortOrder: "asc",
    })

    const invalid = await app.inject({
      method: "GET",
      url: "/internal/v1/conversations?sortOrder=random",
      headers,
    })
    expect(invalid.statusCode).toBe(400)
    await app.close()
  })
  it("uses SSE when EventSource negotiates text/event-stream without a query flag", async () => {
    const { app } = fixture()
    const headers = { "x-luna-dev-user": "usr_sse" }
    const conversation = await app.inject({ method: "POST", url: "/internal/v1/conversations", headers, payload: { title: "SSE", modelId: "aimod_test" } })
    const conversationId = conversation.json<{ id: string }>().id
    const created = await app.inject({
      method: "POST",
      url: `/internal/v1/conversations/${conversationId}/turns`,
      headers: { ...headers, "idempotency-key": "sse-request-1" },
      payload: { modelId: "aimod_test", input: { parts: [{ type: "text", text: "hello" }] }, pageContext: {} },
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
})
