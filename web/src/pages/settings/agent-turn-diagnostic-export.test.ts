import type { AgentObservabilityTraceDetail, AgentObservabilityTraceSpan, AgentObservabilityTurn } from '@/api'
import { describe, expect, it } from 'vitest'
import { agentTurnDiagnosticFilename, buildAgentTurnDiagnosticExport, formatAgentTurnDiagnosticExport } from './agent-turn-diagnostic-export'

const turn: AgentObservabilityTurn = {
  id: 'turn/diagnostic 1',
  conversationId: 'conversation-1',
  conversationTitle: 'Deploy service',
  user: { id: 'user-1', name: 'User', email: 'user@example.com', avatarUrl: '' },
  turnIndex: 1,
  status: 'completed',
  userMessage: 'Deploy the service',
  assistantMessage: 'Deployment completed',
  runId: 'run-1',
  traceId: 'trace-1',
  inputTokens: 10,
  outputTokens: 20,
  toolCallCount: 1,
  durationMs: 50,
  createdAt: '2026-08-13T00:00:00Z',
}

function span(id: string, startTimeUnixNano: string, startOffsetMs: number, events: AgentObservabilityTraceSpan['events'] = []): AgentObservabilityTraceSpan {
  return {
    spanId: id,
    parentSpanId: '',
    name: `agent.${id}`,
    serviceName: 'luna-agent',
    kind: 'internal',
    status: 'ok',
    startTimeUnixNano,
    startOffsetMs,
    durationMs: 5,
    attributes: { 'luna.turn.id': turn.id },
    events,
    raw: { spanId: id, startTimeUnixNano },
  }
}

const detail: AgentObservabilityTraceDetail = {
  traceId: 'trace-1',
  durationMs: 50,
  spanCount: 3,
  errorCount: 0,
  spans: [
    span('tool', '1000000000000000030', 30, [{ name: 'gen_ai.tool.content.input', timeUnixNano: '31', attributes: { 'gen_ai.tool.call.arguments': '{"projectId":"project-1"}' } }]),
    span('root-b', '1000000000000000010', 10),
    span('root-a', '1000000000000000010', 10),
  ],
}

describe('agent turn diagnostic export', () => {
  it('builds a versioned package with every span in stable chronological order', () => {
    const exported = buildAgentTurnDiagnosticExport(turn, detail, '2026-08-13T01:02:03.000Z')

    expect(exported).toMatchObject({
      schemaVersion: 1,
      kind: 'luna-devops.agent-turn-diagnostic',
      exportedAt: '2026-08-13T01:02:03.000Z',
      turn: { id: turn.id, userMessage: turn.userMessage, assistantMessage: turn.assistantMessage },
      trace: { traceId: 'trace-1', spanCount: 3 },
    })
    expect(exported.spans.map(item => [item.sequence, item.spanId])).toEqual([[1, 'root-a'], [2, 'root-b'], [3, 'tool']])
    expect(exported.spans[2]).toMatchObject({
      content: [{ kind: 'toolArguments', value: { projectId: 'project-1' } }],
      raw: { spanId: 'tool' },
    })
    expect('spans' in exported.trace).toBe(false)
  })

  it('formats portable JSON and uses an identifier-only safe filename', () => {
    const json = formatAgentTurnDiagnosticExport(turn, detail, '2026-08-13T01:02:03.000Z')
    const parsed = JSON.parse(json)
    expect(parsed.turn.id).toBe(turn.id)
    expect(parsed.spans).toHaveLength(3)
    expect(parsed.spans[0].spanId).toBe('root-a')
    expect(json.endsWith('\n')).toBe(true)
    expect(agentTurnDiagnosticFilename(turn, new Date('2026-08-13T01:02:03.456Z'))).toBe('luna-agent-turn-turn-diagnostic-1-2026-08-13T01-02-03-456Z.json')
  })
})
