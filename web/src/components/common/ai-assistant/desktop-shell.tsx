import type { ReactNode, RefObject } from 'react'
import type { AIDesktopConversationLayout } from './layout'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { AI_ASSISTANT_SIDEBAR_WIDTH, resolveAIDesktopConversationLayout } from './layout'

interface AIDesktopShellProps {
  chat: ReactNode
  closeLabel: string
  conversationList: (variant: 'drawer' | 'sidebar') => ReactNode
  conversationsOpen: boolean
  /** 测量前的容器宽度；固定 layout 时不需要。 */
  initialWidth?: number
  /** 固定布局；缺省时按容器宽度在侧栏与抽屉之间切换。 */
  layout?: AIDesktopConversationLayout
  listButtonRef: RefObject<HTMLButtonElement | null>
  onCloseConversations: () => void
  onOpenConversations: () => void
}

/**
 * 浮窗与全页共用的会话外壳：宽容器把会话列表作为可折叠侧栏，窄容器作为覆盖抽屉。
 * 抽屉只做进入动画、关闭立即卸载，避免路由回退后旧抽屉残留在可访问树里。
 */
export function AIDesktopShell({
  chat,
  closeLabel,
  conversationList,
  conversationsOpen,
  initialWidth = 0,
  layout: forcedLayout,
  listButtonRef,
  onCloseConversations,
  onOpenConversations,
}: AIDesktopShellProps) {
  const reduceMotion = useReducedMotion()
  const containerRef = useRef<HTMLDivElement>(null)
  const drawerRef = useRef<HTMLDivElement>(null)
  const [containerWidth, setContainerWidth] = useState(initialWidth)
  const layout = forcedLayout ?? resolveAIDesktopConversationLayout(containerWidth)
  const previousLayoutRef = useRef<typeof layout | undefined>(undefined)
  const overlayOpen = conversationsOpen && layout === 'overlay'

  useEffect(() => {
    if (layout === 'split' && previousLayoutRef.current !== 'split')
      onOpenConversations()
    previousLayoutRef.current = layout
  }, [layout, onOpenConversations])

  useEffect(() => {
    const element = containerRef.current
    if (forcedLayout || !element || typeof ResizeObserver === 'undefined')
      return
    const updateWidth = () => setContainerWidth(element.getBoundingClientRect().width || initialWidth)
    const observer = new ResizeObserver(updateWidth)
    observer.observe(element)
    return () => observer.disconnect()
  }, [forcedLayout, initialWidth])

  const closeOverlay = useCallback(() => {
    onCloseConversations()
    window.setTimeout(() => listButtonRef.current?.focus(), 0)
  }, [listButtonRef, onCloseConversations])

  useEffect(() => {
    const drawer = drawerRef.current
    // 抽屉打开后触发按钮位于 inert 区域，焦点需要进入抽屉；列表自带 autoFocus 时保留其选择。
    if (overlayOpen && drawer && !drawer.contains(document.activeElement))
      drawer.focus()
  }, [overlayOpen])

  useEffect(() => {
    if (!overlayOpen)
      return
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape')
        return
      event.preventDefault()
      closeOverlay()
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [closeOverlay, overlayOpen])

  return (
    <div ref={containerRef} className="relative flex size-full min-w-0 overflow-hidden" data-ai-shell-layout={layout}>
      {layout === 'split' && (
        <AnimatePresence initial={false}>
          {conversationsOpen && (
            <motion.div
              animate={{ opacity: 1, width: AI_ASSISTANT_SIDEBAR_WIDTH }}
              className="shrink-0 overflow-hidden"
              exit={{ opacity: 0, width: 0 }}
              initial={{ opacity: 0, width: 0 }}
              transition={reduceMotion ? { duration: 0 } : { type: 'spring', stiffness: 420, damping: 40, mass: 0.8 }}
            >
              {conversationList('sidebar')}
            </motion.div>
          )}
        </AnimatePresence>
      )}

      <div aria-hidden={overlayOpen || undefined} className="flex min-w-0 flex-1 overflow-hidden" inert={overlayOpen || undefined}>
        {chat}
      </div>

      {overlayOpen && (
        <>
          <button
            aria-label={closeLabel}
            className="absolute inset-0 z-20 cursor-default bg-foreground/20 animate-[luna-devops-sheet-overlay-in_180ms_ease-out]"
            type="button"
            onClick={closeOverlay}
          />
          <div ref={drawerRef} className="absolute inset-y-0 left-0 z-30 flex w-[min(20rem,calc(100%-3rem))] outline-none overflow-hidden border-r border-border bg-surface shadow-overlay animate-[luna-devops-sheet-left-in_260ms_var(--ease-emphasized)]" tabIndex={-1}>
            {conversationList('drawer')}
          </div>
        </>
      )}
    </div>
  )
}
