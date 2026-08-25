import type { ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AIAssistantDesktopHost } from './desktop-host'

const runtime = vi.hoisted(() => ({
  closeAssistant: vi.fn(),
  enabled: true,
  open: true,
  openAssistant: vi.fn(),
}))

vi.mock('react-rnd', () => ({
  Rnd: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}))

vi.mock('./runtime-context', () => ({
  useAIAssistantRuntime: () => runtime,
}))

vi.mock('./chat-surface', () => ({
  AIAssistantChatSurface: () => <div>Chat</div>,
}))

vi.mock('./desktop-shell', () => ({
  AIDesktopShell: ({ onCloseConversations, onOpenConversations }: {
    onCloseConversations: () => void
    onOpenConversations: () => void
  }) => (
    <div>
      <button type="button" onClick={onOpenConversations}>Open conversations</button>
      <button type="button" onClick={onCloseConversations}>Close conversations</button>
    </div>
  ),
}))

beforeEach(() => {
  runtime.enabled = true
  runtime.open = true
  runtime.closeAssistant.mockClear()
  runtime.openAssistant.mockClear()
})

describe('ai assistant desktop host view', () => {
  it('reports controlled conversation and chat view changes', async () => {
    const user = userEvent.setup()
    const onViewChange = vi.fn()
    const { rerender } = render(
      <AIAssistantDesktopHost view="chat" onViewChange={onViewChange} />,
    )

    await user.click(screen.getByRole('button', { name: 'Open conversations' }))
    expect(onViewChange).toHaveBeenLastCalledWith('conversations')

    rerender(<AIAssistantDesktopHost view="conversations" onViewChange={onViewChange} />)
    await user.click(screen.getByRole('button', { name: 'Close conversations' }))
    expect(onViewChange).toHaveBeenLastCalledWith('chat')
  })
})
