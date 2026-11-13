import type { DeploymentTargetRuntimePod, Release } from '@/api'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/api'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { WORKFLOW_STATUS_REFETCH_INTERVAL_MS } from '@/lib/polling'

export function useReleaseRuntimePods(projectId: string, release: Release, search = '') {
  return useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['deployment-target-runtime-pods', projectId, release.applicationId, release.deploymentTargetId, release.id, search],
    queryFn: ({ signal }) => api.listDeploymentTargetRuntimePods(projectId, release.applicationId, release.deploymentTargetId, { releaseId: release.id, search }, signal),
    enabled: Boolean(projectId && release.applicationId && release.deploymentTargetId),
    refetchInterval: WORKFLOW_STATUS_REFETCH_INTERVAL_MS,
  })
}

export interface RuntimeContainerOption {
  init: boolean
  name: string
}

export function collectRuntimeContainerOptions(
  pods: DeploymentTargetRuntimePod[],
  selectedPod: string,
  includeInit = false,
): RuntimeContainerOption[] {
  const sourcePods = selectedPod ? pods.filter(pod => pod.name === selectedPod) : pods
  const effectivePods = sourcePods.length > 0 ? sourcePods : pods
  const seen = new Set<string>()
  const options: RuntimeContainerOption[] = []
  for (const pod of effectivePods) {
    for (const name of pod.containers ?? []) {
      if (name && !seen.has(name)) {
        seen.add(name)
        options.push({ init: false, name })
      }
    }
    if (includeInit) {
      for (const name of pod.initContainers ?? []) {
        if (name && !seen.has(name)) {
          seen.add(name)
          options.push({ init: true, name })
        }
      }
    }
  }
  return options
}
