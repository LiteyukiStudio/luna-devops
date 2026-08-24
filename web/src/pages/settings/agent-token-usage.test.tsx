import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import i18next from '@/i18n'
import { AgentTokenUsageInline, AgentTokenUsageStrip } from './agent-token-usage'

const usage = {
  inputTokens: 0,
  outputTokens: 1234,
  cacheReadInputTokens: null,
  cacheWriteInputTokens: undefined,
  reasoningOutputTokens: 0,
}

describe('agent token usage presentation', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('zh-CN')
  })

  it('distinguishes explicit zero from an unavailable breakdown in the compact strip', () => {
    render(<AgentTokenUsageStrip usage={usage} />)

    expect(usageValue('输入 Token')).toBe('0')
    expect(usageValue('输出 Token')).toBe('1,234')
    expect(usageValue('缓存读取 Token')).toBe('—')
    expect(usageValue('缓存写入 Token')).toBe('—')
    expect(usageValue('推理输出 Token')).toBe('0')
  })

  it('shows the same five-field breakdown in inline contexts', () => {
    render(<AgentTokenUsageInline usage={usage} />)

    expect(usageValue('输入 Token')).toBe('0')
    expect(usageValue('输出 Token')).toBe('1,234')
    expect(usageValue('缓存读取 Token')).toBe('—')
    expect(usageValue('缓存写入 Token')).toBe('—')
    expect(usageValue('推理输出 Token')).toBe('0')
  })
})

function usageValue(label: string) {
  return screen.getByText(label).closest('div')?.querySelector('dd')?.textContent
}
