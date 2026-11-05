import type { ReactNode } from 'react'
import type { TerminalConnectionState } from './use-terminal-connection'
import type { StatusTone } from '@/components/common/status-tone'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { StatusDot } from './status-dot'

/** 日志查看器与 Web 终端共用的工具栏：左侧对象信息，右侧操作。 */
export function ViewerToolbar({ actions, children, className }: {
  actions?: ReactNode
  children?: ReactNode
  className?: string
}) {
  return (
    <div className={cn('flex min-h-12 shrink-0 flex-wrap items-center gap-x-3 gap-y-2 border-b border-border bg-surface px-4 py-2', className)}>
      <div className="flex min-w-0 flex-1 flex-wrap items-center gap-x-3 gap-y-2">{children}</div>
      {actions && <div className="flex shrink-0 items-center gap-1">{actions}</div>}
    </div>
  )
}

/** 工具栏图标按钮：本地化 aria-label 同时作为 tooltip；pressed 用于开关类操作。 */
export function ViewerToolbarButton({ disabled, icon, label, pressed, onClick }: {
  disabled?: boolean
  icon: ReactNode
  label: string
  pressed?: boolean
  onClick: () => void
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          aria-label={label}
          aria-pressed={pressed}
          className={cn('size-8 text-muted-foreground', pressed && 'bg-muted text-foreground')}
          disabled={disabled}
          size="icon"
          variant="ghost"
          onClick={onClick}
        >
          {icon}
        </Button>
      </TooltipTrigger>
      <TooltipContent side="bottom">{label}</TooltipContent>
    </Tooltip>
  )
}

/** 深色日志/终端画布中的加载、空、不可用与断开占位。 */
export function TerminalPlaceholder({ action, className, icon, label, tone = 'muted' }: {
  action?: ReactNode
  className?: string
  icon: ReactNode
  label: ReactNode
  tone?: 'danger' | 'muted'
}) {
  return (
    <div
      className={cn('grid h-full min-h-48 place-items-center bg-terminal p-6 text-center', className)}
      role={tone === 'danger' ? 'alert' : 'status'}
    >
      <div className="flex max-w-sm flex-col items-center gap-3 text-sm">
        <span aria-hidden="true" className={cn('grid size-10 place-items-center rounded-full border border-terminal-border [&_svg]:size-4', tone === 'danger' ? 'text-terminal-error' : 'text-terminal-muted')}>
          {icon}
        </span>
        <p className={tone === 'danger' ? 'text-terminal-foreground' : 'text-terminal-muted'}>{label}</p>
        {action}
      </div>
    </div>
  )
}

/** 深色画布上的次要操作按钮（重试、重连）。 */
export function TerminalPlaceholderButton({ children, icon, onClick }: {
  children: ReactNode
  icon: ReactNode
  onClick: () => void
}) {
  return (
    <Button
      className="border-terminal-border bg-transparent text-terminal-foreground shadow-none hover:bg-terminal-foreground/10 hover:text-terminal-foreground"
      size="sm"
      variant="outline"
      onClick={onClick}
    >
      {icon}
      {children}
    </Button>
  )
}

const connectionTones: Record<TerminalConnectionState, StatusTone> = {
  connected: 'success',
  connecting: 'info',
  disconnected: 'neutral',
  failed: 'danger',
  idle: 'neutral',
}

export function TerminalConnectionStatus({ state }: { state: TerminalConnectionState }) {
  const { t } = useTranslation()
  const labels: Record<TerminalConnectionState, string> = {
    connected: t('common.terminal.status.connected'),
    connecting: t('common.terminal.status.connecting'),
    disconnected: t('common.terminal.status.disconnected'),
    failed: t('common.terminal.status.failed'),
    idle: t('common.terminal.status.idle'),
  }
  return (
    <span aria-live="polite" className="inline-flex">
      <StatusDot className="text-xs" label={labels[state]} tone={connectionTones[state]} />
    </span>
  )
}
