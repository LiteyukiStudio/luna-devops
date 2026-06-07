import type { ComponentProps } from 'react'

import { cn } from '@/lib/utils'
import { buttonVariants } from './button-variants'

function Button({
  className,
  size,
  type = 'button',
  variant,
  ...props
}: ComponentProps<'button'> & {
  size?: 'default' | 'icon' | 'lg' | 'sm'
  variant?: 'default' | 'destructive' | 'ghost' | 'link' | 'outline' | 'secondary'
}) {
  return (
    <button
      className={cn(buttonVariants({ className, size, variant }))}
      type={type}
      {...props}
    />
  )
}

export { Button }
