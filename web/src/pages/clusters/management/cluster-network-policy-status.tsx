import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { StatusBadge } from '@/components/common/status-badge'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'

/** 集群项目空间网络隔离的实时比对结果；请求失败时显示不可用，不保留旧值。 */
export function ClusterNetworkPolicyStatus({ clusterId }: { clusterId: string }) {
  const { t } = useTranslation()
  const query = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['runtime-cluster-network-policies', clusterId],
    queryFn: ({ signal }) => api.observeRuntimeClusterNetworkPolicies(clusterId, signal),
    retry: false,
  })
  if (query.isPending)
    return <p className="text-sm text-muted-foreground">{t('clustersPage.networkPolicyStatusLoading')}</p>
  const observation = query.isError ? undefined : query.data
  const status = observation?.status ?? 'unavailable'
  const tone = status === 'ready' ? 'success' : status === 'degraded' ? 'warning' : 'neutral'
  return (
    <div className="flex flex-col gap-1 text-sm" data-testid="cluster-network-policy-status">
      <div className="flex items-center gap-2">
        <span className="text-muted-foreground">{t('clustersPage.networkPolicyStatus')}</span>
        <StatusBadge tone={tone}>{t(`clustersPage.networkPolicyStatusValue.${status}`)}</StatusBadge>
      </div>
      {observation && status !== 'unavailable'
        ? <p className="text-muted-foreground">{t('clustersPage.networkPolicyStatusCounts', { total: observation.namespaceCount, synced: observation.syncedCount, drifted: observation.driftedCount })}</p>
        : <p className="text-muted-foreground">{t(`errors.${observation?.observationCode ?? 'runtime_cluster.network_policy_unavailable'}`, { defaultValue: t('errors.runtime_cluster.network_policy_unavailable') })}</p>}
      {status === 'degraded' ? <p className="text-muted-foreground">{t('clustersPage.networkPolicyDriftHint')}</p> : null}
    </div>
  )
}
