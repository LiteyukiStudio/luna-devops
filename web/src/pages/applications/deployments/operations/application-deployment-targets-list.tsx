import type { ReactNode } from 'react'
import type { DeploymentRuntimeStatus, InternalServiceEndpointValue } from './application-deployment-runtime-utils'
import type { BuildRun, DeploymentTarget, GatewayRoute, Release } from '@/api'
import { CircleStop, Download, Eye, MoreHorizontal, Package, PanelRightOpen, Pencil, Plus, RefreshCw, Rocket, RotateCcw, Terminal, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { CopyableHoverText } from '@/components/common/copyable-hover-text'
import { DataList } from '@/components/common/data-list'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { DataListSkeleton } from '@/components/common/loading-states'
import { ResourceDeletionStatus } from '@/components/common/resource-deletion-status'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Surface } from '@/components/common/surface'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { shortImageRef } from '@/lib/image-ref'
import { deploymentTargetCanRelease } from '@/pages/applications/application-config-utils'
import { AccessUrlLink } from '@/pages/applications/gateway/access-url-link'
import { gatewayRouteAccessUrl, primaryGatewayRoute } from '@/pages/applications/gateway/gateway-route-url'
import { DeploymentRuntimeSpec, DeploymentRuntimeStatusIndicator } from './application-deployment-runtime'
import { deploymentTargetDeleteFailureMessage } from './deployment-target-delete-failure'
import { DeploymentTargetDetailSheet } from './deployment-target-detail-sheet'

export interface DeploymentTargetRow {
  /** 后端统一判定：是否已有成功发布（停用配置仍按其观察状态展示）。 */
  deployed: boolean
  internalEndpoint?: InternalServiceEndpointValue
  release?: Release
  /** 回滚将回到的上一次成功发布；没有时不可回滚。 */
  rollbackTo?: Release
  routes: GatewayRoute[]
  runtimeStatus: DeploymentRuntimeStatus
  target: DeploymentTarget
  webConsoleEnabled: boolean
}

/** 列表数据（部署配置与发布记录）的加载状态；loading、error、empty 互斥渲染。 */
export interface DeploymentListState {
  loading: boolean
  error?: {
    onRetry: () => void
    retrying: boolean
  }
}

/** 正在执行运行操作的部署配置 ID，pending 只影响发起操作的那一行。 */
export interface DeploymentOperationPending {
  pullLatest?: string
  restart?: string
  rollback?: string
}

interface DeploymentTargetRowActions {
  cancelReleasePending: boolean
  createReleasePending: boolean
  deletePending: boolean
  deployableBuildRuns: BuildRun[]
  operationPending: DeploymentOperationPending
  onCancelRelease: (release: Release) => void
  onDeleteTarget: (target: DeploymentTarget) => void
  onEditTarget: (target: DeploymentTarget) => void
  onExportTarget: (target: DeploymentTarget) => void
  onOpenConsole: (release: Release) => void
  onOpenReleaseDialog: (deploymentTargetId: string) => void
  onPullLatestImageDeploy: (target: DeploymentTarget) => void
  onRestart: (target: DeploymentTarget) => void
  onRollback: (item: DeploymentTargetRow) => void
  onViewLogs: (release: Release) => void
}

export function ApplicationDeploymentTargetsList({
  applicationId,
  cancelReleasePending,
  createReleasePending,
  deletePending,
  deployableBuildRuns,
  items,
  listState,
  onCreateTarget,
  onCancelRelease,
  onDeleteTarget,
  onOpenConsole,
  onOpenReleaseDialog,
  onEditTarget,
  onPullLatestImageDeploy,
  onRestart,
  onRollback,
  onViewLogs,
  operationPending,
  projectId,
}: Omit<DeploymentTargetRowActions, 'onExportTarget'> & {
  applicationId: string
  items: DeploymentTargetRow[]
  listState: DeploymentListState
  onCreateTarget: () => void
  projectId: string
}) {
  const { t } = useTranslation()
  const [exportPendingId, setExportPendingId] = useState('')
  const [detailTargetId, setDetailTargetId] = useState('')
  const detailItem = items.find(item => item.target.id === detailTargetId)

  const exportTarget = async (target: DeploymentTarget) => {
    setExportPendingId(target.id)
    try {
      const bundle = await api.exportDeploymentTargetBundle(projectId, applicationId, target.id)
      const url = URL.createObjectURL(new Blob([`${JSON.stringify(bundle, null, 2)}\n`], { type: 'application/json;charset=utf-8' }))
      const link = document.createElement('a')
      link.href = url
      link.download = `luna-deployment-${deploymentBundleFilenamePart(target.stage)}.json`
      link.hidden = true
      document.body.append(link)
      link.click()
      link.remove()
      URL.revokeObjectURL(url)
      toast.success(t('deploymentsPage.bundleExport.exported'))
    }
    catch (error) {
      toast.error(error instanceof Error ? error.message : t('deploymentsPage.bundleExport.failed'))
    }
    finally {
      setExportPendingId('')
    }
  }

  const rowActions: DeploymentTargetRowActions = {
    cancelReleasePending,
    createReleasePending,
    deletePending,
    deployableBuildRuns,
    operationPending,
    onCancelRelease,
    onDeleteTarget,
    onEditTarget,
    onExportTarget: exportTarget,
    onOpenConsole,
    onOpenReleaseDialog,
    onPullLatestImageDeploy,
    onRestart,
    onRollback,
    onViewLogs,
  }
  const emptyActions = (
    <Button type="button" variant="outline" onClick={onCreateTarget}>
      <Plus className="size-4" />
      {t('deploymentsPage.createDeploymentTarget')}
    </Button>
  )
  const listError = listState.error
    ? { ...listState.error, description: t('deploymentsPage.listLoadFailedDescription'), title: t('deploymentsPage.listLoadFailedTitle') }
    : undefined

  return (
    <div className="w-full min-w-0 max-w-full @container/deployment-targets" data-slot="deployment-targets-list">
      <div className="hidden min-w-0 max-w-full @[68rem]/deployment-targets:block" data-slot="deployment-targets-table">
        {/* 列宽上限按 68rem 容器（1440 宽视口）预算；内部地址与规格在 96rem 以上才展开，详情抽屉始终可查看。 */}
        <DataList
          columns={[
            { key: 'name', header: t('common.name'), width: 'primary', maxWidth: 176, minWidth: 144, render: item => <DeploymentTargetSummary item={item} onViewDetails={() => setDetailTargetId(item.target.id)} /> },
            { key: 'runtimeStatus', header: t('deploymentsPage.runtimeStatus'), width: 'status', maxWidth: 152, render: item => <DeploymentRuntimeStatusIndicator availableReplicas={item.target.availableReplicas} deployed={item.deployed} desiredReplicas={item.target.desiredReplicas} readyReplicas={item.target.readyReplicas} status={item.runtimeStatus} /> },
            { key: 'accessUrl', header: t('deploymentsPage.accessAddress'), width: 'normal', maxWidth: 176, minWidth: 128, render: item => <DeploymentAccessAddress item={item} /> },
            { key: 'domain', className: 'hidden @[96rem]/deployment-targets:table-cell', header: t('deploymentsPage.internalAddress'), width: 'secondary', maxWidth: 176, render: item => <DeploymentServiceDomain endpoint={item.internalEndpoint} /> },
            { key: 'image', header: t('deploymentsPage.currentVersion'), width: 'normal', maxWidth: 240, minWidth: 144, render: item => <DeploymentVersionSummary release={item.release} onViewLogs={onViewLogs} /> },
            { key: 'runtimeSize', className: 'hidden @[96rem]/deployment-targets:table-cell', header: t('deploymentsPage.runtimeEnvironment'), width: 'compact', render: item => <DeploymentRuntimeSpec target={item.target} /> },
            {
              key: 'actions',
              header: t('common.actions'),
              sticky: 'right',
              width: 'actions',
              mobileActions: 'inline',
              render: item => (
                <DeploymentTargetActions
                  actions={rowActions}
                  exportPending={exportPendingId === item.target.id}
                  item={item}
                  onViewDetails={() => setDetailTargetId(item.target.id)}
                />
              ),
            },
          ]}
          emptyActions={emptyActions}
          emptyDescription={t('deploymentsPage.emptyDeploymentsDescription')}
          emptyTitle={t('deploymentsPage.emptyDeployments')}
          error={listError}
          items={items}
          loading={listState.loading}
          rowKey={item => item.target.id}
        />
      </div>
      <div className="min-w-0 max-w-full @[68rem]/deployment-targets:hidden" data-slot="deployment-targets-cards">
        {listState.loading
          ? <DataListSkeleton columns={2} rows={3} />
          : listError
            ? <ErrorState description={listError.description} retrying={listError.retrying} title={listError.title} onRetry={listError.onRetry} />
            : items.length === 0
              ? <EmptyState actions={emptyActions} description={t('deploymentsPage.emptyDeploymentsDescription')} title={t('deploymentsPage.emptyDeployments')} variant="plain" />
              : (
                  <div className="grid gap-3">
                    {items.map(item => (
                      <MobileDeploymentTargetCard
                        key={item.target.id}
                        actions={rowActions}
                        exportPending={exportPendingId === item.target.id}
                        item={item}
                        onViewDetails={() => setDetailTargetId(item.target.id)}
                      />
                    ))}
                  </div>
                )}
      </div>
      <DeploymentTargetDetailSheet
        applicationId={applicationId}
        item={detailItem}
        open={Boolean(detailItem)}
        projectId={projectId}
        onOpenChange={(open) => {
          if (!open)
            setDetailTargetId('')
        }}
      />
    </div>
  )
}

function DeploymentTargetActions({
  actions,
  exportPending,
  item,
  onViewDetails,
}: {
  actions: DeploymentTargetRowActions
  exportPending: boolean
  item: DeploymentTargetRow
  onViewDetails: () => void
}) {
  const { t } = useTranslation()
  const { target } = item
  const deleting = target.deleteStatus === 'deleting'
  const canRelease = !deleting && deploymentTargetCanRelease(target, actions.deployableBuildRuns) && !actions.createReleasePending
  const release = item.release
  const pending = actions.operationPending
  return (
    <div className="flex items-center justify-end gap-1">
      <Button disabled={!canRelease} size="sm" variant="outline" onClick={() => actions.onOpenReleaseDialog(target.id)}>
        <Rocket className="size-4" />
        {release ? t('deploymentsPage.createRelease') : t('deploymentsPage.deployToEnvironment')}
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button aria-label={t('deploymentsPage.moreActionsAria', { name: target.name })} size="icon" variant="ghost">
            <MoreHorizontal className="size-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {release && (
            <DropdownMenuItem onSelect={() => actions.onViewLogs(release)}>
              <Eye className="size-4" />
              {t('deploymentsPage.viewLogs')}
            </DropdownMenuItem>
          )}
          {release && (
            <DropdownMenuItem
              disabled={!item.webConsoleEnabled || (release.status !== 'succeeded' && release.status !== 'running')}
              title={!item.webConsoleEnabled ? t('deploymentsPage.webConsoleDisabledHint') : undefined}
              onSelect={() => actions.onOpenConsole(release)}
            >
              <Terminal className="size-4" />
              {t('deploymentsPage.webConsole')}
            </DropdownMenuItem>
          )}
          <DropdownMenuItem disabled={deleting || !release || pending.restart === target.id} onSelect={() => actions.onRestart(target)}>
            <RefreshCw className="size-4" />
            {t('deploymentsPage.restart')}
          </DropdownMenuItem>
          {release && (
            <DropdownMenuItem
              disabled={deleting || release.status !== 'succeeded' || !item.rollbackTo || pending.rollback === target.id}
              title={release.status === 'succeeded' && !item.rollbackTo ? t('deploymentsPage.rollbackUnavailableHint') : undefined}
              onSelect={() => actions.onRollback(item)}
            >
              <RotateCcw className="size-4" />
              {t('deploymentsPage.rollback')}
            </DropdownMenuItem>
          )}
          {release && releaseCancelable(release) && (
            <DropdownMenuItem disabled={actions.cancelReleasePending} variant="destructive" onSelect={() => actions.onCancelRelease(release)}>
              <CircleStop className="size-4" />
              {t('deploymentsPage.cancelRelease')}
            </DropdownMenuItem>
          )}
          <DropdownMenuItem disabled={deleting || !release || pending.pullLatest === target.id} onSelect={() => actions.onPullLatestImageDeploy(target)}>
            <Package className="size-4" />
            {t('deploymentsPage.pullLatestImageDeploy')}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem disabled={deleting} onSelect={() => actions.onEditTarget(target)}>
            <Pencil className="size-4" />
            {t('common.edit')}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={onViewDetails}>
            <PanelRightOpen className="size-4" />
            {t('deploymentsPage.deploymentDetails')}
          </DropdownMenuItem>
          <DropdownMenuItem disabled={exportPending} onSelect={() => actions.onExportTarget(target)}>
            <Download className="size-4" />
            {t('deploymentsPage.bundleExport.action')}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem disabled={actions.deletePending || deleting} variant="destructive" onSelect={() => actions.onDeleteTarget(target)}>
            <Trash2 className="size-4" />
            {t('common.delete')}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}

function DeploymentTargetSummary({ item, onViewDetails }: { item: DeploymentTargetRow, onViewDetails: () => void }) {
  const { t } = useTranslation()
  const { target } = item
  return (
    <div className="grid max-w-full min-w-0 gap-0.5">
      <button
        className="min-w-0 truncate rounded-sm text-left font-medium text-foreground hover:text-primary-text hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        title={target.name}
        type="button"
        onClick={onViewDetails}
      >
        {target.name}
      </button>
      <span className="truncate text-xs text-muted-foreground">
        {t(`deploymentsPage.stageLabels.${target.stage}`, { defaultValue: target.stage })}
        {!target.enabled && ` · ${t('common.disabled')}`}
      </span>
      <ResourceDeletionStatus className="mt-1" message={deploymentTargetDeleteFailureMessage(t, target)} status={target.deleteStatus} />
    </div>
  )
}

function DeploymentAccessAddress({ item }: { item: DeploymentTargetRow }) {
  const primary = primaryGatewayRoute(item.routes)
  if (!primary)
    return <span className="text-sm text-muted-foreground">-</span>
  const extraCount = item.routes.filter(route => route.id !== primary.id && route.enabled && gatewayRouteAccessUrl(route)).length
  return (
    <span className="inline-flex min-w-0 max-w-full items-center gap-1.5">
      <AccessUrlLink url={gatewayRouteAccessUrl(primary)} />
      {extraCount > 0 && <span className="shrink-0 text-xs tabular-nums text-muted-foreground">{`+${extraCount}`}</span>}
    </span>
  )
}

function DeploymentServiceDomain({ endpoint }: { endpoint?: InternalServiceEndpointValue }) {
  if (!endpoint)
    return <span className="text-sm text-muted-foreground">-</span>

  return (
    <CopyableHoverText
      className="max-w-full font-mono text-xs"
      value={endpoint.serviceName}
    />
  )
}

function DeploymentVersionSummary({ onViewLogs, release }: { onViewLogs: (release: Release) => void, release?: Release }) {
  const { t } = useTranslation()
  if (!release)
    return <span className="text-sm text-muted-foreground">{t('deploymentsPage.notDeployed')}</span>

  const failureReason = releaseFailureReason(release, t)
  return (
    <div className="grid min-w-0 max-w-72 gap-0.5">
      <div className="flex min-w-0 items-center gap-2">
        <CopyableHoverText
          className="min-w-0 font-mono text-xs"
          display={shortImageRef(release.imageRef)}
          value={release.imageRef}
        />
        {release.status !== 'succeeded' && (
          <span className="shrink-0"><StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={release.status} /></span>
        )}
      </div>
      {failureReason && (
        <div className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
          <span className="min-w-0 truncate" title={failureReason}>{failureReason}</span>
          <Button className="h-auto shrink-0 px-0 py-0 text-xs" size="sm" variant="link" onClick={() => onViewLogs(release)}>
            {t('deploymentsPage.viewLogs')}
          </Button>
        </div>
      )}
    </div>
  )
}

function releaseCancelable(release: Release) {
  return release.status === 'pending' || release.status === 'running'
}

/** 失败或取消的发布按稳定原因码展示摘要；旧记录没有原因码时提示查看日志。 */
function releaseFailureReason(release: Release, t: ReturnType<typeof useTranslation>['t']) {
  if (release.status !== 'failed' && release.status !== 'canceled')
    return ''
  if (release.failureCode)
    return t(`deploymentsPage.releaseFailureCodes.${release.failureCode}`)
  return t('deploymentsPage.releaseFailureReasonUnknown')
}

function MobileDeploymentTargetCard({
  actions,
  exportPending,
  item,
  onViewDetails,
}: {
  actions: DeploymentTargetRowActions
  exportPending: boolean
  item: DeploymentTargetRow
  onViewDetails: () => void
}) {
  const { t } = useTranslation()

  return (
    <Surface className="grid gap-3 p-4" data-slot="deployment-target-card" variant="bordered">
      <div className="flex min-w-0 items-start justify-between gap-3">
        <DeploymentTargetSummary item={item} onViewDetails={onViewDetails} />
        <DeploymentRuntimeStatusIndicator availableReplicas={item.target.availableReplicas} deployed={item.deployed} desiredReplicas={item.target.desiredReplicas} readyReplicas={item.target.readyReplicas} status={item.runtimeStatus} />
      </div>
      <dl className="grid grid-cols-1 gap-3 text-sm @lg/deployment-targets:grid-cols-2">
        <LabeledValue label={t('deploymentsPage.accessAddress')}>
          <DeploymentAccessAddress item={item} />
        </LabeledValue>
        <LabeledValue label={t('deploymentsPage.currentVersion')}>
          <DeploymentVersionSummary release={item.release} onViewLogs={actions.onViewLogs} />
        </LabeledValue>
        <LabeledValue label={t('deploymentsPage.internalAddress')}>
          <DeploymentServiceDomain endpoint={item.internalEndpoint} />
        </LabeledValue>
        <LabeledValue label={t('deploymentsPage.runtimeEnvironment')}>
          <DeploymentRuntimeSpec target={item.target} />
        </LabeledValue>
      </dl>
      <DeploymentTargetActions actions={actions} exportPending={exportPending} item={item} onViewDetails={onViewDetails} />
    </Surface>
  )
}

function deploymentBundleFilenamePart(value: string) {
  return value.toLowerCase().replace(/[^a-z0-9_-]+/g, '') || 'deployment'
}

function LabeledValue({ children, label }: { children: ReactNode, label: string }) {
  return (
    <div className="grid min-w-0 gap-1">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="min-w-0 text-sm text-foreground">{children}</dd>
    </div>
  )
}
