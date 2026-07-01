import type { ReleaseForm } from './application-deployments-panel-utils'
import type { RepositoryBindingDialogForm, RepositoryBindingDialogFormInput } from './application-repository-binding-dialog'
import type { ArtifactRegistry, BuildRun, DeploymentRuntimeConfigRef, DeploymentTarget, DeploymentTargetPayload, ProjectRuntimeConfigSet, ProjectRuntimeConfigSetPayload, Release, RepositoryBinding, RuntimeConfigRefMode } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { Rocket, Save } from 'lucide-react'
import { useEffect, useImperativeHandle, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { buildRunImageRef, latestDeployableBuildRuns } from '@/components/common/deployment-build-runs'
import { FormField as Field } from '@/components/common/form-field'
import { ProgressiveSection } from '@/components/common/progressive-section'
import { RuntimeConfigFilesEditor } from '@/components/common/runtime-config-files-editor'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { useBillingDisplay } from '@/lib/billing-display'
import { WORKFLOW_STATUS_REFETCH_INTERVAL_MS } from '@/lib/polling'
import { defaultBuildCpuRequest, defaultBuildMemoryRequest, defaultBuildTimeoutSeconds } from './application-build-defaults'
import { defaultTargetImageRef, deploymentReleaseKey, deploymentTargetCanRelease, deploymentTargetImageRef, registryInputPrefix } from './application-config-utils'
import { ApplicationCreateReleaseDialog } from './application-create-release-dialog'
import { RuntimeDataVolumesEditor } from './application-deployment-data-volumes-editor'
import { RuntimeResourceFields } from './application-deployment-resource-fields'
import { buildDeploymentRuntimeStatus, buildInternalServiceEndpoint } from './application-deployment-runtime-utils'
import { ServicePortsEditor } from './application-deployment-service-ports-editor'
import { ApplicationDeploymentBuildSettingsFields, ApplicationDeploymentSourceFields } from './application-deployment-source-fields'
import { ApplicationDeploymentTargetsList } from './application-deployment-targets-list'
import { applyDockerfileBuildDefaults, deploymentTargetDefaults, deploymentTargetRuntimeChanged, normalizeBoolean, normalizeDeploymentTargetPayload, normalizeRuntimeConfigPayload, normalizeRuntimeConfigRefs, normalizeStringIds, parseRuntimeDataVolumes, redeployReleasePayload, releaseDefaults, repositoryBindingItems, runtimeConfigDefaults, runtimeConfigLiveSetIds, runtimeConfigRefIds, serializeRuntimeDataVolumes } from './application-deployments-panel-utils'
import { ApplicationReleaseLogsDialog } from './application-release-logs-dialog'
import { ApplicationRepositoryBindingDialog } from './application-repository-binding-dialog'
import { ApplicationRuntimeConfigSelector } from './application-runtime-config-selector'
import { ApplicationRuntimeConfigSetDialog } from './application-runtime-config-set-dialog'
import { ApplicationWebConsoleDialog } from './application-web-console-dialog'

export interface DeploymentsPanelHandle {
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
  webhookStatus: z.enum(['pending', 'created', 'disabled', 'failed']),
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
  webhookStatus: 'pending',
}

function upsertRuntimeConfigRef(refs: DeploymentRuntimeConfigRef[], nextRef: DeploymentRuntimeConfigRef) {
  const next = normalizeRuntimeConfigRefs(refs).filter(ref => ref.setId !== nextRef.setId)
  return [...next, nextRef]
}

export function ApplicationDeploymentsPanel({ applicationId, appSlug, buildRuns, deploymentTargets, projectId, projectSlug, ref, registries, releases, repositoryBindings }: {
  applicationId: string
  appSlug: string
  buildRuns: BuildRun[]
  deploymentTargets: DeploymentTarget[]
  projectId: string
  projectSlug: string
  ref?: React.Ref<DeploymentsPanelHandle>
  registries: ArtifactRegistry[]
  repositoryBindings: RepositoryBinding[]
  releases: Release[]
}) {
  const { i18n, t } = useTranslation()
  const queryClient = useQueryClient()
  const billingDisplay = useBillingDisplay(i18n.language)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [targetDialogOpen, setTargetDialogOpen] = useState(false)
  const [editingTarget, setEditingTarget] = useState<DeploymentTarget | null>(null)
  const [targetConfigFilesValid, setTargetConfigFilesValid] = useState(true)
  const [targetSecretFilesValid, setTargetSecretFilesValid] = useState(true)
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
  const targetForm = useForm<DeploymentTargetPayload>({ defaultValues: deploymentTargetDefaults, mode: 'onChange' })
  const runtimeConfigForm = useForm<ProjectRuntimeConfigSetPayload>({ defaultValues: runtimeConfigDefaults, mode: 'onChange' })
  const repositoryBindingForm = useForm<RepositoryBindingFormInput, undefined, RepositoryBindingForm>({
    defaultValues: repositoryBindingDefaults,
    mode: 'onChange',
    resolver: zodResolver(repositoryBindingSchema),
  })
  const runtimeHourCost = billingDisplay.runtimeHourCost(targetForm.watch('replicas'), targetForm.watch('cpuRequest'), targetForm.watch('memoryRequest'))
  const buildMinuteCost = billingDisplay.buildMinuteCost(targetForm.watch('buildCpuRequest'), targetForm.watch('buildMemoryRequest'))
  const buildTimeoutMinutes = Math.max(1, Math.round((Number(targetForm.watch('buildTimeoutSeconds')) || defaultBuildTimeoutSeconds) / 60))
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
  const targetSourceType = targetForm.watch('sourceType')
  const targetRepositoryBindingId = targetForm.watch('repositoryBindingId')
  const targetRegistryId = targetForm.watch('targetRegistryId')
  const targetStage = targetForm.watch('stage')
  const targetName = targetForm.watch('name')
  const targetDataRetentionEnabled = normalizeBoolean(targetForm.watch('dataRetentionEnabled'), false)
  const targetDataVolumesValue = targetForm.watch('dataVolumes')
  const targetDataVolumes = useMemo(
    () => parseRuntimeDataVolumes(targetDataVolumesValue, targetForm.getValues('dataMountPath') || '/data', targetForm.getValues('dataCapacity') || '1Gi'),
    [targetDataVolumesValue, targetForm],
  )
  const watchedTargetValues = targetForm.watch()
  const targetImageRefDirty = Boolean(targetForm.formState.dirtyFields.targetImageRef)
  const selectedRuntimeConfigRefs = normalizeRuntimeConfigRefs(targetForm.watch('runtimeConfigRefs'), targetForm.watch('runtimeConfigSetIds'))
  const selectedTargetRepositoryBinding = repositoryBindings.find(binding => binding.id === targetRepositoryBindingId)
  const targetRegistry = registries.find(registry => registry.id === targetRegistryId)
  const targetImagePrefix = targetRegistry ? registryInputPrefix(targetRegistry) : ''
  const gitProviders = useQuery({ queryKey: ['git-providers'], queryFn: () => api.listGitProviders(), enabled: repositoryBindingDialogOpen })
  const gitAccounts = useQuery({ queryKey: ['git-accounts'], queryFn: () => api.listGitAccounts(), enabled: repositoryBindingDialogOpen })
  const selectedRepositoryAccountId = repositoryBindingForm.watch('gitAccountId')
  const selectedRepositoryOwner = repositoryBindingForm.watch('owner')
  const selectedRepositoryName = repositoryBindingForm.watch('repo')
  const repositoryBranches = useQuery({
    queryKey: ['git-branches', selectedRepositoryAccountId, selectedRepositoryOwner, selectedRepositoryName, repositoryBranchSearch],
    queryFn: () => api.listGitBranches(selectedRepositoryAccountId || '', selectedRepositoryOwner || '', selectedRepositoryName || '', { search: repositoryBranchSearch, limit: 50 }),
    enabled: Boolean(repositoryBindingDialogOpen && selectedRepositoryAccountId && selectedRepositoryOwner && selectedRepositoryName),
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
  const targetHasRuntimeChanges = editingTarget ? deploymentTargetRuntimeChanged(editingTarget, normalizeDeploymentTargetPayload(watchedTargetValues)) : false
  const targetCanRedeploy = Boolean(editingTarget && latestEditingTargetRelease && normalizeBoolean(watchedTargetValues.enabled, editingTarget.enabled))
  const targetRuntimeFilesValid = targetConfigFilesValid && targetSecretFilesValid
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
  const runtimeClusters = useQuery({
    queryKey: ['runtime-clusters', projectId],
    queryFn: () => api.listRuntimeClusters(projectId),
    enabled: Boolean(projectId),
  })
  const runtimeClusterMap = useMemo(() => Object.fromEntries((runtimeClusters.data ?? []).map(cluster => [cluster.id, cluster])), [runtimeClusters.data])
  const defaultRuntimeCluster = useMemo(() => {
    const clusters = runtimeClusters.data ?? []
    return clusters.find(cluster => cluster.isDefault) ?? clusters[0]
  }, [runtimeClusters.data])
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
    })),
  })
  const serviceResourceQueries = useQueries({
    queries: workloadClusterIds.map(clusterId => ({
      enabled: Boolean(projectId && applicationId && clusterId),
      queryFn: () => api.listRuntimeClusterResources(clusterId, { kind: 'services', projectId, applicationId }),
      queryKey: ['runtime-cluster-resources', clusterId, 'services', projectId, applicationId],
      refetchInterval: WORKFLOW_STATUS_REFETCH_INTERVAL_MS,
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
    }
  }), [defaultRuntimeCluster, deploymentTargets, latestReleaseByTarget, runtimeClusterMap, serviceResourcesByCluster, workloadErrorByCluster, workloadLoadingByCluster, workloadResourcesByCluster])
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
  const resetTargetForm = (target?: DeploymentTarget | null) => {
    const defaultRegistry = registries.find(registry => registry.credentialSet && registry.isDefault) ?? registries.find(registry => registry.credentialSet) ?? registries.find(registry => registry.isDefault) ?? registries[0]
    const defaultBinding = repositoryBindings[0]
    const sourceType = target?.sourceType ?? 'repository'
    targetForm.reset({
      ...deploymentTargetDefaults,
      ...target,
      sourceType,
      environmentId: target?.environmentId ?? '',
      clusterId: target?.clusterId ?? defaultRuntimeCluster?.id ?? '',
      replicas: target?.replicas ?? 1,
      cpuRequest: target?.cpuRequest || '1',
      memoryRequest: target?.memoryRequest || '1Gi',
      stage: target?.stage || 'prod',
      buildEnvironmentId: target?.buildEnvironmentId || '',
      buildCpuRequest: target?.buildCpuRequest || defaultBuildCpuRequest,
      buildMemoryRequest: target?.buildMemoryRequest || defaultBuildMemoryRequest,
      buildTimeoutSeconds: target?.buildTimeoutSeconds || defaultBuildTimeoutSeconds,
      repositoryBindingId: target?.repositoryBindingId ?? defaultBinding?.id ?? '',
      targetRegistryId: target?.targetRegistryId ?? defaultRegistry?.id ?? '',
      targetImageRef: deploymentTargetImageRef(target ?? undefined) || defaultTargetImageRef(defaultRegistry, projectSlug, appSlug),
      buildHooksEnabled: target?.buildHooksEnabled ?? true,
      buildHookBindings: target?.buildHookBindings ?? [],
      servicePort: target?.servicePort ?? 8080,
      servicePorts: target?.servicePorts?.length ? target.servicePorts : [{ name: 'http', port: target?.servicePort ?? 8080 }],
      buildVariableSetIds: normalizeStringIds(target?.buildVariableSetIds),
      runtimeConfigRefs: normalizeRuntimeConfigRefs(target?.runtimeConfigRefs, target?.runtimeConfigSetIds),
      runtimeConfigSetIds: runtimeConfigLiveSetIds(normalizeRuntimeConfigRefs(target?.runtimeConfigRefs, target?.runtimeConfigSetIds)),
      secretRefs: '',
      secretFiles: '',
      dataRetentionEnabled: target?.dataRetentionEnabled ?? false,
      dataCapacity: target?.dataCapacity || '1Gi',
      dataMountPath: target?.dataMountPath || '/data',
      dataVolumes: target?.dataVolumes || serializeRuntimeDataVolumes(parseRuntimeDataVolumes('', target?.dataMountPath || '/data', target?.dataCapacity || '1Gi')),
      enabled: target?.enabled ?? true,
    })
  }
  const setTargetRuntimeConfigRefs = (refs: DeploymentRuntimeConfigRef[]) => {
    const normalizedRefs = normalizeRuntimeConfigRefs(refs)
    targetForm.setValue('runtimeConfigRefs', normalizedRefs, { shouldDirty: true, shouldValidate: true })
    targetForm.setValue('runtimeConfigSetIds', runtimeConfigLiveSetIds(normalizedRefs), { shouldDirty: true, shouldValidate: true })
  }
  const openTargetDialog = (target?: DeploymentTarget) => {
    setEditingTarget(target ?? null)
    setTargetConfigFilesValid(true)
    setTargetSecretFilesValid(true)
    setRuntimeConfigRestartSetId('')
    setRuntimeConfigRestartAffectedCount(0)
    resetTargetForm(target)
    setTargetDialogOpen(true)
  }
  const toggleRuntimeConfigSet = (setId: string, checked: boolean) => {
    const current = normalizeRuntimeConfigRefs(targetForm.getValues('runtimeConfigRefs'), targetForm.getValues('runtimeConfigSetIds'))
    const next = checked
      ? upsertRuntimeConfigRef(current, { mode: 'live', setId })
      : current.filter(ref => ref.setId !== setId)
    setTargetRuntimeConfigRefs(next)
  }
  const changeRuntimeConfigRefMode = (setId: string, mode: RuntimeConfigRefMode) => {
    const current = normalizeRuntimeConfigRefs(targetForm.getValues('runtimeConfigRefs'), targetForm.getValues('runtimeConfigSetIds'))
    setTargetRuntimeConfigRefs(upsertRuntimeConfigRef(current, { mode, setId }))
  }
  const updateTargetDataVolumes = (rows: typeof targetDataVolumes) => {
    targetForm.setValue('dataVolumes', serializeRuntimeDataVolumes(rows), { shouldDirty: true, shouldValidate: true })
  }
  const targetServicePorts = targetForm.watch('servicePorts')?.length
    ? targetForm.watch('servicePorts')
    : [{ name: 'http', port: targetForm.watch('servicePort') || 8080 }]
  const updateTargetServicePorts = (rows: DeploymentTargetPayload['servicePorts']) => {
    const nextRows = rows.length > 0 ? rows : [{ name: 'http', port: 8080 }]
    targetForm.setValue('servicePorts', nextRows, { shouldDirty: true, shouldValidate: true })
    targetForm.setValue('servicePort', nextRows[0]?.port || 8080, { shouldDirty: true, shouldValidate: true })
  }
  const targetStageLabel = t(`deploymentsPage.stageLabels.${targetForm.watch('stage')}`)
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
        context: targetForm.watch('buildContext') || '.',
        cpu: targetForm.watch('buildCpuRequest') || defaultBuildCpuRequest,
        dockerfile: targetForm.watch('dockerfilePath') || 'Dockerfile',
        memory: targetForm.watch('buildMemoryRequest') || defaultBuildMemoryRequest,
        timeout: buildTimeoutMinutes,
      })
  const targetRuntimeSummary = t('deploymentsPage.progressiveRuntimeSummary', {
    cpu: targetForm.watch('cpuRequest') || '1',
    memory: targetForm.watch('memoryRequest') || '1Gi',
    replicas: targetForm.watch('replicas') || 1,
  })
  const targetPolicySummary = t('deploymentsPage.progressivePolicySummary', {
    autoDeploy: t(normalizeBoolean(targetForm.watch('autoDeploy'), true) ? 'common.enabled' : 'common.disabled'),
    concurrency: t(`apps.buildConcurrencyPolicies.${targetForm.watch('concurrencyPolicy') || 'queue'}`),
  })
  const targetDataSummary = targetDataRetentionEnabled
    ? t('deploymentsPage.progressiveDataEnabledSummary', { count: targetDataVolumes.length })
    : t('deploymentsPage.progressiveDataDisabledSummary')
  const targetHasAdvancedConfig = Boolean(
    String(targetForm.watch('envVars') ?? '').trim()
    || String(targetForm.watch('configRefs') ?? '').trim()
    || String(targetForm.watch('configFiles') ?? '').trim()
    || String(targetForm.watch('secretRefs') ?? '').trim()
    || String(targetForm.watch('secretFiles') ?? '').trim()
    || editingTarget?.secretRefsSet
    || editingTarget?.secretFilesSet,
  )
  const targetConfigSummary = t('deploymentsPage.progressiveConfigSummary', {
    count: runtimeConfigRefIds(selectedRuntimeConfigRefs).length,
    overrides: t(targetHasAdvancedConfig ? 'deploymentsPage.advancedOverridesEnabled' : 'deploymentsPage.advancedOverridesDisabled'),
  })
  const openRuntimeConfigDialog = (set?: ProjectRuntimeConfigSet) => {
    setEditingRuntimeConfigSet(set ?? null)
    setRuntimeConfigFilesValid(true)
    setRuntimeSecretFilesValid(true)
    runtimeConfigForm.reset(set
      ? {
          configFiles: set.configFiles,
          enabled: set.enabled,
          envVars: set.envVars,
          name: set.name,
          secretFiles: '',
          secretRefs: '',
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
  useImperativeHandle(ref, () => ({ openReleaseDialog, openTargetDialog: () => openTargetDialog() }))
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
      webhookStatus: values.webhookStatus,
    }),
    onSuccess: (binding) => {
      toast.success(t('repositories.bindingSaved'))
      queryClient.setQueryData<RepositoryBinding[]>(['repository-bindings', projectId], items => [
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
      const payload = normalizeDeploymentTargetPayload(values)
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
      setTargetDialogOpen(false)
      setEditingTarget(null)
      targetForm.reset(deploymentTargetDefaults)
      queryClient.invalidateQueries({ queryKey: ['deployment-targets', projectId, applicationId] })
      if (redeploy)
        queryClient.invalidateQueries({ queryKey: ['releases', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  return (
    <div className="grid gap-4">
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
      <ApplicationCreateReleaseDialog
        form={form}
        open={dialogOpen}
        pending={createRelease.isPending}
        releaseReadyTargets={releaseReadyTargets}
        selectableBuildRuns={selectableBuildRuns}
        selectedTarget={selectedReleaseTarget}
        onOpenChange={setDialogOpen}
        onSubmit={values => createRelease.mutate(values)}
      />
      <Dialog
        open={targetDialogOpen}
        onOpenChange={(open) => {
          setTargetDialogOpen(open)
          if (!open) {
            setEditingTarget(null)
            targetForm.reset(deploymentTargetDefaults)
          }
        }}
      >
        <DialogContent className="flex max-h-[90vh] max-w-4xl flex-col overflow-hidden p-0">
          <DialogHeader className="border-b border-border px-6 py-4">
            <DialogTitle>{editingTarget ? t('deploymentsPage.editDeploymentTarget') : t('deploymentsPage.createDeploymentTarget')}</DialogTitle>
            <DialogDescription>{t('deploymentsPage.deploymentTargetDialogDescription')}</DialogDescription>
          </DialogHeader>
          <form className="flex min-h-0 flex-1 flex-col" onSubmit={targetForm.handleSubmit(values => saveTarget.mutate({ redeploy: false, values }))}>
            <div className="grid gap-3 overflow-y-auto px-6 py-4 pb-6">
              <ProgressiveSection
                defaultOpen
                description={t('deploymentsPage.progressiveBasicDescription')}
                storageKey="liteyuki.deployments.targetDialog.basic"
                summary={targetBasicSummary}
                title={t('deploymentsPage.progressiveBasicTitle')}
              >
                <div className="grid gap-3 md:grid-cols-2">
                  <Field hint={t('deploymentsPage.deploymentConfigNameHint')} label={t('common.name')} required>
                    <Input {...targetForm.register('name', { required: true })} placeholder={t('deploymentsPage.deploymentConfigNamePattern')} />
                  </Field>
                  <Field label={t('deploymentsPage.stage')}>
                    <Select {...targetForm.register('stage')}>
                      <option value="dev">{t('deploymentsPage.stageDev')}</option>
                      <option value="test">{t('deploymentsPage.stageTest')}</option>
                      <option value="staging">{t('deploymentsPage.stageStaging')}</option>
                      <option value="prod">{t('deploymentsPage.stageProd')}</option>
                    </Select>
                  </Field>
                  <Field hint={t('deploymentsPage.runtimeEnvironmentHint')} label={t('clustersPage.runtimeCluster')}>
                    <Select {...targetForm.register('clusterId')}>
                      <option value="">{defaultRuntimeCluster ? t('deploymentsPage.clusterDefaultOption', { name: defaultRuntimeCluster.name }) : t('common.select')}</option>
                      {(runtimeClusters.data ?? []).map(cluster => <option key={cluster.id} value={cluster.id}>{cluster.name}</option>)}
                    </Select>
                  </Field>
                  <Field label={t('common.status')}>
                    <Select {...targetForm.register('enabled')}>
                      <option value="true">{t('common.enabled')}</option>
                      <option value="false">{t('common.disabled')}</option>
                    </Select>
                  </Field>
                  <ApplicationDeploymentSourceFields
                    registries={registries}
                    repositoryBindings={repositoryBindings}
                    sourceType={targetSourceType}
                    targetForm={targetForm}
                    onBindRepository={openRepositoryBindingDialog}
                  />
                  <div className="grid gap-2 md:col-span-2">
                    <ServicePortsEditor ports={targetServicePorts} onChange={updateTargetServicePorts} />
                  </div>
                </div>
              </ProgressiveSection>
              {targetSourceType === 'repository' && (
                <ProgressiveSection
                  description={t('deploymentsPage.progressiveBuildDescription')}
                  storageKey="liteyuki.deployments.targetDialog.build"
                  summary={targetBuildSummary}
                  title={t('deploymentsPage.progressiveBuildTitle')}
                >
                  <ApplicationDeploymentBuildSettingsFields
                    buildContextSuggestions={buildContextSuggestions}
                    buildMinutePriceText={billingDisplay.formatAmountWithUnit(buildMinuteCost)}
                    buildTimeoutMinutes={buildTimeoutMinutes}
                    dockerfileExposedPorts={dockerfileExposedPorts}
                    dockerfileSuggestions={dockerfileSuggestions}
                    sourceType={targetSourceType}
                    targetForm={targetForm}
                    targetImagePrefix={targetImagePrefix}
                    targetOptionsError={targetBuildOptions.isError}
                    targetOptionsFetching={targetBuildOptions.isFetching}
                  />
                </ProgressiveSection>
              )}
              <ProgressiveSection
                description={t('deploymentsPage.progressiveRuntimeDescription')}
                storageKey="liteyuki.deployments.targetDialog.runtime"
                summary={targetRuntimeSummary}
                title={t('deploymentsPage.progressiveRuntimeTitle')}
              >
                <RuntimeResourceFields form={targetForm} priceText={billingDisplay.formatAmountWithUnit(runtimeHourCost)} />
              </ProgressiveSection>
              <ProgressiveSection
                description={t('deploymentsPage.progressivePolicyDescription')}
                storageKey="liteyuki.deployments.targetDialog.policy"
                summary={targetPolicySummary}
                title={t('deploymentsPage.progressivePolicyTitle')}
              >
                <div className="grid gap-3 md:grid-cols-2">
                  <Field hint={t('deploymentsPage.branchPatternHint')} label={t('deploymentsPage.branchPattern')}>
                    <Input {...targetForm.register('branchPattern')} placeholder={t('deploymentsPage.branchPatternPlaceholder')} />
                  </Field>
                  <Field hint={t('deploymentsPage.tagPatternHint')} label={t('deploymentsPage.tagPattern')}>
                    <Input {...targetForm.register('tagPattern')} placeholder={t('deploymentsPage.tagPatternPlaceholder')} />
                  </Field>
                  <Field hint={t('apps.buildConcurrencyPolicyHint')} label={t('apps.buildConcurrencyPolicy')}>
                    <Select {...targetForm.register('concurrencyPolicy')}>
                      <option value="queue">{t('apps.buildConcurrencyPolicies.queue')}</option>
                      <option value="parallel">{t('apps.buildConcurrencyPolicies.parallel')}</option>
                    </Select>
                  </Field>
                  <Field label={t('deploymentsPage.autoDeploy')}>
                    <Select {...targetForm.register('autoDeploy')}>
                      <option value="false">{t('common.disabled')}</option>
                      <option value="true">{t('common.enabled')}</option>
                    </Select>
                  </Field>
                </div>
              </ProgressiveSection>
              <ProgressiveSection
                description={t('deploymentsPage.runtimeDataDescription')}
                storageKey="liteyuki.deployments.targetDialog.data"
                summary={targetDataSummary}
                title={t('deploymentsPage.runtimeData')}
              >
                <div className="grid gap-3">
                  <Field hint={t('deploymentsPage.dataRetentionHint')} label={t('deploymentsPage.dataRetention')}>
                    <Select {...targetForm.register('dataRetentionEnabled')}>
                      <option value="false">{t('common.disabled')}</option>
                      <option value="true">{t('common.enabled')}</option>
                    </Select>
                  </Field>
                  {targetDataRetentionEnabled && (
                    <RuntimeDataVolumesEditor enabled={targetDataRetentionEnabled} rows={targetDataVolumes} onChange={updateTargetDataVolumes} />
                  )}
                </div>
              </ProgressiveSection>
              <ProgressiveSection
                description={t('deploymentsPage.runtimeConfigDescription')}
                storageKey="liteyuki.deployments.targetDialog.config"
                summary={targetConfigSummary}
                title={t('deploymentsPage.runtimeConfig')}
              >
                <ApplicationRuntimeConfigSelector
                  redeployableCount={runtimeConfigRedeployableTargets.length}
                  redeployPending={redeployRuntimeConfigTargets.isPending}
                  restartAffectedCount={runtimeConfigRestartAffectedCount}
                  selectedRefs={selectedRuntimeConfigRefs}
                  sets={runtimeConfigSets.data ?? []}
                  onCreate={() => openRuntimeConfigDialog()}
                  onDismissRestart={() => {
                    setRuntimeConfigRestartSetId('')
                    setRuntimeConfigRestartAffectedCount(0)
                  }}
                  onEdit={openRuntimeConfigDialog}
                  onModeChange={changeRuntimeConfigRefMode}
                  onRedeployAffected={() => redeployRuntimeConfigTargets.mutate()}
                  onToggle={toggleRuntimeConfigSet}
                />
                <div className="grid gap-3 rounded-md border border-dashed border-border p-3">
                  <p className="text-sm font-medium text-foreground">{t('deploymentsPage.advancedRuntimeOverrides')}</p>
                  <Field hint={t('deploymentsPage.runtimeEnvVarsHint')} label={t('deploymentsPage.runtimeEnvVars')}>
                    <textarea className="min-h-24 rounded-md border border-input bg-background px-3 py-2 text-sm outline-none transition focus-visible:border-primary/60 focus-visible:ring-2 focus-visible:ring-primary/20" {...targetForm.register('envVars')} placeholder={t('deploymentsPage.runtimeEnvVarsPlaceholder')} />
                  </Field>
                  <Field hint={t('deploymentsPage.runtimeConfigRefsHint')} label={t('deploymentsPage.runtimeConfigRefs')}>
                    <textarea className="min-h-24 rounded-md border border-input bg-background px-3 py-2 text-sm outline-none transition focus-visible:border-primary/60 focus-visible:ring-2 focus-visible:ring-primary/20" {...targetForm.register('configRefs')} placeholder={t('deploymentsPage.runtimeConfigRefsPlaceholder')} />
                  </Field>
                  <Field hint={t('deploymentsPage.runtimeConfigFilesHint')} label={t('deploymentsPage.runtimeConfigFiles')}>
                    <RuntimeConfigFilesEditor
                      key={`${editingTarget?.id ?? 'new'}-config-files`}
                      initialValue={targetForm.getValues('configFiles') ?? ''}
                      onChange={value => targetForm.setValue('configFiles', value, { shouldDirty: true, shouldValidate: true })}
                      onValidationChange={setTargetConfigFilesValid}
                    />
                  </Field>
                  <Field hint={editingTarget?.secretRefsSet ? t('deploymentsPage.runtimeSecretRefsConfigured') : t('deploymentsPage.runtimeSecretRefsHint')} label={t('deploymentsPage.runtimeSecretRefs')}>
                    <textarea className="min-h-24 rounded-md border border-input bg-background px-3 py-2 text-sm outline-none transition focus-visible:border-primary/60 focus-visible:ring-2 focus-visible:ring-primary/20" {...targetForm.register('secretRefs')} placeholder={t('deploymentsPage.runtimeSecretRefsPlaceholder')} />
                  </Field>
                  <Field hint={editingTarget?.secretFilesSet ? t('deploymentsPage.runtimeSecretFilesConfigured') : t('deploymentsPage.runtimeSecretFilesHint')} label={t('deploymentsPage.runtimeSecretFiles')}>
                    <RuntimeConfigFilesEditor
                      key={`${editingTarget?.id ?? 'new'}-secret-files`}
                      initialValue={targetForm.getValues('secretFiles') ?? ''}
                      onChange={value => targetForm.setValue('secretFiles', value, { shouldDirty: true, shouldValidate: true })}
                      onValidationChange={setTargetSecretFilesValid}
                    />
                  </Field>
                </div>
              </ProgressiveSection>
              {targetHasRuntimeChanges && (
                <div className="flex gap-3 rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-amber-950 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-100">
                  <Rocket className="mt-0.5 size-4 shrink-0" />
                  <div className="grid gap-1 text-sm">
                    <p className="font-medium">{t('deploymentsPage.runtimeChangesNeedRedeployTitle')}</p>
                    <p className="text-amber-900/80 dark:text-amber-100/80">
                      {targetCanRedeploy ? t('deploymentsPage.runtimeChangesNeedRedeployDescription') : t('deploymentsPage.runtimeChangesNeedRedeployUnavailable')}
                    </p>
                  </div>
                </div>
              )}
            </div>
            <DialogFooter className="shrink-0 border-t border-border bg-background px-6 py-4">
              {targetHasRuntimeChanges && (
                <Button
                  disabled={!targetRuntimeFilesValid || !targetCanRedeploy || saveTarget.isPending}
                  type="button"
                  variant="secondary"
                  onClick={targetForm.handleSubmit(values => saveTarget.mutate({ redeploy: true, values }))}
                >
                  <Rocket className="size-4" />
                  {t('deploymentsPage.saveAndRedeploy')}
                </Button>
              )}
              <Button disabled={!targetRuntimeFilesValid || saveTarget.isPending} type="submit">
                <Save className="size-4" />
                {t('common.save')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <ApplicationRepositoryBindingDialog
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
      <ApplicationRuntimeConfigSetDialog
        editingSet={editingRuntimeConfigSet}
        filesValid={runtimeConfigFilesValid}
        form={runtimeConfigForm}
        open={runtimeConfigDialogOpen}
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
      <ApplicationReleaseLogsDialog
        logView={logView}
        projectId={projectId}
        release={logRelease}
        setLogView={setLogView}
        onOpenChange={open => !open && setLogRelease(null)}
      />
      <ApplicationWebConsoleDialog
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
