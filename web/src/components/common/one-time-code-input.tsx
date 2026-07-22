import type { ComponentProps } from 'react'
import { REGEXP_ONLY_DIGITS } from 'input-otp'
import { InputOTP, InputOTPGroup, InputOTPSeparator, InputOTPSlot } from '@/components/ui/input-otp'
import { cn } from '@/lib/utils'

type OneTimeCodeInputProps = Omit<ComponentProps<typeof InputOTP>, 'autoComplete' | 'children' | 'inputMode' | 'maxLength' | 'pattern' | 'render' | 'type'> & {
  invalid?: boolean
}

/** Six-digit OTP input shared by email verification and TOTP flows. */
export function OneTimeCodeInput({ className, containerClassName, invalid = false, ...props }: OneTimeCodeInputProps) {
  return (
    <InputOTP
      {...props}
      aria-invalid={invalid}
      autoComplete="one-time-code"
      className={cn('font-mono tabular-nums', className)}
      containerClassName={cn('max-w-full', containerClassName)}
      enterKeyHint="done"
      inputMode="numeric"
      maxLength={6}
      pattern={REGEXP_ONLY_DIGITS}
      pushPasswordManagerStrategy="increase-width"
      type="text"
    >
      <InputOTPGroup>
        {[0, 1, 2].map(index => <InputOTPSlot key={index} aria-invalid={invalid} index={index} />)}
      </InputOTPGroup>
      <InputOTPSeparator className="text-muted-foreground [&_svg]:size-4" />
      <InputOTPGroup>
        {[3, 4, 5].map(index => <InputOTPSlot key={index} aria-invalid={invalid} index={index} />)}
      </InputOTPGroup>
    </InputOTP>
  )
}
