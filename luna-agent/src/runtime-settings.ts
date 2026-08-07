export interface RuntimeSettings {
  providerTimeoutMs: number
  runTimeoutMs: number
  agentConcurrentRuns: number
  userConcurrentRuns: number
  contextInputTokenBudget: number
  // 高级设置：模型与执行
  assistantMaxOutputTokens: number
  maxModelSteps: number
  maxInputBytes: number
  navigateActionTtlSeconds: number
  // 高级设置：工具结果与卡片
  toolResultPayloadBudget: number
  maxCardRepairAttempts: number
  // 高级设置：上下文与压缩
  contextCompressionTriggerRatio: number
  contextCompressionTargetRatio: number
  contextRecentTurnCount: number
  contextMaxRecentTurnCount: number
  contextMaxUncompressedTurnCount: number
  contextMaxCompressionTurnsPerCompile: number
  contextSummaryInputTokenBudget: number
  contextSummaryMaxOutputTokens: number
  contextHistoricalToolTokenBudget: number
}

export const defaultRuntimeSettings: RuntimeSettings = {
  providerTimeoutMs: 30_000,
  runTimeoutMs: 300_000,
  agentConcurrentRuns: 10,
  userConcurrentRuns: 10,
  contextInputTokenBudget: 256 * 1024,
  assistantMaxOutputTokens: 4096,
  maxModelSteps: 48,
  maxInputBytes: 48_000,
  navigateActionTtlSeconds: 60,
  toolResultPayloadBudget: 24_000,
  maxCardRepairAttempts: 3,
  contextCompressionTriggerRatio: 0.8,
  contextCompressionTargetRatio: 0.5,
  contextRecentTurnCount: 4,
  contextMaxRecentTurnCount: 8,
  contextMaxUncompressedTurnCount: 24,
  contextMaxCompressionTurnsPerCompile: 96,
  contextSummaryInputTokenBudget: 24_000,
  contextSummaryMaxOutputTokens: 1_500,
  contextHistoricalToolTokenBudget: 4_000,
}

// 平台内部时序参数：保持与平台配置无关，无需暴露为可配置项。
export const agentRuntimeInternals = {
  configRefreshMs: 30_000,
  runPollMs: 500,
  runLeaseSeconds: 30,
} as const
