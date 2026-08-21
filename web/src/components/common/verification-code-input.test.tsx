import { fireEvent, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { VerificationCodeInput } from './verification-code-input'

function ControlledCodeInput({ onValueChange }: { onValueChange?: (value: string) => void }) {
  const [value, setValue] = useState('')
  return (
    <VerificationCodeInput
      aria-label="Verification code"
      value={value}
      onValueChange={(nextValue) => {
        setValue(nextValue)
        onValueChange?.(nextValue)
      }}
    />
  )
}

describe('verification code input', () => {
  it('exposes numeric mobile verification-code semantics', () => {
    render(<ControlledCodeInput />)

    const input = screen.getByRole('textbox', { name: 'Verification code' })
    expect(input).toHaveAttribute('autocomplete', 'one-time-code')
    expect(input).toHaveAttribute('inputmode', 'numeric')
    expect(input).toHaveAttribute('maxlength', '6')
    expect(input).toHaveAttribute('name', 'verification-code')
  })

  it('normalizes input to six digits', () => {
    const onValueChange = vi.fn()
    render(<ControlledCodeInput onValueChange={onValueChange} />)

    fireEvent.change(screen.getByRole('textbox', { name: 'Verification code' }), { target: { value: '12a34567' } })

    expect(screen.getByRole('textbox', { name: 'Verification code' })).toHaveValue('123456')
    expect(onValueChange).toHaveBeenCalledWith('123456')
  })
})
