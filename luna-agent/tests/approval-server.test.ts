import { describe, expect, it } from "vitest"
import { DevelopmentAuthenticator } from "../src/auth.js"
import { loadConfig } from "../src/config.js"
import { PayloadCipher } from "../src/payload-cipher.js"
import { MemoryRepository } from "../src/persistence/memory.js"
import { DeterministicProvider } from "../src/provider/deterministic.js"
import { buildServer } from "../src/server.js"
import { ToolCatalog } from "../src/tools/catalog.js"
import { DeterministicLunaApiClient } from "../src/tools/luna-api-client.js"
import { MemoryToolCallStore, ProjectingToolCallStore, ToolOrchestrator } from "../src/tools/orchestrator.js"
import { responseToolContract } from "./tool-contract-fixtures.js"

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
  contract: responseToolContract({
    resourceTypes: ["thing"], action: "update", sideEffect: "platform-write", replaySafe: false,
    risk: "medium", approval: "always", avoidWhen: ["目标未确认时"], prerequisites: ["必须取得目标"],
  }),
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
  const grantCipher = new PayloadCipher(Buffer.alloc(32, 1))
  const tools = new ToolOrchestrator(
    approvalCatalog,
    client,
    new ProjectingToolCallStore(store, repository),
    undefined,
    undefined,
    async () => "opaque-run-grant",
    undefined,
    async runId => {
      const authorization = await repository.getRunConversationAuthorization(runId)
      return authorization ? grantCipher.decrypt(authorization.grantCiphertext) : undefined
    },
  )
  const app = buildServer({
    config: loadConfig({ NODE_ENV: "test" }),
    repository,
    provider: new DeterministicProvider(),
    authenticator: new DevelopmentAuthenticator(),
    grantCipher,
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

  it("authorizes all pending calls and later tools in the current conversation", async () => {
    const fixture = await approvalFixture()
    const runId = await waitingRun(fixture, "usr_owner", "approve-all")
    const first = await fixture.tools.propose({ runId, operationId: "updateThing", arguments: { value: "one" } })
    const second = await fixture.tools.propose({ runId, operationId: "updateThing", arguments: { value: "two" } })
    await fixture.repository.updateRun(runId, "running", "waiting_approval")

    const response = await fixture.app.inject({
      method: "POST",
      url: `/internal/v1/runs/${runId}/approvals/${first.id}/decision`,
      headers: { "x-luna-dev-user": "usr_owner" },
      payload: {
        decision: "approve_conversation",
        argumentsHash: first.argumentsHash,
        expectedVersion: first.rowVersion,
        conversationId: (await fixture.repository.getRun("usr_owner", runId))!.conversationId,
        conversationAuthorizationGrant: "conversation-authorization-grant",
        conversationAuthorizationExpiresAt: new Date(Date.now() + 60_000).toISOString(),
      },
    })

    expect(response.statusCode).toBe(202)
    expect(fixture.store.records.get(first.id)?.status).toBe("succeeded")
    expect(fixture.store.records.get(second.id)?.status).toBe("succeeded")
    expect(fixture.client.calls).toHaveLength(2)
    expect(fixture.client.calls.every(call => call.conversationAuthorizationGrant === "conversation-authorization-grant")).toBe(true)
    expect((await fixture.repository.getRun("usr_owner", runId))?.status).toBe("queued")

    const conversationId = (await fixture.repository.getRun("usr_owner", runId))!.conversationId
    const authorization = await fixture.app.inject({
      method: "GET",
      url: `/internal/v1/conversations/${conversationId}/authorization`,
      headers: { "x-luna-dev-user": "usr_owner" },
    })
    expect(authorization.statusCode).toBe(200)
    expect(authorization.json()).toMatchObject({ active: true })

    const later = await fixture.repository.createTurn("usr_owner", {
      conversationId,
      input: "later",
      pageContext: {},
      idempotencyKey: "approval-later",
    })
    const laterCall = await fixture.tools.propose({ runId: later.run.id, operationId: "updateThing", arguments: { value: "later" } })
    expect(laterCall.status).toBe("succeeded")
    expect(fixture.client.calls.at(-1)?.conversationAuthorizationGrant).toBe("conversation-authorization-grant")

    const revoked = await fixture.app.inject({
      method: "DELETE",
      url: `/internal/v1/conversations/${conversationId}/authorization`,
      headers: { "x-luna-dev-user": "usr_owner" },
    })
    expect(revoked.statusCode).toBe(204)
    const afterRevoke = await fixture.repository.createTurn("usr_owner", {
      conversationId,
      input: "after revoke",
      pageContext: {},
      idempotencyKey: "approval-after-revoke",
    })
    await expect(fixture.tools.propose({ runId: afterRevoke.run.id, operationId: "updateThing", arguments: { value: "after-revoke" } }))
      .resolves.toMatchObject({ status: "awaiting_approval" })
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
