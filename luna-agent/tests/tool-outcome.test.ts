import { describe, expect, it } from "vitest"
import { evaluateVerification } from "../src/executor/verification.js"
import { resumedToolMessages } from "../src/executor/resume.js"
import type { RunToolVerification } from "../src/domain.js"
import type { ToolCallRecord } from "../src/tools/orchestrator.js"
import { toolEffect, toolOutcome, verificationForCall } from "../src/tools/outcome.js"
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
        arguments: { projectId: "prj_1", runId: "bldr_1" },
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
    expect(evaluateVerification(plan, call({
      operationId: "getThing",
      result: { id: "thing_1", status: "provisioning" },
    }))).toMatchObject({ kind: "pending" })
    expect(evaluateVerification(plan, call({
      operationId: "getThing",
      result: { id: "thing_1", status: "ready" },
    }))).toMatchObject({ kind: "verified" })
  })

  it("verifies collection deletion by target ID absence", () => {
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

    expect(evaluateVerification(plan, call({
      operationId: "listThings",
      result: { items: [{ id: "thing_1" }] },
    }))).toMatchObject({ kind: "pending" })
    expect(evaluateVerification(plan, call({
      operationId: "listThings",
      result: { items: [] },
    }))).toMatchObject({ kind: "verified" })
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

    expect(plan).toMatchObject({ arguments: { page: 1, pageSize: 100 } })
  })

  it("reads root-array collections and honors delete and cancel terminal states", () => {
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

    expect(evaluateVerification(deletePlan, call({
      operationId: "listThings",
      result: [{ id: "thing_1", status: "delete_failed" }],
    }))).toMatchObject({ kind: "failed", errorCode: "ai.tool_execution_failed" })
    expect(evaluateVerification(deletePlan, call({
      operationId: "listThings",
      result: [],
    }))).toMatchObject({ kind: "verified" })

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

    expect(evaluateVerification(cancelPlan, call({
      operationId: "getThing",
      result: { id: "thing_1", status: "running" },
    }))).toMatchObject({ kind: "pending" })
    expect(evaluateVerification(cancelPlan, call({
      operationId: "getThing",
      result: { id: "thing_1", status: "cancelled" },
    }))).toMatchObject({ kind: "verified" })
  })

  it("verifies bulk project order and billing rule updates against authoritative lists", () => {
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

    expect(projectPlan).toMatchObject({ arguments: { page: 1, pageSize: 100 }, expected: ["prj_1", "prj_2"] })
    expect(evaluateVerification(projectPlan, call({
      operationId: "listProjects",
      result: { items: [{ id: "prj_1", dashboardOrder: 1 }, { id: "prj_2", dashboardOrder: 2 }] },
    }))).toMatchObject({ kind: "verified" })
    expect(evaluateVerification(projectPlan, call({
      operationId: "listProjects",
      result: { items: [{ id: "prj_1", dashboardOrder: 2 }, { id: "prj_2", dashboardOrder: 1 }] },
    }))).toMatchObject({ kind: "failed", errorCode: "ai.verification_inconclusive" })

    const billingPlan = verificationForCall(testToolOperation("updateBillingRateRules", {
      method: "PUT",
      effect: "write",
      verification: [{ operationId: "listBillingRateRules", family: "billing_rate_rules" }],
    }), call({
      operationId: "updateBillingRateRules",
      arguments: { body: { rules: [{ meter: "cpu", creditsPerUnit: "2.50", enabled: true }] } },
      result: [],
    }), operationId => operationId === "listBillingRateRules"
      ? testToolOperation("listBillingRateRules")
      : undefined)
    if (!billingPlan) throw new Error("missing billing verification plan")

    expect(evaluateVerification(billingPlan, call({
      operationId: "listBillingRateRules",
      result: [{ id: "brr_cpu", meter: "cpu", creditsPerUnit: "2.5", enabled: true }],
    }))).toMatchObject({ kind: "verified" })
    expect(evaluateVerification(billingPlan, call({
      operationId: "listBillingRateRules",
      result: [{ id: "brr_cpu", meter: "cpu", creditsPerUnit: "3", enabled: true }],
    }))).toMatchObject({ kind: "failed", errorCode: "ai.verification_inconclusive" })
  })

  it("verifies project runtime config secrets without exposing plaintext", () => {
    const read = testToolOperation("getProjectRuntimeConfigSetRuntimeSecretsSummary", {
      inputSchema: {
        type: "object",
        properties: { projectId: { type: "string" }, setId: { type: "string" } },
        required: ["projectId", "setId"],
        additionalProperties: false,
      },
      verificationOnly: true,
    })
    const plan = verificationForCall(testToolOperation("updateProjectRuntimeConfigSetRuntimeSecrets", {
      method: "PUT",
      effect: "write",
      verification: [{
        operationId: "getProjectRuntimeConfigSetRuntimeSecretsSummary",
        family: "runtime_config_secrets",
      }],
    }), call({
      operationId: "updateProjectRuntimeConfigSetRuntimeSecrets",
      arguments: {
        projectId: "prj_1",
        setId: "prcs_1",
        body: { items: [{ key: "TOKEN", valueMode: "secret", operation: "set", value: "secret-value" }] },
      },
      result: { environmentVariables: [] },
    }), operationId => operationId === "getProjectRuntimeConfigSetRuntimeSecretsSummary" ? read : undefined)
    if (!plan) throw new Error("missing project runtime secret verification plan")

    expect(plan).toMatchObject({
      family: "runtime_config_secrets",
      arguments: { projectId: "prj_1", setId: "prcs_1" },
      requiredKeys: ["TOKEN"],
    })
    expect(JSON.stringify(plan)).not.toContain("secret-value")
    expect(evaluateVerification(plan, call({
      operationId: "getProjectRuntimeConfigSetRuntimeSecretsSummary",
      result: { environmentVariables: [{ key: "TOKEN", valueMode: "secret", configured: true }] },
    }))).toMatchObject({ kind: "verified" })
  })
})

describe("authoritative verification outcomes", () => {
  const plan: RunToolVerification = {
    runId: "airun_1",
    sourceToolCallId: "aitool_source",
    sourceOperationId: "createGatewayRoute",
    operationId: "getGatewayRoute",
    family: "gateway_route",
    expectation: "present",
    arguments: { projectId: "prj_1", routeId: "route_1" },
    externalOperationId: "route_1",
    attempts: 0,
  }

  it("waits for both the route and its TLS certificate to become ready", () => {
    expect(evaluateVerification(plan, call({
      operationId: "getGatewayRoute",
      result: { id: "route_1", status: "ready", tlsMode: "http-challenge", certificateStatus: "pending" },
    }))).toMatchObject({ kind: "pending" })
    expect(evaluateVerification(plan, call({
      operationId: "getGatewayRoute",
      result: { id: "route_1", status: "ready", tlsMode: "http-challenge", certificateStatus: "issued" },
    }))).toMatchObject({ kind: "verified" })
  })

  it("accepts manual and disabled certificates but fails degraded routes", () => {
    expect(evaluateVerification(plan, call({
      operationId: "getGatewayRoute",
      result: { id: "route_1", status: "ready", tlsMode: "manual-cert", certificateStatus: "manual" },
    }))).toMatchObject({ kind: "verified" })
    expect(evaluateVerification(plan, call({
      operationId: "getGatewayRoute",
      result: { id: "route_1", status: "ready", tlsMode: "http-only", certificateStatus: "disabled" },
    }))).toMatchObject({ kind: "verified" })
    expect(evaluateVerification(plan, call({
      operationId: "getGatewayRoute",
      result: { id: "route_1", status: "degraded", tlsMode: "manual-cert", certificateStatus: "manual" },
    }))).toMatchObject({ kind: "failed", errorCode: "ai.tool_execution_failed" })
  })

  it("does not turn a failed external operation into business completion", () => {
    expect(evaluateVerification({
      ...plan,
      operationId: "getBuildRun",
      family: "build_run",
      externalOperationId: "bldr_1",
    }, call({
      operationId: "getBuildRun",
      result: { id: "bldr_1", status: "timeout" },
    }))).toMatchObject({
      kind: "failed",
      errorCode: "ai.tool_execution_failed",
    })
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
})
