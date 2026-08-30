import type { ComponentPropsWithRef, ReactNode } from 'react'
import { useId } from 'react'
import { Checkbox } from '@/components/ui/checkbox'
import { cn } from '@/lib/utils'

type CheckboxFieldProps = Omit<ComponentPropsWithRef<typeof Checkbox>, 'children' | 'className'> & {
  children: ReactNode
  checkboxClassName?: string
  className?: string
  description?: ReactNode
}

interface BooleanCheckboxControllerField {
  name: string
  value: boolean | undefined
  onBlur: () => void
  onChange: (value: boolean) => void
  ref: (instance: unknown) => void
}

type ControlledCheckboxFieldProps = Omit<CheckboxFieldProps, 'checked' | 'defaultChecked' | 'name' | 'onBlur' | 'onCheckedChange' | 'ref'> & {
  field: BooleanCheckboxControllerField
}

export function CheckboxField({
  'aria-describedby': ariaDescribedBy,
  children,
  checkboxClassName,
  className,
  description,
  id,
  ref,
  ...props
}: CheckboxFieldProps) {
  const generatedId = useId()
  const checkboxId = id ?? generatedId
  const descriptionId = description ? `${checkboxId}-description` : undefined
  const describedBy = [ariaDescribedBy, descriptionId].filter(Boolean).join(' ') || undefined

  return (
    <label className={cn('flex items-start gap-3 text-sm text-foreground', className)} htmlFor={checkboxId}>
      <Checkbox
        ref={ref}
        aria-describedby={describedBy}
        className={cn('mt-0.5', checkboxClassName)}
        id={checkboxId}
        {...props}
      />
      <span className="min-w-0">
        <span className="block font-medium leading-5">{children}</span>
        {description && (
          <span id={descriptionId} className="mt-1 block text-xs leading-5 text-muted-foreground">
            {description}
          </span>
        )}
      </span>
    </label>
  )
}

export function ControlledCheckboxField({ field, ...props }: ControlledCheckboxFieldProps) {
  return (
    <CheckboxField
      {...props}
      checked={field.value === true}
      name={field.name}
      ref={field.ref}
      onBlur={field.onBlur}
      onCheckedChange={checked => field.onChange(checked === true)}
    />
  )
}
