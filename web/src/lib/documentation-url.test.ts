import { describe, expect, it } from 'vitest'
import { documentationHomeUrl, mcpDocumentationUrl } from './documentation-url'

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

describe('mcpDocumentationUrl', () => {
  it('routes Chinese languages to Chinese MCP docs', () => {
    expect(mcpDocumentationUrl('zh-CN')).toBe('https://luna-devops.liteyuki.org/use/mcp')
    expect(mcpDocumentationUrl('zh-TW')).toBe('https://luna-devops.liteyuki.org/use/mcp')
  })

  it('routes other supported languages to English MCP docs', () => {
    expect(mcpDocumentationUrl('en-US')).toBe('https://luna-devops.liteyuki.org/en/use/mcp')
    expect(mcpDocumentationUrl('ja-JP')).toBe('https://luna-devops.liteyuki.org/en/use/mcp')
    expect(mcpDocumentationUrl('ko-KR')).toBe('https://luna-devops.liteyuki.org/en/use/mcp')
  })
})
