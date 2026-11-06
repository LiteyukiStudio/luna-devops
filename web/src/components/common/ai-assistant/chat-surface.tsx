import type { RefObject } from 'react'
import type { AISuggestionSet } from './suggestions'
import type { AIUIAction } from '@/api'
import { Bug, LoaderCircle, Maximize2, Minimize2, PanelLeft, SquarePen, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { AIAssistantMark } from './assistant-mark'
import { AIAssistantComposer } from './composer'
import { AIOptionsBar } from './options'
import { useAIAssistantRuntime } from './runtime-context'
import { AIAssistantTimeline } from './timeline'

export interface AIAssistantChatSurfaceProps {
  closeLabel?: string
  conversationButtonRef?: RefObject<HTMLButtonElement | null>
  conversationsOpen: boolean
  onClose?: () => void
  /** 全页切回桌面浮窗；仅在当前设备可以使用浮窗时提供。 */
  onCollapse?: () => void
  /** 桌面浮窗展开为全页。 */
  onExpand?: () => void
  onToggleConversations: () => void
  showSuggestions?: boolean
  surface?: 'page' | 'window'
}

export function AIAssistantChatSurface({
  closeLabel,
  conversationButtonRef,
  conversationsOpen,
  onClose,
  onCollapse,
  onExpand,
  onToggleConversations,
  showSuggestions = true,
  surface = 'window',
}: AIAssistantChatSurfaceProps) {
  const { t } = useTranslation()
  const runtime = useAIAssistantRuntime()
  const page = surface === 'page'
  const Title = page ? 'h1' : 'h2'
  const resolvedCloseLabel = closeLabel ?? t('common.close')
  const conversationTitle = runtime.conversationTitle
  // 页面模式在触屏设备上保持 44px 命中区，精确指针设备回到 36px 的控件尺寸。
  const headerButtonClassName = cn('text-muted-foreground hover:text-foreground', page ? 'size-11 pointer-fine:size-9' : 'size-8')

  return (
    <div className="flex min-w-0 flex-1 flex-col overflow-hidden bg-surface" data-ai-assistant-surface={surface}>
      <header className={cn(
        'flex shrink-0 select-none items-center gap-1 border-b border-border pt-[env(safe-area-inset-top)]',
        page
          ? 'h-[calc(3.5rem+env(safe-area-inset-top))] pl-[max(0.5rem,env(safe-area-inset-left))] pr-[max(0.5rem,env(safe-area-inset-right))] pointer-fine:h-[calc(3rem+env(safe-area-inset-top))]'
          : 'h-12 px-2',
      )}
      >
        <Button
          ref={conversationButtonRef}
          aria-expanded={conversationsOpen}
          aria-label={t('aiAssistant.conversations.title')}
          aria-pressed={conversationsOpen}
          className={cn(headerButtonClassName, conversationsOpen && 'bg-muted text-foreground')}
          size="icon"
          title={t('aiAssistant.conversations.title')}
          variant="ghost"
          onClick={onToggleConversations}
        >
          <PanelLeft />
        </Button>
        <div className="flex min-w-0 flex-1 items-center gap-2 px-1">
          {!conversationTitle && <AIAssistantMark className="max-[359px]:hidden" />}
          <Title className="min-w-0 truncate text-sm font-medium leading-5" title={conversationTitle || undefined}>
            {conversationTitle || t('aiAssistant.title')}
          </Title>
        </div>
        {runtime.canDebugInternalTools && (
          <Button
            aria-label={t(runtime.toolDebugMode.enabled ? 'aiAssistant.toolDebug.disable' : 'aiAssistant.toolDebug.enable')}
            aria-pressed={runtime.toolDebugMode.enabled}
            className={cn(headerButtonClassName, runtime.toolDebugMode.enabled && 'bg-warning-subtle text-warning hover:bg-warning-subtle hover:text-warning')}
            size="icon"
            title={t(runtime.toolDebugMode.enabled ? 'aiAssistant.toolDebug.disable' : 'aiAssistant.toolDebug.enable')}
            variant="ghost"
            onClick={runtime.toolDebugMode.toggle}
          >
            <Bug />
          </Button>
        )}
        <Button
          aria-label={t('aiAssistant.conversations.new')}
          className={headerButtonClassName}
          disabled={runtime.creatingConversation || !runtime.canCreateConversation}
          size="icon"
          title={t('aiAssistant.conversations.new')}
          variant="ghost"
          onClick={runtime.createConversation}
        >
          {runtime.creatingConversation ? <LoaderCircle className="animate-spin motion-reduce:animate-none" /> : <SquarePen />}
        </Button>
        {onExpand && (
          <Button aria-label={t('aiAssistant.expand')} className={headerButtonClassName} size="icon" title={t('aiAssistant.expand')} variant="ghost" onClick={onExpand}>
            <Maximize2 />
          </Button>
        )}
        {onCollapse && (
          <Button aria-label={t('aiAssistant.collapse')} className={headerButtonClassName} size="icon" title={t('aiAssistant.collapse')} variant="ghost" onClick={onCollapse}>
            <Minimize2 />
          </Button>
        )}
        {onClose && (
          <Button aria-label={resolvedCloseLabel} className={headerButtonClassName} size="icon" title={resolvedCloseLabel} variant="ghost" onClick={onClose}>
            <X />
          </Button>
        )}
      </header>
      <div className="relative flex min-h-0 min-w-0 flex-1 overflow-hidden">
        <AIAssistantTimeline
          {...runtime.timelineProps}
          emptyState={(
            <AIAssistantWelcome
              suggestions={showSuggestions ? runtime.suggestions : null}
              surface={surface}
              onAction={runtime.executeAction}
            />
          )}
          surface={surface}
        />
      </div>
      <AIAssistantComposer
        {...runtime.composerProps}
        contextLabel={t('aiAssistant.context', { path: runtime.workspaceLocation.pathname })}
        surface={surface}
      />
    </div>
  )
}

function AIAssistantWelcome({ suggestions, surface, onAction }: {
  suggestions: AISuggestionSet | null
  surface: 'page' | 'window'
  onAction: (action: AIUIAction) => Promise<boolean>
}) {
  const { t } = useTranslation()
  return (
    <div className="mx-auto grid min-h-full w-full max-w-3xl content-center gap-6 py-6" data-ai-welcome>
      <div className="grid justify-items-center gap-3 text-center">
        <AIAssistantMark size="lg" />
        <div className="grid gap-1.5">
          <p className="text-lg font-semibold leading-7 text-foreground">{t('aiAssistant.emptyTitle')}</p>
          <p className="mx-auto max-w-sm text-sm leading-6 text-muted-foreground">{t('aiAssistant.empty')}</p>
        </div>
      </div>
      {suggestions && (
        <AIOptionsBar
          actions={suggestions.actions}
          placement="welcome"
          sourceKey={suggestions.sourceKey}
          surface={surface}
          onAction={onAction}
        />
      )}
    </div>
  )
}
