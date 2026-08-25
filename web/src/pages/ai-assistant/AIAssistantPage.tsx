import { Sparkles } from 'lucide-react'
import { useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation, useNavigate } from 'react-router-dom'
import { AIAssistantChatSurface } from '@/components/common/ai-assistant/chat-surface'
import { AIConversationList } from '@/components/common/ai-assistant/conversation-list'
import { AIMobileViewport } from '@/components/common/ai-assistant/mobile-viewport'
import {
  AI_ASSISTANT_ROUTE_PATH,
  readAIAssistantRouteState,
  resolveAIAssistantReturnPath,
  withAIAssistantRouteView,
} from '@/components/common/ai-assistant/route-state'
import { useAIAssistantRuntime } from '@/components/common/ai-assistant/runtime-context'
import { Button } from '@/components/ui/button'

export function AIAssistantPage() {
  const { t } = useTranslation()
  const location = useLocation()
  const navigate = useNavigate()
  const runtime = useAIAssistantRuntime()
  const routeState = useMemo(() => readAIAssistantRouteState(location.state), [location.state])
  const rememberWorkspaceLocation = runtime.rememberWorkspaceLocation

  useEffect(() => {
    rememberWorkspaceLocation(routeState.returnTo)
  }, [rememberWorkspaceLocation, routeState.returnTo])

  const leaveAssistant = () => {
    runtime.closeAssistant()
    if (hasReturnLocationState(location.state)) {
      navigate(-1)
      return
    }
    navigate(resolveAIAssistantReturnPath(location.state), { replace: true })
  }
  const returnToChat = () => {
    if (routeState.hasChatHistoryEntry) {
      navigate(-1)
      return
    }
    navigate(AI_ASSISTANT_ROUTE_PATH, {
      replace: true,
      state: withAIAssistantRouteView(location.state, 'chat'),
    })
  }

  if (!runtime.enabled) {
    return (
      <AIMobileViewport>
        <section className="flex size-full min-h-0 flex-col bg-surface" data-ai-assistant-page>
          <header className="flex h-[calc(3.5rem+env(safe-area-inset-top))] shrink-0 items-center border-b border-separator-subtle pl-[max(0.75rem,env(safe-area-inset-left))] pr-[max(0.75rem,env(safe-area-inset-right))] pt-[env(safe-area-inset-top)]">
            <Button className="min-h-11" variant="ghost" onClick={leaveAssistant}>{t('aiAssistant.page.backToWorkspace')}</Button>
          </header>
          <div className="grid min-h-0 flex-1 place-items-center pb-[env(safe-area-inset-bottom)] pl-[max(1.5rem,env(safe-area-inset-left))] pr-[max(1.5rem,env(safe-area-inset-right))] text-center">
            <div className="grid max-w-sm justify-items-center gap-3">
              <span className="grid size-12 place-items-center rounded-feature bg-primary-subtle text-primary-text"><Sparkles className="size-5" /></span>
              <h1 className="text-lg font-semibold">{t('aiAssistant.page.unavailableTitle')}</h1>
              <p className="text-sm leading-6 text-muted-foreground">{t('aiAssistant.page.unavailableDescription')}</p>
            </div>
          </div>
        </section>
      </AIMobileViewport>
    )
  }

  if (routeState.aiView === 'conversations') {
    const { refetch, ...conversationListProps } = runtime.conversationListProps
    return (
      <AIMobileViewport>
        <section className="flex size-full min-h-0 flex-col bg-surface" data-ai-assistant-page>
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
        </section>
      </AIMobileViewport>
    )
  }

  return (
    <AIMobileViewport>
      <section className="flex size-full min-h-0 flex-col bg-surface" data-ai-assistant-page>
        <AIAssistantChatSurface
          closeLabel={t('aiAssistant.page.backToWorkspace')}
          conversationsOpen={false}
          surface="page"
          onClose={leaveAssistant}
          onToggleConversations={() => {
            navigate(AI_ASSISTANT_ROUTE_PATH, {
              state: withAIAssistantRouteView(location.state, 'conversations'),
            })
          }}
        />
      </section>
    </AIMobileViewport>
  )
}

function hasReturnLocationState(state: unknown): boolean {
  return typeof state === 'object' && state !== null && 'returnTo' in state
}
