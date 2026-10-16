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

/**
 * Run 内循环保护的可持久化计数。checkpoint 保存该状态，
 * 使重启、审批恢复和后续多副本执行不会重置调用保护。
 * executed 计数由 RunCheckpoint.toolCallCount 承载，不在两处保存。
 */
export type LoopGuardState = {
  proposed: number
  occurrences: Array<{ operationId: string, argumentsHash: string, count: number }>
  nonRetryableCalls: Array<{ operationId: string, argumentsHash: string }>
}

/**
 * 由工具执行事实推导出的已落地调用。崩溃批次已经执行的调用必须计入计数，
 * 否则重启后 Run 的工具预算会被放宽。
 */
export type SettledToolCalls = {
  proposed: number
  executed: number
  occurrences: Array<{ operationId: string, argumentsHash: string }>
  nonRetryableCalls: Array<{ operationId: string, argumentsHash: string }>
}

export interface LoopGuard {
  setMaxToolCalls(limit: number): void
  setRunMaxToolCalls(runId: string, limit: number): void
  beforePropose(call: ToolLoopCall): void
  beforeExecute(call: ToolLoopCall): void
  blockNonRetryable(call: ToolLoopCall): void
  snapshot(runId: string): ToolLoopSnapshot
  exportRun(runId: string): LoopGuardState | undefined
  restoreRun(runId: string, state: LoopGuardState | undefined, executed: number): void
  raiseFromSettledCalls(runId: string, settled: SettledToolCalls): void
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

  exportRun(runId: string): LoopGuardState | undefined {
    const state = this.runs.get(runId)
    if (!state) return undefined
    return {
      proposed: state.proposed,
      occurrences: [...state.occurrences].map(([key, count]) => ({ ...parseCallKey(key), count })),
      nonRetryableCalls: [...state.nonRetryableCalls].map(key => parseCallKey(key)),
    }
  }

  /**
   * 计数恢复到不低于 checkpoint 记录的水平：重启后按 checkpoint 恢复，
   * 同进程内已有的计数只抬高不降低，避免恢复过程放宽调用保护。
   */
  restoreRun(runId: string, state: LoopGuardState | undefined, executed: number): void {
    const current = this.runs.get(runId)
    const occurrences = new Map(current?.occurrences ?? [])
    for (const item of state?.occurrences ?? []) {
      const key = callKey(item)
      occurrences.set(key, Math.max(occurrences.get(key) ?? 0, item.count))
    }
    this.runs.set(runId, {
      proposed: Math.max(current?.proposed ?? 0, state?.proposed ?? 0),
      executed: Math.max(current?.executed ?? 0, executed),
      maxToolCalls: current?.maxToolCalls ?? this.maxToolCalls,
      occurrences,
      nonRetryableCalls: new Set([
        ...(current?.nonRetryableCalls ?? []),
        ...(state?.nonRetryableCalls ?? []).map(item => callKey(item)),
      ]),
    })
  }

  /** 只抬高不降低，因此可以重复应用；用于把崩溃批次已落地的调用计入计数。 */
  raiseFromSettledCalls(runId: string, settled: SettledToolCalls): void {
    const state = this.state(runId)
    state.proposed = Math.max(state.proposed, settled.proposed)
    state.executed = Math.max(state.executed, settled.executed)
    const occurrences = new Map<string, number>()
    for (const call of settled.occurrences) {
      const key = callKey(call)
      occurrences.set(key, (occurrences.get(key) ?? 0) + 1)
    }
    for (const [key, count] of occurrences)
      state.occurrences.set(key, Math.max(state.occurrences.get(key) ?? 0, count))
    for (const call of settled.nonRetryableCalls) state.nonRetryableCalls.add(callKey(call))
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

function callKey(call: { operationId: string, argumentsHash: string }): string {
  return `${call.operationId}\u0000${call.argumentsHash}`
}

// 计数键使用 NUL 分隔，不能直接进入 jsonb；导出时拆回结构化字段。
function parseCallKey(key: string): { operationId: string, argumentsHash: string } {
  const separator = key.indexOf("\u0000")
  return separator < 0
    ? { operationId: key, argumentsHash: "" }
    : { operationId: key.slice(0, separator), argumentsHash: key.slice(separator + 1) }
}

function stop(code: ToolLoopStoppedError["code"], call: ToolLoopCall): ToolLoopStoppedError {
  return new ToolLoopStoppedError(code, call.operationId)
}
