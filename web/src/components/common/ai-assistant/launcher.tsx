import type { PointerEvent, Ref } from 'react'
import type { Position } from './layout'
import { Sparkles } from 'lucide-react'
import { useRef } from 'react'
import { Button } from '@/components/ui/button'
import { aiAssistantLauncherClassName } from './launcher-appearance'
import { clampAssistantPosition, LAUNCHER_SIZE } from './layout'

const LAUNCHER_TAP_MAX_DISTANCE = 8

interface DragSession {
  pointerId: number
  start: Position
  origin: Position
}

interface AIAssistantLauncherProps {
  label: string
  position: Position
  ref?: Ref<HTMLButtonElement>
  onOpen: () => void
  onPositionChange: (position: Position) => void
}

export function AIAssistantLauncher({
  label,
  position,
  ref,
  onOpen,
  onPositionChange,
}: AIAssistantLauncherProps) {
  const dragRef = useRef<DragSession | null>(null)
  const movedRef = useRef(false)
  const updatePosition = (event: PointerEvent<HTMLButtonElement>) => {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== event.pointerId)
      return
    const next = clampAssistantPosition({
      x: drag.origin.x + event.clientX - drag.start.x,
      y: drag.origin.y + event.clientY - drag.start.y,
    }, LAUNCHER_SIZE, LAUNCHER_SIZE)
    movedRef.current = movedRef.current
      || Math.hypot(event.clientX - drag.start.x, event.clientY - drag.start.y) > LAUNCHER_TAP_MAX_DISTANCE
    onPositionChange(next)
  }
  const releasePointer = (event: PointerEvent<HTMLButtonElement>) => {
    if (event.currentTarget.hasPointerCapture?.(event.pointerId))
      event.currentTarget.releasePointerCapture(event.pointerId)
  }
  const finishPointer = (event: PointerEvent<HTMLButtonElement>) => {
    if (dragRef.current?.pointerId !== event.pointerId)
      return
    releasePointer(event)
    dragRef.current = null
    if (!movedRef.current)
      onOpen()
  }
  const cancelPointer = (event: PointerEvent<HTMLButtonElement>) => {
    if (dragRef.current?.pointerId !== event.pointerId)
      return
    releasePointer(event)
    dragRef.current = null
  }

  return (
    <div className="pointer-events-none fixed inset-0 z-40">
      <Button
        ref={ref}
        aria-label={label}
        className={`${aiAssistantLauncherClassName} pointer-events-auto fixed`}
        size="icon"
        style={{ left: position.x, top: position.y }}
        onClick={event => event.detail === 0 && onOpen()}
        onPointerCancel={cancelPointer}
        onPointerDown={(event) => {
          if (event.isPrimary === false || event.button !== 0)
            return
          movedRef.current = false
          dragRef.current = {
            origin: position,
            pointerId: event.pointerId,
            start: { x: event.clientX, y: event.clientY },
          }
          event.currentTarget.setPointerCapture?.(event.pointerId)
        }}
        onPointerMove={updatePosition}
        onPointerUp={finishPointer}
      >
        <Sparkles className="size-5" />
      </Button>
    </div>
  )
}
