import type { TFunction } from 'i18next'

export function diagnosisFailureLabel(t: TFunction, errorCode?: string): string {
  switch (errorCode) {
    case 'ai.wallet_balance_insufficient':
      return t('aiAssistant.deliveryDiagnosis.failedWalletInsufficient')
    case 'ai.disabled':
      return t('errors.ai.disabled')
    case 'ai.agent_unavailable':
      return t('errors.ai.agent_unavailable')
    case 'ai.not_configured':
      return t('errors.ai.not_configured')
    case 'ai.provider_unavailable':
      return t('errors.ai.provider_unavailable')
    case 'ai.timeout':
    case 'ai.provider_timeout':
      return t('errors.ai.provider_timeout')
    case 'ai.provider_rate_limited':
    case 'ai.rate_limited':
      return t('errors.ai.provider_rate_limited')
    case 'ai.model_not_available':
      return t('errors.ai.model_not_available')
    case 'ai.user_not_allowed':
      return t('errors.ai.user_not_allowed')
    case 'ai.structured_output_invalid':
      return t('errors.ai.structured_output_invalid')
    case 'auth.account.disabled':
      return t('errors.auth.account.disabled')
    default:
      return t('aiAssistant.deliveryDiagnosis.failed')
  }
}
