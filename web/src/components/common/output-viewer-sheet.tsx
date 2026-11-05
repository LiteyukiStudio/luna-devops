import type { ReactNode } from 'react'
import { Maximize2, Minimize2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { cn } from '@/lib/utils'
import { ViewerToolbarButton } from './terminal-chrome'

/** 日志与终端的右侧抽屉外壳；全屏切换为抽屉占满视口宽度。 */
export function OutputViewerSheet({
  children,
  description,
  title,
  titleAccessory,
  onClose,
}: {
  children: ReactNode
  description: ReactNode
  title: ReactNode
  titleAccessory?: ReactNode
  onClose: () => void
}) {
  const { t } = useTranslation()
  const [fullscreen, setFullscreen] = useState(false)
  const fullscreenLabel = fullscreen ? t('common.viewer.exitFullscreen') : t('common.viewer.fullscreen')

  return (
    <Sheet open onOpenChange={open => !open && onClose()}>
      <SheetContent className={cn('max-w-none gap-0 overflow-hidden p-0 sm:max-w-none', fullscreen ? 'w-screen' : 'w-[min(100vw,52rem)]')} side="right">
        <SheetHeader className="shrink-0 flex-row items-start gap-3 border-b border-border px-4 py-3 pr-14">
          <div className="grid min-w-0 flex-1 gap-1">
            <div className="flex min-w-0 items-center gap-2">
              <SheetTitle className="truncate text-base">{title}</SheetTitle>
              {titleAccessory}
            </div>
            <SheetDescription className="truncate">{description}</SheetDescription>
          </div>
          {/* 与抽屉右上角关闭按钮垂直居中对齐 */}
          <div className="mt-1.5 shrink-0">
            <ViewerToolbarButton
              icon={fullscreen ? <Minimize2 aria-hidden="true" /> : <Maximize2 aria-hidden="true" />}
              label={fullscreenLabel}
              onClick={() => setFullscreen(value => !value)}
            />
          </div>
        </SheetHeader>
        <div className="flex min-h-0 flex-1 flex-col">
          {children}
        </div>
      </SheetContent>
    </Sheet>
  )
}
