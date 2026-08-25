import type { ReactNode, UIEvent } from 'react'
import type { AIBlock } from './state'
import type { AIApprovalDecision, ToolCallBlock } from './tool-call'
import type { MessageBlock } from './turns'
import type { AIUIAction } from '@/api'
import { AlertCircle, ArrowDown, Bot, ChevronRight, CircleStop, LoaderCircle, RotateCcw, Sparkles } from 'lucide-react'
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { parseAIOptionActions } from './actions'
import { AIContextCompactedBadge } from './context-compacted-badge'
import { runFailureTranslationKey } from './errors'
import { useInfiniteLoadTrigger } from './infinite-load-trigger'
import { AIInteractionCardPlaceholder } from './interaction-card-placeholder'
import { AIInteractionCards } from './interaction-cards'
import { AIMarkdown } from './markdown'
import { AIMessageMeta } from './message-meta'
import { AINavigationEvent } from './navigation-event'
import { AIOptionsBar } from './options'
import { shouldShowTypingIndicator } from './timeline-stream-state'
import { AIToolCallCard } from './tool-call'
import { groupAIAssistantBlocksByTurn } from './turns'

interface AIAssistantTimelineProps {
  activeTurnId?: string
  bottomInset?: boolean
  blocks: AIBlock[]
  error: Error | null
  generating: boolean
  hasOlder?: boolean
  loading: boolean
  loadingOlder?: boolean
  olderError?: Error | null
  outputStreaming?: boolean
  onAction: (action: AIUIAction) => Promise<boolean>
  onApproval: (block: ToolCallBlock, decision: AIApprovalDecision, reason?: string) => Promise<void>
  onLoadOlder?: () => Promise<void>
  onResend: (message: string) => void
  onRetry: () => void
  resetKey?: string
  resendDisabled?: boolean
  showInternalTools?: boolean
  surface?: 'page' | 'window'
  topContent?: ReactNode
}

const latestPositionThreshold = {
  page: 48,
  window: 12,
} as const

function isVisibleResponseBlock(block: AIBlock, showInternalTools: boolean): boolean {
  if (block.type === 'tool_call' && block.operationId === 'create_options')
    return block.status === 'succeeded' && parseAIOptionActions(block.uiActions).length > 0 ? true : showInternalTools
  return block.type !== 'tool_call' || block.visibility !== 'internal' || showInternalTools
}

export function AIAssistantTimeline({ activeTurnId, bottomInset = false, blocks, error, generating, hasOlder = false, loading, loadingOlder = false, olderError = null, outputStreaming = false, onAction, onApproval, onLoadOlder = async () => {}, onResend, onRetry, resetKey, resendDisabled, showInternalTools = false, surface = 'window', topContent }: AIAssistantTimelineProps) {
  const { t } = useTranslation()
  const page = surface === 'page'
  const viewportRef = useRef<HTMLDivElement>(null)
  const shouldFollowLatestRef = useRef(true)
  const skipLatestFollowRef = useRef(false)
  const lastScrollTopRef = useRef(0)
  const prependAnchorRef = useRef<{ element: HTMLElement, offsetTop: number, scrollTop: number } | undefined>(undefined)
  const [historyAutoLoadKey, setHistoryAutoLoadKey] = useState<string>()
  const [showBackToLatest, setShowBackToLatest] = useState(false)
  const historyAutoLoadEnabled = Boolean(resetKey && historyAutoLoadKey === resetKey)
  const showTypingIndicator = shouldShowTypingIndicator({ activeTurnId, blocks, generating, outputStreaming })
  const turns = groupAIAssistantBlocksByTurn(blocks)

  const scrollToLatest = useCallback(() => {
    const viewport = viewportRef.current
    if (viewport) {
      viewport.scrollTop = viewport.scrollHeight
      shouldFollowLatestRef.current = true
      setShowBackToLatest(false)
    }
  }, [])

  const handleScroll = (event: UIEvent<HTMLDivElement>) => {
    const viewport = event.currentTarget
    if (viewport.scrollTop < lastScrollTopRef.current - 1)
      setHistoryAutoLoadKey(resetKey)
    lastScrollTopRef.current = viewport.scrollTop
    const distanceFromLatest = viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight
    shouldFollowLatestRef.current = distanceFromLatest <= latestPositionThreshold[surface]
    if (page)
      setShowBackToLatest(!shouldFollowLatestRef.current)
  }

  const loadOlder = useCallback(async () => {
    const viewport = viewportRef.current
    const firstTurn = viewport?.querySelector<HTMLElement>('[data-ai-turn]')
    if (viewport && firstTurn) {
      prependAnchorRef.current = {
        element: firstTurn,
        offsetTop: firstTurn.offsetTop,
        scrollTop: viewport.scrollTop,
      }
    }
    await onLoadOlder()
  }, [onLoadOlder])
  const olderTrigger = useInfiniteLoadTrigger({
    enabled: hasOlder,
    loading: loadingOlder,
    observe: historyAutoLoadEnabled,
    onLoad: loadOlder,
    rootRef: viewportRef,
    rootMargin: '320px 0px 0px',
  })

  useLayoutEffect(() => {
    const anchor = prependAnchorRef.current
    const viewport = viewportRef.current
    if (!anchor || !viewport || loadingOlder)
      return
    viewport.scrollTop = anchor.scrollTop + anchor.element.offsetTop - anchor.offsetTop
    prependAnchorRef.current = undefined
    skipLatestFollowRef.current = true
  }, [blocks, loadingOlder])

  useEffect(() => {
    shouldFollowLatestRef.current = true
    lastScrollTopRef.current = 0
    const frame = requestAnimationFrame(scrollToLatest)
    return () => cancelAnimationFrame(frame)
  }, [resetKey, scrollToLatest])

  useEffect(() => {
    if (skipLatestFollowRef.current) {
      skipLatestFollowRef.current = false
      return
    }
    if (!shouldFollowLatestRef.current)
      return
    const frame = requestAnimationFrame(scrollToLatest)
    return () => cancelAnimationFrame(frame)
  }, [blocks, scrollToLatest, showTypingIndicator])

  useEffect(() => {
    const viewport = viewportRef.current
    if (!viewport || typeof ResizeObserver === 'undefined')
      return

    let frame = 0
    const observer = new ResizeObserver(() => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(() => {
        if (shouldFollowLatestRef.current) {
          scrollToLatest()
          return
        }

        if (page) {
          const distanceFromLatest = viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight
          setShowBackToLatest(distanceFromLatest > latestPositionThreshold[surface])
        }
      })
    })
    observer.observe(viewport)
    return () => {
      cancelAnimationFrame(frame)
      observer.disconnect()
    }
  }, [loading, page, scrollToLatest, surface])

  if (loading) {
    return (
      <div
        className={cn(
          'grid flex-1 content-start gap-2.5 p-3',
          page && 'gap-4 py-4 pl-[max(1rem,env(safe-area-inset-left))] pr-[max(1rem,env(safe-area-inset-right))]',
        )}
        data-ai-timeline-surface={surface}
      >
        <Skeleton className="ml-auto h-14 w-[62%]" />
        <Skeleton className="h-16 w-[70%]" />
        <Skeleton className="h-24 w-[78%]" />
      </div>
    )
  }
  if (error) {
    return (
      <div
        className={cn(
          'grid flex-1 place-content-center gap-3 p-6 text-center',
          page && 'pl-[max(1.5rem,env(safe-area-inset-left))] pr-[max(1.5rem,env(safe-area-inset-right))]',
        )}
        data-ai-timeline-surface={surface}
      >
        <p className={cn('text-[13px] font-medium', page && 'text-sm')}>{t('aiAssistant.errors.timeline')}</p>
        <Button className={cn(page && 'min-h-11')} size="sm" variant="outline" onClick={onRetry}>
          <RotateCcw className="size-4" />
          {t('common.retry')}
        </Button>
      </div>
    )
  }
  const timelineViewport = (
    <div
      ref={viewportRef}
      aria-live={page ? undefined : 'polite'}
      className={cn(
        'min-h-0 min-w-0 flex-1 overflow-x-hidden overflow-y-auto overscroll-x-none bg-surface p-3',
        page && 'py-4 pl-[max(1rem,env(safe-area-inset-left))] pr-[max(1rem,env(safe-area-inset-right))]',
        bottomInset && 'pb-16',
      )}
      data-ai-timeline-surface={surface}
      data-slot="ai-assistant-timeline"
      role={page ? 'log' : undefined}
      onScroll={handleScroll}
    >
      {topContent}
      {hasOlder && (
        <div ref={olderTrigger.sentinelRef} className="flex min-h-8 items-center justify-center gap-2 pb-2" data-ai-history-sentinel>
          {olderError && <span className="text-[11px] text-danger">{t('aiAssistant.olderLoadFailed')}</span>}
          <Button
            aria-label={t(loadingOlder ? 'aiAssistant.loadingOlder' : 'aiAssistant.loadOlder')}
            className={cn('h-7 gap-1.5 px-2 text-[11px] text-muted-foreground', page && 'min-h-11 px-3 text-sm')}
            disabled={loadingOlder}
            size="sm"
            variant="ghost"
            onClick={() => void olderTrigger.load()}
          >
            {loadingOlder && <LoaderCircle className="size-3 animate-spin motion-reduce:animate-none" />}
            {t(loadingOlder ? 'aiAssistant.loadingOlder' : 'aiAssistant.loadOlder')}
          </Button>
        </div>
      )}
      {turns.length === 0 && !showTypingIndicator
        ? (
            <div className="mx-auto grid h-full max-w-64 place-content-center gap-2.5 text-center text-muted-foreground">
              <span className="mx-auto grid size-10 place-items-center rounded-full bg-primary-subtle text-primary-text"><Sparkles className="size-5" /></span>
              <p className="text-[13px] font-medium text-foreground">{t('aiAssistant.emptyTitle')}</p>
              <p className="text-xs leading-5">{t('aiAssistant.empty')}</p>
            </div>
          )
        : (
            <div className={cn('mx-auto grid w-full min-w-0 gap-4', page ? 'max-w-none' : 'max-w-[48rem]')}>
              {turns.map((turn, index) => (
                <ConversationTurn
                  key={turn.id}
                  generating={showTypingIndicator && (activeTurnId ? turn.id === activeTurnId : index === turns.length - 1)}
                  responseBlocks={turn.responseBlocks}
                  userMessage={turn.userMessage}
                  onAction={onAction}
                  onApproval={onApproval}
                  onResend={onResend}
                  resendDisabled={resendDisabled}
                  showInternalTools={showInternalTools}
                  surface={surface}
                />
              ))}
              {turns.length === 0 && showTypingIndicator && <AssistantReply generating responseBlocks={[]} surface={surface} onAction={onAction} onApproval={onApproval} />}
            </div>
          )}
    </div>
  )

  if (!page)
    return timelineViewport

  return (
    <div className="relative flex min-h-0 min-w-0 flex-1 overflow-hidden" data-ai-timeline-container-surface={surface}>
      {timelineViewport}
      {showBackToLatest && (
        <Button
          aria-label={t('aiAssistant.thinking.backToLatest')}
          className={cn(
            'absolute left-1/2 z-10 min-h-11 -translate-x-1/2 rounded-full px-4 shadow-raised',
            bottomInset ? 'bottom-16' : 'bottom-3',
          )}
          size="sm"
          variant="secondary"
          onClick={scrollToLatest}
        >
          <ArrowDown className="size-4" />
          {t('aiAssistant.thinking.backToLatest')}
        </Button>
      )}
    </div>
  )
}

function ConversationTurn({ generating, responseBlocks, userMessage, onAction, onApproval, onResend, resendDisabled, showInternalTools, surface }: {
  generating: boolean
  responseBlocks: AIBlock[]
  userMessage?: MessageBlock
  onAction: (action: AIUIAction) => Promise<boolean>
  onApproval: (block: ToolCallBlock, decision: AIApprovalDecision, reason?: string) => Promise<void>
  onResend: (message: string) => void
  resendDisabled?: boolean
  showInternalTools?: boolean
  surface: 'page' | 'window'
}) {
  const page = surface === 'page'
  const visibleResponseBlocks = responseBlocks.filter(block => isVisibleResponseBlock(block, showInternalTools ?? false))
  const turnId = userMessage?.turnId ?? visibleResponseBlocks[0]?.turnId
  return (
    <article className="grid min-w-0 gap-2.5" data-ai-turn data-ai-turn-id={turnId}>
      {userMessage && (
        <div className="flex min-w-0 max-w-full justify-end" data-ai-user-message>
          <div className={cn('group/message flex min-w-0 max-w-[78%] flex-col items-end', page && 'max-w-[84%]')} data-ai-message-group>
            <div className={cn('min-w-0 max-w-full whitespace-pre-wrap break-words rounded-container rounded-br-sm bg-primary px-3 py-2 text-[13px] leading-5 text-primary-foreground', page && 'px-4 py-2.5 text-base leading-6')} data-ai-user-bubble>
              {userMessage.text}
            </div>
            <AIMessageMeta
              align="end"
              copyText={userMessage.text}
              createdAt={userMessage.createdAt}
              resendDisabled={resendDisabled}
              surface={surface}
              onResend={() => onResend(userMessage.text)}
            />
          </div>
        </div>
      )}
      {(visibleResponseBlocks.length > 0 || generating) && (
        <AssistantReply
          generating={generating}
          responseBlocks={visibleResponseBlocks}
          surface={surface}
          onAction={onAction}
          onApproval={onApproval}
        />
      )}
    </article>
  )
}

function AssistantReply({ generating, responseBlocks, surface, onAction, onApproval }: {
  generating: boolean
  responseBlocks: AIBlock[]
  surface: 'page' | 'window'
  onAction: (action: AIUIAction) => Promise<boolean>
  onApproval: (block: ToolCallBlock, decision: AIApprovalDecision, reason?: string) => Promise<void>
}) {
  const page = surface === 'page'
  const hasWideContent = responseBlocks.some(block =>
    block.type === 'tool_call'
    && block.operationId === 'create_interaction_cards')
  const assistantMessages = responseBlocks.filter((block): block is MessageBlock => block.type === 'message' && block.role === 'assistant' && Boolean(block.text.trim()))
  const copyText = assistantMessages.map(block => block.text).join('\n\n')
  const createdAt = assistantMessages.at(-1)?.createdAt
  return (
    <div className="flex min-w-0 max-w-full items-start gap-2" data-ai-reply>
      <span className="mt-0.5 grid size-6 shrink-0 place-items-center rounded-control bg-primary-subtle text-primary-text"><Bot className="size-3" /></span>
      <div
        className={cn(
          'group/message flex min-w-0 flex-col items-start',
          page || hasWideContent ? 'w-full max-w-full flex-1' : 'w-fit max-w-[78%] flex-none',
        )}
        data-ai-message-group
      >
        <div className={cn('grid min-w-0 max-w-full gap-2.5 rounded-container rounded-tl-sm bg-surface-subtle px-3 py-2.5', page && 'w-full gap-3 px-4 py-3 [&_button]:min-h-11 [&_button]:min-w-11')} data-ai-assistant-bubble>
          {responseBlocks.map(block => <ResponseBlock key={block.id} block={block} surface={surface} onAction={onAction} onApproval={onApproval} />)}
          {generating && <TypingIndicator />}
        </div>
        {createdAt && <AIMessageMeta align="start" copyText={copyText} createdAt={createdAt} surface={surface} />}
      </div>
    </div>
  )
}

function TypingIndicator() {
  const { t } = useTranslation()
  return (
    <div aria-label={t('aiAssistant.generating')} className="flex h-7 items-center" role="status">
      <span className="flex items-center gap-1">
        <span className="size-1.5 animate-bounce rounded-full bg-muted-foreground [animation-delay:-300ms] motion-reduce:animate-pulse" />
        <span className="size-1.5 animate-bounce rounded-full bg-muted-foreground [animation-delay:-150ms] motion-reduce:animate-pulse" />
        <span className="size-1.5 animate-bounce rounded-full bg-muted-foreground motion-reduce:animate-pulse" />
      </span>
    </div>
  )
}

function ResponseBlock({ block, surface, onAction, onApproval }: { block: AIBlock, surface: 'page' | 'window', onAction: (action: AIUIAction) => Promise<boolean>, onApproval: (block: ToolCallBlock, decision: AIApprovalDecision, reason?: string) => Promise<void> }) {
  const page = surface === 'page'
  if (block.type === 'thinking')
    return <ThinkingBlock block={block} surface={surface} />
  if (block.type === 'tool_call' && block.operationId === 'create_interaction_cards' && block.status === 'running')
    return <AIInteractionCardPlaceholder arguments={block.arguments} result={block.result} />
  if (block.type === 'tool_call' && block.operationId === 'create_interaction_cards' && block.status === 'succeeded')
    return <AIInteractionCards arguments={block.arguments} onAction={onAction} />
  if (block.type === 'tool_call' && block.operationId === 'create_options' && block.status === 'succeeded' && parseAIOptionActions(block.uiActions).length > 0) {
    return (
      <AIOptionsBar
        actions={block.uiActions}
        placement="inline"
        sourceKey={`agent:${block.id}`}
        surface={surface}
        onAction={onAction}
      />
    )
  }
  if (block.type === 'tool_call' && block.operationId === 'navigate_to_route')
    return <AINavigationEvent block={block} onAction={onAction} />
  if (block.type === 'tool_call')
    return <AIToolCallCard block={block} surface={surface} onAction={onAction} onApproval={onApproval} />
  if (block.type === 'run_status')
    return <RunStatusBlock block={block} />
  if (block.type === 'context_compacted')
    return <AIContextCompactedBadge />
  if (block.role === 'user')
    return null
  return (
    <AIMarkdown className={cn('min-w-0 max-w-full text-foreground', page && 'text-base leading-6')}>
      {block.text}
    </AIMarkdown>
  )
}

function RunStatusBlock({ block }: { block: Extract<AIBlock, { type: 'run_status' }> }) {
  const { t } = useTranslation()
  const message = block.status === 'failed'
    ? t(runFailureTranslationKey(block.errorCode))
    : t(block.status === 'interrupted' ? 'aiAssistant.runInterrupted' : 'aiAssistant.runCanceled')
  return (
    <div className={cn(
      'flex items-start gap-2 rounded-control px-2.5 py-2 text-xs leading-5',
      block.status === 'failed' ? 'bg-danger-subtle text-danger' : 'bg-surface-subtle text-muted-foreground',
    )}
    >
      {block.status === 'failed' ? <AlertCircle className="mt-0.5 size-4 shrink-0" /> : <CircleStop className="mt-0.5 size-4 shrink-0" />}
      <span>{message}</span>
    </div>
  )
}

function ThinkingBlock({ block, surface }: { block: Extract<AIBlock, { type: 'thinking' }>, surface: 'page' | 'window' }) {
  const { t } = useTranslation()
  const page = surface === 'page'
  const viewportRef = useRef<HTMLDivElement>(null)
  const [following, setFollowing] = useState(true)
  const [expanded, setExpanded] = useState(false)

  // 折叠状态下始终跟随最新内容
  useEffect(() => {
    if (!expanded && viewportRef.current)
      viewportRef.current.scrollTop = viewportRef.current.scrollHeight
  }, [block.text, expanded])

  // 展开时跟随滚动到底部
  useEffect(() => {
    if (expanded && following && viewportRef.current)
      viewportRef.current.scrollTop = viewportRef.current.scrollHeight
  }, [block.text, expanded, following])

  return (
    <div className="min-w-0 rounded-control bg-surface-inset/70 px-2.5 py-1.5">
      <button aria-expanded={expanded} className={cn('flex w-full items-center gap-1.5 text-left text-[11px] font-medium text-muted-foreground', page && 'min-h-11 text-sm')} type="button" onClick={() => setExpanded(value => !value)}>
        <Sparkles className="size-3.5" />
        <span>{t(block.display === 'summary' ? `aiAssistant.thinking.${block.status === 'completed' ? 'summaryComplete' : 'summaryStreaming'}` : `aiAssistant.thinking.${block.status === 'completed' ? 'progressComplete' : 'progressStreaming'}`)}</span>
        <ChevronRight className={cn('ml-auto size-3.5 transition-transform', expanded && 'rotate-90')} />
      </button>
      <div
        ref={viewportRef}
        className={cn(
          'whitespace-pre-wrap break-words text-[11px] leading-4 text-muted-foreground',
          page && 'text-sm leading-5',
          expanded
            ? 'mt-1 max-h-56 overflow-y-auto'
            : 'mt-1 max-h-12 overflow-hidden',
        )}
        onScroll={expanded
          ? (event) => {
              const target = event.currentTarget
              setFollowing(target.scrollHeight - target.scrollTop - target.clientHeight < 8)
            }
          : undefined}
      >
        {block.text}
      </div>
      {expanded && !following && <button className={cn('mt-1 text-[10px] font-medium text-primary-text', page && 'min-h-11 text-sm')} type="button" onClick={() => setFollowing(true)}>{t('aiAssistant.thinking.backToLatest')}</button>}
    </div>
  )
}
