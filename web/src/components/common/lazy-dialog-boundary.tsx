import type { ReactNode } from 'react'
import { LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogTitle } from '@/components/ui/dialog'
import { LazyLoadBoundary } from './lazy-load-boundary'

export function LazyDialogBoundary({ children, onOpenChange, resetKey }: {
  children: ReactNode
  onOpenChange: (open: boolean) => void
  resetKey: string
}) {
  const { t } = useTranslation()
  const loading = (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogTitle>{t('common.loading')}</DialogTitle>
        <DialogDescription className="sr-only">{t('common.loading')}</DialogDescription>
        <div className="grid min-h-32 place-items-center" role="status">
          <LoaderCircle className="size-6 animate-spin text-muted-foreground motion-reduce:animate-none" />
        </div>
      </DialogContent>
    </Dialog>
  )
  const failed = (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogTitle>{t('common.lazyLoadFailedTitle')}</DialogTitle>
        <DialogDescription>{t('common.lazyLoadFailedDescription')}</DialogDescription>
        <DialogFooter>
          <Button variant="outline" onClick={() => window.location.reload()}>{t('common.retry')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
  return (
    <LazyLoadBoundary errorFallback={failed} fallback={loading} resetKey={resetKey}>
      {children}
    </LazyLoadBoundary>
  )
}
