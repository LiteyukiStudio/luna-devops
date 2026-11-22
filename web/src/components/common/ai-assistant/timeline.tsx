import type { ReactNode, UIEvent } from 'react'
import type { AIBlock } from './state'
import type { AIApprovalDecision, ToolCallBlock } from './tool-call'
import type { MessageBlock } from './turns'
import type { AIUIAction } from '@/api'
import { AlertCircle, ArrowDown, Brain, ChevronRight, CircleStop, LoaderCircle, RotateCcw, Sparkles } from 'lucide-react'
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { parseAIOptionActions } from './actions'
import { AIAssistantMark } from './assistant-mark'
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
import { toolDisplayName } from './tool-display-name'
import { groupAIAssistantBlocksByTurn, isInteractionCardOperationId } from './turns'

interface AIAssistantTimelineProps {
  activeTurnId?: string
  blocks: AIBlock[]
  /** 会话没有任何轮次时的欢迎内容；缺省为通用说明。 */
  emptyState?: ReactNode
  error: Error | null
  generating: boolean
  hasOlder?: boolean
  loading: boolean
  loadingOlder?: boolean
  olderError?: Error | null
  outputStreaming?: boolean
  onAction: (action: AIUIAction) => Promise<boolean>
  onApproval: (block: ToolCallBlock, decision: AIApprovalDecision) => Promise<void>
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
  if (block.type === 'tool_call' && isInteractionCardOperationId(block.operationId))
    return true
  if (block.type === 'tool_call' && block.operationId === 'create_options')
    return block.status === 'succeeded' && parseAIOptionActions(block.uiActions).length > 0 ? true : showInternalTools
  if (block.type === 'tool_call' && block.operationId === 'navigate_to_route')
    return block.status === 'succeeded' && parseAIOptionActions(block.uiActions).some(action => action.type === 'navigate') ? true : showInternalTools
  return block.type !== 'tool_call' || block.visibility !== 'internal' || showInternalTools
}

export function AIAssistantTimeline({ activeTurnId, blocks, emptyState, error, generating, hasOlder = false, loading, loadingOlder = false, olderError = null, outputStreaming = false, onAction, onApproval, onLoadOlder = async () => {}, onResend, onRetry, resetKey, resendDisabled, showInternalTools = false, surface = 'window', topContent }: AIAssistantTimelineProps) {
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
        aria-label={t('common.loading')}
        className={cn(
          'flex-1 p-4',
          page && 'pl-[max(1rem,env(safe-area-inset-left))] pr-[max(1rem,env(safe-area-inset-right))]',
        )}
        data-ai-timeline-surface={surface}
        role="status"
      >
        <div className="mx-auto grid w-full max-w-3xl gap-5">
          <Skeleton className="ml-auto h-10 w-[56%] rounded-container" />
          <div className="flex gap-3">
            <Skeleton className="size-6 shrink-0 rounded-full" />
            <div className="grid flex-1 gap-2">
              <Skeleton className="h-3.5 w-[82%]" />
              <Skeleton className="h-3.5 w-[64%]" />
              <Skeleton className="h-3.5 w-[72%]" />
            </div>
          </div>
        </div>
      </div>
    )
  }
  if (error) {
    return (
      <div
        className={cn(
          'grid flex-1 place-content-center justify-items-center gap-3 p-6 text-center',
          page && 'pl-[max(1.5rem,env(safe-area-inset-left))] pr-[max(1.5rem,env(safe-area-inset-right))]',
        )}
        data-ai-timeline-surface={surface}
        role="alert"
      >
        <span className="grid size-10 place-items-center rounded-full bg-danger-subtle text-danger"><AlertCircle className="size-5" /></span>
        <p className="text-sm font-medium">{t('aiAssistant.errors.timeline')}</p>
        <Button className={cn(page && 'min-h-11 pointer-fine:min-h-8')} size="sm" variant="outline" onClick={onRetry}>
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
        'min-h-0 min-w-0 flex-1 overflow-x-hidden overflow-y-auto overscroll-x-none px-4 py-4',
        page && 'py-5 pl-[max(1rem,env(safe-area-inset-left))] pr-[max(1rem,env(safe-area-inset-right))]',
      )}
      data-ai-timeline-surface={surface}
      data-slot="ai-assistant-timeline"
      role={page ? 'log' : undefined}
      onScroll={handleScroll}
    >
      {topContent}
      {hasOlder && (
        <div ref={olderTrigger.sentinelRef} className="flex min-h-8 items-center justify-center gap-2 pb-3" data-ai-history-sentinel>
          {olderError && <span className="text-xs text-danger">{t('aiAssistant.olderLoadFailed')}</span>}
          <Button
            aria-label={t(loadingOlder ? 'aiAssistant.loadingOlder' : 'aiAssistant.loadOlder')}
            className={cn('h-7 gap-1.5 px-2.5 text-xs text-muted-foreground', page && 'min-h-11 px-3 text-sm pointer-fine:min-h-8')}
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
        ? emptyState ?? (
          <div className="mx-auto grid h-full max-w-xs place-content-center justify-items-center gap-3 text-center">
            <AIAssistantMark size="lg" />
            <p className="text-base font-semibold text-foreground">{t('aiAssistant.emptyTitle')}</p>
            <p className="text-sm leading-6 text-muted-foreground">{t('aiAssistant.empty')}</p>
          </div>
        )
        : (
            <div className="mx-auto grid w-full min-w-0 max-w-3xl gap-6">
              {turns.map((turn, index) => (
                <ConversationTurn
                  key={turn.id}
                  generating={showTypingIndicator && (activeTurnId ? turn.id === activeTurnId : index === turns.length - 1)}
                  historical={index < turns.length - 1}
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
          className="absolute bottom-3 left-1/2 z-10 min-h-11 -translate-x-1/2 rounded-full border border-border bg-surface px-4 text-foreground shadow-raised hover:bg-surface-subtle pointer-fine:min-h-8"
          size="sm"
          variant="ghost"
          onClick={scrollToLatest}
        >
          <ArrowDown className="size-4" />
          {t('aiAssistant.thinking.backToLatest')}
        </Button>
      )}
    </div>
  )
}

function ConversationTurn({ generating, historical = false, responseBlocks, userMessage, onAction, onApproval, onResend, resendDisabled, showInternalTools, surface }: {
  generating: boolean
  historical?: boolean
  responseBlocks: AIBlock[]
  userMessage?: MessageBlock
  onAction: (action: AIUIAction) => Promise<boolean>
  onApproval: (block: ToolCallBlock, decision: AIApprovalDecision) => Promise<void>
  onResend: (message: string) => void
  resendDisabled?: boolean
  showInternalTools?: boolean
  surface: 'page' | 'window'
}) {
  const page = surface === 'page'
  const visibleResponseBlocks = responseBlocks.filter(block => isVisibleResponseBlock(block, showInternalTools ?? false))
  const turnId = userMessage?.turnId ?? visibleResponseBlocks[0]?.turnId
  return (
    <article className="grid min-w-0 gap-3" data-ai-turn data-ai-turn-id={turnId}>
      {userMessage && (
        <div className="flex min-w-0 max-w-full justify-end" data-ai-user-message>
          <div className={cn('group/message flex min-w-0 max-w-[85%] flex-col items-end', page && 'max-w-[88%]')} data-ai-message-group>
            <div className={cn('min-w-0 max-w-full whitespace-pre-wrap break-words rounded-container bg-primary-subtle px-3.5 py-2 text-[13px] leading-5.5 text-foreground', page && 'px-4 py-2.5 text-base leading-6')} data-ai-user-bubble>
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
          historical={historical}
          responseBlocks={visibleResponseBlocks}
          showInternalTools={showInternalTools}
          surface={surface}
          onAction={onAction}
          onApproval={onApproval}
        />
      )}
    </article>
  )
}

const READ_DISCOVERY_TOOL_PATTERN = /^(?:[a-z]+_(?:list|get|search|check|inspect|preview|read)_|(?:list|get|search|read|fetch)[A-Z_]|webSearch\b|ops_web_)/

function isCollapsibleProcessBlock(block: AIBlock): boolean {
  if (block.type === 'thinking')
    return true
  if (block.type !== 'tool_call')
    return false
  if (block.status !== 'succeeded' && block.status !== 'running' && block.status !== 'proposed')
    return false
  if (block.uiActions.length > 0)
    return false
  if (isInteractionCardOperationId(block.operationId) || block.operationId === 'create_options' || block.operationId === 'navigate_to_route' || block.operationId === 'wait_for_task' || block.operationId === 'rename_conversation')
    return false
  if (block.visibility === 'internal')
    return false
  return READ_DISCOVERY_TOOL_PATTERN.test(block.operationId)
}

function isProcessBlockActive(block: AIBlock): boolean {
  if (block.type === 'thinking')
    return block.status === 'streaming'
  return block.type === 'tool_call' && (block.status === 'running' || block.status === 'proposed')
}

type ReplySegment
  = | { kind: 'single', block: AIBlock }
    | { kind: 'process_group', id: string, blocks: AIBlock[] }

function segmentAssistantResponseBlocks(blocks: AIBlock[]): ReplySegment[] {
  const segments: ReplySegment[] = []
  let index = 0
  while (index < blocks.length) {
    const current = blocks[index]!
    if (!isCollapsibleProcessBlock(current)) {
      segments.push({ kind: 'single', block: current })
      index += 1
      continue
    }
    const run: AIBlock[] = [current]
    let cursor = index + 1
    while (cursor < blocks.length && isCollapsibleProcessBlock(blocks[cursor]!)) {
      run.push(blocks[cursor]!)
      cursor += 1
    }
    if (run.length >= 2) {
      segments.push({ kind: 'process_group', id: `group:${run[0]!.id}`, blocks: run })
    }
    else {
      segments.push({ kind: 'single', block: current })
    }
    index = cursor
  }
  return segments
}

function ProcessStepGroup({ blocks, historical, showInternalTools, surface, onAction, onApproval }: {
  blocks: AIBlock[]
  historical?: boolean
  showInternalTools?: boolean
  surface: 'page' | 'window'
  onAction: (action: AIUIAction) => Promise<boolean>
  onApproval: (block: ToolCallBlock, decision: AIApprovalDecision) => Promise<void>
}) {
  const { t } = useTranslation()
  const page = surface === 'page'
  const [isOpen, setIsOpen] = useState(false)
  const isRunning = blocks.some(isProcessBlockActive)
  const toolCount = blocks.filter(block => block.type === 'tool_call').length
  const latestBlock = blocks.at(-1)
  const latestPreview = latestBlock?.type === 'tool_call'
    ? toolDisplayName(t, latestBlock.operationId)
    : latestBlock?.type === 'thinking'
      ? latestBlock.text.trim().split('\n').filter(Boolean).at(-1)
      : undefined
  const label = isRunning
    ? t('aiAssistant.processGroup.running', { count: blocks.length })
    : toolCount > 0
      ? t('aiAssistant.processGroup.completedWithTools', { count: blocks.length, tools: toolCount })
      : t('aiAssistant.processGroup.completed', { count: blocks.length })

  return (
    <details
      className="group/process min-w-0 rounded-control border border-border/75 bg-surface/88 px-2.5 py-1.5 backdrop-blur-[2px]"
      data-ai-process-group={isRunning ? 'running' : 'completed'}
      open={isOpen}
      onToggle={(event) => {
        if (event.currentTarget.open !== isOpen)
          setIsOpen(event.currentTarget.open)
      }}
    >
      <summary
        className={cn(
          'flex cursor-pointer list-none items-center gap-2 rounded-sm text-xs text-muted-foreground outline-none transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring [&::-webkit-details-marker]:hidden',
          page && 'min-h-9 pointer-fine:min-h-7 sm:text-[13px]',
        )}
      >
        {isRunning
          ? <LoaderCircle aria-hidden="true" className="size-3.5 shrink-0 animate-spin text-info motion-reduce:animate-none" />
          : <Brain aria-hidden="true" className="size-3.5 shrink-0 text-muted-foreground/85" />}
        <span className="shrink-0 font-medium text-foreground/90">{label}</span>
        {isRunning && !isOpen && latestPreview && (
          <span className="min-w-0 flex-1 truncate text-muted-foreground">
            ·
            {' '}
            {latestPreview}
          </span>
        )}
        <ChevronRight
          aria-hidden="true"
          className={cn('ml-auto size-3.5 shrink-0 transition-transform duration-fast', isOpen && 'rotate-90')}
        />
      </summary>
      <div className="mt-2 grid min-w-0 gap-2 border-t border-border/60 pt-2">
        {blocks.map(block => (
          <ResponseBlock
            key={block.id}
            block={block}
            historical={historical}
            parentOpen={isOpen}
            showInternalTools={showInternalTools}
            surface={surface}
            onAction={onAction}
            onApproval={onApproval}
          />
        ))}
      </div>
    </details>
  )
}

function AssistantReply({ generating, historical = false, responseBlocks, showInternalTools, surface, onAction, onApproval }: {
  generating: boolean
  historical?: boolean
  responseBlocks: AIBlock[]
  showInternalTools?: boolean
  surface: 'page' | 'window'
  onAction: (action: AIUIAction) => Promise<boolean>
  onApproval: (block: ToolCallBlock, decision: AIApprovalDecision) => Promise<void>
}) {
  const page = surface === 'page'
  const assistantMessages = responseBlocks.filter((block): block is MessageBlock => block.type === 'message' && block.role === 'assistant' && Boolean(block.text.trim()))
  const copyText = assistantMessages.map(block => block.text).join('\n\n')
  const createdAt = assistantMessages.at(-1)?.createdAt
  const segments = segmentAssistantResponseBlocks(responseBlocks)
  return (
    <div className="flex min-w-0 max-w-full items-start gap-3" data-ai-reply>
      <AIAssistantMark className="mt-px" />
      <div className="group/message flex w-full min-w-0 max-w-full flex-1 flex-col items-start" data-ai-message-group>
        <div className={cn('grid w-full min-w-0 max-w-full gap-3', page && 'pointer-coarse:[&_button]:min-h-11 pointer-coarse:[&_button]:min-w-11')} data-ai-assistant-bubble>
          {segments.map(segment => segment.kind === 'process_group'
            ? (
                <ProcessStepGroup
                  key={segment.id}
                  blocks={segment.blocks}
                  historical={historical}
                  showInternalTools={showInternalTools}
                  surface={surface}
                  onAction={onAction}
                  onApproval={onApproval}
                />
              )
            : (
                <ResponseBlock
                  key={segment.block.id}
                  block={segment.block}
                  historical={historical}
                  showInternalTools={showInternalTools}
                  surface={surface}
                  onAction={onAction}
                  onApproval={onApproval}
                />
              ))}
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
    <div aria-label={t('aiAssistant.generating')} className="flex h-6 items-center" role="status">
      <span className="flex items-center gap-1">
        <span className="size-1.5 animate-bounce rounded-full bg-muted-foreground [animation-delay:-300ms] motion-reduce:animate-pulse" />
        <span className="size-1.5 animate-bounce rounded-full bg-muted-foreground [animation-delay:-150ms] motion-reduce:animate-pulse" />
        <span className="size-1.5 animate-bounce rounded-full bg-muted-foreground motion-reduce:animate-pulse" />
      </span>
    </div>
  )
}

function ResponseBlock({ block, historical = false, parentOpen = true, showInternalTools = false, surface, onAction, onApproval }: { block: AIBlock, historical?: boolean, parentOpen?: boolean, showInternalTools?: boolean, surface: 'page' | 'window', onAction: (action: AIUIAction) => Promise<boolean>, onApproval: (block: ToolCallBlock, decision: AIApprovalDecision) => Promise<void> }) {
  const page = surface === 'page'
  if (block.type === 'thinking')
    return <ThinkingBlock block={block} parentOpen={parentOpen} surface={surface} />
  if (block.type === 'tool_call' && isInteractionCardOperationId(block.operationId) && block.status === 'running')
    return <AIInteractionCardPlaceholder arguments={block.arguments} result={block.result} />
  if (block.type === 'tool_call' && isInteractionCardOperationId(block.operationId) && block.status === 'succeeded')
    return <AIInteractionCards arguments={block.arguments} historical={historical} onAction={onAction} />
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
    return <AIToolCallCard block={block} defaultOpenTechnical={showInternalTools} surface={surface} onAction={onAction} onApproval={onApproval} />
  if (block.type === 'run_status')
    return <RunStatusBlock block={block} />
  if (block.type === 'context_compacted')
    return <AIContextCompactedBadge />
  if (block.role === 'user') {
    return (
      <div className="flex min-w-0 max-w-full justify-end" data-ai-supplemental-user-message>
        <div className={cn('min-w-0 max-w-[85%] whitespace-pre-wrap break-words rounded-container bg-primary-subtle px-3.5 py-2 text-[13px] leading-5.5 text-foreground', page && 'max-w-[88%] px-4 py-2.5 text-base leading-6')} data-ai-user-bubble>
          {block.text}
        </div>
      </div>
    )
  }
  return (
    <AIMarkdown
      className={cn(
        'min-w-0 max-w-full rounded-control bg-surface/88 px-3 py-1.5 text-foreground backdrop-blur-[2px]',
        page && 'px-3.5 py-2 text-base leading-6',
      )}
    >
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
      'flex items-start gap-2 rounded-control border px-3 py-2 text-xs leading-5',
      block.status === 'failed' ? 'border-danger-border bg-danger-subtle text-danger' : 'border-border bg-surface-subtle text-muted-foreground',
    )}
    >
      {block.status === 'failed' ? <AlertCircle className="mt-0.5 size-4 shrink-0" /> : <CircleStop className="mt-0.5 size-4 shrink-0" />}
      <span>{message}</span>
    </div>
  )
}

function ThinkingBlock({ block, parentOpen = true, surface }: { block: Extract<AIBlock, { type: 'thinking' }>, parentOpen?: boolean, surface: 'page' | 'window' }) {
  const { t } = useTranslation()
  const page = surface === 'page'
  const viewportRef = useRef<HTMLDivElement>(null)
  const [following, setFollowing] = useState(true)
  const [expanded, setExpanded] = useState(false)

  // 折叠状态下始终跟随最新内容，仅当真实滚出顶部时才添加顶部渐隐遮罩
  useLayoutEffect(() => {
    const viewport = viewportRef.current
    if (!viewport)
      return
    if (expanded) {
      delete viewport.dataset.scrolledTop
      return
    }
    if (!parentOpen) {
      viewport.dataset.scrolledTop = 'false'
      return
    }
    viewport.scrollTop = viewport.scrollHeight
    viewport.dataset.scrolledTop = String(viewport.scrollTop > 2)
  }, [block.text, expanded, parentOpen])

  // 展开时跟随滚动到底部
  useEffect(() => {
    if (expanded && following && parentOpen && viewportRef.current)
      viewportRef.current.scrollTop = viewportRef.current.scrollHeight
  }, [block.text, expanded, following, parentOpen])

  return (
    <div
      className="min-w-0 rounded-r-control border-l-2 border-border bg-surface/88 px-3 py-2 backdrop-blur-[2px]"
      data-ai-thinking=""
    >
      <button aria-expanded={expanded} className={cn('flex w-full items-center gap-1.5 rounded-sm text-left text-xs font-medium text-muted-foreground outline-none transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring', page && 'min-h-11 text-sm pointer-fine:min-h-6')} type="button" onClick={() => setExpanded(value => !value)}>
        {block.status === 'completed' ? <Brain className="size-3.5" /> : <Sparkles className="size-3.5 animate-pulse motion-reduce:animate-none" />}
        <span>{t(block.display === 'summary' ? `aiAssistant.thinking.${block.status === 'completed' ? 'summaryComplete' : 'summaryStreaming'}` : `aiAssistant.thinking.${block.status === 'completed' ? 'progressComplete' : 'progressStreaming'}`)}</span>
        <ChevronRight className={cn('ml-auto size-3.5 transition-transform', expanded && 'rotate-90')} />
      </button>
      <div
        ref={viewportRef}
        className={cn(
          'whitespace-pre-wrap break-words text-xs leading-5 text-muted-foreground',
          page && 'text-sm leading-6',
          expanded
            ? 'mt-1 max-h-56 overflow-y-auto'
            : 'mt-1 max-h-10 overflow-hidden data-[scrolled-top=true]:[mask-image:linear-gradient(to_bottom,transparent,black_32%)]',
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
      {expanded && !following && <button className={cn('mt-1 text-xs font-medium text-primary-text', page && 'min-h-11 text-sm pointer-fine:min-h-6')} type="button" onClick={() => setFollowing(true)}>{t('aiAssistant.thinking.backToLatest')}</button>}
    </div>
  )
}
