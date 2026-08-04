import type { AgentObservabilityConversationLoop, AgentObservabilityTrace } from '@/api'
import { Bot, BrainCircuit, Wrench } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { AgentConversationMessage } from './agent-conversation-message'
import { AgentObservabilityToolCall } from './agent-observability-tool-call'

export function AgentConversationLoopView({ loop, onViewTrace }: {
  loop: AgentObservabilityConversationLoop
  onViewTrace: (trace: AgentObservabilityTrace) => void
}) {
  const { t } = useTranslation()
  const toolCount = loop.items.filter(item => item.type === 'tool_call').length
  return (
    <section className="grid gap-3 rounded-container bg-surface-subtle p-3">
      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <BrainCircuit className="size-4" />
        <strong className="text-foreground">{t('operationsDashboardPage.conversationDetail.loopLabel', { index: loop.loopIndex })}</strong>
        <span className="flex items-center gap-1">
          <Wrench className="size-3.5" />
          {t('operationsDashboardPage.conversationDetail.toolCount', { count: toolCount })}
        </span>
      </div>
      <div className="grid gap-3 border-l border-separator-subtle pl-3">
        {loop.items.map((item) => {
          if (item.type === 'reasoning_summary' && item.text)
            return <AgentConversationMessage key={item.id} role="thinking" text={item.text} />
          if (item.type === 'assistant_message' && item.text)
            return <AgentConversationMessage key={item.id} role="assistant" text={item.text} />
          if (item.type === 'tool_call' && item.toolCall)
            return <AgentObservabilityToolCall key={item.id} call={item.toolCall} onViewTrace={onViewTrace} />
          return null
        })}
        {loop.items.length === 0 && (
          <p className="m-0 flex items-center gap-2 text-xs text-muted-foreground">
            <Bot className="size-4" />
            {t('operationsDashboardPage.conversationDetail.emptyLoop')}
          </p>
        )}
      </div>
    </section>
  )
}
