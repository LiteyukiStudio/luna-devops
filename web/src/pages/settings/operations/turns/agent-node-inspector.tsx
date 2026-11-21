import type {
  AgentObservabilityFlowMessage,
  AgentObservabilityFlowNode,
  AgentObservabilityTraceSpan,
} from '@/api'
import { Bot, Braces, ChevronDown, ChevronRight, Database, Minus, Network, Plus, Sparkles, TriangleAlert, Wrench } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { AIMarkdown } from '@/components/common/ai-assistant/markdown'
import { toolDisplayName } from '@/components/common/ai-assistant/tool-display-name'
import { DescriptionListItem } from '@/components/common/description-list-item'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import {
  agentModelOutput,
  agentSpanContentSections,
  agentSpanMessageMarkdown,
  agentSpanMessages,
  agentToolDefinitions,
  formatSpanJSON,
  isAgentSpanContentAttribute,
} from '@/pages/settings/operations/traces/agent-span-content'
import { keyedFlowMessages } from './agent-execution-flow'
import { ObservabilityJsonBlock, ObservabilityJsonValue } from './agent-observability-json'
import { isAgentModelSpan } from './agent-turn-timeline'

type LLMContextMode = 'delta' | 'full'

export function AgentNodeInspector({
  span,
  flowNode,
  contentCaptured,
}: {
  span: AgentObservabilityTraceSpan
  flowNode?: AgentObservabilityFlowNode
  contentCaptured: boolean
}) {
  const { t, i18n } = useTranslation()
  const [contextMode, setContextMode] = useState<LLMContextMode>('delta')
  const [showOmittedPrefix, setShowOmittedPrefix] = useState(false)
  const [selectedSchemaTool, setSelectedSchemaTool] = useState<string | null>(null)

  const attributes = Object.entries(span.attributes)
    .filter(([key]) => !isAgentSpanContentAttribute(key))
  const contentSections = agentSpanContentSections(span)
  const systemSection = contentSections.find(section => section.kind === 'systemInstructions')
  const modelInputSection = contentSections.find(section => section.kind === 'modelInput')
  const modelOutputSection = contentSections.find(section => section.kind === 'modelOutput')
  const modelErrorSection = contentSections.find(section => section.kind === 'modelError')
  const toolDefsSection = contentSections.find(section => section.kind === 'toolDefinitions')

  const fullMessages = modelInputSection ? agentSpanMessages(modelInputSection.value) : []
  const toolDefinitions = toolDefsSection ? agentToolDefinitions(toolDefsSection.value) : []
  const activeToolDef = toolDefinitions.find(item => item.name === selectedSchemaTool) ?? null

  const omittedCount = flowNode?.omittedPrefixCount ?? 0
  const safePrefixSplit = omittedCount > 0 && omittedCount < fullMessages.length ? omittedCount : 0
  const prefixMessages = safePrefixSplit > 0 ? fullMessages.slice(0, safePrefixSplit) : []
  const currentStepMessages = safePrefixSplit > 0 ? fullMessages.slice(safePrefixSplit) : fullMessages

  const deltaMessages = flowNode?.deltaMessages ?? []
  const outputMessage = flowNode?.outputMessage
  const rawOutput = modelOutputSection ? agentModelOutput(modelOutputSection.value) : undefined
  const addedTools = flowNode?.addedTools ?? []
  const removedTools = flowNode?.removedTools ?? []

  const hasLLMDeltaData = deltaMessages.length > 0 || Boolean(outputMessage) || Boolean(modelErrorSection) || addedTools.length > 0 || removedTools.length > 0
  const hasFullContextData = fullMessages.length > 0 || Boolean(systemSection) || toolDefinitions.length > 0

  return (
    <div className="grid gap-4">
      <div className="grid gap-1.5 border-b border-border pb-3">
        <div className="flex flex-wrap items-center gap-2">
          <InspectorIcon flowNode={flowNode} span={span} />
          <h3 className="min-w-0 flex-1 break-all text-sm font-semibold">
            {inspectorTitle(span, flowNode, t)}
          </h3>
          <StatusBadge tone={spanStatusTone(span.status)}>
            {spanStatusLabel(span.status, t)}
          </StatusBadge>
        </div>
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 font-mono text-xs text-muted-foreground">
          <span>{span.name}</span>
          <span>·</span>
          <span>{span.serviceName}</span>
          {flowNode?.finishReason && (
            <>
              <span>·</span>
              <span>{flowNode.finishReason}</span>
            </>
          )}
        </div>
        {flowNode && (flowNode.inputTokens !== undefined || flowNode.outputTokens !== undefined) && (
          <div className="mt-1 flex flex-wrap items-center gap-2 text-xs">
            <span className="rounded-control bg-surface-subtle px-2 py-0.5 font-mono">
              {t('operationsDashboardPage.inputTokens')}
              {': '}
              {formatNumber(flowNode.inputTokens ?? 0, i18n.language)}
            </span>
            <span className="rounded-control bg-surface-subtle px-2 py-0.5 font-mono">
              {t('operationsDashboardPage.outputTokens')}
              {': '}
              {formatNumber(flowNode.outputTokens ?? 0, i18n.language)}
            </span>
            {flowNode.cacheReadInputTokens !== undefined && flowNode.cacheReadInputTokens > 0 && (
              <span className="rounded-control bg-surface-subtle px-2 py-0.5 font-mono text-muted-foreground">
                {t('operationsDashboardPage.cacheReadInputTokens')}
                {': '}
                {formatNumber(flowNode.cacheReadInputTokens, i18n.language)}
              </span>
            )}
          </div>
        )}
      </div>

      {flowNode?.kind === 'llm' && (
        <div className="grid gap-3">
          {hasFullContextData && (
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="flex gap-1 rounded-control bg-surface-subtle p-1">
                <Button size="sm" variant={contextMode === 'delta' ? 'secondary' : 'ghost'} onClick={() => setContextMode('delta')}>
                  {t('operationsDashboardPage.turnDetail.inspector.deltaTab')}
                </Button>
                <Button size="sm" variant={contextMode === 'full' ? 'secondary' : 'ghost'} onClick={() => setContextMode('full')}>
                  {t('operationsDashboardPage.turnDetail.inspector.fullContextTab')}
                </Button>
              </div>
              {flowNode.compacted && (
                <StatusBadge tone="info">{t('operationsDashboardPage.turnDetail.inspector.compactedBadge')}</StatusBadge>
              )}
            </div>
          )}

          {contextMode === 'delta' && (
            <div className="grid gap-3">
              {omittedCount > 0 && (
                <div className="flex items-center justify-between gap-2 rounded-control border border-border bg-surface-subtle/70 px-3 py-2 text-xs text-muted-foreground">
                  <span>{t('operationsDashboardPage.turnDetail.inspector.omittedBaselineHint', { count: omittedCount })}</span>
                  {hasFullContextData && (
                    <button
                      className="shrink-0 font-medium text-primary-text hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                      type="button"
                      onClick={() => {
                        setContextMode('full')
                        setShowOmittedPrefix(true)
                      }}
                    >
                      {t('operationsDashboardPage.turnDetail.inspector.viewFull')}
                    </button>
                  )}
                </div>
              )}

              {(addedTools.length > 0 || removedTools.length > 0) && (
                <div className="grid gap-1.5 rounded-control border border-border bg-surface-subtle/50 p-2.5">
                  <span className="text-xs font-medium text-muted-foreground">{t('operationsDashboardPage.turnDetail.inspector.toolSlotDiff')}</span>
                  <div className="flex flex-wrap gap-1.5">
                    {addedTools.map(tool => (
                      <span key={`add-${tool}`} className="inline-flex items-center gap-1 rounded-control bg-success-subtle px-2 py-0.5 text-xs font-medium text-success">
                        <Plus className="size-3" />
                        {toolDisplayName(t, tool)}
                      </span>
                    ))}
                    {removedTools.map(tool => (
                      <span key={`rm-${tool}`} className="inline-flex items-center gap-1 rounded-control bg-surface px-2 py-0.5 text-xs text-muted-foreground line-through">
                        <Minus className="size-3" />
                        {toolDisplayName(t, tool)}
                      </span>
                    ))}
                  </div>
                </div>
              )}

              {deltaMessages.length > 0 && (
                <section className="grid gap-2">
                  <h4 className="text-xs font-semibold text-muted-foreground">{t('operationsDashboardPage.turnDetail.inspector.stepDeltaInput')}</h4>
                  {keyedFlowMessages(deltaMessages).map(({ key, message }) => (
                    <FlowMessageCard key={key} message={message} />
                  ))}
                </section>
              )}

              {modelErrorSection && (
                <section className="grid gap-1.5">
                  <h4 className="text-xs font-semibold text-danger">{t('operationsDashboardPage.turnDetail.content.modelError')}</h4>
                  {typeof modelErrorSection.value === 'string'
                    ? <MarkdownBox value={modelErrorSection.value} />
                    : <ObservabilityJsonBlock className="max-h-64" value={modelErrorSection.value} />}
                </section>
              )}

              {outputMessage && (outputMessage.reasoning || outputMessage.text || (outputMessage.toolCalls?.length ?? 0) > 0) && (
                <section className="grid gap-2">
                  <h4 className="text-xs font-semibold text-muted-foreground">{t('operationsDashboardPage.turnDetail.inspector.stepModelOutput')}</h4>
                  <div className="grid gap-2.5 rounded-control border border-border bg-surface-subtle/50 p-3">
                    {outputMessage.reasoning && (
                      <MarkdownBox label={t('operationsDashboardPage.turnDetail.reasoningSummary')} value={outputMessage.reasoning} />
                    )}
                    {outputMessage.text && (
                      <MarkdownBox label={t('operationsDashboardPage.assistantMessage')} value={outputMessage.text} />
                    )}
                    {outputMessage.toolCalls && outputMessage.toolCalls.length > 0 && (
                      <div className="grid gap-2">
                        <span className="text-xs font-medium text-muted-foreground">{t('operationsDashboardPage.turnDetail.modelToolCalls')}</span>
                        {outputMessage.toolCalls.map(call => (
                          <div key={call.id || `${call.name}:${JSON.stringify(call.arguments)}`} className="grid gap-1.5 rounded-control border border-border bg-surface p-2.5">
                            <div className="flex flex-wrap items-center justify-between gap-2 text-xs">
                              <span className="inline-flex items-center gap-1.5 font-medium">
                                <Wrench className="size-3.5 text-muted-foreground" />
                                {toolDisplayName(t, call.name)}
                              </span>
                              <code className="font-mono text-muted-foreground">{call.name}</code>
                            </div>
                            <ObservabilityJsonBlock className="max-h-56" value={call.arguments} />
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                </section>
              )}

              {!outputMessage && rawOutput && Boolean(rawOutput.reasoningSummary || rawOutput.text || rawOutput.toolCalls !== undefined) && (
                <section className="grid gap-2">
                  <h4 className="text-xs font-semibold text-muted-foreground">{t('operationsDashboardPage.turnDetail.inspector.stepModelOutput')}</h4>
                  <div className="grid gap-2.5 rounded-control border border-border bg-surface-subtle/50 p-3">
                    {rawOutput.reasoningSummary && <MarkdownBox label={t('operationsDashboardPage.turnDetail.reasoningSummary')} value={rawOutput.reasoningSummary} />}
                    {rawOutput.text && <MarkdownBox label={t('operationsDashboardPage.assistantMessage')} value={rawOutput.text} />}
                    {rawOutput.toolCalls !== undefined && <ObservabilityJsonBlock className="max-h-64" value={rawOutput.toolCalls} />}
                  </div>
                </section>
              )}

              {!hasLLMDeltaData && !contentCaptured && (
                <p className="m-0 rounded-control bg-info-subtle px-3 py-2 text-xs text-info">
                  {t('operationsDashboardPage.turnDetail.contentCaptureUnavailable')}
                </p>
              )}
            </div>
          )}

          {contextMode === 'full' && (
            <div className="grid gap-3">
              {systemSection && (
                <details className="group overflow-hidden rounded-control border border-border bg-surface-subtle/50">
                  <summary className="flex cursor-pointer list-none items-center gap-2 px-3 py-2 text-xs font-medium outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring [&::-webkit-details-marker]:hidden">
                    <ChevronRight className="size-3.5 text-muted-foreground transition-transform group-open:rotate-90" />
                    <Sparkles className="size-3.5 text-muted-foreground" />
                    <span>{t('operationsDashboardPage.turnDetail.content.systemInstructions')}</span>
                  </summary>
                  <div className="border-t border-border bg-surface p-3 max-h-72 overflow-y-auto">
                    <AIMarkdown className="text-sm leading-6">
                      {agentSpanMessageMarkdown(systemSection.value) || formatSpanJSON(systemSection.value)}
                    </AIMarkdown>
                  </div>
                </details>
              )}

              {toolDefinitions.length > 0 && (
                <details className="group overflow-hidden rounded-control border border-border bg-surface-subtle/50">
                  <summary className="flex cursor-pointer list-none items-center gap-2 px-3 py-2 text-xs font-medium outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring [&::-webkit-details-marker]:hidden">
                    <ChevronRight className="size-3.5 text-muted-foreground transition-transform group-open:rotate-90" />
                    <Wrench className="size-3.5 text-muted-foreground" />
                    <span>{t('operationsDashboardPage.turnDetail.inspector.availableToolsCount', { count: toolDefinitions.length })}</span>
                  </summary>
                  <div className="grid gap-2.5 border-t border-border bg-surface p-3">
                    <p className="m-0 text-xs text-muted-foreground">{t('operationsDashboardPage.turnDetail.inspector.selectToolSchemaHint')}</p>
                    <div className="flex flex-wrap gap-1.5">
                      {toolDefinitions.map(tool => (
                        <button
                          key={tool.name}
                          aria-pressed={selectedSchemaTool === tool.name}
                          className={cn(
                            'rounded-control border px-2 py-1 text-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                            selectedSchemaTool === tool.name
                              ? 'border-primary bg-primary-subtle font-medium text-primary-text'
                              : 'border-border bg-surface-subtle text-foreground hover:bg-surface-inset',
                          )}
                          type="button"
                          onClick={() => setSelectedSchemaTool(curr => curr === tool.name ? null : tool.name)}
                        >
                          {toolDisplayName(t, tool.name)}
                        </button>
                      ))}
                    </div>
                    {activeToolDef && (
                      <div className="grid gap-1.5 rounded-control border border-border bg-surface-subtle p-2.5">
                        <div className="flex items-center justify-between gap-2 text-xs">
                          <strong className="font-medium">{toolDisplayName(t, activeToolDef.name)}</strong>
                          <code className="font-mono text-muted-foreground">{activeToolDef.name}</code>
                        </div>
                        {activeToolDef.description && <p className="m-0 text-xs text-muted-foreground">{activeToolDef.description}</p>}
                        {activeToolDef.parameters !== undefined && <ObservabilityJsonBlock className="max-h-60" value={activeToolDef.parameters} />}
                      </div>
                    )}
                  </div>
                </details>
              )}

              {fullMessages.length > 0 && (
                <div className="grid gap-2">
                  <h4 className="text-xs font-semibold text-muted-foreground">{t('operationsDashboardPage.turnDetail.content.modelInput')}</h4>
                  {prefixMessages.length > 0 && (
                    <button
                      aria-expanded={showOmittedPrefix}
                      className="flex items-center justify-center gap-1.5 rounded-control border border-dashed border-border bg-surface-subtle/60 px-3 py-2 text-xs font-medium text-muted-foreground transition-colors hover:bg-surface-subtle hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                      type="button"
                      onClick={() => setShowOmittedPrefix(prev => !prev)}
                    >
                      {showOmittedPrefix ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
                      {showOmittedPrefix
                        ? t('operationsDashboardPage.turnDetail.inspector.collapsePrefixMessages', { count: prefixMessages.length })
                        : t('operationsDashboardPage.turnDetail.inspector.expandPrefixMessages', { count: prefixMessages.length })}
                    </button>
                  )}
                  {showOmittedPrefix && prefixMessages.map(msg => (
                    <RawSpanMessageCard key={msg.id} message={msg} muted />
                  ))}
                  {currentStepMessages.map(msg => (
                    <RawSpanMessageCard key={msg.id} message={msg} />
                  ))}
                </div>
              )}
            </div>
          )}
        </div>
      )}

      {flowNode?.kind === 'tool' && (
        <div className="grid gap-3">
          {flowNode.outputPreview.errorCode && (
            <div className="flex items-center gap-2 rounded-control bg-danger-subtle px-3 py-2 text-xs font-medium text-danger">
              <TriangleAlert className="size-4 shrink-0" />
              <span className="break-all font-mono">{flowNode.outputPreview.errorCode}</span>
            </div>
          )}
          {flowNode.toolArguments !== undefined && (
            <section className="grid gap-1.5">
              <h4 className="text-xs font-semibold text-muted-foreground">{t('operationsDashboardPage.turnDetail.content.toolArguments')}</h4>
              <ObservabilityJsonBlock className="max-h-64" value={flowNode.toolArguments} />
            </section>
          )}
          {flowNode.toolResult !== undefined && (
            <section className="grid gap-1.5">
              <h4 className="text-xs font-semibold text-muted-foreground">{t('operationsDashboardPage.turnDetail.content.toolResult')}</h4>
              <ObservabilityJsonBlock className="max-h-72" value={flowNode.toolResult} />
            </section>
          )}
          {flowNode.toolArguments === undefined && flowNode.toolResult === undefined && !contentCaptured && (
            <p className="m-0 rounded-control bg-info-subtle px-3 py-2 text-xs text-info">
              {t('operationsDashboardPage.turnDetail.contentCaptureUnavailable')}
            </p>
          )}
        </div>
      )}

      {flowNode?.kind === 'agent' && (flowNode.inputPreview.text || flowNode.outputPreview.text) && (
        <div className="grid gap-2.5">
          {flowNode.inputPreview.text && (
            <MarkdownBox label={t('operationsDashboardPage.userMessage')} value={flowNode.inputPreview.text} />
          )}
          {flowNode.outputPreview.text && (
            <MarkdownBox label={t('operationsDashboardPage.assistantMessage')} value={flowNode.outputPreview.text} />
          )}
        </div>
      )}

      <div className="grid gap-3 border-t border-border pt-3">
        <dl className="grid grid-cols-2 gap-3 text-sm [&_[data-slot=description-list-value]]:break-all">
          <DescriptionListItem label={t('operationsDashboardPage.status')} value={<StatusBadge tone={spanStatusTone(span.status)}>{spanStatusLabel(span.status, t)}</StatusBadge>} />
          <DescriptionListItem label={t('operationsDashboardPage.duration')} value={formatDuration(span.durationMs)} />
          <DescriptionListItem label={t('operationsDashboardPage.turnDetail.spanId')} value={span.spanId} mono />
          <DescriptionListItem label={t('operationsDashboardPage.turnDetail.parentSpan')} value={span.parentSpanId || '—'} mono />
        </dl>

        {attributes.length > 0 && (
          <div className="grid gap-2">
            <p className="m-0 text-xs font-medium text-muted-foreground">{t('operationsDashboardPage.turnDetail.attributes')}</p>
            <div className="grid gap-1.5 sm:grid-cols-2">
              {attributes.map(([key, value]) => (
                <div key={key} className="grid min-w-0 gap-0.5 rounded-control bg-surface-subtle p-2">
                  <span className="truncate text-xs font-medium text-muted-foreground" title={key}>{spanAttributeLabel(key, t)}</span>
                  <ObservabilityJsonValue>{spanAttributeValue(key, value, t, i18n.language)}</ObservabilityJsonValue>
                </div>
              ))}
            </div>
          </div>
        )}

        <details className="group overflow-hidden rounded-control bg-surface-subtle">
          <summary className="flex cursor-pointer list-none items-center gap-2 px-3 py-2 text-xs font-medium outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring [&::-webkit-details-marker]:hidden">
            <ChevronRight className="size-3.5 text-muted-foreground transition-transform group-open:rotate-90" />
            {t('operationsDashboardPage.turnDetail.rawJson')}
          </summary>
          <div className="border-t border-border p-3">
            <ObservabilityJsonBlock className="max-h-80" value={span.raw} />
          </div>
        </details>
      </div>
    </div>
  )
}

function FlowMessageCard({ message }: { message: AgentObservabilityFlowMessage }) {
  const { t } = useTranslation()
  if (message.role === 'tool') {
    return (
      <div className="grid min-w-0 gap-1.5 rounded-control border border-border bg-surface p-2.5">
        <div className="flex flex-wrap items-center justify-between gap-2 text-xs">
          <span className="inline-flex items-center gap-1.5 font-semibold text-foreground">
            <Wrench className="size-3.5 text-muted-foreground" />
            {message.toolName ? toolDisplayName(t, message.toolName) : t('operationsDashboardPage.turnDetail.roles.tool')}
          </span>
          {message.toolName && <code className="font-mono text-muted-foreground">{message.toolName}</code>}
        </div>
        {message.toolResult !== undefined
          ? <ObservabilityJsonBlock className="max-h-60" value={message.toolResult} />
          : message.text && <AIMarkdown className="max-h-60 overflow-y-auto text-sm leading-6">{message.text}</AIMarkdown>}
      </div>
    )
  }

  return (
    <div className="grid min-w-0 gap-1.5 rounded-control border border-border bg-surface p-3">
      <span className="text-xs font-semibold text-muted-foreground">
        {roleLabel(message.role, t)}
      </span>
      {message.text
        ? <AIMarkdown className="max-h-72 overflow-y-auto text-sm leading-6">{message.text}</AIMarkdown>
        : <span className="text-xs text-muted-foreground">{t('operationsDashboardPage.turnDetail.noReadableModelContent')}</span>}
    </div>
  )
}

function RawSpanMessageCard({ message, muted = false }: { message: { id: string, role: string, content: unknown }, muted?: boolean }) {
  const { t } = useTranslation()
  return (
    <div className={cn('grid min-w-0 gap-1.5 rounded-control border border-border p-3', muted ? 'bg-surface-subtle/50 opacity-85' : 'bg-surface')}>
      <span className="text-xs font-semibold text-muted-foreground">
        {roleLabel(message.role, t)}
      </span>
      <AIMarkdown className="max-h-64 overflow-y-auto text-sm leading-6">
        {agentSpanMessageMarkdown(message.content) || t('operationsDashboardPage.turnDetail.noReadableModelContent')}
      </AIMarkdown>
    </div>
  )
}

function MarkdownBox({ label, value }: { label?: string, value: string }) {
  return (
    <div className="grid min-w-0 gap-1 rounded-control bg-surface p-2.5 border border-border">
      {label && <span className="text-xs font-medium text-muted-foreground">{label}</span>}
      <AIMarkdown className="max-h-72 overflow-y-auto text-sm leading-6">{value}</AIMarkdown>
    </div>
  )
}

function InspectorIcon({ span, flowNode }: { span: AgentObservabilityTraceSpan, flowNode?: AgentObservabilityFlowNode }) {
  const errorClass = span.status === 'error' ? 'text-danger' : 'text-muted-foreground'
  if (flowNode?.kind === 'agent')
    return <Sparkles className={cn('size-4 shrink-0', errorClass)} />
  if (flowNode?.kind === 'llm' || isAgentModelSpan(span))
    return <Bot className={cn('size-4 shrink-0', errorClass)} />
  if (flowNode?.kind === 'tool' || Boolean(span.attributes['gen_ai.tool.name']))
    return <Wrench className={cn('size-4 shrink-0', errorClass)} />
  if (span.name.includes('db.') || span.name.includes('postgres'))
    return <Database className={cn('size-4 shrink-0', errorClass)} />
  if (span.kind === 'client' || span.kind === 'server')
    return <Network className={cn('size-4 shrink-0', errorClass)} />
  return <Braces className={cn('size-4 shrink-0', errorClass)} />
}

function inspectorTitle(
  span: AgentObservabilityTraceSpan,
  flowNode: AgentObservabilityFlowNode | undefined,
  t: (key: string, options?: Record<string, unknown>) => string,
) {
  if (flowNode?.kind === 'agent')
    return t('operationsDashboardPage.turnDetail.steps.agentStarted')
  if (flowNode?.kind === 'llm') {
    if (flowNode.purpose === 'summary')
      return `${t('operationsDashboardPage.turnDetail.inspector.purposeSummary')} · ${flowNode.name}`
    if (flowNode.purpose === 'title')
      return `${t('operationsDashboardPage.turnDetail.inspector.purposeTitle')} · ${flowNode.name}`
    return flowNode.stepIndex
      ? `${t('operationsDashboardPage.turnDetail.inspector.llmStepTitle', { index: flowNode.stepIndex })} · ${flowNode.name}`
      : `${t('operationsDashboardPage.turnDetail.steps.modelCalled')} · ${flowNode.name}`
  }
  if (flowNode?.kind === 'tool')
    return t('operationsDashboardPage.turnDetail.steps.toolCalled', { name: toolDisplayName(t as never, flowNode.name) })
  return span.name
}

function roleLabel(role: string, t: (key: string, options?: Record<string, unknown>) => string) {
  switch (role) {
    case 'system':
      return t('operationsDashboardPage.turnDetail.roles.system')
    case 'user':
      return t('operationsDashboardPage.turnDetail.roles.user')
    case 'assistant':
      return t('operationsDashboardPage.turnDetail.roles.assistant')
    case 'tool':
      return t('operationsDashboardPage.turnDetail.roles.tool')
    default:
      return role
  }
}

const spanAttributeLabelKeys = {
  'gen_ai.operation.name': 'operation',
  'gen_ai.provider.name': 'provider',
  'gen_ai.agent.name': 'agentName',
  'gen_ai.agent.version': 'agentVersion',
  'gen_ai.request.model': 'requestModel',
  'gen_ai.request.max_tokens': 'maxTokens',
  'gen_ai.request.stream': 'requestStream',
  'gen_ai.request.reasoning.level': 'reasoningLevel',
  'gen_ai.response.id': 'responseId',
  'gen_ai.response.model': 'responseModel',
  'gen_ai.response.finish_reasons': 'finishReasons',
  'gen_ai.response.time_to_first_chunk': 'timeToFirstChunk',
  'gen_ai.usage.input_tokens': 'inputTokens',
  'gen_ai.usage.output_tokens': 'outputTokens',
  'gen_ai.usage.cache_read.input_tokens': 'cacheReadInputTokens',
  'gen_ai.usage.cache_write.input_tokens': 'cacheWriteInputTokens',
  'gen_ai.usage.reasoning.output_tokens': 'reasoningOutputTokens',
  'gen_ai.conversation.compacted': 'conversationCompacted',
  'gen_ai.tool.name': 'toolName',
  'gen_ai.tool.call.id': 'toolCallId',
  'gen_ai.tool.type': 'toolType',
  'gen_ai.conversation.id': 'conversationId',
  'openai.response.service_tier': 'serviceTier',
  'openai.response.system_fingerprint': 'systemFingerprint',
  'luna.turn.id': 'turnId',
  'luna.run.id': 'runId',
  'luna.tool_call.id': 'toolCallId',
  'luna.gen_ai.usage.status': 'usageStatus',
  'luna.gen_ai.usage.unavailable_reason': 'usageUnavailableReason',
  'http.request.method': 'httpMethod',
  'http.response.status_code': 'httpStatus',
  'db.system.name': 'databaseSystem',
  'error.type': 'errorType',
  'luna.run.outcome': 'runOutcome',
} as const

function spanAttributeLabel(key: string, t: (key: string, options?: Record<string, unknown>) => string) {
  const labelKey = spanAttributeLabelKeys[key as keyof typeof spanAttributeLabelKeys]
  return labelKey ? t(`operationsDashboardPage.turnDetail.attributeLabels.${labelKey}`) : key
}

function spanAttributeValue(key: string, value: string, t: (key: string, options?: Record<string, unknown>) => string, language: string) {
  if (key === 'gen_ai.operation.name' || key === 'luna.run.outcome') {
    switch (value) {
      case 'agent':
        return t('operationsDashboardPage.turnDetail.attributeValues.agent')
      case 'invoke_agent':
        return t('operationsDashboardPage.turnDetail.attributeValues.invoke_agent')
      case 'chat':
        return t('operationsDashboardPage.turnDetail.attributeValues.chat')
      case 'execute_tool':
        return t('operationsDashboardPage.turnDetail.attributeValues.execute_tool')
      case 'completed':
        return t('operationsDashboardPage.turnDetail.attributeValues.completed')
      case 'failed':
        return t('operationsDashboardPage.turnDetail.attributeValues.failed')
      case 'canceled':
        return t('operationsDashboardPage.turnDetail.attributeValues.canceled')
      default:
        return value
    }
  }
  if (key === 'luna.gen_ai.usage.status') {
    switch (value) {
      case 'reported':
        return t('operationsDashboardPage.turnDetail.usageStatusValues.reported')
      case 'unavailable':
        return t('operationsDashboardPage.turnDetail.usageStatusValues.unavailable')
      case 'reconciliation_required':
        return t('operationsDashboardPage.turnDetail.usageStatusValues.reconciliation_required')
      default:
        return value
    }
  }
  if (key === 'luna.gen_ai.usage.unavailable_reason') {
    switch (value) {
      case 'missing_usage':
        return t('operationsDashboardPage.turnDetail.usageUnavailableReasonValues.missing_usage')
      case 'invalid_usage':
        return t('operationsDashboardPage.turnDetail.usageUnavailableReasonValues.invalid_usage')
      case 'stream_ended_without_usage':
        return t('operationsDashboardPage.turnDetail.usageUnavailableReasonValues.stream_ended_without_usage')
      case 'request_failed':
        return t('operationsDashboardPage.turnDetail.usageUnavailableReasonValues.request_failed')
      case 'request_outcome_unknown':
        return t('operationsDashboardPage.turnDetail.usageUnavailableReasonValues.request_outcome_unknown')
      case 'hold_deficit':
        return t('operationsDashboardPage.turnDetail.usageUnavailableReasonValues.hold_deficit')
      default:
        return value
    }
  }
  if (key === 'gen_ai.request.stream' || key === 'gen_ai.conversation.compacted')
    return value === 'true' ? t('operationsDashboardPage.turnDetail.booleanValue.true') : t('operationsDashboardPage.turnDetail.booleanValue.false')
  if (key === 'gen_ai.response.time_to_first_chunk') {
    const seconds = Number(value)
    return Number.isFinite(seconds) ? `${new Intl.NumberFormat(language, { maximumFractionDigits: 3 }).format(seconds)} s` : value
  }
  if (
    key === 'gen_ai.usage.input_tokens'
    || key === 'gen_ai.usage.output_tokens'
    || key === 'gen_ai.usage.cache_read.input_tokens'
    || key === 'gen_ai.usage.cache_write.input_tokens'
    || key === 'gen_ai.usage.reasoning.output_tokens'
    || key === 'gen_ai.request.max_tokens'
  ) {
    const number = Number(value)
    return Number.isFinite(number) ? new Intl.NumberFormat(language).format(number) : value
  }
  return value
}

function spanStatusLabel(status: string, t: (key: string, options?: Record<string, unknown>) => string) {
  switch (status) {
    case 'ok':
      return t('operationsDashboardPage.turnDetail.spanStatus.ok')
    case 'error':
      return t('operationsDashboardPage.turnDetail.spanStatus.error')
    case 'unset':
      return t('operationsDashboardPage.turnDetail.spanStatus.unset')
    default:
      return status
  }
}

function spanStatusTone(status: string): 'success' | 'danger' | 'neutral' {
  if (status === 'ok')
    return 'success'
  if (status === 'error')
    return 'danger'
  return 'neutral'
}

function formatNumber(value: number, language: string) {
  return new Intl.NumberFormat(language).format(value)
}

function formatDuration(value: number) {
  if (value <= 0)
    return '0 ms'
  return value < 1 ? `${Math.round(value * 1000)} μs` : value < 1000 ? `${value.toFixed(value < 10 ? 2 : 1)} ms` : `${(value / 1000).toFixed(2)} s`
}
