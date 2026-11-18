import { describe, expect, it } from "vitest"
import { DevelopmentRequestVerifier } from "../src/auth.js"
import { loadConfig } from "../src/config.js"
import { ModelRuntime } from "../src/model-runtime.js"
import { DeterministicProvider } from "../src/provider/deterministic.js"
import { TestRepository } from "./support/test-repository.js"
import { ToolCatalog } from "../src/tools/catalog.js"
import { DeterministicLunaApiClient, type ToolExecutionResult } from "../src/tools/luna-api-client.js"
import { ProjectingToolCallStore, ToolOrchestrator } from "../src/tools/orchestrator.js"
import { presentTimeline } from "../src/timeline-presenter.js"
import { testRegistry } from "./support/model-tool-registry.js"
import { testExecutor } from "./support/run-executor.js"
import { testToolOperation } from "./support/tool-catalog.js"
import { buildTestServer } from "./support/server.js"

const approvalCatalog = ToolCatalog.load([
  {
    ...testToolOperation("updateThing"),
    operationId: "updateThing",
    method: "POST",
    path: "/api/v1/things",
    category: "thing",
    requiresApproval: true,
    requiredScopes: ["project:write"],
    idempotent: true,
    timeoutMs: 1000,
    effect: "write",
    verification: [{ operationId: "getThingState", family: "resource", singleton: true }],
    inputSchema: {
      type: "object",
      properties: { value: { type: "string" } },
      required: ["value"],
      additionalProperties: false,
    },
  },
  {
    ...testToolOperation("getThingState"),
    operationId: "getThingState",
    method: "GET",
    path: "/api/v1/things/state",
    category: "thing",
    requiredScopes: ["project:read"],
  },
  {
    ...testToolOperation("deployTemplate"),
    operationId: "deployTemplate",
    method: "POST",
    path: "/api/v1/templates/deploy",
    category: "thing",
    approvalTier: "L1",
    requiresApproval: true,
    requiredScopes: ["project:write"],
    idempotent: true,
    timeoutMs: 1000,
    effect: "write",
    verification: [{ operationId: "getThingState", family: "resource", singleton: true }],
    inputSchema: {
      type: "object",
      properties: {
        stage: { type: "string" },
        name: { type: "string" },
      },
      required: ["stage", "name"],
      additionalProperties: false,
    },
  },
])

async function approvalFixture(
  client = new DeterministicLunaApiClient(() => ({ status: 200, body: { ok: true } })),
  observeCancel?: (runId: string, repository: TestRepository) => void | Promise<void>,
) {
  const repository = new TestRepository()
  const store = repository.toolCallStore
  const tools = new ToolOrchestrator(
    approvalCatalog,
    client,
    new ProjectingToolCallStore(store, repository),
  )
  const executor = testExecutor(
    repository,
    new ModelRuntime(new DeterministicProvider(), testRegistry()),
    { catalog: approvalCatalog, tools },
  )
  const app = buildTestServer({
    config: loadConfig({ NODE_ENV: "test" }),
    repository,
    requestVerifier: new DevelopmentRequestVerifier(),
    tools,
    cancelExecutingRun: runId => executor.cancelExecuting(runId),
    cancelRun: async (runId) => {
      await observeCancel?.(runId, repository)
      return executor.cancel(runId)
    },
  })
  return { app, repository, store, client, tools, executor }
}

async function waitingRun(fixture: Awaited<ReturnType<typeof approvalFixture>>, userId: string, suffix: string) {
  const conversation = await fixture.repository.createConversation(userId, suffix)
  const created = await fixture.repository.createTurn(userId, {
    conversationId: conversation.id,
    input: suffix,
    pageContext: {},
    idempotencyKey: `approval-${suffix}`,
  })
  await fixture.repository.updateRun(created.run.id, "queued", "running")
  return created.run.id
}

describe("approval decisions", () => {
  it("binds a tool call to its own run", async () => {
    const fixture = await approvalFixture()
    const firstRun = await waitingRun(fixture, "usr_owner", "first")
    const secondRun = await waitingRun(fixture, "usr_owner", "second")
    const first = await fixture.tools.propose({ runId: firstRun, operationId: "updateThing", arguments: { value: "one" } })
    const second = await fixture.tools.propose({ runId: secondRun, operationId: "updateThing", arguments: { value: "two" } })
    await fixture.repository.updateRun(firstRun, "running", "waiting_approval")
    await fixture.repository.updateRun(secondRun, "running", "waiting_approval")

    const response = await fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${firstRun}/approvals/${second.id}/decision`,
      headers: { "x-luna-dev-user": "usr_owner" },
      payload: { decision: "approve" },
    })

    expect(response.statusCode).toBe(404)
    expect(fixture.store.records.get(first.id)?.status).toBe("awaiting_approval")
    expect(fixture.store.records.get(second.id)?.status).toBe("awaiting_approval")
    await fixture.app.close()
  })

  it("enqueues approval asynchronously with idempotent duplicate protection and executes only via RunExecutor", async () => {
    const fixture = await approvalFixture()
    const runId = await waitingRun(fixture, "usr_owner", "approve-one")
    const first = await fixture.tools.propose({ runId, operationId: "updateThing", arguments: { value: "one" } })
    await fixture.repository.saveRunCheckpoint(runId, {
      expectedRevision: 0,
      phase: "awaiting_approval",
      modelStep: 1,
      toolCallCount: 0,
      state: { pendingToolCallId: first.id },
    })
    await fixture.repository.updateRun(runId, "running", "waiting_approval")

    const response = await fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${runId}/approvals/${first.id}/decision`,
      headers: { "x-luna-dev-user": "usr_owner" },
      payload: { decision: "approve" },
    })

    expect(response.statusCode).toBe(202)
    expect(response.json()).toEqual({ runId, state: "queued" })
    // HTTP 层不执行平台工具 HTTP 调用。
    expect(fixture.client.calls).toHaveLength(0)
    expect(fixture.store.records.get(first.id)).toMatchObject({
      status: "proposed",
      approvalDecision: "approve",
    })
    expect((await fixture.repository.getRun("usr_owner", runId))?.status).toBe("queued")

    // 重复点击同一批准幂等返回 202，不抛 500 或 409。
    const duplicate = await fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${runId}/approvals/${first.id}/decision`,
      headers: { "x-luna-dev-user": "usr_owner" },
      payload: { decision: "approve" },
    })
    expect(duplicate.statusCode).toBe(202)
    expect(duplicate.json()).toEqual({ runId, state: "queued" })
    expect(fixture.client.calls).toHaveLength(0)

    await fixture.executor.runOnce()
    expect(fixture.client.calls).toHaveLength(1)
    expect(fixture.store.records.get(first.id)?.status).toBe("succeeded")

    const conversationId = (await fixture.repository.getRun("usr_owner", runId))!.conversationId
    const later = await fixture.repository.createTurn("usr_owner", {
      conversationId,
      input: "later",
      pageContext: {},
      idempotencyKey: "approval-later",
    })
    const laterCall = await fixture.tools.propose({ runId: later.run.id, operationId: "updateThing", arguments: { value: "later" } })
    expect(laterCall.status).toBe("awaiting_approval")
    expect(fixture.client.calls).toHaveLength(1)
    await fixture.app.close()
  })

  it("rejects only the selected ToolCall idempotently and requeues the Run", async () => {
    const fixture = await approvalFixture()
    const runId = await waitingRun(fixture, "usr_owner", "reject")
    const pending = await fixture.tools.propose({ runId, operationId: "updateThing", arguments: { value: "no" } })
    await fixture.repository.updateRun(runId, "running", "waiting_approval")

    const response = await fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${runId}/approvals/${pending.id}/decision`,
      headers: { "x-luna-dev-user": "usr_owner" },
      payload: { decision: "reject" },
    })

    expect(response.statusCode).toBe(202)
    expect(fixture.store.records.get(pending.id)?.status).toBe("rejected")
    expect((await fixture.repository.getRun("usr_owner", runId))?.status).toBe("queued")

    const duplicate = await fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${runId}/approvals/${pending.id}/decision`,
      headers: { "x-luna-dev-user": "usr_owner" },
      payload: { decision: "reject" },
    })
    expect(duplicate.statusCode).toBe(202)
    expect(duplicate.json()).toEqual({ runId, state: "queued" })
    await fixture.app.close()
  })

  it("cancels pending approvals and makes stale approval decisions non-resumable", async () => {
    const fixture = await approvalFixture()
    const runId = await waitingRun(fixture, "usr_owner", "cancel-pending")
    const pending = await fixture.tools.propose({ runId, operationId: "updateThing", arguments: { value: "pending" } })
    await fixture.repository.updateRun(runId, "running", "waiting_approval")
    const run = await fixture.repository.getRun("usr_owner", runId)
    if (!run) throw new Error("missing run")
    const terminalId = "aitool_terminal_existing"
    await fixture.store.insert({
      id: terminalId,
      idempotencyKey: terminalId,
      runId,
      operationId: "updateThing",
      status: "succeeded",
      arguments: { value: "done" },
      argumentsHash: "sha256:terminal",
      attempt: 1,
      rowVersion: 2,
      executionCount: 1,
      result: { ok: true },
    })
    await fixture.repository.appendItem({
      id: `${terminalId}:item`,
      runId,
      turnId: run.turnId,
      type: "tool_call",
      status: "completed",
      content: { toolCallId: terminalId, operationId: "updateThing", status: "succeeded", result: { ok: true } },
    })

    const canceled = await fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${runId}/cancel`,
      headers: { "x-luna-dev-user": "usr_owner" },
    })
    const staleDecision = await fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${runId}/approvals/${pending.id}/decision`,
      headers: { "x-luna-dev-user": "usr_owner" },
      payload: { decision: "approve" },
    })

    expect(canceled.statusCode).toBe(202)
    expect(fixture.store.records.get(pending.id)).toMatchObject({ status: "canceled", errorCode: "ai.run_canceled" })
    expect(fixture.store.records.get(terminalId)).toMatchObject({ status: "succeeded", result: { ok: true } })
    expect(staleDecision.statusCode).toBe(409)
    expect(staleDecision.json()).toMatchObject({ error: { code: "ai.run_not_resumable" } })
    const timeline = await presentTimeline(fixture.repository, "usr_owner", run.conversationId)
    const pendingItem = timeline?.turns[0]?.selectedRun?.items.find(item => item.id === `${pending.id}:item`)
    const terminalItem = timeline?.turns[0]?.selectedRun?.items.find(item => item.id === `${terminalId}:item`)
    expect(pendingItem).toMatchObject({ status: "completed", toolCall: { id: pending.id, status: "canceled", errorCode: "ai.run_canceled" } })
    expect(terminalItem).toMatchObject({ status: "completed", toolCall: { id: terminalId, status: "succeeded" } })
    const events = await fixture.repository.getEvents("usr_owner", runId, 0)
    expect(events.findIndex(event => event.type === "item.finalized")).toBeLessThan(events.findIndex(event => event.type === "run.canceled"))
    expect(fixture.client.calls).toHaveLength(0)
    await fixture.app.close()
  })

  it("keeps the Run canceled when cancellation races RunExecutor executing an approved tool", async () => {
    let receivedSignal: AbortSignal | undefined
    let markStarted!: () => void
    const started = new Promise<void>((resolve) => { markStarted = resolve })
    const client = new DeterministicLunaApiClient(request => {
      receivedSignal = request.signal
      markStarted()
      return new Promise((_, reject) => {
        const signal = request.signal
        if (!signal) return reject(new Error("ai.tool_signal_missing"))
        const onAbort = () => reject(signal.reason instanceof Error ? signal.reason : new Error("ai.run_canceled"))
        if (signal.aborted) onAbort()
        else signal.addEventListener("abort", onAbort, { once: true })
      })
    })
    const fixture = await approvalFixture(client)
    const runId = await waitingRun(fixture, "usr_owner", "cancel-approved")
    const pending = await fixture.tools.propose({ runId, operationId: "updateThing", arguments: { value: "stop" } })
    await fixture.repository.saveRunCheckpoint(runId, {
      expectedRevision: 0,
      phase: "awaiting_approval",
      modelStep: 1,
      toolCallCount: 0,
      state: { pendingToolCallId: pending.id },
    })
    await fixture.repository.updateRun(runId, "running", "waiting_approval")

    const approvalResponse = await fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${runId}/approvals/${pending.id}/decision`,
      headers: { "x-luna-dev-user": "usr_owner" },
      payload: { decision: "approve" },
    })
    expect(approvalResponse.statusCode).toBe(202)
    expect(fixture.client.calls).toHaveLength(0)

    const runningPromise = fixture.executor.runOnce()
    await started
    const cancelResponse = await fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${runId}/cancel`,
      headers: { "x-luna-dev-user": "usr_owner" },
    })
    await runningPromise

    expect(cancelResponse.statusCode).toBe(202)
    expect(cancelResponse.json()).toMatchObject({ id: runId, status: "canceled" })
    expect(receivedSignal?.aborted).toBe(true)
    expect((await fixture.repository.getRun("usr_owner", runId))?.status).toBe("canceled")
    await fixture.app.close()
  })

  it("does not abort an approved tool in RunExecutor after a concurrent completion becomes authoritative", async () => {
    let receivedSignal: AbortSignal | undefined
    let markStarted!: () => void
    let completeTool!: () => void
    const started = new Promise<void>((resolve) => { markStarted = resolve })
    const client = new DeterministicLunaApiClient(request => {
      receivedSignal = request.signal
      markStarted()
      return new Promise<ToolExecutionResult>((resolve) => {
        completeTool = () => resolve({ status: 200, body: { ok: true } })
      })
    })
    const fixture = await approvalFixture(client)
    const runId = await waitingRun(fixture, "usr_owner", "complete-before-cancel")
    const pending = await fixture.tools.propose({ runId, operationId: "updateThing", arguments: { value: "finish" } })
    await fixture.repository.saveRunCheckpoint(runId, {
      expectedRevision: 0,
      phase: "awaiting_approval",
      modelStep: 1,
      toolCallCount: 0,
      state: { pendingToolCallId: pending.id },
    })
    await fixture.repository.updateRun(runId, "running", "waiting_approval")
    fixture.executor.cancelExecuting = async () => false
    fixture.repository.cancelRun = async (ownerUserId, id) => {
      await fixture.repository.updateRun(id, "running", "completed", { completedAt: new Date().toISOString() })
      return fixture.repository.getRun(ownerUserId, id)
    }

    const approvalResponse = await fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${runId}/approvals/${pending.id}/decision`,
      headers: { "x-luna-dev-user": "usr_owner" },
      payload: { decision: "approve" },
    })
    expect(approvalResponse.statusCode).toBe(202)

    const runningPromise = fixture.executor.runOnce()
    await started
    const cancelResponse = await fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${runId}/cancel`,
      headers: { "x-luna-dev-user": "usr_owner" },
    })

    expect(cancelResponse.statusCode).toBe(202)
    expect(cancelResponse.json()).toMatchObject({ id: runId, status: "completed" })
    expect(receivedSignal?.aborted).toBe(false)
    completeTool()
    await runningPromise
    expect((await fixture.repository.getRun("usr_owner", runId))?.status).toBe("completed")
    await fixture.app.close()
  })

  it("remembers L1 approve_and_remember within the conversation, isolates other conversations, and escalates stage=prod or L2 to per-call approval", async () => {
    const fixture = await approvalFixture()
    const conversation = await fixture.repository.createConversation("usr_owner", "L1 session memory")
    const turn1 = await fixture.repository.createTurn("usr_owner", {
      conversationId: conversation.id,
      input: "第一次安装 dev 模板",
      pageContext: {},
      idempotencyKey: "approval-l1-turn1",
    })
    await fixture.repository.updateRun(turn1.run.id, "queued", "running")

    const firstL1 = await fixture.tools.propose({
      runId: turn1.run.id,
      operationId: "deployTemplate",
      arguments: { stage: "dev", name: "redis-1" },
    })
    expect(firstL1).toMatchObject({
      status: "awaiting_approval",
      approvalTier: "L1",
      approvalRememberable: true,
    })
    await fixture.repository.updateRun(turn1.run.id, "running", "waiting_approval")

    const rememberRes = await fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${turn1.run.id}/approvals/${firstL1.id}/decision`,
      headers: { "x-luna-dev-user": "usr_owner" },
      payload: { decision: "approve_and_remember" },
    })
    expect(rememberRes.statusCode).toBe(202)
    expect(fixture.store.records.get(firstL1.id)?.approvalDecision).toBe("approve_and_remember")

    // Subsequent L1 call in the SAME conversation executes automatically with approvalDecision="remembered"
    const turn2 = await fixture.repository.createTurn("usr_owner", {
      conversationId: conversation.id,
      input: "第二次安装 test 模板",
      pageContext: {},
      idempotencyKey: "approval-l1-turn2",
    })
    await fixture.repository.updateRun(turn2.run.id, "queued", "running")
    const secondL1 = await fixture.tools.propose({
      runId: turn2.run.id,
      operationId: "deployTemplate",
      arguments: { stage: "test", name: "redis-2" },
    })
    expect(secondL1).toMatchObject({
      status: "succeeded",
      approvalDecision: "remembered",
      approvalTier: "L1",
    })

    // Same operationId with stage="prod" dynamically escalates to L2, ignores session memory, and rejects approve_and_remember
    const prodEscalated = await fixture.tools.propose({
      runId: turn2.run.id,
      operationId: "deployTemplate",
      arguments: { stage: "prod", name: "redis-prod" },
    })
    expect(prodEscalated).toMatchObject({
      status: "awaiting_approval",
      approvalTier: "L2",
      approvalRememberable: false,
    })
    await fixture.repository.updateRun(turn2.run.id, "running", "waiting_approval")

    const timelineRes = await fixture.app.inject({
      method: "GET",
      url: `/internal/v1/conversations/${conversation.id}/timeline`,
      headers: { "x-luna-dev-user": "usr_owner" },
    })
    const timeline = timelineRes.json<{
      turns: Array<{ selectedRun?: { items: Array<{ toolCall?: Record<string, unknown> }> } }>
    }>()
    const prodTimelineItem = timeline.turns.flatMap(t => t.selectedRun?.items ?? []).find(item => item.toolCall?.id === prodEscalated.id)
    expect(prodTimelineItem?.toolCall).toMatchObject({
      approvalTier: "L2",
      approvalRememberable: false,
    })

    const forbiddenRememberRes = await fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${turn2.run.id}/approvals/${prodEscalated.id}/decision`,
      headers: { "x-luna-dev-user": "usr_owner" },
      payload: { decision: "approve_and_remember" },
    })
    expect(forbiddenRememberRes.statusCode).toBe(409)
    expect(forbiddenRememberRes.json()).toMatchObject({
      error: { code: "ai.approval_remember_not_allowed" },
    })

    // Static L2 operation also rejects approve_and_remember
    const otherRunId = await waitingRun(fixture, "usr_owner", "other-conversation")
    const otherConvL1 = await fixture.tools.propose({
      runId: otherRunId,
      operationId: "deployTemplate",
      arguments: { stage: "dev", name: "isolated" },
    })
    expect(otherConvL1.status).toBe("awaiting_approval")

    const staticL2 = await fixture.tools.propose({
      runId: otherRunId,
      operationId: "updateThing",
      arguments: { value: "high-risk" },
    })
    expect(staticL2).toMatchObject({
      status: "awaiting_approval",
      approvalTier: "L2",
      approvalRememberable: false,
    })
    await fixture.repository.updateRun(otherRunId, "running", "waiting_approval")
    const staticL2RememberRes = await fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${otherRunId}/approvals/${staticL2.id}/decision`,
      headers: { "x-luna-dev-user": "usr_owner" },
      payload: { decision: "approve_and_remember" },
    })
    expect(staticL2RememberRes.statusCode).toBe(409)
    expect(staticL2RememberRes.json()).toMatchObject({
      error: { code: "ai.approval_remember_not_allowed" },
    })

    // Lost CAS race after rejection must NOT pollute session memory
    const rejectFirstRes = await fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${otherRunId}/approvals/${otherConvL1.id}/decision`,
      headers: { "x-luna-dev-user": "usr_owner" },
      payload: { decision: "reject" },
    })
    expect(rejectFirstRes.statusCode).toBe(202)
    const lateRememberRes = await fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${otherRunId}/approvals/${otherConvL1.id}/decision`,
      headers: { "x-luna-dev-user": "usr_owner" },
      payload: { decision: "approve_and_remember" },
    })
    expect(lateRememberRes.statusCode).toBe(409)
    expect(await fixture.repository.hasRememberedToolApproval(otherRunId, "deployTemplate")).toBe(false)

    await fixture.app.close()
  })

  it("clears approvalDecision and escalates to L2 when a remembered L1 call hits HTTP 428 ai.approval_required", async () => {
    let attempts = 0
    const client = new DeterministicLunaApiClient((request) => {
      if (request.operation.operationId === "getThingState") return { status: 200, body: { ready: true } }
      attempts += 1
      if (attempts === 1) {
        return { status: 428, body: { code: "ai.approval_required" } }
      }
      return { status: 200, body: { deployed: true } }
    })
    const fixture = await approvalFixture(client)
    const conversation = await fixture.repository.createConversation("usr_owner", "428 Escalation")
    const turn = await fixture.repository.createTurn("usr_owner", {
      conversationId: conversation.id,
      input: "安装模板",
      pageContext: {},
      idempotencyKey: "approval-428-escalation",
    })
    await fixture.repository.updateRun(turn.run.id, "queued", "running")
    await fixture.repository.rememberToolApproval(turn.run.id, "deployTemplate", "aitool_seed_grant")

    const escalated = await fixture.tools.propose({
      runId: turn.run.id,
      operationId: "deployTemplate",
      arguments: { stage: "dev", name: "escalated-service" },
    })
    expect(escalated).toMatchObject({
      status: "awaiting_approval",
      approvalTier: "L2",
      approvalRememberable: false,
    })
    expect(escalated.approvalDecision).toBeUndefined()
    await fixture.repository.updateRun(turn.run.id, "running", "waiting_approval")

    const rememberAgainRes = await fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${turn.run.id}/approvals/${escalated.id}/decision`,
      headers: { "x-luna-dev-user": "usr_owner" },
      payload: { decision: "approve_and_remember" },
    })
    expect(rememberAgainRes.statusCode).toBe(409)
    expect(rememberAgainRes.json()).toMatchObject({
      error: { code: "ai.approval_remember_not_allowed" },
    })

    const approveOnceRes = await fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${turn.run.id}/approvals/${escalated.id}/decision`,
      headers: { "x-luna-dev-user": "usr_owner" },
      payload: { decision: "approve" },
    })
    expect(approveOnceRes.statusCode).toBe(202)
    expect(fixture.store.records.get(escalated.id)).toMatchObject({
      status: "proposed",
      approvalDecision: "approve",
    })

    await fixture.repository.updateRun(turn.run.id, "queued", "running")
    const executed = await fixture.tools.executePendingCall(escalated.id)
    expect(executed.status).toBe("succeeded")
    expect(attempts).toBe(2)

    await fixture.app.close()
  })
})
