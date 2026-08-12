import type { AgentObservabilityTraceSpan } from '@/api'
import { describe, expect, it } from 'vitest'
import { agentTurnTimelineKind, filterAgentTurnTimelineSpans } from './agent-turn-timeline'

function span(name: string, startOffsetMs: number, status: AgentObservabilityTraceSpan['status'] = 'ok'): AgentObservabilityTraceSpan {
  return {
    spanId: name,
    parentSpanId: '',
    name,
    serviceName: 'luna-agent',
    kind: 'internal',
    status,
    startTimeUnixNano: '1000000000',
    startOffsetMs,
    durationMs: 10,
    attributes: {},
  }
}

describe('agent turn timeline', () => {
  it('maps stable Agent spans to business timeline kinds', () => {
    expect(agentTurnTimelineKind(span('agent.turn.accept', 0))).toBe('turn')
    expect(agentTurnTimelineKind(span('agent.run.execute', 1))).toBe('agent')
    expect(agentTurnTimelineKind(span('agent.model.stream', 2))).toBe('model')
    expect(agentTurnTimelineKind({ ...span('custom', 3), attributes: { 'gen_ai.tool.name': 'get_project' } })).toBe('tool')
    expect(agentTurnTimelineKind(span('agent.repository.turn.create', 4))).toBe('storage')
  })

  it('sorts spans chronologically and filters model, tool, and error steps', () => {
    const spans = [span('agent.tool.execute', 30), span('agent.model.stream', 20, 'error'), span('agent.run.execute', 10)]
    expect(filterAgentTurnTimelineSpans(spans, 'all').map(item => item.name)).toEqual([
      'agent.run.execute',
      'agent.model.stream',
      'agent.tool.execute',
    ])
    expect(filterAgentTurnTimelineSpans(spans, 'model')).toHaveLength(1)
    expect(filterAgentTurnTimelineSpans(spans, 'tool')).toHaveLength(1)
    expect(filterAgentTurnTimelineSpans(spans, 'error').map(item => item.name)).toEqual(['agent.model.stream'])
  })
})
