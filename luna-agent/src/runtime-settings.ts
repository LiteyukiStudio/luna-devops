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
  // Model iterations and wall time bound runaway loops. Tool calls are not capped.
  maxModelSteps: 48,
} as const
