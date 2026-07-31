const publicRunFailureKeys: Readonly<Record<string, string>> = {
  'ai.provider_auth_failed': 'provider_auth_failed',
  'ai.provider_quota_exhausted': 'provider_quota_exhausted',
  'ai.provider_timeout': 'provider_timeout',
  'ai.provider_rate_limited': 'provider_rate_limited',
  'ai.provider_unavailable': 'provider_unavailable',
  'ai.provider_invalid_tool_arguments': 'provider_invalid_tool_arguments',
  'ai.authorization_changed': 'authorization_changed',
  'ai.run_actor_grant_invalid': 'run_actor_grant_invalid',
  'ai.limit_exceeded': 'limit_exceeded',
  'ai.run_failed': 'run_failed',
  'ai.tool_not_available': 'tool_not_available',
  'ai.tool_execution_failed': 'tool_execution_failed',
  'ai.tool_storage_unavailable': 'tool_storage_unavailable',
  'ai.tool_permission_denied': 'tool_permission_denied',
  'resource.not_found': 'resource_not_found',
  'resource.conflict': 'resource_conflict',
  'request.invalid': 'request_invalid',
  'verification_inconclusive': 'verification_inconclusive',
}

export function runFailureTranslationKey(errorCode?: string) {
  return `aiAssistant.runFailure.${errorCode ? publicRunFailureKeys[errorCode] ?? 'generic' : 'generic'}`
}
