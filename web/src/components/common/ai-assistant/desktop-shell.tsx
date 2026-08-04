import type { ReactNode, RefObject } from 'react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { AI_ASSISTANT_SIDEBAR_WIDTH, resolveAIDesktopConversationLayout } from './layout'

interface AIDesktopShellProps {
  chat: ReactNode
  closeLabel: string
  conversationList: (variant: 'drawer' | 'sidebar') => ReactNode
  conversationsOpen: boolean
  initialWidth: number
  listButtonRef: RefObject<HTMLButtonElement | null>
  onCloseConversations: () => void
  onOpenConversations: () => void
}

export function AIDesktopShell({
  chat,
  closeLabel,
  conversationList,
  conversationsOpen,
  initialWidth,
  listButtonRef,
  onCloseConversations,
  onOpenConversations,
}: AIDesktopShellProps) {
  const reduceMotion = useReducedMotion()
  const containerRef = useRef<HTMLDivElement>(null)
  const [containerWidth, setContainerWidth] = useState(initialWidth)
  const layout = resolveAIDesktopConversationLayout(containerWidth)
  const previousLayoutRef = useRef<typeof layout | undefined>(undefined)
  const overlayOpen = conversationsOpen && layout === 'overlay'

  useEffect(() => {
    if (layout === 'split' && previousLayoutRef.current !== 'split')
      onOpenConversations()
    previousLayoutRef.current = layout
  }, [layout, onOpenConversations])

  useEffect(() => {
    const element = containerRef.current
    if (!element || typeof ResizeObserver === 'undefined')
      return
    const updateWidth = () => setContainerWidth(element.getBoundingClientRect().width || initialWidth)
    const observer = new ResizeObserver(updateWidth)
    observer.observe(element)
    return () => observer.disconnect()
  }, [initialWidth])

  const closeOverlay = useCallback(() => {
    onCloseConversations()
    window.setTimeout(() => listButtonRef.current?.focus(), 0)
  }, [listButtonRef, onCloseConversations])

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

  const spring = reduceMotion
    ? { duration: 0.1 }
    : { type: 'spring' as const, stiffness: 420, damping: 38, mass: 0.8 }

  return (
    <div ref={containerRef} className="relative flex size-full min-w-0 overflow-hidden">
      {layout === 'split' && (
        <AnimatePresence initial={false}>
          {conversationsOpen && (
            <motion.div
              animate={{ opacity: 1, width: AI_ASSISTANT_SIDEBAR_WIDTH }}
              className="shrink-0 overflow-hidden"
              exit={{ opacity: 0, width: 0 }}
              initial={{ opacity: 0, width: 0 }}
              transition={spring}
            >
              {conversationList('sidebar')}
            </motion.div>
          )}
        </AnimatePresence>
      )}

      <div aria-hidden={overlayOpen || undefined} className="flex min-w-0 flex-1 overflow-hidden" inert={overlayOpen || undefined}>
        {chat}
      </div>

      <AnimatePresence initial={false}>
        {overlayOpen && (
          <>
            <motion.button
              aria-label={closeLabel}
              animate={{ opacity: 1 }}
              className="absolute inset-0 z-20 cursor-default bg-foreground/15 backdrop-blur-[1px]"
              exit={{ opacity: 0 }}
              initial={{ opacity: 0 }}
              transition={reduceMotion ? { duration: 0.1 } : { duration: 0.18 }}
              type="button"
              onClick={closeOverlay}
            />
            <motion.div
              animate={{ opacity: 1, x: 0 }}
              className="absolute inset-y-0 left-0 z-30 w-[min(18rem,calc(100%-3rem))] overflow-hidden shadow-overlay"
              exit={{ opacity: 0, x: -24 }}
              initial={{ opacity: 0, x: -24 }}
              transition={spring}
            >
              {conversationList('drawer')}
            </motion.div>
          </>
        )}
      </AnimatePresence>
    </div>
  )
}
