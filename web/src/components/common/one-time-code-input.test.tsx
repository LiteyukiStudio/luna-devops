import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { OneTimeCodeInput } from './one-time-code-input'

function ControlledCodeInput({ onComplete }: { onComplete?: (value: string) => void }) {
  const [value, setValue] = useState('')
  return (
    <OneTimeCodeInput
      aria-label="Verification code"
      value={value}
      onChange={setValue}
      onComplete={onComplete}
    />
  )
}

describe('one-time code input', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    cleanup()
    act(() => vi.runOnlyPendingTimers())
    vi.useRealTimers()
  })

  it('exposes password-manager and mobile OTP semantics', () => {
    render(<ControlledCodeInput />)

    const input = screen.getByRole('textbox', { name: 'Verification code' })
    const container = input.closest('[data-input-otp-container]')
    const wrapper = input.closest('[data-slot="one-time-code-input"]')
    expect(input).toHaveAttribute('autocomplete', 'one-time-code')
    expect(input).toHaveAttribute('inputmode', 'numeric')
    expect(input).toHaveAttribute('maxlength', '6')
    expect(input).toHaveAttribute('name', 'one-time-code')
    expect(wrapper).toHaveClass('max-w-full')
    expect(container).toHaveClass('w-fit', 'max-w-full')
    expect(container?.querySelectorAll('[data-slot="input-otp-group"]')).toHaveLength(1)
    expect(container?.querySelectorAll('[data-slot="input-otp-slot"]')).toHaveLength(6)
    expect(container?.querySelector('[data-slot="input-otp-separator"]')).not.toBeInTheDocument()
  })

  it('accepts a complete six-digit code as one input value', () => {
    const onComplete = vi.fn()
    render(<ControlledCodeInput onComplete={onComplete} />)

    fireEvent.change(screen.getByRole('textbox', { name: 'Verification code' }), { target: { value: '123456' } })

    expect(screen.getByRole('textbox', { name: 'Verification code' })).toHaveValue('123456')
    expect(onComplete).toHaveBeenCalledWith('123456')
  })
})
