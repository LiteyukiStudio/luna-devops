import type { InboxFilter, InboxMessage } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Bell, CheckCheck, ChevronRight } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { api } from '@/api'
import { EmptyState } from '@/components/common/empty-state'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { InboxMessageRow } from './inbox-message-row'
import { inboxKeys, invalidateInbox } from './inbox-query'

const filters: InboxFilter[] = ['all', 'unread', 'action']

export function InboxPanel({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [filter, setFilter] = useState<InboxFilter>('all')
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
    if (message.actionRequest?.status === 'pending') {
      navigate(`/inbox?message=${encodeURIComponent(message.id)}`)
      return
    }
    navigate(message.deepLink || `/inbox?message=${encodeURIComponent(message.id)}`)
  }
  const viewAll = () => {
    onClose()
    navigate('/inbox')
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex items-center justify-between gap-3 px-4 pb-2 pt-4">
        <h2 className="font-semibold">{t('inbox.title')}</h2>
        <Button aria-label={t('inbox.actions.markAllRead')} disabled={markAllRead.isPending} size="icon" variant="ghost" onClick={() => markAllRead.mutate()}>
          <CheckCheck className="size-4" />
        </Button>
      </div>
      <Tabs className="min-h-0 flex-1 gap-0" value={filter} onValueChange={value => setFilter(value as InboxFilter)}>
        <TabsList className="mx-4 w-auto shrink-0">
          {filters.map(value => <TabsTrigger key={value} className="flex-1" value={value}>{t(`inbox.tabs.${value}`)}</TabsTrigger>)}
        </TabsList>
        <div className="min-h-0 flex-1 overflow-y-auto p-2">
          {!messages.isLoading && (messages.data?.items.length ?? 0) === 0
            ? <EmptyState description={t('inbox.empty.description')} icon={<Bell className="size-5" />} title={t(`inbox.empty.${filter}`)} variant="plain" />
            : messages.data?.items.map(message => <InboxMessageRow key={message.id} compact message={message} onSelect={selectMessage} />)}
        </div>
      </Tabs>
      <div className="shrink-0 p-3">
        <Button className="w-full justify-between" variant="ghost" onClick={viewAll}>
          {t('inbox.actions.viewAll')}
          <ChevronRight className="size-4" />
        </Button>
      </div>
    </div>
  )
}
