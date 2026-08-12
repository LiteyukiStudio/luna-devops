import type { AgentObservabilityTraceSpan } from '@/api'

export type AgentSpanContentKind = 'modelInput' | 'modelOutput' | 'modelError' | 'toolArguments' | 'toolResult'

export interface AgentSpanContentSection {
  id: string
  kind: AgentSpanContentKind
  value: unknown
}

export interface AgentSpanMessage {
  id: string
  role: string
  content: unknown
}

export interface AgentModelOutput {
  reasoningSummary?: string
  text?: string
  toolCalls?: unknown
}

const contentAttributeKinds: Record<string, AgentSpanContentKind> = {
  'gen_ai.input.messages': 'modelInput',
  'gen_ai.output.messages': 'modelOutput',
  'gen_ai.response.error_body': 'modelError',
  'gen_ai.tool.call.arguments': 'toolArguments',
  'gen_ai.tool.call.result': 'toolResult',
}

export function agentSpanContentSections(span: AgentObservabilityTraceSpan): AgentSpanContentSection[] {
  return span.events.flatMap((event, eventIndex) => Object.entries(event.attributes).flatMap(([key, serialized]) => {
    const kind = contentAttributeKinds[key]
    return kind ? [{ id: `${event.name}-${eventIndex}-${key}`, kind, value: parseSerializedContent(serialized) }] : []
  }))
}

export function agentSpanMessages(value: unknown): AgentSpanMessage[] {
  if (!isRecord(value) || !Array.isArray(value.messages))
    return []
  return value.messages.flatMap((message, index) => {
    if (!isRecord(message) || typeof message.role !== 'string')
      return []
    return [{ id: `${message.role}-${index}`, role: message.role, content: message.content }]
  })
}

export function agentSpanMessageMarkdown(value: unknown): string {
  if (typeof value === 'string')
    return value
  if (Array.isArray(value))
    return value.map(agentSpanMessageMarkdown).filter(Boolean).join('\n\n')
  if (!isRecord(value))
    return ''
  for (const key of ['text', 'content', 'value']) {
    if (typeof value[key] === 'string')
      return value[key]
  }
  return ''
}

export function agentModelOutput(value: unknown): AgentModelOutput {
  if (typeof value === 'string')
    return { text: value }
  if (!isRecord(value))
    return {}
  const text = firstString(value, ['text', 'content', 'output', 'message'])
  const reasoningSummary = firstString(value, ['reasoningSummary', 'reasoning', 'thinking'])
  const toolCalls = Array.isArray(value.toolCalls) && value.toolCalls.length > 0 ? value.toolCalls : undefined
  return {
    ...(text ? { text } : {}),
    ...(reasoningSummary ? { reasoningSummary } : {}),
    ...(toolCalls ? { toolCalls } : {}),
  }
}

export function formatSpanJSON(value: unknown) {
  try {
    return JSON.stringify(value, null, 2) ?? String(value)
  }
  catch {
    return String(value)
  }
}

function firstString(value: Record<string, unknown>, keys: string[]): string | undefined {
  return keys.map(key => value[key]).find(item => typeof item === 'string') as string | undefined
}

function parseSerializedContent(value: string): unknown {
  try {
    return JSON.parse(value)
  }
  catch {
    return value
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}
