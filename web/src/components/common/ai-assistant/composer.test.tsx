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
      models={[{ id: 'aimod_test', name: 'Test model', maxContextTokens: 128_000, maxOutputTokens: 16_000 }]}
      selectedModelId="aimod_test"
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
  it('renders the conversation model picker inside the composer and locks it during a run', () => {
    const { rerender } = render(
      <AIAssistantComposer
        activeRun={false}
        canceling={false}
        canCancel={false}
        draft=""
        inputRef={createRef<HTMLTextAreaElement>()}
        models={[{ id: 'aimod_test', name: 'Test model', maxContextTokens: 128_000, maxOutputTokens: 16_000 }]}
        selectedModelId="aimod_test"
        sending={false}
        submitting={false}
        waitingInput={false}
        onCancel={vi.fn()}
        onDraftChange={vi.fn()}
        onModelChange={vi.fn()}
        onSubmit={vi.fn()}
      />,
    )
    const picker = screen.getByRole('combobox', { name: i18next.t('aiAssistant.modelLabel') })
    expect(picker.closest('footer')).not.toBeNull()
    expect(picker).toBeEnabled()

    rerender(
      <AIAssistantComposer
        activeRun
        canceling={false}
        canCancel
        draft=""
        inputRef={createRef<HTMLTextAreaElement>()}
        models={[{ id: 'aimod_test', name: 'Test model', maxContextTokens: 128_000, maxOutputTokens: 16_000 }]}
        modelSelectionDisabled
        selectedModelId="aimod_test"
        sending={false}
        submitting={false}
        waitingInput={false}
        onCancel={vi.fn()}
        onDraftChange={vi.fn()}
        onModelChange={vi.fn()}
        onSubmit={vi.fn()}
      />,
    )
    expect(screen.getByRole('combobox', { name: i18next.t('aiAssistant.modelLabel') })).toBeDisabled()
  })

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

  it('shows the provider-reported context usage as a ring with a formatted tooltip label', () => {
    render(
      <AIAssistantComposer
        activeRun={false}
        canceling={false}
        canCancel={false}
        contextUsedTokens={25600}
        draft="测试消息"
        inputRef={createRef<HTMLTextAreaElement>()}
        models={[{ id: 'aimod_test', name: 'Test model', maxContextTokens: 128_000, maxOutputTokens: 16_000 }]}
        selectedModelId="aimod_test"
        sending={false}
        submitting={false}
        waitingInput={false}
        onCancel={vi.fn()}
        onDraftChange={vi.fn()}
        onModelChange={vi.fn()}
        onSubmit={vi.fn()}
      />,
    )
    const ring = document.querySelector('span[aria-label*="/128k"]')
    expect(ring).not.toBeNull()
    expect(ring?.getAttribute('aria-label')).toContain('25.6k')
    expect(ring?.getAttribute('aria-label')).toContain('20%')
  })

  it('shows the run token budget as a ring to the left of the model switcher', () => {
    render(
      <AIAssistantComposer
        activeRun={false}
        canceling={false}
        canCancel={false}
        draft="测试消息"
        inputRef={createRef<HTMLTextAreaElement>()}
        models={[{ id: 'aimod_test', name: 'Test model', maxContextTokens: 128_000, maxOutputTokens: 16_000 }]}
        runTokenBudget={2_000_000}
        runUsedTokens={400_000}
        selectedModelId="aimod_test"
        sending={false}
        submitting={false}
        waitingInput={false}
        onCancel={vi.fn()}
        onDraftChange={vi.fn()}
        onModelChange={vi.fn()}
        onSubmit={vi.fn()}
      />,
    )
    const budgetRing = document.querySelector('span[aria-label*="/2000k"]')
    expect(budgetRing).not.toBeNull()
    expect(budgetRing?.getAttribute('aria-label')).toContain('400k')
    expect(budgetRing?.getAttribute('aria-label')).toContain('20%')
    // 预算环必须出现在模型切换器左边。
    const modelSwitcher = document.querySelector('button[aria-label]')
    expect(budgetRing && modelSwitcher
      ? Boolean(budgetRing.compareDocumentPosition(modelSwitcher) & Node.DOCUMENT_POSITION_FOLLOWING)
      : false).toBe(true)
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
