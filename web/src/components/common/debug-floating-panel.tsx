import type { PointerEvent, ReactNode } from 'react'
import { Bug, RotateCcw, ShieldCheck, X } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useSession } from '@/app/session-context'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { NativeSelect } from '@/components/ui/native-select'

const positionStorageKey = 'liteyuki-devops.debug.floatingPosition'
const triggerSize = 56
const panelWidth = 320
const viewportPadding = 12

interface DebugPosition {
  x: number
  y: number
}

interface DragState {
  moved: boolean
  pointerId: number
  startPointerX: number
  startPointerY: number
  startX: number
  startY: number
}

/**
 * 开发环境专用的悬浮调试面板。
 * 用于本地预览不同角色/权限下的 UI 状态；生产模式会自动隐藏，不要把业务操作入口放进这里。
 */
export function DebugFloatingPanel() {
  const isDeveloperMode = import.meta.env.DEV
  const { t } = useTranslation()
  const { actualUser, clearDebugOverride, debugOverride, setDebugOverride, user } = useSession()
  const [open, setOpen] = useState(false)
  const [viewport, setViewport] = useState(() => currentViewport())
  const [position, setPosition] = useState<DebugPosition>(() => readPosition())
  const dragRef = useRef<DragState | null>(null)
  const suppressClickRef = useRef(false)
  const selectedRole = debugOverride?.type === 'role' ? debugOverride.role : 'actual'
  const panelPosition = useMemo(() => {
    const left = clamp(position.x, viewportPadding, Math.max(viewportPadding, viewport.width - panelWidth - viewportPadding))
    const below = position.y + triggerSize + 8
    const top = below + 360 > viewport.height
      ? clamp(position.y - 360 - 8, viewportPadding, Math.max(viewportPadding, viewport.height - 360 - viewportPadding))
      : below
    return { left, top }
  }, [position.x, position.y, viewport.height, viewport.width])

  useEffect(() => {
    if (!isDeveloperMode)
      return

    const handleResize = () => {
      setViewport(currentViewport())
      setPosition(value => clampPosition(value))
    }
    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [isDeveloperMode])

  useEffect(() => {
    if (isDeveloperMode)
      localStorage.setItem(positionStorageKey, JSON.stringify(position))
  }, [isDeveloperMode, position])

  if (!isDeveloperMode || !actualUser || !user)
    return null

  const handlePointerDown = (event: PointerEvent<HTMLButtonElement>) => {
    if (event.button !== 0)
      return

    event.currentTarget.setPointerCapture(event.pointerId)
    dragRef.current = {
      moved: false,
      pointerId: event.pointerId,
      startPointerX: event.clientX,
      startPointerY: event.clientY,
      startX: position.x,
      startY: position.y,
    }
  }

  const handlePointerMove = (event: PointerEvent<HTMLButtonElement>) => {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== event.pointerId)
      return

    const deltaX = event.clientX - drag.startPointerX
    const deltaY = event.clientY - drag.startPointerY
    if (Math.abs(deltaX) > 4 || Math.abs(deltaY) > 4)
      drag.moved = true

    setPosition(clampPosition({ x: drag.startX + deltaX, y: drag.startY + deltaY }))
  }

  const handlePointerUp = (event: PointerEvent<HTMLButtonElement>) => {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== event.pointerId)
      return

    suppressClickRef.current = drag.moved
    dragRef.current = null
    event.currentTarget.releasePointerCapture(event.pointerId)
  }

  const handleClick = () => {
    if (suppressClickRef.current) {
      suppressClickRef.current = false
      return
    }
    setOpen(value => !value)
  }

  const handleRoleChange = (role: string) => {
    if (role === 'actual') {
      clearDebugOverride()
      return
    }
    setDebugOverride({ type: 'role', role: role as 'platform_admin' | 'user' })
  }

  const handleReset = () => {
    clearDebugOverride()
  }

  return (
    <>
      <Button
        aria-label={t('debugPanel.trigger')}
        aria-pressed={open}
        className="fixed z-50 size-14 cursor-grab touch-none rounded-full border border-primary/30 bg-primary text-primary-foreground shadow-lg shadow-primary/20 active:cursor-grabbing"
        size="icon"
        style={{ left: position.x, top: position.y }}
        title={t('debugPanel.trigger')}
        onClick={handleClick}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
      >
        <Bug className="size-6" />
      </Button>
      {open && (
        <section
          className="fixed z-50 w-80 rounded-lg border border-border bg-surface p-4 shadow-xl"
          style={{ left: panelPosition.left, top: panelPosition.top }}
        >
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <h2 className="text-sm font-semibold">{t('debugPanel.title')}</h2>
                <Badge variant="outline">{t('debugPanel.devOnly')}</Badge>
              </div>
              <p className="mt-1 text-xs text-muted-foreground">{t('debugPanel.description')}</p>
            </div>
            <Button aria-label={t('common.close')} className="size-8 shrink-0" size="icon" variant="ghost" onClick={() => setOpen(false)}>
              <X className="size-4" />
            </Button>
          </div>

          <div className="mt-4 grid gap-4">
            <DebugField icon={<ShieldCheck className="size-4" />} label={t('debugPanel.roleView')}>
              <NativeSelect aria-label={t('debugPanel.roleView')} value={selectedRole} onChange={event => handleRoleChange(event.target.value)}>
                <option value="actual">{t('debugPanel.actualRole')}</option>
                <option value="platform_admin">{t('debugPanel.platformAdminView')}</option>
                <option value="user">{t('debugPanel.normalUserView')}</option>
              </NativeSelect>
            </DebugField>

            <div className="rounded-md border border-border bg-muted/40 p-3 text-xs">
              <p className="font-medium">{t('debugPanel.effectiveUser')}</p>
              <p className="mt-1 truncate text-muted-foreground">{user.name || user.email}</p>
              <p className="mt-1 text-muted-foreground">
                {user.role === 'platform_admin' ? t('debugPanel.platformAdminView') : t('debugPanel.normalUserView')}
              </p>
            </div>

            <Button className="w-full justify-center gap-2" variant="outline" onClick={handleReset}>
              <RotateCcw className="size-4" />
              {t('debugPanel.reset')}
            </Button>
          </div>
        </section>
      )}
    </>
  )
}

function DebugField({ children, icon, label }: { children: ReactNode, icon: ReactNode, label: string }) {
  return (
    <label className="grid gap-2 text-sm">
      <span className="flex items-center gap-2 font-medium">
        {icon}
        {label}
      </span>
      {children}
    </label>
  )
}

function currentViewport() {
  if (typeof window === 'undefined')
    return { height: 720, width: 1280 }

  return { height: window.innerHeight, width: window.innerWidth }
}

function readPosition(): DebugPosition {
  if (typeof window === 'undefined')
    return { x: 1200, y: 560 }

  try {
    const raw = localStorage.getItem(positionStorageKey)
    if (raw) {
      const parsed = JSON.parse(raw) as DebugPosition
      if (Number.isFinite(parsed.x) && Number.isFinite(parsed.y))
        return clampPosition(parsed)
    }
  }
  catch {
    localStorage.removeItem(positionStorageKey)
  }

  return clampPosition({
    x: window.innerWidth - triggerSize - 24,
    y: window.innerHeight - triggerSize - 32,
  })
}

function clampPosition(position: DebugPosition): DebugPosition {
  const viewport = currentViewport()
  return {
    x: clamp(position.x, viewportPadding, Math.max(viewportPadding, viewport.width - triggerSize - viewportPadding)),
    y: clamp(position.y, viewportPadding, Math.max(viewportPadding, viewport.height - triggerSize - viewportPadding)),
  }
}

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max)
}
