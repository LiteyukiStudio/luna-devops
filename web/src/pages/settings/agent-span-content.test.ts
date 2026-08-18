import type { AgentObservabilityTraceSpan } from '@/api'
import { describe, expect, it } from 'vitest'
import { agentModelOutput, agentSpanContentSections, agentSpanMessageMarkdown, agentSpanMessages } from './agent-span-content'

const baseSpan: AgentObservabilityTraceSpan = {
  spanId: 'model',
  parentSpanId: 'root',
  name: 'chat gpt-5',
  serviceName: 'luna-agent',
  kind: 'internal',
  status: 'ok',
  startTimeUnixNano: '1000000000',
  startOffsetMs: 0,
  durationMs: 10,
  attributes: {},
  raw: {},
  events: [],
}

describe('agent span content', () => {
  it('extracts official JSON-schema content from span attributes', () => {
    const span: AgentObservabilityTraceSpan = {
      ...baseSpan,
      attributes: {
        'gen_ai.input.messages': '[{"role":"system","parts":[{"type":"text","content":"system prompt"}]},{"role":"user","parts":[{"type":"text","content":"deploy"}]}]',
        'gen_ai.tool.call.arguments': '{"projectId":"proj_1"}',
        'gen_ai.tool.call.result': '{"status":200,"body":{"ok":true}}',
      },
    }
    const sections = agentSpanContentSections(span)
    expect(sections.map(section => section.kind)).toEqual(['modelInput', 'toolArguments', 'toolResult'])
    expect(agentSpanMessages(sections[0].value)).toEqual([
      { id: 'system-0', role: 'system', content: [{ type: 'text', content: 'system prompt' }] },
      { id: 'user-1', role: 'user', content: [{ type: 'text', content: 'deploy' }] },
    ])
    expect(sections[1].value).toEqual({ projectId: 'proj_1' })
    expect(sections[2].value).toEqual({ status: 200, body: { ok: true } })
  })

  it('keeps non-JSON event content readable', () => {
    const span: AgentObservabilityTraceSpan = { ...baseSpan, events: [{ name: 'gen_ai.content.output', timeUnixNano: '1', attributes: { 'gen_ai.output.messages': 'plain response' } }] }
    expect(agentSpanContentSections(span)).toEqual([{ id: 'gen_ai.content.output-0-gen_ai.output.messages', kind: 'modelOutput', value: 'plain response' }])
  })

  it('normalizes model messages and output for Markdown presentation', () => {
    expect(agentSpanMessageMarkdown([{ type: 'text', text: 'First' }, { type: 'text', text: '**Second**' }])).toBe('First\n\n**Second**')
    expect(agentSpanMessageMarkdown({ type: 'tool_call', name: 'getProject', arguments: { projectId: 'proj-1' } })).toContain('`getProject`')
    expect(agentSpanMessageMarkdown({ type: 'tool_call_response', response: { ok: true } })).toContain('"ok": true')
    expect(agentModelOutput({ text: '## Done', reasoningSummary: 'Checked dependencies', toolCalls: [{ operationId: 'getProject' }], usage: { inputTokens: 1 } })).toEqual({
      text: '## Done',
      reasoningSummary: 'Checked dependencies',
      toolCalls: [{ operationId: 'getProject' }],
    })
    expect(agentModelOutput([{
      role: 'assistant',
      parts: [
        { type: 'reasoning', content: 'Checked dependencies' },
        { type: 'text', content: '## Done' },
        { type: 'tool_call', id: 'call-1', name: 'getProject', arguments: { projectId: 'proj-1' } },
      ],
      finish_reason: 'tool_call',
    }])).toEqual({
      text: '## Done',
      reasoningSummary: 'Checked dependencies',
      toolCalls: [{ type: 'tool_call', id: 'call-1', name: 'getProject', arguments: { projectId: 'proj-1' } }],
    })
  })
})
