import { describe, expect, it } from 'vitest'
import { cliDocumentationUrl, documentationHomeUrl } from './documentation-url'

describe('documentationHomeUrl', () => {
  it('routes Chinese languages to Chinese documentation root', () => {
    expect(documentationHomeUrl('zh-CN')).toBe('https://luna-devops.liteyuki.org')
    expect(documentationHomeUrl('zh-TW')).toBe('https://luna-devops.liteyuki.org')
  })

  it('routes other supported languages to English documentation root', () => {
    expect(documentationHomeUrl('en-US')).toBe('https://luna-devops.liteyuki.org/en/')
    expect(documentationHomeUrl('ja-JP')).toBe('https://luna-devops.liteyuki.org/en/')
    expect(documentationHomeUrl('ko-KR')).toBe('https://luna-devops.liteyuki.org/en/')
  })
})

describe('cliDocumentationUrl', () => {
  it('routes Chinese languages to Chinese CLI docs', () => {
    expect(cliDocumentationUrl('zh-CN')).toBe('https://luna-devops.liteyuki.org/use/cli')
    expect(cliDocumentationUrl('zh-TW')).toBe('https://luna-devops.liteyuki.org/use/cli')
  })

  it('routes other supported languages to English CLI docs', () => {
    expect(cliDocumentationUrl('en-US')).toBe('https://luna-devops.liteyuki.org/en/use/cli')
    expect(cliDocumentationUrl('ja-JP')).toBe('https://luna-devops.liteyuki.org/en/use/cli')
    expect(cliDocumentationUrl('ko-KR')).toBe('https://luna-devops.liteyuki.org/en/use/cli')
  })
})
