import type { TFunction } from 'i18next'
import type { InteractionContentBlock } from './interaction-card-schema'
import type { AIUIAction } from '@/api'
import { LoaderCircle, Stethoscope } from 'lucide-react'
import { use, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { aiProgressStreamUrl } from '@/api'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { createTracedEventSource } from '@/lib/telemetry'
import { cn } from '@/lib/utils'
import { AIAssistantRuntimeContext } from './runtime-context'
import { AIStepStatusGlyph } from './step-status'

type LiveProgressBlockDefinition = Extract<InteractionContentBlock, { type: 'live_progress' }>

const progressSnapshotSchema = z.object({
  operationId: z.string(),
  operationType: z.string(),
  revision: z.string().regex(/^\d{8}T\d{6}\.\d{9}Z$/),
  state: z.enum(['queued', 'running', 'waiting_input', 'waiting_approval', 'succeeded', 'failed', 'cancelled']),
  stageCode: z.string(),
  progress: z.object({ mode: z.enum(['determinate', 'indeterminate']), value: z.number().min(0).max(100).optional() }),
  steps: z.array(z.object({ id: z.string(), labelCode: z.string(), status: z.enum(['pending', 'running', 'success', 'warning', 'error', 'skipped']) })),
  startedAt: z.string().optional(),
  updatedAt: z.string(),
  finishedAt: z.string().optional(),
  error: z.object({ code: z.string(), requestId: z.string().optional(), traceId: z.string().optional() }).optional(),
})

type ProgressSnapshot = z.infer<typeof progressSnapshotSchema>

export function LiveProgressBlock({ actionDisabled = false, block, onAction }: { actionDisabled?: boolean, block: LiveProgressBlockDefinition, onAction?: (action: AIUIAction) => Promise<boolean> }) {
  const { t } = useTranslation()
  const runtime = use(AIAssistantRuntimeContext)
  const [snapshot, setSnapshot] = useState<ProgressSnapshot>()
  const [diagnosing, setDiagnosing] = useState(false)
  const [diagnosedKey, setDiagnosedKey] = useState<string>()
  const [connection, setConnection] = useState<'connecting' | 'live' | 'paused' | 'failed'>(() => typeof EventSource === 'undefined' ? 'failed' : 'connecting')
  const latestRevisionRef = useRef('')
  const diagnosingRef = useRef(false)
  const { operationId, operationType, projectId } = block.binding
  const bindingKey = `${projectId}:${operationType}:${operationId}`
  const diagnosed = diagnosedKey === bindingKey
  const conversationBusy = Boolean(
    actionDisabled
    || runtime?.timelineProps.resendDisabled
    || runtime?.composerProps.activeRun
    || runtime?.composerProps.sending,
  )

  useEffect(() => {
    latestRevisionRef.current = ''
    if (typeof EventSource === 'undefined') {
      return
    }
    const source = createTracedEventSource(
      aiProgressStreamUrl(projectId, operationType, operationId),
      { withCredentials: true },
      'ai.progress.events.stream',
    )
    const onSnapshot = (event: MessageEvent<string>) => {
      let payload: unknown
      try {
        payload = JSON.parse(event.data) as unknown
      }
      catch {
        setConnection('failed')
        return
      }
      const parsed = progressSnapshotSchema.safeParse(payload)
      if (!parsed.success || parsed.data.revision <= latestRevisionRef.current)
        return
      latestRevisionRef.current = parsed.data.revision
      setSnapshot(parsed.data)
      setConnection('live')
      if (isTerminal(parsed.data.state))
        source.close()
    }
    const onProgressError = (event: MessageEvent<string>) => {
      let payload: unknown
      try {
        payload = JSON.parse(event.data) as unknown
      }
      catch {
        payload = undefined
      }
      setConnection('failed')
      if (payload)
        source.close()
    }
    const onOpen = () => setConnection('live')
    const onError = () => {
      setConnection(source.readyState === EventSource.CLOSED ? 'failed' : 'paused')
    }
    source.addEventListener('progress.snapshot', onSnapshot as EventListener)
    source.addEventListener('progress.error', onProgressError as EventListener)
    source.addEventListener('open', onOpen)
    source.addEventListener('error', onError)
    return () => {
      source.removeEventListener('progress.snapshot', onSnapshot as EventListener)
      source.removeEventListener('progress.error', onProgressError as EventListener)
      source.removeEventListener('open', onOpen)
      source.removeEventListener('error', onError)
      source.close()
    }
  }, [operationId, operationType, projectId])

  const state = snapshot?.state
  const value = snapshot?.progress.value
  const diagnoseLabel = t('aiAssistant.cards.liveProgress.diagnoseFailure')
  const failureReason = translateFailureCode(t, snapshot?.error?.code)
  const triggerDiagnose = async () => {
    if (!onAction || diagnosingRef.current || diagnosed || conversationBusy)
      return
    try {
      diagnosingRef.current = true
      setDiagnosing(true)
      const ok = await onAction({
        version: 1,
        type: 'send_message',
        label: diagnoseLabel,
        payload: {
          message: t('aiAssistant.cards.liveProgress.diagnoseFailurePrompt', {
            type: translateOperationType(t, operationType),
            id: operationId,
            projectId,
          }),
        },
      })
      if (ok) {
        setDiagnosedKey(bindingKey)
        return
      }
      toast.error(t('aiAssistant.cards.actionFailed'))
    }
    catch (error) {
      toast.error(error instanceof Error ? error.message : t('aiAssistant.cards.actionFailed'))
    }
    finally {
      diagnosingRef.current = false
      setDiagnosing(false)
    }
  }
  return (
    <div className="grid gap-2" data-ai-live-progress={state ?? connection}>
      <div className="flex min-w-0 items-center justify-between gap-2 text-xs">
        <span className="truncate font-medium">{block.label ?? (snapshot ? translateCode(t, snapshot.stageCode) : t('aiAssistant.cards.liveProgress.connecting'))}</span>
        <ProgressStateBadge state={state} connection={connection} />
      </div>
      <div className="h-1.5 overflow-hidden rounded-full bg-muted" role="progressbar" aria-label={block.label ?? t('aiAssistant.cards.liveProgress.label')} aria-valuenow={value}>
        <div
          className={cn(
            'h-full rounded-full transition-[width] duration-500 motion-reduce:transition-none',
            state === 'failed' || connection === 'failed' ? 'bg-danger-solid' : state === 'cancelled' ? 'bg-warning-solid' : 'bg-primary',
            snapshot?.progress.mode !== 'determinate' && !isTerminal(state) && 'w-1/3 animate-[ai-live-progress_1.35s_ease-in-out_infinite] motion-reduce:w-full motion-reduce:animate-none motion-reduce:opacity-60',
          )}
          style={snapshot?.progress.mode === 'determinate' ? { width: `${value ?? 0}%` } : undefined}
        />
      </div>
      {block.detail && <p className="text-[11px] text-muted-foreground">{block.detail}</p>}
      {snapshot && (
        <div className="grid gap-1" aria-live="polite">
          {snapshot.steps.map(step => (
            <div key={step.id} className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
              <AIStepStatusGlyph status={step.status} />
              <span className={cn('truncate', step.status === 'running' && 'font-medium text-foreground')}>{translateCode(t, step.labelCode)}</span>
            </div>
          ))}
          <time className="mt-0.5 text-[11px] text-muted-foreground" dateTime={snapshot.updatedAt}>
            {t('aiAssistant.cards.liveProgress.updatedAt', { time: new Date(snapshot.updatedAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }) })}
          </time>
        </div>
      )}
      {state === 'failed' && (
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-control border border-danger-border bg-danger-subtle px-2.5 py-2 text-xs text-danger" data-ai-live-progress-failed>
          <div className="min-w-0 flex-1">
            <p className="leading-5">{t('aiAssistant.cards.liveProgress.failedHint')}</p>
            {failureReason && (
              <p className="mt-0.5 truncate text-[11px] opacity-90">{failureReason}</p>
            )}
          </div>
          {onAction && (
            <Button
              className="h-7 shrink-0 gap-1.5 px-2.5 text-xs"
              data-ai-live-progress-diagnose
              disabled={diagnosing || diagnosed || conversationBusy}
              size="sm"
              variant="outline"
              onClick={() => void triggerDiagnose()}
            >
              {diagnosing
                ? <LoaderCircle className="size-3.5 animate-spin motion-reduce:animate-none" />
                : <Stethoscope className="size-3.5" />}
              {diagnoseLabel}
            </Button>
          )}
        </div>
      )}
      {(connection === 'paused' || connection === 'failed') && !isTerminal(state) && (
        <p className="text-[11px] text-warning" role="status">{t(connection === 'paused' ? 'aiAssistant.cards.liveProgress.paused' : 'aiAssistant.cards.liveProgress.unavailable')}</p>
      )}
    </div>
  )
}

function ProgressStateBadge({ state, connection }: { state?: ProgressSnapshot['state'], connection: 'connecting' | 'live' | 'paused' | 'failed' }) {
  const { t } = useTranslation()
  const key = state === 'failed' ? 'operation_failed' : state ?? connection
  const tone = state === 'failed' || connection === 'failed' ? 'danger' : state === 'succeeded' ? 'success' : state === 'cancelled' || connection === 'paused' ? 'warning' : 'info'
  return <StatusBadge className="shrink-0 px-1.5 py-0 text-[11px]" tone={tone}>{t(`aiAssistant.cards.liveProgress.states.${key}`)}</StatusBadge>
}

function translateCode(t: TFunction, code: string) {
  return t(`aiAssistant.cards.liveProgress.codes.${code.replaceAll('.', '_')}`, { defaultValue: code })
}

function translateOperationType(t: TFunction, operationType: string) {
  switch (operationType) {
    case 'build_run':
      return t('aiAssistant.cards.liveProgress.operationTypes.build_run')
    case 'release':
      return t('aiAssistant.cards.liveProgress.operationTypes.release')
    case 'hook_run':
      return t('aiAssistant.cards.liveProgress.operationTypes.hook_run')
    case 'app_template_installation':
      return t('aiAssistant.cards.liveProgress.operationTypes.app_template_installation')
    default:
      return t('aiAssistant.cards.liveProgress.operationTypes.default')
  }
}

function translateFailureCode(t: TFunction, code: string | undefined) {
  if (!code || code === 'ai.progress.operation_failed')
    return undefined
  const translated = t(`aiAssistant.cards.liveProgress.failureCodes.${code.replaceAll('.', '_')}`, { defaultValue: '' })
  return translated || undefined
}

function isTerminal(state?: ProgressSnapshot['state']) {
  return state === 'succeeded' || state === 'failed' || state === 'cancelled'
}
