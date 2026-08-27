import type { RefObject } from 'react'
import { Bug, List, LoaderCircle, MessageSquarePlus, Sparkles, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { AIAssistantComposer } from './composer'
import { AIOptionsBar } from './options'
import { useAIAssistantRuntime } from './runtime-context'
import { AIAssistantTimeline } from './timeline'

export interface AIAssistantChatSurfaceProps {
  closeLabel?: string
  conversationButtonRef?: RefObject<HTMLButtonElement | null>
  conversationsOpen: boolean
  onClose?: () => void
  onToggleConversations: () => void
  showSuggestions?: boolean
  surface?: 'page' | 'window'
}

export function AIAssistantChatSurface({
  closeLabel,
  conversationButtonRef,
  conversationsOpen,
  onClose,
  onToggleConversations,
  showSuggestions = true,
  surface = 'window',
}: AIAssistantChatSurfaceProps) {
  const { t } = useTranslation()
  const runtime = useAIAssistantRuntime()
  const visibleSuggestions = showSuggestions ? runtime.suggestions : null
  const page = surface === 'page'
  const Title = page ? 'h1' : 'h2'
  const resolvedCloseLabel = closeLabel ?? t('common.close')

  return (
    <div className="flex min-w-0 flex-1 flex-col overflow-hidden" data-ai-assistant-surface={surface}>
      <header className={cn(
        'flex shrink-0 select-none items-center gap-2 border-b border-separator-subtle pb-0 pt-[env(safe-area-inset-top)]',
        page
          ? 'h-[calc(4rem+env(safe-area-inset-top))] pl-[max(0.75rem,env(safe-area-inset-left))] pr-[max(0.75rem,env(safe-area-inset-right))]'
          : 'ai-assistant-drag-handle h-[calc(3.5rem+env(safe-area-inset-top))] pl-[max(0.75rem,env(safe-area-inset-left))] pr-[max(0.75rem,env(safe-area-inset-right))] sm:h-14 sm:cursor-move sm:touch-none sm:px-3 sm:pt-0',
      )}
      >
        <Button
          ref={conversationButtonRef}
          aria-expanded={conversationsOpen}
          aria-label={t('aiAssistant.conversations.title')}
          aria-pressed={conversationsOpen}
          className={cn(
            conversationsOpen && 'bg-primary-subtle text-primary-text hover:bg-primary-subtle',
            page && 'size-11',
          )}
          size="icon"
          title={t('aiAssistant.conversations.title')}
          variant="ghost"
          onClick={onToggleConversations}
        >
          <List className="size-4" />
        </Button>
        <span className={cn('grid shrink-0 place-items-center rounded-control bg-primary text-primary-foreground', page ? 'size-10 max-[359px]:hidden' : 'size-8')}><Sparkles className="size-4" /></span>
        <div className="min-w-0 flex-1">
          <Title className={cn('truncate font-semibold', page ? 'text-base leading-6' : 'text-[13px] leading-5')}>{runtime.conversationTitle || t('aiAssistant.title')}</Title>
          <p className={cn('truncate text-muted-foreground', page ? 'text-xs leading-5' : 'text-[10px] leading-4')}>{t('aiAssistant.context', { path: runtime.workspaceLocation.pathname })}</p>
        </div>
        {runtime.canDebugInternalTools && (
          <Button
            aria-label={t(runtime.toolDebugMode.enabled ? 'aiAssistant.toolDebug.disable' : 'aiAssistant.toolDebug.enable')}
            aria-pressed={runtime.toolDebugMode.enabled}
            className={cn(
              runtime.toolDebugMode.enabled && 'bg-primary-subtle text-primary-text hover:bg-primary-subtle',
              page && 'size-11',
            )}
            size="icon"
            title={t(runtime.toolDebugMode.enabled ? 'aiAssistant.toolDebug.disable' : 'aiAssistant.toolDebug.enable')}
            variant="ghost"
            onClick={runtime.toolDebugMode.toggle}
          >
            <Bug className="size-4" />
          </Button>
        )}
        <Button
          aria-label={t('aiAssistant.conversations.new')}
          className={page ? 'size-11' : undefined}
          disabled={runtime.creatingConversation || !runtime.canCreateConversation}
          size="icon"
          variant="ghost"
          onClick={runtime.createConversation}
        >
          {runtime.creatingConversation ? <LoaderCircle className="size-4 animate-spin motion-reduce:animate-none" /> : <MessageSquarePlus className="size-4" />}
        </Button>
        {onClose && <Button aria-label={resolvedCloseLabel} className={page ? 'size-11' : undefined} size="icon" title={resolvedCloseLabel} variant="ghost" onClick={onClose}><X className="size-4" /></Button>}
      </header>
      <div className="relative flex min-h-0 min-w-0 flex-1 overflow-hidden">
        <AIAssistantTimeline {...runtime.timelineProps} bottomInset={Boolean(visibleSuggestions)} surface={surface} />
        {visibleSuggestions && (
          <AIOptionsBar
            actions={visibleSuggestions.actions}
            sourceKey={visibleSuggestions.sourceKey}
            surface={surface}
            onAction={runtime.executeAction}
          />
        )}
      </div>
      <AIAssistantComposer {...runtime.composerProps} surface={surface} />
    </div>
  )
}
