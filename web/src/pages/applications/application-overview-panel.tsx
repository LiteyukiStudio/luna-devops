import type { TFunction } from 'i18next'
import type { ReactNode } from 'react'
import type { Application, BuildRun, DeploymentTarget, GatewayRoute, Release } from '@/api'
import { Activity, Globe2, Package, Rocket } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ApplicationIcon } from '@/components/common/application-icon-picker'
import { buildRunImageRef } from '@/components/common/deployment-build-runs'
import { EmptyState } from '@/components/common/empty-state'
import { StatusValueBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { Card } from '@/components/ui/card'
import { formatReleaseTime } from './application-config-utils'

export function ApplicationOverviewPanel({ app, buildRuns, deploymentTargets, releases, routes }: {
  app?: Application
  buildRuns: BuildRun[]
  deploymentTargets: DeploymentTarget[]
  releases: Release[]
  routes: GatewayRoute[]
}) {
  const { t } = useTranslation()
  const enabledTargets = deploymentTargets.filter(target => target.enabled).length
  const latestBuild = latestByDate(buildRuns, run => run.createdAt)
  const latestRelease = latestByDate(releases, release => release.createdAt)
  const healthyReleases = deploymentTargets.filter((target) => {
    const latest = latestReleaseForTarget(releases, target)
    return latest?.status === 'succeeded'
  }).length
  const primaryRoute = routes.find(route => route.status === 'ready') ?? routes[0]
  const recentActivities = [
    latestBuild && {
      id: `build-${latestBuild.id}`,
      label: t('apps.latestBuild'),
      meta: buildOverviewMeta(latestBuild, t),
      status: latestBuild.status,
      time: formatSmartDateTime(latestBuild.createdAt, t),
    },
    latestRelease && {
      id: `release-${latestRelease.id}`,
      label: t('apps.latestRelease'),
      meta: latestRelease.imageRef || latestRelease.id,
      status: latestRelease.status,
      time: formatReleaseTime(latestRelease, t),
    },
    primaryRoute && {
      id: `route-${primaryRoute.id}`,
      label: t('apps.primaryAccess'),
      meta: routeDisplayUrl(primaryRoute),
      status: primaryRoute.status,
      time: primaryRoute.createdAt ? formatSmartDateTime(primaryRoute.createdAt, t) : '',
    },
  ].filter(Boolean) as Array<{ id: string, label: string, meta: string, status: string, time: string }>

  return (
    <div className="grid gap-4">
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        <OverviewMetric
          icon={<Package className="size-4" />}
          label={t('apps.deploymentTargetHealth')}
          meta={t('apps.enabledTotal', { enabled: enabledTargets, total: deploymentTargets.length })}
          value={String(deploymentTargets.length)}
        />
        <OverviewMetric
          icon={<Activity className="size-4" />}
          label={t('apps.buildHealth')}
          meta={latestBuild ? formatSmartDateTime(latestBuild.createdAt, t) : t('apps.noRecentBuild')}
          status={latestBuild?.status}
          value={latestBuild ? t(`buildsPage.statuses.${latestBuild.status}`) : '-'}
        />
        <OverviewMetric
          icon={<Rocket className="size-4" />}
          label={t('apps.deploymentHealth')}
          meta={t('apps.deploymentReadyCount', { ready: healthyReleases, total: enabledTargets })}
          status={latestRelease?.status}
          value={latestRelease ? t(`buildsPage.statuses.${latestRelease.status}`) : '-'}
        />
        <OverviewMetric
          icon={<Globe2 className="size-4" />}
          label={t('apps.accessHealth')}
          meta={primaryRoute ? routeDisplayUrl(primaryRoute) : t('apps.noAccessRoute')}
          status={primaryRoute?.status}
          value={routes.length ? String(routes.length) : '-'}
        />
      </div>
      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.2fr)_minmax(22rem,0.8fr)]">
        <Card className="min-w-0 p-4">
          <div className="flex items-center justify-between gap-3">
            <div className="min-w-0">
              <h3 className="text-base font-semibold">{t('apps.runtimeOverview')}</h3>
              <p className="mt-1 text-sm text-muted-foreground">{t('apps.runtimeOverviewDescription')}</p>
            </div>
            <div className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
              <ApplicationIcon name={app?.icon ?? 'box'} size={18} />
            </div>
          </div>
          <div className="mt-4 grid gap-3 sm:grid-cols-2">
            <OverviewItem label={t('apps.name')} value={app?.name ?? t('common.loading')} />
            <OverviewItem label={t('common.slug')} value={app?.slug ?? '-'} />
            <OverviewItem label={t('apps.buildConfigsTitle')} value={t('apps.enabledTotal', { enabled: enabledTargets, total: deploymentTargets.length })} />
          </div>
        </Card>
        <Card className="min-w-0 p-4">
          <h3 className="text-base font-semibold">{t('apps.accessEntries')}</h3>
          <div className="mt-3 grid gap-2">
            {routes.length
              ? routes.slice(0, 4).map(route => (
                  <a key={route.id} className="flex min-w-0 items-center justify-between gap-3 rounded-md border border-border px-3 py-2 text-sm transition hover:border-primary/40 hover:text-primary" href={routeDisplayUrl(route)} rel="noreferrer" target="_blank">
                    <span className="min-w-0 truncate">{routeDisplayUrl(route)}</span>
                    <StatusValueBadge labelKeyPrefix="gatewayRoutesPage.statuses" value={route.status} />
                  </a>
                ))
              : <EmptyState description={t('apps.noAccessRoute')} title={t('apps.noAccessRouteTitle')} variant="plain" />}
          </div>
        </Card>
      </div>
      <Card className="min-w-0 p-4">
        <div className="flex items-center justify-between gap-3">
          <div className="min-w-0">
            <h3 className="text-base font-semibold">{t('apps.deploymentTargetEntries')}</h3>
            <p className="mt-1 text-sm text-muted-foreground">{t('apps.deploymentTargetEntriesDescription')}</p>
          </div>
          <Package className="size-5 shrink-0 text-muted-foreground" />
        </div>
        <div className="mt-3 grid gap-2 md:grid-cols-2">
          {deploymentTargets.length
            ? deploymentTargets.slice(0, 6).map(target => (
                <div key={target.id} className="flex min-w-0 items-center justify-between gap-3 rounded-md border border-border px-3 py-2 text-sm">
                  <span className="min-w-0 truncate" title={target.name}>{target.name}</span>
                  <div className="flex shrink-0 items-center gap-2">
                    <StatusValueBadge value={target.enabled ? 'enabled' : 'disabled'} />
                  </div>
                </div>
              ))
            : <EmptyState description={t('apps.emptyBuildConfigs')} title={t('apps.emptyBuildConfigs')} variant="plain" />}
        </div>
      </Card>
      <Card className="min-w-0 p-4">
        <h3 className="text-base font-semibold">{t('apps.recentActivity')}</h3>
        <div className="mt-3 grid gap-2">
          {recentActivities.length
            ? recentActivities.map(item => (
                <div key={item.id} className="flex min-w-0 items-center justify-between gap-3 rounded-md border border-border px-3 py-2">
                  <div className="min-w-0">
                    <div className="text-sm font-medium">{item.label}</div>
                    <div className="mt-1 truncate text-xs text-muted-foreground" title={item.meta}>{item.meta}</div>
                  </div>
                  <div className="flex shrink-0 items-center gap-3">
                    <StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={item.status} />
                    {item.time && <span className="text-xs text-muted-foreground">{item.time}</span>}
                  </div>
                </div>
              ))
            : <EmptyState description={t('apps.noRecentActivityDescription')} title={t('apps.noRecentActivity')} variant="plain" />}
        </div>
      </Card>
    </div>
  )
}

function OverviewMetric({ icon, label, meta, status, value }: { icon: ReactNode, label: string, meta: string, status?: string, value: string }) {
  return (
    <Card className="min-w-0 p-4">
      <div className="flex items-center justify-between gap-3">
        <div className="flex size-9 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">{icon}</div>
        {status && <StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={status} />}
      </div>
      <div className="mt-4 text-2xl font-semibold tracking-normal">{value}</div>
      <div className="mt-1 text-sm font-medium text-foreground">{label}</div>
      <div className="mt-1 truncate text-xs text-muted-foreground" title={meta}>{meta}</div>
    </Card>
  )
}

function OverviewItem({ icon, label, value }: { icon?: string, label: string, value: string }) {
  return (
    <div className="min-w-0">
      <div className="text-xs font-medium uppercase text-muted-foreground">{label}</div>
      <div className="mt-1 flex min-w-0 items-center gap-2 text-sm text-foreground" title={value}>
        {icon && (
          <span className="flex size-7 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
            <ApplicationIcon name={icon} size={16} />
          </span>
        )}
        <span className="truncate">{value}</span>
      </div>
    </div>
  )
}

function latestByDate<T>(items: T[], dateOf: (item: T) => string | undefined) {
  return items.reduce<T | undefined>((latest, item) => {
    if (!latest)
      return item
    return new Date(dateOf(item) ?? '').getTime() > new Date(dateOf(latest) ?? '').getTime() ? item : latest
  }, undefined)
}

function latestReleaseForTarget(releases: Release[], target: DeploymentTarget) {
  return latestByDate(
    releases.filter(release => release.deploymentTargetId === target.id),
    release => release.createdAt,
  )
}

function routeDisplayUrl(route: GatewayRoute) {
  if (route.accessUrl?.trim())
    return route.accessUrl.trim()
  const host = route.host.trim()
  if (!host)
    return '-'
  const protocol = route.tlsMode === 'http-only' ? 'http' : 'https'
  const path = route.path?.startsWith('/') ? route.path : `/${route.path || ''}`
  return `${protocol}://${host}${path === '/' ? '' : path}`
}

function buildOverviewMeta(run: BuildRun, t: TFunction) {
  const ref = run.sourceBranch || run.sourceTag || t('common.unknown')
  const image = buildRunImageRef(run) || run.targetRepository || '-'
  return `${ref} · ${image}`
}
