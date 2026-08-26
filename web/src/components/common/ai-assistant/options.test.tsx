import type { AIUIAction } from '@/api'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { AIOptionsBar } from './options'

const actions = [
  {
    version: 1,
    id: 'projects',
    repeatable: true,
    type: 'navigate',
    label: '查看项目空间',
    description: '打开项目空间列表',
    payload: { routeName: 'projects', params: {}, query: {} },
  },
  {
    version: 1,
    id: 'continue',
    repeatable: false,
    type: 'send_message',
    label: '继续诊断',
    payload: { message: '请继续诊断最近失败的构建' },
  },
  {
    version: 1,
    id: 'retry',
    repeatable: false,
    type: 'request_tool',
    label: '重新执行',
    tone: 'danger',
    payload: { operationId: 'retryBuildRun', arguments: { runId: 'run_1' }, message: '请重新执行构建 run_1' },
  },
] satisfies AIUIAction[]

describe('ai assistant options', () => {
  it('renders all three supported next-step actions as visible buttons', async () => {
    await i18next.changeLanguage('zh-CN')
    render(<AIOptionsBar actions={actions} sourceKey="agent:turn-1" onAction={vi.fn()} />)

    expect(screen.getByRole('region', { name: i18next.t('aiAssistant.options.suggested') })).toBeVisible()
    expect(screen.getByRole('button', { name: /查看项目空间/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /继续诊断/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /重新执行/ })).toBeInTheDocument()
    expect(screen.queryByText('打开项目空间列表')).not.toBeInTheDocument()
  })

  it('wraps agent-created options inside the assistant bubble without floating positioning', () => {
    render(<AIOptionsBar actions={actions} placement="inline" sourceKey="agent:turn-inline" onAction={vi.fn()} />)

    const region = screen.getByRole('region', { name: i18next.t('aiAssistant.options.suggested') })
    expect(region).toHaveAttribute('data-ai-options-placement', 'inline')
    expect(screen.getByRole('button', { name: /查看项目空间/ })).toBeInTheDocument()
  })

  it('keeps floating page options inside horizontal safe areas', () => {
    render(<AIOptionsBar actions={actions} sourceKey="page:turn-1" surface="page" onAction={vi.fn()} />)

    const region = screen.getByRole('region', { name: i18next.t('aiAssistant.options.suggested') })
    expect(region).toHaveClass(
      'pl-[max(0.75rem,env(safe-area-inset-left))]',
      'pr-[max(0.75rem,env(safe-area-inset-right))]',
    )
    const option = screen.getByRole('button', { name: /查看项目空间/ })
    expect(option).toHaveClass('h-11', 'p-0')
    expect(option.firstElementChild).toHaveClass('h-7', 'px-2.5', 'text-xs')
  })

  it('keeps inline page options large enough for touch input', () => {
    render(<AIOptionsBar actions={actions} placement="inline" sourceKey="page:turn-inline" surface="page" onAction={vi.fn()} />)

    expect(screen.getByRole('button', { name: /查看项目空间/ })).toHaveClass('min-h-11', '!text-sm')
  })

  it('keeps repeatable navigation and sibling actions available after a one-time choice', async () => {
    const user = userEvent.setup()
    const onAction = vi.fn(async () => true)
    render(<AIOptionsBar actions={actions} sourceKey="agent:turn-1" onAction={onAction} />)

    const navigateButton = screen.getByRole('button', { name: /查看项目空间/ })
    const messageButton = screen.getByRole('button', { name: /继续诊断/ })
    const toolButton = screen.getByRole('button', { name: /重新执行/ })

    await user.click(navigateButton)
    await user.click(navigateButton)
    expect(onAction).toHaveBeenNthCalledWith(1, actions[0])
    expect(onAction).toHaveBeenNthCalledWith(2, actions[0])
    expect(navigateButton).toBeEnabled()
    expect(navigateButton).toHaveAttribute('aria-pressed', 'false')

    await user.click(screen.getByRole('button', { name: /继续诊断/ }))

    expect(onAction).toHaveBeenNthCalledWith(3, actions[1])
    expect(messageButton).toHaveAttribute('aria-pressed', 'true')
    expect(messageButton).toBeDisabled()
    expect(navigateButton).toBeEnabled()
    expect(toolButton).toBeEnabled()

    await user.click(toolButton)
    expect(onAction).toHaveBeenNthCalledWith(4, actions[2])
    expect(toolButton).toBeDisabled()
    expect(navigateButton).toBeEnabled()
  })

  it('locks only the option currently executing and prevents rapid duplicate submission', async () => {
    const user = userEvent.setup()
    let finish: ((success: boolean) => void) | undefined
    const onAction = vi.fn(() => new Promise<boolean>((resolve) => {
      finish = resolve
    }))
    render(<AIOptionsBar actions={actions} sourceKey="agent:turn-1" onAction={onAction} />)

    const navigateButton = screen.getByRole('button', { name: /查看项目空间/ })
    const messageButton = screen.getByRole('button', { name: /继续诊断/ })
    const toolButton = screen.getByRole('button', { name: /重新执行/ })
    await user.dblClick(messageButton)

    expect(onAction).toHaveBeenCalledTimes(1)
    expect(messageButton).toBeDisabled()
    expect(navigateButton).toBeEnabled()
    expect(toolButton).toBeEnabled()

    finish?.(true)
    await waitFor(() => expect(messageButton).toHaveAttribute('aria-pressed', 'true'))
  })

  it('fails closed when the model returns an arbitrary URL', async () => {
    const unsafe = [{
      version: 1,
      type: 'navigate',
      label: '打开外站',
      payload: { routeName: 'https://evil.example', params: {}, query: {} },
    }] as unknown as AIUIAction[]
    render(<AIOptionsBar actions={unsafe} sourceKey="agent:unsafe" onAction={vi.fn()} />)

    expect(screen.queryByRole('button', { name: '打开外站' })).not.toBeInTheDocument()
    expect(screen.queryByRole('region', { name: i18next.t('aiAssistant.options.suggested') })).not.toBeInTheDocument()
  })

  it('renders a consistent icon group in fixed visual slots', () => {
    const visualActions = actions.slice(0, 2).map((action, index) => ({
      ...action,
      visual: { type: 'icon' as const, value: index === 0 ? 'folder-kanban' as const : 'search' as const },
    }))
    const { container } = render(<AIOptionsBar actions={visualActions} sourceKey="agent:visual" onAction={vi.fn()} />)

    expect(container.querySelectorAll('[data-ai-option-visual]')).toHaveLength(2)
    expect(container.querySelectorAll('[data-ai-option-visual] svg')).toHaveLength(2)
  })

  it('fails closed on partial or mixed visual groups without affecting option actions', () => {
    const mixedActions = [
      { ...actions[0], visual: { type: 'emoji' as const, value: '📦' } },
      { ...actions[1], visual: { type: 'icon' as const, value: 'search' as const } },
      actions[2],
    ] satisfies AIUIAction[]
    const { container } = render(<AIOptionsBar actions={mixedActions} sourceKey="agent:mixed-visual" onAction={vi.fn()} />)

    expect(screen.getByRole('button', { name: /查看项目空间/ })).toBeInTheDocument()
    expect(container.querySelector('[data-ai-option-visual]')).not.toBeInTheDocument()
  })
})
