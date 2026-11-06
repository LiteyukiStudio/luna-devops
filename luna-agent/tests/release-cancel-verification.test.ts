import { describe, expect, it } from "vitest"
import type { RunToolVerification } from "../src/domain.js"
import { evaluateVerification } from "../src/executor/verification.js"
import type { ToolCallRecord } from "../src/tools/orchestrator.js"

const plan: RunToolVerification = {
  runId: "airun_1",
  sourceToolCallId: "aitool_cancel",
  sourceOperationId: "cancelRelease",
  operationId: "getRelease",
  family: "release",
  expectation: "canceled",
  arguments: { projectId: "prj_1", releaseId: "rel_1" },
  externalOperationId: "rel_1",
  attempts: 0,
}

function readback(status: string): ToolCallRecord {
  return {
    id: "aitool_read",
    idempotencyKey: "aitool_read",
    runId: "airun_1",
    operationId: "getRelease",
    status: "succeeded",
    arguments: plan.arguments,
    argumentsHash: "sha256:args",
    attempt: 1,
    executionCount: 1,
    rowVersion: 2,
    result: { id: "rel_1", status },
  }
}

describe("release cancel verification", () => {
  it("waits while the release is still active and verifies the canceled state", () => {
    expect(evaluateVerification(plan, readback("running"))).toMatchObject({ kind: "pending" })
    expect(evaluateVerification(plan, readback("canceled"))).toMatchObject({ kind: "verified" })
  })

  it("fails when the release finished instead of being canceled", () => {
    expect(evaluateVerification(plan, readback("succeeded")))
      .toMatchObject({ kind: "failed", errorCode: "ai.tool_execution_failed" })
  })
})
