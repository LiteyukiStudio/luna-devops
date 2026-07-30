import type { TFunction } from 'i18next'
import type { Application, BuildRun, DeploymentTarget, GatewayRoute, Release, RepositoryBinding } from '@/api'
import { Activity, CheckCircle2, Database, GitBranch, Globe2, Package, Play, Rocket } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ApplicationIcon } from '@/components/common/application-icon-picker'
import { buildRunImageRef } from '@/components/common/deployment-build-runs'
import { EmptyState } from '@/components/common/empty-state'
import { MetricGroup, MetricItem } from '@/components/common/metric-group'
import { StatusValueBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { parseRuntimeDataVolumes } from '@/lib/runtime-data-volumes'
import { firstReleaseReadyTarget, formatReleaseTime } from './application-config-utils'
import { formatTargetRuntimeSize } from './application-deployments-panel-utils'

export function ApplicationOverviewPanel({ app, buildRuns, deploymentTargets, onBindRepository, onCreateDeploymentTarget, onCreateGatewayRoute, onCreateRelease, onTriggerBuild, releases, repositoryBindings, routes }: {
  app?: Application
  buildRuns: BuildRun[]
  deploymentTargets: DeploymentTarget[]
  releases: Release[]
  repositoryBindings: RepositoryBinding[]
  routes: GatewayRoute[]
  onBindRepository: () => void
  onCreateDeploymentTarget: () => void
  onCreateGatewayRoute: () => void
  onCreateRelease: () => void
  onTriggerBuild: () => void
}) {
  const { t } = useTranslation()
  const enabledTargets = deploymentTargets.filter(target => target.enabled).length
  const latestBuild = latestByDate(buildRuns, run => run.createdAt)
  const latestRelease = latestByDate(releases, release => release.createdAt)
  const readyDeployments = deploymentTargets.filter(target => target.enabled && target.status === 'ready').length
  const readyRoutes = routes.filter(route => route.status === 'ready').length
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
  const deployGuide = buildDeployGuide({
    buildRuns,
    deploymentTargets,
    releases,
    repositoryBindings,
    routes,
    t,
    onBindRepository,
    onCreateDeploymentTarget,
    onCreateGatewayRoute,
    onCreateRelease,
    onTriggerBuild,
  })
  const deployGuideComplete = deployGuide.steps.every(step => step.done)

  return (
    <div className="grid gap-4">
      <Card className="min-w-0 p-4">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <Rocket className="size-4 text-primary-text" />
              <h3 className="text-base font-semibold">{t('apps.deployGuideTitle')}</h3>
            </div>
            <p className="mt-1 text-sm text-muted-foreground">{t('apps.deployGuideDescription')}</p>
          </div>
          <Button className="w-full shrink-0 lg:w-auto" variant={deployGuideComplete ? 'outline' : 'default'} onClick={deployGuide.action}>
            {deployGuide.icon}
            {deployGuide.actionLabel}
          </Button>
        </div>
        {deployGuideComplete
          ? (
              <div className="mt-3 flex items-center gap-2 text-sm text-success">
                <CheckCircle2 className="size-4 shrink-0" />
                <span>{t('apps.deployGuideCompleteDescription')}</span>
              </div>
            )
          : (
              <div className="mt-4 grid gap-2 md:grid-cols-5">
                {deployGuide.steps.map(step => (
                  <div key={step.key} className="min-w-0 rounded-md border border-border bg-muted/30 px-3 py-2">
                    <div className="flex items-center gap-2">
                      {step.done ? <CheckCircle2 className="size-4 shrink-0 text-success" /> : step.icon}
                      <span className="truncate text-sm font-medium">{step.label}</span>
                    </div>
                    <p className="mt-1 truncate text-xs text-muted-foreground" title={step.meta}>{step.meta}</p>
                  </div>
                ))}
              </div>
            )}
      </Card>
      <MetricGroup>
        <MetricItem
          href="?tab=deployments"
          icon={<Package className="size-4" />}
          label={t('apps.deploymentTargetHealth')}
          meta={t('apps.enabledTotal', { enabled: enabledTargets, total: deploymentTargets.length })}
          tone={deploymentTargets.length > 0 && enabledTargets === 0 ? 'warning' : 'neutral'}
          value={String(deploymentTargets.length)}
        />
        <MetricItem
          emphasis={Boolean(latestBuild)}
          href="?tab=builds"
          icon={<Activity className="size-4" />}
          label={t('apps.buildHealth')}
          meta={latestBuild ? formatSmartDateTime(latestBuild.createdAt, t) : t('apps.noRecentBuild')}
          tone={statusTone(latestBuild?.status)}
          value={latestBuild ? t(`buildsPage.statuses.${latestBuild.status}`) : '-'}
        />
        <MetricItem
          emphasis={enabledTargets > 0}
          href="?tab=deployments"
          icon={<Rocket className="size-4" />}
          label={t('apps.deploymentHealth')}
          meta={t('apps.deploymentReadyCount', { ready: readyDeployments, total: enabledTargets })}
          tone={enabledTargets > 0 && readyDeployments < enabledTargets ? 'danger' : readyDeployments > 0 ? 'success' : 'neutral'}
          value={`${readyDeployments} / ${enabledTargets}`}
        />
        <MetricItem
          emphasis={routes.length > 0}
          href="?tab=gateway"
          icon={<Globe2 className="size-4" />}
          label={t('apps.accessHealth')}
          meta={primaryRoute ? routeDisplayUrl(primaryRoute) : t('apps.noAccessRoute')}
          tone={routes.length > 0 && readyRoutes === 0 ? 'danger' : readyRoutes > 0 ? 'success' : 'neutral'}
          value={`${readyRoutes} / ${routes.length}`}
        />
      </MetricGroup>
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
            <OverviewItem label={t('common.identifier')} value={app?.identifier ?? '-'} />
            <OverviewItem label={t('apps.buildConfigsTitle')} value={t('apps.enabledTotal', { enabled: enabledTargets, total: deploymentTargets.length })} />
          </div>
        </Card>
        <Card className="min-w-0 p-4">
          <h3 className="text-base font-semibold">{t('apps.accessEntries')}</h3>
          <div className="mt-3 grid gap-2">
            {routes.length
              ? routes.slice(0, 4).map(route => (
                  <a key={route.id} className="flex min-w-0 items-center justify-between gap-3 rounded-md border border-border px-3 py-2 text-sm transition hover:border-primary/40 hover:text-primary-text" href={routeDisplayUrl(route)} rel="noreferrer" target="_blank">
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
            <h3 className="text-base font-semibold">{t('apps.deploymentSummaries')}</h3>
            <p className="mt-1 text-sm text-muted-foreground">{t('apps.deploymentSummariesDescription')}</p>
          </div>
          <Package className="size-5 shrink-0 text-muted-foreground" />
        </div>
        <div className="mt-3 divide-y divide-border">
          {deploymentTargets.length
            ? deploymentTargets.map(target => (
                <DeploymentSummary
                  key={target.id}
                  latestRelease={latestReleaseForTarget(releases, target)}
                  target={target}
                  t={t}
                />
              ))
            : <EmptyState description={t('apps.emptyBuildConfigs')} title={t('apps.emptyBuildConfigs')} variant="plain" />}
        </div>
      </Card>
      <Card className="min-w-0 p-4">
        <h3 className="text-base font-semibold">{t('apps.recentActivity')}</h3>
        <div className="mt-3 divide-y divide-border">
          {recentActivities.length
            ? recentActivities.map(item => (
                <div key={item.id} className="flex min-w-0 items-center justify-between gap-3 py-3">
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

function buildDeployGuide({
  buildRuns,
  deploymentTargets,
  onBindRepository,
  onCreateDeploymentTarget,
  onCreateGatewayRoute,
  onCreateRelease,
  onTriggerBuild,
  releases,
  repositoryBindings,
  routes,
  t,
}: {
  buildRuns: BuildRun[]
  deploymentTargets: DeploymentTarget[]
  onBindRepository: () => void
  onCreateDeploymentTarget: () => void
  onCreateGatewayRoute: () => void
  onCreateRelease: () => void
  onTriggerBuild: () => void
  releases: Release[]
  repositoryBindings: RepositoryBinding[]
  routes: GatewayRoute[]
  t: TFunction
}) {
  const repositoryTarget = deploymentTargets.find(target => target.sourceType === 'repository')
  const releaseReadyTarget = firstReleaseReadyTarget(deploymentTargets, buildRuns)
  const hasSuccessfulBuild = buildRuns.some(run => run.status === 'succeeded')
  const hasSuccessfulRelease = releases.some(release => release.status === 'succeeded')
  const hasReadyRoute = routes.some(route => route.status === 'ready')
  const repositoryDone = repositoryBindings.length > 0 || deploymentTargets.some(target => target.sourceType === 'image')
  const steps = [
    {
      done: repositoryDone,
      icon: <GitBranch className="size-4 shrink-0 text-muted-foreground" />,
      key: 'source',
      label: t('apps.deployGuideSource'),
      meta: repositoryDone ? t('apps.deployGuideDone') : t('apps.deployGuideSourceMeta'),
    },
    {
      done: deploymentTargets.length > 0,
      icon: <Package className="size-4 shrink-0 text-muted-foreground" />,
      key: 'target',
      label: t('apps.deployGuideTarget'),
      meta: deploymentTargets.length ? t('apps.deployGuideTargetCount', { count: deploymentTargets.length }) : t('apps.deployGuideTargetMeta'),
    },
    {
      done: hasSuccessfulBuild || deploymentTargets.some(target => target.sourceType === 'image'),
      icon: <Play className="size-4 shrink-0 text-muted-foreground" />,
      key: 'build',
      label: t('apps.deployGuideBuild'),
      meta: hasSuccessfulBuild ? t('apps.deployGuideDone') : t('apps.deployGuideBuildMeta'),
    },
    {
      done: hasSuccessfulRelease,
      icon: <Rocket className="size-4 shrink-0 text-muted-foreground" />,
      key: 'release',
      label: t('apps.deployGuideRelease'),
      meta: hasSuccessfulRelease ? t('apps.deployGuideDone') : t('apps.deployGuideReleaseMeta'),
    },
    {
      done: hasReadyRoute,
      icon: <Globe2 className="size-4 shrink-0 text-muted-foreground" />,
      key: 'access',
      label: t('apps.deployGuideAccess'),
      meta: hasReadyRoute ? t('apps.deployGuideDone') : t('apps.deployGuideAccessMeta'),
    },
  ]

  if (deploymentTargets.length === 0) {
    return { action: onCreateDeploymentTarget, actionLabel: t('apps.deployGuideCreateTarget'), icon: <Package className="size-4" />, steps }
  }
  if (repositoryTarget && !repositoryTarget.repositoryBindingId && repositoryBindings.length === 0) {
    return { action: onBindRepository, actionLabel: t('apps.deployGuideBindRepository'), icon: <GitBranch className="size-4" />, steps }
  }
  if (repositoryTarget && !hasSuccessfulBuild) {
    return { action: onTriggerBuild, actionLabel: t('apps.deployGuideTriggerBuild'), icon: <Play className="size-4" />, steps }
  }
  if (releaseReadyTarget && !hasSuccessfulRelease) {
    return { action: onCreateRelease, actionLabel: t('apps.deployGuideCreateRelease'), icon: <Rocket className="size-4" />, steps }
  }
  if (!hasReadyRoute) {
    return { action: onCreateGatewayRoute, actionLabel: t('apps.deployGuideCreateAccess'), icon: <Globe2 className="size-4" />, steps }
  }
  return { action: onCreateRelease, actionLabel: t('apps.deployGuideRedeploy'), icon: <Rocket className="size-4" />, steps }
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

function DeploymentSummary({ latestRelease, target, t }: { latestRelease?: Release, target: DeploymentTarget, t: TFunction }) {
  const storageItems = target.dataRetentionEnabled ? deploymentStorageItems(target) : []

  return (
    <div className="grid min-w-0 gap-3 py-3 text-sm">
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="truncate font-medium" title={target.name}>{target.name}</div>
          <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
            <span>{t(`deploymentsPage.stageLabels.${target.stage}`, { defaultValue: target.stage })}</span>
            <span>{formatTargetRuntimeSize(target, t)}</span>
          </div>
        </div>
        <div className="flex shrink-0 flex-wrap justify-end gap-2">
          <StatusValueBadge value={target.enabled ? 'enabled' : 'disabled'} />
          <StatusValueBadge labelKeyPrefix="deploymentsPage.runtimeStatuses" value={target.status || 'unknown'} />
        </div>
      </div>
      <div className="grid gap-2 sm:grid-cols-3">
        <DeploymentResourceItem
          label={t('deploymentsPage.replicas')}
          value={target.status === 'unavailable'
            ? '-'
            : `${target.readyReplicas || 0} / ${target.desiredReplicas || target.replicas || 1}`}
        />
        <DeploymentResourceItem label={t('deploymentsPage.cpuRequest')} value={target.cpuRequest || '1'} />
        <DeploymentResourceItem label={t('deploymentsPage.memoryRequest')} value={target.memoryRequest || '1Gi'} />
      </div>
      {latestRelease && (
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span>{t('apps.latestRelease')}</span>
          <StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={latestRelease.status} />
          <span>{formatReleaseTime(latestRelease, t)}</span>
        </div>
      )}
      {storageItems.length > 0 && (
        <div className="grid gap-2 rounded-md bg-muted/40 p-2">
          <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
            <Database className="size-3.5" />
            {t('apps.storageResources')}
          </div>
          <div className="flex flex-wrap gap-2">
            {storageItems.map(item => (
              <span key={`${item.name}-${item.mountPath}`} className="min-w-0 max-w-full truncate rounded bg-background px-2 py-1 text-xs text-foreground" title={item.label}>
                {item.label}
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

function statusTone(status?: string): 'danger' | 'info' | 'neutral' | 'success' | 'warning' {
  if (!status)
    return 'neutral'
  if (['failed', 'error', 'cancelled'].includes(status))
    return 'danger'
  if (['pending', 'queued', 'running', 'building', 'deploying'].includes(status))
    return 'warning'
  if (['succeeded', 'ready', 'healthy'].includes(status))
    return 'success'
  return 'info'
}

function DeploymentResourceItem({ label, value }: { label: string, value: string }) {
  return (
    <div className="min-w-0 rounded-md bg-muted/40 px-2 py-1.5">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-0.5 truncate font-medium tabular-nums" title={value}>{value}</div>
    </div>
  )
}

function deploymentStorageItems(target: DeploymentTarget) {
  return parseRuntimeDataVolumes(target.dataVolumes, target.dataMountPath || '/data', target.dataCapacity || '1Gi')
    .map((volume) => {
      const name = volume.name.trim()
      const mountPath = volume.mountPath.trim()
      const capacity = volume.capacity.trim() || target.dataCapacity || '1Gi'
      const labelPrefix = [name, mountPath].filter(Boolean).join(' · ')
      return {
        name,
        mountPath,
        label: labelPrefix ? `${labelPrefix}: ${capacity}` : capacity,
      }
    })
    .filter(item => item.label.trim())
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
