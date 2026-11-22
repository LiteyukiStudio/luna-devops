import type { InboxDecision, InboxFilter, InboxMessage, InboxSelfHealPreviewOutput } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Archive, Bell, CheckCheck, LoaderCircle, RefreshCw, Sparkles } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import { api } from '@/api'
import { ApiError } from '@/api/core'
import {
  formatHumanApprovalValue,
  resolveApprovalFieldLabel,
  resolveApprovalWarningText,
} from '@/components/common/ai-assistant/approval-presentation'
import { toolDisplayName } from '@/components/common/ai-assistant/tool-display-name'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { ContentTabs } from '@/components/common/content-tabs'
import { DataList } from '@/components/common/data-list'
import { ErrorState } from '@/components/common/error-state'
import { InboxMessageAvatar } from '@/components/common/inbox/inbox-message-row'
import { inboxKeys, invalidateInbox } from '@/components/common/inbox/inbox-query'
import { inboxMessageText, stripInlineMarkdown } from '@/components/common/inbox/message-format'
import { PageShell } from '@/components/common/page-shell'
import { StatusBadge } from '@/components/common/status-badge'
import { formatAbsoluteDateTime, formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'

const filters: InboxFilter[] = ['all', 'unread', 'action']
const RUNTIME_SELF_HEAL_TYPE = 'runtime.self_heal'

function formatArgScalar(value: unknown): string {
  if (value === undefined)
    return '-'
  if (typeof value === 'string')
    return value || '""'
  if (typeof value === 'number' || typeof value === 'boolean' || value === null)
    return String(value)
  return JSON.stringify(value)
}

function isStateDriftOrLockError(error: unknown): boolean {
  if (!(error instanceof ApiError))
    return false
  return (
    error.code === 'inbox.self_heal_state_drifted'
    || error.code === 'ai.preview_lock_mismatch'
    || error.code === 'ai.preview_lock_expired'
    || error.code === 'ai.preview_lock_invalid'
  )
}

function isStaleOrExpiredActionError(error: unknown): boolean {
  if (!(error instanceof ApiError))
    return false
  return (
    error.code === 'inbox.action_request_stale'
    || error.code === 'inbox.action_request_expired'
    || error.code === 'inbox.billing_owner_transfer_stale'
    || error.code === 'inbox.billing_owner_transfer_expired'
  )
}

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
    mutationFn: ({ decision, expectedVersion, previewLock, requestId }: { decision: InboxDecision, expectedVersion: number, previewLock?: string, requestId: string }) =>
      api.decideInboxActionRequest(requestId, decision, expectedVersion, previewLock),
    onSuccess: async () => {
      toast.success(t('inbox.decision.success'))
      await invalidateInbox(queryClient)
    },
    onError: (error) => {
      if (isStaleOrExpiredActionError(error))
        void invalidateInbox(queryClient)
      toast.error(error instanceof Error ? error.message : t('errors.request.failed'))
    },
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
        const rawAiSummary = typeof message.params?.aiRootCauseSummary === 'string' ? message.params.aiRootCauseSummary : ''
        const aiSummary = rawAiSummary ? stripInlineMarkdown(rawAiSummary) : ''
        return (
          <div className="flex min-w-0 items-start gap-3">
            <InboxMessageAvatar className="mt-0.5" message={message} showUnreadDot />
            <div className="min-w-0 flex-1">
              <p className={message.readAt ? 'truncate font-medium text-foreground/90' : 'truncate font-semibold text-foreground'}>{text.title}</p>
              <p className="mt-0.5 line-clamp-2 text-xs leading-5 text-muted-foreground wrap-anywhere">{text.content}</p>
              {aiSummary && (
                <p className="mt-1.5 inline-flex max-w-full min-w-0 items-start gap-1.5 rounded-control border border-primary-border/45 bg-primary-subtle/35 px-2 py-1 text-xs leading-[1.125rem] text-foreground">
                  <Sparkles aria-hidden="true" className="mt-0.5 size-3.5 shrink-0 text-primary" />
                  <span className="min-w-0 line-clamp-2 wrap-anywhere">{aiSummary}</span>
                </p>
              )}
              <div className="mt-2 flex flex-wrap items-center gap-2 @xl/data-list:hidden">
                <StatusBadge>{t(`inbox.categories.${message.category}`)}</StatusBadge>
                {message.actionRequest
                  ? <StatusBadge tone={message.actionRequest.status === 'pending' ? 'warning' : 'neutral'}>{t(`inbox.states.${message.actionRequest.status}`)}</StatusBadge>
                  : !message.readAt
                      ? <StatusBadge tone="info">{t('inbox.states.unread')}</StatusBadge>
                      : null}
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
        <Button aria-label={t('inbox.actions.archive')} className="size-8" disabled={archive.isPending} size="icon" variant="ghost" onClick={() => archive.mutate(message.id)}>
          <Archive />
        </Button>
      ),
    },
  ], [archive, t])

  const selectFilter = (value: string) => {
    setFilter(value as InboxFilter)
    setPage(1)
  }
  const changePageSize = (value: number) => {
    setPageSize(value)
    setPage(1)
  }

  return (
    <PageShell spacing="compact" width="full">
      <ContentTabs
        hashRouting={false}
        tabs={filters.map(value => ({ label: t(`inbox.tabs.${value}`), value }))}
        tools={(
          <Button disabled={markAllRead.isPending} variant="outline" onClick={() => markAllRead.mutate()}>
            <CheckCheck />
            {t('inbox.actions.markAllRead')}
          </Button>
        )}
        value={filter}
        onValueChange={selectFilter}
      >
        <DataList
          columns={columns}
          emptyDescription={t('inbox.empty.description')}
          emptyIcon={<Bell className="size-5" />}
          emptyTitle={t(`inbox.empty.${filter}`)}
          error={messages.isError
            ? {
                description: t('inbox.loadFailedDescription'),
                onRetry: () => void messages.refetch(),
                retrying: messages.isFetching,
                title: t('inbox.loadFailedTitle'),
              }
            : undefined}
          items={messages.data?.items ?? []}
          loading={messages.isPending}
          pagination={messages.data && messages.data.total > 0
            ? {
                page,
                pageSize,
                total: messages.data.total,
                totalPages: messages.data.totalPages,
                pageInfoLabel: t('pagination.pageInfo', { page, totalPages: messages.data.totalPages, total: messages.data.total }),
                onPageChange: setPage,
                onPageSizeChange: changePageSize,
              }
            : undefined}
          rowActionLabel={() => t('inbox.actions.viewDetails')}
          rowKey={message => message.id}
          onRowClick={openDetails}
        />
        <InboxDetail
          archivePending={archive.isPending}
          decisionPending={decide.isPending}
          error={selectedMessage.isError}
          key={`${selectedMessageId}:${selectedMessage.data?.actionRequest?.id ?? ''}:${selectedMessage.data?.actionRequest?.rowVersion ?? 0}`}
          message={selectedMessage.data}
          open={Boolean(selectedMessageId)}
          pending={Boolean(selectedMessageId) && selectedMessage.isPending}
          retrying={selectedMessage.isFetching}
          onArchive={messageId => archive.mutate(messageId)}
          onClose={closeDetails}
          onDecision={async (message, decision, previewLock) => {
            const request = message.actionRequest
            if (!request)
              return
            await decide.mutateAsync({ decision, expectedVersion: request.rowVersion, previewLock, requestId: request.id })
          }}
          onRetry={() => void selectedMessage.refetch()}
        />
      </ContentTabs>
    </PageShell>
  )
}

function InboxDetail({
  archivePending,
  decisionPending,
  error,
  message,
  open,
  pending,
  retrying,
  onArchive,
  onClose,
  onDecision,
  onRetry,
}: {
  archivePending: boolean
  decisionPending: boolean
  error: boolean
  message?: InboxMessage
  open: boolean
  pending: boolean
  retrying: boolean
  onArchive: (messageId: string) => void
  onClose: () => void
  onDecision: (message: InboxMessage, decision: InboxDecision, previewLock?: string) => Promise<void>
  onRetry: () => void
}) {
  const { t } = useTranslation()
  const [decision, setDecision] = useState<InboxDecision | null>(null)
  const [stateDrifted, setStateDrifted] = useState(false)
  const [repreviewOutput, setRepreviewOutput] = useState<InboxSelfHealPreviewOutput | null>(null)

  const actionRequest = message?.actionRequest
  const isRuntimeSelfHeal = actionRequest?.type === RUNTIME_SELF_HEAL_TYPE
  const actionable = actionRequest?.status === 'pending'

  const queryClient = useQueryClient()
  const repreviewMutation = useMutation({
    mutationFn: (requestId: string) => api.previewInboxSelfHealAction(requestId),
    onSuccess: (output) => {
      setRepreviewOutput(output)
      setStateDrifted(output.stateDrifted)
    },
    onError: (err) => {
      if (isStaleOrExpiredActionError(err))
        void invalidateInbox(queryClient)
      toast.error(err instanceof Error ? err.message : t('errors.request.failed'))
    },
  })

  const text = message ? inboxMessageText(message, t) : null
  const params = message?.params ?? {}
  const aiRootCauseSummary = typeof params.aiRootCauseSummary === 'string' ? params.aiRootCauseSummary.trim() : ''
  const aiSuggestions = Array.isArray(params.aiSuggestions)
    ? [...new Set(params.aiSuggestions.filter((item): item is string => typeof item === 'string' && item.trim().length > 0))]
    : []
  const aiAutoHealSummary = typeof params.aiAutoHealSummary === 'string' ? params.aiAutoHealSummary.trim() : ''
  const aiCircuitBreakerTripped = params.aiCircuitBreakerTripped === true
  const proposal = isRuntimeSelfHeal ? actionRequest?.proposal : undefined
  const proposalEntries = proposal?.arguments
    ? Object.entries(proposal.arguments).filter(([, val]) => val !== undefined)
    : []

  const handleConfirmDecision = async () => {
    if (!message || !decision)
      return
    try {
      const lockToPass = decision === 'accept' && isRuntimeSelfHeal ? repreviewOutput?.previewLock : undefined
      await onDecision(message, decision, lockToPass)
      setDecision(null)
      setStateDrifted(false)
      setRepreviewOutput(null)
    }
    catch (err) {
      setDecision(null)
      if (isRuntimeSelfHeal && isStateDriftOrLockError(err)) {
        setStateDrifted(true)
        setRepreviewOutput(null)
      }
    }
  }

  return (
    <>
      <Sheet open={open} onOpenChange={value => !value && onClose()}>
        <SheetContent className="w-[min(92vw,34rem)] max-w-none gap-0 overflow-y-auto p-0 sm:max-w-none" side="right">
          <SheetHeader className="border-b border-border p-6 pr-14">
            <SheetTitle>{text?.title ?? t('inbox.detail.title')}</SheetTitle>
            <SheetDescription>{message ? formatAbsoluteDateTime(message.createdAt) : pending ? t('common.loading') : ''}</SheetDescription>
          </SheetHeader>
          {error
            ? (
                <div className="p-6">
                  <ErrorState
                    description={t('inbox.detail.loadFailedDescription')}
                    retrying={retrying}
                    title={t('inbox.detail.loadFailedTitle')}
                    onRetry={onRetry}
                  />
                </div>
              )
            : pending && !message
              ? (
                  <div aria-label={t('common.loading')} className="grid gap-3 p-6" role="status">
                    <Skeleton className="h-4 w-full" />
                    <Skeleton className="h-4 w-4/5" />
                    <Skeleton className="h-6 w-24" />
                  </div>
                )
              : message && text && (
                <div className="space-y-5 p-6">
                  <p className="whitespace-pre-wrap break-words text-sm leading-6 text-muted-foreground">{text.content}</p>

                  {aiCircuitBreakerTripped && (
                    <div className="flex items-start gap-2.5 rounded-control border border-destructive/40 bg-destructive/5 p-3 text-xs text-destructive" role="alert">
                      <AlertTriangle aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
                      <span>{t('inbox.selfHeal.circuitBreakerBanner')}</span>
                    </div>
                  )}

                  {aiRootCauseSummary && (
                    <div className="space-y-2.5 rounded-control border border-border bg-surface-subtle/50 p-3.5 text-sm">
                      <div className="flex items-center gap-1.5 text-xs font-semibold text-foreground">
                        <Sparkles aria-hidden="true" className="size-3.5 text-primary" />
                        <span>{t('inbox.selfHeal.aiDiagnosisTitle')}</span>
                      </div>
                      <p className="whitespace-pre-wrap break-words text-xs leading-relaxed text-foreground">{stripInlineMarkdown(aiRootCauseSummary, { preserveNewlines: true })}</p>
                      {aiSuggestions.length > 0 && (
                        <div className="space-y-1 pt-1">
                          <div className="text-[11px] font-medium text-muted-foreground">{t('inbox.selfHeal.suggestionsTitle')}</div>
                          <ul className="list-disc space-y-1 pl-4 text-xs text-muted-foreground">
                            {aiSuggestions.map(item => (
                              <li key={item}>{stripInlineMarkdown(item)}</li>
                            ))}
                          </ul>
                        </div>
                      )}
                      {aiAutoHealSummary && (
                        <div className="rounded-control border border-border bg-background/80 p-2.5 text-xs">
                          <div className="font-medium text-foreground">{t('inbox.selfHeal.autoHealExecutedTitle')}</div>
                          <p className="mt-1 whitespace-pre-wrap break-words text-muted-foreground">{stripInlineMarkdown(aiAutoHealSummary, { preserveNewlines: true })}</p>
                        </div>
                      )}
                    </div>
                  )}

                  {isRuntimeSelfHeal && proposal && (
                    <div className="space-y-3 rounded-control border border-border bg-background p-3.5">
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <span className="text-xs font-semibold text-foreground">{t('inbox.selfHeal.proposalTitle')}</span>
                        <StatusBadge tone="info">{toolDisplayName(t, proposal.toolName)}</StatusBadge>
                      </div>
                      {proposal.expectedSummary && (
                        <p className="text-xs leading-relaxed text-muted-foreground">{proposal.expectedSummary}</p>
                      )}
                      {proposalEntries.length > 0 && (
                        <dl className="grid gap-1 rounded-control border border-border bg-surface-subtle/40 p-2.5 text-xs">
                          {proposalEntries.map(([key, val]) => (
                            <div className="flex items-baseline justify-between gap-3" key={key}>
                              <dt className="font-mono text-muted-foreground">{key}</dt>
                              <dd className="min-w-0 break-all font-mono text-foreground">{formatArgScalar(val)}</dd>
                            </div>
                          ))}
                        </dl>
                      )}

                      {stateDrifted && !repreviewOutput && (
                        <div className="space-y-2.5 rounded-control border border-warning/40 bg-warning/5 p-3 text-xs" role="alert">
                          <div className="flex items-start gap-2 font-medium text-foreground">
                            <AlertTriangle aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-warning" />
                            <span>{t('inbox.selfHeal.stateDriftedTitle')}</span>
                          </div>
                          <p className="leading-relaxed text-muted-foreground">{t('inbox.selfHeal.stateDriftedDescription')}</p>
                          {actionable && actionRequest && (
                            <Button
                              disabled={repreviewMutation.isPending}
                              size="sm"
                              type="button"
                              variant="outline"
                              onClick={() => repreviewMutation.mutate(actionRequest.id)}
                            >
                              {repreviewMutation.isPending
                                ? <LoaderCircle aria-hidden="true" className="size-3.5 animate-spin motion-reduce:animate-none" />
                                : <RefreshCw className="size-3.5" />}
                              {t('inbox.selfHeal.repreviewAction')}
                            </Button>
                          )}
                        </div>
                      )}

                      {repreviewOutput && (
                        <div className="space-y-2 rounded-control border border-border bg-surface-subtle/60 p-3 text-xs">
                          <div className="font-medium text-foreground">{t('inbox.selfHeal.repreviewSummaryTitle')}</div>
                          {repreviewOutput.approvalPreview.summary && (
                            <p className="text-muted-foreground">{repreviewOutput.approvalPreview.summary}</p>
                          )}
                          {Array.isArray(repreviewOutput.approvalPreview.changes) && repreviewOutput.approvalPreview.changes.length > 0 && (
                            <dl className="grid gap-1 pt-1">
                              {repreviewOutput.approvalPreview.changes.map(ch => (
                                <div className="flex items-baseline justify-between gap-2" key={ch.field}>
                                  <dt className="text-muted-foreground">{resolveApprovalFieldLabel(t, ch.field)}</dt>
                                  <dd className="font-mono text-foreground">
                                    {ch.before !== undefined
                                      ? `${formatHumanApprovalValue(t, formatArgScalar(ch.before))} -> ${formatHumanApprovalValue(t, formatArgScalar(ch.after))}`
                                      : formatHumanApprovalValue(t, formatArgScalar(ch.after))}
                                  </dd>
                                </div>
                              ))}
                            </dl>
                          )}
                          {Array.isArray(repreviewOutput.approvalPreview.warnings) && repreviewOutput.approvalPreview.warnings.length > 0 && (
                            <ul className="list-disc space-y-1 pl-4 text-warning">
                              {repreviewOutput.approvalPreview.warnings.map(warning => (
                                <li key={warning}>{resolveApprovalWarningText(t, warning)}</li>
                              ))}
                            </ul>
                          )}
                        </div>
                      )}
                    </div>
                  )}

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
                    <Button disabled={archivePending} variant="ghost" onClick={() => onArchive(message.id)}>
                      <Archive className="size-4" />
                      {t('inbox.actions.archive')}
                    </Button>
                    {message.deepLink && <Button asChild variant="outline"><Link to={message.deepLink}>{t('inbox.actions.openResource')}</Link></Button>}
                    {actionable && actionRequest.allowedDecisions.includes('reject') && (
                      <Button variant="outline" onClick={() => setDecision('reject')}>
                        {isRuntimeSelfHeal ? t('inbox.selfHeal.ignoreAction') : t('inbox.actions.reject')}
                      </Button>
                    )}
                    {actionable && actionRequest.allowedDecisions.includes('accept') && (!stateDrifted || Boolean(repreviewOutput)) && (
                      <Button onClick={() => setDecision('accept')}>
                        {isRuntimeSelfHeal
                          ? (repreviewOutput ? t('inbox.selfHeal.confirmWithLockAction') : t('inbox.selfHeal.confirmAction'))
                          : t('inbox.actions.accept')}
                      </Button>
                    )}
                  </div>
                </div>
              )}
        </SheetContent>
      </Sheet>
      <ConfirmDialog
        cancelText={t('common.cancel')}
        confirmText={isRuntimeSelfHeal
          ? (decision === 'accept'
              ? (repreviewOutput ? t('inbox.selfHeal.confirmWithLockAction') : t('inbox.selfHeal.confirmAction'))
              : t('inbox.selfHeal.ignoreAction'))
          : t(decision === 'accept' ? 'inbox.actions.accept' : 'inbox.actions.reject')}
        confirmVariant="default"
        description={isRuntimeSelfHeal
          ? t(decision === 'accept' ? 'inbox.selfHeal.confirmDialogDescription' : 'inbox.selfHeal.ignoreDialogDescription')
          : t(`inbox.decision.${decision === 'accept' ? 'acceptDescription' : 'rejectDescription'}`)}
        open={decision !== null}
        pending={decisionPending}
        title={isRuntimeSelfHeal
          ? t(decision === 'accept' ? 'inbox.selfHeal.confirmDialogTitle' : 'inbox.selfHeal.ignoreDialogTitle')
          : t(`inbox.decision.${decision === 'accept' ? 'acceptTitle' : 'rejectTitle'}`)}
        onConfirm={handleConfirmDecision}
        onOpenChange={value => !value && setDecision(null)}
      />
    </>
  )
}
