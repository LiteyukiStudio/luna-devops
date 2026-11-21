import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { beforeEach, describe, expect, it } from 'vitest'
import i18next from '@/i18n'
import { normalizeUnitInputAmount } from '@/lib/resource-units'
import { CpuQuotaInput, MemoryQuotaInput, StorageQuotaInput } from './unit-input'

function ControlledCpuField({ initial = '1', invalid = false }: { initial?: string, invalid?: boolean }) {
  const [value, setValue] = useState(initial)
  return (
    <div>
      <CpuQuotaInput
        inputProps={{
          'aria-describedby': invalid ? 'cpu-error' : undefined,
          'aria-invalid': invalid ? true : undefined,
          'aria-label': 'CPU quota',
        }}
        value={value}
        onChange={setValue}
      />
      <output data-testid="raw-value">{value}</output>
      {invalid && <p id="cpu-error">Invalid CPU quota</p>}
    </div>
  )
}

function ControlledMemoryAndStorageFields() {
  const [memory, setMemory] = useState('1Gi')
  const [storage, setStorage] = useState('10Gi')
  return (
    <div>
      <MemoryQuotaInput inputProps={{ 'aria-label': 'Memory quota' }} value={memory} onChange={setMemory} />
      <output data-testid="memory-value">{memory}</output>
      <StorageQuotaInput inputProps={{ 'aria-label': 'Storage capacity' }} value={storage} onChange={setStorage} />
      <output data-testid="storage-value">{storage}</output>
    </div>
  )
}

describe('unified quota inputs', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('en-US')
  })

  it('normalizes decimal amounts safely while stripping invalid characters and extra dots', () => {
    expect(normalizeUnitInputAmount('0.5')).toBe('0.5')
    expect(normalizeUnitInputAmount('.5')).toBe('0.5')
    expect(normalizeUnitInputAmount('1..5.2abc')).toBe('1.52')
    expect(normalizeUnitInputAmount('1.')).toBe('1.')
    expect(normalizeUnitInputAmount('abc')).toBe('')
  })

  it('allows typing decimals step-by-step and trims trailing dot on blur', async () => {
    const user = userEvent.setup()
    render(<ControlledCpuField initial="1" />)

    const input = screen.getByRole('textbox', { name: 'CPU quota' })
    expect(input).toHaveAttribute('inputmode', 'decimal')
    expect(input).toHaveValue('1')

    await user.clear(input)
    await user.type(input, '0.5')
    expect(input).toHaveValue('0.5')
    expect(screen.getByTestId('raw-value').textContent).toBe('0.5')

    await user.clear(input)
    await user.type(input, '2.')
    expect(input).toHaveValue('2.')
    await user.tab()
    expect(screen.getByTestId('raw-value').textContent).toBe('2')
  })

  it('highlights the outer wrapper with border-danger when aria-invalid is true', () => {
    render(<ControlledCpuField initial="" invalid />)

    const input = screen.getByRole('textbox', { name: 'CPU quota' })
    expect(input).toHaveAttribute('aria-invalid', 'true')
    expect(input).toHaveAccessibleDescription('Invalid CPU quota')
    expect(input.parentElement?.className).toContain('border-danger')
  })

  it('supports decimal memory and storage quota inputs and unit switching', async () => {
    const user = userEvent.setup()
    render(<ControlledMemoryAndStorageFields />)

    const memoryInput = screen.getByRole('textbox', { name: 'Memory quota' })
    await user.clear(memoryInput)
    await user.type(memoryInput, '1.5')
    expect(screen.getByTestId('memory-value')).toHaveTextContent('1.5Gi')

    await user.click(screen.getByRole('combobox', { name: 'Memory unit' }))
    await user.click(await screen.findByRole('option', { name: 'Mi' }))
    expect(screen.getByTestId('memory-value')).toHaveTextContent('1.5Mi')

    const storageInput = screen.getByRole('textbox', { name: 'Storage capacity' })
    await user.clear(storageInput)
    await user.type(storageInput, '2.5')
    await user.click(screen.getByRole('combobox', { name: 'Storage unit' }))
    await user.click(await screen.findByRole('option', { name: 'Ti' }))
    expect(screen.getByTestId('storage-value')).toHaveTextContent('2.5Ti')
  })
})
