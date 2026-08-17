import type { ReleaseForm } from './application-deployments-panel-utils'
import type { RepositoryBindingDialogForm, RepositoryBindingDialogFormInput } from './application-repository-binding-dialog'
import type { ArtifactRegistry, BuildRun, DeploymentTarget, DeploymentTargetPayload, ProjectRuntimeConfigSet, ProjectRuntimeConfigSetPayload, Release, RepositoryBinding } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { useEffect, useImperativeHandle, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { buildRunImageRef, latestDeployableBuildRuns } from '@/components/common/deployment-build-runs'
import { useBillingDisplay } from '@/lib/billing-display'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { WORKFLOW_STATUS_REFETCH_INTERVAL_MS } from '@/lib/polling'
import { publicRuntimeEnvironmentInputs, publicRuntimeEnvironmentRecord, runtimeSecretKeys } from '@/lib/runtime-environment'
import { defaultBuildCpuRequest, defaultBuildMemoryRequest, defaultBuildTimeoutSeconds } from './application-build-defaults'
import { deploymentReleaseKey, deploymentTargetCanRelease, registryInputPrefix } from './application-config-utils'
import { DeferredCreateReleaseDialog, DeferredDeploymentBundleImportDialog, DeferredDeploymentTargetDialog, DeferredReleaseLogsDialog, DeferredRepositoryBindingDialog, DeferredRuntimeConfigSetDialog, DeferredWebConsoleDialog } from './application-deployment-dialogs'
import { buildDeploymentRuntimeStatus, buildInternalServiceEndpoint } from './application-deployment-runtime-utils'
import { ApplicationDeploymentTargetsList } from './application-deployment-targets-list'
import { applyDockerfileBuildDefaults, deploymentTargetHasRunningInstances, deploymentTargetRuntimeChanged, normalizeBoolean, normalizeDeploymentTargetPayload, normalizeRuntimeConfigPayload, normalizeRuntimeConfigRefs, redeployReleasePayload, releaseDefaults, repositoryBindingItems, runtimeConfigDefaults, runtimeConfigLiveSetIds, runtimeConfigRefIds } from './application-deployments-panel-utils'
import { useDeploymentTargetForm } from './use-deployment-target-form'
import { effectiveWebConsoleEnabled } from './web-console-policy'

export interface DeploymentsPanelHandle {
  openImportDialog: () => void
  openReleaseDialog: (environmentId?: string, deploymentTargetId?: string) => void
  openTargetDialog: () => void
}

const repositoryBindingSchema = z.object({
  autoConfigureWebhook: z.boolean().default(true),
  cloneUrl: z.string().optional(),
  defaultBranch: z.string().optional(),
  gitAccountId: z.string().min(1, i18next.t('repositories.gitAccountRequired')),
  owner: z.string().min(1, i18next.t('repositories.ownerRequired')),
  repo: z.string().min(1, i18next.t('repositories.repoRequired')),
})

type RepositoryBindingFormInput = RepositoryBindingDialogFormInput
type RepositoryBindingForm = RepositoryBindingDialogForm

const repositoryBindingDefaults: RepositoryBindingFormInput = {
  autoConfigureWebhook: true,
  cloneUrl: '',
  defaultBranch: 'main',
  gitAccountId: '',
  owner: '',
  repo: '',
}

function buildArgLineCount(raw?: string) {
  const value = raw?.trim()
  if (!value)
    return 0
  if (value.startsWith('{')) {
    try {
      return Object.keys(JSON.parse(value) as Record<string, string>).length
    }
    catch {
      return 0
    }
  }
  return value.split('\n').map(line => line.trim()).filter(line => line && !line.startsWith('#')).length
}

export function ApplicationDeploymentsPanel({ applicationId, applicationIdentifier, buildRuns, canManageRuntimeSecrets, deploymentTargets, projectId, projectIdentifier, projectWebConsoleEnabled, ref, registries, releases, repositoryBindings }: {
  applicationId: string
  applicationIdentifier: string
  buildRuns: BuildRun[]
  canManageRuntimeSecrets: boolean
  deploymentTargets: DeploymentTarget[]
  projectId: string
  projectIdentifier: string
  projectWebConsoleEnabled: boolean
  ref?: React.Ref<DeploymentsPanelHandle>
  registries: ArtifactRegistry[]
  repositoryBindings: RepositoryBinding[]
  releases: Release[]
}) {
  const { i18n, t } = useTranslation()
  const queryClient = useQueryClient()
  const billingDisplay = useBillingDisplay(i18n.language)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [bundleImportOpen, setBundleImportOpen] = useState(false)
  const [logRelease, setLogRelease] = useState<Release | null>(null)
  const [logView, setLogView] = useState<'deployment' | 'runtime'>('deployment')
  const [consoleRelease, setConsoleRelease] = useState<Release | null>(null)
  const [targetToDelete, setTargetToDelete] = useState<DeploymentTarget | null>(null)
  const [runtimeConfigDialogOpen, setRuntimeConfigDialogOpen] = useState(false)
  const [editingRuntimeConfigSet, setEditingRuntimeConfigSet] = useState<ProjectRuntimeConfigSet | null>(null)
  const [runtimeConfigFilesValid, setRuntimeConfigFilesValid] = useState(true)
  const [runtimeSecretFilesValid, setRuntimeSecretFilesValid] = useState(true)
  const [runtimeConfigRestartSetId, setRuntimeConfigRestartSetId] = useState('')
  const [runtimeConfigRestartAffectedCount, setRuntimeConfigRestartAffectedCount] = useState(0)
  const [repositoryBindingDialogOpen, setRepositoryBindingDialogOpen] = useState(false)
  const [repositoryBranchSearch, setRepositoryBranchSearch] = useState('')
  const form = useForm<ReleaseForm>({ defaultValues: releaseDefaults, mode: 'onChange' })
  const runtimeConfigForm = useForm<ProjectRuntimeConfigSetPayload>({ defaultValues: runtimeConfigDefaults, mode: 'onChange' })
  const repositoryBindingForm = useForm<RepositoryBindingFormInput, undefined, RepositoryBindingForm>({
    defaultValues: repositoryBindingDefaults,
    mode: 'onChange',
    resolver: zodResolver(repositoryBindingSchema),
  })
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
  const {
    buildEnvironmentStatus: targetBuildEnvironmentStatus,
    buildSecretRows: targetBuildSecretRows,
    buildSubmissionPayload,
    buildVariableRows: targetBuildVariableRows,
    changeRuntimeConfigRefMode,
    configFilesValid: targetConfigFilesValid,
    hasDataVolumes: targetHasDataVolumes,
    dataVolumes: targetDataVolumes,
    dialogOpen: targetDialogOpen,
    editingTarget,
    form: targetForm,
    handleDialogOpenChange: handleTargetDialogOpenChange,
    imageRefDirty: targetImageRefDirty,
    openDialog: openTargetFormDialog,
    runtimeFilesValid: targetRuntimeFilesValid,
    secretFilesValid: targetSecretFilesValid,
    selectedHookBindings: selectedDeploymentHookBindings,
    selectedRuntimeConfigRefs,
    servicePorts: targetServicePorts,
    setConfigFilesValid: setTargetConfigFilesValid,
    setHookBindings: setTargetHookBindings,
    setSecretFilesValid: setTargetSecretFilesValid,
    sourceType: targetSourceType,
    targetBuildHooksEnabled,
    toggleRuntimeConfigSet,
    updateBuildSecretRows: setTargetBuildSecretRows,
    updateBuildVariableRows: setTargetBuildVariableRows,
    updateDataVolumes: updateTargetDataVolumes,
    updateServicePorts: updateTargetServicePorts,
    values: watchedTargetValues,
  } = useDeploymentTargetForm({
    applicationId,
    applicationIdentifier,
    defaultRuntimeCluster,
    projectId,
    projectIdentifier,
    registries,
    repositoryBindings,
  })
  const runtimeHourCost = billingDisplay.runtimeHourCost(watchedTargetValues.replicas, watchedTargetValues.cpuRequest, watchedTargetValues.memoryRequest)
  const buildMinuteCost = billingDisplay.buildMinuteCost(watchedTargetValues.buildCpuRequest, watchedTargetValues.buildMemoryRequest)
  const buildTimeoutMinutes = Math.max(1, Math.round((Number(watchedTargetValues.buildTimeoutSeconds) || defaultBuildTimeoutSeconds) / 60))
  const buildRunMap = useMemo(() => Object.fromEntries(buildRuns.map(run => [run.id, run])), [buildRuns])
  const latestReleaseByTarget = useMemo(() => {
    const output: Record<string, Release> = {}
    for (const release of releases) {
      const key = deploymentReleaseKey(release.deploymentTargetId)
      const existing = output[key]
      if (!existing || new Date(release.createdAt).getTime() > new Date(existing.createdAt).getTime())
        output[key] = release
    }
    return output
  }, [releases])
  const deployableBuildRuns = useMemo(() => latestDeployableBuildRuns(buildRuns), [buildRuns])
  const selectedDeploymentTargetId = form.watch('deploymentTargetId')
  const selectedReleaseTarget = deploymentTargets.find(target => target.id === selectedDeploymentTargetId)
  const selectableBuildRuns = useMemo(
    () => selectedDeploymentTargetId ? deployableBuildRuns.filter(run => run.deploymentTargetId === selectedDeploymentTargetId) : deployableBuildRuns,
    [deployableBuildRuns, selectedDeploymentTargetId],
  )
  const targetRepositoryBindingId = watchedTargetValues.repositoryBindingId
  const targetRegistryId = watchedTargetValues.targetRegistryId
  const targetStage = watchedTargetValues.stage
  const targetName = watchedTargetValues.name
  const selectedTargetRepositoryBinding = repositoryBindings.find(binding => binding.id === targetRepositoryBindingId)
  const targetRegistry = registries.find(registry => registry.id === targetRegistryId)
  const targetImagePrefix = targetRegistry ? registryInputPrefix(targetRegistry) : ''
  const gitProviders = useQuery({ queryKey: ['git-providers'], queryFn: () => api.listGitProviders(), enabled: repositoryBindingDialogOpen })
  const gitAccounts = useQuery({
    queryKey: ['git-accounts'],
    queryFn: () => api.listGitAccounts(),
    enabled: repositoryBindingDialogOpen,
    ...liveObservationQueryPolicy,
  })
  const selectedRepositoryAccountId = repositoryBindingForm.watch('gitAccountId')
  const selectedRepositoryOwner = repositoryBindingForm.watch('owner')
  const selectedRepositoryName = repositoryBindingForm.watch('repo')
  const repositoryBranches = useQuery({
    queryKey: ['git-branches', selectedRepositoryAccountId, selectedRepositoryOwner, selectedRepositoryName, repositoryBranchSearch],
    queryFn: () => api.listGitBranches(selectedRepositoryAccountId || '', selectedRepositoryOwner || '', selectedRepositoryName || '', { search: repositoryBranchSearch, limit: 50 }),
    enabled: Boolean(repositoryBindingDialogOpen && selectedRepositoryAccountId && selectedRepositoryOwner && selectedRepositoryName),
    ...liveObservationQueryPolicy,
  })
  const targetBuildOptions = useQuery({
    queryKey: [
      'git-repository-build-options',
      selectedTargetRepositoryBinding?.gitAccountId,
      selectedTargetRepositoryBinding?.owner,
      selectedTargetRepositoryBinding?.repo,
      selectedTargetRepositoryBinding?.defaultBranch,
    ],
    queryFn: () => api.getGitRepositoryBuildOptions(
      selectedTargetRepositoryBinding?.gitAccountId ?? '',
      selectedTargetRepositoryBinding?.owner ?? '',
      selectedTargetRepositoryBinding?.repo ?? '',
      selectedTargetRepositoryBinding?.defaultBranch,
    ),
    enabled: Boolean(targetDialogOpen && targetSourceType === 'repository' && selectedTargetRepositoryBinding?.gitAccountId && selectedTargetRepositoryBinding.owner && selectedTargetRepositoryBinding.repo),
  })
  const buildTemplates = useQuery({
    queryKey: ['build-templates'],
    queryFn: () => api.listBuildTemplates(),
    enabled: Boolean(targetDialogOpen && targetSourceType === 'repository'),
    staleTime: 5 * 60 * 1000,
  })
  const targetImageTemplateDefault = useQuery({
    queryKey: ['registry-image-template-default', targetRegistryId, projectId, applicationId, targetStage, targetName],
    queryFn: () => api.getRegistryImageTemplateDefault(targetRegistryId, {
      applicationId,
      projectId,
      stage: targetStage,
      targetName,
    }),
    enabled: Boolean(targetDialogOpen && !editingTarget && targetSourceType === 'repository' && targetRegistryId && projectId && applicationId),
  })
  const dockerfileSuggestions = useMemo(() => targetBuildOptions.data?.dockerfiles ?? [], [targetBuildOptions.data?.dockerfiles])
  const buildContextSuggestions = useMemo(() => targetBuildOptions.data?.directories ?? [], [targetBuildOptions.data?.directories])
  const dockerfileExposedPorts = useMemo(() => targetBuildOptions.data?.exposedPorts ?? {}, [targetBuildOptions.data?.exposedPorts])
  const releaseReadyTargets = useMemo(() => deploymentTargets.filter(target => deploymentTargetCanRelease(target, deployableBuildRuns)), [deployableBuildRuns, deploymentTargets])
  const selectedBuildRun = buildRunMap[form.watch('buildRunId')]
  const latestEditingTargetRelease = editingTarget ? latestReleaseByTarget[deploymentReleaseKey(editingTarget.id)] : undefined
  const normalizedTargetValues = normalizeDeploymentTargetPayload(watchedTargetValues)
  const targetHasRuntimeChanges = editingTarget ? deploymentTargetRuntimeChanged(editingTarget, normalizedTargetValues) : false
  const targetHasRunningInstances = Boolean(editingTarget && deploymentTargetHasRunningInstances(editingTarget))
  const targetCanRedeploy = Boolean(
    editingTarget
    && targetHasRunningInstances
    && normalizedTargetValues.enabled
    && (normalizedTargetValues.sourceType === 'image' ? normalizedTargetValues.imageRef.trim() : latestEditingTargetRelease?.imageRef.trim()),
  )
  useEffect(() => {
    if (!targetDialogOpen || editingTarget || targetSourceType !== 'repository' || targetImageRefDirty)
      return
    const nextImageRef = targetImageTemplateDefault.data?.targetImageRef
    if (!nextImageRef)
      return
    targetForm.setValue('targetImageRef', nextImageRef, { shouldDirty: false, shouldValidate: true })
  }, [editingTarget, targetDialogOpen, targetForm, targetImageRefDirty, targetImageTemplateDefault.data?.targetImageRef, targetSourceType])
  const copyDeploymentText = (value?: string) => {
    const text = value?.trim()
    if (!text || text === '-')
      return
    navigator.clipboard.writeText(text)
      .then(() => toast.success(t('common.copied')))
      .catch(error => toast.error(error.message))
  }
  const runtimeConfigSets = useQuery({
    queryKey: ['runtime-config-sets', projectId],
    queryFn: () => api.listProjectRuntimeConfigSets(projectId),
    enabled: Boolean(projectId),
  })
  const projectHooks = useQuery({
    queryKey: ['project-hooks', projectId],
    queryFn: () => api.listProjectHooks(projectId),
    enabled: Boolean(projectId && targetDialogOpen),
  })
  const workloadClusterIds = useMemo(() => {
    const ids = new Set<string>()
    for (const target of deploymentTargets) {
      const clusterId = target.clusterId?.trim() || defaultRuntimeCluster?.id
      if (clusterId)
        ids.add(clusterId)
    }
    return [...ids].sort()
  }, [defaultRuntimeCluster?.id, deploymentTargets])
  const workloadResourceQueries = useQueries({
    queries: workloadClusterIds.map(clusterId => ({
      enabled: Boolean(projectId && applicationId && clusterId),
      queryFn: () => api.listRuntimeClusterResources(clusterId, { kind: 'workloads', projectId, applicationId }),
      queryKey: ['runtime-cluster-resources', clusterId, 'workloads', projectId, applicationId],
      refetchInterval: WORKFLOW_STATUS_REFETCH_INTERVAL_MS,
      ...liveObservationQueryPolicy,
    })),
  })
  const serviceResourceQueries = useQueries({
    queries: workloadClusterIds.map(clusterId => ({
      enabled: Boolean(projectId && applicationId && clusterId),
      queryFn: () => api.listRuntimeClusterResources(clusterId, { kind: 'services', projectId, applicationId }),
      queryKey: ['runtime-cluster-resources', clusterId, 'services', projectId, applicationId],
      refetchInterval: WORKFLOW_STATUS_REFETCH_INTERVAL_MS,
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
  }), [defaultRuntimeCluster, deploymentTargets, latestReleaseByTarget, projectWebConsoleEnabled, runtimeClusterMap, serviceResourcesByCluster, workloadErrorByCluster, workloadLoadingByCluster, workloadResourcesByCluster])
  const runtimeConfigRestartTargets = useMemo(() => {
    if (!runtimeConfigRestartSetId)
      return []
    return deploymentTargets.filter((target) => {
      const refs = normalizeRuntimeConfigRefs(target.runtimeConfigRefs, target.runtimeConfigSetIds)
      return runtimeConfigLiveSetIds(refs).includes(runtimeConfigRestartSetId)
    })
  }, [deploymentTargets, runtimeConfigRestartSetId])
  const runtimeConfigRedeployableTargets = useMemo(() => runtimeConfigRestartTargets.filter((target) => {
    const latestRelease = latestReleaseByTarget[deploymentReleaseKey(target.id)]
    return Boolean(redeployReleasePayload(target, latestRelease))
  }), [latestReleaseByTarget, runtimeConfigRestartTargets])
  const openTargetDialog = (target?: DeploymentTarget) => {
    setRuntimeConfigRestartSetId('')
    setRuntimeConfigRestartAffectedCount(0)
    openTargetFormDialog(target)
  }
  const targetStageLabel = t(`deploymentsPage.stageLabels.${watchedTargetValues.stage}`)
  const targetSourceLabel = t(targetSourceType === 'image' ? 'apps.image' : 'apps.repository')
  const targetPrimaryPort = targetServicePorts[0] ?? { name: 'http', port: 8080 }
  const targetPortSummary = targetServicePorts.length > 1
    ? t('deploymentsPage.progressivePortSummary', {
        count: targetServicePorts.length - 1,
        name: targetPrimaryPort.name || 'http',
        port: targetPrimaryPort.port || 8080,
      })
    : t('deploymentsPage.progressiveSinglePortSummary', {
        name: targetPrimaryPort.name || 'http',
        port: targetPrimaryPort.port || 8080,
      })
  const targetBasicSummary = t('deploymentsPage.progressiveBasicSummary', {
    port: targetPortSummary,
    source: targetSourceLabel,
    stage: targetStageLabel,
  })
  const targetBuildSummary = targetSourceType === 'image'
    ? t('deploymentsPage.progressiveBuildSkippedSummary')
    : t('deploymentsPage.progressiveBuildSummary', {
        context: watchedTargetValues.buildContext || '.',
        cpu: watchedTargetValues.buildCpuRequest || defaultBuildCpuRequest,
        dockerfile: watchedTargetValues.dockerfilePath || 'Dockerfile',
        memory: watchedTargetValues.buildMemoryRequest || defaultBuildMemoryRequest,
        args: buildArgLineCount(watchedTargetValues.buildArgs),
        timeout: buildTimeoutMinutes,
      })
  const targetRuntimeSummary = t('deploymentsPage.progressiveRuntimeSummary', {
    cpu: watchedTargetValues.cpuRequest || '1',
    memory: watchedTargetValues.memoryRequest || '1Gi',
    replicas: watchedTargetValues.replicas ?? 1,
  })
  const targetPolicySummary = t('deploymentsPage.progressivePolicySummary', {
    autoDeploy: t(normalizeBoolean(watchedTargetValues.autoDeploy, true) ? 'common.enabled' : 'common.disabled'),
    concurrency: t(`apps.buildConcurrencyPolicies.${watchedTargetValues.concurrencyPolicy || 'queue'}`),
  })
  const targetHooksSummary = targetBuildHooksEnabled
    ? t('deploymentsPage.progressiveHooksSummary', { count: selectedDeploymentHookBindings.length })
    : t('deploymentsPage.progressiveHooksDisabledSummary')
  const targetDataSummary = targetHasDataVolumes
    ? t('deploymentsPage.progressiveDataEnabledSummary', { count: targetDataVolumes.length })
    : t('deploymentsPage.progressiveDataDisabledSummary')
  const targetHasAdvancedConfig = Boolean(
    watchedTargetValues.environmentVariables.length > 0
    || Object.keys(watchedTargetValues.configRefs ?? {}).length > 0
    || String(watchedTargetValues.configFiles ?? '').trim()
    || String(watchedTargetValues.secretFiles ?? '').trim()
    || runtimeSecretKeys(editingTarget?.environmentVariables).length > 0
    || editingTarget?.secretFilesSet,
  )
  const targetConfigSummary = t('deploymentsPage.progressiveConfigSummary', {
    count: runtimeConfigRefIds(selectedRuntimeConfigRefs).length,
    overrides: t(targetHasAdvancedConfig ? 'deploymentsPage.advancedOverridesEnabled' : 'deploymentsPage.advancedOverridesDisabled'),
  })
  const targetKubernetesAdvancedCount = [
    'cpuLimit',
    'memoryLimit',
    'imagePullPolicy',
    'containerCommand',
    'containerArgs',
    'lifecycle',
    'initContainers',
    'sidecarContainers',
    'readinessProbe',
    'livenessProbe',
    'startupProbe',
    'runAsUser',
    'runAsGroup',
    'fsGroup',
    'fsGroupChangePolicy',
    'allowPrivilegeEscalation',
    'capabilityAdd',
    'capabilityDrop',
    'nodeSelector',
    'tolerations',
    'affinity',
    'topologySpreadConstraints',
    'priorityClassName',
    'serviceType',
    'serviceAnnotations',
    'serviceExternalTrafficPolicy',
    'serviceSessionAffinity',
    'autoScalingBehavior',
    'autoScalingMinReplicas',
    'autoScalingMaxReplicas',
    'autoScalingCpuPercent',
    'autoScalingMemoryPercent',
    'webConsoleEnabled',
  ].filter(key => String(watchedTargetValues[key as keyof DeploymentTargetPayload] ?? '').trim()).length
  + (watchedTargetValues.workloadType === 'StatefulSet' ? 1 : 0)
  + (normalizeBoolean(watchedTargetValues.readOnlyRootFilesystem, false) ? 1 : 0)
  + (normalizeBoolean(watchedTargetValues.autoScalingEnabled, false) ? 1 : 0)
  const targetKubernetesAdvancedSummary = targetKubernetesAdvancedCount > 0
    ? t('deploymentsPage.progressiveKubernetesAdvancedEnabledSummary', { count: targetKubernetesAdvancedCount })
    : t('deploymentsPage.progressiveKubernetesAdvancedDisabledSummary')
  const openRuntimeConfigDialog = (set?: ProjectRuntimeConfigSet) => {
    setEditingRuntimeConfigSet(set ?? null)
    setRuntimeConfigFilesValid(true)
    setRuntimeSecretFilesValid(true)
    runtimeConfigForm.reset(set
      ? {
          configFiles: set.configFiles,
          enabled: set.enabled,
          environmentVariables: publicRuntimeEnvironmentInputs(publicRuntimeEnvironmentRecord(set.environmentVariables)),
          name: set.name,
          secretFiles: '',
        }
      : runtimeConfigDefaults)
    setRuntimeConfigDialogOpen(true)
  }
  const resetRepositoryBindingForm = () => {
    repositoryBindingForm.reset(repositoryBindingDefaults)
    setRepositoryBranchSearch('')
  }
  const openRepositoryBindingDialog = () => {
    resetRepositoryBindingForm()
    setRepositoryBindingDialogOpen(true)
  }
  const openReleaseDialog = (_environmentId = '', deploymentTargetId = '') => {
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
      environmentId: defaultTarget?.environmentId ?? '',
      imageRef: matchedRun ? buildRunImageRef(matchedRun) : defaultTarget?.imageRef ?? '',
    })
    setDialogOpen(true)
  }
  useImperativeHandle(ref, () => ({ openImportDialog: () => setBundleImportOpen(true), openReleaseDialog, openTargetDialog: () => openTargetDialog() }))
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
    form.setValue('environmentId', selectedReleaseTarget.environmentId, { shouldDirty: true, shouldValidate: true })
    form.setValue('applicationId', applicationId, { shouldDirty: true, shouldValidate: true })
    if (selectedReleaseTarget.sourceType === 'image')
      form.setValue('imageRef', selectedReleaseTarget.imageRef, { shouldDirty: true, shouldValidate: true })
  }, [applicationId, form, selectedBuildRun, selectedReleaseTarget])
  useEffect(() => {
    if (!targetDialogOpen || editingTarget || targetSourceType !== 'repository')
      return
    const dockerfilePath = dockerfileSuggestions[0]
    if (!dockerfilePath)
      return
    const currentDockerfile = targetForm.getValues('dockerfilePath')?.trim()
    if (currentDockerfile && currentDockerfile !== 'Dockerfile')
      return
    applyDockerfileBuildDefaults(targetForm, dockerfilePath, buildContextSuggestions, dockerfileExposedPorts)
  }, [buildContextSuggestions, dockerfileExposedPorts, dockerfileSuggestions, editingTarget, targetDialogOpen, targetForm, targetSourceType])
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
  const saveRuntimeConfigSet = useMutation({
    mutationFn: (values: ProjectRuntimeConfigSetPayload) => editingRuntimeConfigSet
      ? api.updateProjectRuntimeConfigSet(projectId, editingRuntimeConfigSet.id, normalizeRuntimeConfigPayload(values))
      : api.createProjectRuntimeConfigSet(projectId, normalizeRuntimeConfigPayload(values)),
    onSuccess: (set) => {
      toast.success(t(editingRuntimeConfigSet ? 'runtimeConfigSets.updated' : 'runtimeConfigSets.created'))
      if (!editingRuntimeConfigSet) {
        toggleRuntimeConfigSet(set.id, true)
      }
      else if ((set.affectedDeploymentTargetCount ?? 0) > 0) {
        setRuntimeConfigRestartSetId(set.id)
        setRuntimeConfigRestartAffectedCount(set.affectedDeploymentTargetCount ?? 0)
      }
      setRuntimeConfigDialogOpen(false)
      setEditingRuntimeConfigSet(null)
      runtimeConfigForm.reset(runtimeConfigDefaults)
      queryClient.invalidateQueries({ queryKey: ['runtime-config-sets', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  const redeployRuntimeConfigTargets = useMutation({
    mutationFn: async () => {
      let queued = 0
      let skipped = 0
      for (const target of runtimeConfigRestartTargets) {
        const releasePayload = redeployReleasePayload(target, latestReleaseByTarget[deploymentReleaseKey(target.id)])
        if (!releasePayload) {
          skipped++
          continue
        }
        await api.createRelease(projectId, releasePayload)
        queued++
      }
      return { queued, skipped }
    },
    onSuccess: ({ queued, skipped }) => {
      toast.success(t('deploymentsPage.runtimeConfigRedeployQueued', { queued, skipped }))
      setRuntimeConfigRestartSetId('')
      setRuntimeConfigRestartAffectedCount(0)
      queryClient.invalidateQueries({ queryKey: ['releases', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  const createRepositoryBinding = useMutation({
    mutationFn: (values: RepositoryBindingForm) => api.createRepositoryBinding(projectId, {
      applicationId,
      autoConfigureWebhook: values.autoConfigureWebhook,
      cloneUrl: values.cloneUrl ?? '',
      defaultBranch: values.defaultBranch || 'main',
      gitAccountId: values.gitAccountId,
      owner: values.owner,
      repo: values.repo,
    }),
    onSuccess: (binding) => {
      toast.success(t('repositories.bindingSaved'))
      queryClient.setQueryData<RepositoryBinding[]>(['repository-bindings', projectId, applicationId], items => [
        ...repositoryBindingItems(items).filter(item => item.id !== binding.id),
        binding,
      ])
      targetForm.setValue('repositoryBindingId', binding.id, { shouldDirty: true, shouldValidate: true })
      setRepositoryBindingDialogOpen(false)
      resetRepositoryBindingForm()
      queryClient.invalidateQueries({ queryKey: ['repository-bindings', projectId] })
      queryClient.invalidateQueries({ queryKey: ['applications', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  const saveTarget = useMutation({
    mutationFn: async ({ redeploy, values }: { redeploy: boolean, values: DeploymentTargetPayload }) => {
      const payload = buildSubmissionPayload(values)
      const savedTarget = editingTarget
        ? api.updateDeploymentTarget(projectId, applicationId, editingTarget.id, payload)
        : api.createDeploymentTarget(projectId, applicationId, payload)
      const target = await savedTarget
      if (!redeploy)
        return { redeploy, target }
      const releasePayload = redeployReleasePayload(target, latestEditingTargetRelease)
      if (!releasePayload)
        throw new Error(t('deploymentsPage.redeployUnavailable'))
      await api.createRelease(projectId, releasePayload)
      return { redeploy, target }
    },
    onSuccess: ({ redeploy }) => {
      toast.success(t(redeploy ? 'deploymentsPage.targetUpdatedAndRedeployQueued' : editingTarget ? 'deploymentsPage.targetUpdated' : 'deploymentsPage.targetCreated'))
      handleTargetDialogOpenChange(false)
      queryClient.invalidateQueries({ queryKey: ['deployment-targets', projectId, applicationId] })
      if (redeploy)
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
        onCopy={copyDeploymentText}
        onDeleteTarget={setTargetToDelete}
        onOpenConsole={setConsoleRelease}
        onOpenReleaseDialog={openReleaseDialog}
        onOpenTargetDialog={openTargetDialog}
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
      <DeferredDeploymentTargetDialog
        applicationId={applicationId}
        projectId={projectId}
        buildContextSuggestions={buildContextSuggestions}
        buildMinutePriceText={billingDisplay.formatAmountWithUnit(buildMinuteCost)}
        buildTemplates={buildTemplates.data ?? []}
        buildTimeoutMinutes={buildTimeoutMinutes}
        buildEnvironmentStatus={targetBuildEnvironmentStatus}
        buildSecretRows={targetBuildSecretRows}
        defaultRuntimeCluster={defaultRuntimeCluster}
        dockerfileExposedPorts={dockerfileExposedPorts}
        dockerfileSuggestions={dockerfileSuggestions}
        editingTarget={editingTarget}
        form={targetForm}
        hooks={projectHooks.data ?? []}
        hooksError={projectHooks.isError}
        hooksLoading={projectHooks.isLoading}
        open={targetDialogOpen}
        registries={registries}
        repositoryBindings={repositoryBindings}
        recommendedTemplateIds={targetBuildOptions.data?.recommendedTemplateIds ?? []}
        runtimeClusters={runtimeClusters.data ?? []}
        runtimeConfigRedeployableCount={runtimeConfigRedeployableTargets.length}
        runtimeConfigRedeployPending={redeployRuntimeConfigTargets.isPending}
        runtimeConfigRestartAffectedCount={runtimeConfigRestartAffectedCount}
        runtimeConfigSets={runtimeConfigSets.data ?? []}
        runtimeCostText={billingDisplay.formatAmountWithUnit(runtimeHourCost)}
        savePending={saveTarget.isPending}
        selectedHookBindings={selectedDeploymentHookBindings}
        selectedRuntimeConfigRefs={selectedRuntimeConfigRefs}
        servicePorts={targetServicePorts}
        sourceType={targetSourceType}
        summaries={{
          basic: targetBasicSummary,
          build: targetBuildSummary,
          config: targetConfigSummary,
          data: targetDataSummary,
          hooks: targetHooksSummary,
          kubernetesAdvanced: targetKubernetesAdvancedSummary,
          policy: targetPolicySummary,
          runtime: targetRuntimeSummary,
        }}
        targetBuildHooksEnabled={targetBuildHooksEnabled}
        buildVariableRows={targetBuildVariableRows}
        canManageRuntimeSecrets={canManageRuntimeSecrets}
        targetBuildOptionsError={targetBuildOptions.isError}
        targetBuildOptionsFetching={targetBuildOptions.isFetching}
        targetCanRedeploy={targetCanRedeploy}
        targetConfigFilesValid={targetConfigFilesValid}
        targetHasDataVolumes={targetHasDataVolumes}
        targetDataVolumes={targetDataVolumes}
        targetHasRuntimeChanges={targetHasRuntimeChanges}
        targetHasRunningInstances={targetHasRunningInstances}
        targetImagePrefix={targetImagePrefix}
        targetRuntimeFilesValid={targetRuntimeFilesValid}
        targetSecretFilesValid={targetSecretFilesValid}
        onBindRepository={openRepositoryBindingDialog}
        onChangeRuntimeConfigMode={changeRuntimeConfigRefMode}
        onDismissRuntimeConfigRestart={() => {
          setRuntimeConfigRestartSetId('')
          setRuntimeConfigRestartAffectedCount(0)
        }}
        onEditRuntimeConfigSet={openRuntimeConfigDialog}
        onOpenChange={handleTargetDialogOpenChange}
        onRedeployRuntimeConfigTargets={() => redeployRuntimeConfigTargets.mutate()}
        onSave={(values, redeploy) => saveTarget.mutate({ redeploy, values })}
        onSetConfigFilesValid={setTargetConfigFilesValid}
        onSetBuildSecretRows={setTargetBuildSecretRows}
        onSetBuildVariableRows={setTargetBuildVariableRows}
        onSetHookBindings={setTargetHookBindings}
        onSetSecretFilesValid={setTargetSecretFilesValid}
        onToggleRuntimeConfigSet={toggleRuntimeConfigSet}
        onUpdateDataVolumes={updateTargetDataVolumes}
        onUpdateServicePorts={updateTargetServicePorts}
      />
      <DeferredRepositoryBindingDialog
        accounts={gitAccounts.data ?? []}
        branchLimited={repositoryBranches.data?.limited}
        branches={repositoryBranches.data?.items ?? []}
        branchSearch={repositoryBranchSearch}
        branchesLoading={repositoryBranches.isFetching}
        form={repositoryBindingForm}
        open={repositoryBindingDialogOpen}
        pending={createRepositoryBinding.isPending}
        providers={gitProviders.data ?? []}
        onBranchSearchChange={setRepositoryBranchSearch}
        onOpenChange={(open) => {
          setRepositoryBindingDialogOpen(open)
          if (!open)
            resetRepositoryBindingForm()
        }}
        onSubmit={values => createRepositoryBinding.mutate(values)}
      />
      <DeferredRuntimeConfigSetDialog
        editingSet={editingRuntimeConfigSet}
        filesValid={runtimeConfigFilesValid}
        form={runtimeConfigForm}
        open={runtimeConfigDialogOpen}
        projectId={projectId}
        canManageRuntimeSecrets={canManageRuntimeSecrets}
        pending={saveRuntimeConfigSet.isPending}
        secretFilesValid={runtimeSecretFilesValid}
        setFilesValid={setRuntimeConfigFilesValid}
        setSecretFilesValid={setRuntimeSecretFilesValid}
        onOpenChange={(open) => {
          setRuntimeConfigDialogOpen(open)
          if (!open)
            setEditingRuntimeConfigSet(null)
        }}
        onSubmit={values => saveRuntimeConfigSet.mutate(values)}
      />
      <DeferredReleaseLogsDialog
        logView={logView}
        projectId={projectId}
        release={logRelease}
        setLogView={setLogView}
        onOpenChange={open => !open && setLogRelease(null)}
      />
      <DeferredWebConsoleDialog
        projectId={projectId}
        release={consoleRelease}
        onOpenChange={open => !open && setConsoleRelease(null)}
      />
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
