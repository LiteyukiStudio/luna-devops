import { describe, expect, it } from "vitest"
import { InMemoryLoopGuard, ToolLoopStoppedError, type LoopGuardState } from "../src/tools/loop-guard.js"

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
  })

  it("stops only repeated operationId plus normalized arguments in the same Run", () => {
    const guard = new InMemoryLoopGuard({ sameCallLimit: 3 })
    for (let index = 0; index < 3; index += 1) guard.beforePropose(call)
    expect(() => guard.beforePropose(call)).toThrowError(expect.objectContaining({ code: "ai.tool_repeated_in_run" }))
    expect(() => guard.beforePropose({ ...call, argumentsHash: "sha256:different" })).not.toThrow()
    expect(() => guard.beforePropose({ ...call, runId: "airun_other" })).not.toThrow()
  })

  it("stops the next identical proposal after an explicitly non-retryable result", () => {
    const guard = new InMemoryLoopGuard({ sameCallLimit: 3 })
    guard.beforePropose(call)
    guard.blockNonRetryable(call)
    expect(() => guard.beforePropose(call)).toThrowError(expect.objectContaining({
      code: "ai.tool_repeated_in_run",
      retryable: false,
    }))
    expect(() => guard.beforePropose({ ...call, argumentsHash: "sha256:corrected" })).not.toThrow()
  })

  it("snapshots limits per Run and clears all same-Run state", () => {
    const guard = new InMemoryLoopGuard({ maxToolCalls: 32 })
    guard.beforePropose(call)
    guard.setMaxToolCalls(64)
    expect(guard.snapshot(call.runId).maxToolCalls).toBe(32)
    guard.clearRun(call.runId)
    expect(guard.snapshot(call.runId)).toEqual({ proposed: 0, executed: 0, maxToolCalls: 64 })
  })

  it("round-trips run state through a JSON-safe export for checkpoint replay", () => {
    const guard = new InMemoryLoopGuard({ sameCallLimit: 3 })
    guard.beforePropose(call)
    guard.beforeExecute(call)
    guard.beforePropose(call)
    guard.blockNonRetryable(call)
    const exported = guard.exportRun(call.runId)
    expect(exported).toEqual({
      proposed: 2,
      occurrences: [{ operationId: call.operationId, argumentsHash: call.argumentsHash, count: 2 }],
      nonRetryableCalls: [{ operationId: call.operationId, argumentsHash: call.argumentsHash }],
    })

    // checkpoint 走 jsonb，导出结果必须能原样序列化并恢复同一份调用保护。
    const restored = new InMemoryLoopGuard({ sameCallLimit: 3 })
    restored.restoreRun(call.runId, JSON.parse(JSON.stringify(exported)) as LoopGuardState, 1)
    expect(restored.snapshot(call.runId)).toMatchObject({ proposed: 2, executed: 1 })
    expect(() => restored.beforePropose(call)).toThrowError(expect.objectContaining({ code: "ai.tool_repeated_in_run" }))
    expect(restored.exportRun("airun_unknown")).toBeUndefined()
  })

  it("keeps the higher counter when restoring on top of live state", () => {
    const guard = new InMemoryLoopGuard({ sameCallLimit: 3 })
    guard.beforePropose(call)
    guard.beforeExecute(call)
    guard.restoreRun(call.runId, { proposed: 0, occurrences: [], nonRetryableCalls: [] }, 0)
    expect(guard.snapshot(call.runId)).toMatchObject({ proposed: 1, executed: 1 })
  })

  it("raises run counters from settled facts idempotently", () => {
    const guard = new InMemoryLoopGuard({ sameCallLimit: 3 })
    const settled = {
      proposed: 3,
      executed: 2,
      occurrences: [
        { operationId: "getProject", argumentsHash: "sha256:a" },
        { operationId: "getProject", argumentsHash: "sha256:a" },
        { operationId: "getBuild", argumentsHash: "sha256:b" },
      ],
      nonRetryableCalls: [{ operationId: "getBuild", argumentsHash: "sha256:b" }],
    }
    guard.restoreRun(call.runId, { proposed: 1, occurrences: [{ operationId: "getProject", argumentsHash: "sha256:a", count: 1 }], nonRetryableCalls: [] }, 1)
    guard.raiseFromSettledCalls(call.runId, settled)
    guard.raiseFromSettledCalls(call.runId, settled)

    expect(guard.snapshot(call.runId)).toMatchObject({ proposed: 3, executed: 2 })
    expect(guard.exportRun(call.runId)).toEqual({
      proposed: 3,
      occurrences: [
        { operationId: "getProject", argumentsHash: "sha256:a", count: 2 },
        { operationId: "getBuild", argumentsHash: "sha256:b", count: 1 },
      ],
      nonRetryableCalls: [{ operationId: "getBuild", argumentsHash: "sha256:b" }],
    })
    expect(() => guard.beforePropose({ runId: call.runId, operationId: "getBuild", argumentsHash: "sha256:b" }))
      .toThrowError(expect.objectContaining({ code: "ai.tool_repeated_in_run" }))
  })

  it("rejects invalid runtime limits", () => {
    const guard = new InMemoryLoopGuard()
    expect(() => guard.setMaxToolCalls(31)).toThrow("ai.run_max_tool_calls_invalid")
    expect(() => guard.setMaxToolCalls(2049)).toThrow("ai.run_max_tool_calls_invalid")
    expect(() => guard.setMaxToolCalls(32.5)).toThrow("ai.run_max_tool_calls_invalid")
  })
})
