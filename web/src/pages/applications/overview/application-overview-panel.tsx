import type { TFunction } from 'i18next'
import type { BuildRun, DeploymentTarget, GatewayRoute, Release, RepositoryBinding } from '@/api'
import { CheckCircle2, Circle, GitBranch, Globe2, Package, Play, Rocket } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { buildRunImageRef } from '@/components/common/deployment-build-runs'
import { EmptyState } from '@/components/common/empty-state'
import { HoverText } from '@/components/common/hover-text'
import { Section } from '@/components/common/section'
import { StatusValueBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { parseRuntimeDataVolumes } from '@/lib/runtime-data-volumes'
import { cn } from '@/lib/utils'
import { DeploymentRuntimeSpec, ReplicaStatusDot } from '@/pages/applications/deployments/operations/application-deployment-runtime'
import { firstReleaseReadyTarget, formatReleaseTime } from '../application-config-utils'
import { latestByDate, latestReleaseForTarget } from './application-facts'

export function ApplicationOverviewPanel({ buildRuns, deploymentTargets, onBindRepository, onCreateDeploymentTarget, onCreateGatewayRoute, onCreateRelease, onTriggerBuild, releases, repositoryBindings, routes }: {
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
          <ol className="grid gap-x-6 gap-y-3 sm:grid-cols-2 xl:grid-cols-5">
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
      <div className="grid grid-cols-1 items-start gap-4 xl:grid-cols-[minmax(0,1.4fr)_minmax(20rem,0.6fr)]">
        <Section description={t('apps.deploymentSummariesDescription')} title={t('apps.deploymentSummaries')} variant="bordered">
          {deploymentTargets.length
            ? (
                <ul className="-my-3 divide-y divide-border">
                  {deploymentTargets.map(target => (
                    <DeploymentSummary
                      key={target.id}
                      latestRelease={latestReleaseForTarget(releases, target)}
                      target={target}
                      t={t}
                    />
                  ))}
                </ul>
              )
            : <EmptyState description={t('apps.deployGuideTargetMeta')} title={t('apps.emptyBuildConfigs')} variant="plain" />}
        </Section>
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
      done: hasSuccessfulRelease,
      key: 'release',
      label: t('apps.deployGuideRelease'),
      meta: hasSuccessfulRelease ? t('apps.deployGuideDone') : t('apps.deployGuideReleaseMeta'),
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
  if (releaseReadyTarget && !hasSuccessfulRelease) {
    return { action: onCreateRelease, actionLabel: t('apps.deployGuideCreateRelease'), icon: <Rocket className="size-4" />, steps }
  }
  return { action: onCreateGatewayRoute, actionLabel: t('apps.deployGuideCreateAccess'), icon: <Globe2 className="size-4" />, steps }
}

function DeploymentSummary({ latestRelease, target, t }: { latestRelease?: Release, target: DeploymentTarget, t: TFunction }) {
  const storageItems = deploymentStorageItems(target)

  return (
    <li className="grid min-w-0 gap-2 py-3 text-sm sm:grid-cols-[minmax(0,1fr)_auto] sm:items-start sm:gap-4">
      <div className="min-w-0">
        <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
          <span className="truncate font-medium" title={target.name}>{target.name}</span>
          <span className="text-xs text-muted-foreground">{t(`deploymentsPage.stageLabels.${target.stage}`, { defaultValue: target.stage })}</span>
          {!target.enabled && <StatusValueBadge value="disabled" />}
        </div>
        <dl className="mt-1 flex min-w-0 flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
          <div className="flex min-w-0 gap-1">
            <dt>{t('apps.runtimeSpec')}</dt>
            <dd><DeploymentRuntimeSpec target={target} /></dd>
          </div>
          {latestRelease && (
            <div className="flex min-w-0 gap-1">
              <dt>{t('apps.latestRelease')}</dt>
              <dd className="truncate">
                {t(`buildsPage.statuses.${latestRelease.status}`, { defaultValue: latestRelease.status })}
                {' · '}
                {formatReleaseTime(latestRelease, t)}
              </dd>
            </div>
          )}
          {storageItems.length > 0 && (
            <div className="flex min-w-0 gap-1">
              <dt>{t('apps.storageResources')}</dt>
              <dd className="truncate font-mono" title={storageItems.join('\n')}>{storageItems.join(', ')}</dd>
            </div>
          )}
        </dl>
      </div>
      <ReplicaStatusDot
        availableReplicas={target.availableReplicas}
        deployed={Boolean(latestRelease)}
        desiredReplicas={target.desiredReplicas}
        readyReplicas={target.readyReplicas}
        status={target.enabled ? target.status || 'unknown' : 'disabled'}
      />
    </li>
  )
}

function deploymentStorageItems(target: DeploymentTarget) {
  return parseRuntimeDataVolumes(target.dataVolumes)
    .map((volume) => {
      const name = volume.name.trim()
      const mountPath = volume.mountPath.trim() || volume.devicePath.trim()
      return [name, mountPath].filter(Boolean).join(' → ')
    })
    .filter(Boolean)
}

function buildOverviewMeta(run: BuildRun, t: TFunction) {
  const ref = run.sourceBranch || run.sourceTag || t('common.unknown')
  const image = buildRunImageRef(run) || run.targetRepository || '-'
  return `${ref} · ${image}`
}
