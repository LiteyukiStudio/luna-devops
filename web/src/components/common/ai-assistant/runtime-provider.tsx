import type { ReactNode } from 'react'
import type { AutomaticRouteDelivery } from './automatic-actions'
import type { AIAssistantWorkspaceLocation } from './runtime-context'
import type { AITimelineInfiniteData, AITimelineQueryData } from './timeline-query'
import type { AICapabilities, AIUIAction } from '@/api'
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useReducer, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { isPlatformAdmin } from '@/lib/roles'
import { executeAIUIAction } from './actions'
import {
  automaticRouteDeliveryFromEvent,
  automaticRouteDeliveryFromPending,
} from './automatic-actions'
import { executeAutomaticRouteDelivery } from './automatic-route-delivery'
import { readAIClientInstanceId } from './client-instance'
import {
  aiConversationSessionReducer,
  initialAIConversationSessionState,
  isRecentConversationInteraction,
  REFRESH_CONVERSATION_RETURN_DURATION_MS,
} from './conversation-session'
import { AI_ASSISTANT_OPEN_EVENT } from './events'
import { aiConversationModelKey, resolveAIConversationModel } from './model-selection'
import { buildAIPageContext } from './page-context'
import { pendingUIActionsPollInterval } from './pending-ui-actions-query'
import { AIRefreshConversationReturn } from './refresh-conversation-return'
import { isAIAssistantRoutePath, readAIAssistantRouteState } from './route-state'
import { useAIRunStreamManager } from './run-stream-manager'
import { AIAssistantRuntimeContext } from './runtime-context'
import { emptyAIAssistantState } from './state'
import { resolveAIPresetSuggestions } from './suggestions'
import {
  activeRunStreamSubscriptions,
  addOptimisticTimelineTurn,
  AI_TIMELINE_PAGE_SIZE,
  aiTimelineQueryKey,
  applyTimelineInfiniteEvent,
  mergeLatestTimelineSnapshot,
  mergeTimelineInfiniteSnapshot,
  olderTimelinePageParam,
  recoverTimelineOnce,
  runStreamRecoveryFromTimeline,
  timelineQueryDataFromInfinite,
  timelineQueryDataFromSnapshot,
} from './timeline-query'
import { useAIToolDebugMode } from './tool-debug-mode'

export interface AIAssistantRuntimeProviderProps {
  capabilities?: AICapabilities
  children: ReactNode
  initiallyOpen?: boolean
}

type RuntimeConversationSessionAction
  = Parameters<typeof aiConversationSessionReducer>[1]
    | { type: 'reset' }

function runtimeConversationSessionReducer(
  state: typeof initialAIConversationSessionState,
  action: RuntimeConversationSessionAction,
) {
  return action.type === 'reset'
    ? initialAIConversationSessionState
    : aiConversationSessionReducer(state, action)
}

const INVALID_ACTION_SESSION_GENERATION = -1

export function AIAssistantRuntimeProvider({ capabilities, children, initiallyOpen = false }: AIAssistantRuntimeProviderProps) {
  const { i18n, t } = useTranslation()
  const { actualUser } = useSession()
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const location = useLocation()
  const enabled = capabilities?.enabled === true
  const currentWorkspaceLocation = useMemo(() => ({
    hash: location.hash,
    pathname: location.pathname,
    search: location.search,
  }), [location.hash, location.pathname, location.search])
  const assistantRoute = isAIAssistantRoutePath(location.pathname)
  const rememberedWorkspaceLocationRef = useRef(assistantRoute
    ? readAIAssistantRouteState(location.state).returnTo
    : currentWorkspaceLocation)
  const [, refreshWorkspaceLocation] = useReducer(version => version + 1, 0)
  const workspaceLocation = assistantRoute
    ? readAIAssistantRouteState(location.state).returnTo
    : currentWorkspaceLocation
  const locationRef = useRef(location)
  locationRef.current = location
  const pageContext = useCallback(() => buildAIPageContext(workspaceLocation.pathname, workspaceLocation.search, i18n.language, {
    hash: workspaceLocation.hash,
  }), [i18n.language, workspaceLocation.hash, workspaceLocation.pathname, workspaceLocation.search])
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const automaticDeliveryHandlerRef = useRef<((delivery: AutomaticRouteDelivery, actionGeneration: number) => Promise<void>) | undefined>(undefined)
  const processingAutomaticActionsRef = useRef(new Set<string>())
  const actionSessionGenerationRef = useRef(0)
  const automaticActionGenerationsRef = useRef(new Map<string, number>())
  const runActionGenerationsRef = useRef(new Map<string, number>())
  const pageTransitionPendingRef = useRef(false)
  const timelineRecoveriesRef = useRef(new Set<string>())
  const [open, setOpen] = useState(initiallyOpen && enabled)
  const surfaceVisible = (enabled && open) || assistantRoute
  const enabledRef = useRef(enabled)
  const surfaceVisibleRef = useRef(surfaceVisible)
  enabledRef.current = enabled
  surfaceVisibleRef.current = surfaceVisible
  const [conversationSession, dispatchConversationSession] = useReducer(runtimeConversationSessionReducer, initialAIConversationSessionState)
  const [conversationSearch, setConversationSearch] = useState('')
  const [drafts, setDrafts] = useState<Record<string, string>>({})
  const [pendingSends, setPendingSends] = useState<Record<string, number>>({})
  const [modelSelectionOverrides, setModelSelectionOverrides] = useState<Record<string, string>>({})
  const [clientInstanceId] = useState(readAIClientInstanceId)
  const [previousEnabled, setPreviousEnabled] = useState(enabled)
  if (previousEnabled !== enabled) {
    setPreviousEnabled(enabled)
    if (!enabled) {
      actionSessionGenerationRef.current += 1
      automaticActionGenerationsRef.current.clear()
      runActionGenerationsRef.current.clear()
      pageTransitionPendingRef.current = false
      setOpen(false)
      dispatchConversationSession({ type: 'reset' })
    }
  }
  const canDebugInternalTools = isPlatformAdmin(actualUser?.role)
  const toolDebugMode = useAIToolDebugMode(actualUser?.id, canDebugInternalTools)
  const invalidateActionSession = useCallback(() => {
    actionSessionGenerationRef.current += 1
  }, [])
  const isCurrentActionSession = useCallback((generation: number) => (
    enabledRef.current
    && surfaceVisibleRef.current
    && actionSessionGenerationRef.current === generation
  ), [])
  useEffect(() => {
    if (assistantRoute && enabled) {
      dispatchConversationSession({ type: 'open', now: Date.now() })
      if (pageTransitionPendingRef.current) {
        pageTransitionPendingRef.current = false
        setOpen(false)
      }
    }
  }, [assistantRoute, enabled])
  const previousAssistantRouteRef = useRef(assistantRoute)
  useEffect(() => {
    const wasAssistantRoute = previousAssistantRouteRef.current
    previousAssistantRouteRef.current = assistantRoute
    if (wasAssistantRoute && !assistantRoute)
      invalidateActionSession()
  }, [assistantRoute, invalidateActionSession])
  useEffect(() => {
    if (!assistantRoute)
      rememberedWorkspaceLocationRef.current = currentWorkspaceLocation
  }, [assistantRoute, currentWorkspaceLocation])
  const aiModels = useQuery({
    queryKey: ['ai', 'models'],
    queryFn: api.listAIModels,
    enabled: enabled && surfaceVisible,
    staleTime: 0,
    retry: false,
  })
  const pendingUIActions = useQuery({
    queryKey: ['ai', 'ui-actions', 'pending', clientInstanceId],
    queryFn: async ({ signal }) => {
      const actionGeneration = actionSessionGenerationRef.current
      const response = await api.listPendingAIUIActions(clientInstanceId, signal)
      response.items.forEach((item) => {
        if (!automaticActionGenerationsRef.current.has(item.actionId)) {
          automaticActionGenerationsRef.current.set(
            item.actionId,
            runActionGenerationsRef.current.get(item.runId) ?? actionGeneration,
          )
        }
      })
      return response
    },
    enabled: enabled && surfaceVisible,
    retry: false,
    refetchInterval: query => pendingUIActionsPollInterval(query.state.data, Boolean(query.state.error)),
    staleTime: 0,
  })

  const conversations = useInfiniteQuery({
    queryKey: ['ai', 'conversations', conversationSearch],
    queryFn: ({ pageParam }) => api.listAIConversations({ page: pageParam, pageSize: 50, search: conversationSearch || undefined }),
    enabled: enabled && surfaceVisible,
    retry: false,
    initialPageParam: 1,
    getNextPageParam: page => page.page < page.totalPages ? page.page + 1 : undefined,
  })
  const conversationItems = useMemo(() => {
    const seen = new Set<string>()
    return conversations.data?.pages.flatMap(page => page.items.filter((conversation) => {
      if (seen.has(conversation.id))
        return false
      seen.add(conversation.id)
      return true
    })) ?? []
  }, [conversations.data?.pages])
  const previousConversation = conversationItems[0]
  const refreshReturnOpenedAt = conversationSession.refreshReturnExpiresAt
    ? conversationSession.refreshReturnExpiresAt - REFRESH_CONVERSATION_RETURN_DURATION_MS
    : undefined
  const canReturnToPreviousConversation = Boolean(
    previousConversation
    && refreshReturnOpenedAt !== undefined
    && isRecentConversationInteraction(previousConversation.updatedAt, refreshReturnOpenedAt),
  )
  const selectedConversationId = conversationSession.activeConversationId
  const draftKey = aiConversationModelKey(selectedConversationId)
  const draft = drafts[draftKey] ?? ''
  const setDraft = useCallback((value: string) => {
    setDrafts(current => ({ ...current, [draftKey]: value }))
  }, [draftKey])

  const timeline = useInfiniteQuery<AITimelineQueryData, Error, AITimelineInfiniteData, ReturnType<typeof aiTimelineQueryKey>, string | null>({
    queryKey: aiTimelineQueryKey(selectedConversationId),
    queryFn: async ({ pageParam }) => timelineQueryDataFromSnapshot(await api.getAIConversationTimeline(selectedConversationId!, {
      before: pageParam ?? undefined,
      limit: AI_TIMELINE_PAGE_SIZE,
    })),
    enabled: enabled && surfaceVisible && Boolean(selectedConversationId),
    retry: false,
    initialPageParam: null as string | null,
    getPreviousPageParam: olderTimelinePageParam,
    getNextPageParam: () => undefined,
    structuralSharing: (current, incoming) => mergeTimelineInfiniteSnapshot(
      current as AITimelineInfiniteData | undefined,
      incoming as AITimelineInfiniteData,
    ),
  })
  const timelineData = useMemo(() => timelineQueryDataFromInfinite(timeline.data), [timeline.data])
  const selectedConversation = useMemo(
    () => conversationItems.find(conversation => conversation.id === selectedConversationId),
    [conversationItems, selectedConversationId],
  )
  const timelineSnapshotConversation = timelineData?.snapshot?.conversation
  const timelineConversation = timelineSnapshotConversation?.id === selectedConversationId
    ? timelineSnapshotConversation
    : undefined
  const selectedModel = useMemo(() => resolveAIConversationModel(
    aiModels.data ?? [],
    selectedConversationId,
    timelineConversation?.modelId ?? selectedConversation?.modelId,
    modelSelectionOverrides,
  ), [aiModels.data, modelSelectionOverrides, selectedConversation?.modelId, selectedConversationId, timelineConversation?.modelId])
  const newConversationModel = useMemo(() => resolveAIConversationModel(
    aiModels.data ?? [],
    undefined,
    undefined,
    modelSelectionOverrides,
  ), [aiModels.data, modelSelectionOverrides])
  const streamState = timelineData?.state ?? emptyAIAssistantState
  const recoverTimeline = useCallback((conversationId: string) => recoverTimelineOnce(
    timelineRecoveriesRef.current,
    conversationId,
    async () => {
      const latest = timelineQueryDataFromSnapshot(await api.getAIConversationTimeline(conversationId, { limit: AI_TIMELINE_PAGE_SIZE }))
      let recovered = latest
      queryClient.setQueryData<AITimelineInfiniteData>(aiTimelineQueryKey(conversationId), (current) => {
        const next = mergeLatestTimelineSnapshot(current, latest)
        recovered = timelineQueryDataFromInfinite(next) ?? latest
        return next
      })
      return recovered
    },
  ), [queryClient])
  const handleStreamEvent = useCallback((event: Parameters<typeof applyTimelineInfiniteEvent>[1]) => {
    let desynced = false
    let acknowledged = false
    let projected = false
    queryClient.setQueryData<AITimelineInfiniteData>(aiTimelineQueryKey(event.conversationId), (current) => {
      const next = applyTimelineInfiniteEvent(current, event)
      if (!next)
        return current
      const nextData = timelineQueryDataFromInfinite(next)
      acknowledged = (nextData?.state.lastEventSequences[event.runId] ?? 0) >= event.eventSequence
      desynced = !acknowledged && (nextData?.state.desyncedRunIds.has(event.runId) ?? false)
      projected = next !== current
        && acknowledged
      return next
    })
    const automaticDelivery = projected
      ? automaticRouteDeliveryFromEvent(event)
      : undefined
    if (automaticDelivery) {
      const actionGeneration = runActionGenerationsRef.current.get(event.runId) ?? INVALID_ACTION_SESSION_GENERATION
      if (!automaticActionGenerationsRef.current.has(automaticDelivery.actionId))
        automaticActionGenerationsRef.current.set(automaticDelivery.actionId, actionGeneration)
      if (enabledRef.current && surfaceVisibleRef.current && automaticDeliveryHandlerRef.current)
        void automaticDeliveryHandlerRef.current(automaticDelivery, actionGeneration)
    }
    if (event.type === 'conversation.title.updated') {
      void queryClient.invalidateQueries({ queryKey: ['ai', 'conversations'] })
      void queryClient.invalidateQueries({ queryKey: aiTimelineQueryKey(event.conversationId) })
    }
    if (event.type === 'run.completed' || event.type === 'run.failed' || event.type === 'run.canceled' || event.type === 'run.interrupted') {
      runActionGenerationsRef.current.delete(event.runId)
      void queryClient.invalidateQueries({ queryKey: ['ai', 'conversations'] })
      void queryClient.invalidateQueries({ queryKey: aiTimelineQueryKey(event.conversationId) })
    }
    return { accepted: acknowledged, desynced }
  }, [queryClient])
  const {
    connect: connectRunStream,
    reconnect: reconnectRunStream,
    streamStates: runStreamStates,
    subscriptions: runSubscriptions,
    syncConversation: syncRunStreams,
  } = useAIRunStreamManager({
    // AiAssistant 在关闭时仍常驻布局；保持已知 Run 的唯一流连接，避免关窗后
    // 后台任务失去事件。组件真正卸载时 manager 才统一关闭连接。
    enabled,
    onEvent: handleStreamEvent,
    onCapabilitiesChanged: () => void queryClient.invalidateQueries({ queryKey: ['ai', 'capabilities'] }),
    onMalformedEvent: async (subscription) => {
      await recoverTimeline(subscription.conversationId)
    },
    onSequenceGap: async (subscription) => {
      const recovered = await recoverTimeline(subscription.conversationId)
      return runStreamRecoveryFromTimeline(recovered, subscription)
    },
  })

  useEffect(() => {
    if (!enabled || !selectedConversationId)
      return
    syncRunStreams(selectedConversationId, activeRunStreamSubscriptions(timelineData, selectedConversationId, runSubscriptions))
  }, [enabled, runSubscriptions, selectedConversationId, syncRunStreams, timelineData])

  const createConversation = useMutation({
    mutationFn: ({ modelId }: { modelId: string }) => api.createAIConversation({ modelId, projectId: pageContext().projectId }),
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
        queryClient.invalidateQueries({ queryKey: aiTimelineQueryKey(conversation.id) }),
      ])
    },
    onError: error => toast.error(error instanceof Error ? error.message : t('aiAssistant.errors.renameConversation')),
  })
  const updateConversationModel = useMutation({
    mutationFn: ({ conversationId, modelId }: { conversationId: string, modelId: string }) => api.updateAIConversation(conversationId, { modelId }),
    onSuccess: async (_, { conversationId, modelId }) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['ai', 'conversations'] }),
        queryClient.invalidateQueries({ queryKey: aiTimelineQueryKey(conversationId) }),
      ])
      setModelSelectionOverrides((current) => {
        const key = aiConversationModelKey(conversationId)
        if (current[key] !== modelId)
          return current
        const next = { ...current }
        delete next[key]
        return next
      })
    },
    onError: (error, { conversationId, modelId }) => {
      setModelSelectionOverrides((current) => {
        const key = aiConversationModelKey(conversationId)
        if (current[key] !== modelId)
          return current
        const next = { ...current }
        delete next[key]
        return next
      })
      toast.error(error instanceof Error ? error.message : t('aiAssistant.errors.updateModel'))
    },
  })
  const deleteConversations = useMutation({
    mutationFn: async (ids: string[]) => Promise.all(ids.map(id => api.deleteAIConversation(id))),
    onSuccess: async (_, deletedIds) => {
      dispatchConversationSession({ type: 'clear_deleted', conversationIds: deletedIds })
      deletedIds.forEach((conversationId) => {
        syncRunStreams(conversationId, [])
        queryClient.removeQueries({ queryKey: aiTimelineQueryKey(conversationId), exact: true })
      })
      await queryClient.invalidateQueries({ queryKey: ['ai', 'conversations'] })
    },
    onError: error => toast.error(error instanceof Error ? error.message : t('aiAssistant.errors.deleteConversation')),
  })
  const activeRunId = useMemo(() => {
    const activeFromTimeline = Object.entries(streamState.runStatuses).find(([, status]) => ['queued', 'running', 'waiting_approval', 'waiting_input'].includes(status))?.[0]
    if (activeFromTimeline)
      return activeFromTimeline
    const liveSubscription = runSubscriptions.find(subscription => subscription.conversationId === selectedConversationId)
    if (liveSubscription && !['completed', 'failed', 'canceled', 'interrupted'].includes(streamState.runStatuses[liveSubscription.runId] ?? 'queued'))
      return liveSubscription.runId
    return undefined
  }, [runSubscriptions, selectedConversationId, streamState.runStatuses])
  const sendTurn = useMutation({
    mutationFn: async ({ actionGeneration, conversationId: requestedConversationId, message }: { actionGeneration: number, conversationId?: string, message: string }) => {
      const text = message.trim()
      if (!selectedModel)
        throw new Error(t('aiAssistant.modelUnavailable'))
      let conversationId = requestedConversationId
      if (!conversationId) {
        const conversation = await api.createAIConversation({ modelId: selectedModel.id, projectId: pageContext().projectId })
        conversationId = conversation.id
        dispatchConversationSession({ type: 'select', conversationId })
      }
      const result = await api.createAITurn(conversationId, {
        modelId: selectedModel.id,
        input: { parts: [{ type: 'text', text }] },
        pageContext: pageContext(),
        clientInstanceId,
      }, crypto.randomUUID())
      return { ...result, actionGeneration, conversationId, text, sourceDraftKey: aiConversationModelKey(requestedConversationId) }
    },
    onMutate: ({ conversationId }) => {
      const key = aiConversationModelKey(conversationId)
      setPendingSends(current => ({ ...current, [key]: (current[key] ?? 0) + 1 }))
    },
    onSuccess: async (result) => {
      runActionGenerationsRef.current.set(result.runId, result.actionGeneration)
      setDrafts(current => ({ ...current, [result.sourceDraftKey]: '', [result.conversationId]: '' }))
      queryClient.setQueryData<AITimelineInfiniteData>(aiTimelineQueryKey(result.conversationId), current => addOptimisticTimelineTurn(current, {
        turnId: result.turnId,
        turnIndex: result.turnIndex,
        runId: result.runId,
        text: result.text,
      }))
      connectRunStream({ conversationId: result.conversationId, eventsUrl: result.eventsUrl, runId: result.runId, after: 0 })
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['ai', 'conversations'] }),
        queryClient.invalidateQueries({ queryKey: aiTimelineQueryKey(result.conversationId) }),
      ])
    },
    onError: (error, { actionGeneration }) => {
      if (isCurrentActionSession(actionGeneration))
        toast.error(error instanceof Error ? error.message : t('aiAssistant.errors.send'))
    },
    onSettled: (_data, _error, { conversationId }) => {
      const key = aiConversationModelKey(conversationId)
      setPendingSends((current) => {
        const next = { ...current, [key]: Math.max(0, (current[key] ?? 1) - 1) }
        if (next[key] === 0)
          delete next[key]
        return next
      })
    },
  })
  const sendTurnAsync = sendTurn.mutateAsync
  const cancelRun = useMutation({
    mutationFn: ({ runId }: { runId: string, conversationId: string }) => api.cancelAIRun(runId),
    onSuccess: (_, { conversationId }) => void queryClient.invalidateQueries({ queryKey: aiTimelineQueryKey(conversationId) }),
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
  const activeRunStreamState = activeRunId ? runStreamStates.find(state => state.runId === activeRunId) : undefined
  const timelineActiveTurnId = timelineData?.snapshot?.turns.find(turn => turn.selectedRun?.id === activeRunId)?.id
  const activeTurnId = activeRunId ? timelineActiveTurnId ?? streamState.blocks.at(-1)?.turnId : undefined
  const generating = activeRunStatus === 'queued' || activeRunStatus === 'running'
  const waitingInput = activeRunStatus === 'waiting_input'
  const sendingSelected = Boolean(pendingSends[draftKey])
  const cancelingSelected = cancelRun.isPending && cancelRun.variables?.runId === activeRunId
  const submittingSelected = submitRunInput.isPending && submitRunInput.variables?.conversationId === selectedConversationId
  const modelChangingSelected = updateConversationModel.isPending && updateConversationModel.variables?.conversationId === selectedConversationId
  const runningConversationIds = useMemo(() => {
    const ids = new Set(runSubscriptions.map(subscription => subscription.conversationId))
    if (generating && selectedConversationId)
      ids.add(selectedConversationId)
    return ids
  }, [generating, runSubscriptions, selectedConversationId])

  useEffect(() => {
    if (!selectedConversationId || !activeRunId || activeRunStreamState?.status !== 'stalled')
      return
    void recoverTimeline(selectedConversationId).finally(() => reconnectRunStream(activeRunId))
  }, [activeRunId, activeRunStreamState?.status, reconnectRunStream, recoverTimeline, selectedConversationId])
  const allowPresetSuggestions = !conversations.isLoading
    && (!selectedConversationId || timelineData?.snapshot?.turns.length === 0)
    && !activeRunId
  const suggestions = useMemo(
    () => resolveAIPresetSuggestions(workspaceLocation.pathname, t, allowPresetSuggestions),
    [allowPresetSuggestions, t, workspaceLocation.pathname],
  )
  const { mutateAsync: requestToolActionAsync } = useMutation({
    mutationFn: async ({
      action,
      actionGeneration,
      conversationId,
    }: {
      action: Extract<AIUIAction, { type: 'request_tool' }>
      actionGeneration: number
      conversationId: string
    }) => {
      const result = await api.executeAIToolAction(conversationId, {
        operationId: action.payload.operationId,
        arguments: action.payload.arguments ?? {},
        message: action.payload.message,
        clientInstanceId,
      }, crypto.randomUUID())
      return { ...result, action, actionGeneration, conversationId }
    },
    onSuccess: async (result) => {
      runActionGenerationsRef.current.set(result.runId, result.actionGeneration)
      queryClient.setQueryData<AITimelineInfiniteData>(aiTimelineQueryKey(result.conversationId), current => addOptimisticTimelineTurn(current, {
        turnId: result.turnId,
        turnIndex: result.turnIndex,
        runId: result.runId,
        text: result.action.payload.message,
      }))
      connectRunStream({ conversationId: result.conversationId, eventsUrl: result.eventsUrl, runId: result.runId, after: 0 })
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['ai', 'conversations'] }),
        queryClient.invalidateQueries({ queryKey: aiTimelineQueryKey(result.conversationId) }),
      ])
    },
  })
  const executeAction = useCallback((action: AIUIAction) => {
    const actionGeneration = actionSessionGenerationRef.current
    if (!isCurrentActionSession(actionGeneration))
      return Promise.resolve(false)
    return executeAIUIAction(action, {
      pathname: workspaceLocation.pathname,
      search: workspaceLocation.search,
      navigate,
      queryClient,
      sendMessage: async (message) => {
        if (activeRunId)
          throw new Error(t('aiAssistant.actions.runActive'))
        await sendTurnAsync({ actionGeneration, conversationId: selectedConversationId, message })
      },
      requestTool: async (action) => {
        if (activeRunId)
          throw new Error(t('aiAssistant.actions.runActive'))
        if (!selectedConversationId)
          throw new Error('ai.conversation_missing')
        await requestToolActionAsync({ action, actionGeneration, conversationId: selectedConversationId })
      },
    })
  }, [activeRunId, isCurrentActionSession, navigate, queryClient, requestToolActionAsync, selectedConversationId, sendTurnAsync, t, workspaceLocation.pathname, workspaceLocation.search])
  const processAutomaticDelivery = useCallback(async (delivery: AutomaticRouteDelivery, actionGeneration: number) => {
    if (processingAutomaticActionsRef.current.has(delivery.actionId))
      return
    processingAutomaticActionsRef.current.add(delivery.actionId)
    let terminallyAcknowledged = false
    try {
      const success = await executeAutomaticRouteDelivery({
        delivery,
        execute: action => isCurrentActionSession(actionGeneration)
          ? executeAction(action)
          : Promise.resolve(false),
        currentPath: () => `${locationRef.current.pathname}${locationRef.current.search}`,
        acknowledge: async (actionId, acknowledgement) => {
          await api.acknowledgeAIUIAction(actionId, {
            ...acknowledgement,
            clientInstanceId,
          })
          terminallyAcknowledged = true
        },
      })
      if (!success && isCurrentActionSession(actionGeneration))
        toast.error(t('aiAssistant.actions.unavailable'))
      await queryClient.invalidateQueries({ queryKey: ['ai', 'ui-actions', 'pending', clientInstanceId] })
    }
    catch (error) {
      if (isCurrentActionSession(actionGeneration))
        toast.error(error instanceof Error ? error.message : t('aiAssistant.actions.unavailable'))
    }
    finally {
      if (terminallyAcknowledged)
        automaticActionGenerationsRef.current.delete(delivery.actionId)
      processingAutomaticActionsRef.current.delete(delivery.actionId)
    }
  }, [clientInstanceId, executeAction, isCurrentActionSession, queryClient, t])
  useEffect(() => {
    automaticDeliveryHandlerRef.current = processAutomaticDelivery
    return () => {
      automaticDeliveryHandlerRef.current = undefined
    }
  }, [processAutomaticDelivery])
  useEffect(() => {
    pendingUIActions.data?.items.forEach((item) => {
      const delivery = automaticRouteDeliveryFromPending(item)
      if (delivery) {
        const actionGeneration = automaticActionGenerationsRef.current.get(item.actionId) ?? INVALID_ACTION_SESSION_GENERATION
        void processAutomaticDelivery(delivery, actionGeneration)
      }
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
    sendTurn.mutate({
      actionGeneration: actionSessionGenerationRef.current,
      conversationId: selectedConversationId,
      message: draft,
    })
  }
  const selectModel = (modelId: string) => {
    const key = aiConversationModelKey(selectedConversationId)
    setModelSelectionOverrides(current => ({ ...current, [key]: modelId }))
    if (selectedConversationId)
      updateConversationModel.mutate({ conversationId: selectedConversationId, modelId })
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
    if (!open || assistantRoute)
      return
    const timeout = window.setTimeout(() => inputRef.current?.focus(), 0)
    return () => window.clearTimeout(timeout)
  }, [assistantRoute, open])

  const openAssistant = useCallback(() => {
    if (!enabledRef.current)
      return
    pageTransitionPendingRef.current = false
    dispatchConversationSession({ type: 'open', now: Date.now() })
    setOpen(true)
  }, [])
  // 监听侧边栏等入口派发的打开请求，与悬浮球打开行为一致
  useEffect(() => {
    const handleOpenRequest = () => openAssistant()
    window.addEventListener(AI_ASSISTANT_OPEN_EVENT, handleOpenRequest)
    return () => window.removeEventListener(AI_ASSISTANT_OPEN_EVENT, handleOpenRequest)
  }, [openAssistant])

  const closeAssistant = useCallback(() => {
    pageTransitionPendingRef.current = false
    invalidateActionSession()
    setOpen(false)
  }, [invalidateActionSession])
  const transitionAssistantToPage = useCallback(() => {
    if (enabledRef.current)
      pageTransitionPendingRef.current = true
  }, [])
  const selectConversation = useCallback((conversationId: string) => {
    dispatchConversationSession({ type: 'select', conversationId })
  }, [])
  const startNewConversation = useCallback(() => {
    dispatchConversationSession({ type: 'start_new' })
  }, [])
  const rememberWorkspaceLocation = useCallback((nextLocation: AIAssistantWorkspaceLocation) => {
    if (isAIAssistantRoutePath(nextLocation.pathname))
      return
    const current = rememberedWorkspaceLocationRef.current
    if (
      current.pathname === nextLocation.pathname
      && current.search === nextLocation.search
      && current.hash === nextLocation.hash
    ) {
      return
    }
    rememberedWorkspaceLocationRef.current = {
      hash: nextLocation.hash,
      pathname: nextLocation.pathname,
      search: nextLocation.search,
    }
    refreshWorkspaceLocation()
  }, [])
  const conversationListProps = {
    activeId: selectedConversationId,
    conversations: conversationItems,
    deleting: deleteConversations.isPending,
    error: conversations.error,
    hasMore: conversations.hasNextPage,
    loadingMore: conversations.isFetchingNextPage,
    loading: conversations.isLoading,
    search: conversationSearch,
    onDeleteMany: (ids: string[]) => deleteConversations.mutateAsync(ids).then(() => undefined),
    onRename: (id: string, title: string) => renameConversation.mutate({ id, title }),
    runningConversationIds,
    onSearch: setConversationSearch,
    onSelect: selectConversation,
    onLoadMore: () => conversations.fetchNextPage().then(() => undefined),
    refetch: () => conversations.refetch().then(() => undefined),
  }

  return (
    <AIAssistantRuntimeContext
      value={{
        capabilities,
        canCreateConversation: Boolean(newConversationModel),
        canDebugInternalTools,
        closeAssistant,
        composerProps: {
          activeRun: Boolean(activeRunId),
          canceling: cancelingSelected,
          canCancel: Boolean(activeRunId && selectedConversationId),
          models: aiModels.data ?? [],
          modelAvailable: Boolean(selectedModel),
          modelChanging: modelChangingSelected,
          contextUsage: streamState.contextUsage,
          isNewConversation: !selectedConversationId,
          modelSelectionDisabled: Boolean(activeRunId || sendingSelected || sendTurn.isPending),
          selectedModelId: selectedModel?.id,
          draft,
          inputRef,
          maxLength: capabilities?.maxInputBytes,
          sending: sendingSelected,
          submitting: submittingSelected,
          waitingInput,
          onCancel: () => {
            if (activeRunId && selectedConversationId)
              cancelRun.mutate({ runId: activeRunId, conversationId: selectedConversationId })
          },
          onDraftChange: setDraft,
          onModelChange: selectModel,
          onSubmit: submitDraft,
        },
        conversationListProps,
        conversationSearch,
        conversationTitle: timelineData?.snapshot?.conversation.title,
        createConversation: () => {
          if (newConversationModel)
            createConversation.mutate({ modelId: newConversationModel.id })
        },
        creatingConversation: createConversation.isPending,
        enabled,
        executeAction,
        open: enabled && open,
        openAssistant,
        rememberWorkspaceLocation,
        selectedConversationId,
        selectConversation,
        setConversationSearch,
        startNewConversation,
        suggestions,
        surfaceVisible,
        timelineProps: {
          activeTurnId,
          blocks: streamState.blocks,
          error: timeline.data ? null : timeline.error,
          generating,
          hasOlder: timeline.hasPreviousPage,
          loadingOlder: timeline.isFetchingPreviousPage,
          olderError: timeline.isFetchPreviousPageError ? timeline.error : null,
          loading: timeline.isLoading,
          outputStreaming: activeRunStreamState?.status === 'streaming',
          onAction: executeAction,
          onApproval: async (block, decision, reason) => {
            await api.decideAIToolApproval(block.runId, block.toolCallId, {
              decision,
              reason,
            })
          },
          onResend: message => sendTurn.mutate({
            actionGeneration: actionSessionGenerationRef.current,
            conversationId: selectedConversationId,
            message,
          }),
          onRetry: () => void timeline.refetch(),
          onLoadOlder: () => timeline.fetchPreviousPage().then(() => undefined),
          resetKey: selectedConversationId,
          resendDisabled: Boolean(activeRunId || sendingSelected),
          showInternalTools: toolDebugMode.enabled,
          topContent: conversationSession.refreshReturnExpiresAt && canReturnToPreviousConversation && !selectedConversationId && !conversationSearch
            ? (
                <AIRefreshConversationReturn
                  expiresAt={conversationSession.refreshReturnExpiresAt}
                  onExpire={dismissRefreshReturn}
                  onReturn={returnToPreviousConversation}
                />
              )
            : undefined,
        },
        transitionAssistantToPage,
        toolDebugMode,
        workspaceLocation,
      }}
    >
      {children}
    </AIAssistantRuntimeContext>
  )
}
