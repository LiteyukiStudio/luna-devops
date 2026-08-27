import type { WindowPreference } from './layout'
import type { AIAssistantView } from './route-state'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Rnd } from 'react-rnd'
import { safeStorageSet } from '@/lib/safe-storage'
import { AIAssistantChatSurface } from './chat-surface'
import { AIConversationList } from './conversation-list'
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
  VIEWPORT_GUTTER,
  WINDOW_STORAGE_KEY,
} from './layout'
import { useAIAssistantRuntime } from './runtime-context'

interface AIAssistantDesktopHostProps {
  view?: AIAssistantView
  onViewChange?: (view: AIAssistantView) => void
}

function clampWindowPreference(preference: WindowPreference): WindowPreference {
  const availableWidth = Math.max(MIN_WINDOW_WIDTH, window.innerWidth - VIEWPORT_GUTTER * 2)
  const availableHeight = Math.max(MIN_WINDOW_HEIGHT, window.innerHeight - VIEWPORT_GUTTER * 2)
  const width = Math.min(availableWidth, Math.max(MIN_WINDOW_WIDTH, preference.width))
  const height = Math.min(availableHeight, Math.max(MIN_WINDOW_HEIGHT, preference.height))
  return {
    ...clampAssistantPosition(preference, width, height),
    width,
    height,
  }
}

export function AIAssistantDesktopHost({ view, onViewChange }: AIAssistantDesktopHostProps = {}) {
  const { t } = useTranslation()
  const runtime = useAIAssistantRuntime()
  const triggerRef = useRef<HTMLButtonElement>(null)
  const conversationButtonRef = useRef<HTMLButtonElement>(null)
  const [internalView, setInternalView] = useState<AIAssistantView>('chat')
  const [preference, setPreference] = useState(readWindowPreference)
  const [launcherPosition, setLauncherPosition] = useState(readLauncherPosition)
  const assistantView = view ?? internalView
  const setAssistantView = (nextView: AIAssistantView) => {
    if (view === undefined)
      setInternalView(nextView)
    onViewChange?.(nextView)
  }

  useEffect(() => {
    if (!runtime.enabled)
      return
    safeStorageSet(WINDOW_STORAGE_KEY, JSON.stringify(preference))
  }, [preference, runtime.enabled])
  useEffect(() => {
    if (!runtime.enabled)
      return
    safeStorageSet(LAUNCHER_STORAGE_KEY, JSON.stringify(launcherPosition))
  }, [launcherPosition, runtime.enabled])
  useEffect(() => {
    if (!runtime.enabled)
      return
    const handleResize = () => {
      setPreference(clampWindowPreference)
      setLauncherPosition(current => clampAssistantPosition(current, LAUNCHER_SIZE, LAUNCHER_SIZE))
    }
    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [runtime.enabled])

  if (!runtime.enabled)
    return null

  if (!runtime.open) {
    return (
      <AIAssistantLauncher
        ref={triggerRef}
        label={t('aiAssistant.open')}
        position={launcherPosition}
        onOpen={runtime.openAssistant}
        onPositionChange={setLauncherPosition}
      />
    )
  }

  const close = () => {
    runtime.closeAssistant()
    window.setTimeout(() => triggerRef.current?.focus(), 0)
  }
  const showConversations = assistantView === 'conversations'
  const panelPreference = clampWindowPreference(preference)
  const chat = (
    <AIAssistantChatSurface
      conversationButtonRef={conversationButtonRef}
      conversationsOpen={showConversations}
      surface="window"
      onClose={close}
      onToggleConversations={() => setAssistantView(showConversations ? 'chat' : 'conversations')}
    />
  )

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
        position={{ x: panelPreference.x, y: panelPreference.y }}
        size={{ width: panelPreference.width, height: panelPreference.height }}
        onDragStop={(_, data) => setPreference(current => ({ ...current, x: data.x, y: data.y }))}
        onResizeStop={(_, __, element, ___, position) => {
          setPreference(clampWindowPreference({
            x: position.x,
            y: position.y,
            width: element.offsetWidth,
            height: element.offsetHeight,
          }))
        }}
      >
        <section aria-label={t('aiAssistant.title')} className="relative flex size-full overflow-hidden rounded-feature border border-border bg-surface text-[13px] shadow-overlay">
          <AIDesktopShell
            chat={chat}
            closeLabel={t('common.back')}
            conversationsOpen={showConversations}
            initialWidth={panelPreference.width}
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
        </section>
      </Rnd>
    </div>
  )
}
