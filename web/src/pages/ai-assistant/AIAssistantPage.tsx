import { Sparkles } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation, useNavigate } from 'react-router-dom'
import { AIAssistantChatSurface } from '@/components/common/ai-assistant/chat-surface'
import { AIConversationList } from '@/components/common/ai-assistant/conversation-list'
import { AIDesktopShell } from '@/components/common/ai-assistant/desktop-shell'
import { AIMobileViewport } from '@/components/common/ai-assistant/mobile-viewport'
import { useAIAssistantPresentationMode } from '@/components/common/ai-assistant/presentation-mode'
import {
  AI_ASSISTANT_ROUTE_PATH,
  readAIAssistantRouteState,
  resolveAIAssistantReturnPath,
  withAIAssistantRouteView,
} from '@/components/common/ai-assistant/route-state'
import { useAIAssistantRuntime } from '@/components/common/ai-assistant/runtime-context'
import { Button } from '@/components/ui/button'

/** 宽屏全页把会话列表作为常驻侧栏；窄屏沿用路由视图驱动的抽屉，浏览器返回即可关闭。 */
const WIDE_PAGE_MEDIA_QUERY = '(min-width: 64rem)'

function subscribeWidePage(onStoreChange: () => void) {
  if (typeof window.matchMedia !== 'function')
    return () => {}
  const mediaQuery = window.matchMedia(WIDE_PAGE_MEDIA_QUERY)
  mediaQuery.addEventListener('change', onStoreChange)
  return () => mediaQuery.removeEventListener('change', onStoreChange)
}

function useWidePage() {
  return useSyncExternalStore(
    subscribeWidePage,
    () => typeof window.matchMedia === 'function' && window.matchMedia(WIDE_PAGE_MEDIA_QUERY).matches,
    () => false,
  )
}

export function AIAssistantPage() {
  const { t } = useTranslation()
  const location = useLocation()
  const navigate = useNavigate()
  const runtime = useAIAssistantRuntime()
  const routeState = useMemo(() => readAIAssistantRouteState(location.state), [location.state])
  const rememberWorkspaceLocation = runtime.rememberWorkspaceLocation
  const wide = useWidePage()
  const [sidebarOpen, setSidebarOpen] = useState(true)
  const conversationButtonRef = useRef<HTMLButtonElement>(null)
  // 用工作区路径判断：当前设备是否能使用桌面浮窗，决定是否提供“收起为浮窗”。
  const canUseWindow = useAIAssistantPresentationMode(runtime.workspaceLocation.pathname) === 'window'

  useEffect(() => {
    rememberWorkspaceLocation(routeState.returnTo)
  }, [rememberWorkspaceLocation, routeState.returnTo])

  const returnToWorkspace = () => {
    if (hasReturnLocationState(location.state)) {
      navigate(-1)
      return
    }
    navigate(resolveAIAssistantReturnPath(location.state), { replace: true })
  }
  const leaveAssistant = () => {
    runtime.closeAssistant()
    returnToWorkspace()
  }
  const collapseToWindow = () => {
    runtime.openAssistant()
    returnToWorkspace()
  }
  const returnToChat = useCallback(() => {
    if (routeState.hasChatHistoryEntry) {
      navigate(-1)
      return
    }
    navigate(AI_ASSISTANT_ROUTE_PATH, {
      replace: true,
      state: withAIAssistantRouteView(location.state, 'chat'),
    })
  }, [location.state, navigate, routeState.hasChatHistoryEntry])
  const openConversations = useCallback(() => {
    navigate(AI_ASSISTANT_ROUTE_PATH, {
      state: withAIAssistantRouteView(location.state, 'conversations'),
    })
  }, [location.state, navigate])
  const openSidebar = useCallback(() => setSidebarOpen(true), [])
  const closeSidebar = useCallback(() => setSidebarOpen(false), [])

  if (!runtime.enabled) {
    return (
      <AIMobileViewport>
        <section className="flex size-full min-h-0 flex-col bg-surface" data-ai-assistant-page>
          <header className="flex h-[calc(3.5rem+env(safe-area-inset-top))] shrink-0 items-center border-b border-border pl-[max(0.75rem,env(safe-area-inset-left))] pr-[max(0.75rem,env(safe-area-inset-right))] pt-[env(safe-area-inset-top)]">
            <Button className="min-h-11 pointer-fine:min-h-9" variant="ghost" onClick={leaveAssistant}>{t('aiAssistant.page.backToWorkspace')}</Button>
          </header>
          <div className="grid min-h-0 flex-1 place-items-center pb-[env(safe-area-inset-bottom)] pl-[max(1.5rem,env(safe-area-inset-left))] pr-[max(1.5rem,env(safe-area-inset-right))] text-center">
            <div className="grid max-w-sm justify-items-center gap-3">
              <span className="grid size-12 place-items-center rounded-full border border-border bg-surface-subtle text-muted-foreground"><Sparkles className="size-5" /></span>
              <h1 className="text-lg font-semibold">{t('aiAssistant.page.unavailableTitle')}</h1>
              <p className="text-sm leading-6 text-muted-foreground">{t('aiAssistant.page.unavailableDescription')}</p>
            </div>
          </div>
        </section>
      </AIMobileViewport>
    )
  }

  const { refetch, ...conversationListProps } = runtime.conversationListProps
  const conversationsOpen = wide ? sidebarOpen : routeState.aiView === 'conversations'

  return (
    <AIMobileViewport>
      <section className="flex size-full min-h-0 flex-col bg-surface" data-ai-assistant-page>
        <AIDesktopShell
          chat={(
            <AIAssistantChatSurface
              closeLabel={t('aiAssistant.page.backToWorkspace')}
              conversationButtonRef={conversationButtonRef}
              conversationsOpen={conversationsOpen}
              surface="page"
              onClose={leaveAssistant}
              onCollapse={canUseWindow ? collapseToWindow : undefined}
              onToggleConversations={() => {
                if (wide)
                  setSidebarOpen(open => !open)
                else if (conversationsOpen)
                  returnToChat()
                else
                  openConversations()
              }}
            />
          )}
          closeLabel={t('aiAssistant.conversations.close')}
          conversationList={variant => variant === 'sidebar'
            ? (
                <AIConversationList
                  {...conversationListProps}
                  variant="sidebar"
                  onRetry={() => void refetch()}
                />
              )
            : (
                <AIConversationList
                  {...conversationListProps}
                  surface="page"
                  variant="mobile"
                  onBack={returnToChat}
                  onClearSearch={() => runtime.setConversationSearch('')}
                  onRetry={() => void refetch()}
                  onSelect={(conversationId) => {
                    runtime.selectConversation(conversationId)
                    returnToChat()
                  }}
                />
              )}
          conversationsOpen={conversationsOpen}
          layout={wide ? 'split' : 'overlay'}
          listButtonRef={conversationButtonRef}
          onCloseConversations={wide ? closeSidebar : returnToChat}
          onOpenConversations={wide ? openSidebar : openConversations}
        />
      </section>
    </AIMobileViewport>
  )
}

function hasReturnLocationState(state: unknown): boolean {
  return typeof state === 'object' && state !== null && 'returnTo' in state
}
