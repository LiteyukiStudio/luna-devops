export interface RuntimeSettings {
  // 权威平台配置：Provider 请求、Run 超时和调度并发。
  providerTimeoutMs: number
  maxRequestRetries: number
  runTimeoutMs: number
  agentConcurrentRuns: number
  userConcurrentRuns: number
  // Agent 模型循环与执行预算（支持环境变量显式覆盖）。
  assistantMaxOutputTokens: number
  maxModelSteps: number
  runMaxToolCalls: number
  // Agent 固定不变量：卡片修复与结构化摘要参数。
  maxCardRepairAttempts: number
  contextRecentTurnCount: number
  contextMaxCompressionTurnsPerCompile: number
  contextSummaryMaxOutputTokens: number
}

export type RemoteRuntimeSettings = Pick<RuntimeSettings,
  | "providerTimeoutMs"
  | "maxRequestRetries"
  | "runTimeoutMs"
  | "agentConcurrentRuns"
  | "userConcurrentRuns"
>

export type LocalBudgetOverrides = Partial<Pick<RuntimeSettings, "maxModelSteps" | "runMaxToolCalls">>

export const defaultRuntimeSettings: RuntimeSettings = Object.freeze({
  providerTimeoutMs: 300_000,
  maxRequestRetries: 5,
  runTimeoutMs: 3_600_000,
  agentConcurrentRuns: 10,
  userConcurrentRuns: 10,
  assistantMaxOutputTokens: 64 * 1024,
  maxModelSteps: 32,
  runMaxToolCalls: 64,
  maxCardRepairAttempts: 2,
  contextRecentTurnCount: 16,
  contextMaxCompressionTurnsPerCompile: 32,
  contextSummaryMaxOutputTokens: 16 * 1024,
})

export function runtimeSettingsSnapshot(settings: RuntimeSettings): RuntimeSettings {
  return Object.freeze({
    ...defaultRuntimeSettings,
    providerTimeoutMs: settings.providerTimeoutMs,
    maxRequestRetries: settings.maxRequestRetries,
    runTimeoutMs: settings.runTimeoutMs,
    agentConcurrentRuns: settings.agentConcurrentRuns,
    userConcurrentRuns: settings.userConcurrentRuns,
    assistantMaxOutputTokens: settings.assistantMaxOutputTokens,
    maxModelSteps: settings.maxModelSteps,
    runMaxToolCalls: settings.runMaxToolCalls,
    maxCardRepairAttempts: settings.maxCardRepairAttempts,
    contextRecentTurnCount: settings.contextRecentTurnCount,
    contextMaxCompressionTurnsPerCompile: settings.contextMaxCompressionTurnsPerCompile,
    contextSummaryMaxOutputTokens: settings.contextSummaryMaxOutputTokens,
  })
}

export function runtimeSettingsFromRemote(
  settings: RemoteRuntimeSettings,
  overrides: LocalBudgetOverrides = {},
): RuntimeSettings {
  return runtimeSettingsSnapshot({
    ...defaultRuntimeSettings,
    ...settings,
    ...(overrides.maxModelSteps !== undefined ? { maxModelSteps: overrides.maxModelSteps } : {}),
    ...(overrides.runMaxToolCalls !== undefined ? { runMaxToolCalls: overrides.runMaxToolCalls } : {}),
  })
}

// 平台内部时序参数：保持与平台配置无关，无需暴露为可配置项。
export const agentRuntimeInternals = {
  configRefreshMs: 30_000,
  configFetchTimeoutMs: 10_000,
  runPollMs: 500,
} as const
