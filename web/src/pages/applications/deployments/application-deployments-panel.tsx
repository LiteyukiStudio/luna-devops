import type { TFunction } from 'i18next'
import type { ReleaseForm } from './application-deployments-panel-utils'
import type { ApplicationDeploymentSummary, BuildRun, DeploymentTarget, GatewayRoute, Release } from '@/api'
import type { DeploymentListState, DeploymentOperationPending } from '@/pages/applications/deployments/operations/application-deployment-targets-list'
import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useImperativeHandle, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { api } from '@/api'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { CopyableHoverText } from '@/components/common/copyable-hover-text'
import { buildRunImageRef, latestDeployableBuildRuns } from '@/components/common/deployment-build-runs'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { shortImageRef } from '@/lib/image-ref'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { statusRefetchInterval } from '@/lib/polling'
import { deploymentReleaseKey, deploymentTargetCanRelease, formatReleaseTime } from '@/pages/applications/application-config-utils'
import { buildDeploymentRuntimeStatus, buildInternalServiceEndpoint } from '@/pages/applications/deployments/operations/application-deployment-runtime-utils'
import { ApplicationDeploymentTargetsList } from '@/pages/applications/deployments/operations/application-deployment-targets-list'
import { ApplicationWebConsolePanel } from '@/pages/applications/runtime/application-web-console-panel'
import { effectiveWebConsoleEnabled } from '@/pages/applications/runtime/web-console-policy'
import { DeferredCreateReleaseDialog, DeferredDeploymentBundleImportDialog } from './application-deployment-dialogs'
import { latestReleaseByTarget as latestReleasesByTarget, previousSuccessfulRelease, redeployReleasePayload, releaseDefaults } from './application-deployments-panel-utils'
import { ApplicationReleaseLogsPanel } from './application-release-logs-panel'

export interface DeploymentsPanelHandle {
  openImportDialog: () => void
  openReleaseDialog: (deploymentTargetId?: string) => void
}

/** 高风险运行操作先确认再执行；回滚确认展示将回到的版本。 */
type ConfirmOperation
  = | { kind: 'pullLatest', target: DeploymentTarget }
    | { kind: 'restart', target: DeploymentTarget }
    | { kind: 'rollback', release: Release, rollbackTo: Release, target: DeploymentTarget }

interface LogTarget {
  release: Release
  view: 'deployment' | 'runtime'
}

export function ApplicationDeploymentsPanel({ applicationId, buildRuns, deploymentTargets, listState, projectId, projectWebConsoleEnabled, ref, releases, routes, runtimeSummary }: {
  applicationId: string
  buildRuns: BuildRun[]
  deploymentTargets: DeploymentTarget[]
  listState: DeploymentListState
  projectId: string
  projectWebConsoleEnabled: boolean
  ref?: React.Ref<DeploymentsPanelHandle>
  releases: Release[]
  routes: GatewayRoute[]
  /** 后端统一的应用运行结论；提供每个部署配置是否已有成功发布。 */
  runtimeSummary?: ApplicationDeploymentSummary
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [bundleImportOpen, setBundleImportOpen] = useState(false)
  const [logTarget, setLogTarget] = useState<LogTarget | null>(null)
  const [consoleRelease, setConsoleRelease] = useState<Release | null>(null)
  const [targetToDelete, setTargetToDelete] = useState<DeploymentTarget | null>(null)
  const [deleteConfirmation, setDeleteConfirmation] = useState('')
  const [confirmOperation, setConfirmOperation] = useState<ConfirmOperation | null>(null)
  const [releaseToCancel, setReleaseToCancel] = useState<Release | null>(null)
  const form = useForm<ReleaseForm>({ defaultValues: releaseDefaults, mode: 'onChange' })
  const runtimeClusters = useQuery({
    queryKey: ['runtime-clusters', projectId],
    queryFn: () => api.listRuntimeClusters(projectId),
    enabled: Boolean(projectId),
    ...liveObservationQueryPolicy,
  })
  const runtimeClusterMap = useMemo(() => Object.fromEntries((runtimeClusters.data ?? []).map(cluster => [cluster.id, cluster])), [runtimeClusters.data])
  const buildRunMap = useMemo(() => Object.fromEntries(buildRuns.map(run => [run.id, run])), [buildRuns])
  const latestReleaseByTarget = useMemo(() => latestReleasesByTarget(releases), [releases])
  const deployableBuildRuns = useMemo(() => latestDeployableBuildRuns(buildRuns), [buildRuns])
  const selectedDeploymentTargetId = form.watch('deploymentTargetId')
  const selectedReleaseTarget = deploymentTargets.find(target => target.id === selectedDeploymentTargetId)
  const selectableBuildRuns = useMemo(
    () => selectedDeploymentTargetId ? deployableBuildRuns.filter(run => run.deploymentTargetId === selectedDeploymentTargetId) : deployableBuildRuns,
    [deployableBuildRuns, selectedDeploymentTargetId],
  )
  const releaseReadyTargets = useMemo(() => deploymentTargets.filter(target => deploymentTargetCanRelease(target, deployableBuildRuns)), [deployableBuildRuns, deploymentTargets])
  const selectedBuildRun = buildRunMap[form.watch('buildRunId')]
  // 生效集群由后端统一解析（effectiveClusterId），前端不推断默认集群。
  const workloadClusterIds = useMemo(() => {
    const ids = new Set<string>()
    for (const target of deploymentTargets) {
      if (target.effectiveClusterId)
        ids.add(target.effectiveClusterId)
    }
    return [...ids].sort()
  }, [deploymentTargets])
  const runtimeObservationInterval = statusRefetchInterval(releases.some(release => release.status === 'pending' || release.status === 'running'))
  const workloadResourceQueries = useQueries({
    queries: workloadClusterIds.map(clusterId => ({
      enabled: Boolean(projectId && applicationId && clusterId),
      queryFn: () => api.listRuntimeClusterResources(clusterId, { resourceCategory: 'workloads', projectId, applicationId }),
      queryKey: ['runtime-cluster-resources', clusterId, 'workloads', projectId, applicationId],
      refetchInterval: runtimeObservationInterval,
      ...liveObservationQueryPolicy,
    })),
  })
  const serviceResourceQueries = useQueries({
    queries: workloadClusterIds.map(clusterId => ({
      enabled: Boolean(projectId && applicationId && clusterId),
      queryFn: () => api.listRuntimeClusterResources(clusterId, { resourceCategory: 'services', projectId, applicationId }),
      queryKey: ['runtime-cluster-resources', clusterId, 'services', projectId, applicationId],
      refetchInterval: runtimeObservationInterval,
      ...liveObservationQueryPolicy,
    })),
  })
  const workloadResourcesByCluster = useMemo(() => Object.fromEntries(workloadClusterIds.map((clusterId, index) => [clusterId, workloadResourceQueries[index]?.data ?? []] as const)), [workloadClusterIds, workloadResourceQueries])
  const workloadLoadingByCluster = useMemo(() => Object.fromEntries(workloadClusterIds.map((clusterId, index) => {
    const query = workloadResourceQueries[index]
    return [clusterId, Boolean(query?.isLoading && !query.data)] as const
  })), [workloadClusterIds, workloadResourceQueries])
  const workloadErrorByCluster = useMemo(() => Object.fromEntries(workloadClusterIds.map((clusterId, index) => [clusterId, Boolean(workloadResourceQueries[index]?.isError)] as const)), [workloadClusterIds, workloadResourceQueries])
  const serviceResourcesByCluster = useMemo(() => Object.fromEntries(workloadClusterIds.map((clusterId, index) => [clusterId, serviceResourceQueries[index]?.data ?? []] as const)), [serviceResourceQueries, workloadClusterIds])
  const deploymentRows = useMemo(() => deploymentTargets.map((target) => {
    const clusterId = target.effectiveClusterId
    const targetSummary = runtimeSummary?.targets.find(item => item.id === target.id)
    const release = latestReleaseByTarget[deploymentReleaseKey(target.id)]
    return {
      // 只有后端明确判定为未部署时才隐藏实时状态；结论加载中或不可用时如实展示观察值。
      deployed: targetSummary ? targetSummary.status !== 'not-deployed' : true,
      internalEndpoint: buildInternalServiceEndpoint(target, serviceResourcesByCluster[clusterId] ?? []),
      release,
      rollbackTo: release ? previousSuccessfulRelease(releases, release) : undefined,
      routes: routes.filter(route => route.deploymentTargetId === target.id),
      runtimeStatus: buildDeploymentRuntimeStatus(
        target,
        runtimeClusterMap[clusterId],
        workloadResourcesByCluster,
        workloadLoadingByCluster,
        workloadErrorByCluster,
      ),
      target,
      webConsoleEnabled: effectiveWebConsoleEnabled(projectWebConsoleEnabled, target.webConsoleEnabled),
    }
  }), [deploymentTargets, latestReleaseByTarget, projectWebConsoleEnabled, releases, routes, runtimeClusterMap, runtimeSummary, serviceResourcesByCluster, workloadErrorByCluster, workloadLoadingByCluster, workloadResourcesByCluster])
  const openTargetEditor = (target?: DeploymentTarget) => {
    const base = `/projects/${projectId}/apps/${applicationId}/deployments`
    navigate(target ? `${base}/${target.id}/edit` : `${base}/new`)
  }
  const openReleaseDialog = (deploymentTargetId = '') => {
    const defaultTarget = deploymentTargetId
      ? deploymentTargets.find(target => target.id === deploymentTargetId)
      : releaseReadyTargets[0]
    const targetId = defaultTarget?.id ?? deploymentTargetId
    const matchedRun = targetId ? deployableBuildRuns.find(run => run.deploymentTargetId === targetId) : undefined
    form.reset({
      ...releaseDefaults,
      applicationId: matchedRun?.applicationId ?? applicationId,
      deploymentTargetId: targetId ?? '',
      buildRunId: matchedRun?.id ?? '',
      imageRef: matchedRun ? buildRunImageRef(matchedRun) : defaultTarget?.imageRef ?? '',
    })
    setDialogOpen(true)
  }
  useImperativeHandle(ref, () => ({ openImportDialog: () => setBundleImportOpen(true), openReleaseDialog }))
  useEffect(() => {
    if (!selectedBuildRun)
      return
    form.setValue('deploymentTargetId', selectedBuildRun.deploymentTargetId, { shouldDirty: true, shouldValidate: true })
    form.setValue('applicationId', selectedBuildRun.applicationId, { shouldDirty: true, shouldValidate: true })
    form.setValue('imageRef', buildRunImageRef(selectedBuildRun), { shouldDirty: true, shouldValidate: true })
  }, [form, selectedBuildRun])
  useEffect(() => {
    if (!selectedReleaseTarget || selectedBuildRun)
      return
    form.setValue('applicationId', applicationId, { shouldDirty: true, shouldValidate: true })
    if (selectedReleaseTarget.sourceType === 'image')
      form.setValue('imageRef', selectedReleaseTarget.imageRef, { shouldDirty: true, shouldValidate: true })
  }, [applicationId, form, selectedBuildRun, selectedReleaseTarget])
  // 长任务创建后在 toast 中直接给出进度入口，日志面板按发布 ID 读取最新状态。
  const viewLogsAction = (release: Release, view: LogTarget['view'] = 'deployment') => ({
    label: t(view === 'runtime' ? 'deploymentsPage.viewRuntimeLogs' : 'deploymentsPage.viewProgress'),
    onClick: () => setLogTarget({ release, view }),
  })
  const createRelease = useMutation({
    mutationFn: (values: ReleaseForm) => api.createRelease(projectId, values),
    onSuccess: (release) => {
      toast.success(t('deploymentsPage.releaseCreated'), { action: viewLogsAction(release) })
      setDialogOpen(false)
      form.reset(releaseDefaults)
      queryClient.invalidateQueries({ queryKey: ['releases', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  const rollbackRelease = useMutation({
    mutationFn: (source: Release) => api.rollbackRelease(projectId, source.id),
    onSuccess: (release) => {
      toast.success(t('deploymentsPage.rollbackQueued'), { action: viewLogsAction(release) })
      queryClient.invalidateQueries({ queryKey: ['releases', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  const cancelRelease = useMutation({
    mutationFn: (release: Release) => api.cancelRelease(projectId, release.id),
    onSuccess: () => {
      toast.success(t('deploymentsPage.releaseCanceled'))
      setReleaseToCancel(null)
      queryClient.invalidateQueries({ queryKey: ['releases', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  const restartTarget = useMutation({
    mutationFn: (target: DeploymentTarget) => api.restartDeploymentTarget(projectId, applicationId, target.id),
    onSuccess: (_, target) => {
      const release = latestReleaseByTarget[deploymentReleaseKey(target.id)]
      toast.success(t('deploymentsPage.restartQueued'), release ? { action: viewLogsAction(release, 'runtime') } : undefined)
      queryClient.invalidateQueries({ queryKey: ['runtime-cluster-resources'] })
    },
    onError: error => toast.error(error.message),
  })
  const pullLatestImageDeploy = useMutation({
    mutationFn: async (target: DeploymentTarget) => {
      const releasePayload = redeployReleasePayload(target, latestReleaseByTarget[deploymentReleaseKey(target.id)], { forceImagePull: true })
      if (!releasePayload)
        throw new Error(t('deploymentsPage.redeployUnavailable'))
      return api.createRelease(projectId, releasePayload)
    },
    onSuccess: (release) => {
      toast.success(t('deploymentsPage.pullLatestImageDeployQueued'), { action: viewLogsAction(release) })
      queryClient.invalidateQueries({ queryKey: ['releases', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  const deleteTarget = useMutation({
    mutationFn: (target: DeploymentTarget) => api.deleteDeploymentTarget(projectId, applicationId, target.id),
    onSuccess: () => {
      toast.success(t('deploymentsPage.targetDeleted'))
      setTargetToDelete(null)
      setDeleteConfirmation('')
      queryClient.invalidateQueries({ queryKey: ['deployment-targets', projectId, applicationId] })
      queryClient.invalidateQueries({ queryKey: ['application-runtime', projectId, applicationId] })
      queryClient.invalidateQueries({ queryKey: ['releases', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  // pending 精确到发起操作的那一行，其他部署配置的同类操作不受影响。
  const operationPending: DeploymentOperationPending = {
    pullLatest: pullLatestImageDeploy.isPending ? pullLatestImageDeploy.variables?.id : undefined,
    restart: restartTarget.isPending ? restartTarget.variables?.id : undefined,
    rollback: rollbackRelease.isPending ? rollbackRelease.variables?.deploymentTargetId : undefined,
  }
  const runConfirmedOperation = (operation: ConfirmOperation) => {
    if (operation.kind === 'restart')
      restartTarget.mutate(operation.target)
    else if (operation.kind === 'rollback')
      rollbackRelease.mutate(operation.release)
    else
      pullLatestImageDeploy.mutate(operation.target)
  }
  const confirmCopy = confirmOperation ? operationConfirmCopy(confirmOperation, t) : null
  const deleteConfirmationTarget = targetToDelete?.name ?? ''
  const deleteConfirmationMatches = Boolean(deleteConfirmationTarget) && deleteConfirmation === deleteConfirmationTarget
  const logRelease = logTarget ? releases.find(release => release.id === logTarget.release.id) ?? logTarget.release : null
  return (
    <div className="grid min-w-0 max-w-full gap-4">
      <ApplicationDeploymentTargetsList
        applicationId={applicationId}
        cancelReleasePending={cancelRelease.isPending}
        createReleasePending={createRelease.isPending}
        deletePending={deleteTarget.isPending}
        deployableBuildRuns={deployableBuildRuns}
        items={deploymentRows}
        listState={listState}
        operationPending={operationPending}
        projectId={projectId}
        onCreateTarget={() => openTargetEditor()}
        onDeleteTarget={(target) => {
          setTargetToDelete(target)
          setDeleteConfirmation('')
        }}
        onCancelRelease={setReleaseToCancel}
        onOpenConsole={setConsoleRelease}
        onOpenReleaseDialog={openReleaseDialog}
        onEditTarget={openTargetEditor}
        onPullLatestImageDeploy={target => setConfirmOperation({ kind: 'pullLatest', target })}
        onRestart={target => setConfirmOperation({ kind: 'restart', target })}
        onRollback={(item) => {
          if (item.release && item.rollbackTo)
            setConfirmOperation({ kind: 'rollback', release: item.release, rollbackTo: item.rollbackTo, target: item.target })
        }}
        onViewLogs={release => setLogTarget({ release, view: 'deployment' })}
      />
      <DeferredDeploymentBundleImportDialog
        applicationId={applicationId}
        open={bundleImportOpen}
        projectId={projectId}
        onImported={() => {
          void queryClient.invalidateQueries({ queryKey: ['deployment-targets', projectId, applicationId] })
          void queryClient.invalidateQueries({ queryKey: ['application-runtime', projectId, applicationId] })
        }}
        onOpenChange={setBundleImportOpen}
      />
      <DeferredCreateReleaseDialog
        applicationId={applicationId}
        form={form}
        open={dialogOpen}
        pending={createRelease.isPending}
        projectId={projectId}
        releaseReadyTargets={releaseReadyTargets}
        selectableBuildRuns={selectableBuildRuns}
        selectedTarget={selectedReleaseTarget}
        onOpenChange={setDialogOpen}
        onSubmit={values => createRelease.mutate(values)}
      />
      {logTarget && logRelease && (
        <ApplicationReleaseLogsPanel
          key={`${logRelease.id}-${logTarget.view}`}
          initialView={logTarget.view}
          projectId={projectId}
          release={logRelease}
          onClose={() => setLogTarget(null)}
        />
      )}
      {consoleRelease && (
        <ApplicationWebConsolePanel
          projectId={projectId}
          release={consoleRelease}
          onClose={() => setConsoleRelease(null)}
        />
      )}
      <ConfirmDialog
        cancelText={t('common.cancel')}
        confirmText={confirmCopy?.confirm}
        confirmVariant={confirmOperation?.kind === 'rollback' ? 'destructive' : 'default'}
        content={confirmOperation?.kind === 'rollback'
          ? <RollbackTargetSummary release={confirmOperation.rollbackTo} target={confirmOperation.target} />
          : undefined}
        description={confirmCopy?.description ?? ''}
        open={Boolean(confirmOperation)}
        title={confirmCopy?.title ?? ''}
        onConfirm={() => {
          if (confirmOperation)
            runConfirmedOperation(confirmOperation)
        }}
        onOpenChange={open => !open && setConfirmOperation(null)}
      />
      <ConfirmDialog
        cancelText={t('common.cancel')}
        confirmDisabled={!deleteConfirmationMatches}
        confirmText={t('deploymentsPage.deleteDeploymentConfigConfirm')}
        content={(
          <div className="grid gap-3">
            <Label htmlFor="deployment-target-delete-confirmation">{t('deploymentsPage.deleteDeploymentConfigConfirmationLabel', { name: deleteConfirmationTarget })}</Label>
            <Input
              id="deployment-target-delete-confirmation"
              aria-invalid={Boolean(deleteConfirmation) && !deleteConfirmationMatches}
              autoComplete="off"
              placeholder={deleteConfirmationTarget}
              value={deleteConfirmation}
              onChange={event => setDeleteConfirmation(event.target.value)}
            />
            <p className="text-xs text-muted-foreground">{t('deploymentsPage.deleteDeploymentConfigConfirmationHint')}</p>
          </div>
        )}
        description={targetToDelete ? t('deploymentsPage.deleteDeploymentConfigDescription', { name: targetToDelete.name }) : ''}
        open={Boolean(targetToDelete)}
        pending={deleteTarget.isPending}
        title={t('deploymentsPage.deleteDeploymentConfigTitle')}
        onConfirm={() => {
          if (targetToDelete && deleteConfirmationMatches)
            deleteTarget.mutate(targetToDelete)
        }}
        onOpenChange={(open) => {
          if (!open) {
            setTargetToDelete(null)
            setDeleteConfirmation('')
          }
        }}
      />
      <ConfirmDialog
        cancelText={t('common.cancel')}
        confirmText={t('deploymentsPage.cancelReleaseConfirm')}
        description={t('deploymentsPage.cancelReleaseDescription')}
        open={Boolean(releaseToCancel)}
        pending={cancelRelease.isPending}
        title={t('deploymentsPage.cancelReleaseTitle')}
        onConfirm={() => releaseToCancel && cancelRelease.mutate(releaseToCancel)}
        onOpenChange={open => !open && setReleaseToCancel(null)}
      />
    </div>
  )
}

function operationConfirmCopy(operation: ConfirmOperation, t: TFunction) {
  const name = operation.target.name
  switch (operation.kind) {
    case 'restart':
      return {
        confirm: t('deploymentsPage.operationConfirm.restart.confirm'),
        description: t('deploymentsPage.operationConfirm.restart.description', { name }),
        title: t('deploymentsPage.operationConfirm.restart.title'),
      }
    case 'rollback':
      return {
        confirm: t('deploymentsPage.operationConfirm.rollback.confirm'),
        description: t('deploymentsPage.operationConfirm.rollback.description', { name }),
        title: t('deploymentsPage.operationConfirm.rollback.title'),
      }
    case 'pullLatest':
      return {
        confirm: t('deploymentsPage.operationConfirm.pullLatest.confirm'),
        description: t('deploymentsPage.operationConfirm.pullLatest.description', { name }),
        title: t('deploymentsPage.operationConfirm.pullLatest.title'),
      }
  }
}

/** 回滚确认里展示将回到的版本：镜像、发布时间与部署配置。 */
function RollbackTargetSummary({ release, target }: { release: Release, target: DeploymentTarget }) {
  const { t } = useTranslation()
  return (
    <dl className="grid gap-2 rounded-control border border-border bg-muted/40 p-3 text-sm" data-slot="rollback-target">
      <div className="grid grid-cols-[6rem_minmax(0,1fr)] items-baseline gap-2">
        <dt className="text-muted-foreground">{t('deploymentsPage.operationConfirm.rollback.image')}</dt>
        <dd className="min-w-0">
          <CopyableHoverText className="font-mono text-xs" display={shortImageRef(release.imageRef)} value={release.imageRef} />
        </dd>
      </div>
      <div className="grid grid-cols-[6rem_minmax(0,1fr)] items-baseline gap-2">
        <dt className="text-muted-foreground">{t('deploymentsPage.operationConfirm.rollback.releasedAt')}</dt>
        <dd className="min-w-0">{`${formatReleaseTime(release, t)} · #${release.revision}`}</dd>
      </div>
      <div className="grid grid-cols-[6rem_minmax(0,1fr)] items-baseline gap-2">
        <dt className="text-muted-foreground">{t('deploymentsPage.operationConfirm.rollback.deploymentTarget')}</dt>
        <dd className="min-w-0 truncate" title={target.name}>{target.name}</dd>
      </div>
    </dl>
  )
}
