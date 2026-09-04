import type { ReactNode } from 'react'
import type { DeploymentRuntimeStatus, InternalServiceEndpointValue } from './application-deployment-runtime-utils'
import type { BuildRun, DeploymentTarget, GatewayRoute, Release } from '@/api'
import { Download, Eye, MoreHorizontal, Package, PanelRightOpen, Pencil, RefreshCw, RotateCcw, Terminal, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { CopyableHoverText } from '@/components/common/copyable-hover-text'
import { DataList } from '@/components/common/data-list'
import { EmptyState } from '@/components/common/empty-state'
import { ResourceDeletionStatus } from '@/components/common/resource-deletion-status'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { deploymentTargetCanRelease } from '@/pages/applications/application-config-utils'
import { shortImageRef } from '@/pages/applications/deployments/application-deployments-panel-utils'
import { DeploymentRuntimeSpecBadge, DeploymentRuntimeStatusBadge } from './application-deployment-runtime'
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
  onOpenTargetDialog,
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
  onOpenTargetDialog: (target: DeploymentTarget) => void
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
            { key: 'name', header: t('common.name'), width: 'primary', render: item => <DeploymentTargetSummary item={item} /> },
            { key: 'domain', header: t('deploymentsPage.serviceDomain'), width: 'secondary', render: item => <DeploymentServiceDomain endpoint={item.internalEndpoint} /> },
            { key: 'stage', header: t('deploymentsPage.stage'), width: 'compact', render: item => t(`deploymentsPage.stageLabels.${item.target.stage}`, { defaultValue: item.target.stage }) },
            { key: 'runtimeSize', header: t('deploymentsPage.runtimeEnvironment'), width: 'secondary', render: item => <DeploymentRuntimeSpecBadge target={item.target} /> },
            { key: 'runtimeStatus', header: t('deploymentsPage.runtimeStatus'), width: 'status', render: item => <DeploymentRuntimeStatusBadge availableReplicas={item.target.availableReplicas} deployed={Boolean(item.release)} desiredReplicas={item.target.desiredReplicas} readyReplicas={item.target.readyReplicas} status={item.runtimeStatus} /> },
            { key: 'status', header: t('deploymentsPage.releaseStatus'), width: 'status', render: item => <ReleaseStatusSummary release={item.release} /> },
            { key: 'image', header: t('deploymentsPage.imageSummary'), width: 'normal', render: item => <DeploymentImageSummary release={item.release} /> },
            {
              key: 'actions',
              header: t('common.actions'),
              cellClassName: 'bg-card',
              className: 'border-l border-border text-right',
              headerClassName: 'z-20 bg-muted/95',
              sticky: 'right',
              width: 'actions',
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
                  onOpenTargetDialog={onOpenTargetDialog}
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
                    onOpenTargetDialog={onOpenTargetDialog}
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
  onOpenTargetDialog,
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
  onOpenTargetDialog: (target: DeploymentTarget) => void
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
  return (
    <div className="flex justify-end">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button aria-label={t('common.actions')} size="icon" variant="ghost">
            <MoreHorizontal className="size-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={onViewDetails}>
            <PanelRightOpen className="size-4" />
            {t('deploymentsPage.deploymentDetails')}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem disabled={deleting || !deploymentTargetCanRelease(item.target, deployableBuildRuns) || createReleasePending} onSelect={() => onOpenReleaseDialog(item.target.id)}>
            <Package className="size-4" />
            {item.release ? t('deploymentsPage.createRelease') : t('deploymentsPage.deployToEnvironment')}
          </DropdownMenuItem>
          <DropdownMenuItem disabled={deleting} onSelect={() => onOpenTargetDialog(item.target)}>
            <Pencil className="size-4" />
            {t('common.edit')}
          </DropdownMenuItem>
          <DropdownMenuItem disabled={exportPending} onSelect={() => onExportTarget(item.target)}>
            <Download className="size-4" />
            {t('deploymentsPage.bundleExport.action')}
          </DropdownMenuItem>
          {item.release && (
            <DropdownMenuItem onSelect={() => item.release && onViewLogs(item.release)}>
              <Eye className="size-4" />
              {t('deploymentsPage.viewLogs')}
            </DropdownMenuItem>
          )}
          {item.release && (
            <DropdownMenuItem
              disabled={!item.webConsoleEnabled || (item.release.status !== 'succeeded' && item.release.status !== 'running')}
              title={!item.webConsoleEnabled ? t('deploymentsPage.webConsoleDisabledHint') : undefined}
              onSelect={() => item.release && onOpenConsole(item.release)}
            >
              <Terminal className="size-4" />
              {t('deploymentsPage.webConsole')}
            </DropdownMenuItem>
          )}
          {item.release && (
            <DropdownMenuItem disabled={item.release.status !== 'succeeded' || rollbackPending} onSelect={() => item.release && onRollback(item.release.id)}>
              <RotateCcw className="size-4" />
              {t('deploymentsPage.rollback')}
            </DropdownMenuItem>
          )}
          <DropdownMenuItem disabled={deleting || !item.release || restartPending} onSelect={() => onRestart(item.target)}>
            <RefreshCw className="size-4" />
            {t('deploymentsPage.restart')}
          </DropdownMenuItem>
          <DropdownMenuItem disabled={deleting || !item.release || pullLatestPending} onSelect={() => onPullLatestImageDeploy(item.target)}>
            <Package className="size-4" />
            {t('deploymentsPage.pullLatestImageDeploy')}
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

function DeploymentTargetSummary({ item }: { item: DeploymentTargetRow }) {
  const { target } = item
  return (
    <div className="grid max-w-80 min-w-0 gap-2">
      <span className="block truncate" title={target.name}>{target.name}</span>
      <ResourceDeletionStatus className="mt-1" message={target.deleteMessage} status={target.deleteStatus} />
    </div>
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

function ReleaseStatusSummary({ release }: { release?: Release }) {
  const { t } = useTranslation()
  const message = release?.message?.trim()
  const badge = release
    ? <StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={release.status} />
    : <StatusBadge tone="warning">{t('deploymentsPage.notDeployed')}</StatusBadge>

  if (!message)
    return badge

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex max-w-full" tabIndex={0}>
          {badge}
        </span>
      </TooltipTrigger>
      <TooltipContent className="max-w-96 whitespace-pre-wrap break-words leading-5">
        {message}
      </TooltipContent>
    </Tooltip>
  )
}

function DeploymentImageSummary({ release }: { release?: Release }) {
  if (!release)
    return <span className="text-sm text-muted-foreground">-</span>

  return (
    <CopyableHoverText
      className="max-w-60 rounded bg-background px-2 py-1 font-mono text-xs"
      display={shortImageRef(release.imageRef)}
      value={release.imageRef}
    />
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
  onOpenTargetDialog,
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
  onOpenTargetDialog: (target: DeploymentTarget) => void
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
    <article className="grid gap-3 rounded-lg border border-border bg-card p-4 shadow-sm">
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="truncate text-sm font-semibold">{item.target.name}</h3>
          <div className="mt-1 flex flex-wrap items-center gap-2">
            <span className="text-xs text-muted-foreground">
              {t(`deploymentsPage.stageLabels.${item.target.stage}`, { defaultValue: item.target.stage })}
            </span>
            <DeploymentRuntimeSpecBadge target={item.target} />
          </div>
          {item.internalEndpoint && (
            <p className="mt-2 truncate font-mono text-xs text-muted-foreground" title={item.internalEndpoint.serviceName}>
              {item.internalEndpoint.serviceName}
            </p>
          )}
        </div>
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
          onOpenTargetDialog={onOpenTargetDialog}
          onPullLatestImageDeploy={onPullLatestImageDeploy}
          onRestart={onRestart}
          onRollback={onRollback}
          onViewDetails={onViewDetails}
          onViewLogs={onViewLogs}
        />
      </div>
      <div className="grid grid-cols-2 gap-3 text-xs">
        <LabeledValue label={t('deploymentsPage.runtimeStatus')}>
          <DeploymentRuntimeStatusBadge availableReplicas={item.target.availableReplicas} deployed={Boolean(item.release)} desiredReplicas={item.target.desiredReplicas} readyReplicas={item.target.readyReplicas} status={item.runtimeStatus} />
        </LabeledValue>
        <LabeledValue label={t('deploymentsPage.releaseStatus')}>
          <ReleaseStatusSummary release={item.release} />
        </LabeledValue>
      </div>
      <LabeledValue label={t('deploymentsPage.imageSummary')}>
        <DeploymentImageSummary release={item.release} />
      </LabeledValue>
    </article>
  )
}

function deploymentBundleFilenamePart(value: string) {
  return value.toLowerCase().replace(/[^a-z0-9_-]+/g, '') || 'deployment'
}

function LabeledValue({ children, label }: { children: ReactNode, label: string }) {
  return (
    <div className="grid min-w-0 gap-1">
      <span className="text-xs font-medium uppercase text-muted-foreground">{label}</span>
      <div className="min-w-0 text-sm text-foreground">{children}</div>
    </div>
  )
}
