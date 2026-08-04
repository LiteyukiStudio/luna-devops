import type { AgentObservabilityConversationToolCall, AgentObservabilityTrace } from '@/api'
import { Check, ChevronRight, CircleAlert, CircleDashed, Network } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { CopyableCodeBlock } from '@/components/common/ai-assistant/copyable-code-block'
import { toolDisplayName } from '@/components/common/ai-assistant/tool-display-name'
import { StatusBadge } from '@/components/common/status-badge'
import { formatMillisecondsDuration } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'

export function AgentObservabilityToolCall({ call, onViewTrace }: {
  call: AgentObservabilityConversationToolCall
  onViewTrace: (trace: AgentObservabilityTrace) => void
}) {
  const { t, i18n } = useTranslation()
  return (
    <details className="group overflow-hidden rounded-container bg-surface" open>
      <summary className="flex min-h-10 cursor-pointer list-none items-center gap-2 px-3 py-2 outline-none hover:bg-surface-inset focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring [&::-webkit-details-marker]:hidden">
        <ToolStatusIcon status={call.status} />
        <strong className="min-w-0 flex-1 truncate text-xs font-medium">{toolDisplayName(t, call.operationId)}</strong>
        <StatusBadge tone={toolTone(call.status)}>{t(`aiAssistant.status.${call.status}`, { defaultValue: call.status })}</StatusBadge>
        {call.durationMs !== undefined && <span className="text-xs text-muted-foreground">{formatMillisecondsDuration(call.durationMs, i18n.language)}</span>}
        <ChevronRight className="size-3.5 shrink-0 text-muted-foreground transition-transform group-open:rotate-90" />
      </summary>
      <div className="grid gap-3 border-t border-separator-subtle bg-surface-subtle/40 p-3">
        <ToolPayload label={t('aiAssistant.arguments')} value={call.arguments} />
        <ToolPayload label={t('aiAssistant.returnValue')} value={call.result} emptyLabel={t('operationsDashboardPage.conversationDetail.toolResultPending')} />
        {call.errorCode && <p className="m-0 break-all rounded-control bg-danger-subtle px-3 py-2 text-xs text-danger">{call.errorCode}</p>}
        {call.traceId && (
          <div className="flex justify-end">
            <Button size="sm" variant="outline" onClick={() => onViewTrace(toolTrace(call))}>
              <Network className="size-4" />
              {t('operationsDashboardPage.conversationDetail.viewToolTrace')}
            </Button>
          </div>
        )}
      </div>
    </details>
  )
}

function ToolPayload({ label, value, emptyLabel }: { label: string, value: unknown, emptyLabel?: string }) {
  return (
    <section className="grid min-w-0 gap-1.5">
      <h4 className="text-[11px] font-semibold text-muted-foreground">{label}</h4>
      {value === undefined
        ? <p className="m-0 rounded-control bg-surface px-3 py-2 text-xs text-muted-foreground">{emptyLabel}</p>
        : <CopyableCodeBlock className="max-h-64" value={formatJSON(value)}><code>{formatJSON(value)}</code></CopyableCodeBlock>}
    </section>
  )
}

function ToolStatusIcon({ status }: { status: string }) {
  if (status === 'succeeded')
    return <Check className="size-4 shrink-0 text-success" />
  if (status === 'failed')
    return <CircleAlert className="size-4 shrink-0 text-danger" />
  return <CircleDashed className="size-4 shrink-0 text-muted-foreground" />
}

function toolTone(status: string): 'success' | 'danger' | 'warning' | 'neutral' {
  if (status === 'succeeded')
    return 'success'
  if (status === 'failed')
    return 'danger'
  if (status.startsWith('awaiting_'))
    return 'warning'
  return 'neutral'
}

function formatJSON(value: unknown) {
  try {
    return JSON.stringify(value, null, 2)
  }
  catch {
    return String(value)
  }
}

function toolTrace(call: AgentObservabilityConversationToolCall): AgentObservabilityTrace {
  return { traceId: call.traceId ?? '', rootServiceName: 'luna-agent', rootTraceName: 'agent.tool.execute', startTimeUnixNano: '0', durationMs: call.durationMs ?? 0 }
}
