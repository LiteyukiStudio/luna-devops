import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { FloatingFormSaveBar } from './floating-form-save-bar'

describe('floatingFormSaveBar', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('zh-CN')
  })

  it('stays hidden when clean and renders discard + save actions when dirty', async () => {
    const user = userEvent.setup()
    const onDiscard = vi.fn()
    const onSave = vi.fn()
    const { rerender } = render(
      <FloatingFormSaveBar open={false} onDiscard={onDiscard} onSave={onSave} />,
    )

    expect(document.querySelector('[data-slot="floating-form-save-bar"]')).toBeNull()

    rerender(
      <FloatingFormSaveBar
        open
        saveType="button"
        onDiscard={onDiscard}
        onSave={onSave}
      />,
    )

    expect(screen.getByText(i18next.t('common.unsavedChanges'))).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: i18next.t('common.discardChanges') }))
    expect(onDiscard).toHaveBeenCalledTimes(1)

    await user.click(screen.getByRole('button', { name: i18next.t('common.save') }))
    expect(onSave).toHaveBeenCalledTimes(1)
  })
})
