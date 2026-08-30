import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it } from 'vitest'
import i18next from '@/i18n'
import { AgentTokenUsageInline, AgentTokenUsageStrip } from './agent-token-usage'

const usage = {
  inputTokens: 0,
  outputTokens: 1234,
  cacheReadInputTokens: null,
  cacheWriteInputTokens: undefined,
  reasoningOutputTokens: 0,
  cacheHitRate: null,
}
const cacheHitRateDescription = '缓存读取 ÷ 输入；先汇总当前展示范围内纳入的模型调用再计算。缓存写入只计入输入分母。'

describe('agent token usage presentation', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('zh-CN')
  })

  it('distinguishes explicit zero from unavailable detail values in the strip', () => {
    render(<AgentTokenUsageStrip usage={usage} />)

    expect(usageValue('输入')).toBe('0')
    expect(usageValue('输出')).toBe('1,234')
    expect(usageValue('缓存读取')).toBe('—')
    expect(usageValue('缓存写入')).toBe('—')
    expect(usageValue('推理输出')).toBe('0')
    expect(usageValue('缓存命中率')).toBe('—')
    expect(screen.getByRole('button', { name: cacheHitRateDescription })).toBeInTheDocument()
    expect(screen.getByText('输入').closest('dl')).toHaveClass('@[60rem]/agent-usage:grid-cols-6')
    expect(screen.getByText('输入').closest('dl')).not.toHaveClass('lg:grid-cols-6')
  })

  it('exposes the weighted-rate explanation on hover', async () => {
    const user = userEvent.setup()
    render(<AgentTokenUsageStrip usage={usage} />)

    await user.hover(screen.getByRole('button', { name: cacheHitRateDescription }))
    expect(await screen.findByRole('tooltip')).toHaveTextContent(cacheHitRateDescription)
  })

  it('shows the same five-field breakdown in inline contexts', () => {
    render(<AgentTokenUsageInline usage={usage} />)

    expect(usageValue('输入')).toBe('0')
    expect(usageValue('输出')).toBe('1,234')
    expect(usageValue('缓存读取')).toBe('—')
    expect(usageValue('缓存写入')).toBe('—')
    expect(usageValue('推理输出')).toBe('0')
    expect(screen.queryByText('缓存命中率')).not.toBeInTheDocument()
  })

  it('limits summary contexts to input and output', () => {
    render(<AgentTokenUsageInline usage={usage} variant="summary" />)

    expect(usageValue('输入')).toBe('0')
    expect(usageValue('输出')).toBe('1,234')
    expect(screen.queryByText('缓存读取')).not.toBeInTheDocument()
    expect(screen.queryByText('缓存写入')).not.toBeInTheDocument()
    expect(screen.queryByText('推理输出')).not.toBeInTheDocument()
  })

  it('formats the backend cache hit percentage without losing explicit zero', () => {
    const { rerender } = render(<AgentTokenUsageStrip usage={{ ...usage, cacheHitRate: 12.34 }} />)
    expect(usageValue('缓存命中率')).toBe('12.3%')

    rerender(<AgentTokenUsageStrip usage={{ ...usage, cacheHitRate: 0.01 }} />)
    expect(usageValue('缓存命中率')).toBe('<0.1%')

    rerender(<AgentTokenUsageStrip usage={{ ...usage, cacheHitRate: 99.96 }} />)
    expect(usageValue('缓存命中率')).toBe('>99.9%')

    rerender(<AgentTokenUsageStrip usage={{ ...usage, cacheHitRate: 0 }} />)
    expect(usageValue('缓存命中率')).toBe('0%')
  })
})

function usageValue(label: string) {
  return screen.getByText(label).closest('div')?.querySelector('dd')?.textContent
}
