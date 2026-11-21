import type {
  AgentObservabilityExecutionFlow,
  AgentObservabilityFlowMessage,
  AgentObservabilityFlowNode,
  AgentObservabilityTraceDetail,
  AgentObservabilityTraceSpan,
  AgentObservabilityTurn,
} from '@/api'
import { agentModelOutput, agentSpanContentSections, agentSpanMessageMarkdown, agentSpanMessages } from '@/pages/settings/operations/traces/agent-span-content'
import { filterAgentTraceDisplaySpans, isAgentModelSpan, isCanonicalAgentToolSpan } from './agent-turn-timeline'

export type AgentFlowFilter = 'all' | 'agent' | 'llm' | 'tool' | 'error'
export type AgentTurnExplorerView = 'flow' | 'waterfall'

export type AgentTurnExplorerTarget
  = | { kind: 'turn', turn: AgentObservabilityTurn }
    | { kind: 'trace', traceId: string, focusToolCallId?: string, focusToolName?: string }

export function resolveExecutionFlow(
  detail: AgentObservabilityTraceDetail | undefined,
  fallbackTurn?: { userMessage?: string, assistantMessage?: string } | null,
): AgentObservabilityExecutionFlow {
  if (!detail)
    return { contentCaptured: false, nodes: [] }
  if (detail.flow)
    return detail.flow

  const ordered = [...detail.spans].sort((a, b) => a.startOffsetMs - b.startOffsetMs || a.durationMs - b.durationMs)
  const byId = new Map(ordered.map(span => [span.spanId, span]))
  const semanticKinds = new Map<string, 'agent' | 'llm' | 'tool'>()
  let contentCaptured = false

  for (const span of ordered) {
    if (agentSpanContentSections(span).length > 0)
      contentCaptured = true
    const kind = fallbackSemanticKind(span)
    if (kind)
      semanticKinds.set(span.spanId, kind)
  }

  const semanticSpans = ordered.filter(span => semanticKinds.has(span.spanId))
  const parentById = new Map<string, string>()
  const childrenByParent = new Map<string, AgentObservabilityTraceSpan[]>()

  for (const span of semanticSpans) {
    const visited = new Set<string>([span.spanId])
    let cursor = span.parentSpanId
    let parentId = ''
    while (cursor && !visited.has(cursor)) {
      visited.add(cursor)
      if (semanticKinds.has(cursor)) {
        parentId = cursor
        break
      }
      cursor = byId.get(cursor)?.parentSpanId ?? ''
    }
    parentById.set(span.spanId, parentId)
    childrenByParent.set(parentId, [...(childrenByParent.get(parentId) ?? []), span])
  }

  const nodes: AgentObservabilityFlowNode[] = []
  let assistantStep = 0
  const visitedWalk = new Set<string>()

  const visit = (span: AgentObservabilityTraceSpan, depth: number) => {
    if (visitedWalk.has(span.spanId))
      return
    visitedWalk.add(span.spanId)
    const kind = semanticKinds.get(span.spanId)!
    const purpose = span.attributes['luna.gen_ai.request.purpose']?.trim() || undefined
    const isAssistant = kind === 'llm' && (!purpose || purpose === 'assistant')
    if (isAssistant)
      assistantStep += 1

    const sections = agentSpanContentSections(span)
    const modelIn = sections.find(s => s.kind === 'modelInput')
    const modelOut = sections.find(s => s.kind === 'modelOutput')
    const toolArgs = sections.find(s => s.kind === 'toolArguments')
    const toolRes = sections.find(s => s.kind === 'toolResult')

    const parsedMsgs = modelIn ? agentSpanMessages(modelIn.value).filter(m => m.role !== 'system') : []
    const lastInput = parsedMsgs.at(-1)
    const parsedOut = modelOut ? agentModelOutput(modelOut.value) : undefined

    const inputTokens = parseTokenNumber(span.attributes['gen_ai.usage.input_tokens'])
    const outputTokens = parseTokenNumber(span.attributes['gen_ai.usage.output_tokens'])
    const cacheReadInputTokens = parseTokenNumber(span.attributes['gen_ai.usage.cache_read.input_tokens'])
    const reasoningOutputTokens = parseTokenNumber(span.attributes['gen_ai.usage.reasoning.output_tokens'])

    const deltaMessages: AgentObservabilityFlowMessage[] | undefined = lastInput
      ? [{
          role: normalizeFlowRole(lastInput.role),
          text: agentSpanMessageMarkdown(lastInput.content),
        }]
      : (isAssistant && assistantStep === 1 && fallbackTurn?.userMessage)
          ? [{ role: 'user', text: fallbackTurn.userMessage }]
          : undefined

    const outputMessage: AgentObservabilityFlowMessage | undefined = parsedOut && (parsedOut.text || parsedOut.reasoningSummary || parsedOut.toolCalls)
      ? {
          role: 'assistant',
          ...(parsedOut.text ? { text: parsedOut.text } : {}),
          ...(parsedOut.reasoningSummary ? { reasoning: parsedOut.reasoningSummary } : {}),
        }
      : undefined

    nodes.push({
      spanId: span.spanId,
      parentNodeId: parentById.get(span.spanId) ?? '',
      depth,
      kind,
      ...(isAssistant ? { stepIndex: assistantStep } : {}),
      ...(purpose ? { purpose } : {}),
      name: fallbackPrimaryName(span, kind),
      spanName: span.name,
      status: span.status,
      startTimeUnixNano: span.startTimeUnixNano,
      startOffsetMs: span.startOffsetMs,
      durationMs: span.durationMs,
      ...(inputTokens !== undefined ? { inputTokens } : {}),
      ...(outputTokens !== undefined ? { outputTokens } : {}),
      ...(cacheReadInputTokens !== undefined ? { cacheReadInputTokens } : {}),
      ...(reasoningOutputTokens !== undefined ? { reasoningOutputTokens } : {}),
      compacted: span.attributes['gen_ai.conversation.compacted'] === 'true',
      inputPreview: kind === 'agent' && fallbackTurn?.userMessage
        ? { role: 'user', text: singleLine(fallbackTurn.userMessage) }
        : deltaMessages?.[0]?.text
          ? { role: deltaMessages[0].role, text: singleLine(deltaMessages[0].text) }
          : {},
      outputPreview: kind === 'agent' && fallbackTurn?.assistantMessage
        ? { role: 'assistant', text: singleLine(fallbackTurn.assistantMessage) }
        : outputMessage?.text
          ? { role: 'assistant', text: singleLine(outputMessage.text) }
          : {},
      ...(deltaMessages ? { deltaMessages } : {}),
      ...(outputMessage ? { outputMessage } : {}),
      ...(span.attributes['gen_ai.tool.call.id'] || span.attributes['luna.tool_call.id']
        ? { toolCallId: span.attributes['gen_ai.tool.call.id'] || span.attributes['luna.tool_call.id'] }
        : {}),
      ...(isObjectRecord(toolArgs?.value) ? { toolArguments: toolArgs.value } : {}),
      ...(toolRes ? { toolResult: toolRes.value } : {}),
    })

    for (const child of childrenByParent.get(span.spanId) ?? [])
      visit(child, depth + 1)
  }

  for (const root of childrenByParent.get('') ?? [])
    visit(root, 0)
  for (const span of semanticSpans)
    visit(span, 0)

  return { contentCaptured, nodes }
}

export function filterExecutionFlowNodes(
  nodes: AgentObservabilityFlowNode[],
  filter: AgentFlowFilter,
  searchQuery: string,
): AgentObservabilityFlowNode[] {
  const keyword = searchQuery.trim().toLowerCase()
  return nodes.filter((node) => {
    if (filter === 'error' && node.status !== 'error')
      return false
    if (filter === 'agent' && node.kind !== 'agent')
      return false
    if (filter === 'llm' && node.kind !== 'llm')
      return false
    if (filter === 'tool' && node.kind !== 'tool')
      return false
    if (!keyword)
      return true
    const haystack = [
      node.name,
      node.spanName,
      node.inputPreview.text,
      node.outputPreview.text,
      node.outputPreview.reasoning,
      node.outputPreview.errorCode,
      ...(node.inputPreview.toolNames ?? []),
      ...(node.outputPreview.toolNames ?? []),
      ...(node.inputPreview.kvItems?.map(kv => `${kv.key}=${kv.value}`) ?? []),
      ...(node.outputPreview.kvItems?.map(kv => `${kv.key}=${kv.value}`) ?? []),
    ]
      .filter(Boolean)
      .join(' ')
      .toLowerCase()
    return haystack.includes(keyword)
  })
}

export function getCollapsibleWaterfallSpanIds(spans: AgentObservabilityTraceSpan[]) {
  const display = filterAgentTraceDisplaySpans(spans)
  const ids = new Set(display.map(span => span.spanId))
  return new Set(display.filter(span => ids.has(span.parentSpanId)).map(span => span.parentSpanId))
}

export function buildSpanWaterfallRows(spans: AgentObservabilityTraceSpan[], collapsedSpanIds: Set<string>) {
  const display = filterAgentTraceDisplaySpans(spans)
  const children = new Map<string, AgentObservabilityTraceSpan[]>()
  const ids = new Set(display.map(span => span.spanId))
  for (const span of display) {
    const parent = ids.has(span.parentSpanId) ? span.parentSpanId : ''
    children.set(parent, [...(children.get(parent) ?? []), span])
  }
  for (const values of children.values())
    values.sort((a, b) => a.startOffsetMs - b.startOffsetMs)
  const result: Array<{ span: AgentObservabilityTraceSpan, depth: number, hasChildren: boolean }> = []
  const visit = (span: AgentObservabilityTraceSpan, depth: number) => {
    const nested = children.get(span.spanId) ?? []
    result.push({ span, depth, hasChildren: nested.length > 0 })
    if (collapsedSpanIds.has(span.spanId))
      return
    for (const child of nested)
      visit(child, depth + 1)
  }
  for (const root of children.get('') ?? [])
    visit(root, 0)
  return result
}

function fallbackSemanticKind(span: AgentObservabilityTraceSpan): 'agent' | 'llm' | 'tool' | null {
  if (span.attributes['gen_ai.operation.name'] === 'invoke_agent' || span.name === 'agent.run.execute')
    return 'agent'
  if (isAgentModelSpan(span))
    return 'llm'
  if (isCanonicalAgentToolSpan(span))
    return 'tool'
  return null
}

function fallbackPrimaryName(span: AgentObservabilityTraceSpan, kind: 'agent' | 'llm' | 'tool') {
  if (kind === 'agent')
    return span.attributes['gen_ai.agent.name'] || span.name.replace(/^invoke_agent\s+/, '')
  if (kind === 'llm')
    return span.attributes['gen_ai.response.model'] || span.attributes['gen_ai.request.model'] || span.name.replace(/^chat\s+/, '')
  return span.attributes['gen_ai.tool.name'] || span.name.replace(/^execute_tool\s+/, '')
}

function parseTokenNumber(raw: string | undefined): number | undefined {
  if (raw === undefined)
    return undefined
  const num = Number(raw)
  return Number.isFinite(num) && num >= 0 ? num : undefined
}

function normalizeFlowRole(role: string): 'system' | 'user' | 'assistant' | 'tool' {
  if (role === 'system' || role === 'user' || role === 'assistant' || role === 'tool')
    return role
  return 'user'
}

function singleLine(text: string) {
  return text.replace(/\s+/g, ' ').trim().slice(0, 180)
}

function isObjectRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

export function formatByteSize(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0)
    return '0 B'
  if (bytes < 1024)
    return `${bytes} B`
  if (bytes < 1024 * 1024)
    return `${(bytes / 1024).toFixed(1)} KiB`
  return `${(bytes / (1024 * 1024)).toFixed(2)} MiB`
}

export function keyedFlowMessages(messages: AgentObservabilityFlowMessage[]) {
  const counts = new Map<string, number>()
  return messages.map((message) => {
    const base = `${message.role}:${message.toolCallId ?? message.turnIndex ?? ''}:${(message.text ?? message.toolName ?? '').slice(0, 32)}`
    const seen = (counts.get(base) ?? 0) + 1
    counts.set(base, seen)
    return { key: `${base}#${seen}`, message }
  })
}
