import type { ComponentProps } from 'react'

import { ChevronDown } from 'lucide-react'

import { cn } from '@/lib/utils'

interface NativeSelectProps extends ComponentProps<'select'> {
  containerClassName?: string
}

function NativeSelect({ className, containerClassName, ...props }: NativeSelectProps) {
  return (
    <div className={cn('relative min-w-0', containerClassName ?? 'w-full')}>
      <select
        data-slot="native-select"
        className={cn(
          'h-9 w-full min-w-0 appearance-none rounded-control border border-input bg-surface py-1 pr-9 pl-3 text-base shadow-xs outline-none transition-[color,box-shadow] disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 md:text-sm dark:bg-input/30 dark:aria-invalid:ring-destructive/40',
          className,
        )}
        {...props}
      />
      <ChevronDown className="pointer-events-none absolute top-1/2 right-3 size-4 -translate-y-1/2 text-muted-foreground" />
    </div>
  )
}

export { NativeSelect }
