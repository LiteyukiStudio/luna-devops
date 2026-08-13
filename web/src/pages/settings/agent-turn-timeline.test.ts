import type { AgentObservabilityTraceSpan } from '@/api'
import { describe, expect, it } from 'vitest'
import { agentTurnTimelineKind, filterAgentTraceDisplaySpans, filterAgentTurnTimelineSpans } from './agent-turn-timeline'

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
    events: [],
    raw: {},
  }
}

describe('agent turn timeline', () => {
  it('maps stable Agent spans to business timeline kinds', () => {
    expect(agentTurnTimelineKind(span('agent.turn.accept', 0))).toBe('turn')
    expect(agentTurnTimelineKind(span('agent.run.execute', 1))).toBe('agent')
    expect(agentTurnTimelineKind(span('agent.model.stream', 2))).toBe('model')
    expect(agentTurnTimelineKind(span('agent.tools.available', 2.5))).toBe('toolset')
    expect(agentTurnTimelineKind({ ...span('custom', 3), attributes: { 'gen_ai.tool.name': 'get_project' } })).toBe('tool')
    expect(agentTurnTimelineKind(span('agent.repository.turn.create', 4))).toBe('storage')
    expect(agentTurnTimelineKind(span('luna_api.tool.execute', 5))).toBe('external')
  })

  it('sorts spans chronologically and filters model, tool, and error steps', () => {
    const spans = [span('agent.tool.execute', 30), span('agent.model.stream', 20, 'error'), span('agent.tools.available', 19), span('agent.run.execute', 10)]
    expect(filterAgentTurnTimelineSpans(spans, 'all').map(item => item.name)).toEqual([
      'agent.run.execute',
      'agent.tools.available',
      'agent.model.stream',
      'agent.tool.execute',
    ])
    expect(filterAgentTurnTimelineSpans(spans, 'model').map(item => item.name)).toEqual(['agent.tools.available', 'agent.model.stream'])
    expect(filterAgentTurnTimelineSpans(spans, 'tool')).toHaveLength(1)
    expect(filterAgentTurnTimelineSpans(spans, 'error').map(item => item.name)).toEqual(['agent.model.stream'])
  })

  it('hides infrastructure spans by default while retaining Agent, model, and tool steps', () => {
    const spans = [span('agent.run.execute', 1), span('agent.model.stream', 2), span('agent.tool.execute', 3), span('agent.repository.turn.create', 4), { ...span('http.request', 5), kind: 'client' }]
    expect(filterAgentTurnTimelineSpans(spans, 'all').map(item => item.name)).toEqual([
      'agent.run.execute',
      'agent.model.stream',
      'agent.tool.execute',
    ])
  })

  it('includes infrastructure spans only when external services are shown', () => {
    const spans = [span('agent.run.execute', 1), span('agent.repository.turn.create', 2), { ...span('http.request', 3), kind: 'client' }, span('luna_api.tool.execute', 4)]
    expect(filterAgentTurnTimelineSpans(spans, 'all', true).map(item => item.name)).toEqual([
      'agent.run.execute',
      'agent.repository.turn.create',
      'http.request',
    ])
  })

  it('shows one canonical tool step by default instead of its API transport child', () => {
    const logical = span('agent.tool.execute', 1)
    const transport = { ...span('luna_api.tool.execute', 2), parentSpanId: logical.spanId, kind: 'client' as const }
    const request = { ...span('HTTP POST', 3), parentSpanId: transport.spanId, kind: 'client' as const }
    expect(filterAgentTurnTimelineSpans([logical, transport], 'tool').map(item => item.name)).toEqual(['agent.tool.execute'])
    expect(filterAgentTurnTimelineSpans([logical, transport], 'all', true).map(item => item.name)).toEqual(['agent.tool.execute'])
    expect(filterAgentTraceDisplaySpans([logical, transport, request]).map(item => [item.name, item.parentSpanId])).toEqual([
      ['agent.tool.execute', ''],
      ['HTTP POST', logical.spanId],
    ])
  })
})
