import type { ReleaseForm } from './application-deployments-panel-utils'
import type { ArtifactRegistry, BuildRun, DeploymentTarget, DeploymentTargetPayload, ProjectRuntimeConfigSet, ProjectRuntimeConfigSetPayload, Release, RepositoryBinding } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { FileCode2, Pencil, Plus, Rocket, Save, Trash2 } from 'lucide-react'
import { useEffect, useImperativeHandle, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { CheckboxField } from '@/components/common/checkbox-field'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { buildRunImageRef, buildRunOptionLabel, latestDeployableBuildRuns } from '@/components/common/deployment-build-runs'
import { FormField as Field } from '@/components/common/form-field'
import { GitRepositoryPicker } from '@/components/common/git-repository-picker'
import { RuntimeConfigFilesEditor } from '@/components/common/runtime-config-files-editor'
import { SearchSelect } from '@/components/common/search-select'
import { TargetImageRefInput } from '@/components/common/target-image-ref-input'
import { UnitInput } from '@/components/common/unit-input'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { useBillingDisplay } from '@/lib/billing-display'
import { WORKFLOW_STATUS_REFETCH_INTERVAL_MS } from '@/lib/polling'
import { branchOptions, defaultTargetImageRef, deploymentReleaseKey, deploymentTargetCanRelease, deploymentTargetImageRef, registryInputPrefix, registryOptionLabel } from './application-config-utils'
import { buildDeploymentRuntimeStatus, buildInternalServiceEndpoint } from './application-deployment-runtime-utils'
import { ApplicationDeploymentTargetsList } from './application-deployment-targets-list'
import { applyDockerfileBuildDefaults, deploymentTargetDefaults, deploymentTargetRuntimeChanged, emptyRuntimeDataVolumeRow, normalizeBoolean, normalizeDeploymentTargetPayload, normalizeRuntimeConfigPayload, normalizeStringIds, parseRuntimeDataVolumes, redeployReleasePayload, releaseDefaults, repositoryBindingItems, runtimeConfigDefaults, serializeRuntimeDataVolumes } from './application-deployments-panel-utils'
import { ApplicationReleaseLogsDialog } from './application-release-logs-dialog'
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

type RepositoryBindingFormInput = z.input<typeof repositoryBindingSchema>
type RepositoryBindingForm = z.output<typeof repositoryBindingSchema>

const repositoryBindingDefaults: RepositoryBindingFormInput = {
  autoConfigureWebhook: true,
  cloneUrl: '',
  defaultBranch: 'main',
  gitAccountId: '',
  owner: '',
  repo: '',
  webhookStatus: 'pending',
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
  const targetDataRetentionEnabled = normalizeBoolean(targetForm.watch('dataRetentionEnabled'), false)
  const targetDataVolumesValue = targetForm.watch('dataVolumes')
  const targetDataVolumes = useMemo(
    () => parseRuntimeDataVolumes(targetDataVolumesValue, targetForm.getValues('dataMountPath') || '/data', targetForm.getValues('dataCapacity') || '1Gi'),
    [targetDataVolumesValue, targetForm],
  )
  const watchedTargetValues = targetForm.watch()
  const selectedRuntimeConfigSetIds = normalizeStringIds(targetForm.watch('runtimeConfigSetIds'))
  const selectedTargetRepositoryBinding = repositoryBindings.find(binding => binding.id === targetRepositoryBindingId)
  const targetRegistry = registries.find(registry => registry.id === targetForm.watch('targetRegistryId'))
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
  const dockerfileSuggestions = useMemo(() => targetBuildOptions.data?.dockerfiles ?? [], [targetBuildOptions.data?.dockerfiles])
  const buildContextSuggestions = useMemo(() => targetBuildOptions.data?.directories ?? [], [targetBuildOptions.data?.directories])
  const dockerfileExposedPorts = useMemo(() => targetBuildOptions.data?.exposedPorts ?? {}, [targetBuildOptions.data?.exposedPorts])
  const buildDirectorySuggestions = buildContextSuggestions.filter(option => option !== '.')
  const dockerfilePathField = targetForm.register('dockerfilePath', { required: true })
  const releaseReadyTargets = useMemo(() => deploymentTargets.filter(target => deploymentTargetCanRelease(target, deployableBuildRuns)), [deployableBuildRuns, deploymentTargets])
  const selectedBuildRun = buildRunMap[form.watch('buildRunId')]
  const latestEditingTargetRelease = editingTarget ? latestReleaseByTarget[deploymentReleaseKey(editingTarget.id)] : undefined
  const targetHasRuntimeChanges = editingTarget ? deploymentTargetRuntimeChanged(editingTarget, normalizeDeploymentTargetPayload(watchedTargetValues)) : false
  const targetCanRedeploy = Boolean(editingTarget && latestEditingTargetRelease && normalizeBoolean(watchedTargetValues.enabled, editingTarget.enabled))
  const targetRuntimeFilesValid = targetConfigFilesValid && targetSecretFilesValid
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
    return deploymentTargets.filter(target => normalizeStringIds(target.runtimeConfigSetIds).includes(runtimeConfigRestartSetId))
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
      buildCpuRequest: target?.buildCpuRequest || '1',
      buildMemoryRequest: target?.buildMemoryRequest || '1Gi',
      repositoryBindingId: target?.repositoryBindingId ?? defaultBinding?.id ?? '',
      targetRegistryId: target?.targetRegistryId ?? defaultRegistry?.id ?? '',
      targetImageRef: deploymentTargetImageRef(target ?? undefined) || defaultTargetImageRef(defaultRegistry, projectSlug, appSlug),
      buildHooksEnabled: target?.buildHooksEnabled ?? true,
      buildHookBindings: target?.buildHookBindings ?? [],
      servicePort: target?.servicePort ?? 8080,
      buildVariableSetIds: normalizeStringIds(target?.buildVariableSetIds),
      runtimeConfigSetIds: normalizeStringIds(target?.runtimeConfigSetIds),
      secretRefs: '',
      secretFiles: '',
      dataRetentionEnabled: target?.dataRetentionEnabled ?? false,
      dataCapacity: target?.dataCapacity || '1Gi',
      dataMountPath: target?.dataMountPath || '/data',
      dataVolumes: target?.dataVolumes || serializeRuntimeDataVolumes(parseRuntimeDataVolumes('', target?.dataMountPath || '/data', target?.dataCapacity || '1Gi')),
      enabled: target?.enabled ?? true,
    })
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
    const current = new Set(normalizeStringIds(targetForm.getValues('runtimeConfigSetIds')))
    if (checked)
      current.add(setId)
    else
      current.delete(setId)
    targetForm.setValue('runtimeConfigSetIds', Array.from(current), { shouldDirty: true, shouldValidate: true })
  }
  const updateTargetDataVolumes = (rows: typeof targetDataVolumes) => {
    targetForm.setValue('dataVolumes', serializeRuntimeDataVolumes(rows), { shouldDirty: true, shouldValidate: true })
  }
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
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('deploymentsPage.createRelease')}</DialogTitle>
            <DialogDescription>{t('deploymentsPage.releaseDialogDescription')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={form.handleSubmit(values => createRelease.mutate(values))}>
            {selectedReleaseTarget?.sourceType !== 'image' && (
              <Field hint={t('deploymentsPage.buildRunHint')} label={t('deploymentsPage.buildRun')} required>
                <Select {...form.register('buildRunId', { required: true })}>
                  <option value="">{t('common.select')}</option>
                  {selectableBuildRuns.map(run => <option key={run.id} value={run.id}>{buildRunOptionLabel(run)}</option>)}
                </Select>
              </Field>
            )}
            <Field label={t('buildsPage.buildConfig')}>
              <Select {...form.register('deploymentTargetId', { required: true })}>
                <option value="">{t('common.select')}</option>
                {releaseReadyTargets.map(target => <option key={target.id} value={target.id}>{target.name}</option>)}
              </Select>
            </Field>
            <Field label={t('deploymentsPage.image')} required><Input {...form.register('imageRef', { required: true })} /></Field>
            <DialogFooter><Button disabled={!form.formState.isValid || createRelease.isPending} type="submit">{t('common.save')}</Button></DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
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
            <div className="grid gap-5 overflow-y-auto px-6 py-4 pb-6">
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
                <Field hint={t('apps.sourceTypeHint')} label={t('apps.sourceType')} required>
                  <Select {...targetForm.register('sourceType', { required: true })}>
                    <option value="repository">{t('apps.repository')}</option>
                    <option value="image">{t('apps.image')}</option>
                  </Select>
                </Field>
                <Field label={t('common.status')}>
                  <Select {...targetForm.register('enabled')}>
                    <option value="true">{t('common.enabled')}</option>
                    <option value="false">{t('common.disabled')}</option>
                  </Select>
                </Field>
                <Field hint={t('deploymentsPage.servicePortHint')} label={t('deploymentsPage.servicePort')} required>
                  <Input {...targetForm.register('servicePort', { valueAsNumber: true })} min={1} max={65535} type="number" />
                </Field>
                <div className="grid gap-3 md:col-span-2">
                  <div className="grid gap-3 md:grid-cols-3">
                    <Field label={t('deploymentsPage.replicas')} required>
                      <Input {...targetForm.register('replicas', { valueAsNumber: true })} min={1} type="number" />
                    </Field>
                    <Field label={t('deploymentsPage.cpuRequest')} required>
                      <UnitInput
                        unitSelectLabel={t('deploymentsPage.cpuRequest')}
                        units={[
                          { label: 'm', value: 'm' },
                          { label: t('deploymentsPage.cpuUnits.core'), value: '' },
                        ]}
                        value={targetForm.watch('cpuRequest')}
                        onChange={value => targetForm.setValue('cpuRequest', value, { shouldDirty: true, shouldValidate: true })}
                      />
                    </Field>
                    <Field label={t('deploymentsPage.memoryRequest')} required>
                      <UnitInput
                        unitSelectLabel={t('deploymentsPage.memoryRequest')}
                        units={[
                          { label: 'Mi', value: 'Mi' },
                          { label: 'Gi', value: 'Gi' },
                        ]}
                        value={targetForm.watch('memoryRequest')}
                        onChange={value => targetForm.setValue('memoryRequest', value, { shouldDirty: true, shouldValidate: true })}
                      />
                    </Field>
                  </div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t('deploymentsPage.runtimeEstimatedPrice', { price: billingDisplay.formatAmountWithUnit(runtimeHourCost) })}
                  </p>
                </div>
              </div>
              {targetSourceType === 'repository'
                ? (
                    <div className="grid gap-4">
                      <div className="grid gap-3 md:grid-cols-2">
                        <Field label={t('apps.repository')} required>
                          <div className="flex flex-col gap-2 sm:flex-row">
                            <Select containerClassName="min-w-0 flex-1" {...targetForm.register('repositoryBindingId', { required: targetSourceType === 'repository' })}>
                              <option value="">{t('common.select')}</option>
                              {repositoryBindings.map(binding => (
                                <option key={binding.id} value={binding.id}>
                                  {binding.owner}
                                  /
                                  {binding.repo}
                                </option>
                              ))}
                            </Select>
                            <Button className="shrink-0" type="button" variant="secondary" onClick={openRepositoryBindingDialog}>
                              <Plus className="size-4" />
                              {t('deploymentsPage.bindRepositoryInTarget')}
                            </Button>
                          </div>
                        </Field>
                        <Field label={t('buildsPage.targetRegistry')} required>
                          <Select {...targetForm.register('targetRegistryId', { required: targetSourceType === 'repository' })}>
                            <option value="">{t('common.select')}</option>
                            {registries.map(registry => <option key={registry.id} value={registry.id}>{registryOptionLabel(registry)}</option>)}
                          </Select>
                        </Field>
                        <Field hint={t('buildsPage.dockerfileLookupHint')} label={t('buildsPage.dockerfilePath')} required>
                          <Input
                            {...dockerfilePathField}
                            list="deployment-target-dockerfile-options"
                            placeholder={t('deploymentsPage.dockerfilePathPlaceholder')}
                            onChange={(event) => {
                              dockerfilePathField.onChange(event)
                              applyDockerfileBuildDefaults(targetForm, event.target.value, buildContextSuggestions, dockerfileExposedPorts)
                            }}
                          />
                          <datalist id="deployment-target-dockerfile-options">
                            {dockerfileSuggestions.map(option => <option key={option} value={option} />)}
                          </datalist>
                          {targetBuildOptions.isFetching && <p className="mt-1 text-xs text-muted-foreground">{t('apps.detectingRepository')}</p>}
                          {targetBuildOptions.isError && <p className="mt-1 text-xs text-destructive">{t('deploymentsPage.buildOptionsLoadFailed')}</p>}
                        </Field>
                        <Field hint={t('buildsPage.buildContextLookupHint')} label={t('buildsPage.buildContext')} required>
                          <Input {...targetForm.register('buildContext', { required: true })} list="deployment-target-build-context-options" placeholder={t('deploymentsPage.buildContextPlaceholder')} />
                          <datalist id="deployment-target-build-context-options">
                            {buildContextSuggestions.map(option => <option key={option} value={option} />)}
                          </datalist>
                        </Field>
                        <Field hint={t('buildsPage.buildDirectoryHint')} label={t('buildsPage.buildDirectory')}>
                          <Input {...targetForm.register('buildDirectory')} list="deployment-target-build-directory-options" placeholder={t('buildsPage.buildDirectoryPlaceholder')} />
                          <datalist id="deployment-target-build-directory-options">
                            {buildDirectorySuggestions.map(option => <option key={option} value={option} />)}
                          </datalist>
                        </Field>
                        <Field hint={t('buildsPage.targetImageRefHint')} label={t('buildsPage.targetImageRef')} required>
                          <TargetImageRefInput
                            placeholder={t('buildsPage.targetImageRefPlaceholder')}
                            prefix={targetImagePrefix}
                            register={targetForm.register('targetImageRef', { required: targetSourceType === 'repository' })}
                          />
                        </Field>
                      </div>
                      <div className="grid gap-3">
                        <div>
                          <h3 className="text-sm font-semibold">{t('deploymentsPage.buildEnvironment')}</h3>
                          <p className="mt-1 text-sm text-muted-foreground">{t('deploymentsPage.buildEnvironmentDescription')}</p>
                        </div>
                        <div className="grid gap-3 md:grid-cols-2">
                          <Field label={t('deploymentsPage.buildCpuRequest')} required>
                            <UnitInput
                              unitSelectLabel={t('deploymentsPage.buildCpuRequest')}
                              units={[
                                { label: 'm', value: 'm' },
                                { label: t('deploymentsPage.cpuUnits.core'), value: '' },
                              ]}
                              value={targetForm.watch('buildCpuRequest')}
                              onChange={value => targetForm.setValue('buildCpuRequest', value, { shouldDirty: true, shouldValidate: true })}
                            />
                          </Field>
                          <Field label={t('deploymentsPage.buildMemoryRequest')} required>
                            <UnitInput
                              unitSelectLabel={t('deploymentsPage.buildMemoryRequest')}
                              units={[
                                { label: 'Mi', value: 'Mi' },
                                { label: 'Gi', value: 'Gi' },
                              ]}
                              value={targetForm.watch('buildMemoryRequest')}
                              onChange={value => targetForm.setValue('buildMemoryRequest', value, { shouldDirty: true, shouldValidate: true })}
                            />
                            <p className="mt-1 text-xs text-muted-foreground">
                              {t('deploymentsPage.buildEstimatedPrice', { price: billingDisplay.formatAmountWithUnit(buildMinuteCost) })}
                            </p>
                          </Field>
                        </div>
                      </div>
                    </div>
                  )
                : (
                    <Field hint={t('apps.imageReferenceHint')} label={t('apps.imageReference')} required>
                      <Input {...targetForm.register('imageRef', { required: targetSourceType === 'image' })} placeholder={t('apps.imageReferencePlaceholder')} />
                    </Field>
                  )}
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
              <div className="grid gap-3">
                <div>
                  <h3 className="text-sm font-semibold">{t('deploymentsPage.runtimeData')}</h3>
                  <p className="mt-1 text-sm text-muted-foreground">{t('deploymentsPage.runtimeDataDescription')}</p>
                </div>
                <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
                  <Field hint={t('deploymentsPage.dataRetentionHint')} label={t('deploymentsPage.dataRetention')}>
                    <Select {...targetForm.register('dataRetentionEnabled')}>
                      <option value="false">{t('common.disabled')}</option>
                      <option value="true">{t('common.enabled')}</option>
                    </Select>
                  </Field>
                  <Field hint={t('deploymentsPage.dataVolumesHint')} label={t('deploymentsPage.dataVolumes')} required={targetDataRetentionEnabled}>
                    <div className="grid gap-2 rounded-md border border-input bg-background p-3">
                      <div className="hidden gap-2 px-1 text-xs font-medium text-muted-foreground md:grid md:grid-cols-[minmax(7rem,0.7fr)_minmax(0,1.5fr)_minmax(10rem,0.7fr)_auto]">
                        <span>{t('deploymentsPage.dataVolumeName')}</span>
                        <span>{t('deploymentsPage.dataMountPath')}</span>
                        <span>{t('deploymentsPage.dataCapacity')}</span>
                        <span className="sr-only">{t('common.actions')}</span>
                      </div>
                      {targetDataVolumes.map((volume, index) => (
                        <div key={volume.id} className="grid gap-2 md:grid-cols-[minmax(7rem,0.7fr)_minmax(0,1.5fr)_minmax(10rem,0.7fr)_auto]">
                          <Input
                            disabled={!targetDataRetentionEnabled}
                            placeholder={t('deploymentsPage.dataVolumeNamePlaceholder')}
                            value={volume.name}
                            onChange={(event) => {
                              const rows = [...targetDataVolumes]
                              rows[index] = { ...volume, name: event.target.value }
                              updateTargetDataVolumes(rows)
                            }}
                          />
                          <Input
                            disabled={!targetDataRetentionEnabled}
                            placeholder={t('deploymentsPage.dataMountPathPlaceholder')}
                            value={volume.mountPath}
                            onChange={(event) => {
                              const rows = [...targetDataVolumes]
                              rows[index] = { ...volume, mountPath: event.target.value }
                              updateTargetDataVolumes(rows)
                            }}
                          />
                          <UnitInput
                            disabled={!targetDataRetentionEnabled}
                            inputProps={{ placeholder: t('deploymentsPage.dataCapacityPlaceholder') }}
                            unitSelectLabel={t('deploymentsPage.dataCapacity')}
                            units={[
                              { label: 'Mi', value: 'Mi' },
                              { label: 'Gi', value: 'Gi' },
                            ]}
                            value={volume.capacity}
                            onChange={(value) => {
                              const rows = [...targetDataVolumes]
                              rows[index] = { ...volume, capacity: value }
                              updateTargetDataVolumes(rows)
                            }}
                          />
                          <Button
                            aria-label={t('deploymentsPage.removeDataVolume')}
                            disabled={!targetDataRetentionEnabled || targetDataVolumes.length <= 1}
                            size="icon"
                            type="button"
                            variant="ghost"
                            onClick={() => updateTargetDataVolumes(targetDataVolumes.filter(row => row.id !== volume.id))}
                          >
                            <Trash2 className="size-4" />
                          </Button>
                        </div>
                      ))}
                      <div>
                        <Button
                          disabled={!targetDataRetentionEnabled}
                          size="sm"
                          type="button"
                          variant="secondary"
                          onClick={() => updateTargetDataVolumes([...targetDataVolumes, emptyRuntimeDataVolumeRow(targetDataVolumes.length)])}
                        >
                          <Plus className="size-4" />
                          {t('deploymentsPage.addDataVolume')}
                        </Button>
                      </div>
                    </div>
                  </Field>
                </div>
              </div>
              <div className="grid gap-3">
                <div>
                  <h3 className="text-sm font-semibold">{t('deploymentsPage.runtimeConfig')}</h3>
                  <p className="mt-1 text-sm text-muted-foreground">{t('deploymentsPage.runtimeConfigDescription')}</p>
                </div>
                <Field hint={t('deploymentsPage.runtimeConfigSetsHint')} label={t('deploymentsPage.runtimeConfigSets')}>
                  <div className="grid gap-3 rounded-md border border-input bg-background p-3">
                    <div className="flex items-center justify-between gap-3">
                      <span className="text-sm font-medium text-foreground">{t('deploymentsPage.runtimeConfigSets')}</span>
                      <Button size="sm" type="button" variant="secondary" onClick={() => openRuntimeConfigDialog()}>
                        <FileCode2 className="size-4" />
                        {t('runtimeConfigSets.createTitle')}
                      </Button>
                    </div>
                    {(runtimeConfigSets.data ?? []).length > 0
                      ? (runtimeConfigSets.data ?? []).map(set => (
                          <div key={set.id} className="flex items-center justify-between gap-3 rounded-md px-2 py-1.5 text-sm hover:bg-muted/60">
                            <label className="flex min-w-0 flex-1 items-center gap-3">
                              <input
                                checked={selectedRuntimeConfigSetIds.includes(set.id)}
                                className="size-4 shrink-0 accent-primary"
                                disabled={!set.enabled}
                                type="checkbox"
                                onChange={event => toggleRuntimeConfigSet(set.id, event.target.checked)}
                              />
                              <span className="min-w-0">
                                <span className="block truncate font-medium" title={set.name}>{set.name}</span>
                                <span className="block truncate text-xs text-muted-foreground">{set.enabled ? t('common.enabled') : t('common.disabled')}</span>
                              </span>
                            </label>
                            <Button aria-label={t('runtimeConfigSets.editTitle')} size="sm" type="button" variant="ghost" onClick={() => openRuntimeConfigDialog(set)}>
                              <Pencil className="size-4" />
                            </Button>
                          </div>
                        ))
                      : <p className="text-sm text-muted-foreground">{t('deploymentsPage.emptyRuntimeConfigSets')}</p>}
                  </div>
                </Field>
                {runtimeConfigRestartAffectedCount > 0 && (
                  <div className="flex gap-3 rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-amber-950 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-100">
                    <Rocket className="mt-0.5 size-4 shrink-0" />
                    <div className="grid flex-1 gap-2 text-sm">
                      <div className="grid gap-1">
                        <p className="font-medium">{t('deploymentsPage.runtimeConfigSetChangedTitle')}</p>
                        <p className="text-amber-900/80 dark:text-amber-100/80">
                          {t('deploymentsPage.runtimeConfigSetChangedDescription', {
                            count: runtimeConfigRestartAffectedCount,
                            redeployable: runtimeConfigRedeployableTargets.length,
                          })}
                        </p>
                      </div>
                      <div className="flex flex-wrap gap-2">
                        <Button
                          disabled={runtimeConfigRedeployableTargets.length === 0 || redeployRuntimeConfigTargets.isPending}
                          size="sm"
                          type="button"
                          variant="secondary"
                          onClick={() => redeployRuntimeConfigTargets.mutate()}
                        >
                          <Rocket className="size-4" />
                          {t('deploymentsPage.redeployAffectedRuntimeConfig')}
                        </Button>
                        <Button
                          size="sm"
                          type="button"
                          variant="ghost"
                          onClick={() => {
                            setRuntimeConfigRestartSetId('')
                            setRuntimeConfigRestartAffectedCount(0)
                          }}
                        >
                          {t('common.close')}
                        </Button>
                      </div>
                    </div>
                  </div>
                )}
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
      <Dialog
        open={repositoryBindingDialogOpen}
        onOpenChange={(open) => {
          setRepositoryBindingDialogOpen(open)
          if (!open)
            resetRepositoryBindingForm()
        }}
      >
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>{t('repositories.bindRepoTitle')}</DialogTitle>
            <DialogDescription>{t('deploymentsPage.repositoryBindingDialogDescription')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={repositoryBindingForm.handleSubmit(values => createRepositoryBinding.mutate(values))}>
            <GitRepositoryPicker
              accounts={gitAccounts.data ?? []}
              providers={gitProviders.data ?? []}
              value={{
                gitAccountId: repositoryBindingForm.watch('gitAccountId') || '',
                owner: repositoryBindingForm.watch('owner') || '',
                repo: repositoryBindingForm.watch('repo') || '',
                cloneUrl: repositoryBindingForm.watch('cloneUrl') || '',
                defaultBranch: repositoryBindingForm.watch('defaultBranch') || 'main',
              }}
              onChange={(next) => {
                repositoryBindingForm.setValue('gitAccountId', next.gitAccountId, { shouldDirty: true, shouldValidate: true })
                repositoryBindingForm.setValue('owner', next.owner, { shouldDirty: true, shouldValidate: true })
                repositoryBindingForm.setValue('repo', next.repo, { shouldDirty: true, shouldValidate: true })
                repositoryBindingForm.setValue('cloneUrl', next.cloneUrl, { shouldDirty: true, shouldValidate: true })
                repositoryBindingForm.setValue('defaultBranch', next.defaultBranch || 'main', { shouldDirty: true, shouldValidate: true })
                setRepositoryBranchSearch('')
              }}
            />
            <div className="grid gap-3 md:grid-cols-3">
              <Field error={repositoryBindingForm.formState.errors.owner?.message} label={t('repositories.owner')} required>
                <Input {...repositoryBindingForm.register('owner')} aria-invalid={Boolean(repositoryBindingForm.formState.errors.owner)} placeholder={t('repositories.ownerPlaceholder')} />
              </Field>
              <Field error={repositoryBindingForm.formState.errors.repo?.message} label={t('repositories.repo')} required>
                <Input {...repositoryBindingForm.register('repo')} aria-invalid={Boolean(repositoryBindingForm.formState.errors.repo)} placeholder={t('repositories.repoPlaceholder')} />
              </Field>
              <Field error={repositoryBindingForm.formState.errors.defaultBranch?.message} label={t('repositories.defaultBranch')}>
                <SearchSelect
                  disabled={!selectedRepositoryAccountId || !selectedRepositoryOwner || !selectedRepositoryName}
                  emptyLabel={t('repositories.noBranches')}
                  limited={repositoryBranches.data?.limited}
                  loading={repositoryBranches.isFetching}
                  options={branchOptions(repositoryBranches.data?.items ?? [], repositoryBindingForm.watch('defaultBranch'))}
                  placeholder={t('repositories.defaultBranchPlaceholder')}
                  search={repositoryBranchSearch}
                  value={repositoryBindingForm.watch('defaultBranch') || ''}
                  onSearchChange={setRepositoryBranchSearch}
                  onValueChange={value => repositoryBindingForm.setValue('defaultBranch', value, { shouldDirty: true, shouldValidate: true })}
                />
              </Field>
            </div>
            <div className="grid gap-3 md:grid-cols-2">
              <Field error={repositoryBindingForm.formState.errors.cloneUrl?.message} label={t('repositories.cloneUrl')}>
                <Input {...repositoryBindingForm.register('cloneUrl')} aria-invalid={Boolean(repositoryBindingForm.formState.errors.cloneUrl)} placeholder={t('repositories.cloneUrlPlaceholder')} />
              </Field>
              <CheckboxField
                className="rounded-md border border-border bg-muted/30 p-3"
                description={t('repositories.autoConfigureWebhookHint')}
                {...repositoryBindingForm.register('autoConfigureWebhook')}
              >
                {t('repositories.autoConfigureWebhook')}
              </CheckboxField>
            </div>
            <DialogFooter>
              <Button disabled={createRepositoryBinding.isPending || (gitAccounts.data ?? []).length === 0 || !repositoryBindingForm.formState.isValid} type="submit">
                <Plus className="size-4" />
                {t('repositories.saveBinding')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
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
