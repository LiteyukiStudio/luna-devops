export interface RuntimeSettings {
  providerTimeoutMs: number
  runTimeoutMs: number
  agentConcurrentRuns: number
}

export const defaultRuntimeSettings: RuntimeSettings = {
  providerTimeoutMs: 30_000,
  runTimeoutMs: 300_000,
  agentConcurrentRuns: 2,
}

export const agentRuntimeInternals = {
  configRefreshMs: 30_000,
  runPollMs: 500,
  runLeaseSeconds: 30,
  maxInputBytes: 48_000,
  // This bounds runaway model/tool loops; task completion is decided by workflow evidence.
  maxModelSteps: 48,
  maxToolCalls: 64,
} as const
