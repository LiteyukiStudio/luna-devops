import type { ReactNode } from 'react'
import type { DeploymentRuntimeStatus, InternalServiceEndpointValue } from './application-deployment-runtime-utils'
import type { BuildRun, DeploymentTarget, GatewayRoute, Release } from '@/api'
import { Download, Eye, MoreHorizontal, Package, PanelRightOpen, Pencil, RefreshCw, Rocket, RotateCcw, Terminal, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { CopyableHoverText } from '@/components/common/copyable-hover-text'
import { DataList } from '@/components/common/data-list'
import { EmptyState } from '@/components/common/empty-state'
import { ResourceDeletionStatus } from '@/components/common/resource-deletion-status'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Surface } from '@/components/common/surface'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { shortImageRef } from '@/lib/image-ref'
import { deploymentTargetCanRelease } from '@/pages/applications/application-config-utils'
import { AccessUrlLink } from '@/pages/applications/gateway/access-url-link'
import { gatewayRouteAccessUrl, primaryGatewayRoute } from '@/pages/applications/gateway/gateway-route-url'
import { DeploymentRuntimeSpec, DeploymentRuntimeStatusDot } from './application-deployment-runtime'
import { DeploymentTargetDetailSheet } from './deployment-target-detail-sheet'

export interface DeploymentTargetRow {
  internalEndpoint?: InternalServiceEndpointValue
  release?: Release
  routes: GatewayRoute[]
  runtimeStatus: DeploymentRuntimeStatus
  target: DeploymentTarget
  webConsoleEnabled: boolean
}

export function ApplicationDeploymentTargetsList({
  applicationId,
  createReleasePending,
  deletePending,
  deployableBuildRuns,
  items,
  onDeleteTarget,
  onOpenConsole,
  onOpenReleaseDialog,
  onEditTarget,
  onPullLatestImageDeploy,
  onRestart,
  onRollback,
  onViewLogs,
  projectId,
  pullLatestPending,
  restartPending,
  rollbackPending,
}: {
  applicationId: string
  createReleasePending: boolean
  deletePending: boolean
  deployableBuildRuns: BuildRun[]
  items: DeploymentTargetRow[]
  onDeleteTarget: (target: DeploymentTarget) => void
  onOpenConsole: (release: Release) => void
  onOpenReleaseDialog: (deploymentTargetId: string) => void
  onEditTarget: (target: DeploymentTarget) => void
  onPullLatestImageDeploy: (target: DeploymentTarget) => void
  onRestart: (target: DeploymentTarget) => void
  onRollback: (releaseId: string) => void
  onViewLogs: (release: Release) => void
  projectId: string
  pullLatestPending: boolean
  restartPending: boolean
  rollbackPending: boolean
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

  return (
    <div className="w-full min-w-0 max-w-full @container/deployment-targets" data-slot="deployment-targets-list">
      <div className="hidden min-w-0 max-w-full @[68rem]/deployment-targets:block" data-slot="deployment-targets-table">
        <DataList
          columns={[
            { key: 'name', header: t('common.name'), width: 'primary', render: item => <DeploymentTargetSummary item={item} onViewDetails={() => setDetailTargetId(item.target.id)} /> },
            { key: 'runtimeStatus', header: t('deploymentsPage.runtimeStatus'), width: 'status', render: item => <DeploymentRuntimeStatusDot availableReplicas={item.target.availableReplicas} deployed={Boolean(item.release)} desiredReplicas={item.target.desiredReplicas} readyReplicas={item.target.readyReplicas} status={item.runtimeStatus} /> },
            { key: 'accessUrl', header: t('deploymentsPage.accessAddress'), width: 'normal', render: item => <DeploymentAccessAddress item={item} /> },
            { key: 'domain', header: t('deploymentsPage.internalAddress'), width: 'secondary', render: item => <DeploymentServiceDomain endpoint={item.internalEndpoint} /> },
            { key: 'image', header: t('deploymentsPage.currentVersion'), width: 'normal', render: item => <DeploymentVersionSummary release={item.release} /> },
            { key: 'runtimeSize', header: t('deploymentsPage.runtimeEnvironment'), width: 'compact', render: item => <DeploymentRuntimeSpec target={item.target} /> },
            {
              key: 'actions',
              header: t('common.actions'),
              sticky: 'right',
              width: 'actions',
              mobileActions: 'inline',
              render: item => (
                <DeploymentTargetActions
                  createReleasePending={createReleasePending}
                  deletePending={deletePending}
                  deployableBuildRuns={deployableBuildRuns}
                  item={item}
                  exportPending={exportPendingId === item.target.id}
                  pullLatestPending={pullLatestPending}
                  restartPending={restartPending}
                  rollbackPending={rollbackPending}
                  onDeleteTarget={onDeleteTarget}
                  onExportTarget={exportTarget}
                  onOpenConsole={onOpenConsole}
                  onOpenReleaseDialog={onOpenReleaseDialog}
                  onEditTarget={onEditTarget}
                  onPullLatestImageDeploy={onPullLatestImageDeploy}
                  onRestart={onRestart}
                  onRollback={onRollback}
                  onViewDetails={() => setDetailTargetId(item.target.id)}
                  onViewLogs={onViewLogs}
                />
              ),
            },
          ]}
          emptyDescription={t('deploymentsPage.emptyDeploymentsDescription')}
          emptyTitle={t('deploymentsPage.emptyDeployments')}
          items={items}
          rowKey={item => item.target.id}
        />
      </div>
      <div className="min-w-0 max-w-full @[68rem]/deployment-targets:hidden" data-slot="deployment-targets-cards">
        {items.length === 0
          ? <EmptyState description={t('deploymentsPage.emptyDeploymentsDescription')} title={t('deploymentsPage.emptyDeployments')} variant="plain" />
          : (
              <div className="grid gap-3">
                {items.map(item => (
                  <MobileDeploymentTargetCard
                    key={item.target.id}
                    createReleasePending={createReleasePending}
                    deletePending={deletePending}
                    deployableBuildRuns={deployableBuildRuns}
                    item={item}
                    exportPending={exportPendingId === item.target.id}
                    pullLatestPending={pullLatestPending}
                    restartPending={restartPending}
                    rollbackPending={rollbackPending}
                    onDeleteTarget={onDeleteTarget}
                    onExportTarget={exportTarget}
                    onOpenConsole={onOpenConsole}
                    onOpenReleaseDialog={onOpenReleaseDialog}
                    onEditTarget={onEditTarget}
                    onPullLatestImageDeploy={onPullLatestImageDeploy}
                    onRestart={onRestart}
                    onRollback={onRollback}
                    onViewDetails={() => setDetailTargetId(item.target.id)}
                    onViewLogs={onViewLogs}
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
  createReleasePending,
  deletePending,
  deployableBuildRuns,
  item,
  exportPending,
  onDeleteTarget,
  onExportTarget,
  onOpenConsole,
  onOpenReleaseDialog,
  onEditTarget,
  onPullLatestImageDeploy,
  onRestart,
  onRollback,
  onViewDetails,
  onViewLogs,
  pullLatestPending,
  restartPending,
  rollbackPending,
}: {
  createReleasePending: boolean
  deletePending: boolean
  deployableBuildRuns: BuildRun[]
  item: DeploymentTargetRow
  exportPending: boolean
  onDeleteTarget: (target: DeploymentTarget) => void
  onExportTarget: (target: DeploymentTarget) => void
  onOpenConsole: (release: Release) => void
  onOpenReleaseDialog: (deploymentTargetId: string) => void
  onEditTarget: (target: DeploymentTarget) => void
  onPullLatestImageDeploy: (target: DeploymentTarget) => void
  onRestart: (target: DeploymentTarget) => void
  onRollback: (releaseId: string) => void
  onViewDetails: () => void
  onViewLogs: (release: Release) => void
  pullLatestPending: boolean
  restartPending: boolean
  rollbackPending: boolean
}) {
  const { t } = useTranslation()
  const deleting = item.target.deleteStatus === 'deleting'
  const canRelease = !deleting && deploymentTargetCanRelease(item.target, deployableBuildRuns) && !createReleasePending
  const release = item.release
  return (
    <div className="flex items-center justify-end gap-1">
      <Button disabled={!canRelease} size="sm" variant="outline" onClick={() => onOpenReleaseDialog(item.target.id)}>
        <Rocket className="size-4" />
        {release ? t('deploymentsPage.createRelease') : t('deploymentsPage.deployToEnvironment')}
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button aria-label={t('common.actions')} size="icon" variant="ghost">
            <MoreHorizontal className="size-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {release && (
            <DropdownMenuItem onSelect={() => onViewLogs(release)}>
              <Eye className="size-4" />
              {t('deploymentsPage.viewLogs')}
            </DropdownMenuItem>
          )}
          {release && (
            <DropdownMenuItem
              disabled={!item.webConsoleEnabled || (release.status !== 'succeeded' && release.status !== 'running')}
              title={!item.webConsoleEnabled ? t('deploymentsPage.webConsoleDisabledHint') : undefined}
              onSelect={() => onOpenConsole(release)}
            >
              <Terminal className="size-4" />
              {t('deploymentsPage.webConsole')}
            </DropdownMenuItem>
          )}
          <DropdownMenuItem disabled={deleting || !release || restartPending} onSelect={() => onRestart(item.target)}>
            <RefreshCw className="size-4" />
            {t('deploymentsPage.restart')}
          </DropdownMenuItem>
          {release && (
            <DropdownMenuItem disabled={release.status !== 'succeeded' || rollbackPending} onSelect={() => onRollback(release.id)}>
              <RotateCcw className="size-4" />
              {t('deploymentsPage.rollback')}
            </DropdownMenuItem>
          )}
          <DropdownMenuItem disabled={deleting || !release || pullLatestPending} onSelect={() => onPullLatestImageDeploy(item.target)}>
            <Package className="size-4" />
            {t('deploymentsPage.pullLatestImageDeploy')}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem disabled={deleting} onSelect={() => onEditTarget(item.target)}>
            <Pencil className="size-4" />
            {t('common.edit')}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={onViewDetails}>
            <PanelRightOpen className="size-4" />
            {t('deploymentsPage.deploymentDetails')}
          </DropdownMenuItem>
          <DropdownMenuItem disabled={exportPending} onSelect={() => onExportTarget(item.target)}>
            <Download className="size-4" />
            {t('deploymentsPage.bundleExport.action')}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem disabled={deletePending || deleting} variant="destructive" onSelect={() => onDeleteTarget(item.target)}>
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
    <div className="grid max-w-80 min-w-0 gap-0.5">
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
      <ResourceDeletionStatus className="mt-1" message={target.deleteMessage} status={target.deleteStatus} />
    </div>
  )
}

function DeploymentAccessAddress({ item }: { item: DeploymentTargetRow }) {
  const primary = primaryGatewayRoute(item.routes)
  if (!primary)
    return <span className="text-sm text-muted-foreground">-</span>
  const extraCount = item.routes.filter(route => route.id !== primary.id && route.enabled && gatewayRouteAccessUrl(route)).length
  return (
    <span className="inline-flex min-w-0 max-w-64 items-center gap-1.5">
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
      className="max-w-52 font-mono text-xs"
      value={endpoint.serviceName}
    />
  )
}

function DeploymentVersionSummary({ release }: { release?: Release }) {
  const { t } = useTranslation()
  if (!release)
    return <span className="text-sm text-muted-foreground">{t('deploymentsPage.notDeployed')}</span>

  const message = release.message?.trim()
  const status = release.status === 'succeeded'
    ? null
    : <StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={release.status} />
  return (
    <div className="flex min-w-0 max-w-72 items-center gap-2">
      <CopyableHoverText
        className="min-w-0 font-mono text-xs"
        display={shortImageRef(release.imageRef)}
        value={release.imageRef}
      />
      {status && (message
        ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="inline-flex shrink-0 rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" tabIndex={0}>
                  {status}
                </span>
              </TooltipTrigger>
              <TooltipContent className="max-w-96 whitespace-pre-wrap break-words leading-5">
                {message}
              </TooltipContent>
            </Tooltip>
          )
        : <span className="shrink-0">{status}</span>)}
    </div>
  )
}

function MobileDeploymentTargetCard({
  createReleasePending,
  deletePending,
  deployableBuildRuns,
  item,
  exportPending,
  onDeleteTarget,
  onExportTarget,
  onOpenConsole,
  onOpenReleaseDialog,
  onEditTarget,
  onPullLatestImageDeploy,
  onRestart,
  onRollback,
  onViewDetails,
  onViewLogs,
  pullLatestPending,
  restartPending,
  rollbackPending,
}: {
  createReleasePending: boolean
  deletePending: boolean
  deployableBuildRuns: BuildRun[]
  item: DeploymentTargetRow
  exportPending: boolean
  onDeleteTarget: (target: DeploymentTarget) => void
  onExportTarget: (target: DeploymentTarget) => void
  onOpenConsole: (release: Release) => void
  onOpenReleaseDialog: (deploymentTargetId: string) => void
  onEditTarget: (target: DeploymentTarget) => void
  onPullLatestImageDeploy: (target: DeploymentTarget) => void
  onRestart: (target: DeploymentTarget) => void
  onRollback: (releaseId: string) => void
  onViewDetails: () => void
  onViewLogs: (release: Release) => void
  pullLatestPending: boolean
  restartPending: boolean
  rollbackPending: boolean
}) {
  const { t } = useTranslation()

  return (
    <Surface className="grid gap-3 p-4" data-slot="deployment-target-card" variant="bordered">
      <div className="flex min-w-0 items-start justify-between gap-3">
        <DeploymentTargetSummary item={item} onViewDetails={onViewDetails} />
        <DeploymentRuntimeStatusDot availableReplicas={item.target.availableReplicas} deployed={Boolean(item.release)} desiredReplicas={item.target.desiredReplicas} readyReplicas={item.target.readyReplicas} status={item.runtimeStatus} />
      </div>
      <dl className="grid grid-cols-1 gap-3 text-sm sm:grid-cols-2">
        <LabeledValue label={t('deploymentsPage.accessAddress')}>
          <DeploymentAccessAddress item={item} />
        </LabeledValue>
        <LabeledValue label={t('deploymentsPage.currentVersion')}>
          <DeploymentVersionSummary release={item.release} />
        </LabeledValue>
        <LabeledValue label={t('deploymentsPage.internalAddress')}>
          <DeploymentServiceDomain endpoint={item.internalEndpoint} />
        </LabeledValue>
        <LabeledValue label={t('deploymentsPage.runtimeEnvironment')}>
          <DeploymentRuntimeSpec target={item.target} />
        </LabeledValue>
      </dl>
      <DeploymentTargetActions
        createReleasePending={createReleasePending}
        deletePending={deletePending}
        deployableBuildRuns={deployableBuildRuns}
        item={item}
        exportPending={exportPending}
        pullLatestPending={pullLatestPending}
        restartPending={restartPending}
        rollbackPending={rollbackPending}
        onDeleteTarget={onDeleteTarget}
        onExportTarget={onExportTarget}
        onOpenConsole={onOpenConsole}
        onOpenReleaseDialog={onOpenReleaseDialog}
        onEditTarget={onEditTarget}
        onPullLatestImageDeploy={onPullLatestImageDeploy}
        onRestart={onRestart}
        onRollback={onRollback}
        onViewDetails={onViewDetails}
        onViewLogs={onViewLogs}
      />
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
