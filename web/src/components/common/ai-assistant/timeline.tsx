import type { ReactNode, UIEvent } from 'react'
import type { AIBlock } from './state'
import type { AIApprovalDecision, ToolCallBlock } from './tool-call'
import type { MessageBlock } from './turns'
import type { AIUIAction } from '@/api'
import { AlertCircle, Bot, ChevronRight, CircleStop, RotateCcw, Sparkles } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { runFailureTranslationKey } from './errors'
import { AIInteractionCardPlaceholder } from './interaction-card-placeholder'
import { AIInteractionCards } from './interaction-cards'
import { AIMarkdown } from './markdown'
import { AIMessageMeta } from './message-meta'
import { AINavigationEvent } from './navigation-event'
import { AIToolCallCard } from './tool-call'
import { groupAIAssistantBlocksByTurn } from './turns'

interface AIAssistantTimelineProps {
  bottomInset?: boolean
  blocks: AIBlock[]
  error: Error | null
  generating: boolean
  loading: boolean
  onAction: (action: AIUIAction) => Promise<boolean>
  onApproval: (block: ToolCallBlock, decision: AIApprovalDecision, reason?: string) => Promise<void>
  onMFA: (block: ToolCallBlock, code: string) => Promise<void>
  onResend: (message: string) => void
  onRetry: () => void
  resetKey?: string
  resendDisabled?: boolean
  showInternalTools?: boolean
  topContent?: ReactNode
}

const latestPositionThreshold = 12

function isVisibleResponseBlock(block: AIBlock, showInternalTools: boolean): boolean {
  return block.type !== 'tool_call' || block.visibility !== 'internal' || showInternalTools
}

export function AIAssistantTimeline({ bottomInset = false, blocks, error, generating, loading, onAction, onApproval, onMFA, onResend, onRetry, resetKey, resendDisabled, showInternalTools = false, topContent }: AIAssistantTimelineProps) {
  const { t } = useTranslation()
  const viewportRef = useRef<HTMLDivElement>(null)
  const shouldFollowLatestRef = useRef(true)
  const showTypingIndicator = generating && !blocks.some(block => block.status === 'streaming')
  const turns = groupAIAssistantBlocksByTurn(blocks)

  const scrollToLatest = () => {
    const viewport = viewportRef.current
    if (viewport)
      viewport.scrollTop = viewport.scrollHeight
  }

  const handleScroll = (event: UIEvent<HTMLDivElement>) => {
    const viewport = event.currentTarget
    const distanceFromLatest = viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight
    shouldFollowLatestRef.current = distanceFromLatest <= latestPositionThreshold
  }

  useEffect(() => {
    shouldFollowLatestRef.current = true
    const frame = requestAnimationFrame(scrollToLatest)
    return () => cancelAnimationFrame(frame)
  }, [resetKey])

  useEffect(() => {
    if (!shouldFollowLatestRef.current)
      return
    const frame = requestAnimationFrame(scrollToLatest)
    return () => cancelAnimationFrame(frame)
  }, [blocks, showTypingIndicator])
  if (loading) {
    return (
      <div className="grid flex-1 content-start gap-2.5 p-3">
        <Skeleton className="ml-auto h-14 w-[62%]" />
        <Skeleton className="h-16 w-[70%]" />
        <Skeleton className="h-24 w-[78%]" />
      </div>
    )
  }
  if (error) {
    return (
      <div className="grid flex-1 place-content-center gap-3 p-6 text-center">
        <p className="text-[13px] font-medium">{t('aiAssistant.errors.timeline')}</p>
        <Button size="sm" variant="outline" onClick={onRetry}>
          <RotateCcw className="size-4" />
          {t('common.retry')}
        </Button>
      </div>
    )
  }
  return (
    <div
      ref={viewportRef}
      aria-live="polite"
      className={cn(
        'min-h-0 min-w-0 flex-1 overflow-x-hidden overflow-y-auto overscroll-x-none bg-surface p-3',
        bottomInset && 'pb-16',
      )}
      data-slot="ai-assistant-timeline"
      onScroll={handleScroll}
    >
      {topContent}
      {turns.length === 0 && !showTypingIndicator
        ? (
            <div className="mx-auto grid h-full max-w-64 place-content-center gap-2.5 text-center text-muted-foreground">
              <span className="mx-auto grid size-10 place-items-center rounded-full bg-primary-subtle text-primary-text"><Sparkles className="size-5" /></span>
              <p className="text-[13px] font-medium text-foreground">{t('aiAssistant.emptyTitle')}</p>
              <p className="text-xs leading-5">{t('aiAssistant.empty')}</p>
            </div>
          )
        : (
            <div className="mx-auto grid w-full min-w-0 max-w-[48rem] gap-4">
              {turns.map((turn, index) => (
                <ConversationTurn
                  key={turn.id}
                  generating={showTypingIndicator && index === turns.length - 1}
                  responseBlocks={turn.responseBlocks}
                  userMessage={turn.userMessage}
                  onAction={onAction}
                  onApproval={onApproval}
                  onMFA={onMFA}
                  onResend={onResend}
                  resendDisabled={resendDisabled}
                  showInternalTools={showInternalTools}
                />
              ))}
              {turns.length === 0 && showTypingIndicator && <AssistantReply generating responseBlocks={[]} onAction={onAction} onApproval={onApproval} onMFA={onMFA} />}
            </div>
          )}
    </div>
  )
}

function ConversationTurn({ generating, responseBlocks, userMessage, onAction, onApproval, onMFA, onResend, resendDisabled, showInternalTools }: {
  generating: boolean
  responseBlocks: AIBlock[]
  userMessage?: MessageBlock
  onAction: (action: AIUIAction) => Promise<boolean>
  onApproval: (block: ToolCallBlock, decision: AIApprovalDecision, reason?: string) => Promise<void>
  onMFA: (block: ToolCallBlock, code: string) => Promise<void>
  onResend: (message: string) => void
  resendDisabled?: boolean
  showInternalTools?: boolean
}) {
  const visibleResponseBlocks = responseBlocks.filter(block => isVisibleResponseBlock(block, showInternalTools ?? false))
  return (
    <article className="grid min-w-0 gap-2.5" data-ai-turn>
      {userMessage && (
        <div className="flex min-w-0 max-w-full justify-end" data-ai-user-message>
          <div className="group/message flex min-w-0 max-w-[78%] flex-col items-end" data-ai-message-group>
            <div className="min-w-0 max-w-full whitespace-pre-wrap break-words rounded-container rounded-br-sm bg-primary px-3 py-2 text-[13px] leading-5 text-primary-foreground" data-ai-user-bubble>
              {userMessage.text}
            </div>
            <AIMessageMeta
              align="end"
              copyText={userMessage.text}
              createdAt={userMessage.createdAt}
              resendDisabled={resendDisabled}
              onResend={() => onResend(userMessage.text)}
            />
          </div>
        </div>
      )}
      {(visibleResponseBlocks.length > 0 || generating) && (
        <AssistantReply
          generating={generating}
          responseBlocks={visibleResponseBlocks}
          onAction={onAction}
          onApproval={onApproval}
          onMFA={onMFA}
        />
      )}
    </article>
  )
}

function AssistantReply({ generating, responseBlocks, onAction, onApproval, onMFA }: {
  generating: boolean
  responseBlocks: AIBlock[]
  onAction: (action: AIUIAction) => Promise<boolean>
  onApproval: (block: ToolCallBlock, decision: AIApprovalDecision, reason?: string) => Promise<void>
  onMFA: (block: ToolCallBlock, code: string) => Promise<void>
}) {
  const hasWideContent = responseBlocks.some(block =>
    block.type === 'tool_call'
    && (block.operationId === 'prepare_interaction_cards' || block.operationId === 'create_interaction_cards'))
  const assistantMessages = responseBlocks.filter((block): block is MessageBlock => block.type === 'message' && block.role === 'assistant' && Boolean(block.text.trim()))
  const copyText = assistantMessages.map(block => block.text).join('\n\n')
  const createdAt = assistantMessages.at(-1)?.createdAt
  return (
    <div className="flex min-w-0 max-w-full items-start gap-2" data-ai-reply>
      <span className="mt-0.5 grid size-6 shrink-0 place-items-center rounded-control bg-primary-subtle text-primary-text"><Bot className="size-3" /></span>
      <div
        className={cn(
          'group/message flex min-w-0 flex-col items-start',
          hasWideContent ? 'w-full max-w-full flex-1' : 'w-fit max-w-[78%] flex-none',
        )}
        data-ai-message-group
      >
        <div className="grid min-w-0 max-w-full gap-2.5 rounded-container rounded-tl-sm bg-surface-subtle px-3 py-2.5" data-ai-assistant-bubble>
          {responseBlocks.map(block => <ResponseBlock key={block.id} block={block} onAction={onAction} onApproval={onApproval} onMFA={onMFA} />)}
          {generating && <TypingIndicator />}
        </div>
        {createdAt && <AIMessageMeta align="start" copyText={copyText} createdAt={createdAt} />}
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

function ResponseBlock({ block, onAction, onApproval, onMFA }: { block: AIBlock, onAction: (action: AIUIAction) => Promise<boolean>, onApproval: (block: ToolCallBlock, decision: AIApprovalDecision, reason?: string) => Promise<void>, onMFA: (block: ToolCallBlock, code: string) => Promise<void> }) {
  if (block.type === 'thinking')
    return <ThinkingBlock block={block} />
  if (block.type === 'tool_call' && block.operationId === 'prepare_interaction_cards' && block.status === 'running')
    return <AIInteractionCardPlaceholder arguments={block.arguments} result={block.result} />
  if (block.type === 'tool_call' && block.operationId === 'create_interaction_cards' && block.status === 'succeeded')
    return <AIInteractionCards arguments={block.arguments} onAction={onAction} />
  if (block.type === 'tool_call' && block.operationId === 'navigate_to_route')
    return <AINavigationEvent block={block} onAction={onAction} />
  if (block.type === 'tool_call')
    return <AIToolCallCard block={block} onAction={onAction} onApproval={onApproval} onMFA={onMFA} />
  if (block.type === 'run_status')
    return <RunStatusBlock block={block} />
  if (block.role === 'user')
    return null
  return (
    <AIMarkdown className="min-w-0 max-w-full text-foreground">
      {block.text}
    </AIMarkdown>
  )
}

function RunStatusBlock({ block }: { block: Extract<AIBlock, { type: 'run_status' }> }) {
  const { t } = useTranslation()
  return (
    <div className={cn(
      'flex items-start gap-2 rounded-control px-2.5 py-2 text-xs leading-5',
      block.status === 'failed' ? 'bg-danger-subtle text-danger' : 'bg-surface-subtle text-muted-foreground',
    )}
    >
      {block.status === 'failed' ? <AlertCircle className="mt-0.5 size-4 shrink-0" /> : <CircleStop className="mt-0.5 size-4 shrink-0" />}
      <span>{block.status === 'failed' ? t(runFailureTranslationKey(block.errorCode)) : t('aiAssistant.runCanceled')}</span>
    </div>
  )
}

function ThinkingBlock({ block }: { block: Extract<AIBlock, { type: 'thinking' }> }) {
  const { t } = useTranslation()
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
      <button aria-expanded={expanded} className="flex w-full items-center gap-1.5 text-left text-[11px] font-medium text-muted-foreground" type="button" onClick={() => setExpanded(value => !value)}>
        <Sparkles className="size-3.5" />
        <span>{t(block.display === 'summary' ? `aiAssistant.thinking.${block.status === 'completed' ? 'summaryComplete' : 'summaryStreaming'}` : `aiAssistant.thinking.${block.status === 'completed' ? 'progressComplete' : 'progressStreaming'}`)}</span>
        <ChevronRight className={cn('ml-auto size-3.5 transition-transform', expanded && 'rotate-90')} />
      </button>
      <div
        ref={viewportRef}
        className={cn(
          'whitespace-pre-wrap break-words text-[11px] leading-4 text-muted-foreground',
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
      {expanded && !following && <button className="mt-1 text-[10px] font-medium text-primary-text" type="button" onClick={() => setFollowing(true)}>{t('aiAssistant.thinking.backToLatest')}</button>}
    </div>
  )
}
