import type { PointerEvent as ReactPointerEvent } from 'react'
import type { AIAssistantState, AIBlock } from './ai-assistant-state'
import type { AIConversation, AIEvent, AIToolStatus, AIUIAction } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Bot, Check, ChevronRight, CircleStop, Ellipsis, History, LoaderCircle, MessageSquarePlus, Minimize2, Pencil, RotateCcw, Search, Send, Sparkles, Trash2, X } from 'lucide-react'
import { useCallback, useEffect, useMemo, useReducer, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { api, isUsableAICapabilities } from '@/api'
import { executeAIUIAction } from '@/components/common/ai-assistant-actions'
import { emptyAIAssistantState, isValidAITimeline, reduceAIEvent, stateFromTimeline } from '@/components/common/ai-assistant-state'
import { createAIEventSource } from '@/components/common/ai-assistant-stream'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { OneTimeCodeInput } from '@/components/common/one-time-code-input'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

const WINDOW_STORAGE_KEY = 'luna.ai-assistant.window.v1'
const SECRET_FIELD = /authorization|cookie|password|secret|token|credential/i
const AI_EVENT_TYPES = [
  'run.started',
  'model.started',
  'item.started',
  'content.delta',
  'content.completed',
  'thinking.started',
  'thinking.delta',
  'thinking.completed',
  'item.completed',
  'tool.started',
  'tool.progress',
  'tool.completed',
  'tool.failed',
  'approval.required',
  'approval.resolved',
  'mfa.required',
  'mfa.resolved',
  'ui.action',
  'model.completed',
  'run.failed',
  'run.completed',
  'run.canceled',
] as const

interface WindowPreference {
  x: number
  y: number
  width: number
  height: number
}

function readWindowPreference(): WindowPreference {
  try {
    const value = JSON.parse(localStorage.getItem(WINDOW_STORAGE_KEY) ?? '')
    if (typeof value.x === 'number' && typeof value.y === 'number' && typeof value.width === 'number' && typeof value.height === 'number')
      return value
  }
  catch {}
  return { x: 0, y: 0, width: 420, height: 640 }
}

function safePageContext(pathname: string, search: string, locale: string) {
  const projectMatch = pathname.match(/^\/projects\/([^/]+)/)
  const applicationMatch = pathname.match(/^\/projects\/[^/]+\/apps\/([^/]+)/)
  const query = new URLSearchParams(search)
  let routeName = 'unknown'
  if (pathname === '/dashboard')
    routeName = 'dashboard'
  else if (pathname === '/projects')
    routeName = 'projects'
  else if (applicationMatch)
    routeName = 'application.detail'
  else if (projectMatch)
    routeName = 'project.workspace'
  else if (pathname === '/events')
    routeName = 'events'
  else if (pathname === '/clusters')
    routeName = 'clusters'
  else if (pathname === '/registries')
    routeName = 'registries'
  else if (pathname === '/billing')
    routeName = 'billing'
  return {
    routeName,
    projectId: projectMatch?.[1],
    applicationId: applicationMatch?.[1],
    activeTab: query.get('tab') ?? undefined,
    locale,
  }
}

export function AiAssistant() {
  const { i18n, t } = useTranslation()
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const location = useLocation()
  const triggerRef = useRef<HTMLButtonElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const panelRef = useRef<HTMLElement>(null)
  const [open, setOpen] = useState(false)
  const [capabilityEpoch, invalidateOpenWindow] = useReducer(value => value + 1, 0)
  const [openedCapabilityEpoch, setOpenedCapabilityEpoch] = useState(0)
  const [minimized, setMinimized] = useState(false)
  const [showConversations, setShowConversations] = useState(false)
  const [activeConversationId, setActiveConversationId] = useState<string>()
  const [conversationSearch, setConversationSearch] = useState('')
  const [liveSubscription, setLiveSubscription] = useState<{ eventsUrl: string, runId: string }>()
  const [draft, setDraft] = useState('')
  const [preference, setPreference] = useState(readWindowPreference)
  const dragRef = useRef<{ originX: number, originY: number, startX: number, startY: number } | null>(null)

  const capabilities = useQuery({
    queryKey: ['ai', 'capabilities'],
    queryFn: api.getAICapabilities,
    retry: false,
    staleTime: 30_000,
    refetchInterval: 30_000,
  })
  const available = isUsableAICapabilities(capabilities.data)
  const assistantOpen = open && capabilityEpoch === openedCapabilityEpoch

  const conversations = useQuery({
    queryKey: ['ai', 'conversations', conversationSearch],
    queryFn: () => api.listAIConversations({ page: 1, pageSize: 50, search: conversationSearch || undefined }),
    enabled: available && assistantOpen,
  })
  const selectedConversationId = activeConversationId ?? conversations.data?.items[0]?.id

  const timeline = useQuery({
    queryKey: ['ai', 'timeline', selectedConversationId],
    queryFn: () => api.getAIConversationTimeline(selectedConversationId!),
    enabled: available && assistantOpen && Boolean(selectedConversationId),
  })
  const timelineValid = isValidAITimeline(timeline.data)
  const [streamState, dispatchStream] = useReducer((state: AIAssistantState, event: AIEvent | { type: 'snapshot', timeline: NonNullable<typeof timeline.data> }) =>
    'timeline' in event ? stateFromTimeline(event.timeline) : reduceAIEvent(state, event), emptyAIAssistantState)
  useEffect(() => {
    if (timelineValid)
      dispatchStream({ type: 'snapshot', timeline: timeline.data! })
  }, [timeline.data, timelineValid])

  const subscribe = useCallback((runId: string, after: number, explicitUrl?: string) => {
    const rawUrl = explicitUrl || `/api/v1/ai/runs/${encodeURIComponent(runId)}/events`
    const source = createAIEventSource(rawUrl, after)
    const receive = (rawEvent: Event) => {
      try {
        dispatchStream(JSON.parse((rawEvent as MessageEvent<string>).data) as AIEvent)
      }
      catch {
        source.close()
      }
    }
    source.onmessage = receive
    AI_EVENT_TYPES.forEach(type => source.addEventListener(type, receive))
    source.addEventListener('ai.capabilities_changed', () => {
      invalidateOpenWindow()
      void queryClient.invalidateQueries({ queryKey: ['ai', 'capabilities'] })
    })
    return source
  }, [queryClient])

  useEffect(() => {
    if (!assistantOpen || !timelineValid || !capabilities.data?.features.streaming)
      return
    const subscriptions = timeline.data!.eventCursors.map(cursor => subscribe(cursor.runId, cursor.after))
    return () => subscriptions.forEach(source => source.close())
  }, [assistantOpen, capabilities.data?.features.streaming, subscribe, timeline.data, timelineValid])
  useEffect(() => {
    if (!assistantOpen || !liveSubscription)
      return
    const source = subscribe(liveSubscription.runId, 0, liveSubscription.eventsUrl)
    return () => source.close()
  }, [assistantOpen, liveSubscription, subscribe])

  const createConversation = useMutation({
    mutationFn: () => api.createAIConversation({ projectId: safePageContext(location.pathname, location.search, i18n.language).projectId }),
    onSuccess: async (conversation) => {
      await queryClient.invalidateQueries({ queryKey: ['ai', 'conversations'] })
      setActiveConversationId(conversation.id)
    },
    onError: error => toast.error(error instanceof Error ? error.message : t('aiAssistant.errors.createConversation')),
  })
  const renameConversation = useMutation({
    mutationFn: ({ id, title }: { id: string, title: string }) => api.renameAIConversation(id, title),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['ai', 'conversations'] }),
    onError: error => toast.error(error instanceof Error ? error.message : t('aiAssistant.errors.renameConversation')),
  })
  const deleteConversation = useMutation({
    mutationFn: api.deleteAIConversation,
    onSuccess: async (_, deletedId) => {
      if (activeConversationId === deletedId)
        setActiveConversationId(undefined)
      await queryClient.invalidateQueries({ queryKey: ['ai', 'conversations'] })
    },
    onError: error => toast.error(error instanceof Error ? error.message : t('aiAssistant.errors.deleteConversation')),
  })
  const activeRunId = useMemo(() => {
    const activeFromTimeline = Object.entries(streamState.runStatuses).find(([, status]) => ['queued', 'running', 'waiting_approval', 'waiting_mfa', 'waiting_input'].includes(status))?.[0]
    if (activeFromTimeline)
      return activeFromTimeline
    if (liveSubscription && !['completed', 'failed', 'canceled'].includes(streamState.runStatuses[liveSubscription.runId] ?? 'queued'))
      return liveSubscription.runId
    return undefined
  }, [liveSubscription, streamState.runStatuses])
  const sendTurn = useMutation({
    mutationFn: async () => {
      let conversationId = selectedConversationId
      if (!conversationId) {
        const conversation = await api.createAIConversation({ projectId: safePageContext(location.pathname, location.search, i18n.language).projectId })
        conversationId = conversation.id
        setActiveConversationId(conversationId)
      }
      const result = await api.createAITurn(conversationId, {
        input: { parts: [{ type: 'text', text: draft.trim() }] },
        pageContext: safePageContext(location.pathname, location.search, i18n.language),
      }, crypto.randomUUID())
      return { ...result, conversationId }
    },
    onSuccess: async (result) => {
      setDraft('')
      setLiveSubscription({ eventsUrl: result.eventsUrl, runId: result.runId })
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['ai', 'conversations'] }),
        queryClient.invalidateQueries({ queryKey: ['ai', 'timeline', result.conversationId] }),
      ])
    },
    onError: error => toast.error(error instanceof Error ? error.message : t('aiAssistant.errors.send')),
  })
  const cancelRun = useMutation({
    mutationFn: () => api.cancelAIRun(activeRunId!),
    onError: error => toast.error(error instanceof Error ? error.message : t('aiAssistant.errors.stop')),
  })
  const submitRunInput = useMutation({
    mutationFn: () => api.submitAIRunInput(activeRunId!, {
      input: { parts: [{ type: 'text', text: draft.trim() }] },
      expectedVersion: streamState.runExpectedVersions[activeRunId!] ?? -1,
    }),
    onSuccess: () => setDraft(''),
    onError: error => toast.error(error instanceof Error ? error.message : t('aiAssistant.errors.input')),
  })
  const activeRunStatus = activeRunId ? streamState.runStatuses[activeRunId] ?? 'queued' : undefined
  const waitingInput = activeRunStatus === 'waiting_input'
  const submitDraft = () => waitingInput ? submitRunInput.mutate() : sendTurn.mutate()

  useEffect(() => {
    if (!assistantOpen || minimized)
      return
    const timeout = window.setTimeout(() => inputRef.current?.focus(), 0)
    return () => window.clearTimeout(timeout)
  }, [assistantOpen, minimized])

  useEffect(() => {
    if (!panelRef.current || minimized)
      return
    const observer = new ResizeObserver(([entry]) => {
      const width = Math.min(window.innerWidth - 24, Math.max(360, entry.contentRect.width))
      const height = Math.min(window.innerHeight - 24, Math.max(480, entry.contentRect.height))
      setPreference(current => ({ ...current, width, height }))
    })
    observer.observe(panelRef.current)
    return () => observer.disconnect()
  }, [assistantOpen, minimized])
  useEffect(() => {
    localStorage.setItem(WINDOW_STORAGE_KEY, JSON.stringify(preference))
  }, [preference])

  const handlePointerDown = (event: ReactPointerEvent<HTMLElement>) => {
    if (event.button !== 0 || (event.target as HTMLElement).closest('button,input'))
      return
    dragRef.current = { originX: event.clientX, originY: event.clientY, startX: preference.x, startY: preference.y }
    event.currentTarget.setPointerCapture(event.pointerId)
  }
  const handlePointerMove = (event: ReactPointerEvent<HTMLElement>) => {
    if (!dragRef.current)
      return
    setPreference(current => ({
      ...current,
      x: Math.min(window.innerWidth / 2 - 24, Math.max(-window.innerWidth / 2 + current.width + 24, dragRef.current!.startX + event.clientX - dragRef.current!.originX)),
      y: Math.min(window.innerHeight / 2 - 24, Math.max(-window.innerHeight / 2 + current.height + 24, dragRef.current!.startY + event.clientY - dragRef.current!.originY)),
    }))
  }
  const handlePointerUp = (event: ReactPointerEvent<HTMLElement>) => {
    dragRef.current = null
    event.currentTarget.releasePointerCapture(event.pointerId)
  }

  if (!available)
    return null
  if (!assistantOpen) {
    return (
      <Button
        ref={triggerRef}
        aria-label={t('aiAssistant.open')}
        className="fixed bottom-6 right-6 z-40 size-14 rounded-feature shadow-overlay max-sm:bottom-[calc(1.5rem+env(safe-area-inset-bottom))]"
        size="icon"
        onClick={() => {
          setOpenedCapabilityEpoch(capabilityEpoch)
          setOpen(true)
        }}
      >
        <Sparkles className="size-5" />
      </Button>
    )
  }

  const close = () => {
    setOpen(false)
    window.setTimeout(() => triggerRef.current?.focus(), 0)
  }
  return (
    <section
      ref={panelRef}
      aria-label={t('aiAssistant.title')}
      className={cn(
        'fixed z-40 flex overflow-hidden border border-border bg-surface shadow-overlay max-sm:inset-0 max-sm:h-dvh max-sm:w-screen max-sm:rounded-none sm:bottom-6 sm:right-6',
        minimized ? 'h-14 w-80 rounded-container' : 'min-h-120 min-w-90 resize rounded-feature',
      )}
      style={minimized ? undefined : { width: preference.width, height: preference.height, maxWidth: 'calc(100vw - 24px)', maxHeight: 'calc(100dvh - 24px)', transform: `translate(${preference.x}px, ${preference.y}px)` }}
    >
      {showConversations && !minimized && (
        <ConversationList
          activeId={selectedConversationId}
          conversations={conversations.data?.items ?? []}
          loading={conversations.isLoading}
          search={conversationSearch}
          onClose={() => setShowConversations(false)}
          onCreate={() => createConversation.mutate()}
          onDelete={id => deleteConversation.mutate(id)}
          onRename={(id, title) => renameConversation.mutate({ id, title })}
          onSearch={setConversationSearch}
          onSelect={(id) => {
            setActiveConversationId(id)
            setShowConversations(false)
          }}
        />
      )}
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 shrink-0 touch-none select-none items-center gap-2 border-b border-separator-subtle px-3" onPointerDown={handlePointerDown} onPointerMove={handlePointerMove} onPointerUp={handlePointerUp}>
          <span className="grid size-8 shrink-0 place-items-center rounded-control bg-primary text-primary-foreground"><Sparkles className="size-4" /></span>
          <div className="min-w-0 flex-1">
            <h2 className="truncate text-sm font-semibold">{timeline.data?.conversation.title || t('aiAssistant.title')}</h2>
            <p className="truncate text-[11px] text-muted-foreground">{t('aiAssistant.context', { path: location.pathname })}</p>
          </div>
          {!minimized && <Button aria-label={t('aiAssistant.conversations.title')} size="icon" variant="ghost" onClick={() => setShowConversations(value => !value)}><History className="size-4" /></Button>}
          <Button aria-label={minimized ? t('aiAssistant.expand') : t('aiAssistant.minimize')} size="icon" variant="ghost" onClick={() => setMinimized(value => !value)}>{minimized ? <Sparkles className="size-4" /> : <Minimize2 className="size-4" />}</Button>
          <Button aria-label={t('common.close')} size="icon" variant="ghost" onClick={close}><X className="size-4" /></Button>
        </header>
        {!minimized && (
          <>
            <Timeline
              blocks={streamState.blocks}
              error={timeline.error ?? (timeline.data && !timelineValid ? new Error('ai_invalid_timeline') : null)}
              loading={timeline.isLoading}
              onAction={async action => executeAIUIAction(action, { pathname: location.pathname, search: location.search, navigate, queryClient })}
              onApproval={(block, decision, reason) => api.decideAIToolApproval(block.runId, block.toolCallId, {
                decision,
                argumentsHash: block.argumentsHash!,
                expectedVersion: block.expectedVersion!,
                reason,
              })}
              onMFA={async (block, code) => {
                const verification = await api.verifyMFA({ code, purpose: block.mfaPurpose! })
                if (!verification.stepUpAssertionId)
                  throw new Error(t('aiAssistant.errors.mfaAssertion'))
                await api.resumeAIToolMFA(block.runId, block.toolCallId, {
                  stepUpAssertionId: verification.stepUpAssertionId,
                  expectedVersion: block.expectedVersion!,
                })
              }}
              onRetry={() => void timeline.refetch()}
            />
            <footer className="shrink-0 border-t border-separator-subtle bg-surface p-3 pb-[max(0.75rem,env(safe-area-inset-bottom))]">
              <div className="flex min-h-18 gap-2 rounded-container border border-input bg-surface px-3 py-2 focus-within:ring-2 focus-within:ring-ring">
                <textarea
                  ref={inputRef}
                  aria-label={t('aiAssistant.inputLabel')}
                  className="min-h-12 min-w-0 flex-1 resize-none bg-transparent text-sm outline-none placeholder:text-muted-foreground"
                  disabled={sendTurn.isPending || submitRunInput.isPending || (Boolean(activeRunId) && !waitingInput)}
                  maxLength={capabilities.data?.limits.maxInputBytes}
                  placeholder={waitingInput ? t('aiAssistant.inputRequired') : activeRunId ? t('aiAssistant.inputRunning') : t('aiAssistant.inputPlaceholder')}
                  value={draft}
                  onChange={event => setDraft(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter' && !event.shiftKey && draft.trim()) {
                      event.preventDefault()
                      submitDraft()
                    }
                  }}
                />
                {activeRunId && !waitingInput
                  ? <Button aria-label={t('aiAssistant.stop')} className="self-end" disabled={cancelRun.isPending} size="icon" variant="outline" onClick={() => cancelRun.mutate()}><CircleStop className="size-4" /></Button>
                  : <Button aria-label={waitingInput ? t('aiAssistant.continue') : t('aiAssistant.send')} className="self-end" disabled={!draft.trim() || sendTurn.isPending || submitRunInput.isPending} size="icon" onClick={submitDraft}>{sendTurn.isPending || submitRunInput.isPending ? <LoaderCircle className="size-4 animate-spin motion-reduce:animate-none" /> : <Send className="size-4" />}</Button>}
              </div>
              <p className="mt-2 text-[10px] text-muted-foreground">{t('aiAssistant.securityHint')}</p>
            </footer>
          </>
        )}
      </div>
    </section>
  )
}

function Timeline({ blocks, error, loading, onAction, onApproval, onMFA, onRetry }: { blocks: AIBlock[], error: Error | null, loading: boolean, onAction: (action: AIUIAction) => Promise<boolean>, onApproval: (block: Extract<AIBlock, { type: 'tool_call' }>, decision: 'approve' | 'reject', reason?: string) => Promise<void>, onMFA: (block: Extract<AIBlock, { type: 'tool_call' }>, code: string) => Promise<void>, onRetry: () => void }) {
  const { t } = useTranslation()
  const viewportRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    viewportRef.current?.scrollTo({ top: viewportRef.current.scrollHeight, behavior: 'smooth' })
  }, [blocks])
  if (loading) {
    return (
      <div className="grid flex-1 content-start gap-3 p-4">
        <Skeleton className="ml-auto h-14 w-3/4" />
        <Skeleton className="h-16 w-4/5" />
        <Skeleton className="h-24 w-full" />
      </div>
    )
  }
  if (error) {
    return (
      <div className="grid flex-1 place-content-center gap-3 p-6 text-center">
        <p className="text-sm font-medium">{t('aiAssistant.errors.timeline')}</p>
        <Button size="sm" variant="outline" onClick={onRetry}>
          <RotateCcw className="size-4" />
          {t('common.retry')}
        </Button>
      </div>
    )
  }
  return (
    <div ref={viewportRef} aria-live="polite" className="min-h-0 flex-1 overflow-y-auto bg-surface p-4">
      {blocks.length === 0
        ? (
            <div className="grid h-full place-content-center gap-2 text-center text-muted-foreground">
              <Bot className="mx-auto size-8" />
              <p className="text-sm">{t('aiAssistant.empty')}</p>
            </div>
          )
        : <div className="grid gap-3">{blocks.map(block => <TimelineBlock key={block.id} block={block} onAction={onAction} onApproval={onApproval} onMFA={onMFA} />)}</div>}
    </div>
  )
}

function TimelineBlock({ block, onAction, onApproval, onMFA }: { block: AIBlock, onAction: (action: AIUIAction) => Promise<boolean>, onApproval: (block: Extract<AIBlock, { type: 'tool_call' }>, decision: 'approve' | 'reject', reason?: string) => Promise<void>, onMFA: (block: Extract<AIBlock, { type: 'tool_call' }>, code: string) => Promise<void> }) {
  if (block.type === 'thinking')
    return <ThinkingBlock block={block} />
  if (block.type === 'tool_call')
    return <ToolCallCard block={block} onAction={onAction} onApproval={onApproval} onMFA={onMFA} />
  return (
    <div className={cn('flex', block.role === 'user' ? 'justify-end' : 'gap-2')}>
      {block.role === 'assistant' && <span className="grid size-7 shrink-0 place-items-center rounded-control bg-primary-subtle text-primary-text"><Bot className="size-3.5" /></span>}
      <div className={cn('max-w-[86%] whitespace-pre-wrap rounded-container px-3 py-2.5 text-sm leading-6', block.role === 'user' ? 'rounded-br-sm bg-primary text-primary-foreground' : 'rounded-tl-sm bg-surface-subtle')}>{block.text}</div>
    </div>
  )
}

function ThinkingBlock({ block }: { block: Extract<AIBlock, { type: 'thinking' }> }) {
  const { t } = useTranslation()
  const viewportRef = useRef<HTMLDivElement>(null)
  const [following, setFollowing] = useState(true)
  const [expanded, setExpanded] = useState(false)
  useEffect(() => {
    if (following && viewportRef.current)
      viewportRef.current.scrollTop = viewportRef.current.scrollHeight
  }, [block.text, following])
  return (
    <div className="ml-9 rounded-r-container border-l-2 border-primary-border bg-primary-subtle/50 px-3 py-2">
      <button className="flex w-full items-center gap-1.5 text-left text-xs font-medium text-primary-text" type="button" onClick={() => setExpanded(value => !value)}>
        <Sparkles className="size-3.5" />
        <span>{t(block.display === 'summary' ? `aiAssistant.thinking.${block.status === 'completed' ? 'summaryComplete' : 'summaryStreaming'}` : `aiAssistant.thinking.${block.status === 'completed' ? 'progressComplete' : 'progressStreaming'}`)}</span>
        <ChevronRight className={cn('ml-auto size-3.5 transition-transform', expanded && 'rotate-90')} />
      </button>
      <div
        ref={viewportRef}
        className={cn('mt-1.5 overflow-y-auto whitespace-pre-wrap text-xs leading-5 text-muted-foreground', expanded ? 'max-h-64' : 'h-[3.75rem]')}
        onScroll={(event) => {
          const target = event.currentTarget
          setFollowing(target.scrollHeight - target.scrollTop - target.clientHeight < 8)
        }}
      >
        {block.text}
      </div>
      {!following && <button className="mt-1 text-[10px] font-medium text-primary-text" type="button" onClick={() => setFollowing(true)}>{t('aiAssistant.thinking.backToLatest')}</button>}
    </div>
  )
}

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

function ToolCallCard({ block, onAction, onApproval, onMFA }: { block: Extract<AIBlock, { type: 'tool_call' }>, onAction: (action: AIUIAction) => Promise<boolean>, onApproval: (block: Extract<AIBlock, { type: 'tool_call' }>, decision: 'approve' | 'reject', reason?: string) => Promise<void>, onMFA: (block: Extract<AIBlock, { type: 'tool_call' }>, code: string) => Promise<void> }) {
  const { t, i18n } = useTranslation()
  const entries = Object.entries(block.arguments).filter(([key]) => !SECRET_FIELD.test(key)).slice(0, 20)
  const title = block.titleKey && i18n.exists(block.titleKey) ? t(block.titleKey) : block.operationId
  const summary = block.result?.summaryKey && i18n.exists(block.result.summaryKey) ? t(block.result.summaryKey, block.result.summaryParams) : t('aiAssistant.resultAvailable')
  return (
    <details className="group ml-9 overflow-hidden rounded-container border border-border bg-surface" open={block.status === 'awaiting_approval' || block.status === 'awaiting_mfa' ? true : undefined}>
      <summary className="flex min-h-12 cursor-pointer list-none items-center gap-2 px-3 py-2 outline-none hover:bg-surface-subtle focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring [&::-webkit-details-marker]:hidden">
        <ChevronRight className="size-4 shrink-0 text-muted-foreground transition-transform group-open:rotate-90" />
        <span className="grid size-7 shrink-0 place-items-center rounded-control bg-primary-subtle text-primary-text">{block.status === 'succeeded' ? <Check className="size-3.5" /> : <LoaderCircle className={cn('size-3.5', block.status === 'running' && 'animate-spin motion-reduce:animate-none')} />}</span>
        <span className="min-w-0 flex-1">
          <strong className="block truncate text-xs font-medium">{title}</strong>
          <span className="block truncate text-[10px] text-muted-foreground">
            {block.result ? summary : t('aiAssistant.toolNoResult')}
            {block.durationMs ? ` · ${block.durationMs} ms` : ''}
          </span>
        </span>
        <span className={cn('rounded-full px-2 py-0.5 text-[10px] font-medium', statusTone[block.status])}>{t(`aiAssistant.status.${block.status}`)}</span>
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

function ActionButton({ action, onAction }: { action: AIUIAction, onAction: (action: AIUIAction) => Promise<boolean> }) {
  const { t } = useTranslation()
  const [done, setDone] = useState(false)
  const execute = async () => {
    const success = await onAction(action)
    if (success)
      setDone(true)
    else
      toast.error(t('aiAssistant.actions.unavailable'))
  }
  return <Button disabled={done} size="sm" variant="outline" onClick={() => void execute()}>{done ? t('aiAssistant.actions.opened') : t(`aiAssistant.actions.${action.type}`)}</Button>
}

function ApprovalControls({ block, onApproval }: { block: Extract<AIBlock, { type: 'tool_call' }>, onApproval: (block: Extract<AIBlock, { type: 'tool_call' }>, decision: 'approve' | 'reject', reason?: string) => Promise<void> }) {
  const { t } = useTranslation()
  const [reason, setReason] = useState('')
  const [pending, setPending] = useState(false)
  const validBinding = Boolean(block.argumentsHash && block.expectedVersion !== undefined)
  const decide = async (decision: 'approve' | 'reject') => {
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
      <div className="flex justify-end gap-2">
        <Button disabled={pending || !validBinding} size="sm" variant="outline" onClick={() => void decide('reject')}>{t('aiAssistant.approval.reject')}</Button>
        <Button disabled={pending || !validBinding} size="sm" onClick={() => void decide('approve')}>{t('aiAssistant.approval.approve')}</Button>
      </div>
    </div>
  )
}

function MFAControls({ block, onMFA }: { block: Extract<AIBlock, { type: 'tool_call' }>, onMFA: (block: Extract<AIBlock, { type: 'tool_call' }>, code: string) => Promise<void> }) {
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

function ConversationList({ activeId, conversations, loading, search, onClose, onCreate, onDelete, onRename, onSearch, onSelect }: { activeId?: string, conversations: AIConversation[], loading: boolean, search: string, onClose: () => void, onCreate: () => void, onDelete: (id: string) => void, onRename: (id: string, title: string) => void, onSearch: (search: string) => void, onSelect: (id: string) => void }) {
  const { t } = useTranslation()
  const [renamingId, setRenamingId] = useState<string>()
  const [deleting, setDeleting] = useState<AIConversation>()
  const [title, setTitle] = useState('')
  return (
    <aside className="absolute inset-0 z-10 flex flex-col bg-surface sm:static sm:w-64 sm:shrink-0 sm:border-r sm:border-separator-subtle">
      <div className="flex h-14 items-center gap-2 border-b border-separator-subtle px-3">
        <Button aria-label={t('common.close')} size="icon" variant="ghost" onClick={onClose}><ChevronRight className="size-4 rotate-180" /></Button>
        <h2 className="flex-1 text-sm font-semibold">{t('aiAssistant.conversations.title')}</h2>
        <Button aria-label={t('aiAssistant.conversations.new')} size="icon" variant="ghost" onClick={onCreate}><MessageSquarePlus className="size-4" /></Button>
      </div>
      <div className="p-2">
        <div className="relative">
          <Search className="absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input aria-label={t('aiAssistant.conversations.search')} className="h-8 pl-8 text-xs" placeholder={t('aiAssistant.conversations.search')} value={search} onChange={event => onSearch(event.target.value)} />
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto px-2 pb-2">
        {loading && <Skeleton className="h-12 w-full" />}
        {conversations.map(conversation => (
          <div key={conversation.id} className={cn('group flex items-center rounded-control px-2 py-1.5 hover:bg-surface-subtle', activeId === conversation.id && 'bg-primary-subtle text-primary-text')}>
            {renamingId === conversation.id
              ? (
                  <Input
                    autoFocus
                    className="h-8 min-w-0 flex-1 text-xs"
                    value={title}
                    onBlur={() => setRenamingId(undefined)}
                    onChange={event => setTitle(event.target.value)}
                    onKeyDown={(event) => {
                      if (event.key === 'Enter' && title.trim()) {
                        onRename(conversation.id, title.trim())
                        setRenamingId(undefined)
                      }
                    }}
                  />
                )
              : (
                  <button className="min-w-0 flex-1 text-left" type="button" onClick={() => onSelect(conversation.id)}>
                    <strong className="block truncate text-xs font-medium">{conversation.title}</strong>
                    <span className="block text-[10px] text-muted-foreground">{new Date(conversation.updatedAt).toLocaleString()}</span>
                  </button>
                )}
            <DropdownMenu>
              <DropdownMenuTrigger asChild><Button aria-label={t('common.actions')} className="size-7 shrink-0 opacity-0 group-hover:opacity-100 focus:opacity-100" size="icon" variant="ghost"><Ellipsis className="size-3.5" /></Button></DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onClick={() => {
                  setTitle(conversation.title)
                  setRenamingId(conversation.id)
                }}
                >
                  <Pencil className="size-4" />
                  {t('common.edit')}
                </DropdownMenuItem>
                <DropdownMenuItem className="text-danger" onClick={() => setDeleting(conversation)}>
                  <Trash2 className="size-4" />
                  {t('common.delete')}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        ))}
      </div>
      <ConfirmDialog
        confirmText={t('common.delete')}
        description={t('aiAssistant.conversations.deleteDescription', { title: deleting?.title })}
        open={Boolean(deleting)}
        title={t('aiAssistant.conversations.deleteTitle')}
        onConfirm={() => {
          if (deleting)
            onDelete(deleting.id)
          setDeleting(undefined)
        }}
        onOpenChange={open => !open && setDeleting(undefined)}
      />
    </aside>
  )
}
