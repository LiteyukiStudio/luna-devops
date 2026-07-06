import type { ClusterResource, ClusterResourceEvent, CurrentUser, RuntimeCluster } from '@/api'
import { ChevronDown, ChevronRight, FileCode2, ScrollText, SquareTerminal, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CopyableHoverText } from '@/components/common/copyable-hover-text'
import { DataList } from '@/components/common/data-list'
import { EmptyState } from '@/components/common/empty-state'
import { StatusValueBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { canDeleteClusterResource } from './cluster-resource-utils'

export interface ClusterResourcePagination {
  page: number
  pageSize: number
  total: number
  totalPages: number
  pageInfoLabel: string
  pageSizeOptions: number[]
  onPageChange: (page: number) => void
  onPageSizeChange: (pageSize: number) => void
}

type ClusterResourceRow = ClusterResource & {
  depth?: number
  hasChildren?: boolean
  parentId?: string
}
export function ClusterResourcesPanel({ items, loading, pagination, selectedCluster, selectedResourceKeys, tab, user, onDeleteResource, onOpenConsole, onOpenEvents, onOpenYAML, onSelectionChange }: {
  items: ClusterResource[]
  loading: boolean
  pagination?: ClusterResourcePagination
  selectedCluster?: RuntimeCluster
  selectedResourceKeys: string[]
  tab: string
  user?: CurrentUser
  onDeleteResource: (resource: ClusterResource) => void
  onOpenConsole: (resource: ClusterResource) => void
  onOpenEvents: (resource: ClusterResource) => void
  onOpenYAML: (resource: ClusterResource) => void
  onSelectionChange: (keys: string[]) => void
}) {
  const { t } = useTranslation()
  const [expandedResourceKeys, setExpandedResourceKeys] = useState<string[]>([])
  const expandedResourceKeySet = useMemo(() => new Set(expandedResourceKeys), [expandedResourceKeys])
  const rowItems = useMemo<ClusterResourceRow[]>(() => {
    if (tab !== 'workloads')
      return items
    return items.flatMap((item) => {
      const children = item.children ?? []
      const parent: ClusterResourceRow = { ...item, hasChildren: children.length > 0 }
      if (!expandedResourceKeySet.has(item.id))
        return [parent]
      return [
        parent,
        ...children.map(child => ({ ...child, depth: 1, parentId: item.id })),
      ]
    })
  }, [expandedResourceKeySet, items, tab])
  useEffect(() => {
    if (tab !== 'workloads')
      return
    const validKeys = new Set(items.map(item => item.id))
    setExpandedResourceKeys(keys => keys.filter(key => validKeys.has(key)))
  }, [items, tab])
  const itemKeys = new Set(rowItems.map(item => item.id))
  const visibleSelectedResourceKeys = selectedResourceKeys.filter(key => itemKeys.has(key))
  const selectedResources = rowItems.filter(item => visibleSelectedResourceKeys.includes(item.id) && canDeleteClusterResource(user, item) && !item.parentId)
  const canOpenWebConsole = (item: ClusterResourceRow) => {
    return tab === 'workloads' && user?.role === 'platform_admin' && item.kind.toLowerCase() === 'pod' && Boolean(item.namespace && item.name)
  }
  const toggleResourceExpansion = (resource: ClusterResourceRow) => {
    setExpandedResourceKeys((keys) => {
      if (keys.includes(resource.id))
        return keys.filter(key => key !== resource.id)
      return [...keys, resource.id]
    })
  }
  if (!selectedCluster) {
    return (
      <EmptyState
        title={t('clustersPage.noManageableClusterTitle')}
        description={t('clustersPage.noManageableClusterDescription')}
      />
    )
  }
  return (
    <DataList
      columns={[
        { key: 'kind', header: t('clustersPage.resourceKind'), className: 'w-32 whitespace-nowrap', render: item => item.kind },
        {
          key: 'name',
          header: t('common.name'),
          className: 'min-w-56 whitespace-nowrap',
          render: item => (
            <div className="flex min-w-0 items-center gap-2">
              {tab === 'workloads' && !item.parentId && (
                item.hasChildren
                  ? (
                      <button
                        aria-label={expandedResourceKeySet.has(item.id) ? t('clustersPage.collapseWorkloadPods') : t('clustersPage.expandWorkloadPods')}
                        className="inline-flex size-6 shrink-0 items-center justify-center rounded-md text-muted-foreground transition hover:bg-muted hover:text-foreground"
                        type="button"
                        onClick={() => toggleResourceExpansion(item)}
                      >
                        {expandedResourceKeySet.has(item.id) ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
                      </button>
                    )
                  : <span className="size-6 shrink-0" />
              )}
              {tab === 'workloads' && item.parentId && (
                <span className="ml-8 flex h-6 w-9 shrink-0 items-center border-l border-border">
                  <span className="h-px w-full border-t border-border" />
                </span>
              )}
              <TruncatedResourceText className={`${item.parentId ? 'max-w-64 text-muted-foreground' : 'max-w-72'} font-mono text-sm`} value={clusterResourceName(item, tab === 'namespaces')} />
            </div>
          ),
        },
        ...(tab === 'namespaces'
          ? []
          : [{ key: 'namespace', header: t('deploymentsPage.namespace'), className: 'w-44 whitespace-nowrap', render: (item: ClusterResource) => <TruncatedResourceText className="max-w-44 font-mono text-sm" value={item.namespace || '-'} /> }]),
        { key: 'status', header: t('common.status'), className: 'w-28 whitespace-nowrap', render: item => <StatusValueBadge value={normalizeClusterResourceStatus(item.status)} /> },
        { key: 'updatedAt', header: t('clustersPage.resourceUpdatedAt'), className: 'w-32 whitespace-nowrap', render: item => formatSmartDateTime(item.updatedAt || item.createdAt, t) },
        { key: 'owner', header: t('clustersPage.resourceOwner'), className: 'min-w-56', render: item => <TruncatedResourceText className="max-w-72 text-sm" value={clusterResourceOwner(item)} /> },
        { key: 'summary', header: t('clustersPage.resourceSummary'), className: 'min-w-64', render: item => <TruncatedResourceText className="max-w-80 text-sm text-muted-foreground" value={item.summary || '-'} /> },
        {
          key: 'actions',
          header: t('common.actions'),
          className: 'w-72 min-w-72 whitespace-nowrap text-right',
          sticky: 'right',
          render: item => (
            <div className="flex justify-end gap-2">
              {canOpenWebConsole(item) && (
                <Button size="sm" variant="ghost" onClick={() => onOpenConsole(item)}>
                  <SquareTerminal className="size-4" />
                  {t('clustersPage.webConsole')}
                </Button>
              )}
              <Button size="sm" variant="ghost" onClick={() => onOpenEvents(item)}>
                <ScrollText className="size-4" />
                {t('clustersPage.viewEvents')}
              </Button>
              <Button size="sm" variant="ghost" onClick={() => onOpenYAML(item)}>
                <FileCode2 className="size-4" />
                {t('clustersPage.viewYaml')}
              </Button>
              {canDeleteClusterResource(user, item) && (
                <Button size="sm" variant="ghost" onClick={() => onDeleteResource(item)}>
                  <Trash2 className="size-4" />
                  {t('common.delete')}
                </Button>
              )}
            </div>
          ),
        },
      ]}
      emptyDescription={loading ? t('common.loading') : t('clustersPage.resourceEmptyDescription')}
      emptyTitle={loading ? t('common.loading') : t(`clustersPage.${tab}EmptyTitle`)}
      items={rowItems}
      pagination={pagination}
      rowKey={item => item.id}
      selection={{
        isRowSelectable: item => canDeleteClusterResource(user, item) && !item.parentId,
        selectAllLabel: t('clustersPage.selectAllResources'),
        selectedKeys: visibleSelectedResourceKeys,
        selectedLabel: t('clustersPage.selectedResources', { count: selectedResources.length }),
        selectRowLabel: item => t('clustersPage.selectResource', { name: clusterResourceDisplayName(item) }),
        onSelectionChange,
      }}
    />
  )
}

function TruncatedResourceText({ className = 'max-w-56', value }: { className?: string, value: string }) {
  const content = value || '-'
  return (
    <CopyableHoverText
      className={className}
      display={content}
      value={content === '-' ? undefined : content}
    />
  )
}

function clusterResourceDisplayName(item: ClusterResource) {
  if (item.namespace?.trim())
    return `${item.namespace}/${item.name}`
  return item.name
}

function clusterResourceName(item: ClusterResource, includeNamespace: boolean) {
  if (includeNamespace)
    return clusterResourceDisplayName(item)
  return item.name
}

export function ClusterResourceEventsList({ events, loading }: { events: ClusterResourceEvent[], loading: boolean }) {
  const { t } = useTranslation()
  if (loading) {
    return (
      <EmptyState
        title={t('common.loading')}
        description={t('clustersPage.resourceEventsLoading')}
      />
    )
  }
  if (events.length === 0) {
    return (
      <EmptyState
        title={t('clustersPage.resourceEventsEmptyTitle')}
        description={t('clustersPage.resourceEventsEmptyDescription')}
      />
    )
  }
  return (
    <div className="grid gap-3">
      {events.map(event => (
        <div key={event.id} className="rounded-md border border-border bg-surface-subtle p-3">
          <div className="flex flex-wrap items-center gap-2">
            <StatusValueBadge value={event.type || 'normal'} />
            <span className="font-medium text-foreground">{event.reason || t('common.none')}</span>
            <span className="text-xs text-muted-foreground">{formatSmartDateTime(event.lastSeen, t)}</span>
            {event.count > 1 && <span className="text-xs text-muted-foreground">{t('clustersPage.resourceEventCount', { count: event.count })}</span>}
          </div>
          <p className="mt-2 break-words text-sm text-foreground">{event.message || '-'}</p>
          <div className="mt-2 text-xs text-muted-foreground">
            {event.source || t('common.none')}
          </div>
        </div>
      ))}
    </div>
  )
}

function clusterResourceOwner(item: ClusterResource) {
  const project = item.projectName?.trim() || item.projectId?.trim()
  const application = item.applicationName?.trim() || item.applicationId?.trim()
  const deploymentTarget = item.deploymentTargetName?.trim() || item.deploymentTargetId?.trim()
  return [project, application, deploymentTarget].filter(Boolean).join(' / ') || '-'
}

function normalizeClusterResourceStatus(status: string) {
  const value = status.toLowerCase()
  if (value === 'running' || value === 'ready' || value === 'active' || value === 'bound')
    return 'ready'
  if (value === 'failed' || value === 'pending')
    return value
  return status || 'unknown'
}
