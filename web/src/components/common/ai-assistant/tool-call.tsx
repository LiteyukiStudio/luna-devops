import type { AIBlock } from './state'
import type { AIToolStatus, AIUIAction } from '@/api'
import { Check, ChevronRight, CircleAlert, CircleDashed, CircleStop, LoaderCircle, LockKeyhole, Minus, ShieldAlert } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { OneTimeCodeInput } from '@/components/common/one-time-code-input'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { isAIUIActionRepeatable } from './actions'
import { runFailureTranslationKey } from './errors'

export type ToolCallBlock = Extract<AIBlock, { type: 'tool_call' }>
export type AIApprovalDecision = 'approve' | 'reject' | 'approve_all'

const SECRET_FIELD = /authorization|cookie|password|secret|token|credential/i

const statusTone: Record<AIToolStatus, string> = {
  proposed: 'bg-surface-inset text-muted-foreground',
  awaiting_approval: 'bg-warning-subtle text-warning',
  awaiting_mfa: 'bg-warning-subtle text-warning',
  running: 'bg-info-subtle text-info',
  succeeded: 'bg-success-subtle text-success',
  failed: 'bg-danger-subtle text-danger',
  canceled: 'bg-surface-inset text-muted-foreground',
  skipped: 'bg-surface-inset text-muted-foreground',
}

function displayValue(value: unknown): string {
  if (value === null)
    return '—'
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean')
    return String(value)
  if (Array.isArray(value))
    return value.slice(0, 5).map(displayValue).join(', ')
  return '—'
}

export function AIToolCallCard({ block, onAction, onApproval, onMFA }: { block: ToolCallBlock, onAction: (action: AIUIAction) => Promise<boolean>, onApproval: (block: ToolCallBlock, decision: AIApprovalDecision, reason?: string) => Promise<void>, onMFA: (block: ToolCallBlock, code: string) => Promise<void> }) {
  const { t, i18n } = useTranslation()
  const entries = Object.entries(block.arguments).filter(([key]) => !SECRET_FIELD.test(key)).slice(0, 20)
  const title = block.titleKey && i18n.exists(block.titleKey) ? t(block.titleKey) : block.operationId
  const summary = block.status === 'failed'
    ? t(runFailureTranslationKey(block.titleKey))
    : block.result?.summaryKey && i18n.exists(block.result.summaryKey)
      ? t(block.result.summaryKey, block.result.summaryParams)
      : t('aiAssistant.resultAvailable')
  return (
    <details className="group overflow-hidden rounded-container bg-surface" open={block.status === 'awaiting_approval' || block.status === 'awaiting_mfa' ? true : undefined}>
      <summary className="flex min-h-9 cursor-pointer list-none items-center gap-1.5 px-2 py-1 outline-none hover:bg-surface-inset focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring [&::-webkit-details-marker]:hidden" data-ai-tool-summary>
        <strong className="min-w-0 flex-1 truncate text-xs font-medium">{title}</strong>
        <Badge className={cn('gap-1 border-transparent px-1.5 py-0 text-[10px] leading-4', statusTone[block.status])}>
          <ToolStatusIcon status={block.status} />
          {t(`aiAssistant.status.${block.status}`)}
        </Badge>
        <ChevronRight className="size-3.5 shrink-0 text-muted-foreground transition-transform group-open:rotate-90" />
      </summary>
      <div className="border-t border-separator-subtle bg-surface-subtle/40 px-3 pb-3">
        <h3 className="mb-2 mt-3 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">{t('aiAssistant.arguments')}</h3>
        {entries.length
          ? (
              <dl className="grid grid-cols-[minmax(5rem,35%)_minmax(0,1fr)] gap-x-2 gap-y-1.5 text-xs">
                {entries.map(([key, value]) => (
                  <div key={key} className="contents">
                    <dt className="truncate text-muted-foreground">{key}</dt>
                    <dd className="m-0 break-words font-medium">{displayValue(value)}</dd>
                  </div>
                ))}
              </dl>
            )
          : <p className="text-xs text-muted-foreground">{t('aiAssistant.noArguments')}</p>}
        <h3 className="mb-2 mt-3 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">{t('aiAssistant.result')}</h3>
        {block.result
          ? (
              <div className="grid gap-2 rounded-control bg-surface px-2.5 py-2 text-xs">
                <p>{summary}</p>
                {block.durationMs !== undefined && (
                  <div className="flex justify-between gap-3">
                    <span className="text-muted-foreground">{t('aiAssistant.duration')}</span>
                    <strong>{`${block.durationMs} ms`}</strong>
                  </div>
                )}
                {block.result.fields?.map(field => (
                  <div key={field.labelKey} className="flex justify-between gap-3">
                    <span className="text-muted-foreground">{i18n.exists(field.labelKey) ? t(field.labelKey) : field.labelKey}</span>
                    <strong className="break-all text-right">{displayValue(field.value)}</strong>
                  </div>
                ))}
              </div>
            )
          : (
              <div className="grid gap-2" aria-label={t('aiAssistant.resultPending')}>
                <Skeleton className="h-2.5 w-full" />
                <Skeleton className="h-2.5 w-2/3" />
              </div>
            )}
        {block.uiActions.length > 0 && (
          <div className="mt-3 flex flex-wrap justify-end gap-2">
            {block.uiActions.map(action => <ActionButton key={`${action.type}-${JSON.stringify(action.payload)}`} action={action} onAction={onAction} />)}
          </div>
        )}
        {block.status === 'awaiting_approval' && <ApprovalControls block={block} onApproval={onApproval} />}
        {block.status === 'awaiting_mfa' && <MFAControls block={block} onMFA={onMFA} />}
      </div>
    </details>
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
  if (status === 'canceled')
    return <CircleStop aria-hidden="true" className={className} data-ai-tool-status-icon={status} />
  if (status === 'skipped')
    return <Minus aria-hidden="true" className={className} data-ai-tool-status-icon={status} />
  if (status === 'awaiting_approval')
    return <ShieldAlert aria-hidden="true" className={`${className} text-warning`} data-ai-tool-status-icon={status} />
  if (status === 'awaiting_mfa')
    return <LockKeyhole aria-hidden="true" className={`${className} text-warning`} data-ai-tool-status-icon={status} />
  return <CircleDashed aria-hidden="true" className={className} data-ai-tool-status-icon={status} />
}

function ActionButton({ action, onAction }: { action: AIUIAction, onAction: (action: AIUIAction) => Promise<boolean> }) {
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
  return <Button disabled={pending || done} size="sm" variant={variant} onClick={() => void execute()}>{done ? t('aiAssistant.actions.opened') : label}</Button>
}

function ApprovalControls({ block, onApproval }: { block: ToolCallBlock, onApproval: (block: ToolCallBlock, decision: AIApprovalDecision, reason?: string) => Promise<void> }) {
  const { t } = useTranslation()
  const [reason, setReason] = useState('')
  const [pending, setPending] = useState(false)
  const validBinding = Boolean(block.argumentsHash && block.expectedVersion !== undefined)
  const decide = async (decision: AIApprovalDecision) => {
    if (!validBinding)
      return
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
      <Input aria-label={t('aiAssistant.approval.reason')} disabled={pending} maxLength={500} placeholder={t('aiAssistant.approval.reasonPlaceholder')} value={reason} onChange={event => setReason(event.target.value)} />
      {!validBinding && <p className="text-xs text-danger">{t('aiAssistant.approval.invalidBinding')}</p>}
      <div className="flex flex-wrap justify-end gap-2">
        <Button disabled={pending || !validBinding} size="sm" variant="outline" onClick={() => void decide('reject')}>{t('aiAssistant.approval.reject')}</Button>
        <Button disabled={pending || !validBinding} size="sm" variant="outline" onClick={() => void decide('approve_all')}>{t('aiAssistant.approval.approveAll')}</Button>
        <Button disabled={pending || !validBinding} size="sm" onClick={() => void decide('approve')}>{t('aiAssistant.approval.approve')}</Button>
      </div>
    </div>
  )
}

function MFAControls({ block, onMFA }: { block: ToolCallBlock, onMFA: (block: ToolCallBlock, code: string) => Promise<void> }) {
  const { t } = useTranslation()
  const [code, setCode] = useState('')
  const [pending, setPending] = useState(false)
  const validBinding = block.expectedVersion !== undefined && Boolean(block.mfaPurpose)
  const verify = async (candidate = code) => {
    if (!validBinding || candidate.length !== 6)
      return
    try {
      setPending(true)
      await onMFA(block, candidate)
    }
    catch (error) {
      toast.error(error instanceof Error ? error.message : t('aiAssistant.errors.mfa'))
    }
    finally {
      setPending(false)
    }
  }
  return (
    <div className="mt-3 grid gap-3 rounded-control bg-primary-subtle p-3">
      <div>
        <strong className="text-xs text-primary-text">{t('aiAssistant.mfa.title')}</strong>
        <p className="mt-1 text-xs text-muted-foreground">{t('aiAssistant.mfa.description')}</p>
      </div>
      <OneTimeCodeInput
        aria-label={t('aiAssistant.mfa.code')}
        disabled={pending}
        invalid={!validBinding}
        value={code}
        onChange={setCode}
        onComplete={value => void verify(value)}
      />
      <Button disabled={pending || !validBinding || code.length !== 6} size="sm" onClick={() => void verify()}>{t('aiAssistant.mfa.verify')}</Button>
    </div>
  )
}
