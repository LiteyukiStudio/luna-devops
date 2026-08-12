import type { AgentObservabilityTraceSpan } from '@/api'

export type AgentTurnTimelineFilter = 'all' | 'model' | 'tool' | 'error'
export type AgentTurnTimelineKind = 'turn' | 'agent' | 'model' | 'tool' | 'storage' | 'external' | 'other'

export function agentTurnTimelineKind(span: AgentObservabilityTraceSpan): AgentTurnTimelineKind {
  if (span.name === 'agent.turn.accept')
    return 'turn'
  if (span.name === 'agent.run.execute')
    return 'agent'
  if (span.name.includes('model') || span.name.startsWith('gen_ai.chat'))
    return 'model'
  if (span.name.includes('tool') || Boolean(span.attributes['gen_ai.tool.name']))
    return 'tool'
  if (span.name.includes('repository') || span.name.includes('db.') || span.name.includes('postgres'))
    return 'storage'
  if (span.kind === 'client' || span.kind === 'server' || span.name.startsWith('luna_api.'))
    return 'external'
  return 'other'
}

export function filterAgentTurnTimelineSpans(spans: AgentObservabilityTraceSpan[], filter: AgentTurnTimelineFilter, hideExternalServices = false) {
  return [...spans]
    .sort((left, right) => left.startOffsetMs - right.startOffsetMs || left.durationMs - right.durationMs)
    .filter((span) => {
      const kind = agentTurnTimelineKind(span)
      if (hideExternalServices && !['turn', 'agent', 'model', 'tool'].includes(kind))
        return false
      if (filter === 'error')
        return span.status === 'error'
      if (filter === 'model' || filter === 'tool')
        return kind === filter
      return true
    })
}
