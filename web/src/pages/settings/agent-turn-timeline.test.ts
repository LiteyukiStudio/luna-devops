import type { AgentObservabilityTraceSpan } from '@/api'
import { describe, expect, it } from 'vitest'
import { agentTurnTimelineKind, filterAgentTraceDisplaySpans, filterAgentTurnTimelineSpans } from './agent-turn-timeline'

function span(name: string, startOffsetMs: number, status: AgentObservabilityTraceSpan['status'] = 'ok', attributes: Record<string, string> = {}): AgentObservabilityTraceSpan {
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
    attributes,
    events: [],
    raw: {},
  }
}

describe('agent turn timeline', () => {
  it('maps official GenAI operations to business timeline kinds', () => {
    expect(agentTurnTimelineKind(span('agent.turn.accept', 0))).toBe('turn')
    expect(agentTurnTimelineKind(span('invoke_agent Luna Agent', 1, 'ok', { 'gen_ai.operation.name': 'invoke_agent' }))).toBe('agent')
    expect(agentTurnTimelineKind(span('chat gpt-5', 2, 'ok', { 'gen_ai.operation.name': 'chat' }))).toBe('model')
    expect(agentTurnTimelineKind(span('agent.tools.available', 2.5))).toBe('toolset')
    expect(agentTurnTimelineKind(span('execute_tool getProject', 3, 'ok', { 'gen_ai.operation.name': 'execute_tool', 'gen_ai.tool.name': 'getProject' }))).toBe('tool')
    expect(agentTurnTimelineKind(span('agent.repository.turn.create', 4))).toBe('storage')
    expect(agentTurnTimelineKind(span('luna_api.tool.execute', 5))).toBe('external')
  })

  it('retains historical Luna Agent span classification', () => {
    expect(agentTurnTimelineKind(span('agent.run.execute', 1))).toBe('agent')
    expect(agentTurnTimelineKind(span('agent.model.stream', 2))).toBe('model')
    expect(agentTurnTimelineKind(span('agent.tool.execute', 3))).toBe('tool')
  })

  it('sorts spans chronologically and filters model, tool, and error steps', () => {
    const spans = [
      span('execute_tool getProject', 30, 'ok', { 'gen_ai.operation.name': 'execute_tool', 'gen_ai.tool.name': 'getProject' }),
      span('chat gpt-5', 20, 'error', { 'gen_ai.operation.name': 'chat' }),
      span('agent.tools.available', 19),
      span('invoke_agent Luna Agent', 10, 'ok', { 'gen_ai.operation.name': 'invoke_agent' }),
    ]
    expect(filterAgentTurnTimelineSpans(spans, 'all').map(item => item.name)).toEqual([
      'invoke_agent Luna Agent',
      'agent.tools.available',
      'chat gpt-5',
      'execute_tool getProject',
    ])
    expect(filterAgentTurnTimelineSpans(spans, 'model').map(item => item.name)).toEqual(['agent.tools.available', 'chat gpt-5'])
    expect(filterAgentTurnTimelineSpans(spans, 'tool')).toHaveLength(1)
    expect(filterAgentTurnTimelineSpans(spans, 'error').map(item => item.name)).toEqual(['chat gpt-5'])
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
    const logical = span('execute_tool getProject', 1, 'ok', { 'gen_ai.operation.name': 'execute_tool', 'gen_ai.tool.name': 'getProject' })
    const transport = { ...span('luna_api.tool.execute', 2), parentSpanId: logical.spanId, kind: 'client' as const }
    const request = { ...span('HTTP POST', 3), parentSpanId: transport.spanId, kind: 'client' as const }
    expect(filterAgentTurnTimelineSpans([logical, transport], 'tool').map(item => item.name)).toEqual(['execute_tool getProject'])
    expect(filterAgentTurnTimelineSpans([logical, transport], 'all', true).map(item => item.name)).toEqual(['execute_tool getProject'])
    expect(filterAgentTraceDisplaySpans([logical, transport, request]).map(item => [item.name, item.parentSpanId])).toEqual([
      ['execute_tool getProject', ''],
      ['HTTP POST', logical.spanId],
    ])
  })
})
