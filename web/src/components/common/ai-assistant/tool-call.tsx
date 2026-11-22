import type { AIBlock } from './state'
import type { AIUIAction } from '@/api'
import { ChevronRight, LoaderCircle, ShieldAlert } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { ApiError } from '@/api'
import { StatusBadge } from '@/components/common/status-badge'
import { StatusGlyph } from '@/components/common/status-indicator'
import { formatMillisecondsDuration } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { isAIUIActionRepeatable } from './actions'
import {
  extractToolTargetContext,
  formatHumanApprovalValue,
  isOpaqueResourceId,
  resolveApprovalFieldLabel,
  resolveApprovalScopeSummary,
  resolveApprovalWarningText,
} from './approval-presentation'
import { runFailureTranslationKey } from './errors'
import { LiveProgressBlock } from './live-progress-block'
import { aiToolIndicatorState } from './status-indicator-states'
import { AIToolCallDetails } from './tool-call-details'
import { toolDisplayName } from './tool-display-name'

export type ToolCallBlock = Extract<AIBlock, { type: 'tool_call' }>
export type AIApprovalDecision = 'reject' | 'approve' | 'approve_and_remember'

interface WaitForTaskDisplayInfo {
  projectId: string
  taskType: 'build_run' | 'release'
  taskId: string
  taskStatus?: 'pending' | 'running' | 'succeeded' | 'failed' | 'canceled'
  timedOut?: boolean
}

const safeResourceIdentifierPattern = /^[a-z0-9][\w.:-]{0,63}$/i

function extractWaitForTaskInfo(block: ToolCallBlock): WaitForTaskDisplayInfo | undefined {
  if (block.operationId !== 'wait_for_task')
    return undefined
  const projectId = typeof block.arguments.projectId === 'string' ? block.arguments.projectId.trim() : ''
  const taskType = block.arguments.taskType === 'build_run' || block.arguments.taskType === 'release'
    ? block.arguments.taskType
    : undefined
  const taskId = typeof block.arguments.taskId === 'string' ? block.arguments.taskId.trim() : ''
  if (!safeResourceIdentifierPattern.test(projectId) || !taskType || !safeResourceIdentifierPattern.test(taskId))
    return undefined
  const data = block.result?.data && typeof block.result.data === 'object' && !Array.isArray(block.result.data)
    ? block.result.data as Record<string, unknown>
    : undefined
  const rawTaskStatus = data?.taskStatus
  const taskStatus = rawTaskStatus === 'pending'
    || rawTaskStatus === 'running'
    || rawTaskStatus === 'succeeded'
    || rawTaskStatus === 'failed'
    || rawTaskStatus === 'canceled'
    ? rawTaskStatus
    : undefined
  return {
    projectId,
    taskType,
    taskId,
    ...(taskStatus ? { taskStatus } : {}),
    ...(typeof data?.timedOut === 'boolean' ? { timedOut: data.timedOut } : {}),
  }
}

export function AIToolCallCard({ block, defaultOpenTechnical = false, onAction, onApproval, surface = 'window' }: { block: ToolCallBlock, defaultOpenTechnical?: boolean, onAction: (action: AIUIAction) => Promise<boolean>, onApproval: (block: ToolCallBlock, decision: AIApprovalDecision) => Promise<void>, surface?: 'page' | 'window' }) {
  const { t, i18n } = useTranslation()
  const page = surface === 'page'
  const waitTask = extractWaitForTaskInfo(block)
  const waitTargetLabel = waitTask
    ? waitTask.taskType === 'build_run'
      ? t('aiAssistant.waitForTask.targetBuild', { id: waitTask.taskId })
      : t('aiAssistant.waitForTask.targetRelease', { id: waitTask.taskId })
    : undefined
  const baseTitle = block.titleKey && i18n.exists(block.titleKey) ? t(block.titleKey) : toolDisplayName(t, block.operationId)
  const contextTarget = !waitTargetLabel ? extractToolTargetContext(block.arguments, block.approvalPreview?.targetResource) : undefined
  const title = waitTargetLabel
    ? `${baseTitle} · ${waitTargetLabel}`
    : contextTarget
      ? `${baseTitle} · ${contextTarget}`
      : baseTitle
  const errorCode = block.errorCode ?? block.result?.errorCode
  const summary = block.status === 'failed'
    ? t(runFailureTranslationKey(errorCode))
    : block.result?.summaryKey && i18n.exists(block.result.summaryKey)
      ? t(block.result.summaryKey, block.result.summaryParams)
      : t('aiAssistant.resultAvailable')
  const waitOutcomeBadge = waitTask && block.status === 'succeeded'
    ? waitTask.timedOut
      ? { tone: 'warning' as const, label: t('aiAssistant.waitForTask.timedOut') }
      : waitTask.taskStatus === 'failed'
        ? { tone: 'danger' as const, label: t('aiAssistant.cards.liveProgress.states.operation_failed') }
        : waitTask.taskStatus === 'canceled'
          ? { tone: 'warning' as const, label: t('aiAssistant.cards.liveProgress.states.cancelled') }
          : waitTask.taskStatus === 'succeeded'
            ? { tone: 'success' as const, label: t('aiAssistant.cards.liveProgress.states.succeeded') }
            : undefined
    : undefined
  return (
    <div
      className={cn(
        'min-w-0 overflow-hidden rounded-control border bg-surface',
        block.status === 'failed' ? 'border-danger-border' : block.status === 'awaiting_approval' ? 'border-warning-border' : 'border-border',
      )}
      data-ai-tool-call={block.status}
      data-ai-tool-operation={block.operationId}
    >
      <details className="group/tool">
        <summary className={cn('relative flex cursor-pointer list-none items-center gap-2 outline-none transition-colors hover:bg-surface-subtle focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring [&::-webkit-details-marker]:hidden', page ? 'min-h-11 px-3 py-1.5 pointer-fine:min-h-9' : 'min-h-9 px-2.5 py-1.5')} data-ai-tool-summary data-ai-tool-surface={surface}>
          <span className="grid size-5 shrink-0 place-items-center text-sm" data-ai-tool-status-icon={block.status}>
            <StatusGlyph state={aiToolIndicatorState(block.status)} />
          </span>
          <strong className={cn('min-w-0 flex-1 truncate font-medium', page ? 'text-sm' : 'text-xs')}>{title}</strong>
          {block.visibility === 'internal' && (
            <StatusBadge className="shrink-0 px-1.5 py-0 text-[11px] leading-4" tone="warning">
              {t('aiAssistant.toolDebug.internal')}
            </StatusBadge>
          )}
          {waitOutcomeBadge && (
            <StatusBadge className="shrink-0 px-1.5 py-0 text-[11px] leading-4" tone={waitOutcomeBadge.tone}>
              {waitOutcomeBadge.label}
            </StatusBadge>
          )}
          <span className="flex shrink-0 items-center gap-1.5 text-xs text-muted-foreground">
            <span>{t(`aiAssistant.status.${block.status}`)}</span>
            {block.durationMs !== undefined && (
              <span className="hidden tabular-nums sm:contents" data-ai-tool-duration>
                <span aria-hidden="true" className="opacity-60">·</span>
                <span>{formatMillisecondsDuration(block.durationMs, i18n.language)}</span>
              </span>
            )}
          </span>
          <ChevronRight aria-hidden="true" className="size-3.5 shrink-0 text-muted-foreground transition-transform duration-fast group-open/tool:rotate-90" />
          {block.status === 'running' && (
            <span aria-hidden="true" className="absolute inset-x-0 bottom-0 h-0.5 overflow-hidden">
              <span className="block h-full w-1/3 bg-info-solid/70 animate-[ai-live-progress_1.35s_ease-in-out_infinite] motion-reduce:hidden" />
            </span>
          )}
        </summary>
        <div className={page ? 'pointer-coarse:[&_button]:min-h-11 pointer-coarse:[&_button]:min-w-11' : undefined}>
          <AIToolCallDetails block={block} defaultOpenTechnical={defaultOpenTechnical} errorCode={errorCode} summary={summary} />
        </div>
        {block.uiActions.length > 0 && (
          <div className="flex flex-wrap justify-end gap-2 border-t border-border px-3 py-2.5">
            {block.uiActions.map(action => <ActionButton key={`${action.type}-${JSON.stringify(action.payload)}`} action={action} surface={surface} onAction={onAction} />)}
          </div>
        )}
      </details>
      {waitTask && block.status === 'running' && waitTargetLabel && (
        <div className="border-t border-border px-3 py-2.5" data-ai-wait-for-task-progress>
          <LiveProgressBlock
            block={{
              id: `${block.id}:progress`,
              type: 'live_progress',
              label: waitTargetLabel,
              binding: {
                projectId: waitTask.projectId,
                operationType: waitTask.taskType,
                operationId: waitTask.taskId,
              },
            }}
          />
        </div>
      )}
      {block.status === 'awaiting_approval' && (
        <div className="border-t border-warning-border" data-ai-tool-intervention>
          <ApprovalControls block={block} surface={surface} onApproval={onApproval} />
        </div>
      )}
    </div>
  )
}

function ActionButton({ action, onAction, surface }: { action: AIUIAction, onAction: (action: AIUIAction) => Promise<boolean>, surface: 'page' | 'window' }) {
  const { t } = useTranslation()
  const [done, setDone] = useState(false)
  const [pending, setPending] = useState(false)
  const executingRef = useRef(false)
  const repeatable = isAIUIActionRepeatable(action)
  const execute = async () => {
    if (executingRef.current || (done && !repeatable))
      return
    try {
      executingRef.current = true
      setPending(true)
      const success = await onAction(action)
      if (success) {
        if (!repeatable)
          setDone(true)
        return
      }
      toast.error(t('aiAssistant.actions.unavailable'))
    }
    catch (error) {
      toast.error(error instanceof Error ? error.message : t('aiAssistant.actions.unavailable'))
    }
    finally {
      executingRef.current = false
      setPending(false)
    }
  }
  const label = 'label' in action && action.label ? action.label : t(`aiAssistant.actions.${action.type}`)
  const variant = 'tone' in action && action.tone === 'primary' ? 'default' : 'outline'
  return <Button className={surface === 'page' ? 'min-h-11 px-3 pointer-fine:min-h-8' : 'h-8 px-3 text-xs'} disabled={pending || done} size="sm" variant={variant} onClick={() => void execute()}>{done ? t('aiAssistant.actions.opened') : label}</Button>
}

function formatApprovalParameterEntries(args: Record<string, unknown>): Array<{ key: string, value: string }> {
  const entries: Array<{ key: string, value: string }> = []
  const pushEntry = (key: string, raw: unknown) => {
    if (raw === undefined)
      return
    const formatted = typeof raw === 'string'
      ? raw
      : typeof raw === 'number' || typeof raw === 'boolean' || raw === null
        ? String(raw)
        : JSON.stringify(raw)
    entries.push({ key, value: formatted })
  }
  for (const [key, value] of Object.entries(args)) {
    if (key === 'body' && value && typeof value === 'object' && !Array.isArray(value)) {
      for (const [bodyKey, bodyValue] of Object.entries(value as Record<string, unknown>)) {
        pushEntry(`body.${bodyKey}`, bodyValue)
      }
      continue
    }
    pushEntry(key, value)
  }
  return entries
}

function formatPreviewScalar(raw: unknown): string {
  if (raw === undefined)
    return '-'
  if (typeof raw === 'string')
    return raw || '""'
  if (typeof raw === 'number' || typeof raw === 'boolean' || raw === null)
    return String(raw)
  return JSON.stringify(raw)
}

function ApprovalControls({ block, onApproval, surface }: { block: ToolCallBlock, onApproval: (block: ToolCallBlock, decision: AIApprovalDecision) => Promise<void>, surface: 'page' | 'window' }) {
  const { t } = useTranslation()
  const page = surface === 'page'
  const [pendingDecision, setPendingDecision] = useState<AIApprovalDecision | null>(null)
  const decidingRef = useRef(false)
  const pending = pendingDecision !== null
  const isL1 = block.approvalTier === 'L1'
  const rememberable = block.approvalRememberable === true && isL1
  const preview = block.approvalPreview
  const parameterEntries = formatApprovalParameterEntries(block.arguments)
  const hasDiffPreview = Boolean(preview?.changes && preview.changes.length > 0)
  const scopeSummary = resolveApprovalScopeSummary(block.arguments, preview)
  const nonOpaqueEntries = parameterEntries.filter(entry => !isOpaqueResourceId(entry.value))
  const opaqueEntries = parameterEntries.filter(entry => isOpaqueResourceId(entry.value))
  // Keep human settings first, followed by compact target/scope IDs so no resource binding is ever hidden from review.
  const orderedPrimaryEntries = [...nonOpaqueEntries, ...opaqueEntries]
  const hasOpaqueEntries = opaqueEntries.length > 0

  const decide = async (decision: AIApprovalDecision) => {
    if (decidingRef.current)
      return
    try {
      decidingRef.current = true
      setPendingDecision(decision)
      await onApproval(block, decision)
    }
    catch (error) {
      if (error instanceof ApiError && error.code === 'ai.approval_remember_not_allowed') {
        toast.error(t('aiAssistant.approval.rememberNotAllowed'))
      }
      else {
        toast.error(error instanceof Error ? error.message : t('aiAssistant.errors.approval'))
      }
    }
    finally {
      decidingRef.current = false
      setPendingDecision(null)
    }
  }
  return (
    <div className="grid gap-2.5 bg-warning-subtle/50 px-3 py-3" data-ai-approval-tier={isL1 ? 'L1' : 'L2'}>
      <div className="flex items-start gap-2">
        <ShieldAlert aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-warning" />
        <div className="grid min-w-0 flex-1 gap-0.5">
          <div className="flex flex-wrap items-center gap-1.5">
            <strong className="text-sm font-medium text-foreground">
              {isL1 ? t('aiAssistant.approval.reversibleTitle') : t('aiAssistant.approval.title')}
            </strong>
            <StatusBadge className="px-1.5 py-0 text-[11px] leading-4" tone={isL1 ? 'info' : 'warning'}>
              {isL1 ? t('aiAssistant.approval.tierL1') : t('aiAssistant.approval.tierL2')}
            </StatusBadge>
          </div>
          {preview?.summary && (
            <p className="text-xs font-medium leading-5 text-foreground" data-ai-approval-preview-summary>
              {preview.summary}
            </p>
          )}
          <p className="text-xs leading-5 text-muted-foreground">
            {rememberable
              ? t('aiAssistant.approval.rememberHint')
              : isL1
                ? t('aiAssistant.approval.strictHint')
                : t('aiAssistant.approval.bindingHint')}
          </p>
        </div>
      </div>

      {(scopeSummary.project || scopeSummary.target) && (
        <div className="grid gap-1 rounded-control border border-border/70 bg-surface px-2.5 py-1.5 text-xs" data-ai-approval-scope>
          {scopeSummary.project && (
            <div className="flex items-center justify-between gap-2">
              <span className="shrink-0 text-muted-foreground">{t('aiAssistant.tools.resources.project')}</span>
              <span className="min-w-0 truncate font-mono text-[11px] font-medium text-foreground" title={scopeSummary.project.rawTitle}>
                {scopeSummary.project.display}
              </span>
            </div>
          )}
          {scopeSummary.target && (
            <div className="flex items-center justify-between gap-2">
              <span className="shrink-0 text-muted-foreground">{t('aiAssistant.approval.targetLabel')}</span>
              <span className="min-w-0 truncate font-medium text-foreground" title={scopeSummary.target.rawTitle}>
                {scopeSummary.target.display}
              </span>
            </div>
          )}
        </div>
      )}

      {preview?.changes && preview.changes.length > 0 && (
        <div className="grid gap-1.5 rounded-control border border-border/70 bg-surface px-2.5 py-2 text-xs" data-ai-approval-diff>
          <span className="text-[11px] font-medium text-muted-foreground">{t('aiAssistant.approval.changesTitle')}</span>
          <div className="grid max-h-44 gap-1.5 overflow-y-auto">
            {preview.changes.map(change => (
              <div key={`${change.field}-${change.action ?? 'diff'}-${formatPreviewScalar(change.after)}`} className="flex flex-wrap items-baseline justify-between gap-2 text-xs">
                <span className="text-muted-foreground">{resolveApprovalFieldLabel(t, change.field)}</span>
                <span className="min-w-0 break-all text-right font-mono text-[11px] font-medium text-foreground">
                  {change.before !== undefined
                    ? `${formatHumanApprovalValue(t, formatPreviewScalar(change.before))} -> ${formatHumanApprovalValue(t, formatPreviewScalar(change.after))}`
                    : formatHumanApprovalValue(t, formatPreviewScalar(change.after))}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}

      {preview?.sensitiveKeys && preview.sensitiveKeys.length > 0 && (
        <div className="grid gap-0.5 rounded-control border border-border/70 bg-surface px-2.5 py-1.5 text-[11px]" data-ai-approval-sensitive-keys>
          <span className="font-medium text-muted-foreground">{t('aiAssistant.approval.sensitiveKeysTitle')}</span>
          <span className="font-mono text-foreground">{preview.sensitiveKeys.join(', ')}</span>
        </div>
      )}

      {preview?.impact && preview.impact.length > 0 && (
        <div className="grid gap-0.5 rounded-control border border-border/70 bg-surface px-2.5 py-1.5 text-[11px]" data-ai-approval-impact>
          <span className="font-medium text-muted-foreground">{t('aiAssistant.approval.impactTitle')}</span>
          {preview.impact.map(line => (
            <span key={line} className="text-foreground">{line}</span>
          ))}
        </div>
      )}

      {preview?.warnings && preview.warnings.length > 0 && (
        <div className="grid gap-0.5 rounded-control border border-warning/40 bg-warning-subtle/80 px-2.5 py-1.5 text-[11px]" data-ai-approval-warnings>
          <span className="font-medium text-warning">{t('aiAssistant.approval.warningsTitle')}</span>
          {preview.warnings.map(line => (
            <span key={line} className="text-foreground">{resolveApprovalWarningText(t, line)}</span>
          ))}
        </div>
      )}

      {parameterEntries.length > 0 && (
        hasDiffPreview
          ? (
              <details className="group/raw rounded-control border border-border/70 bg-surface text-xs" data-ai-approval-parameters>
                <summary className="flex cursor-pointer list-none items-center justify-between gap-2 px-2.5 py-1.5 text-[11px] font-medium text-muted-foreground outline-none transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring [&::-webkit-details-marker]:hidden">
                  <span>{t('aiAssistant.approval.rawParametersTitle')}</span>
                  <ChevronRight aria-hidden="true" className="size-3.5 shrink-0 transition-transform duration-fast group-open/raw:rotate-90" />
                </summary>
                <dl className="grid max-h-40 gap-1 overflow-y-auto border-t border-border/60 px-2.5 py-2">
                  {parameterEntries.map(entry => (
                    <div key={entry.key} className="flex items-baseline justify-between gap-3">
                      <dt className="shrink-0 font-mono text-[11px] text-muted-foreground">{entry.key}</dt>
                      <dd className="min-w-0 break-all text-right font-mono text-[11px] text-foreground">{entry.value}</dd>
                    </div>
                  ))}
                </dl>
              </details>
            )
          : (
              <div className="grid gap-1.5 rounded-control border border-border/70 bg-surface px-2.5 py-2 text-xs" data-ai-approval-parameters>
                <span className="text-[11px] font-medium text-muted-foreground">{t('aiAssistant.approval.keySettingsTitle')}</span>
                <dl className="grid max-h-44 gap-1.5 overflow-y-auto">
                  {orderedPrimaryEntries.map((entry) => {
                    const label = resolveApprovalFieldLabel(t, entry.key)
                    const opaque = isOpaqueResourceId(entry.value)
                    return (
                      <div key={entry.key} className="flex items-baseline justify-between gap-3">
                        <dt className="shrink-0 text-xs text-muted-foreground">{label}</dt>
                        <dd
                          className={cn(
                            'min-w-0 break-all text-right font-mono text-[11px]',
                            opaque ? 'text-muted-foreground' : 'font-medium text-foreground',
                          )}
                          title={opaque ? entry.value : undefined}
                        >
                          {formatHumanApprovalValue(t, entry.value)}
                        </dd>
                      </div>
                    )
                  })}
                </dl>
                {hasOpaqueEntries && (
                  <details className="group/raw mt-0.5 border-t border-border/60 pt-1.5">
                    <summary className="flex cursor-pointer list-none items-center justify-between gap-2 rounded-sm text-[11px] font-medium text-muted-foreground outline-none transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring [&::-webkit-details-marker]:hidden">
                      <span>{t('aiAssistant.approval.rawParametersTitle')}</span>
                      <ChevronRight aria-hidden="true" className="size-3 shrink-0 transition-transform duration-fast group-open/raw:rotate-90" />
                    </summary>
                    <dl className="mt-1.5 grid max-h-36 gap-1 overflow-y-auto">
                      {parameterEntries.map(entry => (
                        <div key={`raw:${entry.key}`} className="flex items-baseline justify-between gap-3">
                          <dt className="shrink-0 font-mono text-[11px] text-muted-foreground">{entry.key}</dt>
                          <dd className="min-w-0 break-all text-right font-mono text-[11px] text-foreground">{entry.value}</dd>
                        </div>
                      ))}
                    </dl>
                  </details>
                )}
              </div>
            )
      )}

      <div className={page ? 'grid grid-cols-2 gap-2 pointer-fine:flex pointer-fine:flex-wrap pointer-fine:justify-end' : 'flex flex-wrap justify-end gap-2'}>
        <Button className={page ? 'min-h-12 w-full pointer-fine:min-h-8 pointer-fine:w-auto' : 'h-8 px-3 text-xs'} disabled={pending} size="sm" variant="outline" onClick={() => void decide('reject')}>{t('aiAssistant.approval.reject')}</Button>
        {rememberable && (
          <Button className={page ? 'min-h-12 w-full pointer-fine:min-h-8 pointer-fine:w-auto' : 'h-8 px-3 text-xs'} disabled={pending} size="sm" variant="outline" onClick={() => void decide('approve_and_remember')}>
            {pendingDecision === 'approve_and_remember' && <LoaderCircle className="animate-spin motion-reduce:animate-none" />}
            {t('aiAssistant.approval.approveAndRemember')}
          </Button>
        )}
        <Button className={page ? 'min-h-12 w-full pointer-fine:min-h-8 pointer-fine:w-auto' : 'h-8 px-3 text-xs'} disabled={pending} size="sm" onClick={() => void decide('approve')}>
          {pendingDecision === 'approve' && <LoaderCircle className="animate-spin motion-reduce:animate-none" />}
          {t('aiAssistant.approval.approve')}
        </Button>
      </div>
    </div>
  )
}
