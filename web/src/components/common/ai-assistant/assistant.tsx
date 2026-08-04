import type { AutomaticRouteDelivery } from './automatic-actions'
import type { LiveSubscription } from './session'
import type { AIEvent, AIUIAction } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Bug, List, LoaderCircle, MessageSquarePlus, Sparkles, X } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useCallback, useEffect, useMemo, useReducer, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Rnd } from 'react-rnd'
import { useLocation, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { api, isUsableAICapabilities } from '@/api'
import { useSession } from '@/app/session-context'
import { Button } from '@/components/ui/button'
import { isPlatformAdmin } from '@/lib/roles'
import { executeAIUIAction } from './actions'
import {
  automaticRouteDeliveryFromEvent,
  automaticRouteDeliveryFromPending,
} from './automatic-actions'
import { executeAutomaticRouteDelivery } from './automatic-route-delivery'
import { readAIClientInstanceId } from './client-instance'
import { AIAssistantComposer } from './composer'
import { AIConversationList } from './conversation-list'
import {
  aiConversationSessionReducer,
  initialAIConversationSessionState,
  isRecentConversationInteraction,
  REFRESH_CONVERSATION_RETURN_DURATION_MS,
} from './conversation-session'
import { AIDesktopShell } from './desktop-shell'
import { AIAssistantLauncher } from './launcher'
import {
  clampAssistantPosition,
  LAUNCHER_SIZE,
  LAUNCHER_STORAGE_KEY,
  MIN_WINDOW_HEIGHT,
  MIN_WINDOW_WIDTH,
  readLauncherPosition,
  readWindowPreference,
  useDesktopViewport,
  VIEWPORT_GUTTER,
  WINDOW_STORAGE_KEY,
} from './layout'
import { AIMobileViewport } from './mobile-viewport'
import { AIOptionsBar } from './options'
import { shouldDisplayAIOptions } from './options-visibility'
import { buildAIPageContext } from './page-context'
import { AIRefreshConversationReturn } from './refresh-conversation-return'
import { AI_EVENT_TYPES, sessionStateReducer } from './session'
import { emptyAIAssistantState, isValidAITimeline } from './state'
import { createAIEventSource } from './stream'
import { resolveAISuggestions } from './suggestions'
import { AIAssistantTimeline } from './timeline'
import { useAIToolDebugMode } from './tool-debug-mode'

type AssistantView = 'chat' | 'conversations'

export function AiAssistant() {
  const { i18n, t } = useTranslation()
  const { actualUser } = useSession()
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const location = useLocation()
  const locationRef = useRef(location)
  locationRef.current = location
  const pageContext = useCallback(() => buildAIPageContext(location.pathname, location.search, i18n.language, {
    hash: location.hash,
  }), [i18n.language, location.hash, location.pathname, location.search])
  const triggerRef = useRef<HTMLButtonElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const conversationButtonRef = useRef<HTMLButtonElement>(null)
  const automaticDeliveryHandlerRef = useRef<((delivery: AutomaticRouteDelivery) => Promise<void>) | undefined>(undefined)
  const processingAutomaticActionsRef = useRef(new Set<string>())
  const desktop = useDesktopViewport()
  const reduceMotion = useReducedMotion()
  const [open, setOpen] = useState(false)
  const [capabilityEpoch, invalidateOpenWindow] = useReducer(value => value + 1, 0)
  const [openedCapabilityEpoch, setOpenedCapabilityEpoch] = useState(0)
  const [assistantView, setAssistantView] = useState<AssistantView>('chat')
  const [conversationSession, dispatchConversationSession] = useReducer(aiConversationSessionReducer, initialAIConversationSessionState)
  const [conversationSearch, setConversationSearch] = useState('')
  const [liveSubscriptions, setLiveSubscriptions] = useState<Record<string, LiveSubscription>>({})
  const [drafts, setDrafts] = useState<Record<string, string>>({})
  const [pendingSends, setPendingSends] = useState<Record<string, number>>({})
  const [streamStates, dispatchStream] = useReducer(sessionStateReducer, {})
  const [preference, setPreference] = useState(readWindowPreference)
  const [launcherPosition, setLauncherPosition] = useState(readLauncherPosition)
  const [clientInstanceId] = useState(readAIClientInstanceId)
  const canDebugInternalTools = isPlatformAdmin(actualUser?.role)
  const toolDebugMode = useAIToolDebugMode(actualUser?.id, canDebugInternalTools)

  const capabilities = useQuery({
    queryKey: ['ai', 'capabilities'],
    queryFn: api.getAICapabilities,
    retry: false,
    staleTime: 30_000,
    refetchInterval: 30_000,
  })
  const available = isUsableAICapabilities(capabilities.data)
  const assistantOpen = open && capabilityEpoch === openedCapabilityEpoch
  const pendingUIActions = useQuery({
    queryKey: ['ai', 'ui-actions', 'pending', clientInstanceId],
    queryFn: () => api.listPendingAIUIActions(clientInstanceId),
    enabled: available,
    refetchInterval: 5_000,
    staleTime: 0,
  })

  const conversations = useQuery({
    queryKey: ['ai', 'conversations', conversationSearch],
    queryFn: () => api.listAIConversations({ page: 1, pageSize: 50, search: conversationSearch || undefined }),
    enabled: available && assistantOpen,
  })
  const previousConversation = conversations.data?.items[0]
  const refreshReturnOpenedAt = conversationSession.refreshReturnExpiresAt
    ? conversationSession.refreshReturnExpiresAt - REFRESH_CONVERSATION_RETURN_DURATION_MS
    : undefined
  const canReturnToPreviousConversation = Boolean(
    previousConversation
    && refreshReturnOpenedAt !== undefined
    && isRecentConversationInteraction(previousConversation.updatedAt, refreshReturnOpenedAt),
  )
  const selectedConversationId = conversationSession.activeConversationId
  const draftKey = selectedConversationId ?? '__new__'
  const draft = drafts[draftKey] ?? ''
  const setDraft = useCallback((value: string) => {
    setDrafts(current => ({ ...current, [draftKey]: value }))
  }, [draftKey])

  const timeline = useQuery({
    queryKey: ['ai', 'timeline', selectedConversationId],
    queryFn: () => api.getAIConversationTimeline(selectedConversationId!),
    enabled: available && assistantOpen && Boolean(selectedConversationId),
  })
  const timelineValid = isValidAITimeline(timeline.data)
  const streamState = selectedConversationId ? streamStates[selectedConversationId] ?? emptyAIAssistantState : emptyAIAssistantState
  const desyncedRunsKey = [...streamState.desyncedRunIds].sort().join(',')
  useEffect(() => {
    if (timelineValid && selectedConversationId)
      dispatchStream({ type: 'snapshot', conversationId: selectedConversationId, timeline: timeline.data! })
  }, [selectedConversationId, timeline.data, timelineValid])
  useEffect(() => {
    if (selectedConversationId && desyncedRunsKey)
      void queryClient.invalidateQueries({ queryKey: ['ai', 'timeline', selectedConversationId] })
  }, [desyncedRunsKey, queryClient, selectedConversationId])

  const subscribe = useCallback((runId: string, conversationId: string, after: number, explicitUrl?: string) => {
    const rawUrl = explicitUrl || `/api/v1/ai/runs/${encodeURIComponent(runId)}/events`
    const source = createAIEventSource(rawUrl, after)
    const receive = (rawEvent: Event) => {
      try {
        const event = JSON.parse((rawEvent as MessageEvent<string>).data) as AIEvent
        dispatchStream({ type: 'event', event })
        const automaticDelivery = automaticRouteDeliveryFromEvent(event)
        if (automaticDelivery && automaticDeliveryHandlerRef.current)
          void automaticDeliveryHandlerRef.current(automaticDelivery)
        if (event.type === 'run.completed' || event.type === 'run.failed' || event.type === 'run.canceled') {
          void queryClient.invalidateQueries({ queryKey: ['ai', 'conversations'] })
          void queryClient.invalidateQueries({ queryKey: ['ai', 'timeline', conversationId] })
          setLiveSubscriptions(current => Object.fromEntries(Object.entries(current).filter(([id]) => id !== runId)))
          source.close()
        }
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
    if (!assistantOpen || !timelineValid || !selectedConversationId || !capabilities.data?.features.streaming)
      return
    const subscriptions = timeline.data!.eventCursors
      .filter(cursor => !liveSubscriptions[cursor.runId])
      .map(cursor => subscribe(cursor.runId, selectedConversationId, cursor.after))
    return () => subscriptions.forEach(source => source.close())
  }, [assistantOpen, capabilities.data?.features.streaming, liveSubscriptions, selectedConversationId, subscribe, timeline.data, timelineValid])
  useEffect(() => {
    if (!assistantOpen)
      return
    const sources = Object.values(liveSubscriptions).map(subscription =>
      subscribe(subscription.runId, subscription.conversationId, 0, subscription.eventsUrl))
    return () => sources.forEach(source => source.close())
  }, [assistantOpen, liveSubscriptions, subscribe])

  const createConversation = useMutation({
    mutationFn: () => api.createAIConversation({ projectId: pageContext().projectId }),
    onMutate: () => dispatchConversationSession({ type: 'start_new' }),
    onSuccess: async (conversation) => {
      await queryClient.invalidateQueries({ queryKey: ['ai', 'conversations'] })
      dispatchConversationSession({ type: 'select', conversationId: conversation.id })
      window.setTimeout(() => inputRef.current?.focus(), 0)
    },
    onError: error => toast.error(error instanceof Error ? error.message : t('aiAssistant.errors.createConversation')),
  })
  const renameConversation = useMutation({
    mutationFn: ({ id, title }: { id: string, title: string }) => api.renameAIConversation(id, title),
    onSuccess: async (conversation) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['ai', 'conversations'] }),
        queryClient.invalidateQueries({ queryKey: ['ai', 'timeline', conversation.id] }),
      ])
    },
    onError: error => toast.error(error instanceof Error ? error.message : t('aiAssistant.errors.renameConversation')),
  })
  const deleteConversations = useMutation({
    mutationFn: async (ids: string[]) => Promise.all(ids.map(id => api.deleteAIConversation(id))),
    onSuccess: async (_, deletedIds) => {
      dispatchConversationSession({ type: 'clear_deleted', conversationIds: deletedIds })
      await queryClient.invalidateQueries({ queryKey: ['ai', 'conversations'] })
    },
    onError: error => toast.error(error instanceof Error ? error.message : t('aiAssistant.errors.deleteConversation')),
  })
  const activeRunId = useMemo(() => {
    const activeFromTimeline = Object.entries(streamState.runStatuses).find(([, status]) => ['queued', 'running', 'waiting_approval', 'waiting_mfa', 'waiting_input'].includes(status))?.[0]
    if (activeFromTimeline)
      return activeFromTimeline
    const liveSubscription = Object.values(liveSubscriptions).find(subscription => subscription.conversationId === selectedConversationId)
    if (liveSubscription && !['completed', 'failed', 'canceled'].includes(streamState.runStatuses[liveSubscription.runId] ?? 'queued'))
      return liveSubscription.runId
    return undefined
  }, [liveSubscriptions, selectedConversationId, streamState.runStatuses])
  const sendTurn = useMutation({
    mutationFn: async ({ conversationId: requestedConversationId, message }: { conversationId?: string, message: string }) => {
      const text = message.trim()
      let conversationId = requestedConversationId
      if (!conversationId) {
        const conversation = await api.createAIConversation({ projectId: pageContext().projectId })
        conversationId = conversation.id
        dispatchConversationSession({ type: 'select', conversationId })
      }
      const result = await api.createAITurn(conversationId, {
        input: { parts: [{ type: 'text', text }] },
        pageContext: pageContext(),
        clientInstanceId,
      }, crypto.randomUUID())
      return { ...result, conversationId, text, sourceDraftKey: requestedConversationId ?? '__new__' }
    },
    onMutate: ({ conversationId }) => {
      const key = conversationId ?? '__new__'
      setPendingSends(current => ({ ...current, [key]: (current[key] ?? 0) + 1 }))
    },
    onSuccess: async (result) => {
      setDrafts(current => ({ ...current, [result.sourceDraftKey]: '', [result.conversationId]: '' }))
      dispatchStream({
        type: 'optimistic_turn',
        conversationId: result.conversationId,
        turnId: result.turnId,
        turnIndex: result.turnIndex,
        runId: result.runId,
        text: result.text,
      })
      setLiveSubscriptions(current => ({
        ...current,
        [result.runId]: { conversationId: result.conversationId, eventsUrl: result.eventsUrl, runId: result.runId },
      }))
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['ai', 'conversations'] }),
        queryClient.invalidateQueries({ queryKey: ['ai', 'timeline', result.conversationId] }),
      ])
    },
    onError: error => toast.error(error instanceof Error ? error.message : t('aiAssistant.errors.send')),
    onSettled: (_data, _error, { conversationId }) => {
      const key = conversationId ?? '__new__'
      setPendingSends((current) => {
        const next = { ...current, [key]: Math.max(0, (current[key] ?? 1) - 1) }
        if (next[key] === 0)
          delete next[key]
        return next
      })
    },
  })
  const cancelRun = useMutation({
    mutationFn: ({ runId }: { runId: string, conversationId: string }) => api.cancelAIRun(runId),
    onSuccess: (_, { conversationId }) => void queryClient.invalidateQueries({ queryKey: ['ai', 'timeline', conversationId] }),
    onError: error => toast.error(error instanceof Error ? error.message : t('aiAssistant.errors.stop')),
  })
  const submitRunInput = useMutation({
    mutationFn: ({ runId, text, expectedVersion }: { runId: string, conversationId: string, text: string, expectedVersion: number }) => api.submitAIRunInput(runId, {
      input: { parts: [{ type: 'text', text }] },
      expectedVersion,
    }),
    onSuccess: (_, { conversationId }) => setDrafts(current => ({ ...current, [conversationId]: '' })),
    onError: error => toast.error(error instanceof Error ? error.message : t('aiAssistant.errors.input')),
  })
  const activeRunStatus = activeRunId ? streamState.runStatuses[activeRunId] ?? 'queued' : undefined
  const generating = activeRunStatus === 'queued' || activeRunStatus === 'running'
  const waitingInput = activeRunStatus === 'waiting_input'
  const sendingSelected = Boolean(pendingSends[draftKey])
  const cancelingSelected = cancelRun.isPending && cancelRun.variables?.runId === activeRunId
  const submittingSelected = submitRunInput.isPending && submitRunInput.variables?.conversationId === selectedConversationId
  const runningConversationIds = useMemo(() => {
    const ids = new Set(Object.values(liveSubscriptions)
      .filter(subscription => ['queued', 'running'].includes(streamStates[subscription.conversationId]?.runStatuses[subscription.runId] ?? 'queued'))
      .map(subscription => subscription.conversationId))
    if (generating && selectedConversationId)
      ids.add(selectedConversationId)
    return ids
  }, [generating, liveSubscriptions, selectedConversationId, streamStates])
  const allowPresetSuggestions = !conversations.isLoading
    && (!selectedConversationId || (timelineValid && timeline.data!.turns.length === 0))
  const suggestions = useMemo(
    () => resolveAISuggestions(streamState.blocks, location.pathname, t, Boolean(activeRunId), allowPresetSuggestions),
    [activeRunId, allowPresetSuggestions, location.pathname, streamState.blocks, t],
  )
  const executeAction = useCallback((action: AIUIAction) => executeAIUIAction(action, {
    pathname: location.pathname,
    search: location.search,
    navigate,
    queryClient,
    sendMessage: async (message) => {
      if (activeRunId)
        throw new Error(t('aiAssistant.actions.runActive'))
      await sendTurn.mutateAsync({ conversationId: selectedConversationId, message })
    },
  }), [activeRunId, location.pathname, location.search, navigate, queryClient, selectedConversationId, sendTurn, t])
  const processAutomaticDelivery = useCallback(async (delivery: AutomaticRouteDelivery) => {
    if (processingAutomaticActionsRef.current.has(delivery.actionId))
      return
    processingAutomaticActionsRef.current.add(delivery.actionId)
    try {
      const success = await executeAutomaticRouteDelivery({
        delivery,
        execute: executeAction,
        currentPath: () => `${locationRef.current.pathname}${locationRef.current.search}`,
        acknowledge: (actionId, acknowledgement) => api.acknowledgeAIUIAction(actionId, {
          ...acknowledgement,
          clientInstanceId,
        }),
      })
      if (!success)
        toast.error(t('aiAssistant.actions.unavailable'))
      await queryClient.invalidateQueries({ queryKey: ['ai', 'ui-actions', 'pending', clientInstanceId] })
    }
    catch (error) {
      toast.error(error instanceof Error ? error.message : t('aiAssistant.actions.unavailable'))
    }
    finally {
      processingAutomaticActionsRef.current.delete(delivery.actionId)
    }
  }, [clientInstanceId, executeAction, queryClient, t])
  useEffect(() => {
    automaticDeliveryHandlerRef.current = processAutomaticDelivery
    return () => {
      automaticDeliveryHandlerRef.current = undefined
    }
  }, [processAutomaticDelivery])
  useEffect(() => {
    pendingUIActions.data?.items.forEach((item) => {
      const delivery = automaticRouteDeliveryFromPending(item)
      if (delivery)
        void processAutomaticDelivery(delivery)
    })
  }, [pendingUIActions.data, processAutomaticDelivery])
  const submitDraft = () => {
    if (waitingInput && activeRunId && selectedConversationId) {
      submitRunInput.mutate({
        runId: activeRunId,
        conversationId: selectedConversationId,
        text: draft.trim(),
        expectedVersion: streamState.runExpectedVersions[activeRunId] ?? -1,
      })
      return
    }
    if (generating)
      return
    sendTurn.mutate({ conversationId: selectedConversationId, message: draft })
  }
  const dismissRefreshReturn = useCallback(() => {
    dispatchConversationSession({ type: 'dismiss_refresh_return' })
  }, [])
  const returnToPreviousConversation = useCallback(() => {
    const previousConversationId = previousConversation?.id
    if (previousConversationId)
      dispatchConversationSession({ type: 'select', conversationId: previousConversationId })
  }, [previousConversation?.id])

  useEffect(() => {
    if (!assistantOpen)
      return
    const timeout = window.setTimeout(() => inputRef.current?.focus(), 0)
    return () => window.clearTimeout(timeout)
  }, [assistantOpen])

  useEffect(() => {
    localStorage.setItem(WINDOW_STORAGE_KEY, JSON.stringify(preference))
  }, [preference])
  useEffect(() => {
    localStorage.setItem(LAUNCHER_STORAGE_KEY, JSON.stringify(launcherPosition))
  }, [launcherPosition])
  useEffect(() => {
    const handleResize = () => {
      setPreference(current => ({
        ...current,
        ...clampAssistantPosition(current, current.width, current.height),
      }))
      setLauncherPosition(current => clampAssistantPosition(current, LAUNCHER_SIZE, LAUNCHER_SIZE))
    }
    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [])

  if (!available)
    return null
  if (!assistantOpen) {
    const openAssistant = () => {
      setOpenedCapabilityEpoch(capabilityEpoch)
      dispatchConversationSession({ type: 'open', now: Date.now() })
      setOpen(true)
    }
    return (
      <AIAssistantLauncher
        ref={triggerRef}
        label={t('aiAssistant.open')}
        position={launcherPosition}
        onOpen={openAssistant}
        onPositionChange={setLauncherPosition}
      />
    )
  }

  const close = () => {
    setOpen(false)
    window.setTimeout(() => triggerRef.current?.focus(), 0)
  }
  const showConversations = assistantView === 'conversations'
  const visibleSuggestions = suggestions && shouldDisplayAIOptions(desktop, showConversations)
    ? suggestions
    : undefined
  const conversationListProps = {
    activeId: selectedConversationId,
    conversations: conversations.data?.items ?? [],
    deleting: deleteConversations.isPending,
    loading: conversations.isLoading,
    search: conversationSearch,
    onDeleteMany: (ids: string[]) => deleteConversations.mutateAsync(ids).then(() => undefined),
    onRename: (id: string, title: string) => renameConversation.mutate({ id, title }),
    runningConversationIds,
    onSearch: setConversationSearch,
    onSelect: (id: string) => dispatchConversationSession({ type: 'select', conversationId: id } as const),
  }
  const chatView = (
    <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
      <header className="ai-assistant-drag-handle flex h-[calc(3.5rem+env(safe-area-inset-top))] shrink-0 select-none items-center gap-2 border-b border-separator-subtle pb-0 pl-[max(0.75rem,env(safe-area-inset-left))] pr-[max(0.75rem,env(safe-area-inset-right))] pt-[env(safe-area-inset-top)] sm:h-14 sm:touch-none sm:px-3 sm:pt-0 sm:cursor-move">
        <Button
          ref={conversationButtonRef}
          aria-expanded={showConversations}
          aria-label={t('aiAssistant.conversations.title')}
          aria-pressed={showConversations}
          className={showConversations ? 'bg-primary-subtle text-primary-text hover:bg-primary-subtle' : undefined}
          size="icon"
          title={t('aiAssistant.conversations.title')}
          variant="ghost"
          onClick={() => setAssistantView(showConversations ? 'chat' : 'conversations')}
        >
          <List className="size-4" />
        </Button>
        <span className="grid size-8 shrink-0 place-items-center rounded-control bg-primary text-primary-foreground"><Sparkles className="size-4" /></span>
        <div className="min-w-0 flex-1">
          <h2 className="truncate text-[13px] font-semibold leading-5">{timeline.data?.conversation.title || t('aiAssistant.title')}</h2>
          <p className="truncate text-[10px] leading-4 text-muted-foreground">{t('aiAssistant.context', { path: location.pathname })}</p>
        </div>
        {canDebugInternalTools && (
          <Button
            aria-label={t(toolDebugMode.enabled ? 'aiAssistant.toolDebug.disable' : 'aiAssistant.toolDebug.enable')}
            aria-pressed={toolDebugMode.enabled}
            className={toolDebugMode.enabled ? 'bg-primary-subtle text-primary-text hover:bg-primary-subtle' : undefined}
            size="icon"
            title={t(toolDebugMode.enabled ? 'aiAssistant.toolDebug.disable' : 'aiAssistant.toolDebug.enable')}
            variant="ghost"
            onClick={toolDebugMode.toggle}
          >
            <Bug className="size-4" />
          </Button>
        )}
        <Button aria-label={t('aiAssistant.conversations.new')} disabled={createConversation.isPending} size="icon" variant="ghost" onClick={() => createConversation.mutate()}>{createConversation.isPending ? <LoaderCircle className="size-4 animate-spin motion-reduce:animate-none" /> : <MessageSquarePlus className="size-4" />}</Button>
        <Button aria-label={t('common.close')} size="icon" variant="ghost" onClick={close}><X className="size-4" /></Button>
      </header>
      <div className="relative flex min-h-0 min-w-0 flex-1 overflow-hidden">
        <AIAssistantTimeline
          bottomInset={Boolean(visibleSuggestions)}
          blocks={streamState.blocks}
          error={timeline.error ?? (timeline.data && !timelineValid ? new Error('ai_invalid_timeline') : null)}
          generating={generating}
          loading={timeline.isLoading}
          onAction={executeAction}
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
          onResend={message => sendTurn.mutate({ conversationId: selectedConversationId, message })}
          onRetry={() => void timeline.refetch()}
          resendDisabled={Boolean(activeRunId || sendingSelected)}
          showInternalTools={toolDebugMode.enabled}
          topContent={conversationSession.refreshReturnExpiresAt && canReturnToPreviousConversation && !selectedConversationId && !conversationSearch
            ? (
                <AIRefreshConversationReturn
                  expiresAt={conversationSession.refreshReturnExpiresAt}
                  onExpire={dismissRefreshReturn}
                  onReturn={returnToPreviousConversation}
                />
              )
            : undefined}
        />
        {visibleSuggestions && (
          <AIOptionsBar
            actions={visibleSuggestions.actions}
            sourceKey={visibleSuggestions.sourceKey}
            onAction={executeAction}
          />
        )}
      </div>
      <AIAssistantComposer
        activeRun={Boolean(activeRunId)}
        canceling={cancelingSelected}
        canCancel={Boolean(activeRunId && selectedConversationId)}
        draft={draft}
        inputRef={inputRef}
        maxLength={capabilities.data?.limits.maxInputBytes}
        sending={sendingSelected}
        submitting={submittingSelected}
        waitingInput={waitingInput}
        onCancel={() => {
          if (activeRunId && selectedConversationId)
            cancelRun.mutate({ runId: activeRunId, conversationId: selectedConversationId })
        }}
        onDraftChange={setDraft}
        onSubmit={submitDraft}
      />
    </div>
  )
  const panel = (
    <section
      aria-label={t('aiAssistant.title')}
      className={desktop
        ? 'relative flex size-full overflow-hidden rounded-feature border border-border bg-surface text-[13px] shadow-overlay'
        : 'ai-assistant-mobile relative flex size-full overflow-hidden bg-surface text-[13px] [&_input]:!text-base [&_select]:!text-base [&_textarea]:!text-base'}
    >
      {desktop
        ? (
            <AIDesktopShell
              chat={chatView}
              closeLabel={t('common.back')}
              conversationsOpen={showConversations}
              initialWidth={preference.width}
              listButtonRef={conversationButtonRef}
              conversationList={variant => (
                <AIConversationList
                  {...conversationListProps}
                  variant={variant}
                  onBack={() => setAssistantView('chat')}
                />
              )}
              onCloseConversations={() => setAssistantView('chat')}
              onOpenConversations={() => setAssistantView('conversations')}
            />
          )
        : (
            <AnimatePresence initial={false} mode="popLayout">
              <motion.div
                key={assistantView}
                animate={{ opacity: 1, x: 0 }}
                className="relative flex size-full min-w-0 overflow-hidden"
                exit={reduceMotion ? { opacity: 0 } : { opacity: 0, x: showConversations ? -32 : 32 }}
                initial={reduceMotion ? { opacity: 0 } : { opacity: 0, x: showConversations ? -32 : 32 }}
                transition={reduceMotion ? { duration: 0.1 } : { type: 'spring', stiffness: 420, damping: 34, mass: 0.8 }}
              >
                {showConversations
                  ? (
                      <AIConversationList
                        {...conversationListProps}
                        variant="mobile"
                        onBack={() => setAssistantView('chat')}
                        onClose={close}
                      />
                    )
                  : chatView}
              </motion.div>
            </AnimatePresence>
          )}
    </section>
  )

  if (!desktop)
    return <AIMobileViewport>{panel}</AIMobileViewport>

  const panelPosition = clampAssistantPosition(preference, preference.width, preference.height)
  return (
    <div className="pointer-events-none fixed inset-0 z-40">
      <Rnd
        bounds="parent"
        cancel="button,input,textarea,select,a,[role='button'],[role='menuitem']"
        className="pointer-events-auto"
        dragHandleClassName="ai-assistant-drag-handle"
        maxHeight={`calc(100dvh - ${VIEWPORT_GUTTER * 2}px)`}
        maxWidth={`calc(100vw - ${VIEWPORT_GUTTER * 2}px)`}
        minHeight={MIN_WINDOW_HEIGHT}
        minWidth={MIN_WINDOW_WIDTH}
        position={panelPosition}
        size={{ width: preference.width, height: preference.height }}
        onDragStop={(_, data) => setPreference(current => ({ ...current, x: data.x, y: data.y }))}
        onResizeStop={(_, __, element, ___, position) => {
          setPreference({
            x: position.x,
            y: position.y,
            width: element.offsetWidth,
            height: element.offsetHeight,
          })
        }}
      >
        {panel}
      </Rnd>
    </div>
  )
}
