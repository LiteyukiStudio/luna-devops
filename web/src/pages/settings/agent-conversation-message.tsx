import { Bot, Sparkles, UserRound } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { AIMarkdown } from '@/components/common/ai-assistant/markdown'
import { cn } from '@/lib/utils'

export function AgentConversationMessage({ role, text }: { role: 'user' | 'assistant' | 'thinking', text: string }) {
  const { t } = useTranslation()
  const Icon = role === 'user' ? UserRound : role === 'thinking' ? Sparkles : Bot
  return (
    <div className={cn('grid min-w-0 gap-2', role === 'thinking' && 'rounded-control bg-surface-inset/70 p-3')}>
      <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
        <Icon className="size-4" />
        {t(`operationsDashboardPage.conversationDetail.${role}`)}
      </div>
      <div className={cn('min-w-0 break-words text-sm leading-6', role !== 'thinking' && 'rounded-control bg-muted px-4 py-3')}>
        <AIMarkdown className="min-w-0 max-w-full text-foreground">{text}</AIMarkdown>
      </div>
    </div>
  )
}
