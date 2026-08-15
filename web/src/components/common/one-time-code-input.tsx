import type { ComponentProps } from 'react'
import { REGEXP_ONLY_DIGITS } from 'input-otp'
import { InputOTP, InputOTPGroup, InputOTPSlot } from '@/components/ui/input-otp'
import { cn } from '@/lib/utils'

type OneTimeCodeInputProps = Omit<ComponentProps<typeof InputOTP>, 'autoComplete' | 'children' | 'inputMode' | 'maxLength' | 'pattern' | 'render' | 'type'> & {
  invalid?: boolean
}

/** Six-digit OTP input shared by email verification and TOTP flows. */
export function OneTimeCodeInput({ className, containerClassName, invalid = false, name = 'one-time-code', ...props }: OneTimeCodeInputProps) {
  return (
    <div className="max-w-full" data-slot="one-time-code-input">
      <InputOTP
        {...props}
        aria-invalid={invalid}
        autoComplete="one-time-code"
        className={cn('font-mono tabular-nums', className)}
        containerClassName={cn('w-fit max-w-full', containerClassName)}
        enterKeyHint="done"
        inputMode="numeric"
        maxLength={6}
        name={name}
        pattern={REGEXP_ONLY_DIGITS}
        pushPasswordManagerStrategy="increase-width"
        type="text"
      >
        <InputOTPGroup>
          {[0, 1, 2, 3, 4, 5].map(index => (
            <InputOTPSlot key={index} aria-invalid={invalid} className="size-8 sm:size-9" index={index} />
          ))}
        </InputOTPGroup>
      </InputOTP>
    </div>
  )
}
