import { describe, expect, it } from "vitest"
import { DevelopmentAuthenticator } from "../src/auth.js"
import { loadConfig } from "../src/config.js"
import { RunGrantCipher } from "../src/grant-cipher.js"
import { MemoryRepository } from "../src/persistence/memory.js"
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
  risk: "write",
  requiredScopes: ["project:write"],
  approval: "always",
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
  const repository = new MemoryRepository()
  const store = new MemoryToolCallStore()
  const client = new DeterministicLunaApiClient(() => ({ status: 200, body: { ok: true } }))
  const tools = new ToolOrchestrator(
    approvalCatalog,
    client,
    new ProjectingToolCallStore(store, repository),
    undefined,
    12,
    undefined,
    async () => "opaque-run-grant",
  )
  const app = buildServer({
    config: loadConfig({ NODE_ENV: "test" }),
    repository,
    provider: new DeterministicProvider(),
    authenticator: new DevelopmentAuthenticator(),
    graphVersions: ["assistant-v1"],
    grantCipher: new RunGrantCipher(Buffer.alloc(32, 1)),
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
      payload: { decision: "approve", argumentsHash: second.argumentsHash, expectedVersion: second.rowVersion },
    })

    expect(response.statusCode).toBe(404)
    expect(fixture.store.records.get(first.id)?.status).toBe("awaiting_approval")
    expect(fixture.store.records.get(second.id)?.status).toBe("awaiting_approval")
    await fixture.app.close()
  })

  it("approves only already pending calls in the current run", async () => {
    const fixture = await approvalFixture()
    const runId = await waitingRun(fixture, "usr_owner", "approve-all")
    const first = await fixture.tools.propose({ runId, operationId: "updateThing", arguments: { value: "one" } })
    const second = await fixture.tools.propose({ runId, operationId: "updateThing", arguments: { value: "two" } })
    await fixture.repository.updateRun(runId, "running", "waiting_approval")

    const response = await fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${runId}/approvals/${first.id}/decision`,
      headers: { "x-luna-dev-user": "usr_owner" },
      payload: { decision: "approve_all", argumentsHash: first.argumentsHash, expectedVersion: first.rowVersion },
    })

    expect(response.statusCode).toBe(202)
    expect(fixture.store.records.get(first.id)?.status).toBe("succeeded")
    expect(fixture.store.records.get(second.id)?.status).toBe("succeeded")
    expect(fixture.client.calls).toHaveLength(2)
    expect((await fixture.repository.getRun("usr_owner", runId))?.status).toBe("queued")
    await fixture.app.close()
  })

  it("rejects the bound call and cancels the run", async () => {
    const fixture = await approvalFixture()
    const runId = await waitingRun(fixture, "usr_owner", "reject")
    const pending = await fixture.tools.propose({ runId, operationId: "updateThing", arguments: { value: "no" } })
    await fixture.repository.updateRun(runId, "running", "waiting_approval")

    const response = await fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${runId}/approvals/${pending.id}/decision`,
      headers: { "x-luna-dev-user": "usr_owner" },
      payload: { decision: "reject", argumentsHash: pending.argumentsHash, expectedVersion: pending.rowVersion },
    })

    expect(response.statusCode).toBe(200)
    expect(fixture.store.records.get(pending.id)?.status).toBe("canceled")
    expect((await fixture.repository.getRun("usr_owner", runId))?.status).toBe("canceled")
    await fixture.app.close()
  })
})
