import type { DeploymentTargetEvent, DeploymentTargetRuntimePod, Release } from '@/api'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/api'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { WORKFLOW_STATUS_REFETCH_INTERVAL_MS } from '@/lib/polling'

export interface DeploymentTargetRuntimeDiagnosticsState {
  active: boolean
  events: DeploymentTargetEvent[]
  eventsFetching: boolean
  eventsLoading: boolean
  eventsUnavailable: boolean
  latestWarningReason?: string
  pods: DeploymentTargetRuntimePod[]
  podsFetching: boolean
  podsLoading: boolean
  podsUnavailable: boolean
  refetchEvents: () => void
  refetchPods: () => void
  warningCount: number
  warningEvents: DeploymentTargetEvent[]
}

/**
 * 统一订阅当前部署目标的 Pods 与近期集群事件，供概览与集群事件 Tab 共享同一份数据源。
 */
export function useDeploymentTargetRuntimeDiagnostics({
  applicationId,
  deployed,
  enabled,
  open,
  projectId,
  release,
  targetId,
}: {
  applicationId: string
  deployed: boolean
  enabled: boolean
  open: boolean
  projectId: string
  release?: Release
  targetId: string
}): DeploymentTargetRuntimeDiagnosticsState {
  const active = Boolean(open && enabled && (deployed || release) && projectId && applicationId && targetId)

  const podsQuery = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['deployment-target-detail-pods', projectId, applicationId, targetId],
    queryFn: ({ signal }) => api.listDeploymentTargetRuntimePods(projectId, applicationId, targetId, { sortBy: 'restartCount', sortOrder: 'desc' }, signal),
    enabled: active,
    refetchInterval: WORKFLOW_STATUS_REFETCH_INTERVAL_MS,
  })

  const eventsQuery = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['deployment-target-events', projectId, applicationId, targetId],
    queryFn: ({ signal }) => api.listDeploymentTargetEvents(projectId, applicationId, targetId, { pageSize: 20 }, signal),
    enabled: active,
    refetchInterval: WORKFLOW_STATUS_REFETCH_INTERVAL_MS,
  })

  const pods = podsQuery.data?.status === 'ready' ? podsQuery.data.items : []
  const events = eventsQuery.data?.status === 'ready' ? eventsQuery.data.items : []
  const warningEvents = events.filter(event => event.type === 'Warning')

  return {
    active,
    events,
    eventsFetching: eventsQuery.isFetching,
    eventsLoading: eventsQuery.isLoading,
    eventsUnavailable: eventsQuery.isError || eventsQuery.data?.status === 'unavailable',
    latestWarningReason: warningEvents[0]?.reason,
    pods,
    podsFetching: podsQuery.isFetching,
    podsLoading: podsQuery.isLoading,
    podsUnavailable: podsQuery.isError || podsQuery.data?.status === 'unavailable',
    refetchEvents: () => {
      void eventsQuery.refetch()
    },
    refetchPods: () => {
      void podsQuery.refetch()
    },
    warningCount: warningEvents.length,
    warningEvents,
  }
}
