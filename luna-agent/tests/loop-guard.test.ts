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
  })

  it("stops only repeated operationId plus normalized arguments in the same Run", () => {
    const guard = new InMemoryLoopGuard({ sameCallLimit: 3 })
    for (let index = 0; index < 3; index += 1) guard.beforePropose(call)
    expect(() => guard.beforePropose(call)).toThrowError(expect.objectContaining({ code: "ai.tool_repeated_in_run" }))
    expect(() => guard.beforePropose({ ...call, argumentsHash: "sha256:different" })).not.toThrow()
    expect(() => guard.beforePropose({ ...call, runId: "airun_other" })).not.toThrow()
  })

  it("snapshots limits per Run and clears all same-Run state", () => {
    const guard = new InMemoryLoopGuard({ maxToolCalls: 32 })
    guard.beforePropose(call)
    guard.setMaxToolCalls(64)
    expect(guard.snapshot(call.runId).maxToolCalls).toBe(32)
    guard.clearRun(call.runId)
    expect(guard.snapshot(call.runId)).toEqual({ proposed: 0, executed: 0, maxToolCalls: 64 })
  })

  it("rejects invalid runtime limits", () => {
    const guard = new InMemoryLoopGuard()
    expect(() => guard.setMaxToolCalls(31)).toThrow("ai.run_max_tool_calls_invalid")
    expect(() => guard.setMaxToolCalls(2049)).toThrow("ai.run_max_tool_calls_invalid")
    expect(() => guard.setMaxToolCalls(32.5)).toThrow("ai.run_max_tool_calls_invalid")
  })
})
