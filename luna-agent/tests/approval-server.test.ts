import { describe, expect, it } from "vitest"
import { DevelopmentRequestVerifier } from "../src/auth.js"
import { loadConfig } from "../src/config.js"
import { TestRepository } from "./support/test-repository.js"
import { DeterministicProvider } from "../src/provider/deterministic.js"
import { buildServer } from "../src/server.js"
import { ToolCatalog } from "../src/tools/catalog.js"
import { DeterministicLunaApiClient } from "../src/tools/luna-api-client.js"
import { MemoryToolCallStore, ProjectingToolCallStore, ToolOrchestrator } from "../src/tools/orchestrator.js"

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

async function approvalFixture() {
  const repository = new TestRepository()
  const store = new MemoryToolCallStore()
  const client = new DeterministicLunaApiClient(() => ({ status: 200, body: { ok: true } }))
  const tools = new ToolOrchestrator(
    approvalCatalog,
    client,
	new ProjectingToolCallStore(store, repository),
	undefined,
	repository,
	)
  const app = buildServer({
    config: loadConfig({ NODE_ENV: "test" }),
    repository,
    provider: new DeterministicProvider(),
    requestVerifier: new DevelopmentRequestVerifier(),
    tools,
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
})
