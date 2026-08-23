export interface RuntimeSettings {
  providerTimeoutMs: number
  maxRequestRetries: number
  runTimeoutMs: number
  agentConcurrentRuns: number
  userConcurrentRuns: number
  // 高级设置：模型与执行
  assistantMaxOutputTokens: number
  maxModelSteps: number
  runMaxToolCalls: number
  maxInputBytes: number
  navigateActionTtlSeconds: number
  // 高级设置：工具结果与卡片
  toolResultPayloadBudget: number
  maxCardRepairAttempts: number
  // 高级设置：上下文与压缩
  contextCompressionTriggerRatio: number
  contextRecentTurnCount: number
  contextMaxUncompressedTurnCount: number
  contextMaxCompressionTurnsPerCompile: number
  contextSummaryMaxOutputTokens: number
  contextMaxHistoryPayloadBytes: number
  contextMaxSummaryPayloadBytes: number
  contextMaxContinuationPayloadBytes: number
}

export type RemoteRuntimeSettings = Omit<RuntimeSettings,
  | "toolResultPayloadBudget"
  | "contextCompressionTriggerRatio"
  | "contextRecentTurnCount"
  | "contextMaxHistoryPayloadBytes"
  | "contextMaxSummaryPayloadBytes"
  | "contextMaxContinuationPayloadBytes"
>

export const defaultRuntimeSettings: RuntimeSettings = {
  providerTimeoutMs: 300_000,
  maxRequestRetries: 5,
  runTimeoutMs: 3_600_000,
  agentConcurrentRuns: 10,
  userConcurrentRuns: 10,
  assistantMaxOutputTokens: 64 * 1024,
  maxModelSteps: 256,
  runMaxToolCalls: 256,
  maxInputBytes: 1024 * 1024,
  navigateActionTtlSeconds: 120,
  toolResultPayloadBudget: 512 * 1024,
  maxCardRepairAttempts: 5,
  contextCompressionTriggerRatio: 0.9,
  contextRecentTurnCount: 16,
  contextMaxUncompressedTurnCount: 64,
  contextMaxCompressionTurnsPerCompile: 512,
  contextSummaryMaxOutputTokens: 16 * 1024,
  contextMaxHistoryPayloadBytes: 4 * 1024 * 1024,
  contextMaxSummaryPayloadBytes: 512 * 1024,
  contextMaxContinuationPayloadBytes: 1024 * 1024,
}

// 平台内部时序参数：保持与平台配置无关，无需暴露为可配置项。
export const agentRuntimeInternals = {
  configRefreshMs: 30_000,
  runPollMs: 500,
} as const
