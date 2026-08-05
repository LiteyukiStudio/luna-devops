import type { ComponentProps } from 'react'
import { LoaderCircle, Save } from 'lucide-react'
import { Button } from '@/components/ui/button'

interface SettingsTabSaveButtonProps extends Omit<ComponentProps<typeof Button>, 'children'> {
  label: string
  pending?: boolean
}

/** 当前设置 Tab 的统一保存入口。 */
export function SettingsTabSaveButton({ disabled, label, pending = false, ...props }: SettingsTabSaveButtonProps) {
  return (
    <Button {...props} aria-busy={pending} data-slot="settings-tab-save" disabled={disabled || pending}>
      {pending ? <LoaderCircle aria-hidden="true" className="size-4 animate-spin" /> : <Save aria-hidden="true" className="size-4" />}
      {label}
    </Button>
  )
}
