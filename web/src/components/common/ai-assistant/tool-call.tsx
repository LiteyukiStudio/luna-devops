import type { AIBlock } from './state'
import type { AIToolStatus, AIUIAction } from '@/api'
import { Check, ChevronRight, CircleAlert, CircleDashed, CircleStop, LoaderCircle, Minus, ShieldAlert } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { formatMillisecondsDuration } from '@/components/common/time-format'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { isAIUIActionRepeatable } from './actions'
import { runFailureTranslationKey } from './errors'
import { AIToolCallDetails } from './tool-call-details'
import { toolDisplayName } from './tool-display-name'

export type ToolCallBlock = Extract<AIBlock, { type: 'tool_call' }>
export type AIApprovalDecision = 'reject' | 'approve' | 'approve_always'

const statusTone: Record<AIToolStatus, string> = {
  proposed: 'bg-surface-inset text-muted-foreground',
  awaiting_approval: 'bg-warning-subtle text-warning',
  running: 'bg-info-subtle text-info',
  succeeded: 'bg-success-subtle text-success',
  failed: 'bg-danger-subtle text-danger',
  rejected: 'bg-surface-inset text-muted-foreground',
  canceled: 'bg-surface-inset text-muted-foreground',
  skipped: 'bg-surface-inset text-muted-foreground',
}

export function AIToolCallCard({ block, onAction, onApproval, surface = 'window' }: { block: ToolCallBlock, onAction: (action: AIUIAction) => Promise<boolean>, onApproval: (block: ToolCallBlock, decision: AIApprovalDecision, reason?: string) => Promise<void>, surface?: 'page' | 'window' }) {
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
    <div className="overflow-hidden rounded-container bg-surface">
      <details className="group">
        <summary className={cn('flex cursor-pointer list-none items-center gap-1.5 outline-none hover:bg-surface-inset focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring [&::-webkit-details-marker]:hidden', page ? 'min-h-11 px-3 py-1.5' : 'min-h-9 px-2 py-1')} data-ai-tool-summary data-ai-tool-surface={surface}>
          <strong className={page ? 'min-w-0 flex-1 truncate text-sm font-medium' : 'min-w-0 flex-1 truncate text-xs font-medium'}>{title}</strong>
          {block.visibility === 'internal' && (
            <Badge className="border-transparent bg-warning-subtle px-1.5 py-0 text-[10px] leading-4 text-warning">
              {t('aiAssistant.toolDebug.internal')}
            </Badge>
          )}
          <Badge className={cn('gap-1 border-transparent px-1.5 py-0 text-[10px] leading-4', statusTone[block.status])}>
            <ToolStatusIcon status={block.status} />
            <span>{t(`aiAssistant.status.${block.status}`)}</span>
            {block.durationMs !== undefined && (
              <span className="hidden sm:contents" data-ai-tool-duration>
                <span aria-hidden="true" className="opacity-60">·</span>
                <span>{formatMillisecondsDuration(block.durationMs, i18n.language)}</span>
              </span>
            )}
          </Badge>
          <ChevronRight className="size-3.5 shrink-0 text-muted-foreground transition-transform group-open:rotate-90" />
        </summary>
        <div className={page ? '[&_button]:min-h-11 [&_button]:min-w-11' : undefined}>
          <AIToolCallDetails block={block} errorCode={errorCode} summary={summary} />
        </div>
        {block.uiActions.length > 0 && (
          <div className="bg-surface-subtle/40 px-3 pb-3">
            <div className="mt-3 flex flex-wrap justify-end gap-2">
              {block.uiActions.map(action => <ActionButton key={`${action.type}-${JSON.stringify(action.payload)}`} action={action} surface={surface} onAction={onAction} />)}
            </div>
          </div>
        )}
      </details>
      {block.status === 'awaiting_approval' && (
        <div className="bg-surface-subtle/40 px-3 pb-3" data-ai-tool-intervention>
          <ApprovalControls block={block} surface={surface} onApproval={onApproval} />
        </div>
      )}
    </div>
  )
}

function ToolStatusIcon({ status }: { status: AIToolStatus }) {
  const className = 'size-3'
  if (status === 'running')
    return <LoaderCircle aria-hidden="true" className={`${className} animate-spin text-info motion-reduce:animate-pulse`} data-ai-tool-status-icon={status} />
  if (status === 'succeeded')
    return <Check aria-hidden="true" className={`${className} text-success`} data-ai-tool-status-icon={status} />
  if (status === 'failed')
    return <CircleAlert aria-hidden="true" className={`${className} text-danger`} data-ai-tool-status-icon={status} />
  if (status === 'rejected')
    return <Minus aria-hidden="true" className={className} data-ai-tool-status-icon={status} />
  if (status === 'canceled')
    return <CircleStop aria-hidden="true" className={className} data-ai-tool-status-icon={status} />
  if (status === 'skipped')
    return <Minus aria-hidden="true" className={className} data-ai-tool-status-icon={status} />
  if (status === 'awaiting_approval')
    return <ShieldAlert aria-hidden="true" className={`${className} text-warning`} data-ai-tool-status-icon={status} />
  return <CircleDashed aria-hidden="true" className={className} data-ai-tool-status-icon={status} />
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
  return <Button className={surface === 'page' ? 'min-h-11 px-3 !text-sm' : 'h-7 px-2.5 !text-[11px]'} disabled={pending || done} size="sm" variant={variant} onClick={() => void execute()}>{done ? t('aiAssistant.actions.opened') : label}</Button>
}

function ApprovalControls({ block, onApproval, surface }: { block: ToolCallBlock, onApproval: (block: ToolCallBlock, decision: AIApprovalDecision, reason?: string) => Promise<void>, surface: 'page' | 'window' }) {
  const { t } = useTranslation()
  const page = surface === 'page'
  const [reason, setReason] = useState('')
  const [pending, setPending] = useState(false)
  const decide = async (decision: AIApprovalDecision) => {
    try {
      setPending(true)
      await onApproval(block, decision, reason.trim() || undefined)
    }
    catch (error) {
      toast.error(error instanceof Error ? error.message : t('aiAssistant.errors.approval'))
    }
    finally {
      setPending(false)
    }
  }
  return (
    <div className="mt-3 grid gap-2 rounded-control bg-warning-subtle p-3">
      <strong className="text-xs text-warning">{t('aiAssistant.approval.title')}</strong>
      <p className="text-xs text-muted-foreground">{t('aiAssistant.approval.bindingHint')}</p>
      <Input aria-label={t('aiAssistant.approval.reason')} className={page ? 'h-11 text-base' : undefined} disabled={pending} maxLength={500} placeholder={t('aiAssistant.approval.reasonPlaceholder')} value={reason} onChange={event => setReason(event.target.value)} />
      <div className={page ? 'grid grid-cols-2 gap-2' : 'flex flex-wrap justify-end gap-2'}>
        <Button className={page ? 'min-h-12 w-full px-3 !text-sm' : 'h-7 px-2.5 !text-[11px]'} disabled={pending} size="sm" variant="outline" onClick={() => void decide('reject')}>{t('aiAssistant.approval.reject')}</Button>
        <Button className={page ? 'min-h-12 w-full px-3 !text-sm' : 'h-7 px-2.5 !text-[11px]'} disabled={pending} size="sm" variant="outline" onClick={() => void decide('approve')}>{t('aiAssistant.approval.approve')}</Button>
        <Button className={page ? 'col-span-2 min-h-12 w-full px-3 !text-sm' : 'h-7 px-2.5 !text-[11px]'} disabled={pending} size="sm" onClick={() => void decide('approve_always')}>{t('aiAssistant.approval.approveAlways')}</Button>
      </div>
    </div>
  )
}
