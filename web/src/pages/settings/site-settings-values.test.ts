import { describe, expect, it } from 'vitest'
import { changedConfigValues } from './site-settings-values'

describe('site settings changed values', () => {
  it('does not submit unchanged step-up fields with an unrelated setting', () => {
    const current = {
      'site.title': 'Luna DevOps',
      'security.stepUpMfa.enabled': 'false',
      'security.stepUpMfa.idleTimeoutMinutes': '10',
      'security.stepUpMfa.absoluteTimeoutMinutes': '60',
    }

    expect(changedConfigValues({
      ...current,
      'site.title': 'My DevOps',
    }, current)).toEqual({ 'site.title': 'My DevOps' })
  })
})
