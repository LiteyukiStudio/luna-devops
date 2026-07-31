import type { Ref } from 'react'
import type { Position } from './layout'
import { Sparkles } from 'lucide-react'
import { useRef } from 'react'
import { Rnd } from 'react-rnd'
import { Button } from '@/components/ui/button'
import { clampAssistantPosition, LAUNCHER_SIZE } from './layout'

const aiAssistantLauncherClassName = 'size-14 touch-none rounded-full border border-[color:var(--ai-assistant-launcher-border)] text-[color:var(--ai-assistant-launcher-foreground)] [background:var(--ai-assistant-launcher-background)] [box-shadow:var(--ai-assistant-launcher-shadow)] hover:[box-shadow:var(--ai-assistant-launcher-shadow-hover)]'

interface LauncherTouchPoint {
  x: number
  y: number
}

const LAUNCHER_TAP_MAX_DISTANCE = 8

function isLauncherTap(start: LauncherTouchPoint | undefined, end: LauncherTouchPoint | undefined): boolean {
  if (!start || !end)
    return false
  return Math.hypot(end.x - start.x, end.y - start.y) <= LAUNCHER_TAP_MAX_DISTANCE
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
  const dragStartRef = useRef<Position | undefined>(undefined)
  const touchStartRef = useRef<Position | undefined>(undefined)
  const suppressClickRef = useRef(false)

  return (
    <div className="pointer-events-none fixed inset-0 z-40">
      <Rnd
        bounds="parent"
        className="pointer-events-auto"
        enableResizing={false}
        position={position}
        size={{ width: LAUNCHER_SIZE, height: LAUNCHER_SIZE }}
        onDragStart={(_, data) => {
          dragStartRef.current = { x: data.x, y: data.y }
          suppressClickRef.current = false
        }}
        onDragStop={(_, data) => {
          const start = dragStartRef.current
          suppressClickRef.current = Boolean(start && (Math.abs(data.x - start.x) > 3 || Math.abs(data.y - start.y) > 3))
          onPositionChange(clampAssistantPosition({ x: data.x, y: data.y }, LAUNCHER_SIZE, LAUNCHER_SIZE))
        }}
      >
        <Button
          ref={ref}
          aria-label={label}
          className={aiAssistantLauncherClassName}
          size="icon"
          onTouchStart={(event) => {
            const touch = event.touches[0]
            touchStartRef.current = touch ? { x: touch.clientX, y: touch.clientY } : undefined
          }}
          onTouchCancel={() => {
            touchStartRef.current = undefined
          }}
          onTouchEnd={(event) => {
            const touch = event.changedTouches[0]
            const end = touch ? { x: touch.clientX, y: touch.clientY } : undefined
            const tap = isLauncherTap(touchStartRef.current, end)
            touchStartRef.current = undefined
            if (!tap)
              return
            suppressClickRef.current = true
            onOpen()
          }}
          onClick={() => {
            if (suppressClickRef.current) {
              suppressClickRef.current = false
              return
            }
            onOpen()
          }}
        >
          <Sparkles className="size-5" />
        </Button>
      </Rnd>
    </div>
  )
}
