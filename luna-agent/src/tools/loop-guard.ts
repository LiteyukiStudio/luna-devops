export const DEFAULT_RUN_MAX_TOOL_CALLS = 256
export const MIN_RUN_MAX_TOOL_CALLS = 32
export const MAX_RUN_MAX_TOOL_CALLS = 2048
export const DEFAULT_SAME_CALL_LIMIT = 3

export type ToolLoopCall = {
  runId: string
  operationId: string
  argumentsHash: string
}

export type ToolLoopSnapshot = {
  proposed: number
  executed: number
  maxToolCalls: number
}

export interface LoopGuard {
  setMaxToolCalls(limit: number): void
  setRunMaxToolCalls(runId: string, limit: number): void
  beforePropose(call: ToolLoopCall): void
  beforeExecute(call: ToolLoopCall): void
  blockNonRetryable(call: ToolLoopCall): void
  snapshot(runId: string): ToolLoopSnapshot
  clearRun(runId: string): void
}

export class ToolLoopStoppedError extends Error {
  readonly retryable = false as const

  constructor(
    readonly code: "ai.run_tool_call_budget_exceeded" | "ai.tool_repeated_in_run",
    readonly operationId: string,
  ) {
    super(code)
    this.name = "ToolLoopStoppedError"
  }

  toJSON() {
    return { code: this.code, retryable: this.retryable, operationId: this.operationId }
  }
}

type RunLoopState = {
  proposed: number
  executed: number
  maxToolCalls: number
  occurrences: Map<string, number>
  nonRetryableCalls: Set<string>
}

/**
 * Run 内循环保护依据 operationId 与规范化参数计数，并只信任平台显式返回的
 * retryable=false 熔断下一次原样调用；不从错误文本推断，避免误伤合法轮询。
 */
export class InMemoryLoopGuard implements LoopGuard {
  private maxToolCalls: number
  private readonly sameCallLimit: number
  private readonly runs = new Map<string, RunLoopState>()

  constructor(options: { maxToolCalls?: number, sameCallLimit?: number } = {}) {
    this.maxToolCalls = validateMaxToolCalls(options.maxToolCalls ?? DEFAULT_RUN_MAX_TOOL_CALLS)
    this.sameCallLimit = Math.max(2, Math.floor(options.sameCallLimit ?? DEFAULT_SAME_CALL_LIMIT))
  }

  setMaxToolCalls(limit: number): void {
    this.maxToolCalls = validateMaxToolCalls(limit)
  }

  setRunMaxToolCalls(runId: string, limit: number): void {
    const state = this.state(runId)
    state.maxToolCalls = validateMaxToolCalls(limit)
  }

  beforePropose(call: ToolLoopCall): void {
    const state = this.state(call.runId)
    state.proposed += 1
    if (state.proposed > state.maxToolCalls) throw stop("ai.run_tool_call_budget_exceeded", call)

    const key = callKey(call)
    if (state.nonRetryableCalls.has(key)) throw stop("ai.tool_repeated_in_run", call)
    const occurrences = (state.occurrences.get(key) ?? 0) + 1
    state.occurrences.set(key, occurrences)
    if (occurrences > this.sameCallLimit) throw stop("ai.tool_repeated_in_run", call)
  }

  beforeExecute(call: ToolLoopCall): void {
    const state = this.state(call.runId)
    state.executed += 1
    if (state.executed > state.maxToolCalls) throw stop("ai.run_tool_call_budget_exceeded", call)
  }

  blockNonRetryable(call: ToolLoopCall): void {
    this.state(call.runId).nonRetryableCalls.add(callKey(call))
  }

  snapshot(runId: string): ToolLoopSnapshot {
    const state = this.runs.get(runId)
    return {
      proposed: state?.proposed ?? 0,
      executed: state?.executed ?? 0,
      maxToolCalls: state?.maxToolCalls ?? this.maxToolCalls,
    }
  }

  clearRun(runId: string): void {
    this.runs.delete(runId)
  }

  private state(runId: string): RunLoopState {
    let state = this.runs.get(runId)
    if (!state) {
      state = { proposed: 0, executed: 0, maxToolCalls: this.maxToolCalls, occurrences: new Map(), nonRetryableCalls: new Set() }
      this.runs.set(runId, state)
    }
    return state
  }
}

export function validateMaxToolCalls(limit: number): number {
  if (!Number.isInteger(limit) || limit < MIN_RUN_MAX_TOOL_CALLS || limit > MAX_RUN_MAX_TOOL_CALLS)
    throw new RangeError("ai.run_max_tool_calls_invalid")
  return limit
}

function callKey(call: ToolLoopCall): string {
  return `${call.operationId}\u0000${call.argumentsHash}`
}

function stop(code: ToolLoopStoppedError["code"], call: ToolLoopCall): ToolLoopStoppedError {
  return new ToolLoopStoppedError(code, call.operationId)
}
