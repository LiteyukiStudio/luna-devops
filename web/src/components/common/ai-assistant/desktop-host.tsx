import type { KeyboardEvent, PointerEvent } from 'react'
import type { AIAssistantView } from './route-state'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { AIAssistantChatSurface } from './chat-surface'
import { AIConversationList } from './conversation-list'
import { AIDesktopShell } from './desktop-shell'
import {
  AI_ASSISTANT_PANEL_KEYBOARD_STEP,
  AI_ASSISTANT_PANEL_MIN_WIDTH,
  clampAssistantPanelWidth,
  maxAssistantPanelWidth,
  readAssistantPanelWidth,
  saveAssistantPanelWidth,
} from './layout'
import { useAIAssistantRuntime } from './runtime-context'

interface AIAssistantDesktopHostProps {
  view?: AIAssistantView
  /** 把停靠面板展开为全页助手；由布局层负责记住工作区位置并导航。 */
  onExpand?: () => void
  onViewChange?: (view: AIAssistantView) => void
}

/**
 * 桌面端右侧停靠面板：宽屏（xl）并排挤压工作区，较窄桌面覆盖在工作区右侧。
 * 入口在顶栏右上角，关闭后焦点回到该入口；左边缘可拖拽或用方向键调整宽度并记住。
 */
export function AIAssistantDesktopHost({ view, onExpand, onViewChange }: AIAssistantDesktopHostProps = {}) {
  const { t } = useTranslation()
  const runtime = useAIAssistantRuntime()
  const conversationButtonRef = useRef<HTMLButtonElement>(null)
  const [internalView, setInternalView] = useState<AIAssistantView>('chat')
  const [preferredWidth, setPreferredWidth] = useState(readAssistantPanelWidth)
  const [viewportWidth, setViewportWidth] = useState(() => window.innerWidth)
  const panelWidth = clampAssistantPanelWidth(preferredWidth, viewportWidth)

  useEffect(() => {
    const handleResize = () => setViewportWidth(window.innerWidth)
    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [])
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
      className="fixed inset-y-2 right-2 z-40 flex max-w-[calc(100vw-1rem)] shrink-0 overflow-hidden rounded-container border border-border bg-surface text-[13px] shadow-overlay animate-[luna-devops-sheet-right-in_260ms_var(--ease-emphasized)] motion-reduce:animate-none xl:relative xl:inset-auto xl:my-2 xl:mr-2 xl:shadow-sm"
      data-ai-assistant-panel
      style={{ width: panelWidth }}
    >
      <PanelResizeHandle
        label={t('aiAssistant.resizePanel')}
        maxWidth={maxAssistantPanelWidth(viewportWidth)}
        width={panelWidth}
        onResize={setPreferredWidth}
        onResizeEnd={saveAssistantPanelWidth}
      />
      <AIDesktopShell
        chat={chat}
        closeLabel={t('aiAssistant.conversations.close')}
        conversationsOpen={showConversations}
        initialWidth={panelWidth}
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

/** 面板左边缘的宽度拖拽条：指针拖拽实时调整，松开后保存；键盘用左右方向键步进。 */
function PanelResizeHandle({ label, maxWidth, width, onResize, onResizeEnd }: {
  label: string
  maxWidth: number
  width: number
  onResize: (width: number) => void
  onResizeEnd: (width: number) => void
}) {
  const dragRef = useRef<{ pointerId: number, startX: number, startWidth: number, latest: number } | null>(null)
  const [dragging, setDragging] = useState(false)
  const resizeTo = (next: number) => {
    const clamped = clampAssistantPanelWidth(next, window.innerWidth)
    onResize(clamped)
    return clamped
  }
  const finish = (event: PointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== event.pointerId)
      return
    dragRef.current = null
    setDragging(false)
    if (event.currentTarget.hasPointerCapture?.(event.pointerId))
      event.currentTarget.releasePointerCapture(event.pointerId)
    onResizeEnd(drag.latest)
  }
  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const delta = event.key === 'ArrowLeft' ? AI_ASSISTANT_PANEL_KEYBOARD_STEP : event.key === 'ArrowRight' ? -AI_ASSISTANT_PANEL_KEYBOARD_STEP : 0
    if (!delta)
      return
    event.preventDefault()
    onResizeEnd(resizeTo(width + delta))
  }

  return (
    <div
      aria-label={label}
      aria-orientation="vertical"
      aria-valuemax={maxWidth}
      aria-valuemin={AI_ASSISTANT_PANEL_MIN_WIDTH}
      aria-valuenow={width}
      className="absolute inset-y-0 left-0 z-30 w-1.5 cursor-col-resize touch-none outline-none select-none after:absolute after:inset-y-0 after:left-0 after:w-0.5 after:bg-primary after:opacity-0 after:transition-opacity hover:after:opacity-60 focus-visible:after:opacity-100 data-[dragging]:after:opacity-100"
      data-ai-panel-resize-handle
      data-dragging={dragging || undefined}
      role="separator"
      tabIndex={0}
      title={label}
      onKeyDown={handleKeyDown}
      onPointerCancel={finish}
      onPointerDown={(event) => {
        if (event.button !== 0)
          return
        event.preventDefault()
        dragRef.current = { pointerId: event.pointerId, startX: event.clientX, startWidth: width, latest: width }
        setDragging(true)
        event.currentTarget.setPointerCapture?.(event.pointerId)
      }}
      onPointerMove={(event) => {
        const drag = dragRef.current
        if (!drag || drag.pointerId !== event.pointerId)
          return
        drag.latest = resizeTo(drag.startWidth + drag.startX - event.clientX)
      }}
      onPointerUp={finish}
    />
  )
}
