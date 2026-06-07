import type { ReactNode } from 'react'
import i18next from 'i18next'
import { CircleHelp } from 'lucide-react'

import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

/**
 * 表单字段的统一外壳。
 * 用于包装 input/select/textarea 等控件，集中处理 label、必填标记、说明 tooltip 和字段错误；不要在业务页重复手写这些结构。
 */
export function FormField({
  label,
  required,
  error,
  hint,
  children,
}: {
  label: string
  required?: boolean
  error?: string
  hint?: string
  children: ReactNode
}) {
  return (
    <Field className="group gap-1.5" data-invalid={Boolean(error)}>
      <div className="flex min-w-0 items-center justify-between gap-3">
        <FieldLabel className="min-w-0 gap-1.5">
          <span className="truncate">
            {label}
            {required && <span className="ml-1 text-primary">*</span>}
          </span>
          {hint && (
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  aria-label={`${label}${i18next.t('common.helpSuffix')}`}
                  className="inline-flex shrink-0 text-muted-foreground outline-none hover:text-primary focus:text-primary"
                  type="button"
                >
                  <CircleHelp className="size-3.5 transition" />
                </button>
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
