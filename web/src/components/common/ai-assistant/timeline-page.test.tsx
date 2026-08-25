import type { AIBlock } from './state'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { AIAssistantTimeline } from './timeline'

const blocks: AIBlock[] = [
  {
    id: 'page-user-message',
    turnId: 'page-turn',
    index: -1,
    type: 'message',
    role: 'user',
    status: 'completed',
    text: '检查移动页面',
    createdAt: '2026-08-25T09:00:00+08:00',
  },
  {
    id: 'page-assistant-message',
    turnId: 'page-turn',
    index: 0,
    type: 'message',
    role: 'assistant',
    status: 'completed',
    text: '移动页面已检查。',
    createdAt: '2026-08-25T09:00:01+08:00',
  },
]

const timelineProps = {
  error: null,
  generating: false,
  loading: false,
  onAction: vi.fn(async () => true),
  onApproval: vi.fn(async () => {}),
  onResend: vi.fn(),
  onRetry: vi.fn(),
  surface: 'page' as const,
}

const defaultResizeObserver = globalThis.ResizeObserver

afterEach(() => {
  Object.defineProperty(globalThis, 'ResizeObserver', {
    configurable: true,
    value: defaultResizeObserver,
  })
})

describe('ai assistant page timeline', () => {
  it('uses the full message lane for assistant content and page-sized message actions', () => {
    const { container } = render(
      <MemoryRouter>
        <AIAssistantTimeline {...timelineProps} blocks={blocks} />
      </MemoryRouter>,
    )

    expect(container.querySelector('[data-slot="ai-assistant-timeline"]')).toHaveAttribute('role', 'log')
    expect(container.querySelector('[data-slot="ai-assistant-timeline"]')).toHaveClass(
      'pl-[max(1rem,env(safe-area-inset-left))]',
      'pr-[max(1rem,env(safe-area-inset-right))]',
    )
    expect(container.querySelector('[data-ai-turn]')?.parentElement).toHaveClass('max-w-none')
    expect(container.querySelector('[data-ai-reply] [data-ai-message-group]')).toHaveClass('w-full', 'max-w-full')
    expect(container.querySelector('[data-ai-assistant-bubble]')).toHaveClass('w-full')
    for (const action of screen.getAllByRole('button', { name: i18next.t('aiAssistant.messageActions.copy') }))
      expect(action).toHaveClass('size-11')
  })

  it('follows within 48px and offers a page-local back-to-latest action beyond it', () => {
    const requestFrame = vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      callback(0)
      return 1
    })
    const cancelFrame = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
    const { container, rerender } = render(
      <MemoryRouter>
        <AIAssistantTimeline {...timelineProps} blocks={blocks} resetKey="page-conversation" />
      </MemoryRouter>,
    )
    const viewport = container.querySelector<HTMLElement>('[data-slot="ai-assistant-timeline"]')!
    Object.defineProperties(viewport, {
      clientHeight: { configurable: true, value: 400 },
      scrollHeight: { configurable: true, value: 1000 },
    })

    viewport.scrollTop = 560
    fireEvent.scroll(viewport)
    expect(screen.queryByRole('button', { name: i18next.t('aiAssistant.thinking.backToLatest') })).not.toBeInTheDocument()

    const updatedBlocks: AIBlock[] = [...blocks, {
      id: 'page-assistant-update',
      turnId: 'page-turn',
      index: 1,
      type: 'message',
      role: 'assistant',
      status: 'completed',
      text: '追加更新。',
      createdAt: '2026-08-25T09:00:02+08:00',
    }]
    rerender(
      <MemoryRouter>
        <AIAssistantTimeline {...timelineProps} blocks={updatedBlocks} resetKey="page-conversation" />
      </MemoryRouter>,
    )
    expect(viewport.scrollTop).toBe(1000)

    viewport.scrollTop = 500
    fireEvent.scroll(viewport)
    const backToLatest = screen.getByRole('button', { name: i18next.t('aiAssistant.thinking.backToLatest') })
    expect(backToLatest).toHaveClass('min-h-11')

    fireEvent.click(backToLatest)
    expect(viewport.scrollTop).toBe(1000)
    expect(screen.queryByRole('button', { name: i18next.t('aiAssistant.thinking.backToLatest') })).not.toBeInTheDocument()

    requestFrame.mockRestore()
    cancelFrame.mockRestore()
  })

  it('stays pinned across container resizes only while the user is following the latest turn', () => {
    let resizeCallback: ResizeObserverCallback | undefined
    class ResizeObserverTestDouble implements ResizeObserver {
      constructor(callback: ResizeObserverCallback) {
        resizeCallback = callback
      }

      disconnect() {}
      observe() {}
      unobserve() {}
    }
    Object.defineProperty(globalThis, 'ResizeObserver', {
      configurable: true,
      value: ResizeObserverTestDouble,
    })
    const requestFrame = vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      callback(0)
      return 1
    })
    const cancelFrame = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
    const { container, rerender } = render(
      <MemoryRouter>
        <AIAssistantTimeline {...timelineProps} blocks={blocks} resetKey="resizing-page" />
      </MemoryRouter>,
    )
    const viewport = container.querySelector<HTMLElement>('[data-slot="ai-assistant-timeline"]')!
    let clientHeight = 400
    Object.defineProperties(viewport, {
      clientHeight: { configurable: true, get: () => clientHeight },
      scrollHeight: { configurable: true, value: 1000 },
    })

    viewport.scrollTop = 560
    fireEvent.scroll(viewport)
    clientHeight = 320
    act(() => resizeCallback?.([], {} as ResizeObserver))
    expect(viewport.scrollTop).toBe(1000)

    viewport.scrollTop = 300
    fireEvent.scroll(viewport)
    clientHeight = 700
    act(() => resizeCallback?.([], {} as ResizeObserver))
    expect(viewport.scrollTop).toBe(300)

    rerender(
      <MemoryRouter>
        <AIAssistantTimeline
          {...timelineProps}
          blocks={[...blocks, {
            id: 'resize-update',
            turnId: 'page-turn',
            index: 2,
            type: 'message',
            role: 'assistant',
            status: 'completed',
            text: '尺寸变化后的更新。',
            createdAt: '2026-08-25T09:00:03+08:00',
          }]}
          resetKey="resizing-page"
        />
      </MemoryRouter>,
    )
    expect(viewport.scrollTop).toBe(300)

    requestFrame.mockRestore()
    cancelFrame.mockRestore()
  })
})
