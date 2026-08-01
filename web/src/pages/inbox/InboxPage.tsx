import type { InboxDecision, InboxFilter, InboxMessage } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Archive, Bell, CheckCheck, Eye } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import { api } from '@/api'
import { DataList } from '@/components/common/data-list'
import { inboxKeys, invalidateInbox } from '@/components/common/inbox/inbox-query'
import { inboxMessageText } from '@/components/common/inbox/message-format'
import { StatusBadge } from '@/components/common/status-badge'
import { formatAbsoluteDateTime, formatSmartDateTime } from '@/components/common/time-format'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

const filters: InboxFilter[] = ['all', 'unread', 'action']

export function InboxPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [searchParams, setSearchParams] = useSearchParams()
  const initialFilter = filters.includes(searchParams.get('filter') as InboxFilter) ? searchParams.get('filter') as InboxFilter : 'all'
  const [filter, setFilter] = useState<InboxFilter>(initialFilter)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const selectedMessageId = searchParams.get('message') ?? ''
  const messages = useQuery({
    queryKey: inboxKeys.list(filter, page, pageSize),
    queryFn: () => api.listInboxMessages({ page, pageSize, sortBy: 'createdAt', sortOrder: 'desc', filter }),
  })
  const selectedMessage = useQuery({
    queryKey: inboxKeys.detail(selectedMessageId),
    queryFn: () => api.getInboxMessage(selectedMessageId),
    enabled: Boolean(selectedMessageId),
  })
  const closeDetails = useCallback(() => setSearchParams((current) => {
    const next = new URLSearchParams(current)
    next.delete('message')
    return next
  }), [setSearchParams])
  const markAllRead = useMutation({ mutationFn: api.markAllInboxMessagesRead, onSuccess: () => invalidateInbox(queryClient) })
  const archive = useMutation({
    mutationFn: api.archiveInboxMessage,
    onSuccess: async () => {
      closeDetails()
      await invalidateInbox(queryClient)
    },
  })
  const decide = useMutation({
    mutationFn: ({ decision, expectedVersion, requestId }: { decision: InboxDecision, expectedVersion: number, requestId: string }) =>
      api.decideInboxActionRequest(requestId, decision, expectedVersion),
    onSuccess: async () => {
      toast.success(t('inbox.decision.success'))
      await invalidateInbox(queryClient)
    },
    onError: error => toast.error(error instanceof Error ? error.message : t('errors.request.failed')),
  })
  const openDetails = useCallback((message: InboxMessage) => {
    setSearchParams((current) => {
      const next = new URLSearchParams(current)
      next.set('message', message.id)
      return next
    })
    if (!message.readAt) {
      void api.markInboxMessageRead(message.id)
        .then(() => invalidateInbox(queryClient))
        .catch(() => undefined)
    }
  }, [queryClient, setSearchParams])
  const columns = useMemo<DataListColumn<InboxMessage>[]>(() => [
    {
      key: 'message',
      header: t('inbox.title'),
      width: 'primary',
      render: (message) => {
        const text = inboxMessageText(message, t)
        return (
          <div className="flex min-w-0 items-start gap-3">
            <span className="relative mt-1 grid size-8 shrink-0 place-items-center rounded-lg bg-muted text-muted-foreground">
              <Bell className="size-4" />
              {!message.readAt && <span className="absolute -right-0.5 -top-0.5 size-2 rounded-full bg-primary" />}
            </span>
            <div className="min-w-0">
              <p className={message.readAt ? 'truncate font-medium' : 'truncate font-semibold'}>{text.title}</p>
              <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">{text.content}</p>
              <div className="mt-2 flex flex-wrap gap-2 md:hidden">
                <StatusBadge>{t(`inbox.categories.${message.category}`)}</StatusBadge>
                <span className="text-xs text-muted-foreground">{formatSmartDateTime(message.createdAt, t)}</span>
              </div>
            </div>
          </div>
        )
      },
    },
    { key: 'category', header: t('common.type'), mobile: 'hidden', width: 'status', render: message => <StatusBadge>{t(`inbox.categories.${message.category}`)}</StatusBadge> },
    {
      key: 'status',
      header: t('common.status'),
      mobile: 'hidden',
      width: 'status',
      render: message => message.actionRequest
        ? <StatusBadge tone={message.actionRequest.status === 'pending' ? 'warning' : 'neutral'}>{t(`inbox.states.${message.actionRequest.status}`)}</StatusBadge>
        : !message.readAt ? <StatusBadge tone="info">{t('inbox.states.unread')}</StatusBadge> : null,
    },
    { key: 'createdAt', header: t('time.createdAt', { defaultValue: t('eventsPage.columns.time') }), mobile: 'hidden', width: 'compact', render: message => <span className="whitespace-nowrap text-sm text-muted-foreground">{formatSmartDateTime(message.createdAt, t)}</span> },
    {
      key: 'actions',
      header: t('common.actions'),
      sticky: 'right',
      width: 'actions',
      render: message => (
        <div className="flex items-center gap-1">
          <Button aria-label={t('inbox.actions.viewDetails')} size="icon" variant="ghost" onClick={() => void openDetails(message)}><Eye className="size-4" /></Button>
          <Button aria-label={t('inbox.actions.archive')} size="icon" variant="ghost" onClick={() => archive.mutate(message.id)}><Archive className="size-4" /></Button>
        </div>
      ),
    },
  ], [archive, openDetails, t])

  const selectFilter = (value: string) => {
    setFilter(value as InboxFilter)
    setPage(1)
  }
  const changePageSize = (value: number) => {
    setPageSize(value)
    setPage(1)
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Tabs value={filter} onValueChange={selectFilter}>
          <TabsList>{filters.map(value => <TabsTrigger key={value} value={value}>{t(`inbox.tabs.${value}`)}</TabsTrigger>)}</TabsList>
        </Tabs>
        <Button disabled={markAllRead.isPending} variant="outline" onClick={() => markAllRead.mutate()}>
          <CheckCheck className="size-4" />
          {t('inbox.actions.markAllRead')}
        </Button>
      </div>
      <DataList
        columns={columns}
        emptyDescription={t('inbox.empty.description')}
        emptyIcon={<Bell className="size-5" />}
        emptyTitle={t(`inbox.empty.${filter}`)}
        items={messages.data?.items ?? []}
        loading={messages.isLoading}
        pagination={messages.data && messages.data.total > 0
          ? {
              page,
              pageSize,
              total: messages.data.total,
              totalPages: messages.data.totalPages,
              pageInfoLabel: t('pagination.pageInfo', { current: page, total: messages.data.totalPages }),
              onPageChange: setPage,
              onPageSizeChange: changePageSize,
            }
          : undefined}
        rowKey={message => message.id}
      />
      <InboxDetail
        message={selectedMessage.data}
        open={Boolean(selectedMessageId)}
        pending={selectedMessage.isLoading}
        decisionPending={decide.isPending}
        onArchive={messageId => archive.mutate(messageId)}
        onClose={closeDetails}
        onDecision={async (message, decision) => {
          const request = message.actionRequest
          if (!request)
            return
          await decide.mutateAsync({ decision, expectedVersion: request.rowVersion, requestId: request.id })
        }}
      />
    </div>
  )
}

function InboxDetail({ decisionPending, message, open, pending, onArchive, onClose, onDecision }: { decisionPending: boolean, message?: InboxMessage, open: boolean, pending: boolean, onArchive: (messageId: string) => void, onClose: () => void, onDecision: (message: InboxMessage, decision: InboxDecision) => Promise<void> }) {
  const { t } = useTranslation()
  const [decision, setDecision] = useState<InboxDecision | null>(null)
  const text = message ? inboxMessageText(message, t) : null
  const actionRequest = message?.actionRequest
  const actionable = actionRequest?.status === 'pending'

  return (
    <>
      <Sheet open={open} onOpenChange={value => !value && onClose()}>
        <SheetContent className="w-[min(92vw,32rem)] max-w-none overflow-y-auto p-0" side="right">
          <SheetHeader className="border-b border-border p-6 pr-14">
            <SheetTitle>{text?.title ?? t('inbox.detail.title')}</SheetTitle>
            <SheetDescription>{message ? formatAbsoluteDateTime(message.createdAt) : pending ? t('common.loading') : ''}</SheetDescription>
          </SheetHeader>
          {message && text && (
            <div className="space-y-6 p-6">
              <p className="text-sm leading-6 text-muted-foreground">{text.content}</p>
              <div className="flex flex-wrap gap-2">
                <StatusBadge>{t(`inbox.categories.${message.category}`)}</StatusBadge>
                {actionRequest && <StatusBadge tone={actionable ? 'warning' : 'neutral'}>{t(`inbox.states.${actionRequest.status}`)}</StatusBadge>}
              </div>
              {actionRequest?.expiresAt && (
                <p className="text-sm text-muted-foreground">
                  {t('inbox.detail.expiresAt')}
                  :
                  {' '}
                  {formatAbsoluteDateTime(actionRequest.expiresAt)}
                </p>
              )}
              <div className="flex flex-wrap justify-end gap-2">
                <Button variant="ghost" onClick={() => onArchive(message.id)}>
                  <Archive className="size-4" />
                  {t('inbox.actions.archive')}
                </Button>
                {message.deepLink && <Button asChild variant="outline"><Link to={message.deepLink}>{t('inbox.actions.openResource')}</Link></Button>}
                {actionable && actionRequest.allowedDecisions.includes('reject') && <Button variant="outline" onClick={() => setDecision('reject')}>{t('inbox.actions.reject')}</Button>}
                {actionable && actionRequest.allowedDecisions.includes('accept') && <Button onClick={() => setDecision('accept')}>{t('inbox.actions.accept')}</Button>}
              </div>
            </div>
          )}
        </SheetContent>
      </Sheet>
      <AlertDialog open={decision !== null} onOpenChange={value => !value && setDecision(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t(`inbox.decision.${decision === 'accept' ? 'acceptTitle' : 'rejectTitle'}`)}</AlertDialogTitle>
            <AlertDialogDescription>{t(`inbox.decision.${decision === 'accept' ? 'acceptDescription' : 'rejectDescription'}`)}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              disabled={decisionPending}
              onClick={() => {
                if (message && decision)
                  void onDecision(message, decision)
                setDecision(null)
              }}
            >
              {t(decision === 'accept' ? 'inbox.actions.accept' : 'inbox.actions.reject')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
