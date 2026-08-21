import type { ComponentProps } from 'react'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

type VerificationCodeInputProps = Omit<ComponentProps<typeof Input>, 'inputMode' | 'maxLength' | 'type'> & {
  invalid?: boolean
  onValueChange?: (value: string) => void
}

/** Six-digit numeric input used by email verification flows. */
export function VerificationCodeInput({ className, invalid = false, name = 'verification-code', onChange, onValueChange, ...props }: VerificationCodeInputProps) {
  return (
    <Input
      {...props}
      aria-invalid={invalid}
      autoComplete="one-time-code"
      className={cn('w-40 font-mono tracking-[0.35em] tabular-nums', className)}
      enterKeyHint="done"
      inputMode="numeric"
      maxLength={6}
      name={name}
      pattern="[0-9]*"
      type="text"
      onChange={(event) => {
        const value = event.currentTarget.value.replace(/\D/g, '').slice(0, 6)
        event.currentTarget.value = value
        if (onValueChange)
          onValueChange(value)
        else
          onChange?.(event)
      }}
    />
  )
}
