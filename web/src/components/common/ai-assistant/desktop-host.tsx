import type { AIAssistantView } from './route-state'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { AIAssistantChatSurface } from './chat-surface'
import { AIConversationList } from './conversation-list'
import { AIDesktopShell } from './desktop-shell'
import { AI_ASSISTANT_PANEL_WIDTH } from './layout'
import { useAIAssistantRuntime } from './runtime-context'

interface AIAssistantDesktopHostProps {
  view?: AIAssistantView
  /** 把停靠面板展开为全页助手；由布局层负责记住工作区位置并导航。 */
  onExpand?: () => void
  onViewChange?: (view: AIAssistantView) => void
}

/**
 * 桌面端右侧停靠面板：宽屏（xl）并排挤压工作区，较窄桌面覆盖在工作区右侧。
 * 入口在顶栏右上角，关闭后焦点回到该入口。
 */
export function AIAssistantDesktopHost({ view, onExpand, onViewChange }: AIAssistantDesktopHostProps = {}) {
  const { t } = useTranslation()
  const runtime = useAIAssistantRuntime()
  const conversationButtonRef = useRef<HTMLButtonElement>(null)
  const [internalView, setInternalView] = useState<AIAssistantView>('chat')
  const assistantView = view ?? internalView
  const setAssistantView = (nextView: AIAssistantView) => {
    if (view === undefined)
      setInternalView(nextView)
    onViewChange?.(nextView)
  }

  if (!runtime.enabled || !runtime.open)
    return null

  const close = () => {
    runtime.closeAssistant()
    window.setTimeout(() => document.querySelector<HTMLElement>('[data-ai-assistant-trigger]')?.focus(), 0)
  }
  const showConversations = assistantView === 'conversations'
  const chat = (
    <AIAssistantChatSurface
      conversationButtonRef={conversationButtonRef}
      conversationsOpen={showConversations}
      surface="window"
      onClose={close}
      onExpand={onExpand}
      onToggleConversations={() => setAssistantView(showConversations ? 'chat' : 'conversations')}
    />
  )

  return (
    <aside
      aria-label={t('aiAssistant.title')}
      className="fixed inset-y-2 right-2 z-40 flex max-w-[calc(100vw-1rem)] shrink-0 overflow-hidden rounded-container border border-border bg-surface text-[13px] shadow-overlay animate-[luna-devops-sheet-right-in_260ms_var(--ease-emphasized)] motion-reduce:animate-none xl:static xl:my-2 xl:mr-2 xl:shadow-sm"
      data-ai-assistant-panel
      style={{ width: AI_ASSISTANT_PANEL_WIDTH }}
    >
      <AIDesktopShell
        chat={chat}
        closeLabel={t('aiAssistant.conversations.close')}
        conversationsOpen={showConversations}
        initialWidth={AI_ASSISTANT_PANEL_WIDTH}
        listButtonRef={conversationButtonRef}
        conversationList={(variant) => {
          const { refetch, ...conversationListProps } = runtime.conversationListProps
          return (
            <AIConversationList
              {...conversationListProps}
              variant={variant}
              onBack={() => setAssistantView('chat')}
              onRetry={() => void refetch()}
            />
          )
        }}
        onCloseConversations={() => setAssistantView('chat')}
        onOpenConversations={() => setAssistantView('conversations')}
      />
    </aside>
  )
}
