import type { UseFormRegisterReturn } from 'react-hook-form'
import { Input } from '@/components/ui/input'

interface TargetImageRefInputProps {
  placeholder: string
  prefix: string
  register: UseFormRegisterReturn
}

export function TargetImageRefInput({ placeholder, prefix, register }: TargetImageRefInputProps) {
  if (!prefix)
    return <Input {...register} placeholder={placeholder} />

  return (
    <div className="flex min-w-0 overflow-hidden rounded-md border border-input bg-background shadow-sm focus-within:ring-[3px] focus-within:ring-ring/50">
      <span className="flex shrink-0 items-center border-r border-border bg-muted px-3 text-sm text-muted-foreground">
        {prefix}
      </span>
      <Input
        {...register}
        className="rounded-none border-0 shadow-none focus-visible:ring-0"
        placeholder={placeholder}
      />
    </div>
  )
}
