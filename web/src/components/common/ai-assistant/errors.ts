const publicRunFailureKeys: Readonly<Record<string, string>> = {
  'ai.provider_auth_failed': 'provider_auth_failed',
  'ai.provider_quota_exhausted': 'provider_quota_exhausted',
  'ai.provider_timeout': 'provider_timeout',
  'ai.provider_rate_limited': 'provider_rate_limited',
  'ai.provider_unavailable': 'provider_unavailable',
  'ai.provider_invalid_tool_arguments': 'provider_invalid_tool_arguments',
  'ai.authorization_changed': 'authorization_changed',
  'ai.model_context_limit_invalid': 'model_context_limit_invalid',
  'ai.model_context_insufficient': 'model_context_insufficient',
  'ai.model_output_limit_invalid': 'model_output_limit_invalid',
  'ai.run_token_budget_exhausted': 'run_token_budget_exhausted',
  'ai.run_credit_budget_exhausted': 'run_credit_budget_exhausted',
  'ai.wallet_balance_insufficient': 'wallet_balance_insufficient',
  'ai.limit_exceeded': 'limit_exceeded',
  'ai.run_failed': 'run_failed',
  'ai.tool_not_available': 'tool_not_available',
  'ai.tool_execution_failed': 'tool_execution_failed',
  'ai.tool_storage_unavailable': 'tool_storage_unavailable',
  'ai.tool_arguments_key_unavailable': 'tool_arguments_key_unavailable',
  'ai.tool_permission_denied': 'tool_permission_denied',
  'ai.sensitive_input_requires_user_form': 'sensitive_input_requires_user_form',
  'ai.web_target_blocked': 'web_target_blocked',
  'ai.web_content_rejected': 'web_content_rejected',
  'ai.web_request_failed': 'web_request_failed',
  'resource.not_found': 'resource_not_found',
  'resource.conflict': 'resource_conflict',
  'request.invalid': 'request_invalid',
  'verification_inconclusive': 'verification_inconclusive',
}

export function runFailureTranslationKey(errorCode?: string) {
  return `aiAssistant.runFailure.${errorCode ? publicRunFailureKeys[errorCode] ?? 'generic' : 'generic'}`
}
