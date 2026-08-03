import { fireEvent, render, screen } from '@testing-library/react'
import { createRef } from 'react'
import { describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { AIAssistantComposer } from './composer'

function renderComposer({ activeRun = false, onCancel = vi.fn(), onDraftChange = vi.fn(), onSubmit = vi.fn() } = {}) {
  render(
    <AIAssistantComposer
      activeRun={activeRun}
      canceling={false}
      canCancel={activeRun}
      draft="测试消息"
      inputRef={createRef<HTMLTextAreaElement>()}
      sending={false}
      submitting={false}
      waitingInput={false}
      onCancel={onCancel}
      onDraftChange={onDraftChange}
      onSubmit={onSubmit}
    />,
  )
  return {
    input: screen.getByRole('textbox', { name: i18next.t('aiAssistant.inputLabel') }),
    onCancel,
    onDraftChange,
    onSubmit,
  }
}

describe('ai assistant composer keyboard submission', () => {
  it('does not submit when Enter confirms an IME candidate', () => {
    const onSubmit = vi.fn()
    const { input } = renderComposer({ onSubmit })

    fireEvent.keyDown(input, { key: 'Enter', isComposing: true })
    fireEvent.keyDown(input, { key: 'Enter', keyCode: 229 })

    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('submits with plain Enter and keeps Shift+Enter for a newline', () => {
    const onSubmit = vi.fn()
    const { input } = renderComposer({ onSubmit })

    fireEvent.keyDown(input, { key: 'Enter', shiftKey: true })
    expect(onSubmit).not.toHaveBeenCalled()

    fireEvent.keyDown(input, { key: 'Enter' })
    expect(onSubmit).toHaveBeenCalledOnce()
  })

  it('keeps the draft editable but blocks submission while the current run is active', () => {
    const onCancel = vi.fn()
    const onDraftChange = vi.fn()
    const onSubmit = vi.fn()
    const { input } = renderComposer({ activeRun: true, onCancel, onDraftChange, onSubmit })

    expect(input).toBeEnabled()
    expect(input).toHaveAttribute('placeholder', i18next.t('aiAssistant.inputRunning'))

    fireEvent.change(input, { target: { value: '下一条消息' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(onDraftChange).toHaveBeenCalledWith('下一条消息')
    expect(onSubmit).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: i18next.t('aiAssistant.stop') }))
    expect(onCancel).toHaveBeenCalledOnce()
  })
})
