import { createServer } from "node:http"
import { describe, expect, it } from "vitest"
import { resumedToolMessages } from "../src/executor/resume.js"
import type { ToolCallRecord } from "../src/tools/orchestrator.js"
import { toolEffect, toolOutcome, verificationForCall } from "../src/tools/outcome.js"
import { HttpLunaApiTaskStatusClient } from "../src/tools/wait-for-task.js"
import { testToolOperation } from "./support/tool-catalog.js"

function call(overrides: Partial<ToolCallRecord> = {}): ToolCallRecord {
  const id = overrides.id ?? "aitool_1"
  return {
    id,
    idempotencyKey: id,
    runId: "airun_1",
    operationId: "getBuildRun",
    status: "succeeded",
    arguments: {},
    argumentsHash: "sha256:args",
    attempt: 1,
    executionCount: 1,
    rowVersion: 2,
    ...overrides,
  }
}

describe("tool outcomes", () => {
  it.each([
    ["discovery", false],
    ["read", false],
    ["execute", false],
    ["interactive", false],
    ["presentation", false],
    ["write", true],
  ] as const)("classifies %s tools without treating presentation as completion evidence", (effect, verificationRequired) => {
    const outcome = toolOutcome(testToolOperation("testOperation", {
      effect,
      method: effect === "read" ? "GET" : "POST",
    }), call())

    expect(outcome).toMatchObject({ kind: "succeeded", effect, verificationRequired })
  })

  it("maps an accepted asynchronous operation to running with an authoritative readback plan", () => {
    const catalogOperation = testToolOperation("triggerBuildRun", {
      method: "POST",
      effect: "async",
      verification: [{ operationId: "getBuildRun", family: "build_run" }],
      inputSchema: {
        type: "object",
        properties: { projectId: { type: "string" }, body: { type: "object" } },
        required: ["projectId", "body"],
        additionalProperties: false,
      },
    })
    const outcome = toolOutcome(catalogOperation, call({
      operationId: "triggerBuildRun",
      arguments: { projectId: "prj_1", body: { applicationId: "app_1" } },
      result: { id: "bldr_1", status: "queued" },
    }))

    expect(outcome).toEqual({
      kind: "running",
      effect: "async",
      result: { id: "bldr_1", status: "queued" },
      externalOperationId: "bldr_1",
      verification: {
        runId: "airun_1",
        sourceToolCallId: "aitool_1",
        sourceOperationId: "triggerBuildRun",
        operationId: "getBuildRun",
        family: "build_run",
        expectation: "present",
        arguments: { project: "prj_1", buildRunId: "bldr_1" },
        externalOperationId: "bldr_1",
        attempts: 0,
      },
    })
  })

  it("selects the template verification path that matches the write response", () => {
    const catalogOperation = testToolOperation("installAppTemplate", {
      method: "POST",
      effect: "write",
      verification: [
        { operationId: "getRelease", family: "release", whenResultHas: "release" },
        { operationId: "getApplication", family: "application" },
      ],
      inputSchema: {
        type: "object",
        properties: { projectId: { type: "string" }, body: { type: "object" } },
        required: ["projectId", "body"],
        additionalProperties: false,
      },
    })

    const withoutRelease = toolOutcome(catalogOperation, call({
      operationId: "installAppTemplate",
      arguments: { projectId: "prj_1", body: { installNow: false } },
      result: { application: { id: "app_1" }, deploymentTarget: { id: "dplt_1" } },
    }))
    const withRelease = toolOutcome(catalogOperation, call({
      operationId: "installAppTemplate",
      arguments: { projectId: "prj_1", body: { installNow: true } },
      result: { application: { id: "app_1" }, release: { id: "rel_1" } },
    }))

    expect(withoutRelease).toMatchObject({
      kind: "succeeded",
      verification: { operationId: "getApplication", family: "application", externalOperationId: "app_1" },
    })
    expect(withRelease).toMatchObject({
      kind: "succeeded",
      verification: { operationId: "getRelease", family: "release", externalOperationId: "rel_1" },
    })
  })

  it("defaults GET operations to read and mutations to write", () => {
    expect(toolEffect(testToolOperation("readThing", { method: "GET" }))).toBe("read")
    expect(toolEffect(testToolOperation("writeThing", { method: "PATCH" }))).toBe("write")
  })

  it("builds a generic resource readback from the write result and configured read schema", () => {
    const read = testToolOperation("getThing", {
      inputSchema: {
        type: "object",
        properties: { thingId: { type: "string" } },
        required: ["thingId"],
        additionalProperties: false,
      },
    })
    const write = testToolOperation("createThing", {
      method: "POST",
      effect: "write",
      verification: [{
        operationId: "getThing",
        family: "resource",
        idFrom: "result.id",
        readIdArgument: "thingId",
        statusField: "status",
        successStatuses: ["ready"],
        pendingStatuses: ["provisioning"],
        failureStatuses: ["error"],
      }],
    })
    const source = call({
      operationId: "createThing",
      arguments: { projectId: "prj_1" },
      result: { id: "thing_1", projectId: "prj_1" },
    })
    const plan = verificationForCall(write, source, operationId => operationId === "getThing" ? read : undefined)
    if (!plan) throw new Error("missing generic verification plan")

    expect(plan).toMatchObject({
      family: "resource",
      externalOperationId: "thing_1",
      arguments: { thingId: "thing_1" },
      resource: { singleton: false, collection: false, targetField: "id" },
    })
  })

  it("builds a collection deletion verification plan targeting absence", () => {
    const read = testToolOperation("listThings")
    const plan = verificationForCall(testToolOperation("deleteThing", {
      method: "DELETE",
      effect: "write",
      verification: [{
        operationId: "listThings",
        family: "resource",
        idFrom: "argument.thingId",
        collection: true,
        expect: "absent",
      }],
    }), call({
      operationId: "deleteThing",
      status: "succeeded",
      arguments: { thingId: "thing_1" },
      result: undefined,
    }), operationId => operationId === "listThings" ? read : undefined)
    if (!plan) throw new Error("missing deletion verification plan")

    expect(plan).toMatchObject({
      family: "resource",
      expectation: "absent",
      externalOperationId: "thing_1",
      resource: { collection: true, targetField: "id" },
    })
  })

  it("uses the full bounded page when a collection read exposes pagination", () => {
    const read = testToolOperation("listThings", {
      inputSchema: {
        type: "object",
        properties: { page: { type: "integer" }, pageSize: { type: "integer" } },
        required: [],
        additionalProperties: false,
      },
    })
    const plan = verificationForCall(testToolOperation("createThing", {
      method: "POST",
      effect: "write",
      verification: [{
        operationId: "listThings",
        family: "resource",
        idFrom: "result.id",
        collection: true,
      }],
    }), call({
      operationId: "createThing",
      result: { id: "thing_1" },
    }), operationId => operationId === "listThings" ? read : undefined)

    expect(plan).toMatchObject({ arguments: { page: 1, pageSize: 50 } })
  })

  it("preserves delete and cancel status configuration in verification plans", () => {
    const read = testToolOperation("listThings")
    const deletePlan = verificationForCall(testToolOperation("deleteThing", {
      method: "DELETE",
      effect: "write",
      verification: [{
        operationId: "listThings",
        family: "resource",
        idFrom: "argument.thingId",
        collection: true,
        expect: "absent",
        statusField: "status",
        pendingStatuses: ["deleting"],
        successStatuses: ["deleted"],
        failureStatuses: ["delete_failed"],
      }],
    }), call({ operationId: "deleteThing", arguments: { thingId: "thing_1" }, result: undefined }),
    operationId => operationId === "listThings" ? read : undefined)
    if (!deletePlan) throw new Error("missing delete verification plan")

    expect(deletePlan).toMatchObject({
      expectation: "absent",
      resource: {
        statusField: "status",
        pendingStatuses: ["deleting"],
        successStatuses: ["deleted"],
        failureStatuses: ["delete_failed"],
      },
    })

    const cancelPlan = verificationForCall(testToolOperation("cancelThing", {
      method: "POST",
      effect: "write",
      verification: [{
        operationId: "getThing",
        family: "resource",
        idFrom: "argument.thingId",
        expect: "canceled",
        statusField: "status",
        pendingStatuses: ["running"],
        successStatuses: ["cancelled"],
        failureStatuses: ["failed"],
      }],
    }), call({ operationId: "cancelThing", arguments: { thingId: "thing_1" }, result: undefined }),
    operationId => operationId === "getThing"
      ? testToolOperation("getThing", {
          inputSchema: {
            type: "object",
            properties: { thingId: { type: "string" } },
            required: ["thingId"],
            additionalProperties: false,
          },
        })
      : undefined)
    if (!cancelPlan) throw new Error("missing cancel verification plan")

    expect(cancelPlan).toMatchObject({
      expectation: "canceled",
      arguments: { thingId: "thing_1" },
    })
  })

  it("builds bulk project order and billing rule verification plans", () => {
    const projectRead = testToolOperation("listProjects", {
      inputSchema: {
        type: "object",
        properties: { page: { type: "integer" }, pageSize: { type: "integer" } },
        required: [],
        additionalProperties: false,
      },
    })
    const projectPlan = verificationForCall(testToolOperation("updateProjectOrder", {
      method: "PUT",
      effect: "write",
      verification: [{ operationId: "listProjects", family: "project_order" }],
    }), call({
      operationId: "updateProjectOrder",
      arguments: { body: { projectIds: ["prj_1", "prj_2"] } },
      result: { projectIds: ["prj_1", "prj_2"] },
    }), operationId => operationId === "listProjects" ? projectRead : undefined)
    if (!projectPlan) throw new Error("missing project order verification plan")

    expect(projectPlan).toMatchObject({ arguments: { sortBy: "dashboardOrder", sortOrder: "asc", page: 1, pageSize: 50 }, expected: ["prj_1", "prj_2"] })

    const billingPlan = verificationForCall(testToolOperation("admin_update_billing_rate_rules", {
      effect: "write",
      verification: [{ operationId: "admin_inspect_billing", family: "billing_rate_rules", staticArguments: { view: "rate_rules" } }],
    }), call({
      operationId: "admin_update_billing_rate_rules",
      arguments: { rules: [{ meter: "cpu", creditsPerUnit: "2.50", enabled: true }] },
      result: {},
    }), operationId => operationId === "admin_inspect_billing"
      ? testToolOperation("admin_inspect_billing")
      : undefined)
    if (!billingPlan) throw new Error("missing billing verification plan")

    expect(billingPlan).toMatchObject({
      family: "billing_rate_rules",
      arguments: { view: "rate_rules" },
      expected: [{ meter: "cpu", creditsPerUnit: "2.50", enabled: true }],
    })
  })

  it("builds project runtime config secret verification plans without exposing plaintext", () => {
    const read = testToolOperation("delivery_get_config_set", {
      inputSchema: {
        type: "object",
        properties: { project: { type: "string" }, configSet: { type: "string" } },
        required: ["project", "configSet"],
        additionalProperties: false,
      },
    })
    const plan = verificationForCall(testToolOperation("delivery_configure_config_set", {
      effect: "write",
      verification: [{
        operationId: "delivery_get_config_set",
        family: "resource",
        idFrom: "result.id",
        readIdArgument: "configSet",
      }],
    }), call({
      operationId: "delivery_configure_config_set",
      arguments: {
        project: "prj_1",
        configSet: "prcs_1",
        secretActions: [{ key: "TOKEN", operation: "set", value: "secret-value" }],
      },
      result: { id: "prcs_1", projectId: "prj_1", environmentVariables: [] },
    }), operationId => operationId === "delivery_get_config_set" ? read : undefined)
    if (!plan) throw new Error("missing project runtime secret verification plan")

    expect(plan).toMatchObject({
      family: "resource",
      arguments: { project: "prj_1", configSet: "prcs_1" },
      requiredKeys: ["TOKEN"],
    })
    expect(JSON.stringify(plan)).not.toContain("secret-value")
  })
})

describe("resumed tool exchange", () => {
  it("rebuilds the reasoning of the paused model step from the thinking item", () => {
    const messages = resumedToolMessages([
      {
        itemId: "reasoning-item",
        type: "reasoning_summary",
        status: "completed",
        content: { summary: "先确认构建记录。", display: "summary" },
      },
      {
        itemId: "call-item",
        type: "tool_call",
        status: "completed",
        content: {
          toolCallId: "aitool_call",
          operationId: "getBuildRun",
          inputMode: "model",
          status: "succeeded",
          arguments: { buildRunId: "bldr_1" },
          result: { id: "bldr_1", status: "succeeded" },
        },
      },
      {
        itemId: "call-result",
        type: "tool_result",
        status: "completed",
        content: { relatedItemId: "call-item", result: { id: "bldr_1", status: "succeeded" } },
      },
    ])

    expect(messages[0]).toMatchObject({ role: "assistant", reasoning: "先确认构建记录。" })
  })
})

describe("durable verification projection", () => {
  it("restores the write and its authoritative verification as one tool result", () => {
    const messages = resumedToolMessages([
      {
        itemId: "source-item",
        type: "tool_call",
        status: "completed",
        content: {
          toolCallId: "aitool_source",
          operationId: "triggerBuildRun",
          inputMode: "model",
          status: "succeeded",
          arguments: { projectId: "prj_1", body: { applicationId: "app_1" } },
          result: { id: "bldr_1", status: "queued" },
        },
      },
      {
        itemId: "source-result",
        type: "tool_result",
        status: "completed",
        content: { relatedItemId: "source-item", result: { id: "bldr_1", status: "queued" } },
      },
      {
        itemId: "verification-item",
        type: "tool_call",
        status: "completed",
        content: {
          toolCallId: "aitool_verification",
          operationId: "getBuildRun",
          inputMode: "verification",
          verificationForToolCallId: "aitool_source",
          status: "succeeded",
          result: { id: "bldr_1", status: "succeeded" },
        },
      },
      {
        itemId: "verification-result",
        type: "tool_result",
        status: "completed",
        content: { relatedItemId: "verification-item", result: { id: "bldr_1", status: "succeeded" } },
      },
    ])

    expect(messages).toHaveLength(2)
    expect(messages[1]).toMatchObject({
      role: "tool",
      toolCallId: "aitool_source",
    })
    expect(messages[1]?.content).toContain('"verifiedBy"')
    expect(messages[1]?.content).toContain('"status":"succeeded"')
  })

  it("[P2-2] clears pendingContent across tool-less model steps so earlier progress never bleeds into a later tool batch", () => {
    const messages = resumedToolMessages([
      {
        itemId: "step0-progress",
        type: "progress",
        status: "completed",
        content: { parts: [{ type: "text", text: "无工具协议修复过渡文本" }], standalone: true },
      },
      {
        itemId: "step1-reasoning",
        type: "reasoning_summary",
        status: "completed",
        content: { summary: "第二步思考" },
      },
      {
        itemId: "step1-call",
        type: "tool_call",
        status: "completed",
        content: {
          toolCallId: "call_step1",
          modelCallId: "call_step1",
          batchId: "batch_1",
          operationId: "search_tools",
          status: "succeeded",
          arguments: { query: "build" },
          result: { items: [] },
        },
      },
    ])
    expect(messages[0]).toEqual({
      role: "assistant",
      content: "",
      reasoning: "第二步思考",
      toolCalls: [{ id: "call_step1", operationId: "search_tools", arguments: { query: "build" } }],
    })
  })

  it("[P1-4] times out hung task status requests and returns transient_error without hanging forever", async () => {
    const server = createServer((_req, res) => {
      // 模拟半开挂起连接，不结束响应
      setTimeout(() => res.end(), 500).unref()
    })
    await new Promise<void>(resolve => server.listen(0, "127.0.0.1", () => resolve()))
    const address = server.address()
    const port = typeof address === "object" && address ? address.port : 0
    try {
      const client = new HttpLunaApiTaskStatusClient(`http://127.0.0.1:${port}`, "svc_token", 40)
      const result = await client.checkTaskStatus({
        runId: "airun_timeout",
        projectId: "prj_1",
        taskType: "build_run",
        taskId: "bldr_1",
      })
      expect(result).toEqual({ kind: "transient_error", errorCode: "ai.progress.unavailable" })
    }
    finally {
      server.close()
    }
  })
})
