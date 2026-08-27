export interface RuntimeSettings {
  // 权威平台配置：Provider 请求、Run 超时和调度并发。
  providerTimeoutMs: number
  maxRequestRetries: number
  runTimeoutMs: number
  agentConcurrentRuns: number
  userConcurrentRuns: number
  // Agent 固定不变量：模型循环与执行预算。
  assistantMaxOutputTokens: number
  maxModelSteps: number
  runMaxToolCalls: number
  // Agent 固定不变量：工具结果与卡片预算。
  toolResultPayloadBudget: number
  maxCardRepairAttempts: number
  // Agent 固定不变量：上下文与压缩预算。
  contextRecentTurnCount: number
  contextMaxUncompressedTurnCount: number
  contextMaxCompressionTurnsPerCompile: number
  contextSummaryMaxOutputTokens: number
  contextMaxHistoryPayloadBytes: number
  contextMaxSummaryPayloadBytes: number
  contextMaxContinuationPayloadBytes: number
}

export type RemoteRuntimeSettings = Pick<RuntimeSettings,
  | "providerTimeoutMs"
  | "maxRequestRetries"
  | "runTimeoutMs"
  | "agentConcurrentRuns"
  | "userConcurrentRuns"
>

export const defaultRuntimeSettings: RuntimeSettings = Object.freeze({
  providerTimeoutMs: 300_000,
  maxRequestRetries: 5,
  runTimeoutMs: 3_600_000,
  agentConcurrentRuns: 10,
  userConcurrentRuns: 10,
  assistantMaxOutputTokens: 64 * 1024,
  maxModelSteps: 256,
  runMaxToolCalls: 256,
  toolResultPayloadBudget: 512 * 1024,
  maxCardRepairAttempts: 2,
  contextRecentTurnCount: 16,
  contextMaxUncompressedTurnCount: 32,
  contextMaxCompressionTurnsPerCompile: 32,
  contextSummaryMaxOutputTokens: 16 * 1024,
  contextMaxHistoryPayloadBytes: 4 * 1024 * 1024,
  contextMaxSummaryPayloadBytes: 512 * 1024,
  contextMaxContinuationPayloadBytes: 1024 * 1024,
})

export function runtimeSettingsSnapshot(settings: RuntimeSettings): RuntimeSettings {
  return Object.freeze({
    ...defaultRuntimeSettings,
    providerTimeoutMs: settings.providerTimeoutMs,
    maxRequestRetries: settings.maxRequestRetries,
    runTimeoutMs: settings.runTimeoutMs,
    agentConcurrentRuns: settings.agentConcurrentRuns,
    userConcurrentRuns: settings.userConcurrentRuns,
  })
}

export function runtimeSettingsFromRemote(settings: RemoteRuntimeSettings): RuntimeSettings {
  return runtimeSettingsSnapshot({ ...defaultRuntimeSettings, ...settings })
}

// 平台内部时序参数：保持与平台配置无关，无需暴露为可配置项。
export const agentRuntimeInternals = {
  configRefreshMs: 30_000,
  configFetchTimeoutMs: 10_000,
  runPollMs: 500,
} as const
