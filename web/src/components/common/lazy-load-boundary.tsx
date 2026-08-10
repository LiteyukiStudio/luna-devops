import type { ErrorInfo, ReactNode } from 'react'
import { Component, Suspense } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ErrorState } from './error-state'
import { ToolViewportSkeleton } from './loading-states'

interface LazyLoadBoundaryProps {
  children: ReactNode
  errorFallback?: ReactNode
  fallback?: ReactNode
  resetKey?: string
}

export function LazyLoadBoundary({ children, errorFallback, fallback, resetKey }: LazyLoadBoundaryProps) {
  return (
    <LazyErrorBoundary key={resetKey} fallback={errorFallback ?? <DefaultLazyLoadError />}>
      <Suspense fallback={fallback ?? <ToolViewportSkeleton />}>
        {children}
      </Suspense>
    </LazyErrorBoundary>
  )
}

class LazyErrorBoundary extends Component<{ children: ReactNode, fallback: ReactNode }, { failed: boolean }> {
  state = { failed: false }

  static getDerivedStateFromError() {
    return { failed: true }
  }

  componentDidCatch(_error: unknown, _info: ErrorInfo) {
    // Lazy chunk failures are presented in the UI; detailed diagnostics remain in browser tooling.
  }

  render() {
    return this.state.failed ? this.props.fallback : this.props.children
  }
}

function DefaultLazyLoadError() {
  const { t } = useTranslation()
  return (
    <div className="grid gap-3">
      <ErrorState title={t('common.lazyLoadFailedTitle')} description={t('common.lazyLoadFailedDescription')} />
      <Button className="justify-self-start" variant="outline" onClick={() => window.location.reload()}>
        {t('common.retry')}
      </Button>
    </div>
  )
}
