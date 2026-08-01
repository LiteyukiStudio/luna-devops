import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { AIRefreshConversationReturn } from './refresh-conversation-return'

describe('ai refresh conversation return notice', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('returns to the previous conversation on demand', async () => {
    await i18next.changeLanguage('zh-CN')
    const onReturn = vi.fn()
    render(<AIRefreshConversationReturn expiresAt={Date.now() + 8_000} onExpire={vi.fn()} onReturn={onReturn} />)

    fireEvent.click(screen.getByRole('button', { name: /返回上一个会话/ }))

    expect(onReturn).toHaveBeenCalledOnce()
  })

  it('expires after the countdown', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-01T00:00:00.000Z'))
    const onExpire = vi.fn()
    render(<AIRefreshConversationReturn expiresAt={Date.now() + 8_000} onExpire={onExpire} onReturn={vi.fn()} />)

    await vi.advanceTimersByTimeAsync(8_000)

    expect(onExpire).toHaveBeenCalled()
  })
})
