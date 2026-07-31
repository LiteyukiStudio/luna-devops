import type { InteractionCardGroup } from './interaction-card-schema'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { extremeInteractionCardFixture, interactionCardTemplateFixtures, templateSelectionInteractionCardFixture } from './interaction-card-fixtures'
import { interactionCardTemplateConfigs } from './interaction-card-templates'
import { AIInteractionCards } from './interaction-cards'

const expectedContentBlocks = {
  catalog: ['key_value'],
  comparison: ['metrics', 'data_table'],
  inspector: ['key_value', 'relations', 'resource_links'],
  form: ['callout'],
  wizard: [],
  diagnosis: ['callout', 'status_list', 'code'],
  plan: ['timeline', 'callout'],
  progress: ['progress', 'status_list'],
  result: ['callout', 'key_value', 'resource_links'],
  dashboard: ['metrics', 'chart', 'status_list'],
} as const

beforeAll(async () => {
  await i18next.changeLanguage('zh-CN')
})

describe.each(Object.entries(interactionCardTemplateFixtures))('%s interaction card template', (template, fixture) => {
  it('renders its intended structure and expansion behavior', () => {
    const { container } = render(<AIInteractionCards arguments={fixture} onAction={vi.fn()} />)
    const group = container.querySelector(`[data-ai-card-group="${template}"]`)
    expect(group).not.toBeNull()
    expect(group).toHaveAttribute('data-ai-card-density', interactionCardTemplateConfigs[template as keyof typeof interactionCardTemplateConfigs].defaultDensity)
    expect(group).toHaveAttribute('data-ai-card-mode', fixture.mode)
    expect(container.querySelectorAll(`[data-ai-card-template="${template}"]`)).toHaveLength(fixture.cards.length)

    const toggleButtons = screen.queryAllByRole('button', { name: '展开或收起卡片详情' })
    if (toggleButtons.length > 0) {
      const shouldExpand = interactionCardTemplateConfigs[template as keyof typeof interactionCardTemplateConfigs].expandByDefault || fixture.cards.some(card => 'form' in card && Boolean(card.form))
      expect(toggleButtons[0]).toHaveAttribute('aria-expanded', String(shouldExpand))
      if (!shouldExpand)
        fireEvent.click(toggleButtons[0]!)
    }

    for (const blockType of expectedContentBlocks[template as keyof typeof expectedContentBlocks])
      expect(container.querySelector(`[data-ai-content-block="${blockType}"]`)).not.toBeNull()
  })
})

describe('interaction card template edge cases', () => {
  it('keeps maximum-size candidate sets bounded and their long content wrappable', () => {
    const { container } = render(<AIInteractionCards arguments={extremeInteractionCardFixture} onAction={vi.fn()} />)

    expect(container.querySelectorAll('[data-ai-card]')).toHaveLength(12)
    expect(container.querySelector('[data-ai-card-group="catalog"]')).toHaveClass('min-w-0')
    expect(screen.getAllByRole('button', { name: /选择第/ })).toHaveLength(12)
    expect(container.querySelector('[data-ai-card-sources]')).not.toBeNull()
  })

  it('keeps wide comparison tables inside their own horizontal scroll container', () => {
    const { container } = render(<AIInteractionCards arguments={interactionCardTemplateFixtures.comparison} onAction={vi.fn()} />)
    const table = screen.getByRole('table')
    const scroller = table.parentElement

    expect(scroller).toHaveClass('max-w-full', 'overflow-x-auto')
    expect(container.querySelector('[data-ai-card-group="comparison"]')).toHaveClass('min-w-0')
  })

  it('shows trusted sources with a readable label and hidden trust description', () => {
    render(<AIInteractionCards arguments={interactionCardTemplateFixtures.inspector} onAction={vi.fn()} />)
    const sources = screen.getByText('Luna DevOps 实时数据')

    expect(sources).toBeVisible()
    expect(within(sources.parentElement!).getByText('平台数据')).toHaveClass('sr-only')
  })

  it('renders semantic error and trend states without raw status text controls', () => {
    const { container } = render(<AIInteractionCards arguments={interactionCardTemplateFixtures.diagnosis} onAction={vi.fn()} />)
    expect(screen.getByText('阻断构建')).toHaveClass('bg-danger-subtle', 'text-danger')

    const dashboard = render(<AIInteractionCards arguments={interactionCardTemplateFixtures.dashboard} onAction={vi.fn()} />)
    expect(dashboard.container.querySelector('[data-ai-content-block="metrics"] svg')).not.toBeNull()
    expect(container.querySelector('[data-ai-content-block="callout"]')).not.toBeNull()
  })

  it.each(['line', 'bar', 'area', 'donut'] as const)('renders the %s chart with the declared chart semantics', (chartType) => {
    const fixture: InteractionCardGroup = structuredClone(interactionCardTemplateFixtures.dashboard)
    const chart = fixture.cards[0].blocks?.find(block => block.type === 'chart')
    if (!chart || chart.type !== 'chart')
      throw new Error('dashboard chart fixture is missing')
    chart.chartType = chartType

    const { container } = render(<AIInteractionCards arguments={fixture} onAction={vi.fn()} />)
    expect(container.querySelector(`[data-ai-chart-type="${chartType}"]`)).toHaveAccessibleName('请求量')
  })

  it('renders segmented choices as choices instead of silently falling back to a select', () => {
    render(<AIInteractionCards arguments={interactionCardTemplateFixtures.wizard} onAction={vi.fn()} />)

    expect(screen.queryByRole('combobox', { name: '代码源' })).not.toBeInTheDocument()
    expect(screen.getByRole('radio', { name: 'GitHub' })).toBeVisible()
    expect(screen.getByRole('radio', { name: 'Gitea' })).toBeVisible()
  })

  it('uses a compact selectable field instead of a long display-only list for many candidates', () => {
    const { container } = render(<AIInteractionCards arguments={templateSelectionInteractionCardFixture} onAction={vi.fn()} />)

    expect(container.querySelector('[data-ai-card-mode="interactive"]')).not.toBeNull()
    expect(screen.getByRole('combobox', { name: /应用模板/ })).toBeVisible()
    expect(screen.getAllByRole('option')).toHaveLength(9)
    expect(container.querySelector('[data-ai-content-block="item_list"]')).toBeNull()
    expect(screen.getByRole('button', { name: '继续配置' })).toBeDisabled()
  })

  it('locks one-time actions after success and keeps navigation actions repeatable', async () => {
    const onAction = vi.fn().mockResolvedValue(true)
    render(<AIInteractionCards arguments={interactionCardTemplateFixtures.catalog} onAction={onAction} />)
    fireEvent.click(screen.getByRole('button', { name: '展开或收起卡片详情' }))
    const choose = screen.getByRole('button', { name: '选择生产 Harbor' })
    fireEvent.click(choose)
    await waitFor(() => expect(choose).toBeDisabled())

    const navigation = vi.fn().mockResolvedValue(true)
    render(<AIInteractionCards arguments={interactionCardTemplateFixtures.result} onAction={navigation} />)
    const openEvents = screen.getByRole('button', { name: '查看事件' })
    fireEvent.click(openEvents)
    await waitFor(() => expect(navigation).toHaveBeenCalledOnce())
    expect(openEvents).toBeEnabled()
  })

  it('keeps an explicitly repeatable refresh action available after success', async () => {
    const onAction = vi.fn().mockResolvedValue(true)
    render(<AIInteractionCards arguments={interactionCardTemplateFixtures.progress} onAction={onAction} />)
    const refresh = screen.getByRole('button', { name: '刷新发布状态' })

    fireEvent.click(refresh)
    await waitFor(() => expect(onAction).toHaveBeenCalledOnce())
    expect(refresh).toBeEnabled()
    fireEvent.click(refresh)
    await waitFor(() => expect(onAction).toHaveBeenCalledTimes(2))
  })
})
