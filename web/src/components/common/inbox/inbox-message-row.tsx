import type { InboxMessage } from '@/api'
import { Bell, Boxes, CreditCard, Megaphone, Rocket, ShieldAlert, UserRoundCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { StatusBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { cn } from '@/lib/utils'
import { inboxMessageText } from './message-format'

export function InboxMessageRow({ compact = false, message, onSelect }: { compact?: boolean, message: InboxMessage, onSelect: (message: InboxMessage) => void }) {
  const { t } = useTranslation()
  const text = inboxMessageText(message, t)
  const pending = message.actionRequest?.status === 'pending'

  return (
    <button
      className={cn(
        'group flex w-full min-w-0 items-start gap-3 rounded-control p-3 text-left outline-none transition-colors hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring',
        !message.readAt && 'bg-primary-subtle',
      )}
      type="button"
      onClick={() => onSelect(message)}
    >
      <span className={cn('mt-0.5 grid size-9 shrink-0 place-items-center rounded-lg bg-muted text-muted-foreground', !message.readAt && 'bg-primary/10 text-primary-text')}>
        {categoryIcon(message.category)}
      </span>
      <span className="min-w-0 flex-1">
        <span className="flex min-w-0 items-start justify-between gap-2">
          <span className={cn('min-w-0 truncate text-sm', !message.readAt ? 'font-semibold' : 'font-medium')}>{text.title}</span>
          {!message.readAt && <span aria-label={t('inbox.states.unread')} className="mt-1.5 size-2 shrink-0 rounded-full bg-primary" />}
        </span>
        <span className={cn('mt-1 block text-xs leading-5 text-muted-foreground', compact ? 'line-clamp-2' : 'line-clamp-3')}>{text.content}</span>
        <span className="mt-2 flex flex-wrap items-center gap-2">
          <span className="text-xs text-muted-foreground">{formatSmartDateTime(message.createdAt, t)}</span>
          {pending && <StatusBadge tone="warning">{t('inbox.states.pending')}</StatusBadge>}
          {message.priority === 'critical' && <StatusBadge tone="danger">{t('inbox.priority.critical')}</StatusBadge>}
          {message.priority === 'high' && !pending && <StatusBadge tone="info">{t('inbox.priority.high')}</StatusBadge>}
        </span>
      </span>
    </button>
  )
}

function categoryIcon(category: InboxMessage['category']) {
  switch (category) {
    case 'action': return <UserRoundCheck className="size-4" />
    case 'project': return <Boxes className="size-4" />
    case 'billing': return <CreditCard className="size-4" />
    case 'security': return <ShieldAlert className="size-4" />
    case 'delivery': return <Rocket className="size-4" />
    case 'system': return <Megaphone className="size-4" />
    default: return <Bell className="size-4" />
  }
}
