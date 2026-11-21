import type { ComponentProps, ReactNode } from 'react'
import { LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

export type FloatingFormSaveBarTone = 'primary' | 'warning' | 'danger'

const toneDotClasses: Record<FloatingFormSaveBarTone, string> = {
  danger: 'bg-danger-solid',
  primary: 'bg-primary',
  warning: 'bg-warning-solid',
}

interface FloatingFormSaveBarProps {
  actions?: ReactNode
  className?: string
  discardDisabled?: boolean
  discardLabel?: string
  open: boolean
  pending?: boolean
  saveDisabled?: boolean
  saveLabel?: string
  saveType?: 'button' | 'submit'
  slot?: string
  statusContent?: ReactNode
  tone?: FloatingFormSaveBarTone
  onDiscard?: () => void
  onSave?: () => void
}

/**
 * 长页面表单的悬浮胶囊保存提示条。
 * 未修改时隐藏，存在脏改动或需要立即关注的状态时贴合工作区底部居中浮现。
 */
export function FloatingFormSaveBar({
  actions,
  className,
  discardDisabled = false,
  discardLabel,
  open,
  pending = false,
  saveDisabled = false,
  saveLabel,
  saveType = 'submit',
  slot = 'floating-form-save-bar',
  statusContent,
  tone = 'primary',
  onDiscard,
  onSave,
}: FloatingFormSaveBarProps) {
  const { t } = useTranslation()

  if (!open)
    return null

  return (
    <div
      className={cn(
        'pointer-events-none flex justify-center pt-1 @lg/workspace:sticky @lg/workspace:bottom-4 @lg/workspace:z-10',
        className,
      )}
      data-slot={slot}
    >
      <div className="pointer-events-auto flex w-full max-w-2xl flex-col gap-2.5 rounded-3xl border border-primary-border/75 bg-surface/95 px-4 py-2.5 shadow-raised backdrop-blur-md @sm/workspace:flex-row @sm/workspace:items-center @sm/workspace:justify-between @sm/workspace:rounded-full @sm/workspace:py-2 @sm/workspace:pl-5 @sm/workspace:pr-2">
        <div aria-live="polite" className="flex min-w-0 items-center gap-2.5 text-sm font-medium">
          <span aria-hidden="true" className={cn('size-2.5 shrink-0 rounded-full', toneDotClasses[tone])} />
          {statusContent ?? (
            <p className="m-0 truncate text-foreground">
              {t('common.unsavedChanges')}
            </p>
          )}
        </div>
        <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
          {actions ?? (
            <>
              {onDiscard && (
                <FloatingFormSaveBarSecondaryButton
                  disabled={pending || discardDisabled}
                  type="button"
                  onClick={onDiscard}
                >
                  {discardLabel ?? t('common.discardChanges')}
                </FloatingFormSaveBarSecondaryButton>
              )}
              <Button
                aria-busy={pending}
                className="rounded-full px-5"
                disabled={pending || saveDisabled}
                type={saveType}
                onClick={onSave}
              >
                {pending && <LoaderCircle aria-hidden="true" className="size-4 animate-spin" />}
                {saveLabel ?? t('common.save')}
              </Button>
            </>
          )}
        </div>
      </div>
    </div>
  )
}

export function FloatingFormSaveBarSecondaryButton({ className, ...props }: ComponentProps<typeof Button>) {
  return (
    <Button
      className={cn(
        'rounded-full border-transparent bg-surface-inset px-4 text-foreground shadow-none hover:bg-surface-subtle',
        className,
      )}
      variant="outline"
      {...props}
    />
  )
}
