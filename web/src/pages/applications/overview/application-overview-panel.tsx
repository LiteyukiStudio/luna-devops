import type { TFunction } from 'i18next'
import type { ApplicationDeploymentSummary, BuildRun, DeploymentTarget, GatewayRoute, Release, RepositoryBinding } from '@/api'
import { CheckCircle2, Circle, GitBranch, Globe2, Package, Play, Rocket } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { buildRunImageRef } from '@/components/common/deployment-build-runs'
import { EmptyState } from '@/components/common/empty-state'
import { HoverText } from '@/components/common/hover-text'
import { Section } from '@/components/common/section'
import { StatusValueBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { primaryGatewayRoute } from '@/pages/applications/gateway/gateway-route-url'
import { firstReleaseReadyTarget, formatReleaseTime } from '../application-config-utils'
import { latestByDate } from './application-facts'

export function ApplicationOverviewPanel({ buildRuns, deploymentTargets, onBindRepository, onCreateDeploymentTarget, onCreateGatewayRoute, onCreateRelease, onTriggerBuild, releases, repositoryBindings, routes, runtimeSummary }: {
  buildRuns: BuildRun[]
  deploymentTargets: DeploymentTarget[]
  releases: Release[]
  repositoryBindings: RepositoryBinding[]
  routes: GatewayRoute[]
  /** 后端统一的应用运行结论；加载中或不可用时为空。 */
  runtimeSummary?: ApplicationDeploymentSummary
  onBindRepository: () => void
  onCreateDeploymentTarget: () => void
  onCreateGatewayRoute: () => void
  onCreateRelease: () => void
  onTriggerBuild: () => void
}) {
  const { t } = useTranslation()
  const latestBuild = latestByDate(buildRuns, run => run.createdAt)
  const latestRelease = latestByDate(releases, release => release.createdAt)
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
  ].filter(Boolean) as Array<{ id: string, label: string, meta: string, status: string, time: string }>
  const deployGuide = buildDeployGuide({
    buildRuns,
    deploymentTargets,
    repositoryBindings,
    routes,
    runtimeSummary,
    t,
    onBindRepository,
    onCreateDeploymentTarget,
    onCreateGatewayRoute,
    onCreateRelease,
    onTriggerBuild,
  })

  return (
    <div className="grid grid-cols-1 gap-section">
      {deployGuide && (
        <Section
          description={t('apps.deployGuideDescription')}
          icon={<Rocket className="size-4" />}
          title={t('apps.deployGuideTitle')}
          tools={(
            <Button variant="outline" onClick={deployGuide.action}>
              {deployGuide.icon}
              {deployGuide.actionLabel}
            </Button>
          )}
          variant="bordered"
        >
          <ol className="grid gap-x-6 gap-y-3 @lg/workspace:grid-cols-2 @4xl/workspace:grid-cols-5">
            {deployGuide.steps.map(step => (
              <li key={step.key} className="flex min-w-0 items-start gap-2">
                {step.done
                  ? <CheckCircle2 aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-success" />
                  : <Circle aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-muted-foreground" />}
                <div className="min-w-0">
                  <div className={cn('truncate text-sm font-medium', step.done && 'text-muted-foreground')}>{step.label}</div>
                  <p className="truncate text-xs text-muted-foreground" title={step.meta}>{step.meta}</p>
                </div>
              </li>
            ))}
          </ol>
        </Section>
      )}
      <Section title={t('apps.recentActivity')} variant="bordered">
        {recentActivities.length
          ? (
              <ul className="-my-3 divide-y divide-border">
                {recentActivities.map(item => (
                  <li key={item.id} className="flex min-w-0 items-center justify-between gap-3 py-3">
                    <div className="min-w-0 flex-1">
                      <div className="text-sm font-medium">{item.label}</div>
                      <HoverText className="mt-0.5 font-mono text-xs text-muted-foreground" value={item.meta} />
                    </div>
                    <div className="flex shrink-0 flex-col items-end gap-1">
                      <StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={item.status} />
                      {item.time && <span className="text-xs text-muted-foreground">{item.time}</span>}
                    </div>
                  </li>
                ))}
              </ul>
            )
          : <EmptyState description={t('apps.noRecentActivityDescription')} title={t('apps.noRecentActivity')} variant="plain" />}
      </Section>
    </div>
  )
}

/** 上线引导：全部完成时返回 null，不再占用概览。 */
function buildDeployGuide({
  buildRuns,
  deploymentTargets,
  onBindRepository,
  onCreateDeploymentTarget,
  onCreateGatewayRoute,
  onCreateRelease,
  onTriggerBuild,
  repositoryBindings,
  routes,
  runtimeSummary,
  t,
}: {
  buildRuns: BuildRun[]
  deploymentTargets: DeploymentTarget[]
  onBindRepository: () => void
  onCreateDeploymentTarget: () => void
  onCreateGatewayRoute: () => void
  onCreateRelease: () => void
  onTriggerBuild: () => void
  repositoryBindings: RepositoryBinding[]
  routes: GatewayRoute[]
  runtimeSummary?: ApplicationDeploymentSummary
  t: TFunction
}) {
  const repositoryTarget = deploymentTargets.find(target => target.sourceType === 'repository')
  const releaseReadyTarget = firstReleaseReadyTarget(deploymentTargets, buildRuns)
  const hasSuccessfulBuild = buildRuns.some(run => run.status === 'succeeded')
  // “发布运行”与事实条使用同一份后端运行结论：部署后实际在运行才算完成。
  const runtimeStatus = runtimeSummary?.status
  const releaseRunning = runtimeStatus === 'ready' || runtimeStatus === 'scaled-to-zero'
  const releaseMeta = releaseRunning
    ? t('apps.deployGuideDone')
    : runtimeStatus && runtimeStatus !== 'not-deployed'
      ? t(`deploymentsPage.runtimeStatuses.${runtimeStatus}`, { defaultValue: runtimeStatus })
      : t('apps.deployGuideReleaseMeta')
  // “开放访问”与事实条的主要访问入口是同一条路由。
  const hasReadyRoute = primaryGatewayRoute(routes)?.status === 'ready'
  const repositoryDone = repositoryBindings.length > 0 || deploymentTargets.some(target => target.sourceType === 'image')
  const steps = [
    {
      done: repositoryDone,
      key: 'source',
      label: t('apps.deployGuideSource'),
      meta: repositoryDone ? t('apps.deployGuideDone') : t('apps.deployGuideSourceMeta'),
    },
    {
      done: deploymentTargets.length > 0,
      key: 'target',
      label: t('apps.deployGuideTarget'),
      meta: deploymentTargets.length ? t('apps.deployGuideTargetCount', { count: deploymentTargets.length }) : t('apps.deployGuideTargetMeta'),
    },
    {
      done: hasSuccessfulBuild || deploymentTargets.some(target => target.sourceType === 'image'),
      key: 'build',
      label: t('apps.deployGuideBuild'),
      meta: hasSuccessfulBuild ? t('apps.deployGuideDone') : t('apps.deployGuideBuildMeta'),
    },
    {
      done: releaseRunning,
      key: 'release',
      label: t('apps.deployGuideRelease'),
      meta: releaseMeta,
    },
    {
      done: hasReadyRoute,
      key: 'access',
      label: t('apps.deployGuideAccess'),
      meta: hasReadyRoute ? t('apps.deployGuideDone') : t('apps.deployGuideAccessMeta'),
    },
  ]

  if (steps.every(step => step.done))
    return null
  if (deploymentTargets.length === 0) {
    return { action: onCreateDeploymentTarget, actionLabel: t('apps.deployGuideCreateTarget'), icon: <Package className="size-4" />, steps }
  }
  if (repositoryTarget && !repositoryTarget.repositoryBindingId && repositoryBindings.length === 0) {
    return { action: onBindRepository, actionLabel: t('apps.deployGuideBindRepository'), icon: <GitBranch className="size-4" />, steps }
  }
  if (repositoryTarget && !hasSuccessfulBuild) {
    return { action: onTriggerBuild, actionLabel: t('apps.deployGuideTriggerBuild'), icon: <Play className="size-4" />, steps }
  }
  if (releaseReadyTarget && !releaseRunning) {
    return { action: onCreateRelease, actionLabel: t('apps.deployGuideCreateRelease'), icon: <Rocket className="size-4" />, steps }
  }
  return { action: onCreateGatewayRoute, actionLabel: t('apps.deployGuideCreateAccess'), icon: <Globe2 className="size-4" />, steps }
}

function buildOverviewMeta(run: BuildRun, t: TFunction) {
  const ref = run.sourceBranch || run.sourceTag || t('common.unknown')
  const image = buildRunImageRef(run) || run.targetRepository || '-'
  return `${ref} · ${image}`
}
