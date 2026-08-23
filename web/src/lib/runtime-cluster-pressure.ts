import type { RuntimeClusterPressure, RuntimeClusterPressureLevel } from '@/api'
import type { StatusTone } from '@/components/common/status-tone'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/api'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'

export function useRuntimeClusterPressure({ clusterIds, enabled = true, projectId }: {
  clusterIds: string[]
  enabled?: boolean
  projectId?: string
}) {
  const normalizedIds = [...new Set(clusterIds.filter(Boolean))].sort()
  const query = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['runtime-cluster-pressure', projectId ?? '', normalizedIds],
    queryFn: () => api.observeRuntimeClusterPressure(normalizedIds, projectId),
    enabled: enabled && normalizedIds.length > 0,
    refetchInterval: 10_000,
    refetchIntervalInBackground: false,
    retry: false,
  })
  const observations = query.isError ? [] : query.data ?? []
  const byClusterId = Object.fromEntries(observations.map(item => [item.clusterId, item])) as Record<string, RuntimeClusterPressure>
  return { ...query, byClusterId }
}

export function runtimeClusterPressureTone(level: RuntimeClusterPressureLevel): StatusTone {
  switch (level) {
    case 'idle':
      return 'success'
    case 'light':
      return 'neutral'
    case 'moderate':
      return 'info'
    case 'heavy':
      return 'warning'
    case 'full':
      return 'danger'
    case 'unavailable':
      return 'neutral'
  }
}
