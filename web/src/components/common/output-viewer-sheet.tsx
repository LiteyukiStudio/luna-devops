import type { ReactNode } from 'react'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { cn } from '@/lib/utils'

export function OutputViewerSheet({
  children,
  contentClassName,
  description,
  title,
  titleAccessory,
  onClose,
}: {
  children: ReactNode
  contentClassName?: string
  description: ReactNode
  title: ReactNode
  titleAccessory?: ReactNode
  onClose: () => void
}) {
  return (
    <Sheet open onOpenChange={open => !open && onClose()}>
      <SheetContent className={cn('w-[min(100vw,48rem)] max-w-none gap-0 overflow-hidden p-0 sm:max-w-none', contentClassName)} side="right">
        <SheetHeader className="shrink-0 gap-1 border-b border-border px-4 py-3 pr-14">
          <div className="flex min-w-0 items-center gap-2">
            <SheetTitle className="truncate text-base">{title}</SheetTitle>
            {titleAccessory}
          </div>
          <SheetDescription>{description}</SheetDescription>
        </SheetHeader>
        <div className="flex min-h-0 flex-1 flex-col">
          {children}
        </div>
      </SheetContent>
    </Sheet>
  )
}
