import type { RuntimeCluster } from '@/api'
import { StatusBadge } from '@/components/common/status-badge'
import { isPlatformAdmin } from '@/lib/roles'

export function canManageCluster(cluster: RuntimeCluster, userID?: string, role?: string) {
  if (isPlatformAdmin(role))
    return true
  if (cluster.scope === 'user')
    return cluster.ownerRef === userID
  if (cluster.scope === 'project')
    return true
  return false
}

export function canInspectClusterKubeconfig(cluster: RuntimeCluster, userID?: string, role?: string) {
  return isPlatformAdmin(role) || cluster.createdBy === userID
}

export function normalizeFormPort(value: number, fallback: number) {
  return Number.isFinite(value) && value >= 1 && value <= 65535 ? Math.floor(value) : fallback
}

export function defaultGatewayPublicPort(scheme: RuntimeCluster['gatewayPublicScheme']) {
  return scheme === 'https' ? 443 : 80
}

export function gatewayPublicPortSummary(cluster: RuntimeCluster) {
  const scheme = cluster.gatewayPublicScheme || 'http'
  const port = cluster.gatewayPublicPort || defaultGatewayPublicPort(scheme)
  return `${scheme}:${port}`
}

export function gatewayDomainSuffixSummary(cluster: RuntimeCluster) {
  const suffixes = runtimeClusterDomainSuffixes(cluster)
  if (suffixes.length <= 1)
    return suffixes[0] ?? 'apps.local'
  return `${suffixes[0]} +${suffixes.length - 1}`
}

export function formatGatewayDomainSuffixes(cluster: RuntimeCluster) {
  return runtimeClusterDomainSuffixes(cluster).join('\n')
}

export function runtimeClusterDomainSuffixes(cluster: RuntimeCluster) {
  return parseGatewayDomainSuffixes(cluster.gatewayDomainSuffixes.join('\n'))
}

export function parseGatewayDomainSuffixes(value: string) {
  const seen = new Set<string>()
  const suffixes = value
    .split(/[\n,;]/)
    .map(item => item.trim().toLowerCase().replace(/^\.+|\.+$/g, ''))
    .filter(Boolean)
    .filter((item) => {
      if (seen.has(item))
        return false
      seen.add(item)
      return true
    })
  return suffixes.length > 0 ? suffixes : ['apps.local']
}

export function kubeconfigContextOptionLabel(context: { cluster: string, name: string, namespace: string, server: string }) {
  const details = [context.cluster, context.server, context.namespace].filter(Boolean).join(' · ')
  return details ? `${context.name} (${details})` : context.name
}

export function scopeLabel(cluster: RuntimeCluster, projectMap: Record<string, { name: string }>, t: (key: string, options?: Record<string, unknown>) => string) {
  if (cluster.scope === 'project') {
    return (
      <div className="flex flex-wrap gap-2">
        {(cluster.projectIds ?? []).map(projectId => (
          <StatusBadge key={projectId}>{projectMap[projectId]?.name ?? projectId}</StatusBadge>
        ))}
      </div>
    )
  }
  if (cluster.scope === 'user')
    return t('codeRepositoriesView.scopeUser')
  return t('codeRepositoriesView.scopeGlobal')
}
