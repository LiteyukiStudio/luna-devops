import { useQuery } from '@tanstack/react-query'
import { ExternalLink, RefreshCw, Settings } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { ForbiddenPage } from '@/components/common/forbidden-page'
import { ToolViewportSkeleton } from '@/components/common/loading-states'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { isPlatformAdmin } from '@/lib/roles'

const OPERATIONS_DASHBOARD_URL_KEY = 'site.operationsDashboardUrl'

export function OperationsDashboardPage() {
  const { t } = useTranslation()
  const { user } = useSession()
  const platformAdmin = isPlatformAdmin(user?.role)
  const configs = useQuery({
    queryKey: ['configs'],
    queryFn: api.getConfigs,
    enabled: platformAdmin,
  })
  const dashboardUrl = configs.data?.[OPERATIONS_DASHBOARD_URL_KEY]?.trim() ?? ''
  const iframeUrl = useMemo(() => resolveIframeUrl(dashboardUrl), [dashboardUrl])

  if (!platformAdmin)
    return <ForbiddenPage />

  if (configs.isLoading)
    return <OperationsDashboardSkeleton />

  if (configs.isError) {
    return (
      <ErrorState
        description={t('operationsDashboardPage.loadFailedDescription')}
        title={t('operationsDashboardPage.loadFailedTitle')}
      />
    )
  }

  if (!dashboardUrl) {
    return (
      <EmptyState
        actions={(
          <Button asChild>
            <Link to="/settings/site">
              <Settings size={16} />
              {t('operationsDashboardPage.configure')}
            </Link>
          </Button>
        )}
        description={t('operationsDashboardPage.emptyDescription')}
        title={t('operationsDashboardPage.emptyTitle')}
      />
    )
  }

  if (!iframeUrl) {
    return (
      <ErrorState
        description={t('operationsDashboardPage.invalidDescription')}
        title={t('operationsDashboardPage.invalidTitle')}
      />
    )
  }

  return <OperationsDashboardViewport key={iframeUrl} iframeUrl={iframeUrl} />
}

function OperationsDashboardSkeleton() {
  return <ToolViewportSkeleton />
}

function OperationsDashboardViewport({ iframeUrl }: { iframeUrl: string }) {
  const { t } = useTranslation()
  const [loaded, setLoaded] = useState(false)
  const [timedOut, setTimedOut] = useState(false)

  useEffect(() => {
    const timeout = window.setTimeout(setTimedOut, 10000, true)
    return () => window.clearTimeout(timeout)
  }, [])

  return (
    <Card className="relative overflow-hidden p-0">
      <div className="flex items-center justify-end gap-2 border-b border-border p-2">
        <Button asChild size="sm" variant="ghost">
          <a href={iframeUrl} rel="noreferrer" target="_blank">
            <ExternalLink className="size-4" />
            {t('operationsDashboardPage.openInNewWindow')}
          </a>
        </Button>
      </div>
      {!loaded && !timedOut && <div className="absolute inset-x-0 bottom-0 top-12 z-10"><ToolViewportSkeleton /></div>}
      {timedOut && !loaded && (
        <div className="absolute inset-x-0 bottom-0 top-12 z-20 grid place-items-center bg-surface-raised/95 p-4">
          <div className="grid max-w-lg gap-4 text-center">
            <ErrorState description={t('operationsDashboardPage.iframeTimeoutDescription')} title={t('operationsDashboardPage.iframeTimeoutTitle')} />
            <div className="flex flex-wrap justify-center gap-2">
              <Button onClick={() => window.location.reload()}>
                <RefreshCw className="size-4" />
                {t('common.refresh')}
              </Button>
              <Button asChild variant="outline">
                <a href={iframeUrl} rel="noreferrer" target="_blank">
                  <ExternalLink className="size-4" />
                  {t('operationsDashboardPage.openInNewWindow')}
                </a>
              </Button>
              <Button asChild variant="ghost">
                <Link to="/settings/site">
                  <Settings className="size-4" />
                  {t('operationsDashboardPage.configure')}
                </Link>
              </Button>
            </div>
          </div>
        </div>
      )}
      <iframe
        className="h-[72dvh] min-h-128 max-h-192 w-full border-0 bg-background"
        referrerPolicy="strict-origin-when-cross-origin"
        src={iframeUrl}
        title={t('operationsDashboard')}
        onError={() => setTimedOut(true)}
        onLoad={() => setLoaded(true)}
      />
    </Card>
  )
}

function resolveIframeUrl(rawUrl: string) {
  try {
    const parsed = new URL(rawUrl)
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:')
      return ''
    return parsed.toString()
  }
  catch {
    return ''
  }
}
