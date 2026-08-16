import type { ArtifactRegistry, BuildEnvironmentConfig, DeploymentRuntimeConfigRef, DeploymentTarget, DeploymentTargetPayload, RepositoryBinding, RuntimeCluster, RuntimeConfigRefMode } from '@/api'
import type { KeyValueRow } from '@/components/common/key-value-rows-editor'
import { useQueryClient } from '@tanstack/react-query'
import { useCallback, useMemo, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { buildVariableRecordToRows, buildVariableRowsToRecord, secretStateToRows } from '@/lib/build-variables'
import { defaultBuildCpuRequest, defaultBuildMemoryRequest, defaultBuildTimeoutSeconds } from './application-build-defaults'
import { defaultTargetImageRef, deploymentTargetImageRef } from './application-config-utils'
import { deploymentTargetDefaults, normalizeBoolean, normalizeDeploymentHookBindings, normalizeDeploymentTargetPayload, normalizeRuntimeConfigRefs, normalizeStringIds, parseRuntimeDataVolumes, runtimeConfigLiveSetIds, serializeRuntimeDataVolumes } from './application-deployments-panel-utils'
import { normalizeWebConsoleOverride } from './web-console-policy'

type BuildEnvironmentStatus = 'loading' | 'ready' | 'unavailable'

interface DeploymentTargetFormOptions {
  applicationId: string
  applicationIdentifier: string
  defaultRuntimeCluster?: RuntimeCluster
  loadBuildEnvironment?: (targetId: string) => Promise<BuildEnvironmentConfig>
  onBuildEnvironmentLoadError?: (error: unknown) => void
  projectId: string
  projectIdentifier: string
  registries: ArtifactRegistry[]
  repositoryBindings: RepositoryBinding[]
}

interface DeploymentTargetFormDefaultsOptions {
  applicationIdentifier: string
  defaultRuntimeCluster?: RuntimeCluster
  projectIdentifier: string
  registries: ArtifactRegistry[]
  repositoryBindings: RepositoryBinding[]
  target?: DeploymentTarget | null
}

function upsertRuntimeConfigRef(refs: DeploymentRuntimeConfigRef[], nextRef: DeploymentRuntimeConfigRef) {
  return [...normalizeRuntimeConfigRefs(refs).filter(ref => ref.setId !== nextRef.setId), nextRef]
}

export function deploymentTargetFormValues({
  applicationIdentifier,
  defaultRuntimeCluster,
  projectIdentifier,
  registries,
  repositoryBindings,
  target,
}: DeploymentTargetFormDefaultsOptions): DeploymentTargetPayload {
  const defaultRegistry = registries.find(registry => registry.credentialSet && registry.isDefault)
    ?? registries.find(registry => registry.credentialSet)
    ?? registries.find(registry => registry.isDefault)
    ?? registries[0]
  const runtimeConfigRefs = normalizeRuntimeConfigRefs(target?.runtimeConfigRefs, target?.runtimeConfigSetIds)

  return {
    ...deploymentTargetDefaults,
    ...target,
    sourceType: target?.sourceType ?? 'repository',
    environmentId: target?.environmentId ?? '',
    clusterId: target?.clusterId ?? defaultRuntimeCluster?.id ?? '',
    replicas: target?.replicas ?? 1,
    cpuRequest: target?.cpuRequest || '1',
    memoryRequest: target?.memoryRequest || '1Gi',
    stage: target?.stage || 'dev',
    buildEnvironmentId: target?.buildEnvironmentId || '',
    buildCpuRequest: target?.buildCpuRequest || defaultBuildCpuRequest,
    buildMemoryRequest: target?.buildMemoryRequest || defaultBuildMemoryRequest,
    buildTimeoutSeconds: target?.buildTimeoutSeconds || defaultBuildTimeoutSeconds,
    buildArgs: target?.buildArgs || '',
    repositoryBindingId: target?.repositoryBindingId ?? repositoryBindings[0]?.id ?? '',
    targetRegistryId: target?.targetRegistryId ?? defaultRegistry?.id ?? '',
    targetImageRef: deploymentTargetImageRef(target ?? undefined) || defaultTargetImageRef(defaultRegistry, projectIdentifier, applicationIdentifier),
    buildHooksEnabled: target?.buildHooksEnabled ?? true,
    buildHookBindings: target?.buildHookBindings ?? [],
    servicePort: target?.servicePort ?? 8080,
    servicePorts: target?.servicePorts?.length ? target.servicePorts : [{ name: 'http', port: target?.servicePort ?? 8080 }],
    buildVariableSetIds: normalizeStringIds(target?.buildVariableSetIds),
    runtimeConfigRefs,
    runtimeConfigSetIds: runtimeConfigLiveSetIds(runtimeConfigRefs),
    secretRefs: '',
    secretFiles: '',
    dataVolumes: target?.dataVolumes ?? [],
    webConsoleEnabled: normalizeWebConsoleOverride(target?.webConsoleEnabled),
    enabled: target?.enabled ?? true,
  }
}

export function useDeploymentTargetForm({
  applicationId,
  applicationIdentifier,
  defaultRuntimeCluster,
  loadBuildEnvironment,
  onBuildEnvironmentLoadError,
  projectId,
  projectIdentifier,
  registries,
  repositoryBindings,
}: DeploymentTargetFormOptions) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const form = useForm<DeploymentTargetPayload>({ defaultValues: deploymentTargetDefaults, mode: 'onChange' })
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingTarget, setEditingTarget] = useState<DeploymentTarget | null>(null)
  const [configFilesValid, setConfigFilesValid] = useState(true)
  const [secretFilesValid, setSecretFilesValid] = useState(true)
  const [buildVariableRows, setBuildVariableRows] = useState<KeyValueRow[]>(() => buildVariableRecordToRows({}))
  const [buildSecretRows, setBuildSecretRows] = useState<KeyValueRow[]>(() => secretStateToRows({}))
  const [buildEnvironmentDirty, setBuildEnvironmentDirty] = useState(false)
  const [buildEnvironmentStatus, setBuildEnvironmentStatus] = useState<BuildEnvironmentStatus>('ready')
  const environmentRequestRef = useRef(0)
  const values = form.watch()

  const resetForm = useCallback((target?: DeploymentTarget | null) => {
    form.reset(deploymentTargetFormValues({
      applicationIdentifier,
      defaultRuntimeCluster,
      projectIdentifier,
      registries,
      repositoryBindings,
      target,
    }))
  }, [applicationIdentifier, defaultRuntimeCluster, form, projectIdentifier, registries, repositoryBindings])

  const closeDialog = useCallback(() => {
    environmentRequestRef.current++
    setDialogOpen(false)
    setEditingTarget(null)
    setBuildEnvironmentDirty(false)
    setBuildEnvironmentStatus('ready')
    form.reset(deploymentTargetDefaults)
  }, [form])

  const handleDialogOpenChange = useCallback((open: boolean) => {
    if (open)
      setDialogOpen(true)
    else
      closeDialog()
  }, [closeDialog])

  const openDialog = useCallback((target?: DeploymentTarget) => {
    const requestId = ++environmentRequestRef.current
    setEditingTarget(target ?? null)
    setConfigFilesValid(true)
    setSecretFilesValid(true)
    resetForm(target)
    setBuildVariableRows(buildVariableRecordToRows({}))
    setBuildSecretRows(secretStateToRows({}))
    setBuildEnvironmentDirty(false)
    setBuildEnvironmentStatus(target ? 'loading' : 'ready')
    setDialogOpen(true)
    if (!target)
      return

    const request = loadBuildEnvironment
      ? loadBuildEnvironment(target.id)
      : queryClient.fetchQuery({
          queryKey: ['build-environment-config', 'deployment', projectId, applicationId, target.id],
          queryFn: () => api.getBuildEnvironmentConfig({ scope: 'deployment', projectId, applicationId, deploymentTargetId: target.id }),
        })
    void request.then((config) => {
      if (environmentRequestRef.current !== requestId)
        return
      setBuildVariableRows(buildVariableRecordToRows(config.variables))
      setBuildSecretRows(secretStateToRows(config.secrets))
      setBuildEnvironmentStatus('ready')
    }).catch((error) => {
      if (environmentRequestRef.current !== requestId)
        return
      setBuildEnvironmentStatus('unavailable')
      if (onBuildEnvironmentLoadError)
        onBuildEnvironmentLoadError(error)
      else
        toast.error(error instanceof Error ? error.message : t('buildsPage.buildEnvironmentLoadFailed'))
    })
  }, [applicationId, loadBuildEnvironment, onBuildEnvironmentLoadError, projectId, queryClient, resetForm, t])

  const updateBuildVariableRows = useCallback((rows: KeyValueRow[]) => {
    setBuildVariableRows(rows)
    setBuildEnvironmentDirty(true)
  }, [])

  const updateBuildSecretRows = useCallback((rows: KeyValueRow[]) => {
    setBuildSecretRows(rows)
    setBuildEnvironmentDirty(true)
  }, [])

  const setRuntimeConfigRefs = useCallback((refs: DeploymentRuntimeConfigRef[]) => {
    const normalizedRefs = normalizeRuntimeConfigRefs(refs)
    form.setValue('runtimeConfigRefs', normalizedRefs, { shouldDirty: true, shouldValidate: true })
    form.setValue('runtimeConfigSetIds', runtimeConfigLiveSetIds(normalizedRefs), { shouldDirty: true, shouldValidate: true })
  }, [form])

  const toggleRuntimeConfigSet = useCallback((setId: string, checked: boolean) => {
    const current = normalizeRuntimeConfigRefs(form.getValues('runtimeConfigRefs'), form.getValues('runtimeConfigSetIds'))
    setRuntimeConfigRefs(checked
      ? upsertRuntimeConfigRef(current, { mode: 'live', setId })
      : current.filter(ref => ref.setId !== setId))
  }, [form, setRuntimeConfigRefs])

  const changeRuntimeConfigRefMode = useCallback((setId: string, mode: RuntimeConfigRefMode) => {
    const current = normalizeRuntimeConfigRefs(form.getValues('runtimeConfigRefs'), form.getValues('runtimeConfigSetIds'))
    setRuntimeConfigRefs(upsertRuntimeConfigRef(current, { mode, setId }))
  }, [form, setRuntimeConfigRefs])

  const setHookBindings = useCallback((bindings: DeploymentTargetPayload['buildHookBindings']) => {
    form.setValue('buildHookBindings', normalizeDeploymentHookBindings(bindings), { shouldDirty: true, shouldValidate: true })
  }, [form])

  const dataVolumes = useMemo(() => parseRuntimeDataVolumes(values.dataVolumes), [values.dataVolumes])

  const updateDataVolumes = useCallback((rows: typeof dataVolumes) => {
    form.setValue('dataVolumes', serializeRuntimeDataVolumes(rows), { shouldDirty: true, shouldValidate: true })
  }, [form])

  const servicePorts = values.servicePorts?.length
    ? values.servicePorts
    : [{ appProtocol: '', name: 'http', port: values.servicePort || 8080 }]

  const updateServicePorts = useCallback((rows: DeploymentTargetPayload['servicePorts']) => {
    const nextRows = rows.length > 0 ? rows : [{ name: 'http', port: 8080 }]
    form.setValue('servicePorts', nextRows, { shouldDirty: true, shouldValidate: true })
    form.setValue('servicePort', nextRows[0]?.port || 8080, { shouldDirty: true, shouldValidate: true })
  }, [form])

  const buildSubmissionPayload = useCallback((nextValues: DeploymentTargetPayload) => {
    const normalized = normalizeDeploymentTargetPayload(nextValues)
    if (!buildEnvironmentDirty)
      return normalized
    return {
      ...normalized,
      buildVariables: buildVariableRowsToRecord(buildVariableRows),
      buildSecrets: buildVariableRowsToRecord(buildSecretRows),
    }
  }, [buildEnvironmentDirty, buildSecretRows, buildVariableRows])

  return {
    buildEnvironmentStatus,
    buildSecretRows,
    buildSubmissionPayload,
    buildVariableRows,
    changeRuntimeConfigRefMode,
    closeDialog,
    configFilesValid,
    hasDataVolumes: dataVolumes.length > 0,
    dataVolumes,
    dialogOpen,
    editingTarget,
    form,
    handleDialogOpenChange,
    imageRefDirty: Boolean(form.formState.dirtyFields.targetImageRef),
    openDialog,
    runtimeFilesValid: configFilesValid && secretFilesValid,
    secretFilesValid,
    selectedHookBindings: normalizeDeploymentHookBindings(values.buildHookBindings),
    selectedRuntimeConfigRefs: normalizeRuntimeConfigRefs(values.runtimeConfigRefs, values.runtimeConfigSetIds),
    servicePorts,
    setConfigFilesValid,
    setHookBindings,
    setSecretFilesValid,
    sourceType: values.sourceType,
    targetBuildHooksEnabled: normalizeBoolean(values.buildHooksEnabled, true),
    toggleRuntimeConfigSet,
    updateBuildSecretRows,
    updateBuildVariableRows,
    updateDataVolumes,
    updateServicePorts,
    values,
  }
}
