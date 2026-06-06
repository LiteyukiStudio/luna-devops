import type { InputHTMLAttributes, ReactNode, SelectHTMLAttributes, TextareaHTMLAttributes } from 'react'
import { cn } from '../../lib/utils'

export function Input({ className, ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={cn(
        'h-9 w-full rounded-md border border-border bg-background px-3 text-sm outline-none transition focus:border-primary aria-[invalid=true]:border-primary/70 aria-[invalid=true]:bg-primary/5',
        className,
      )}
      {...props}
    />
  )
}

export function Textarea({ className, ...props }: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return (
    <textarea
      className={cn(
        'min-h-20 w-full rounded-md border border-border bg-background px-3 py-2 text-sm outline-none transition focus:border-primary aria-[invalid=true]:border-primary/70 aria-[invalid=true]:bg-primary/5',
        className,
      )}
      {...props}
    />
  )
}

export function Select({ className, ...props }: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select
      className={cn(
        'h-9 w-full rounded-md border border-border bg-background px-3 text-sm outline-none transition focus:border-primary aria-[invalid=true]:border-primary/70 aria-[invalid=true]:bg-primary/5',
        className,
      )}
      {...props}
    />
  )
}

export function Field({
  label,
  required,
  error,
  children,
}: {
  label: string
  required?: boolean
  error?: string
  children: ReactNode
}) {
  return (
    <label className="group grid gap-1.5 text-sm font-medium text-foreground">
      <span className="flex min-w-0 items-center justify-between gap-3">
        <span className="truncate">
          {label}
          {required && <span className="ml-1 text-primary">*</span>}
        </span>
        {error && (
          <span className="max-w-[55%] truncate text-xs font-normal text-primary opacity-0 transition group-hover:opacity-100 group-focus-within:opacity-100">
            {error}
          </span>
        )}
      </span>
      {children}
    </label>
  )
}
