const publicRunFailureCodes: ReadonlySet<string> = new Set([
  'ai.provider_auth_failed',
  'ai.provider_quota_exhausted',
  'ai.provider_timeout',
  'ai.provider_rate_limited',
  'ai.provider_unavailable',
  'ai.authorization_changed',
  'ai.run_actor_grant_invalid',
  'ai.tool_not_available',
  'ai.tool_execution_failed',
  'ai.tool_target_out_of_scope',
])

export function runFailureTranslationKey(errorCode?: string) {
  return `aiAssistant.runFailure.${errorCode && publicRunFailureCodes.has(errorCode) ? errorCode.slice(3).replaceAll('.', '_') : 'generic'}`
}
