import type { InboxMessage } from '@/api'
import { AlertCircle, AlertTriangle, Bell, Boxes, CheckCircle2, CreditCard, Megaphone, Rocket, ShieldAlert, Sparkles, UserRoundCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { StatusBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { cn } from '@/lib/utils'
import { inboxMessageText, inboxMessageVisualKind, stripInlineMarkdown } from './message-format'

export function InboxMessageAvatar({
  className,
  message,
  showUnreadDot = false,
}: {
  className?: string
  message: InboxMessage
  showUnreadDot?: boolean
}) {
  const visual = inboxMessageVisualKind(message)
  const unread = !message.readAt

  const toneClass
    = visual === 'danger'
      ? 'bg-danger-subtle text-danger'
      : visual === 'warning'
        ? 'bg-warning-subtle text-warning'
        : visual === 'success'
          ? 'bg-success-subtle text-success'
          : unread
            ? 'bg-primary/10 text-primary-text'
            : 'bg-surface-subtle text-muted-foreground'

  return (
    <span className={cn('relative grid size-8 shrink-0 place-items-center rounded-control', toneClass, className)}>
      {visualIcon(visual, message.category)}
      {showUnreadDot && unread && (
        <span aria-hidden="true" className="absolute -right-0.5 -top-0.5 size-2 rounded-full bg-primary ring-2 ring-surface" />
      )}
    </span>
  )
}

export function InboxMessageRow({ compact = false, message, onSelect }: { compact?: boolean, message: InboxMessage, onSelect: (message: InboxMessage) => void }) {
  const { t } = useTranslation()
  const text = inboxMessageText(message, t)
  const pending = message.actionRequest?.status === 'pending'
  const rawAiSummary = typeof message.params?.aiRootCauseSummary === 'string' ? message.params.aiRootCauseSummary : ''
  const aiSummary = rawAiSummary ? stripInlineMarkdown(rawAiSummary) : ''

  return (
    <button
      className={cn(
        'group flex w-full max-w-full min-w-0 items-start gap-3 overflow-hidden rounded-control p-2.5 text-left outline-none transition-colors duration-fast focus-visible:ring-2 focus-visible:ring-ring',
        !message.readAt
          ? 'bg-primary-subtle/55 hover:bg-primary-subtle'
          : 'hover:bg-muted/70',
      )}
      type="button"
      onClick={() => onSelect(message)}
    >
      <InboxMessageAvatar className="mt-0.5" message={message} />
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="flex min-w-0 items-center justify-between gap-2">
          <span className={cn('min-w-0 truncate text-sm leading-5', !message.readAt ? 'font-semibold text-foreground' : 'font-medium text-foreground/90')}>
            {text.title}
          </span>
          {!message.readAt && (
            <>
              <span aria-hidden="true" className="size-2 shrink-0 rounded-full bg-primary" />
              <span className="sr-only">{t('inbox.states.unread')}</span>
            </>
          )}
        </span>
        <span className={cn('mt-0.5 block min-w-0 text-xs leading-5 text-muted-foreground wrap-anywhere', compact ? 'line-clamp-2' : 'line-clamp-3')}>
          {text.content}
        </span>
        {aiSummary && (
          <span className="mt-1.5 flex min-w-0 items-start gap-1.5 rounded-control border border-primary-border/45 bg-background/80 px-2 py-1.5 text-xs leading-[1.125rem] text-foreground">
            <Sparkles aria-hidden="true" className="mt-0.5 size-3.5 shrink-0 text-primary" />
            <span className={cn('min-w-0 flex-1 wrap-anywhere', compact ? 'line-clamp-2' : 'line-clamp-3')}>
              {aiSummary}
            </span>
          </span>
        )}
        <span className="mt-1.5 flex min-w-0 flex-wrap items-center gap-1.5">
          <span className="text-[11px] leading-4 text-muted-foreground">{formatSmartDateTime(message.createdAt, t)}</span>
          {pending && <StatusBadge tone="warning">{t('inbox.states.pending')}</StatusBadge>}
          {message.priority === 'critical' && <StatusBadge tone="danger">{t('inbox.priority.critical')}</StatusBadge>}
          {message.priority === 'high' && !pending && <StatusBadge tone="warning">{t('inbox.priority.high')}</StatusBadge>}
        </span>
      </span>
    </button>
  )
}

function visualIcon(visual: ReturnType<typeof inboxMessageVisualKind>, category: InboxMessage['category']) {
  if (visual === 'danger')
    return <AlertCircle aria-hidden="true" className="size-4" />
  if (visual === 'warning')
    return category === 'action' ? <UserRoundCheck aria-hidden="true" className="size-4" /> : <AlertTriangle aria-hidden="true" className="size-4" />
  if (visual === 'success')
    return <CheckCircle2 aria-hidden="true" className="size-4" />
  switch (category) {
    case 'action': return <UserRoundCheck aria-hidden="true" className="size-4" />
    case 'project': return <Boxes aria-hidden="true" className="size-4" />
    case 'billing': return <CreditCard aria-hidden="true" className="size-4" />
    case 'security': return <ShieldAlert aria-hidden="true" className="size-4" />
    case 'delivery': return <Rocket aria-hidden="true" className="size-4" />
    case 'system': return <Megaphone aria-hidden="true" className="size-4" />
    default: return <Bell aria-hidden="true" className="size-4" />
  }
}
