import type { ReactNode } from 'react'
import type { BuildRun, DeploymentTarget, GatewayRoute, Release } from '@/api'
import { useTranslation } from 'react-i18next'
import { CopyableHoverText } from '@/components/common/copyable-hover-text'
import { StatusDot } from '@/components/common/status-dot'
import { statusToneFor } from '@/components/common/status-tone'
import { formatSmartDateTime } from '@/components/common/time-format'
import { Skeleton } from '@/components/ui/skeleton'
import { shortImageRef } from '@/pages/applications/deployments/application-deployments-panel-utils'
import { ReplicaStatusDot } from '@/pages/applications/deployments/operations/application-deployment-runtime'
import { AccessUrlLink } from '@/pages/applications/gateway/access-url-link'
import { gatewayRouteAccessUrl, primaryGatewayRoute } from '@/pages/applications/gateway/gateway-route-url'
import { formatReleaseTime } from '../application-config-utils'
import { aggregateApplicationRuntime, latestByDate } from './application-facts'

/**
 * 应用详情正文顶部的事实条：整体运行状态、主访问地址、当前版本、最近构建。
 * 数据全部来自页面已有查询；没有发布或构建记录时对应事实不展示。
 */
export function ApplicationFactStrip({ buildRuns, deploymentTargets, loading, releases, routes }: {
  buildRuns: BuildRun[]
  deploymentTargets: DeploymentTarget[]
  loading: boolean
  releases: Release[]
  routes: GatewayRoute[]
}) {
  const { t } = useTranslation()
  const runtime = aggregateApplicationRuntime(deploymentTargets, releases)
  const primaryRoute = primaryGatewayRoute(routes)
  const currentRelease = latestByDate(releases.filter(release => release.status === 'succeeded'), release => release.createdAt)
  const currentReleaseTarget = currentRelease ? deploymentTargets.find(target => target.id === currentRelease.deploymentTargetId) : undefined
  const latestBuild = latestByDate(buildRuns, run => run.createdAt)

  return (
    <dl
      aria-label={t('apps.factStripLabel')}
      className="grid gap-px overflow-hidden rounded-container border border-border bg-border sm:grid-cols-2 xl:grid-cols-4"
      data-slot="application-fact-strip"
    >
      <Fact label={t('apps.runtimeStatus')} loading={loading}>
        <ReplicaStatusDot deployed={runtime.deployed} desiredReplicas={runtime.desiredReplicas} readyReplicas={runtime.readyReplicas} status={runtime.status} />
      </Fact>
      <Fact label={t('apps.primaryAccess')} loading={loading}>
        {primaryRoute
          ? <AccessUrlLink className="text-sm" url={gatewayRouteAccessUrl(primaryRoute)} />
          : <span className="text-sm text-muted-foreground">{t('apps.noAccessRoute')}</span>}
      </Fact>
      <Fact label={t('apps.currentVersion')} loading={loading}>
        {currentRelease
          ? (
              <div className="grid min-w-0 gap-0.5">
                <CopyableHoverText className="min-w-0 font-mono text-sm" display={shortImageRef(currentRelease.imageRef)} value={currentRelease.imageRef} />
                <span className="truncate text-xs text-muted-foreground">
                  {[currentReleaseTarget?.name, formatReleaseTime(currentRelease, t)].filter(Boolean).join(' · ')}
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
