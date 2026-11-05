import type { DeploymentTarget, Release } from '@/api'

export function latestByDate<T>(items: T[], dateOf: (item: T) => string | undefined) {
  return items.reduce<T | undefined>((latest, item) => {
    if (!latest)
      return item
    return new Date(dateOf(item) ?? '').getTime() > new Date(dateOf(latest) ?? '').getTime() ? item : latest
  }, undefined)
}

export function latestReleaseForTarget(releases: Release[], target: DeploymentTarget) {
  return latestByDate(
    releases.filter(release => release.deploymentTargetId === target.id),
    release => release.createdAt,
  )
}

// 越靠前越需要关注；聚合时取最需要关注的状态作为应用整体状态。
const runtimeStatusPriority = ['unavailable', 'degraded', 'failed', 'progressing', 'not-found', 'not-configured', 'unknown', 'checking', 'declared', 'scaled-to-zero', 'ready']

export interface ApplicationRuntimeAggregate {
  deployed: boolean
  desiredReplicas: number
  readyReplicas: number
  status: string
}

/**
 * 应用整体运行状态：只统计已启用且有发布记录的部署配置。
 * 副本数取实时观测值之和，状态取最需要关注的那个。
 */
export function aggregateApplicationRuntime(targets: DeploymentTarget[], releases: Release[]): ApplicationRuntimeAggregate {
  const releasedTargetIds = new Set(releases.map(release => release.deploymentTargetId))
  const deployedTargets = targets.filter(target => target.enabled && releasedTargetIds.has(target.id))
  if (deployedTargets.length === 0)
    return { deployed: false, desiredReplicas: 0, readyReplicas: 0, status: 'not-deployed' }
  const statuses = deployedTargets.map(target => target.status?.trim().toLowerCase() || 'unknown')
  const status = [...statuses].sort((left, right) => priorityOf(left) - priorityOf(right))[0] ?? 'unknown'
  return {
    deployed: true,
    desiredReplicas: deployedTargets.reduce((total, target) => total + target.desiredReplicas, 0),
    readyReplicas: deployedTargets.reduce((total, target) => total + target.readyReplicas, 0),
    status,
  }
}

function priorityOf(status: string) {
  const index = runtimeStatusPriority.indexOf(status)
  return index === -1 ? runtimeStatusPriority.length : index
}
