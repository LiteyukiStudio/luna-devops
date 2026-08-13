import type { AgentObservabilityTraceSpan } from '@/api'

export function availableToolNames(span: Pick<AgentObservabilityTraceSpan, 'name' | 'attributes'>): string[] {
  if (span.name !== 'agent.tools.available')
    return []
  try {
    const value: unknown = JSON.parse(span.attributes['luna.agent.available_tool.names'] ?? '[]')
    return Array.isArray(value)
      ? [...new Set(value.filter((item): item is string => typeof item === 'string' && item.length > 0))].sort()
      : []
  }
  catch {
    return []
  }
}
