import type { ReleaseForm } from './application-deployments-panel-utils'
import type { BuildRun, DeploymentTarget, GatewayRoute, Release } from '@/api'
import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useImperativeHandle, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { api } from '@/api'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { buildRunImageRef, latestDeployableBuildRuns } from '@/components/common/deployment-build-runs'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { statusRefetchInterval } from '@/lib/polling'
import { deploymentReleaseKey, deploymentTargetCanRelease } from '@/pages/applications/application-config-utils'
import { buildDeploymentRuntimeStatus, buildInternalServiceEndpoint } from '@/pages/applications/deployments/operations/application-deployment-runtime-utils'
import { ApplicationDeploymentTargetsList } from '@/pages/applications/deployments/operations/application-deployment-targets-list'
import { ApplicationWebConsolePanel } from '@/pages/applications/runtime/application-web-console-panel'
import { effectiveWebConsoleEnabled } from '@/pages/applications/runtime/web-console-policy'
import { DeferredCreateReleaseDialog, DeferredDeploymentBundleImportDialog } from './application-deployment-dialogs'
import { latestReleaseByTarget as latestReleasesByTarget, redeployReleasePayload, releaseDefaults } from './application-deployments-panel-utils'
import { ApplicationReleaseLogsPanel } from './application-release-logs-panel'

export interface DeploymentsPanelHandle {
  openImportDialog: () => void
  openReleaseDialog: (deploymentTargetId?: string) => void
}

export function ApplicationDeploymentsPanel({ applicationId, buildRuns, deploymentTargets, projectId, projectWebConsoleEnabled, ref, releases, routes }: {
  applicationId: string
  buildRuns: BuildRun[]
  deploymentTargets: DeploymentTarget[]
  projectId: string
  projectWebConsoleEnabled: boolean
  ref?: React.Ref<DeploymentsPanelHandle>
  releases: Release[]
  routes: GatewayRoute[]
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [bundleImportOpen, setBundleImportOpen] = useState(false)
  const [logRelease, setLogRelease] = useState<Release | null>(null)
  const [consoleRelease, setConsoleRelease] = useState<Release | null>(null)
  const [targetToDelete, setTargetToDelete] = useState<DeploymentTarget | null>(null)
  const form = useForm<ReleaseForm>({ defaultValues: releaseDefaults, mode: 'onChange' })
  const runtimeClusters = useQuery({
    queryKey: ['runtime-clusters', projectId],
    queryFn: () => api.listRuntimeClusters(projectId),
    enabled: Boolean(projectId),
    ...liveObservationQueryPolicy,
  })
  const runtimeClusterMap = useMemo(() => Object.fromEntries((runtimeClusters.data ?? []).map(cluster => [cluster.id, cluster])), [runtimeClusters.data])
  const defaultRuntimeCluster = useMemo(() => {
    const clusters = runtimeClusters.data ?? []
    return clusters.find(cluster => cluster.isDefault) ?? clusters[0]
  }, [runtimeClusters.data])
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
  const workloadClusterIds = useMemo(() => {
    const ids = new Set<string>()
    for (const target of deploymentTargets) {
      const clusterId = target.clusterId?.trim() || defaultRuntimeCluster?.id
      if (clusterId)
        ids.add(clusterId)
    }
    return [...ids].sort()
  }, [defaultRuntimeCluster?.id, deploymentTargets])
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
    const runtimeCluster = target.clusterId ? runtimeClusterMap[target.clusterId] : defaultRuntimeCluster
    const clusterId = target.clusterId?.trim() || runtimeCluster?.id || defaultRuntimeCluster?.id || ''
    return {
      internalEndpoint: buildInternalServiceEndpoint(target, serviceResourcesByCluster[clusterId] ?? []),
      release: latestReleaseByTarget[deploymentReleaseKey(target.id)],
      routes: routes.filter(route => route.deploymentTargetId === target.id),
      runtimeStatus: buildDeploymentRuntimeStatus(
        target,
        runtimeCluster ?? defaultRuntimeCluster,
        workloadResourcesByCluster,
        workloadLoadingByCluster,
        workloadErrorByCluster,
      ),
      target,
      webConsoleEnabled: effectiveWebConsoleEnabled(projectWebConsoleEnabled, target.webConsoleEnabled),
    }
  }), [defaultRuntimeCluster, deploymentTargets, latestReleaseByTarget, projectWebConsoleEnabled, routes, runtimeClusterMap, serviceResourcesByCluster, workloadErrorByCluster, workloadLoadingByCluster, workloadResourcesByCluster])
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
  const createRelease = useMutation({
    mutationFn: (values: ReleaseForm) => api.createRelease(projectId, values),
    onSuccess: () => {
      toast.success(t('deploymentsPage.releaseCreated'))
      setDialogOpen(false)
      form.reset(releaseDefaults)
      queryClient.invalidateQueries({ queryKey: ['releases', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  const rollbackRelease = useMutation({
    mutationFn: (releaseId: string) => api.rollbackRelease(projectId, releaseId),
    onSuccess: () => {
      toast.success(t('deploymentsPage.rollbackQueued'))
      queryClient.invalidateQueries({ queryKey: ['releases', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  const restartTarget = useMutation({
    mutationFn: (target: DeploymentTarget) => api.restartDeploymentTarget(projectId, applicationId, target.id),
    onSuccess: () => {
      toast.success(t('deploymentsPage.restartQueued'))
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
    onSuccess: () => {
      toast.success(t('deploymentsPage.pullLatestImageDeployQueued'))
      queryClient.invalidateQueries({ queryKey: ['releases', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  const deleteTarget = useMutation({
    mutationFn: (target: DeploymentTarget) => api.deleteDeploymentTarget(projectId, applicationId, target.id),
    onSuccess: () => {
      toast.success(t('deploymentsPage.targetDeleted'))
      setTargetToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['deployment-targets', projectId, applicationId] })
      queryClient.invalidateQueries({ queryKey: ['releases', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  return (
    <div className="grid min-w-0 max-w-full gap-4">
      <ApplicationDeploymentTargetsList
        applicationId={applicationId}
        createReleasePending={createRelease.isPending}
        deletePending={deleteTarget.isPending}
        deployableBuildRuns={deployableBuildRuns}
        items={deploymentRows}
        projectId={projectId}
        pullLatestPending={pullLatestImageDeploy.isPending}
        restartPending={restartTarget.isPending}
        rollbackPending={rollbackRelease.isPending}
        onDeleteTarget={setTargetToDelete}
        onOpenConsole={setConsoleRelease}
        onOpenReleaseDialog={openReleaseDialog}
        onEditTarget={openTargetEditor}
        onPullLatestImageDeploy={target => pullLatestImageDeploy.mutate(target)}
        onRestart={target => restartTarget.mutate(target)}
        onRollback={releaseId => rollbackRelease.mutate(releaseId)}
        onViewLogs={setLogRelease}
      />
      <DeferredDeploymentBundleImportDialog
        applicationId={applicationId}
        open={bundleImportOpen}
        projectId={projectId}
        onImported={() => queryClient.invalidateQueries({ queryKey: ['deployment-targets', projectId, applicationId] })}
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
      {logRelease && (
        <ApplicationReleaseLogsPanel
          projectId={projectId}
          release={logRelease}
          onClose={() => setLogRelease(null)}
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
        confirmText={t('common.delete')}
        description={t('deploymentsPage.deleteDeploymentConfigDescription')}
        open={Boolean(targetToDelete)}
        pending={deleteTarget.isPending}
        title={t('deploymentsPage.deleteDeploymentConfigTitle')}
        onConfirm={() => targetToDelete && deleteTarget.mutate(targetToDelete)}
        onOpenChange={open => !open && setTargetToDelete(null)}
      />
    </div>
  )
}
