import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { UnitInput } from './unit-input'

const units = [
  { label: 'millicores', value: 'm' },
  { label: 'cores', suffix: '', value: 'core' },
]

describe('unit input', () => {
  it('uses the shared native select while preserving amount and unit formatting', async () => {
    const interaction = userEvent.setup()
    const onChange = vi.fn()
    render(
      <UnitInput
        inputProps={{ 'aria-label': 'CPU amount' }}
        unitSelectLabel="CPU unit"
        units={units}
        value="500m"
        onChange={onChange}
      />,
    )

    const amount = screen.getByRole('textbox', { name: 'CPU amount' })
    const unit = screen.getByRole('combobox', { name: 'CPU unit' })
    expect(unit).toHaveAttribute('data-slot', 'native-select')
    expect(unit).toHaveValue('m')

    fireEvent.change(amount, { target: { value: '750 invalid' } })
    expect(onChange).toHaveBeenLastCalledWith('750m')

    await interaction.selectOptions(unit, 'core')
    expect(onChange).toHaveBeenLastCalledWith('500')
  })

  it('disables both controls without changing the composed field styling', () => {
    render(
      <UnitInput
        disabled
        inputProps={{ 'aria-label': 'Memory amount' }}
        unitSelectLabel="Memory unit"
        units={[{ label: 'MiB', value: 'Mi' }]}
        value="512Mi"
        onChange={vi.fn()}
      />,
    )

    expect(screen.getByRole('textbox', { name: 'Memory amount' })).toBeDisabled()
    expect(screen.getByRole('combobox', { name: 'Memory unit' })).toBeDisabled()
    expect(screen.getByRole('combobox', { name: 'Memory unit' })).toHaveClass('border-0', 'bg-transparent', 'focus-visible:ring-0')
  })
})
