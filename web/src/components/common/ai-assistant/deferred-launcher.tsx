import type { PointerEvent } from 'react'
import type { Position } from './layout'
import { Sparkles } from 'lucide-react'
import { useRef } from 'react'
import { Button } from '@/components/ui/button'
import { aiAssistantLauncherClassName } from './launcher-appearance'
import { clampAssistantPosition, LAUNCHER_SIZE } from './layout'

interface DragSession {
  pointerId: number
  start: Position
  origin: Position
}

export function DeferredAIAssistantLauncher({ label, position, onOpen, onPositionChange }: {
  label: string
  position: Position
  onOpen: () => void
  onPositionChange: (position: Position) => void
}) {
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
      || Math.hypot(event.clientX - drag.start.x, event.clientY - drag.start.y) > 8
    onPositionChange(next)
  }
  const finishPointer = (event: PointerEvent<HTMLButtonElement>) => {
    if (dragRef.current?.pointerId !== event.pointerId)
      return
    event.currentTarget.releasePointerCapture(event.pointerId)
    dragRef.current = null
    if (!movedRef.current)
      onOpen()
  }
  const cancelPointer = (event: PointerEvent<HTMLButtonElement>) => {
    if (dragRef.current?.pointerId !== event.pointerId)
      return
    event.currentTarget.releasePointerCapture(event.pointerId)
    dragRef.current = null
  }
  return (
    <div className="pointer-events-none fixed inset-0 z-40">
      <Button
        aria-label={label}
        className={`${aiAssistantLauncherClassName} pointer-events-auto fixed`}
        size="icon"
        style={{ left: position.x, top: position.y }}
        onClick={event => event.detail === 0 && onOpen()}
        onPointerCancel={cancelPointer}
        onPointerDown={(event) => {
          movedRef.current = false
          dragRef.current = {
            origin: position,
            pointerId: event.pointerId,
            start: { x: event.clientX, y: event.clientY },
          }
          event.currentTarget.setPointerCapture(event.pointerId)
        }}
        onPointerMove={updatePosition}
        onPointerUp={finishPointer}
      >
        <Sparkles className="size-5" />
      </Button>
    </div>
  )
}
