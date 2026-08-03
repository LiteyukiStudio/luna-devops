import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createRef } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { AIDesktopShell } from './desktop-shell'
import {
  AI_ASSISTANT_SPLIT_MIN_WIDTH,
  resolveAIDesktopConversationLayout,
} from './layout'

describe('ai assistant desktop conversation layout', () => {
  it('uses an overlay drawer without squeezing compact chat windows', () => {
    expect(resolveAIDesktopConversationLayout(360)).toBe('overlay')
    expect(resolveAIDesktopConversationLayout(420)).toBe('overlay')
    expect(resolveAIDesktopConversationLayout(AI_ASSISTANT_SPLIT_MIN_WIDTH - 1)).toBe('overlay')
  })

  it('uses a persistent split sidebar when the assistant has enough room', () => {
    expect(resolveAIDesktopConversationLayout(AI_ASSISTANT_SPLIT_MIN_WIDTH)).toBe('split')
    expect(resolveAIDesktopConversationLayout(900)).toBe('split')
  })

  it('closes a compact drawer with Escape and returns focus to its trigger', async () => {
    const user = userEvent.setup()
    const triggerRef = createRef<HTMLButtonElement>()
    const onClose = vi.fn()
    render(
      <>
        <button ref={triggerRef} type="button">conversations</button>
        <AIDesktopShell
          chat={<main>chat</main>}
          closeLabel="back"
          conversationsOpen
          initialWidth={420}
          listButtonRef={triggerRef}
          conversationList={variant => <aside data-testid="conversation-browser">{variant}</aside>}
          onCloseConversations={onClose}
        />
      </>,
    )

    expect(screen.getByTestId('conversation-browser')).toHaveTextContent('drawer')
    await user.keyboard('{Escape}')
    expect(onClose).toHaveBeenCalledOnce()
    await waitFor(() => expect(triggerRef.current).toHaveFocus())
  })

  it('renders a wide conversation browser as a split sidebar without a scrim', () => {
    const triggerRef = createRef<HTMLButtonElement>()
    render(
      <AIDesktopShell
        chat={<main>chat</main>}
        closeLabel="back"
        conversationsOpen
        initialWidth={900}
        listButtonRef={triggerRef}
        conversationList={variant => <aside data-testid="conversation-browser">{variant}</aside>}
        onCloseConversations={vi.fn()}
      />,
    )

    expect(screen.getByTestId('conversation-browser')).toHaveTextContent('sidebar')
    expect(screen.queryByRole('button', { name: 'back' })).not.toBeInTheDocument()
  })
})
