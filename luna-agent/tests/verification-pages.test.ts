import { describe, expect, it } from "vitest"
import type { RunToolVerification } from "../src/domain.js"
import { ToolVerificationRunner, evaluateVerification } from "../src/executor/verification.js"
import { ToolCatalog } from "../src/tools/catalog.js"
import { DeterministicLunaApiClient } from "../src/tools/luna-api-client.js"
import { MemoryToolCallStore, ToolOrchestrator } from "../src/tools/orchestrator.js"
import { testToolOperation } from "./support/tool-catalog.js"

const plan: RunToolVerification = {
  runId: "run_pages", sourceToolCallId: "source", sourceOperationId: "deleteThing",
  operationId: "listThings", family: "resource", expectation: "absent",
  arguments: { page: 1, pageSize: 100 }, externalOperationId: "target", attempts: 0,
  resource: { collection: true, singleton: false, targetField: "id" },
}

function setup(respond: ConstructorParameters<typeof DeterministicLunaApiClient>[0]) {
  const catalog = ToolCatalog.load([testToolOperation("listThings", {
    effect: "read",
    parameters: [
      { inputName: "page", wireName: "page", in: "query", required: false },
      { inputName: "pageSize", wireName: "pageSize", in: "query", required: false },
    ],
    inputSchema: {
      type: "object", properties: { page: { type: "integer" }, pageSize: { type: "integer" } },
      required: [], additionalProperties: false,
    },
  })])
  const client = new DeterministicLunaApiClient(respond)
  const store = new MemoryToolCallStore()
  const runner = new ToolVerificationRunner(new ToolOrchestrator(catalog, client, store))
  return { runner, client, store }
}

describe("paginated authoritative verification", () => {
  it.each(["resource", "deployment_target"] as const)("finds a %s target on later pages", async (family) => {
    const { runner, client, store } = setup(request => ({ status: 200, body: {
      page: request.arguments.page, totalPages: 2,
      items: request.arguments.page === 2 ? [{ id: "target" }] : [{ id: "other" }],
    } }))
    expect(await runner.verify({ ...plan, family }, new AbortController().signal)).toMatchObject({ kind: "pending" })
    expect(client.calls.map(call => call.arguments.page)).toEqual([1, 2])
    expect(await runner.verify({ ...plan, family, expectation: "present" }, new AbortController().signal)).toMatchObject({ kind: "verified" })
    for (const call of store.records.values())
      expect(evaluateVerification({ ...plan, family }, call).kind).toBe("pending")
  })

  it("requires all pages to prove absence and preserves a target found on the first page", async () => {
    const absent = setup(request => ({ status: 200, body: { items: [], page: request.arguments.page, totalPages: 3 } }))
    expect(await absent.runner.verify(plan, new AbortController().signal)).toMatchObject({ kind: "verified" })
    expect(absent.client.calls.map(call => call.arguments.page)).toEqual([1, 2, 3])
    const present = setup(request => ({ status: 200, body: {
      items: request.arguments.page === 1 ? [{ id: "target" }] : [], page: request.arguments.page, totalPages: 2,
    } }))
    expect(await present.runner.verify(plan, new AbortController().signal)).toMatchObject({ kind: "pending" })
  })

  it("combines project order evidence across pages", async () => {
    const { runner } = setup(request => ({ status: 200, body: {
      items: [{ id: `project_${String(request.arguments.page)}`, dashboardOrder: request.arguments.page }],
      page: request.arguments.page, totalPages: 2,
    } }))
    expect(await runner.verify({ ...plan, family: "project_order", expectation: "present", expected: ["project_1", "project_2"] }, new AbortController().signal))
      .toMatchObject({ kind: "verified" })
  })

  it("fails if a later page cannot be read", async () => {
    const { runner } = setup(request => request.arguments.page === 1
      ? { status: 200, body: { items: [], page: 1, totalPages: 2 } }
      : { status: 503, body: { code: "service.unavailable" } })
    expect(await runner.verify(plan, new AbortController().signal)).toMatchObject({ kind: "failed", errorCode: "service.unavailable" })
  })

  it("stops pagination when canceled", async () => {
    const controller = new AbortController()
    const { runner, client } = setup(() => {
      controller.abort(new Error("ai.run_canceled"))
      return { status: 200, body: { items: [], page: 1, totalPages: 2 } }
    })
    await expect(runner.verify(plan, controller.signal)).rejects.toThrow("ai.run_canceled")
    expect(client.calls).toHaveLength(1)
  })

  it("rejects a server that repeats the first page", async () => {
    const { runner, client } = setup(() => ({ status: 200, body: { items: [], page: 1, totalPages: 2 } }))
    expect(await runner.verify(plan, new AbortController().signal)).toMatchObject({ kind: "failed", errorCode: "ai.verification_inconclusive" })
    expect(client.calls).toHaveLength(2)
  })
})
