import { describe, expect, it } from "vitest"
import { DevelopmentRequestVerifier } from "../src/auth.js"
import { loadConfig } from "../src/config.js"
import { TestRepository } from "./support/test-repository.js"
import { DeterministicProvider } from "../src/provider/deterministic.js"
import { buildServer } from "../src/server.js"
import { ToolCatalog } from "../src/tools/catalog.js"
import { DeterministicLunaApiClient, type ToolExecutionResult } from "../src/tools/luna-api-client.js"
import { ProjectingToolCallStore, ToolOrchestrator } from "../src/tools/orchestrator.js"
import { presentTimeline } from "../src/timeline-presenter.js"

const approvalCatalog = ToolCatalog.load([{
  operationId: "updateThing",
  method: "POST",
  path: "/api/v1/things",
  category: "thing",
  requiresApproval: true,
  requiredScopes: ["project:write"],
  idempotent: true,
  timeoutMs: 1000,
  inputSchema: {
    type: "object",
    properties: { value: { type: "string" } },
    required: ["value"],
    additionalProperties: false,
  },
}])

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
  const app = buildServer({
    config: loadConfig({ NODE_ENV: "test" }),
    repository,
    provider: new DeterministicProvider(),
    requestVerifier: new DevelopmentRequestVerifier(),
    tools,
    cancelRun: async (runId) => {
      await observeCancel?.(runId, repository)
      return tools.cancelRun(runId)
    },
  })
  return { app, repository, store, client, tools }
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

	it("authorizes only the selected parameter-bound call", async () => {
	  const fixture = await approvalFixture()
	  const runId = await waitingRun(fixture, "usr_owner", "approve-one")
	  const first = await fixture.tools.propose({ runId, operationId: "updateThing", arguments: { value: "one" } })
	  await fixture.repository.updateRun(runId, "running", "waiting_approval")

    const response = await fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${runId}/approvals/${first.id}/decision`,
      headers: { "x-luna-dev-user": "usr_owner" },
	  payload: { decision: "approve" },
	})

	expect(response.statusCode).toBe(202)
	expect(fixture.store.records.get(first.id)?.status).toBe("succeeded")
	expect(fixture.client.calls).toHaveLength(1)
	expect(fixture.store.records.get(first.id)?.approvalDecision).toBe("approve")
	expect((await fixture.repository.getRun("usr_owner", runId))?.status).toBe("queued")

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

  it("rejects only the selected ToolCall and requeues the Run", async () => {
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
      runId,
      operationId: "updateThing",
      status: "succeeded",
      arguments: { value: "done" },
      argumentsHash: "sha256:terminal",
      attempt: 1,
      rowVersion: 2,
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

  it("keeps the Run canceled when cancellation races an approved tool execution", async () => {
    let receivedSignal: AbortSignal | undefined
    let statusAtAbort: string | undefined
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
    const fixture = await approvalFixture(client, async (runId, repository) => {
      statusAtAbort = (await repository.getRun("usr_owner", runId))?.status
    })
    const runId = await waitingRun(fixture, "usr_owner", "cancel-approved")
    const pending = await fixture.tools.propose({ runId, operationId: "updateThing", arguments: { value: "stop" } })
    await fixture.repository.updateRun(runId, "running", "waiting_approval")

    const approvalResponse = fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${runId}/approvals/${pending.id}/decision`,
      headers: { "x-luna-dev-user": "usr_owner" },
      payload: { decision: "approve" },
    })
    await started
    const cancelResponse = await fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${runId}/cancel`,
      headers: { "x-luna-dev-user": "usr_owner" },
    })
    const approvalResult = await approvalResponse

    expect(cancelResponse.statusCode).toBe(202)
    expect(cancelResponse.json()).toMatchObject({ id: runId, status: "canceled" })
    expect(approvalResult.statusCode).toBe(409)
    expect(statusAtAbort).toBe("canceled")
    expect(receivedSignal?.aborted).toBe(true)
    expect(fixture.store.records.get(pending.id)).toMatchObject({ status: "canceled", errorCode: "ai.run_canceled" })
    expect((await fixture.repository.getRun("usr_owner", runId))?.status).toBe("canceled")
    await fixture.app.close()
  })

  it("does not abort a tool after a concurrent completion becomes authoritative", async () => {
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
    await fixture.repository.updateRun(runId, "running", "waiting_approval")
    fixture.repository.cancelRun = async (ownerUserId, id) => {
      await fixture.repository.updateRun(id, "running", "completed", { completedAt: new Date().toISOString() })
      return fixture.repository.getRun(ownerUserId, id)
    }

    const approvalResponse = fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${runId}/approvals/${pending.id}/decision`,
      headers: { "x-luna-dev-user": "usr_owner" },
      payload: { decision: "approve" },
    })
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
    const approvalResult = await approvalResponse
    expect(approvalResult.statusCode).toBe(409)
    expect((await fixture.repository.getRun("usr_owner", runId))?.status).toBe("completed")
    await fixture.app.close()
  })
})
