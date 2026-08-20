import { describe, expect, it } from "vitest"
import { InMemoryLoopGuard, ToolLoopStoppedError } from "../src/tools/loop-guard.js"

const call = { runId: "airun_guard", operationId: "getBuildRun", argumentsHash: "sha256:args" }

describe("run tool loop guard", () => {
  it("enforces separate proposed and executed budgets", () => {
    const proposed = new InMemoryLoopGuard({ maxToolCalls: 32 })
    for (let index = 0; index < 32; index += 1) proposed.beforePropose({ ...call, argumentsHash: `sha256:${index}` })
    expect(() => proposed.beforePropose({ ...call, argumentsHash: "sha256:overflow" })).toThrowError(expect.objectContaining({
      code: "ai.run_tool_call_budget_exceeded",
      retryable: false,
    }))
    expect(proposed.snapshot(call.runId)).toEqual({ proposed: 33, executed: 0, maxToolCalls: 32 })

    const executed = new InMemoryLoopGuard({ maxToolCalls: 32 })
    for (let index = 0; index < 32; index += 1) executed.beforeExecute({ ...call, argumentsHash: `sha256:${index}` })
    expect(() => executed.beforeExecute({ ...call, argumentsHash: "sha256:overflow" })).toThrowError(ToolLoopStoppedError)
    expect(executed.snapshot(call.runId)).toEqual({ proposed: 0, executed: 33, maxToolCalls: 32 })
  })

  it("rejects invalid runtime limits and accepts the configured range", () => {
    const guard = new InMemoryLoopGuard()
    expect(guard.snapshot(call.runId).maxToolCalls).toBe(256)
    expect(() => guard.setMaxToolCalls(31)).toThrow("ai.run_max_tool_calls_invalid")
    expect(() => guard.setMaxToolCalls(2049)).toThrow("ai.run_max_tool_calls_invalid")
    expect(() => guard.setMaxToolCalls(32.5)).toThrow("ai.run_max_tool_calls_invalid")
    guard.setMaxToolCalls(2048)
    expect(guard.snapshot(call.runId).maxToolCalls).toBe(2048)
  })

  it("snapshots the limit on a run's first call while updates affect later runs", () => {
    const guard = new InMemoryLoopGuard({ maxToolCalls: 32 })
    guard.beforePropose(call)
    guard.setMaxToolCalls(64)
    expect(guard.snapshot(call.runId).maxToolCalls).toBe(32)

    const laterRun = { ...call, runId: "airun_later" }
    guard.beforePropose(laterRun)
    expect(guard.snapshot(laterRun.runId).maxToolCalls).toBe(64)
  })

  it("blocks an unchanged deterministic failure but allows corrected arguments", () => {
    const guard = new InMemoryLoopGuard()
    guard.beforePropose(call)
    guard.recordFailure({ ...call, errorCode: "cluster.resource_category_invalid", stableResultHash: "sha256:error", deterministic: true })
    expect(captureLoopStop(() => guard.beforePropose(call))).toMatchObject({
      code: "ai.tool_deterministic_failure_repeated",
      fingerprint: { stableErrorCode: "cluster.resource_category_invalid" },
    })
    expect(() => guard.beforePropose({ ...call, argumentsHash: "sha256:corrected" })).not.toThrow()
  })

  it("allows transient failures to retry", () => {
    const guard = new InMemoryLoopGuard()
    guard.recordFailure({ ...call, errorCode: "provider_unavailable", deterministic: false })
    expect(() => guard.beforePropose(call)).not.toThrow()
  })

  it("allows one authoritative re-read and stops after two identical results", () => {
    const guard = new InMemoryLoopGuard()
    guard.recordResult({ ...call, stableResultHash: "sha256:same" })
    expect(() => guard.beforePropose(call)).not.toThrow()
    guard.recordResult({ ...call, stableResultHash: "sha256:same" })
    expect(captureLoopStop(() => guard.beforePropose(call))).toMatchObject({
      code: "ai.tool_no_new_information",
      fingerprint: { stableResultHash: "sha256:same" },
    })
  })

  it("keeps polling when the authoritative result changes", () => {
    const guard = new InMemoryLoopGuard()
    guard.recordResult({ ...call, stableResultHash: "sha256:first" })
    guard.beforePropose(call)
    guard.recordResult({ ...call, stableResultHash: "sha256:second" })

    expect(() => guard.beforePropose(call)).not.toThrow()
  })

  it("does not apply no-new-information blocking to legal async readback polling", () => {
    const guard = new InMemoryLoopGuard({ isAsyncReadbackOperation: operationId => operationId === "getRelease" })
    const poll = { ...call, operationId: "getRelease" }
    for (let index = 0; index < 20; index += 1) {
      guard.beforePropose(poll)
      guard.recordResult({ ...poll, stableResultHash: "sha256:pending" })
    }
    expect(guard.snapshot(call.runId)).toMatchObject({ proposed: 20 })
  })

  it("blocks a stable result inherited from the immediately preceding conversation turn", () => {
    const guard = new InMemoryLoopGuard()
    guard.seedResult({ ...call, stableResultHash: "sha256:empty" })

    expect(captureLoopStop(() => guard.beforePropose(call))).toMatchObject({
      code: "ai.tool_no_new_information",
      fingerprint: { stableResultHash: "sha256:empty" },
    })
  })

  it("releases per-run fingerprints and counters", () => {
    const guard = new InMemoryLoopGuard()
    guard.beforePropose(call)
    guard.recordFailure({ ...call, errorCode: "invalid", deterministic: true })
    guard.clearRun(call.runId)
    expect(guard.snapshot(call.runId)).toEqual({ proposed: 0, executed: 0, maxToolCalls: 256 })
    expect(() => guard.beforePropose(call)).not.toThrow()
  })
})

function captureLoopStop(callback: () => void): ToolLoopStoppedError {
  try {
    callback()
  }
  catch (error) {
    if (error instanceof ToolLoopStoppedError) return error
    throw error
  }
  throw new Error("expected loop guard to stop the call")
}
