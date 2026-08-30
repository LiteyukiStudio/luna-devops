import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { BrandColorPresetField } from './brand-color-preset-field'

describe('brand color preset field', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('en-US')
  })

  it('keeps platform inheritance as a dedicated radio option', () => {
    const onValueChange = vi.fn()
    render(
      <TooltipProvider>
        <BrandColorPresetField
          ariaLabel="Color theme"
          inheritedPreset="harbor"
          inheritLabel="Follow platform"
          options={['aurora', 'blue']}
          value=""
          onValueChange={onValueChange}
        />
      </TooltipProvider>,
    )

    expect(screen.getByRole('radio', { name: 'Follow platform' })).toBeChecked()
    fireEvent.click(screen.getByRole('radio', { name: 'Blue' }))
    expect(onValueChange).toHaveBeenCalledWith('blue')
  })

  it('uses only the accepted catalog and falls back a hidden value to blue', () => {
    const { rerender } = render(
      <TooltipProvider>
        <BrandColorPresetField
          ariaLabel="Color theme"
          inheritedPreset="aurora"
          value="botanical"
          onValueChange={vi.fn()}
        />
      </TooltipProvider>,
    )

    expect(screen.getByRole('radio', { name: 'Botanical' })).toBeChecked()
    expect(screen.getByRole('radio', { name: 'Blue' })).toBeInTheDocument()
    expect(screen.queryByRole('radio', { name: 'Ruby' })).not.toBeInTheDocument()

    rerender(
      <TooltipProvider>
        <BrandColorPresetField
          ariaLabel="Color theme"
          inheritedPreset="aurora"
          value="ruby"
          onValueChange={vi.fn()}
        />
      </TooltipProvider>,
    )

    expect(screen.getByRole('radio', { name: 'Blue' })).toBeChecked()
    expect(screen.queryByRole('radio', { name: 'Ruby' })).not.toBeInTheDocument()
  })

  it('marks the bright lime swatch for a contrast-safe foreground', () => {
    const { container } = render(
      <TooltipProvider>
        <BrandColorPresetField
          ariaLabel="Color theme"
          value="lime"
          onValueChange={vi.fn()}
        />
      </TooltipProvider>,
    )

    expect(container.querySelector('#brand-color-lime + label .brand-theme-swatch')).toHaveAttribute('data-dark-foreground', 'true')
  })
})
