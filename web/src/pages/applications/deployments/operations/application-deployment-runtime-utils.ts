import type { ClusterResource, DeploymentTarget, RuntimeCluster } from '@/api'
import type { StatusIndicatorState } from '@/components/common/status-tone'
import { statusIndicatorStateFor } from '@/components/common/status-tone'

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
  // 生效集群只取后端统一解析结果，前端不推断默认集群。
  const clusterId = target.effectiveClusterId
  const clusterName = runtimeCluster?.name
  const observedValue = normalizeDeploymentRuntimeStatus(target.status)
  const statusValue = (fallback: string) => observedValue === 'unknown' ? fallback : observedValue
  if (!clusterId)
    return { clusterName, podCount: 0, summary: '', value: statusValue('not-configured') }
  if (errorByCluster[clusterId])
    return { clusterName, podCount: 0, summary: '', value: statusValue('unavailable') }
  if (loadingByCluster[clusterId])
    return { clusterName, podCount: 0, summary: '', value: statusValue('checking') }

  const resources = (resourcesByCluster[clusterId] ?? []).filter(resource => resource.deploymentTargetId === target.id)
  const pods = resources.filter(resource => resource.kind.toLowerCase() === 'pod')
  const workloads = resources.filter(resource => ['deployment', 'statefulset'].includes(resource.kind.toLowerCase()))
  if (resources.length === 0)
    return { clusterName, podCount: 0, summary: '', value: statusValue('not-found') }

  const podStatus = aggregatePodRuntimeStatus(pods)
  if (podStatus)
    return { clusterName, ...podStatus, value: statusValue(podStatus.value) }

  const workload = workloads[0]
  if (!workload)
    return { clusterName, podCount: 0, summary: '', value: observedValue }
  return {
    clusterName,
    podCount: 0,
    summary: workload.summary,
    value: statusValue(normalizeDeploymentRuntimeStatus(workload.status)),
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
  if (value === 'scaled-to-zero')
    return 'scaled-to-zero'
  if (value === 'failed')
    return 'failed'
  return value || 'unknown'
}

const replicaUnobservedStatuses = new Set(['checking', 'disabled', 'not-configured', 'not-found', 'unavailable', 'unknown'])

export interface ReplicaRuntimeState {
  /** 用于状态文案与语义 tone 的有效状态；未部署时为 `not-deployed`。 */
  value: string
  /** 是否有可信的副本观测，可展示 `ready/desired`。 */
  showReplicas: boolean
}

/**
 * 把工作负载观测状态与副本数收敛为一个可展示的有效状态。
 * ready 但副本未全部就绪时降级为 progressing；期望副本为 0 时视为已缩容。
 */
export function effectiveReplicaState({ availableReplicas, deployed = true, desiredReplicas, readyReplicas, status }: {
  availableReplicas?: number
  deployed?: boolean
  desiredReplicas: number
  readyReplicas: number
  status: string
}): ReplicaRuntimeState {
  const normalized = status.trim().toLowerCase() || 'unknown'
  if (!deployed || normalized === 'not-deployed')
    return { showReplicas: false, value: 'not-deployed' }
  if (normalized === 'scaled-to-zero' || (normalized === 'ready' && desiredReplicas === 0))
    return { showReplicas: true, value: 'scaled-to-zero' }
  if (normalized === 'ready' && (readyReplicas < desiredReplicas || (availableReplicas !== undefined && availableReplicas < desiredReplicas)))
    return { showReplicas: true, value: 'progressing' }
  // 停用不会停止已运行的工作负载：后端仍回读真实副本，仍在运行时一并展示。
  if (normalized === 'disabled')
    return { showReplicas: desiredReplicas > 0 || readyReplicas > 0, value: 'disabled' }
  return { showReplicas: !replicaUnobservedStatuses.has(normalized), value: normalized }
}

/** 运行状态到状态指示器：未部署和已缩容不是故障，统一中性。 */
export function replicaIndicatorState(value: string): StatusIndicatorState {
  if (value === 'not-deployed' || value === 'scaled-to-zero')
    return 'neutral'
  return statusIndicatorStateFor(value)
}
