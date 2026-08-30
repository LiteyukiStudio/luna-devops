import type { ReactNode } from 'react'
import i18next from 'i18next'
import { CircleHelp } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

/**
 * 表单字段的统一外壳。
 * 用于包装 input/select/textarea 等控件，集中处理 label、必填标记、说明 tooltip 和字段错误；不要在业务页重复手写这些结构。
 */
export function FormField({
  className,
  label,
  required,
  error,
  hint,
  children,
}: {
  className?: string
  label: string
  required?: boolean
  error?: string
  hint?: string
  children: ReactNode
}) {
  return (
    <Field className={cn('group min-w-0 gap-1.5', className)} data-invalid={Boolean(error)}>
      <div className="flex min-w-0 items-center justify-between gap-3">
        <FieldLabel className="min-w-0 gap-1.5">
          <span className="truncate">
            {label}
            {required && <span className="ml-1 text-primary-text">*</span>}
          </span>
          {hint && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  aria-label={`${label}${i18next.t('common.helpSuffix')}`}
                  className="size-auto shrink-0 p-0 text-muted-foreground hover:bg-transparent hover:text-primary-text focus:text-primary-text focus-visible:ring-0 [&_svg]:size-3.5"
                  size="icon"
                  tabIndex={-1}
                  type="button"
                  variant="ghost"
                >
                  <CircleHelp className="size-3.5 transition" />
                </Button>
              </TooltipTrigger>
              <TooltipContent className="max-w-64 leading-5" side="top">
                {hint}
              </TooltipContent>
            </Tooltip>
          )}
        </FieldLabel>
        {error && (
          <FieldError className="max-w-[55%] truncate text-xs opacity-0 transition group-hover:opacity-100 group-focus-within:opacity-100">
            {error}
          </FieldError>
        )}
      </div>
      {children}
    </Field>
  )
}
