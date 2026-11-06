import type { AIBlock } from './state'
import type { AIToolStatus, AIUIAction } from '@/api'
import type { StatusTone } from '@/components/common/status-tone'
import { Check, ChevronRight, CircleAlert, CircleDashed, CircleStop, LoaderCircle, Minus, ShieldAlert } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { StatusBadge } from '@/components/common/status-badge'
import { StatusDot } from '@/components/common/status-dot'
import { formatMillisecondsDuration } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { isAIUIActionRepeatable } from './actions'
import { runFailureTranslationKey } from './errors'
import { AIToolCallDetails } from './tool-call-details'
import { toolDisplayName } from './tool-display-name'

export type ToolCallBlock = Extract<AIBlock, { type: 'tool_call' }>
export type AIApprovalDecision = 'reject' | 'approve'

const statusTone: Record<AIToolStatus, StatusTone> = {
  proposed: 'neutral',
  awaiting_approval: 'warning',
  running: 'info',
  succeeded: 'success',
  failed: 'danger',
  rejected: 'neutral',
  canceled: 'neutral',
  skipped: 'neutral',
}

export function AIToolCallCard({ block, onAction, onApproval, surface = 'window' }: { block: ToolCallBlock, onAction: (action: AIUIAction) => Promise<boolean>, onApproval: (block: ToolCallBlock, decision: AIApprovalDecision) => Promise<void>, surface?: 'page' | 'window' }) {
  const { t, i18n } = useTranslation()
  const page = surface === 'page'
  const title = block.titleKey && i18n.exists(block.titleKey) ? t(block.titleKey) : toolDisplayName(t, block.operationId)
  const errorCode = block.errorCode ?? block.result?.errorCode
  const summary = block.status === 'failed'
    ? t(runFailureTranslationKey(errorCode))
    : block.result?.summaryKey && i18n.exists(block.result.summaryKey)
      ? t(block.result.summaryKey, block.result.summaryParams)
      : t('aiAssistant.resultAvailable')
  return (
    <div
      className={cn(
        'min-w-0 overflow-hidden rounded-control border bg-surface',
        block.status === 'failed' ? 'border-danger-border' : block.status === 'awaiting_approval' ? 'border-warning-border' : 'border-border',
      )}
      data-ai-tool-call={block.status}
    >
      <details className="group/tool">
        <summary className={cn('relative flex cursor-pointer list-none items-center gap-2 outline-none transition-colors hover:bg-surface-subtle focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring [&::-webkit-details-marker]:hidden', page ? 'min-h-11 px-3 py-1.5 pointer-fine:min-h-9' : 'min-h-9 px-2.5 py-1.5')} data-ai-tool-summary data-ai-tool-surface={surface}>
          <span className={cn('grid size-5 shrink-0 place-items-center rounded-md', block.status === 'failed' ? 'bg-danger-subtle' : 'bg-muted')}>
            <ToolStatusIcon status={block.status} />
          </span>
          <strong className={cn('min-w-0 flex-1 truncate font-medium', page ? 'text-sm' : 'text-xs')}>{title}</strong>
          {block.visibility === 'internal' && (
            <StatusBadge className="shrink-0 px-1.5 py-0 text-[11px] leading-4" tone="warning">
              {t('aiAssistant.toolDebug.internal')}
            </StatusBadge>
          )}
          <span className="flex shrink-0 items-center gap-1.5 text-xs text-muted-foreground">
            <StatusDot className="text-xs [&_[data-slot=status-dot-indicator]]:size-1.5" label={t(`aiAssistant.status.${block.status}`)} tone={statusTone[block.status]} />
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
          <AIToolCallDetails block={block} errorCode={errorCode} summary={summary} />
        </div>
        {block.uiActions.length > 0 && (
          <div className="flex flex-wrap justify-end gap-2 border-t border-border px-3 py-2.5">
            {block.uiActions.map(action => <ActionButton key={`${action.type}-${JSON.stringify(action.payload)}`} action={action} surface={surface} onAction={onAction} />)}
          </div>
        )}
      </details>
      {block.status === 'awaiting_approval' && (
        <div className="border-t border-warning-border" data-ai-tool-intervention>
          <ApprovalControls block={block} surface={surface} onApproval={onApproval} />
        </div>
      )}
    </div>
  )
}

function ToolStatusIcon({ status }: { status: AIToolStatus }) {
  const className = 'size-3'
  if (status === 'running')
    return <LoaderCircle aria-hidden="true" className={cn(className, 'animate-spin text-info motion-reduce:animate-none')} data-ai-tool-status-icon={status} />
  if (status === 'succeeded')
    return <Check aria-hidden="true" className={cn(className, 'text-success')} data-ai-tool-status-icon={status} />
  if (status === 'failed')
    return <CircleAlert aria-hidden="true" className={cn(className, 'text-danger')} data-ai-tool-status-icon={status} />
  if (status === 'rejected' || status === 'skipped')
    return <Minus aria-hidden="true" className={cn(className, 'text-muted-foreground')} data-ai-tool-status-icon={status} />
  if (status === 'canceled')
    return <CircleStop aria-hidden="true" className={cn(className, 'text-muted-foreground')} data-ai-tool-status-icon={status} />
  if (status === 'awaiting_approval')
    return <ShieldAlert aria-hidden="true" className={cn(className, 'text-warning')} data-ai-tool-status-icon={status} />
  return <CircleDashed aria-hidden="true" className={cn(className, 'text-muted-foreground')} data-ai-tool-status-icon={status} />
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

function ApprovalControls({ block, onApproval, surface }: { block: ToolCallBlock, onApproval: (block: ToolCallBlock, decision: AIApprovalDecision) => Promise<void>, surface: 'page' | 'window' }) {
  const { t } = useTranslation()
  const page = surface === 'page'
  const [pending, setPending] = useState(false)
  const decide = async (decision: AIApprovalDecision) => {
    try {
      setPending(true)
      await onApproval(block, decision)
    }
    catch (error) {
      toast.error(error instanceof Error ? error.message : t('aiAssistant.errors.approval'))
    }
    finally {
      setPending(false)
    }
  }
  return (
    <div className="grid gap-2.5 bg-warning-subtle/60 px-3 py-3">
      <div className="flex items-start gap-2">
        <ShieldAlert aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-warning" />
        <div className="grid min-w-0 gap-0.5">
          <strong className="text-sm font-medium text-foreground">{t('aiAssistant.approval.title')}</strong>
          <p className="text-xs leading-5 text-muted-foreground">{t('aiAssistant.approval.bindingHint')}</p>
        </div>
      </div>
      <div className={page ? 'grid grid-cols-2 gap-2 pointer-fine:flex pointer-fine:justify-end' : 'flex flex-wrap justify-end gap-2'}>
        <Button className={page ? 'min-h-12 w-full pointer-fine:min-h-8 pointer-fine:w-auto' : 'h-8 px-3 text-xs'} disabled={pending} size="sm" variant="outline" onClick={() => void decide('reject')}>{t('aiAssistant.approval.reject')}</Button>
        <Button className={page ? 'min-h-12 w-full pointer-fine:min-h-8 pointer-fine:w-auto' : 'h-8 px-3 text-xs'} disabled={pending} size="sm" onClick={() => void decide('approve')}>
          {pending && <LoaderCircle className="animate-spin motion-reduce:animate-none" />}
          {t('aiAssistant.approval.approve')}
        </Button>
      </div>
    </div>
  )
}
