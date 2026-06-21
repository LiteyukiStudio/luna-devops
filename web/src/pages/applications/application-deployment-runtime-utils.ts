import type { ClusterResource, DeploymentTarget, RuntimeCluster } from '@/api/client'

export interface DeploymentRuntimeStatus {
  clusterName?: string
  podCount: number
  summary: string
  value: string
}

export interface InternalServiceEndpointValue {
  fqdn: string
  namespace: string
  serviceName: string
}

export function buildDeploymentRuntimeStatus(
  target: DeploymentTarget,
  runtimeCluster: RuntimeCluster | undefined,
  resourcesByCluster: Record<string, ClusterResource[]>,
  loadingByCluster: Record<string, boolean>,
  errorByCluster: Record<string, boolean>,
): DeploymentRuntimeStatus {
  const clusterId = target.clusterId?.trim() || runtimeCluster?.id
  const clusterName = runtimeCluster?.name
  if (!clusterId)
    return { clusterName, podCount: 0, summary: '', value: 'not-configured' }
  if (errorByCluster[clusterId])
    return { clusterName, podCount: 0, summary: '', value: 'unavailable' }
  if (loadingByCluster[clusterId])
    return { clusterName, podCount: 0, summary: '', value: 'checking' }

  const resources = (resourcesByCluster[clusterId] ?? []).filter(resource => resource.deploymentTargetId === target.id)
  const pods = resources.filter(resource => resource.kind.toLowerCase() === 'pod')
  const deployments = resources.filter(resource => resource.kind.toLowerCase() === 'deployment')
  if (resources.length === 0)
    return { clusterName, podCount: 0, summary: '', value: 'not-found' }

  const podStatus = aggregatePodRuntimeStatus(pods)
  if (podStatus)
    return { clusterName, ...podStatus }

  const deployment = deployments[0]
  if (!deployment)
    return { clusterName, podCount: 0, summary: '', value: 'unknown' }
  return {
    clusterName,
    podCount: 0,
    summary: deployment.summary,
    value: normalizeDeploymentRuntimeStatus(deployment.status),
  }
}

export function buildInternalServiceEndpoint(target: DeploymentTarget, resources: ClusterResource[]): InternalServiceEndpointValue | undefined {
  const service = resources.find(resource => resource.kind.toLowerCase() === 'service' && resource.deploymentTargetId === target.id)
  const serviceName = service?.name.trim()
  const namespace = service?.namespace.trim()
  if (!serviceName || !namespace)
    return undefined

  return {
    fqdn: `${serviceName}.${namespace}.svc.cluster.local`,
    namespace,
    serviceName,
  }
}

function aggregatePodRuntimeStatus(pods: ClusterResource[]): Omit<DeploymentRuntimeStatus, 'clusterName'> | null {
  if (pods.length === 0)
    return null

  const details = pods.map(pod => ({
    pod,
    value: normalizePodRuntimeStatus(pod),
  }))
  const priority = [
    'crash-loop-back-off',
    'image-pull-back-off',
    'err-image-pull',
    'create-container-config-error',
    'create-container-error',
    'failed',
    'pending',
    'container-creating',
    'not-ready',
    'running',
    'ready',
    'succeeded',
    'unknown',
  ]
  const selected = [...details].sort((left, right) => priority.indexOf(left.value) - priority.indexOf(right.value))[0]
  return {
    podCount: pods.length,
    summary: selected?.pod.summary || '',
    value: selected?.value || 'unknown',
  }
}

function normalizePodRuntimeStatus(pod: ClusterResource) {
  const status = pod.status.trim().toLowerCase()
  const summary = pod.summary.trim().toLowerCase()
  if (summary.includes('crashloopbackoff'))
    return 'crash-loop-back-off'
  if (summary.includes('imagepullbackoff'))
    return 'image-pull-back-off'
  if (summary.includes('errimagepull'))
    return 'err-image-pull'
  if (summary.includes('createcontainerconfigerror'))
    return 'create-container-config-error'
  if (summary.includes('createcontainererror'))
    return 'create-container-error'
  if (summary.includes('containercreating'))
    return 'container-creating'
  if (status === 'failed')
    return 'failed'
  if (status === 'succeeded')
    return 'succeeded'
  if (status === 'pending')
    return 'pending'
  if (status === 'running' && summary.includes('ready 1/1'))
    return 'ready'
  if (status === 'running')
    return 'not-ready'
  return status || 'unknown'
}

function normalizeDeploymentRuntimeStatus(status: string) {
  const value = status.trim().toLowerCase()
  if (value === 'ready')
    return 'ready'
  if (value === 'progressing')
    return 'progressing'
  if (value === 'failed')
    return 'failed'
  return value || 'unknown'
}
