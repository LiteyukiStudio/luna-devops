import type { ReactNode } from 'react'
import type { Application, BuildRun, DeploymentTarget, GatewayRoute } from '@/api'
import { ArrowRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { CopyableHoverText } from '@/components/common/copyable-hover-text'
import { StatusBadge } from '@/components/common/status-badge'
import { StatusDot } from '@/components/common/status-dot'
import { isInProgressStatus, statusToneFor } from '@/components/common/status-tone'
import { formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { shortImageRef } from '@/lib/image-ref'
import { ReplicaStatusDot } from '@/pages/applications/deployments/operations/application-deployment-runtime'
import { AccessUrlLink } from '@/pages/applications/gateway/access-url-link'
import { gatewayRouteAccessUrl, primaryGatewayRoute } from '@/pages/applications/gateway/gateway-route-url'
import { latestByDate } from './application-facts'

/**
 * 应用详情正文顶部的事实条：整体运行状态、主访问地址、当前版本、最近构建。
 * 运行状态与当前版本直接展示后端统一结论（runtime），页面不再自行聚合。
 * 各部署配置的明细只在“部署”Tab 展示，事实条只给出跳转入口。
 */
export function ApplicationFactStrip({ buildRuns, deploymentTargets, loading, onViewDeployments, routes, runtime, runtimeError }: {
  buildRuns: BuildRun[]
  deploymentTargets: DeploymentTarget[]
  loading: boolean
  routes: GatewayRoute[]
  runtime?: Application
  runtimeError: boolean
  /** 当前不在“部署”Tab 时提供，用于从运行状态跳到各部署配置明细。 */
  onViewDeployments?: () => void
}) {
  const { t } = useTranslation()
  const summary = runtime?.deploymentSummary
  const primaryRoute = primaryGatewayRoute(routes)
  const currentRelease = runtime?.currentRelease
  const currentReleaseTarget = currentRelease ? deploymentTargets.find(target => target.id === currentRelease.deploymentTargetId) : undefined
  const latestBuild = latestByDate(buildRuns, run => run.createdAt)

  return (
    <dl
      aria-label={t('apps.factStripLabel')}
      className="grid gap-px overflow-hidden rounded-container border border-border bg-border @lg/workspace:grid-cols-2 @4xl/workspace:grid-cols-4"
      data-slot="application-fact-strip"
    >
      <Fact label={t('apps.runtimeStatus')} loading={loading}>
        <div className="flex min-w-0 flex-wrap items-center justify-between gap-x-3 gap-y-1">
          {summary
            ? <ReplicaStatusDot desiredReplicas={summary.desiredReplicas} readyReplicas={summary.readyReplicas} status={summary.status} />
            : <ReplicaStatusDot desiredReplicas={0} readyReplicas={0} status={runtimeError ? 'unavailable' : 'unknown'} />}
          {onViewDeployments && (
            <Button className="h-auto shrink-0 p-0 text-xs text-primary-text" type="button" variant="link" onClick={onViewDeployments}>
              {t('apps.viewDeployments')}
              <ArrowRight aria-hidden="true" className="size-3.5" />
            </Button>
          )}
        </div>
      </Fact>
      <Fact label={t('apps.primaryAccess')} loading={loading}>
        {primaryRoute
          ? (
              <div className="flex min-w-0 items-center gap-2">
                <AccessUrlLink className="min-w-0 text-sm" url={gatewayRouteAccessUrl(primaryRoute)} />
                {primaryRoute.status !== 'ready' && <StatusBadge className="shrink-0" tone="warning">{t('apps.primaryAccessNotReady')}</StatusBadge>}
              </div>
            )
          : <span className="text-sm text-muted-foreground">{t('apps.noAccessRoute')}</span>}
      </Fact>
      <Fact label={t('apps.currentVersion')} loading={loading}>
        {currentRelease
          ? (
              <div className="grid min-w-0 gap-0.5">
                <CopyableHoverText className="min-w-0 font-mono text-sm" display={shortImageRef(currentRelease.imageRef)} value={currentRelease.imageRef} />
                <span className="truncate text-xs text-muted-foreground">
                  {[currentReleaseTarget?.name, formatSmartDateTime(currentRelease.deployedAt, t)].filter(Boolean).join(' · ')}
                </span>
              </div>
            )
          : <span className="text-sm text-muted-foreground">{t('apps.noCurrentVersion')}</span>}
      </Fact>
      <Fact label={t('apps.latestBuild')} loading={loading}>
        {latestBuild
          ? (
              <StatusDot
                detail={formatSmartDateTime(latestBuild.createdAt, t)}
                label={t(`buildsPage.statuses.${latestBuild.status}`, { defaultValue: latestBuild.status })}
                pulse={isInProgressStatus(latestBuild.status)}
                tone={statusToneFor(latestBuild.status)}
              />
            )
          : <span className="text-sm text-muted-foreground">{t('apps.noRecentBuild')}</span>}
      </Fact>
    </dl>
  )
}

function Fact({ children, label, loading }: { children: ReactNode, label: string, loading: boolean }) {
  return (
    <div className="grid min-w-0 content-start gap-1.5 bg-surface-raised px-4 py-3">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="min-w-0">{loading ? <Skeleton className="h-5 w-28" /> : children}</dd>
    </div>
  )
}
