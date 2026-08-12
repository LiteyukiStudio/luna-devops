import type { AgentObservabilityTraceSpan } from '@/api'
import { describe, expect, it } from 'vitest'
import { agentSpanContentSections, agentSpanMessages } from './agent-span-content'

const baseSpan: AgentObservabilityTraceSpan = {
  spanId: 'model',
  parentSpanId: 'root',
  name: 'agent.model.stream',
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
  it('extracts model messages and tool payloads from trace events', () => {
    const span: AgentObservabilityTraceSpan = {
      ...baseSpan,
      events: [
        { name: 'gen_ai.content.input', timeUnixNano: '1', attributes: { 'gen_ai.input.messages': '{"messages":[{"role":"system","content":"system prompt"},{"role":"user","content":"deploy"}]}' } },
        { name: 'gen_ai.tool.content.input', timeUnixNano: '2', attributes: { 'gen_ai.tool.call.arguments': '{"projectId":"proj_1"}' } },
        { name: 'gen_ai.tool.content.output', timeUnixNano: '3', attributes: { 'gen_ai.tool.call.result': '{"status":200,"body":{"ok":true}}' } },
      ],
    }
    const sections = agentSpanContentSections(span)
    expect(sections.map(section => section.kind)).toEqual(['modelInput', 'toolArguments', 'toolResult'])
    expect(agentSpanMessages(sections[0].value)).toEqual([
      { id: 'system-0', role: 'system', content: 'system prompt' },
      { id: 'user-1', role: 'user', content: 'deploy' },
    ])
    expect(sections[1].value).toEqual({ projectId: 'proj_1' })
    expect(sections[2].value).toEqual({ status: 200, body: { ok: true } })
  })

  it('keeps non-JSON event content readable', () => {
    const span: AgentObservabilityTraceSpan = { ...baseSpan, events: [{ name: 'gen_ai.content.output', timeUnixNano: '1', attributes: { 'gen_ai.output.messages': 'plain response' } }] }
    expect(agentSpanContentSections(span)).toEqual([{ id: 'gen_ai.content.output-0-gen_ai.output.messages', kind: 'modelOutput', value: 'plain response' }])
  })
})
