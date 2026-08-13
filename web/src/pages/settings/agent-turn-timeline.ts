import type { AgentObservabilityTraceSpan } from '@/api'

export type AgentTurnTimelineFilter = 'all' | 'model' | 'tool' | 'error'
export type AgentTurnTimelineKind = 'turn' | 'agent' | 'model' | 'tool' | 'storage' | 'external' | 'other'

export function isCanonicalAgentToolSpan(span: AgentObservabilityTraceSpan) {
  return span.serviceName === 'luna-agent' && (span.name === 'agent.tool.execute' || span.name === 'agent.tool.internal')
}

export function isAgentToolTransportSpan(span: AgentObservabilityTraceSpan) {
  return span.serviceName === 'luna-agent' && span.name === 'luna_api.tool.execute'
}

export function agentTurnTimelineKind(span: AgentObservabilityTraceSpan): AgentTurnTimelineKind {
  if (span.name === 'agent.turn.accept')
    return 'turn'
  if (span.name === 'agent.run.execute')
    return 'agent'
  if (span.name.includes('model') || span.name.startsWith('gen_ai.chat'))
    return 'model'
  if (isCanonicalAgentToolSpan(span))
    return 'tool'
  if (isAgentToolTransportSpan(span))
    return 'external'
  if (span.name.includes('tool') || Boolean(span.attributes['gen_ai.tool.name']))
    return span.kind === 'client' ? 'external' : 'tool'
  if (span.name.includes('repository') || span.name.includes('db.') || span.name.includes('postgres'))
    return 'storage'
  if (span.kind === 'client' || span.kind === 'server' || span.name.startsWith('luna_api.'))
    return 'external'
  return 'other'
}

export function filterAgentTurnTimelineSpans(spans: AgentObservabilityTraceSpan[], filter: AgentTurnTimelineFilter, showExternalServices = false) {
  return [...spans]
    .sort((left, right) => left.startOffsetMs - right.startOffsetMs || left.durationMs - right.durationMs)
    .filter((span) => {
      if (isAgentToolTransportSpan(span))
        return false
      const kind = agentTurnTimelineKind(span)
      if (!showExternalServices && !['turn', 'agent', 'model', 'tool'].includes(kind))
        return false
      if (filter === 'error')
        return span.status === 'error'
      if (filter === 'model' || filter === 'tool')
        return kind === filter
      return true
    })
}

export function filterAgentTraceDisplaySpans(spans: AgentObservabilityTraceSpan[]) {
  const byId = new Map(spans.map(span => [span.spanId, span]))
  const hiddenIds = new Set(spans.filter(isAgentToolTransportSpan).map(span => span.spanId))
  return spans
    .filter(span => !hiddenIds.has(span.spanId))
    .map((span) => {
      let parentSpanId = span.parentSpanId
      const visited = new Set<string>()
      while (hiddenIds.has(parentSpanId) && !visited.has(parentSpanId)) {
        visited.add(parentSpanId)
        parentSpanId = byId.get(parentSpanId)?.parentSpanId ?? ''
      }
      return parentSpanId === span.parentSpanId ? span : { ...span, parentSpanId }
    })
}
