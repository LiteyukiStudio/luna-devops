import type { AgentObservabilityTraceSpan } from '@/api'

export type AgentSpanContentKind = 'systemInstructions' | 'modelInput' | 'modelOutput' | 'modelError' | 'toolDefinitions' | 'toolArguments' | 'toolResult'

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
  'gen_ai.system_instructions': 'systemInstructions',
  'gen_ai.input.messages': 'modelInput',
  'gen_ai.output.messages': 'modelOutput',
  'gen_ai.tool.definitions': 'toolDefinitions',
  'luna.gen_ai.response.error_body': 'modelError',
  'gen_ai.response.error_body': 'modelError', // Legacy traces emitted before semantic-convention alignment.
  'gen_ai.tool.call.arguments': 'toolArguments',
  'gen_ai.tool.call.result': 'toolResult',
}

export function agentSpanContentSections(span: AgentObservabilityTraceSpan): AgentSpanContentSection[] {
  const spanSections = Object.entries(span.attributes).flatMap(([key, serialized]) => {
    const kind = contentAttributeKinds[key]
    return kind ? [{ id: `span-${key}`, kind, value: parseSerializedContent(serialized) }] : []
  })
  const eventSections = span.events.flatMap((event, eventIndex) => Object.entries(event.attributes).flatMap(([key, serialized]) => {
    const kind = contentAttributeKinds[key]
    return kind ? [{ id: `${event.name}-${eventIndex}-${key}`, kind, value: parseSerializedContent(serialized) }] : []
  }))
  return [...spanSections, ...eventSections]
}

export function isAgentSpanContentAttribute(key: string) {
  return key in contentAttributeKinds
}

export function agentSpanMessages(value: unknown): AgentSpanMessage[] {
  const messages = Array.isArray(value)
    ? value
    : isRecord(value) && Array.isArray(value.messages)
      ? value.messages
      : undefined
  if (!messages)
    return []
  return messages.flatMap((message, index) => {
    if (!isRecord(message) || typeof message.role !== 'string')
      return []
    const content = Array.isArray(message.parts) ? message.parts : message.content
    return [{ id: `${message.role}-${index}`, role: message.role, content }]
  })
}

export function agentSpanMessageMarkdown(value: unknown): string {
  if (typeof value === 'string')
    return value
  if (Array.isArray(value))
    return value.map(agentSpanMessageMarkdown).filter(Boolean).join('\n\n')
  if (!isRecord(value))
    return ''
  if (value.type === 'tool_call' && typeof value.name === 'string') {
    const argumentsJSON = formatSpanJSON(value.arguments ?? {})
    return `\`${value.name}\`\n\n\`\`\`json\n${argumentsJSON}\n\`\`\``
  }
  if (value.type === 'tool_call_response') {
    const responseJSON = formatSpanJSON(value.response)
    return `\`tool_call_response\`\n\n\`\`\`json\n${responseJSON}\n\`\`\``
  }
  for (const key of ['text', 'content', 'value']) {
    if (typeof value[key] === 'string')
      return value[key]
  }
  return ''
}

export function agentModelOutput(value: unknown): AgentModelOutput {
  if (typeof value === 'string')
    return { text: value }
  if (Array.isArray(value)) {
    const parts = value.flatMap(message => isRecord(message) && Array.isArray(message.parts) ? message.parts : [])
    const text = parts
      .filter(part => isRecord(part) && part.type === 'text' && typeof part.content === 'string')
      .map(part => (part as Record<string, unknown>).content)
      .join('\n\n')
    const reasoningSummary = parts
      .filter(part => isRecord(part) && part.type === 'reasoning' && typeof part.content === 'string')
      .map(part => (part as Record<string, unknown>).content)
      .join('\n\n')
    const toolCalls = parts.filter(part => isRecord(part) && part.type === 'tool_call')
    return {
      ...(text ? { text } : {}),
      ...(reasoningSummary ? { reasoningSummary } : {}),
      ...(toolCalls.length > 0 ? { toolCalls } : {}),
    }
  }
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
