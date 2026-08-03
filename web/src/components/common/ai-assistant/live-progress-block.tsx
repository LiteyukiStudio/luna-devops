import type { TFunction } from 'i18next'
import type { InteractionContentBlock } from './interaction-card-schema'
import { AlertCircle, Check, Circle, LoaderCircle, Pause, X } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'
import { aiProgressStreamUrl } from '@/api'
import { StatusBadge } from '@/components/common/status-badge'
import { createTracedEventSource } from '@/lib/telemetry'
import { cn } from '@/lib/utils'

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

export function LiveProgressBlock({ block }: { block: LiveProgressBlockDefinition }) {
  const { t } = useTranslation()
  const [snapshot, setSnapshot] = useState<ProgressSnapshot>()
  const [connection, setConnection] = useState<'connecting' | 'live' | 'paused' | 'failed'>(() => typeof EventSource === 'undefined' ? 'failed' : 'connecting')
  const latestRevisionRef = useRef('')
  const { operationId, operationType, projectId } = block.binding

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
  return (
    <div className="grid gap-2" data-ai-live-progress={state ?? connection}>
      <div className="flex min-w-0 items-center justify-between gap-2 text-[10px]">
        <span className="truncate font-medium">{block.label ?? (snapshot ? translateCode(t, snapshot.stageCode) : t('aiAssistant.cards.liveProgress.connecting'))}</span>
        <ProgressStateBadge state={state} connection={connection} />
      </div>
      <div className="h-1.5 overflow-hidden rounded-full bg-surface-inset" role="progressbar" aria-label={block.label ?? t('aiAssistant.cards.liveProgress.label')} aria-valuenow={value}>
        <div
          className={cn(
            'h-full rounded-full transition-[width] duration-500 motion-reduce:transition-none',
            state === 'failed' || connection === 'failed' ? 'bg-danger' : state === 'cancelled' ? 'bg-warning' : 'bg-primary',
            snapshot?.progress.mode !== 'determinate' && !isTerminal(state) && 'w-1/3 animate-[ai-live-progress_1.35s_ease-in-out_infinite] motion-reduce:animate-pulse',
          )}
          style={snapshot?.progress.mode === 'determinate' ? { width: `${value ?? 0}%` } : undefined}
        />
      </div>
      {block.detail && <p className="text-[9px] text-muted-foreground">{block.detail}</p>}
      {snapshot && (
        <div className="grid gap-1" aria-live="polite">
          {snapshot.steps.map(step => (
            <div key={step.id} className="flex min-w-0 items-center gap-1.5 text-[10px] text-muted-foreground">
              <StepIcon status={step.status} />
              <span className={cn('truncate', step.status === 'running' && 'font-medium text-foreground')}>{translateCode(t, step.labelCode)}</span>
            </div>
          ))}
          <time className="mt-0.5 text-[9px] text-muted-foreground" dateTime={snapshot.updatedAt}>
            {t('aiAssistant.cards.liveProgress.updatedAt', { time: new Date(snapshot.updatedAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }) })}
          </time>
        </div>
      )}
      {(connection === 'paused' || connection === 'failed') && !isTerminal(state) && (
        <p className="text-[9px] text-warning" role="status">{t(connection === 'paused' ? 'aiAssistant.cards.liveProgress.paused' : 'aiAssistant.cards.liveProgress.unavailable')}</p>
      )}
    </div>
  )
}

function ProgressStateBadge({ state, connection }: { state?: ProgressSnapshot['state'], connection: 'connecting' | 'live' | 'paused' | 'failed' }) {
  const { t } = useTranslation()
  const key = state === 'failed' ? 'operation_failed' : state ?? connection
  const tone = state === 'failed' || connection === 'failed' ? 'danger' : state === 'succeeded' ? 'success' : state === 'cancelled' || connection === 'paused' ? 'warning' : 'info'
  return <StatusBadge className="shrink-0 px-1.5 py-0 text-[9px]" tone={tone}>{t(`aiAssistant.cards.liveProgress.states.${key}`)}</StatusBadge>
}

function StepIcon({ status }: { status: ProgressSnapshot['steps'][number]['status'] }) {
  const className = 'size-3 shrink-0'
  if (status === 'running')
    return <LoaderCircle className={cn(className, 'animate-spin text-info motion-reduce:animate-none')} />
  if (status === 'success')
    return <Check className={cn(className, 'text-success')} />
  if (status === 'error')
    return <AlertCircle className={cn(className, 'text-danger')} />
  if (status === 'warning')
    return <Pause className={cn(className, 'text-warning')} />
  if (status === 'skipped')
    return <X className={className} />
  return <Circle className={className} />
}

function translateCode(t: TFunction, code: string) {
  return t(`aiAssistant.cards.liveProgress.codes.${code.replaceAll('.', '_')}`, { defaultValue: code })
}

function isTerminal(state?: ProgressSnapshot['state']) {
  return state === 'succeeded' || state === 'failed' || state === 'cancelled'
}
