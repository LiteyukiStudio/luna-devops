import { describe, expect, it } from 'vitest'
import i18next from '@/i18n'
import { runFailureTranslationKey } from './errors'

describe('ai assistant error translations', () => {
  it.each([
    'ai.provider_usage_unavailable',
    'ai.provider_request_failed',
    'ai.provider_response_invalid',
    'ai.provider_stream_invalid',
    'ai.provider_empty_stream',
    'ai.provider_config_unavailable',
    'ai.not_configured',
    'ai.model_not_available',
    'ai.model_required',
    'ai.model_context_limit_invalid',
    'ai.model_context_insufficient',
    'ai.model_output_limit_invalid',
    'ai.model_protocol_violation',
    'ai.verification_inconclusive',
    'ai.model_output_truncated',
    'ai.model_output_filtered',
    'ai.wallet_balance_insufficient',
    'ai.run_timeout',
    'ai.final_response_missing',
    'ai.quota.user_concurrent_runs_exceeded',
  ])('maps the stable model execution code %s to a public message', (code) => {
    const key = runFailureTranslationKey(code)
    expect(key).not.toBe('aiAssistant.runFailure.generic')
    expect(i18next.exists(key)).toBe(true)
  })

  it('presents an explicit missing-usage/budget prompt for ai.provider_usage_unavailable', () => {
    const message = i18next.t(runFailureTranslationKey('ai.provider_usage_unavailable'), { lng: 'zh-CN' })
    expect(message).toContain('Token 用量/预算信息')
  })

  it('keeps unknown internal errors behind the generic fallback', () => {
    expect(runFailureTranslationKey('internal.stack_trace'))
      .toBe('aiAssistant.runFailure.generic')
  })
})
