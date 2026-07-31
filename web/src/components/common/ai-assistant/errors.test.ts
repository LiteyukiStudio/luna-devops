import { describe, expect, it } from 'vitest'
import { runFailureTranslationKey } from './errors'

describe('ai assistant error translations', () => {
  it('maps approval argument conflicts to a specific recovery message', () => {
    expect(runFailureTranslationKey('ai.approval_arguments_changed'))
      .toBe('aiAssistant.runFailure.approval_arguments_changed')
  })

  it('keeps unknown internal errors behind the generic fallback', () => {
    expect(runFailureTranslationKey('internal.stack_trace'))
      .toBe('aiAssistant.runFailure.generic')
  })
})
