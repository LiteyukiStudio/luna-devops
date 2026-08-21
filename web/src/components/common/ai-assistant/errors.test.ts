import { describe, expect, it } from 'vitest'
import { runFailureTranslationKey } from './errors'

describe('ai assistant error translations', () => {
  it.each([
    'ai.model_context_limit_invalid',
    'ai.model_context_insufficient',
    'ai.model_output_limit_invalid',
    'ai.wallet_balance_insufficient',
  ])('maps the stable model execution code %s to a public message', (code) => {
    expect(runFailureTranslationKey(code)).not.toBe('aiAssistant.runFailure.generic')
  })

  it('keeps unknown internal errors behind the generic fallback', () => {
    expect(runFailureTranslationKey('internal.stack_trace'))
      .toBe('aiAssistant.runFailure.generic')
  })
})
