import type { AgentObservabilityConversation, AgentObservabilityTrace } from '@/api'
import { useQuery } from '@tanstack/react-query'
import { Bot, Clock3, MessageSquareText, Network, UserRound } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { ErrorState } from '@/components/common/error-state'
import { ToolViewportSkeleton } from '@/components/common/loading-states'
import { MetricGroup, MetricItem } from '@/components/common/metric-group'
import { PaginationController } from '@/components/common/pagination'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'

export function AgentConversationDetailSheet({ conversation, turnPage, onOpenChange, onTurnPageChange, onViewTrace }: {
  conversation: AgentObservabilityConversation | null
  turnPage: number
  onOpenChange: (open: boolean) => void
  onTurnPageChange: (page: number) => void
  onViewTrace: (trace: AgentObservabilityTrace) => void
}) {
  const { t, i18n } = useTranslation()
  const detail = useQuery({
    queryKey: ['agent-observability-conversation', conversation?.id, turnPage],
    queryFn: () => api.getAgentObservabilityConversation(conversation!.id, turnPage, 20),
    enabled: Boolean(conversation),
  })

  return (
    <Sheet open={Boolean(conversation)} onOpenChange={onOpenChange}>
      <SheetContent className="w-[min(96vw,64rem)] max-w-none gap-0 p-0 sm:max-w-none" side="right">
        <SheetHeader className="border-b border-border px-6 py-4">
          <SheetTitle>{conversation?.title || t('operationsDashboardPage.conversationDetail.untitled')}</SheetTitle>
          <SheetDescription>
            {conversation?.user.name || conversation?.user.email}
            {conversation?.user.name && conversation?.user.email ? ` · ${conversation.user.email}` : ''}
          </SheetDescription>
        </SheetHeader>
        <div className="min-h-0 flex-1 overflow-auto p-6">
          {detail.isLoading && <ToolViewportSkeleton />}
          {detail.isError && <ErrorState title={t('operationsDashboardPage.conversationDetail.loadFailed')} description={t('operationsDashboardPage.conversationDetail.loadFailedDescription')} />}
          {detail.data && (
            <div className="grid gap-6">
              <MetricGroup>
                <MetricItem icon={<UserRound className="size-4" />} label={t('operationsDashboardPage.conversationDetail.owner')} value={detail.data.user.name || detail.data.user.email} />
                <MetricItem icon={<MessageSquareText className="size-4" />} label={t('operationsDashboardPage.conversationDetail.turns')} value={String(detail.data.turnCount)} />
                <MetricItem icon={<Network className="size-4" />} label={t('operationsDashboardPage.conversationDetail.traces')} value={String(detail.data.traceCount)} />
                <MetricItem icon={<Clock3 className="size-4" />} label={t('operationsDashboardPage.conversationDetail.updatedAt')} value={formatDate(detail.data.updatedAt, i18n.language)} />
              </MetricGroup>
              <div className="grid gap-4">
                {detail.data.turns.map(turn => (
                  <section key={turn.id} className="overflow-hidden rounded-container bg-surface-raised">
                    <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border px-4 py-3">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="text-sm font-semibold">{t('operationsDashboardPage.conversationDetail.turnLabel', { index: turn.turnIndex + 1 })}</span>
                        <StatusBadge tone={statusTone(turn.status)}>{t(`operationsDashboardPage.runStatus.${turn.status}`, { defaultValue: turn.status })}</StatusBadge>
                        <span className="text-xs text-muted-foreground">{formatDate(turn.createdAt, i18n.language)}</span>
                        {turn.durationMs > 0 && <span className="text-xs text-muted-foreground">{formatDuration(turn.durationMs)}</span>}
                      </div>
                      {turn.traceId && (
                        <Button size="sm" variant="outline" onClick={() => onViewTrace(traceFromTurn(turn.traceId, turn.createdAt, turn.durationMs))}>
                          <Network className="size-4" />
                          {t('operationsDashboardPage.viewTrace')}
                        </Button>
                      )}
                    </div>
                    <div className="grid gap-4 p-4">
                      <Message role="user" text={turn.userMessage} />
                      <Message role="assistant" text={turn.assistantMessage || t('operationsDashboardPage.conversationDetail.noAssistantMessage')} />
                    </div>
                  </section>
                ))}
              </div>
              <PaginationController hideOnSinglePage initialPage={detail.data.turnPage} pageSize={detail.data.turnPageSize} total={detail.data.turnCount} onPageChange={onTurnPageChange} />
            </div>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}

function Message({ role, text }: { role: 'user' | 'assistant', text: string }) {
  const { t } = useTranslation()
  const Icon = role === 'user' ? UserRound : Bot
  return (
    <div className="grid gap-2">
      <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
        <Icon className="size-4" />
        {t(`operationsDashboardPage.conversationDetail.${role}`)}
      </div>
      <div className="whitespace-pre-wrap break-words rounded-control bg-muted px-4 py-3 text-sm leading-6">{text}</div>
    </div>
  )
}

function traceFromTurn(traceId: string, createdAt: string, durationMs: number): AgentObservabilityTrace {
  return {
    traceId,
    rootServiceName: 'luna-agent',
    rootTraceName: 'agent.run.execute',
    startTimeUnixNano: String(new Date(createdAt).getTime() * 1_000_000),
    durationMs,
  }
}

function statusTone(status: string): 'success' | 'danger' | 'warning' | 'neutral' {
  if (status === 'completed')
    return 'success'
  if (status === 'failed' || status === 'canceled' || status === 'expired')
    return 'danger'
  if (status.startsWith('waiting_'))
    return 'warning'
  return 'neutral'
}

function formatDate(value: string, language: string) {
  return new Intl.DateTimeFormat(language, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(value))
}

function formatDuration(value: number) {
  return value < 1000 ? `${value.toFixed(value < 10 ? 2 : 1)} ms` : `${(value / 1000).toFixed(2)} s`
}
