import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

function LoadingRegion({ children, className }: { children: ReactNode, className?: string }) {
  const { t } = useTranslation()
  return (
    <div aria-busy="true" className={className} role="status">
      <span className="sr-only">{t('common.loading')}</span>
      {children}
    </div>
  )
}

/** Initial application bootstrap state shown before the authenticated shell is ready. */
export function AppLoadingState({ logoUrl, title }: { logoUrl: string, title: string }) {
  return (
    <LoadingRegion className="grid min-h-screen place-items-center bg-surface-base p-4">
      <div className="grid justify-items-center gap-4">
        <img alt="" className="size-12 rounded-xl object-contain" src={logoUrl} />
        <div className="grid justify-items-center gap-2">
          <span className="text-sm font-medium text-foreground">{title}</span>
          <Skeleton className="h-2 w-24" />
        </div>
      </div>
    </LoadingRegion>
  )
}

/** Table-shaped loading state that preserves column rhythm and list height. */
export function DataListSkeleton({ columns = 4, rows = 6 }: { columns?: number, rows?: number }) {
  return (
    <LoadingRegion className="min-w-full">
      <div className="grid h-10 items-center gap-4 bg-muted/70 px-4" style={{ gridTemplateColumns: `repeat(${columns}, minmax(7rem, 1fr))` }}>
        {Array.from({ length: columns }, (_, index) => <Skeleton key={index} className="h-3 w-20 max-w-full" />)}
      </div>
      <div>
        {Array.from({ length: rows }, (_, row) => (
          <div key={row} className="grid min-h-14 items-center gap-4 border-t border-border px-4 py-3" style={{ gridTemplateColumns: `repeat(${columns}, minmax(7rem, 1fr))` }}>
            {Array.from({ length: columns }, (_, column) => (
              <Skeleton key={column} className={cn('h-4 max-w-full', column === 0 ? 'w-32' : 'w-20')} />
            ))}
          </div>
        ))}
      </div>
    </LoadingRegion>
  )
}

/** Dashboard/overview loading state with stable metric and content geometry. */
export function OverviewSkeleton() {
  return (
    <LoadingRegion className="grid gap-4">
      <div className="grid gap-px overflow-hidden rounded-lg bg-border sm:grid-cols-2 xl:grid-cols-4">
        {Array.from({ length: 4 }, (_, index) => (
          <div key={index} className="grid gap-3 bg-surface-raised p-4">
            <Skeleton className="h-4 w-24" />
            <Skeleton className="h-7 w-16" />
          </div>
        ))}
      </div>
      <div className="grid gap-4 xl:grid-cols-3">
        <div className="grid gap-3 rounded-lg bg-surface-raised p-4 xl:col-span-2">
          <Skeleton className="h-5 w-28" />
          {Array.from({ length: 4 }, (_, index) => <Skeleton key={index} className="h-12 w-full" />)}
        </div>
        <div className="grid content-start gap-3 rounded-lg bg-surface-raised p-4">
          <Skeleton className="h-5 w-24" />
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
        </div>
      </div>
    </LoadingRegion>
  )
}

/** Settings form loading state with a constrained form width. */
export function SettingsSkeleton() {
  return (
    <LoadingRegion className="grid max-w-3xl gap-4 rounded-lg bg-surface-raised p-4">
      {Array.from({ length: 5 }, (_, index) => (
        <div key={index} className="grid gap-2">
          <Skeleton className="h-4 w-28" />
          <Skeleton className="h-10 w-full" />
        </div>
      ))}
    </LoadingRegion>
  )
}

/** Marketplace loading state that mirrors responsive template cards. */
export function TemplateGridSkeleton() {
  return (
    <LoadingRegion className="grid gap-3 sm:grid-cols-2 sm:gap-4 xl:grid-cols-3">
      {Array.from({ length: 6 }, (_, index) => (
        <div key={index} className="grid min-h-56 gap-4 rounded-lg bg-surface-raised p-4">
          <div className="flex gap-3">
            <Skeleton className="size-12 shrink-0" />
            <div className="grid flex-1 content-start gap-2">
              <Skeleton className="h-5 w-32" />
              <Skeleton className="h-4 w-full" />
              <Skeleton className="h-4 w-4/5" />
            </div>
          </div>
          <Skeleton className="h-12 w-full" />
          <Skeleton className="mt-auto h-9 w-24 justify-self-end" />
        </div>
      ))}
    </LoadingRegion>
  )
}

/** Embedded tool loading state that keeps the viewport visible while content connects. */
export function ToolViewportSkeleton() {
  return (
    <LoadingRegion className="grid min-h-96 place-items-center rounded-lg bg-surface-inset p-4">
      <div className="grid w-full max-w-sm justify-items-center gap-3">
        <Skeleton className="size-10 rounded-full" />
        <Skeleton className="h-4 w-40" />
        <Skeleton className="h-3 w-56 max-w-full" />
      </div>
    </LoadingRegion>
  )
}
