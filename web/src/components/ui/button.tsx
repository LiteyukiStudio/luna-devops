import type { ComponentProps } from 'react'
import { Slot } from 'radix-ui'

import { cn } from '@/lib/utils'
import { buttonVariants } from './button-variants'

function Button({
  asChild = false,
  className,
  size,
  type = 'button',
  variant,
  ...props
}: ComponentProps<'button'> & {
  asChild?: boolean
  size?: 'default' | 'icon' | 'lg' | 'sm'
  variant?: 'default' | 'destructive' | 'ghost' | 'link' | 'outline' | 'secondary'
}) {
  const Component = asChild ? Slot.Root : 'button'
  return (
    <Component
      className={cn(buttonVariants({ className, size, variant }))}
      data-size={size ?? 'default'}
      data-slot="button"
      type={asChild ? undefined : type}
      {...props}
    />
  )
}

export { Button }
