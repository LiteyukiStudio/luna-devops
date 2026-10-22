import type { ReactNode } from 'react'
import i18next from 'i18next'
import { CircleHelp } from 'lucide-react'
import { Slot } from 'radix-ui'
import { isValidElement, useId } from 'react'

import { Button } from '@/components/ui/button'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

/** FormField 分配给内部控件的关联属性；复合控件可用 render 函数把它们放到真正的输入元素上。 */
export interface FormFieldControlProps {
  'id': string
  'aria-describedby'?: string
  'aria-invalid'?: true
}

interface ControlElementProps {
  'id'?: string
  'aria-describedby'?: string
  'aria-invalid'?: boolean | 'true' | 'false'
}

/**
 * 表单字段的统一外壳。
 * 用于包装 input/select/textarea 等控件，集中处理 label 关联、必填标记、说明和字段错误；不要在业务页重复手写这些结构。
 * 单个子元素通过 Slot 获得 id、aria-describedby 与 aria-invalid（子元素自身的同名属性优先）；复合控件传入 render 函数自行分配。
 * 说明文字同时作为隐藏描述关联到控件，问号按钮保持不进入 Tab 序列，避免长表单出现大量额外焦点。
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
  children: ReactNode | ((control: FormFieldControlProps, field: { labelId: string }) => ReactNode)
}) {
  const baseId = useId()
  const labelId = `${baseId}-label`
  const hintId = `${baseId}-hint`
  const errorId = `${baseId}-error`
  const describedBy = [hint ? hintId : '', error ? errorId : ''].filter(Boolean).join(' ') || undefined
  const control: FormFieldControlProps = {
    'id': `${baseId}-control`,
    'aria-describedby': describedBy,
    'aria-invalid': error ? true : undefined,
  }
  const content = typeof children === 'function'
    ? children(control, { labelId })
    : isValidElement<ControlElementProps>(children)
      ? <Slot.Root {...control}>{children}</Slot.Root>
      : children
  const labelTarget = typeof children === 'function' || isValidElement<ControlElementProps>(children)
    ? (isValidElement<ControlElementProps>(children) && children.props.id) || control.id
    : undefined

  return (
    <Field aria-labelledby={labelTarget ? undefined : labelId} className={cn('group min-w-0 gap-1.5', className)} data-invalid={Boolean(error)}>
      <div className="flex min-w-0 items-center gap-1.5">
        <FieldLabel className="min-w-0" htmlFor={labelTarget} id={labelId}>
          <span className="truncate">
            {label}
            {required && <span aria-hidden="true" className="ml-1 text-primary-text">*</span>}
          </span>
        </FieldLabel>
        {hint && (
          <>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  aria-label={`${label}${i18next.t('common.helpSuffix')}`}
                  className="size-auto shrink-0 rounded-sm p-0 text-muted-foreground hover:bg-transparent hover:text-primary-text focus:text-primary-text focus-visible:ring-0 [&_svg]:size-3.5"
                  size="icon"
                  tabIndex={-1}
                  type="button"
                  variant="ghost"
                >
                  <CircleHelp className="size-3.5 transition" />
                </Button>
              </TooltipTrigger>
              <TooltipContent className="max-w-72 leading-5" side="top">
                {hint}
              </TooltipContent>
            </Tooltip>
            <span className="sr-only" id={hintId}>{hint}</span>
          </>
        )}
      </div>
      {content}
      {error && (
        <FieldError className="text-xs" id={errorId}>
          {error}
        </FieldError>
      )}
    </Field>
  )
}
