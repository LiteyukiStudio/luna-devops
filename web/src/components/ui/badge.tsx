import type { ComponentProps } from 'react'
import { cva } from 'class-variance-authority'

import { cn } from '@/lib/utils'

const badgeVariants = cva(
  'inline-flex w-fit shrink-0 items-center justify-center rounded-full border px-2.5 py-0.5 text-xs font-medium whitespace-nowrap transition-colors overflow-hidden',
  {
    variants: {
      variant: {
        default: 'border-transparent bg-primary text-primary-foreground',
        destructive: 'border-transparent bg-danger text-white',
        outline: 'border-border text-foreground',
        secondary: 'border-border bg-muted text-muted-foreground',
      },
    },
    defaultVariants: {
      variant: 'secondary',
    },
  },
)

function Badge({
  className,
  variant,
  ...props
}: ComponentProps<'span'> & {
  variant?: 'default' | 'destructive' | 'outline' | 'secondary'
}) {
  return <span className={cn(badgeVariants({ className, variant }))} {...props} />
}

export { Badge }
