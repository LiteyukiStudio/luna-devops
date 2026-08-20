import type { ToolLoopFingerprint, ToolLoopStop } from "./contracts.js"

export const DEFAULT_RUN_MAX_TOOL_CALLS = 256
export const MIN_RUN_MAX_TOOL_CALLS = 32
export const MAX_RUN_MAX_TOOL_CALLS = 2048

export type ToolLoopCall = {
  runId: string
  operationId: string
  argumentsHash: string
}

export type ToolLoopFailure = ToolLoopCall & {
  errorCode: string
  stableResultHash?: string
  deterministic: boolean
}

export type ToolLoopResult = ToolLoopCall & {
  stableResultHash: string
}

export type ToolLoopSnapshot = {
  proposed: number
  executed: number
  maxToolCalls: number
}

export interface LoopGuard {
  setMaxToolCalls(limit: number): void
  beforePropose(call: ToolLoopCall): void
  beforeExecute(call: ToolLoopCall): void
  recordFailure(failure: ToolLoopFailure): void
  recordResult(result: ToolLoopResult): void
  seedResult(result: ToolLoopResult): void
  snapshot(runId: string): ToolLoopSnapshot
  clearRun(runId: string): void
}

export class ToolLoopStoppedError extends Error implements ToolLoopStop {
  readonly retryable = false as const

  constructor(
    readonly code: ToolLoopStop["code"],
    readonly fingerprint: ToolLoopFingerprint,
  ) {
    super(code)
    this.name = "ToolLoopStoppedError"
  }

  toJSON(): ToolLoopStop {
    return { code: this.code, retryable: this.retryable, fingerprint: this.fingerprint }
  }
}

type RunLoopState = {
  proposed: number
  executed: number
  maxToolCalls: number
  deterministicFailures: Map<string, ToolLoopFingerprint>
  results: Map<string, { fingerprint: ToolLoopFingerprint, occurrences: number }>
}

export class InMemoryLoopGuard implements LoopGuard {
  private maxToolCalls: number
  private readonly runs = new Map<string, RunLoopState>()

  constructor(options: {
    maxToolCalls?: number
    isAsyncReadbackOperation?: (operationId: string) => boolean
    repeatedResultThreshold?: number
  } = {}) {
    this.maxToolCalls = validateMaxToolCalls(options.maxToolCalls ?? DEFAULT_RUN_MAX_TOOL_CALLS)
    this.isAsyncReadbackOperation = options.isAsyncReadbackOperation ?? (() => false)
    this.repeatedResultThreshold = Math.max(2, options.repeatedResultThreshold ?? 2)
  }

  private readonly isAsyncReadbackOperation: (operationId: string) => boolean
  private readonly repeatedResultThreshold: number

  setMaxToolCalls(limit: number): void {
    this.maxToolCalls = validateMaxToolCalls(limit)
  }

  beforePropose(call: ToolLoopCall): void {
    const state = this.state(call.runId)
    state.proposed += 1
    if (state.proposed > state.maxToolCalls)
      throw stop("ai.run_tool_call_budget_exceeded", call)

    const key = callKey(call)
    const deterministicFailure = state.deterministicFailures.get(key)
    if (deterministicFailure)
      throw new ToolLoopStoppedError("ai.tool_deterministic_failure_repeated", deterministicFailure)

    const result = state.results.get(key)
    if (!this.isAsyncReadbackOperation(call.operationId) && result && result.occurrences >= this.repeatedResultThreshold)
      throw new ToolLoopStoppedError("ai.tool_no_new_information", result.fingerprint)
  }

  beforeExecute(call: ToolLoopCall): void {
    const state = this.state(call.runId)
    state.executed += 1
    if (state.executed > state.maxToolCalls)
      throw stop("ai.run_tool_call_budget_exceeded", call)
  }

  recordFailure(failure: ToolLoopFailure): void {
    if (!failure.deterministic) return
    const fingerprint: ToolLoopFingerprint = {
      operationId: failure.operationId,
      argumentsHash: failure.argumentsHash,
      stableErrorCode: failure.errorCode,
      ...(failure.stableResultHash ? { stableResultHash: failure.stableResultHash } : {}),
    }
    this.state(failure.runId).deterministicFailures.set(callKey(failure), fingerprint)
  }

  recordResult(result: ToolLoopResult): void {
    if (this.isAsyncReadbackOperation(result.operationId)) return
    const state = this.state(result.runId)
    const key = callKey(result)
    const previous = state.results.get(key)
    const fingerprint: ToolLoopFingerprint = {
      operationId: result.operationId,
      argumentsHash: result.argumentsHash,
      stableResultHash: result.stableResultHash,
    }
    state.results.set(key, {
      fingerprint,
      occurrences: previous?.fingerprint.stableResultHash === result.stableResultHash ? previous.occurrences + 1 : 1,
    })
  }

  seedResult(result: ToolLoopResult): void {
    if (this.isAsyncReadbackOperation(result.operationId)) return
    const fingerprint: ToolLoopFingerprint = {
      operationId: result.operationId,
      argumentsHash: result.argumentsHash,
      stableResultHash: result.stableResultHash,
    }
    this.state(result.runId).results.set(callKey(result), {
      fingerprint,
      occurrences: this.repeatedResultThreshold,
    })
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
      state = {
        proposed: 0,
        executed: 0,
        maxToolCalls: this.maxToolCalls,
        deterministicFailures: new Map(),
        results: new Map(),
      }
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

function stop(code: ToolLoopStop["code"], call: ToolLoopCall): ToolLoopStoppedError {
  return new ToolLoopStoppedError(code, { operationId: call.operationId, argumentsHash: call.argumentsHash })
}
