import { describe, expect, it } from 'vitest'
import { changedConfigValues } from './site-settings-values'

describe('site settings changed values', () => {
  it('does not submit unchanged fields with an unrelated setting', () => {
    const current = {
      'site.title': 'Luna DevOps',
      'site.description': 'Deployment platform',
    }

    expect(changedConfigValues({
      ...current,
      'site.title': 'My DevOps',
    }, current)).toEqual({ 'site.title': 'My DevOps' })
  })
})
