export interface RuntimeSettings {
  providerTimeoutMs: number
  maxRequestRetries: number
  runTimeoutMs: number
  agentConcurrentRuns: number
  userConcurrentRuns: number
  contextInputTokenBudget: number
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
  providerTimeoutMs: 300_000,
  maxRequestRetries: 5,
  runTimeoutMs: 3_600_000,
  agentConcurrentRuns: 10,
  userConcurrentRuns: 10,
  contextInputTokenBudget: 1024 * 1024,
  assistantMaxOutputTokens: 64 * 1024,
  maxModelSteps: 256,
  runMaxToolCalls: 256,
  maxInputBytes: 1024 * 1024,
  navigateActionTtlSeconds: 120,
  toolResultPayloadBudget: 512 * 1024,
  maxCardRepairAttempts: 5,
  contextCompressionTriggerRatio: 0.9,
  contextCompressionTargetRatio: 0.7,
  contextRecentTurnCount: 16,
  contextMaxRecentTurnCount: 32,
  contextMaxUncompressedTurnCount: 64,
  contextMaxCompressionTurnsPerCompile: 512,
  contextSummaryInputTokenBudget: 256 * 1024,
  contextSummaryMaxOutputTokens: 16 * 1024,
  contextHistoricalToolTokenBudget: 64 * 1024,
}

// 平台内部时序参数：保持与平台配置无关，无需暴露为可配置项。
export const agentRuntimeInternals = {
  configRefreshMs: 30_000,
  runPollMs: 500,
  runLeaseSeconds: 30,
} as const
