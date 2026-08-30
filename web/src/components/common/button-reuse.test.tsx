import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ApplicationIconPicker } from './application-icon-picker'
import { APPLICATION_ICON_NAMES } from './application-icons'
import { CopyableHoverText } from './copyable-hover-text'
import { FormField } from './form-field'
import { SearchMultiSelect, SearchSelect } from './search-select'
import { UsageRing } from './usage-ring'

vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

describe('common button reuse', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('en-US')
  })

  it('keeps the form hint button as a focusable tooltip trigger', async () => {
    render(
      <TooltipProvider>
        <FormField hint="CPU allocation guidance" label="CPU">
          <input aria-label="CPU value" />
        </FormField>
      </TooltipProvider>,
    )

    const help = screen.getByRole('button', { name: /CPU/ })
    expect(help).toHaveClass('size-auto', 'p-0')
    expect(help).toHaveAttribute('tabindex', '-1')
    fireEvent.focus(help)
    expect((await screen.findAllByText('CPU allocation guidance'))[0]).toBeVisible()
  })

  it('keeps searchable options and multi-select clearing interactive', async () => {
    const interaction = userEvent.setup()
    const selectValue = vi.fn()
    const options = [
      { description: 'Primary project', label: 'Alpha', value: 'alpha' },
      { disabled: true, label: 'Disabled', value: 'disabled' },
    ]
    const single = render(<SearchSelect options={options} placeholder="Select project" value="" onValueChange={selectValue} />)

    await interaction.click(screen.getByRole('button', { name: 'Select project' }))
    const option = screen.getByRole('button', { name: /Alpha/ })
    expect(option).toHaveAttribute('data-slot', 'button')
    expect(screen.getByRole('button', { name: 'Disabled' })).toBeDisabled()
    await interaction.click(option)
    expect(selectValue).toHaveBeenCalledWith('alpha')

    single.unmount()
    const clearValue = vi.fn()
    render(<SearchMultiSelect options={options} placeholder="Select projects" value={['alpha']} onValueChange={clearValue} />)
    await interaction.click(screen.getByRole('button', { name: 'Select projects' }))
    const clear = screen.getByRole('button', { name: i18next.t('common.clearSelection') })
    expect(clear).toHaveAttribute('data-slot', 'button')
    await interaction.click(clear)
    expect(clearValue).toHaveBeenCalledWith([])
  })

  it('copies from both the value trigger and its tooltip action', async () => {
    const interaction = userEvent.setup()
    const writeText = vi.fn(async () => undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    render(<CopyableHoverText copyLabel="Copy value" value="registry.example/app">registry.example/app</CopyableHoverText>)

    const trigger = screen.getByRole('button', { name: 'Copy value' })
    expect(trigger).toHaveClass('h-auto', 'p-0')
    await interaction.hover(trigger)
    const tooltipAction = await waitFor(() => {
      const action = document.querySelector<HTMLButtonElement>('[data-slot="tooltip-content"] > [data-slot="button"]')
      expect(action).not.toBeNull()
      return action!
    })
    await interaction.click(tooltipAction)
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('registry.example/app'))

    await interaction.click(trigger)
    await waitFor(() => expect(writeText).toHaveBeenCalledTimes(2))
  })

  it('keeps usage and application icon buttons compact and accessible', async () => {
    const interaction = userEvent.setup()
    const changeIcon = vi.fn()
    const { unmount } = render(<UsageRing ariaLabel="CPU usage" ratio={0.75} tooltip="75% used" />)

    const usage = screen.getByRole('button', { name: 'CPU usage' })
    expect(usage).toHaveClass('size-7')
    await interaction.hover(usage)
    expect((await screen.findAllByText('75% used'))[0]).toBeVisible()

    unmount()
    render(<ApplicationIconPicker value={APPLICATION_ICON_NAMES[0]} onChange={changeIcon} />)
    await interaction.click(screen.getByRole('button', { name: i18next.t('apps.iconPickerAria') }))
    const nextIcon = APPLICATION_ICON_NAMES[1]!
    const iconOption = screen.getByRole('button', { name: i18next.t(`apps.icons.${nextIcon}`) })
    expect(iconOption).toHaveAttribute('data-slot', 'button')
    expect(iconOption).toHaveClass('size-8')
    await interaction.click(iconOption)
    expect(changeIcon).toHaveBeenCalledWith(nextIcon)
  })
})
