import type { AgentObservabilityConversationToolCall, AgentObservabilityTrace, AgentObservabilityTraceContext } from '@/api'
import { MessageSquareText, UserRound } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { StatusBadge } from '@/components/common/status-badge'
import { AgentConversationLoopView } from './agent-conversation-loop'
import { AgentConversationMessage } from './agent-conversation-message'

export function AgentTraceContextPanel({ context, onSelectToolCall }: {
  context: AgentObservabilityTraceContext
  onSelectToolCall: (call: AgentObservabilityConversationToolCall) => void
}) {
  const { t } = useTranslation()
  const { conversation, turn } = context
  const owner = conversation.user.name || conversation.user.email
  const keepTraceOpen = (_trace: AgentObservabilityTrace) => undefined

  return (
    <section className="grid gap-4 rounded-container bg-surface-raised p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="grid min-w-0 gap-1">
          <div className="flex items-center gap-2">
            <MessageSquareText className="size-4 shrink-0 text-muted-foreground" />
            <h3 className="truncate text-sm font-semibold">{conversation.title || t('operationsDashboardPage.conversationDetail.untitled')}</h3>
          </div>
          <p className="m-0 flex items-center gap-1.5 text-xs text-muted-foreground">
            <UserRound className="size-3.5" />
            {owner}
            {conversation.user.name && conversation.user.email ? ` · ${conversation.user.email}` : ''}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-xs font-medium">{t('operationsDashboardPage.conversationDetail.turnLabel', { index: turn.turnIndex + 1 })}</span>
          <StatusBadge tone={statusTone(turn.status)}>{t(`operationsDashboardPage.runStatus.${turn.status}`, { defaultValue: turn.status })}</StatusBadge>
        </div>
      </div>
      <div className="grid gap-4 border-t border-separator-subtle pt-4">
        <AgentConversationMessage role="user" text={turn.userMessage} />
        {turn.loops.length > 0
          ? turn.loops.map(loop => (
              <AgentConversationLoopView
                key={loop.loopIndex}
                loop={loop}
                onSelectToolCall={onSelectToolCall}
                onViewTrace={keepTraceOpen}
              />
            ))
          : <AgentConversationMessage role="assistant" text={turn.assistantMessage || t('operationsDashboardPage.conversationDetail.noAssistantMessage')} />}
      </div>
    </section>
  )
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
