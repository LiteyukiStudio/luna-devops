import { fireEvent, render, screen } from '@testing-library/react'
import { createRef } from 'react'
import { describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { AIAssistantComposer } from './composer'

function renderComposer(onSubmit = vi.fn()) {
  render(
    <AIAssistantComposer
      activeRun={false}
      canceling={false}
      canCancel={false}
      draft="测试消息"
      inputRef={createRef<HTMLTextAreaElement>()}
      sending={false}
      submitting={false}
      waitingInput={false}
      onCancel={vi.fn()}
      onDraftChange={vi.fn()}
      onSubmit={onSubmit}
    />,
  )
  return {
    input: screen.getByRole('textbox', { name: i18next.t('aiAssistant.inputLabel') }),
    onSubmit,
  }
}

describe('ai assistant composer keyboard submission', () => {
  it('does not submit when Enter confirms an IME candidate', () => {
    const { input, onSubmit } = renderComposer()

    fireEvent.keyDown(input, { key: 'Enter', isComposing: true })
    fireEvent.keyDown(input, { key: 'Enter', keyCode: 229 })

    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('submits with plain Enter and keeps Shift+Enter for a newline', () => {
    const { input, onSubmit } = renderComposer()

    fireEvent.keyDown(input, { key: 'Enter', shiftKey: true })
    expect(onSubmit).not.toHaveBeenCalled()

    fireEvent.keyDown(input, { key: 'Enter' })
    expect(onSubmit).toHaveBeenCalledOnce()
  })
})
