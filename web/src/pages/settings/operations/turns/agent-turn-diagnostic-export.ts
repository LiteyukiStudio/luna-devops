import type { AgentObservabilityTraceDetail, AgentObservabilityTraceSpan, AgentObservabilityTurn } from '@/api'
import { agentSpanContentSections } from '@/pages/settings/operations/traces/agent-span-content'

export const agentTurnDiagnosticSchemaVersion = 1

export function buildAgentTurnDiagnosticExport(
  turn: AgentObservabilityTurn,
  detail: AgentObservabilityTraceDetail,
  exportedAt = new Date().toISOString(),
) {
  const { spans: sourceSpans, ...trace } = detail
  const spans = sortAgentTurnDiagnosticSpans(sourceSpans).map((span, index) => ({
    sequence: index + 1,
    spanId: span.spanId,
    parentSpanId: span.parentSpanId,
    name: span.name,
    serviceName: span.serviceName,
    kind: span.kind,
    status: span.status,
    startTimeUnixNano: span.startTimeUnixNano,
    startOffsetMs: span.startOffsetMs,
    durationMs: span.durationMs,
    attributes: span.attributes,
    events: span.events,
    content: agentSpanContentSections(span).map(section => ({ kind: section.kind, value: section.value })),
    raw: span.raw,
  }))

  return {
    schemaVersion: agentTurnDiagnosticSchemaVersion,
    kind: 'luna-devops.agent-turn-diagnostic' as const,
    exportedAt,
    turn: { ...turn },
    trace,
    spans,
  }
}

export function formatAgentTurnDiagnosticExport(turn: AgentObservabilityTurn, detail: AgentObservabilityTraceDetail, exportedAt?: string) {
  return `${JSON.stringify(buildAgentTurnDiagnosticExport(turn, detail, exportedAt), null, 2)}\n`
}

export function agentTurnDiagnosticFilename(turn: AgentObservabilityTurn, exportedAt = new Date()) {
  const timestamp = exportedAt.toISOString().replace(/[:.]/g, '-')
  return `luna-agent-turn-${safeFilenamePart(turn.id)}-${timestamp}.json`
}

export function sortAgentTurnDiagnosticSpans(spans: AgentObservabilityTraceSpan[]) {
  return [...spans].sort((left, right) => {
    const startOrder = compareUnixNano(left.startTimeUnixNano, right.startTimeUnixNano)
    if (startOrder !== 0)
      return startOrder
    if (left.startOffsetMs !== right.startOffsetMs)
      return left.startOffsetMs - right.startOffsetMs
    return left.spanId.localeCompare(right.spanId)
  })
}

function compareUnixNano(left: string, right: string) {
  try {
    const leftValue = BigInt(left)
    const rightValue = BigInt(right)
    return leftValue < rightValue ? -1 : leftValue > rightValue ? 1 : 0
  }
  catch {
    return left.localeCompare(right)
  }
}

function safeFilenamePart(value: string) {
  const safe = value.trim().replace(/[^\w-]+/g, '-').replace(/^-+|-+$/g, '')
  return safe || 'unknown'
}
