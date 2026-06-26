import type { TFunction } from 'i18next'
import type { ArtifactRegistry, BuildRun, DeploymentTarget, Release } from '@/api'
import { latestDeployableBuildRuns } from '@/components/common/deployment-build-runs'
import { formatSmartDateTime } from '@/components/common/time-format'

export function formatReleaseTime(release: Release, t: TFunction) {
  if (release.finishedAt)
    return formatSmartDateTime(release.finishedAt, t)
  if (release.startedAt)
    return formatSmartDateTime(release.startedAt, t)
  return formatSmartDateTime(release.createdAt, t)
}

export function gatewayDeploymentTargetLabel(target: DeploymentTarget, t: TFunction) {
  return `${target.name} · ${t(`deploymentsPage.stages.${target.stage}`, { defaultValue: target.stage })}`
}

export function deploymentReleaseKey(deploymentTargetId: string) {
  return deploymentTargetId
}

export function firstReleaseReadyTarget(targets: DeploymentTarget[], runs: BuildRun[]) {
  const deployableRuns = latestDeployableBuildRuns(runs)
  return targets.find(target => deploymentTargetCanRelease(target, deployableRuns))
}

export function deploymentTargetCanRelease(target: DeploymentTarget, deployableRuns: BuildRun[]) {
  if (!target.enabled)
    return false
  if (target.sourceType === 'image')
    return Boolean(target.imageRef?.trim())
  return deployableRuns.some(run => run.deploymentTargetId === target.id)
}

export function shortBuildId(value: string) {
  const index = value.indexOf('_')
  if (index >= 0)
    return value.slice(index + 1, index + 9)
  return value.slice(0, 8)
}

export function firstSelectableDeploymentTarget(configs: DeploymentTarget[]) {
  return configs.find(config => config.enabled) ?? configs[0]
}

export function deploymentTargetImageRef(config?: DeploymentTarget) {
  if (!config?.targetRepository)
    return ''
  return `${config.targetRepository}:${config.targetTag || 'latest'}`
}

export function registryInputPrefix(registry: ArtifactRegistry) {
  if (isDockerHubRegistry(registry))
    return ''
  const host = registryHost(registry.endpoint)
  return host ? `${host}/` : ''
}

export function registryOptionLabel(registry: ArtifactRegistry) {
  return registry.namespace ? `${registry.name} / ${registry.namespace}` : registry.name
}

export function branchOptions(values: Array<{ name: string }>, current?: string) {
  const options = values.map(branch => ({ value: branch.name, label: branch.name }))
  const normalized = current?.trim()
  if (normalized && !options.some(option => option.value === normalized))
    options.unshift({ value: normalized, label: normalized })
  return options
}

export function defaultTargetImageRef(registry: ArtifactRegistry | undefined, projectSlug: string, appSlug: string) {
  const imageName = [slugSegment(projectSlug), slugSegment(appSlug)].filter(Boolean).join('-')
  if (!imageName)
    return ''
  const namespace = registry?.namespace?.trim().replace(/^\/+|\/+$/g, '') || slugSegment(projectSlug)
  return `${namespace ? `${namespace}/` : ''}${imageName}:latest`
}

function isDockerHubRegistry(registry: ArtifactRegistry) {
  return registry.provider === 'dockerhub' || registry.endpoint.includes('docker.io')
}

function registryHost(endpoint: string) {
  return endpoint.replace(/^https?:\/\//, '').replace(/\/.*$/, '')
}

function slugSegment(value: string) {
  return value.trim().replace(/^\/+|\/+$/g, '').toLowerCase()
}
