import type { InboxFilter, InboxMessage } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Bell, CheckCheck, ChevronRight } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { api } from '@/api'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'
import { InboxMessageRow } from './inbox-message-row'
import { inboxKeys, invalidateInbox } from './inbox-query'

const filters: InboxFilter[] = ['all', 'unread', 'action']

export function InboxPanel({ inSheet = false, onClose }: { inSheet?: boolean, onClose: () => void }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [filter, setFilter] = useState<InboxFilter>('all')
  const unread = useQuery({ queryKey: inboxKeys.unreadCount, queryFn: api.getInboxUnreadCount })
  const unreadCount = unread.data?.unreadCount ?? 0
  const messages = useQuery({
    queryKey: inboxKeys.list(filter, 1, 10),
    queryFn: () => api.listInboxMessages({ page: 1, pageSize: 10, sortBy: 'createdAt', sortOrder: 'desc', filter }),
  })
  const markAllRead = useMutation({
    mutationFn: api.markAllInboxMessagesRead,
    onSuccess: () => invalidateInbox(queryClient),
  })
  const selectMessage = (message: InboxMessage) => {
    if (!message.readAt) {
      void api.markInboxMessageRead(message.id)
        .then(() => invalidateInbox(queryClient))
        .catch(() => undefined)
    }
    onClose()
    if (message.actionRequest?.status === 'pending' || message.titleKey.startsWith('inbox.runtime.')) {
      navigate(`/inbox?message=${encodeURIComponent(message.id)}`)
      return
    }
    navigate(message.deepLink || `/inbox?message=${encodeURIComponent(message.id)}`)
  }
  const viewAll = () => {
    onClose()
    navigate('/inbox')
  }
  const items = messages.data?.items ?? []

  return (
    <div className="flex min-h-0 w-full min-w-0 flex-1 flex-col overflow-hidden">
      <div className={cn('flex min-w-0 shrink-0 items-center justify-between gap-2 px-4 pt-3.5 pb-1.5', inSheet && 'pr-14')}>
        <div className="flex min-w-0 items-center gap-2">
          <h2 className="truncate text-sm font-semibold">{t('inbox.title')}</h2>
          {unreadCount > 0 && (
            <Badge className="h-5 px-1.5 text-[11px] tabular-nums" variant="secondary">
              {unreadCount > 99 ? '99+' : unreadCount}
            </Badge>
          )}
        </div>
        <Button
          className="h-7 shrink-0 gap-1.5 px-2 text-xs text-muted-foreground hover:text-foreground"
          disabled={unreadCount === 0 || markAllRead.isPending}
          size="sm"
          variant="ghost"
          onClick={() => markAllRead.mutate()}
        >
          <CheckCheck aria-hidden="true" className="size-3.5" />
          <span>{t('inbox.actions.markAllRead')}</span>
        </Button>
      </div>
      <Tabs className="min-h-0 w-full min-w-0 flex-1 gap-0" value={filter} onValueChange={value => setFilter(value as InboxFilter)}>
        <div className="shrink-0 border-b border-border px-4">
          <TabsList className="h-9 w-full justify-start gap-1 border-0">
            {filters.map(value => (
              <TabsTrigger key={value} className="h-9 flex-none px-2.5 text-xs" value={value}>
                {t(`inbox.tabs.${value}`)}
              </TabsTrigger>
            ))}
          </TabsList>
        </div>
        <TabsContent
          className="mt-0 min-h-0 w-full min-w-0 flex-1 overflow-x-hidden overflow-y-auto overscroll-contain p-2 outline-none"
          value={filter}
        >
          {messages.isLoading
            ? (
                <div className="flex flex-col gap-1">
                  {[0, 1, 2].map(index => (
                    <div className="flex items-start gap-3 p-2.5" key={index}>
                      <Skeleton className="mt-0.5 size-8 shrink-0 rounded-control" />
                      <div className="min-w-0 flex-1 space-y-2">
                        <Skeleton className="h-4 w-2/5" />
                        <Skeleton className="h-3 w-full" />
                        <Skeleton className="h-3 w-1/3" />
                      </div>
                    </div>
                  ))}
                </div>
              )
            : messages.isError
              ? (
                  <div className="p-3">
                    <ErrorState
                      description={t('inbox.loadFailedDescription')}
                      retrying={messages.isFetching}
                      title={t('inbox.loadFailedTitle')}
                      onRetry={() => void messages.refetch()}
                    />
                  </div>
                )
              : items.length === 0
                ? (
                    <div className="py-6">
                      <EmptyState description={t('inbox.empty.description')} icon={<Bell className="size-5" />} title={t(`inbox.empty.${filter}`)} variant="plain" />
                    </div>
                  )
                : (
                    <div className="flex min-w-0 flex-col gap-1">
                      {items.map(message => <InboxMessageRow key={message.id} compact message={message} onSelect={selectMessage} />)}
                    </div>
                  )}
        </TabsContent>
      </Tabs>
      <div className="shrink-0 border-t border-border bg-surface-subtle/30 p-2">
        <Button className="h-8 w-full justify-between px-3 text-xs font-medium text-muted-foreground hover:text-foreground" size="sm" variant="ghost" onClick={viewAll}>
          <span>{t('inbox.actions.viewAll')}</span>
          <ChevronRight aria-hidden="true" className="size-4 shrink-0" />
        </Button>
      </div>
    </div>
  )
}
