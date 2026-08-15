import { describe, expect, it } from 'vitest'
import { getDocumentationUrl } from './docs-url'

describe('getDocumentationUrl', () => {
  it('uses the default-language route for Chinese documentation', () => {
    expect(getDocumentationUrl('/use/configuration/', 'zh-CN'))
      .toBe('https://luna-devops.liteyuki.org/use/configuration')
  })

  it('adds the English route prefix for English documentation', () => {
    expect(getDocumentationUrl('use/configuration', 'en-US'))
      .toBe('https://luna-devops.liteyuki.org/en/use/configuration')
  })
})
