import type { DeploymentRuntimeStatus, InternalServiceEndpointValue } from './application-deployment-runtime-utils'
import type { BuildRun, DeploymentTarget, Release } from '@/api'
import { Download, Eye, MoreHorizontal, Package, Pencil, RefreshCw, RotateCcw, Terminal, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { deploymentTargetDataExportUrl } from '@/api'
import { CopyableHoverText } from '@/components/common/copyable-hover-text'
import { DataList } from '@/components/common/data-list'
import { HoverText } from '@/components/common/hover-text'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { deploymentTargetCanRelease, formatReleaseTime } from './application-config-utils'
import { DeploymentRuntimeStatusBadge, InternalServiceEndpoint } from './application-deployment-runtime'
import { DeploymentTargetMetricsCell } from './application-deployment-target-metrics-cell'
import { compactReleaseMessage, formatTargetRuntimeSize, shortImageRef } from './application-deployments-panel-utils'

export interface DeploymentTargetRow {
  internalEndpoint?: InternalServiceEndpointValue
  release?: Release
  runtimeStatus: DeploymentRuntimeStatus
  target: DeploymentTarget
}

export function ApplicationDeploymentTargetsList({
  applicationId,
  createReleasePending,
  deletePending,
  deployableBuildRuns,
  items,
  onCopy,
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
  onCopy: (value?: string) => void
  onDeleteTarget: (target: DeploymentTarget) => void
  onOpenConsole: (release: Release) => void
  onOpenReleaseDialog: (environmentId: string, deploymentTargetId: string) => void
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

  return (
    <DataList
      columns={[
        { key: 'name', header: t('common.name'), className: 'w-[1%] whitespace-nowrap px-4 py-3 align-middle', render: item => <DeploymentTargetSummary target={item.target} /> },
        { key: 'deploymentTarget', header: t('buildsPage.buildConfig'), className: 'w-[1%] whitespace-nowrap px-4 py-3 align-middle', render: item => <span className="block max-w-32 truncate" title={item.target.name}>{item.target.name}</span> },
        { key: 'stage', header: t('deploymentsPage.stage'), className: 'w-[1%] whitespace-nowrap px-4 py-3 align-middle', render: item => t(`deploymentsPage.stageLabels.${item.target.stage}`, { defaultValue: item.target.stage }) },
        { key: 'runtimeSize', header: t('deploymentsPage.runtimeEnvironment'), className: 'w-[1%] whitespace-nowrap px-4 py-3 align-middle', render: item => formatTargetRuntimeSize(item.target, t) },
        { key: 'runtimeData', header: t('deploymentsPage.runtimeData'), className: 'w-[1%] whitespace-nowrap px-4 py-3 align-middle', render: item => item.target.dataRetentionEnabled ? (item.target.dataCapacity || '1Gi') : t('common.disabled') },
        { key: 'auto', header: t('deploymentsPage.autoDeploy'), className: 'w-[1%] whitespace-nowrap px-4 py-3 align-middle', render: item => <StatusValueBadge value={item.target.autoDeploy ? 'enabled' : 'disabled'} /> },
        { key: 'runtimeStatus', header: t('deploymentsPage.runtimeStatus'), className: 'w-[1%] whitespace-nowrap px-4 py-3 align-middle', render: item => <DeploymentRuntimeStatusBadge status={item.runtimeStatus} /> },
        { key: 'runtimeMetrics', header: t('deploymentsPage.runtimeMetrics'), className: 'w-[1%] whitespace-nowrap px-4 py-3 align-middle', render: item => <DeploymentTargetMetricsCell applicationId={applicationId} enabled={item.target.enabled && Boolean(item.release)} projectId={projectId} targetId={item.target.id} /> },
        { key: 'internalEndpoint', header: t('deploymentsPage.internalEndpoint'), className: 'min-w-56 px-4 py-3 align-middle', render: item => <InternalServiceEndpoint endpoint={item.internalEndpoint} onCopy={onCopy} /> },
        { key: 'revision', header: t('deploymentsPage.revision'), className: 'w-[1%] whitespace-nowrap px-4 py-3 align-middle', render: item => item.release ? `#${item.release.revision}` : '-' },
        { key: 'image', header: t('deploymentsPage.image'), className: 'min-w-48 px-4 py-3 align-middle', render: item => item.release ? <CopyableHoverText className="max-w-60 rounded bg-background px-2 py-1 font-mono text-xs" display={shortImageRef(item.release.imageRef)} value={item.release.imageRef} /> : '-' },
        { key: 'status', header: t('deploymentsPage.releaseStatus'), className: 'w-[1%] whitespace-nowrap px-4 py-3 align-middle', render: item => item.release ? <StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={item.release.status} /> : <StatusValueBadge label={t('deploymentsPage.notDeployed')} value="pending" /> },
        { key: 'message', header: t('deploymentsPage.rolloutMessage'), className: 'min-w-56 px-4 py-3 align-middle', render: item => <CopyableHoverText className="max-w-72 text-sm text-muted-foreground" display={compactReleaseMessage(item.release?.message)} value={item.release?.message} /> },
        { key: 'time', header: t('deploymentsPage.releaseTime'), className: 'w-[1%] whitespace-nowrap px-4 py-3 align-middle', render: item => item.release ? formatReleaseTime(item.release, t) : '-' },
        {
          key: 'actions',
          header: t('common.actions'),
          cellClassName: 'bg-card',
          className: 'sticky right-0 z-10 w-[1%] whitespace-nowrap px-4 py-3 text-right align-middle shadow-[-10px_0_16px_-16px_rgba(15,23,42,0.6)]',
          headerClassName: 'z-20 bg-muted/95',
          render: item => (
            <DeploymentTargetActions
              applicationId={applicationId}
              createReleasePending={createReleasePending}
              deletePending={deletePending}
              deployableBuildRuns={deployableBuildRuns}
              item={item}
              projectId={projectId}
              pullLatestPending={pullLatestPending}
              restartPending={restartPending}
              rollbackPending={rollbackPending}
              onDeleteTarget={onDeleteTarget}
              onOpenConsole={onOpenConsole}
              onOpenReleaseDialog={onOpenReleaseDialog}
              onOpenTargetDialog={onOpenTargetDialog}
              onPullLatestImageDeploy={onPullLatestImageDeploy}
              onRestart={onRestart}
              onRollback={onRollback}
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
  )
}

function DeploymentTargetActions({
  applicationId,
  createReleasePending,
  deletePending,
  deployableBuildRuns,
  item,
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
  item: DeploymentTargetRow
  onDeleteTarget: (target: DeploymentTarget) => void
  onOpenConsole: (release: Release) => void
  onOpenReleaseDialog: (environmentId: string, deploymentTargetId: string) => void
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
          <DropdownMenuItem disabled={deleting || !deploymentTargetCanRelease(item.target, deployableBuildRuns) || createReleasePending} onSelect={() => onOpenReleaseDialog(item.target.environmentId, item.target.id)}>
            <Package className="size-4" />
            {item.release ? t('deploymentsPage.createRelease') : t('deploymentsPage.deployToEnvironment')}
          </DropdownMenuItem>
          <DropdownMenuItem disabled={deleting} onSelect={() => onOpenTargetDialog(item.target)}>
            <Pencil className="size-4" />
            {t('common.edit')}
          </DropdownMenuItem>
          {item.release && (
            <DropdownMenuItem onSelect={() => item.release && onViewLogs(item.release)}>
              <Eye className="size-4" />
              {t('deploymentsPage.viewLogs')}
            </DropdownMenuItem>
          )}
          {item.release && (
            <DropdownMenuItem
              disabled={item.release.status !== 'succeeded' && item.release.status !== 'running'}
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
          {item.target.dataRetentionEnabled && (
            <DropdownMenuItem onSelect={() => window.open(deploymentTargetDataExportUrl(projectId, applicationId, item.target.id), '_blank', 'noopener,noreferrer')}>
              <Download className="size-4" />
              {t('deploymentsPage.exportData')}
            </DropdownMenuItem>
          )}
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

function DeploymentTargetSummary({ target }: { target: DeploymentTarget }) {
  const deleteFailedMessage = target.deleteStatus === 'delete_failed' ? target.deleteMessage?.trim() : ''
  return (
    <div className="max-w-72 min-w-0">
      <span className="block truncate" title={target.name}>{target.name}</span>
      {target.deleteStatus && target.deleteStatus !== 'active' && (
        <div className="mt-1 flex min-w-0 items-center gap-2">
          <StatusValueBadge labelKeyPrefix="apps.deleteStatuses" value={target.deleteStatus} />
          {deleteFailedMessage && (
            <HoverText className="flex-1 text-xs text-muted-foreground" value={deleteFailedMessage} />
          )}
        </div>
      )}
    </div>
  )
}
