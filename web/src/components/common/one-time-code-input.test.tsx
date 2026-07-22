import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { OneTimeCodeInput } from './one-time-code-input'

function ControlledCodeInput({ onComplete }: { onComplete?: (value: string) => void }) {
  const [value, setValue] = useState('')
  return (
    <OneTimeCodeInput
      aria-label="Verification code"
      name="one-time-code"
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
    expect(input).toHaveAttribute('autocomplete', 'one-time-code')
    expect(input).toHaveAttribute('inputmode', 'numeric')
    expect(input).toHaveAttribute('maxlength', '6')
    expect(input).toHaveAttribute('name', 'one-time-code')
  })

  it('accepts a complete six-digit code as one input value', () => {
    const onComplete = vi.fn()
    render(<ControlledCodeInput onComplete={onComplete} />)

    fireEvent.change(screen.getByRole('textbox', { name: 'Verification code' }), { target: { value: '123456' } })

    expect(screen.getByRole('textbox', { name: 'Verification code' })).toHaveValue('123456')
    expect(onComplete).toHaveBeenCalledWith('123456')
  })
})
