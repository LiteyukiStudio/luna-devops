import type { ReactNode } from 'react'
import type { DeploymentRuntimeStatus, InternalServiceEndpointValue } from './application-deployment-runtime-utils'
import type { BuildRun, DeploymentTarget, Release } from '@/api'
import { Download, Eye, MoreHorizontal, Package, Pencil, RefreshCw, RotateCcw, Terminal, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { CopyableHoverText } from '@/components/common/copyable-hover-text'
import { DataList } from '@/components/common/data-list'
import { EmptyState } from '@/components/common/empty-state'
import { HoverText } from '@/components/common/hover-text'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { deploymentTargetCanRelease, formatReleaseTime } from './application-config-utils'
import { DeploymentRuntimeStatusBadge, InternalServiceEndpoint } from './application-deployment-runtime'
import { DeploymentTargetMetricsCell } from './application-deployment-target-metrics-cell'
import { formatTargetRuntimeSize, shortImageRef } from './application-deployments-panel-utils'
import { openDeploymentTargetDataExport } from './deployment-target-data-export'

export interface DeploymentTargetRow {
  internalEndpoint?: InternalServiceEndpointValue
  release?: Release
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
    <>
      <div className="hidden md:block">
        <DataList
          columns={[
            { key: 'name', header: t('common.name'), width: 'primary', render: item => <DeploymentTargetSummary applicationId={applicationId} item={item} projectId={projectId} onCopy={onCopy} /> },
            { key: 'stage', header: t('deploymentsPage.stage'), width: 'compact', render: item => t(`deploymentsPage.stageLabels.${item.target.stage}`, { defaultValue: item.target.stage }) },
            { key: 'runtimeSize', header: t('deploymentsPage.runtimeEnvironment'), width: 'secondary', render: item => formatTargetRuntimeSize(item.target, t) },
            { key: 'runtimeStatus', header: t('deploymentsPage.runtimeStatus'), width: 'status', render: item => <DeploymentRuntimeStatusBadge status={item.runtimeStatus} /> },
            { key: 'status', header: t('deploymentsPage.releaseStatus'), width: 'status', render: item => <ReleaseStatusSummary release={item.release} /> },
            { key: 'image', header: t('deploymentsPage.imageSummary'), width: 'normal', render: item => <DeploymentImageSummary release={item.release} /> },
            {
              key: 'actions',
              header: t('common.actions'),
              cellClassName: 'bg-card',
              className: 'text-right shadow-[-10px_0_16px_-16px_rgba(15,23,42,0.6)]',
              headerClassName: 'z-20 bg-muted/95',
              sticky: 'right',
              width: 'actions',
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
      </div>
      <div className="md:hidden">
        {items.length === 0
          ? <EmptyState description={t('deploymentsPage.emptyDeploymentsDescription')} title={t('deploymentsPage.emptyDeployments')} variant="plain" />
          : (
              <div className="grid gap-3">
                {items.map(item => (
                  <MobileDeploymentTargetCard
                    key={item.target.id}
                    applicationId={applicationId}
                    createReleasePending={createReleasePending}
                    deletePending={deletePending}
                    deployableBuildRuns={deployableBuildRuns}
                    item={item}
                    projectId={projectId}
                    pullLatestPending={pullLatestPending}
                    restartPending={restartPending}
                    rollbackPending={rollbackPending}
                    onCopy={onCopy}
                    onDeleteTarget={onDeleteTarget}
                    onOpenConsole={onOpenConsole}
                    onOpenReleaseDialog={onOpenReleaseDialog}
                    onOpenTargetDialog={onOpenTargetDialog}
                    onPullLatestImageDeploy={onPullLatestImageDeploy}
                    onRestart={onRestart}
                    onRollback={onRollback}
                    onViewLogs={onViewLogs}
                  />
                ))}
              </div>
            )}
      </div>
    </>
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
  const exportData = () => {
    void openDeploymentTargetDataExport(projectId, applicationId, item.target.id)
      .catch(() => toast.error(t('deploymentsPage.dataExportFailed')))
  }

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
          {item.target.dataRetentionEnabled && (
            <DropdownMenuItem onSelect={exportData}>
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

function DeploymentTargetSummary({ applicationId, item, onCopy, projectId }: { applicationId: string, item: DeploymentTargetRow, onCopy: (value?: string) => void, projectId: string }) {
  const { t } = useTranslation()
  const { target } = item
  const deleteFailedMessage = target.deleteStatus === 'delete_failed' ? target.deleteMessage?.trim() : ''
  return (
    <div className="grid max-w-80 min-w-0 gap-2">
      <span className="block truncate" title={target.name}>{target.name}</span>
      {target.deleteStatus && target.deleteStatus !== 'active' && (
        <div className="mt-1 flex min-w-0 items-center gap-2">
          <StatusValueBadge labelKeyPrefix="apps.deleteStatuses" value={target.deleteStatus} />
          {deleteFailedMessage && (
            <HoverText className="flex-1 text-xs text-muted-foreground" value={deleteFailedMessage} />
          )}
        </div>
      )}
      <DeploymentTargetDetails
        applicationId={applicationId}
        className="text-xs"
        item={item}
        projectId={projectId}
        onCopy={onCopy}
      >
        {t('deploymentsPage.deploymentDetails')}
      </DeploymentTargetDetails>
    </div>
  )
}

function ReleaseStatusSummary({ release }: { release?: Release }) {
  const { t } = useTranslation()
  const message = release?.message?.trim()
  const badge = release
    ? <StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={release.status} />
    : <StatusValueBadge label={t('deploymentsPage.notDeployed')} value="pending" />

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
  applicationId,
  createReleasePending,
  deletePending,
  deployableBuildRuns,
  item,
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
  item: DeploymentTargetRow
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
    <article className="grid gap-3 rounded-lg border border-border bg-card p-4 shadow-sm">
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="truncate text-sm font-semibold">{item.target.name}</h3>
          <p className="mt-1 text-xs text-muted-foreground">
            {t(`deploymentsPage.stageLabels.${item.target.stage}`, { defaultValue: item.target.stage })}
            {' · '}
            {formatTargetRuntimeSize(item.target, t)}
          </p>
        </div>
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
      </div>
      <div className="grid grid-cols-2 gap-3 text-xs">
        <LabeledValue label={t('deploymentsPage.runtimeStatus')}>
          <DeploymentRuntimeStatusBadge status={item.runtimeStatus} />
        </LabeledValue>
        <LabeledValue label={t('deploymentsPage.releaseStatus')}>
          <ReleaseStatusSummary release={item.release} />
        </LabeledValue>
      </div>
      <LabeledValue label={t('deploymentsPage.imageSummary')}>
        <DeploymentImageSummary release={item.release} />
      </LabeledValue>
      <DeploymentTargetDetails
        applicationId={applicationId}
        item={item}
        projectId={projectId}
        onCopy={onCopy}
      >
        {t('deploymentsPage.deploymentDetails')}
      </DeploymentTargetDetails>
    </article>
  )
}

function DeploymentTargetDetails({
  applicationId,
  children,
  className,
  item,
  onCopy,
  projectId,
  showMetrics = true,
}: {
  applicationId: string
  children: ReactNode
  className?: string
  item: DeploymentTargetRow
  onCopy: (value?: string) => void
  projectId: string
  showMetrics?: boolean
}) {
  const { t } = useTranslation()
  const runtimeData = item.target.dataRetentionEnabled ? (item.target.dataCapacity || '1Gi') : t('common.disabled')
  const releaseMessage = item.release?.message?.trim()

  return (
    <details className={cn('group min-w-0 text-sm text-muted-foreground', className)}>
      <summary className="cursor-pointer list-none text-xs font-medium text-muted-foreground transition hover:text-foreground [&::-webkit-details-marker]:hidden">
        {children}
      </summary>
      <div className="mt-3 grid gap-3 rounded-md bg-muted/40 p-3">
        <div className="grid gap-3 sm:grid-cols-2">
          <LabeledValue label={t('deploymentsPage.internalEndpoint')}>
            <InternalServiceEndpoint endpoint={item.internalEndpoint} onCopy={onCopy} />
          </LabeledValue>
          {showMetrics && (
            <LabeledValue label={t('deploymentsPage.runtimeMetrics')}>
              <DeploymentTargetMetricsCell applicationId={applicationId} enabled={item.target.enabled && Boolean(item.release)} projectId={projectId} targetId={item.target.id} />
            </LabeledValue>
          )}
          <LabeledValue label={t('deploymentsPage.runtimeData')}>
            <span>{runtimeData}</span>
          </LabeledValue>
          <LabeledValue label={t('deploymentsPage.autoDeploy')}>
            <StatusValueBadge value={item.target.autoDeploy ? 'enabled' : 'disabled'} />
          </LabeledValue>
          <LabeledValue label={t('deploymentsPage.revision')}>
            <span>{item.release ? `#${item.release.revision}` : '-'}</span>
          </LabeledValue>
          <LabeledValue label={t('deploymentsPage.releaseTime')}>
            <span>{item.release ? formatReleaseTime(item.release, t) : '-'}</span>
          </LabeledValue>
        </div>
        {releaseMessage && (
          <LabeledValue label={t('deploymentsPage.rolloutMessage')}>
            <CopyableHoverText
              className="max-w-full overflow-visible whitespace-pre-wrap break-words text-xs text-muted-foreground"
              display={<span className="whitespace-pre-wrap break-words">{releaseMessage}</span>}
              value={releaseMessage}
            />
          </LabeledValue>
        )}
      </div>
    </details>
  )
}

function LabeledValue({ children, label }: { children: ReactNode, label: string }) {
  return (
    <div className="grid min-w-0 gap-1">
      <span className="text-[11px] font-medium uppercase text-muted-foreground">{label}</span>
      <div className="min-w-0 text-sm text-foreground">{children}</div>
    </div>
  )
}
