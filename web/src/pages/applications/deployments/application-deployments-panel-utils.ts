import type { UseFormReturn } from 'react-hook-form'
import type { DeploymentRuntimeConfigRef, DeploymentTarget, DeploymentTargetHookBinding, DeploymentTargetPayload, HookPhase, ProjectRuntimeConfigSetPayload, Release, RepositoryBinding } from '@/api'
import { emptyRuntimeDataVolumeRow, parseRuntimeDataVolumes, serializeRuntimeDataVolumes } from '@/lib/runtime-data-volumes'
import { publicRuntimeEnvironmentInputs, publicRuntimeEnvironmentRecord } from '@/lib/runtime-environment'
import { defaultBuildCpuRequest, defaultBuildMemoryRequest, defaultBuildTimeoutSeconds } from '@/pages/applications/application-build-defaults'
import { normalizeWebConsoleOverride } from '@/pages/applications/runtime/web-console-policy'

export type ReleaseForm = Omit<Release, 'id' | 'projectId' | 'createdBy' | 'createdAt' | 'rollbackFromId'>

export const releaseDefaults: ReleaseForm = { applicationId: '', buildRunId: '', deploymentTargetId: '', forceImagePull: false, imageRef: '', message: '', revision: 1, status: 'pending', type: 'deploy' }

export const deploymentTargetDefaults: DeploymentTargetPayload = {
  name: '',
  stage: 'dev',
  clusterId: '',
  workloadType: 'Deployment',
  replicas: 1,
  cpuRequest: '1',
  memoryRequest: '1Gi',
  imagePullPolicy: '',
  containerCommand: '',
  containerArgs: '',
  lifecycle: '',
  initContainers: '',
  sidecarContainers: '',
  readinessProbe: '',
  livenessProbe: '',
  startupProbe: '',
  runAsUser: '',
  runAsGroup: '',
  fsGroup: '',
  fsGroupChangePolicy: '',
  readOnlyRootFilesystem: false,
  capabilityDrop: '',
  nodeSelector: '',
  tolerations: '',
  affinity: '',
  topologySpreadConstraints: '',
  priorityClassName: '',
  serviceAnnotations: '',
  serviceSessionAffinity: '',
  autoScalingEnabled: false,
  autoScalingMinReplicas: 1,
  autoScalingMaxReplicas: 1,
  autoScalingCpuPercent: 0,
  autoScalingMemoryPercent: 0,
  autoScalingBehavior: '',
  servicePorts: [{ appProtocol: '', name: 'http', port: 8080 }],
  sourceType: 'repository',
  repositoryBindingId: '',
  buildDefinitionMode: 'repository_dockerfile',
  buildTemplateId: '',
  buildTemplateVersion: '',
  buildTemplateValues: '{}',
  dockerfilePath: 'Dockerfile',
  buildContext: '.',
  buildDirectory: '',
  buildArgs: '',
  buildEnvironmentId: '',
  buildCpuRequest: defaultBuildCpuRequest,
  buildMemoryRequest: defaultBuildMemoryRequest,
  buildTimeoutSeconds: defaultBuildTimeoutSeconds,
  targetRegistryId: '',
  targetRepository: '',
  targetTag: 'latest',
  targetImageRef: '',
  imageRef: '',
  buildVariableSetIds: [],
  buildHooksEnabled: true,
  buildHookBindings: [],
  autoDeploy: true,
  branchPattern: '',
  tagPattern: '',
  concurrencyPolicy: 'queue',
  runtimeConfigRefs: [],
  environmentVariables: [],
  configFiles: '',
  secretFiles: '',
  dataVolumes: [],
  requireApproval: false,
  webConsoleEnabled: null,
  enabled: true,
}

export const runtimeConfigDefaults: ProjectRuntimeConfigSetPayload = {
  configFiles: '',
  enabled: true,
  environmentVariables: [],
  name: '',
  secretFiles: '',
}

export function shortImageRef(imageRef: string) {
  const value = imageRef.trim()
  if (!value)
    return '-'
  const [repository, tag = ''] = value.split(':')
  const parts = repository.split('/').filter(Boolean)
  const compactRepository = parts.length > 2 ? `${parts.at(-2)}/${parts.at(-1)}` : repository
  return tag ? `${compactRepository}:${tag}` : compactRepository
}

export function formatTargetRuntimeSize(target: DeploymentTarget, t: (key: string, options?: Record<string, unknown>) => string) {
  return t('deploymentsPage.runtimeSizeValue', {
    cpu: formatCPU(target.cpuRequest),
    memory: formatMemoryGi(target.memoryRequest),
  })
}

export function redeployReleasePayload(target: DeploymentTarget, latestRelease?: Release, options: { forceImagePull?: boolean } = {}): ReleaseForm | null {
  const imageRef = target.sourceType === 'image'
    ? (target.imageRef?.trim() || latestRelease?.imageRef?.trim() || '')
    : (latestRelease?.imageRef?.trim() || '')
  const buildRunId = target.sourceType === 'repository' ? (latestRelease?.buildRunId ?? '') : ''
  if (!imageRef)
    return null
  return {
    ...releaseDefaults,
    applicationId: target.applicationId,
    buildRunId,
    deploymentTargetId: target.id,
    forceImagePull: options.forceImagePull ?? false,
    imageRef,
    revision: (latestRelease?.revision ?? 0) + 1,
    status: 'pending',
    type: 'deploy',
  }
}

export function deploymentTargetRuntimeChanged(current: DeploymentTarget, next: DeploymentTargetPayload) {
  const currentPayload = normalizeDeploymentTargetPayload({
    ...deploymentTargetDefaults,
    ...current,
    environmentVariables: publicRuntimeEnvironmentInputs(publicRuntimeEnvironmentRecord(current.environmentVariables)),
  })
  const nextPayload = normalizeDeploymentTargetPayload(next)
  const fields: Array<keyof DeploymentTargetPayload> = [
    'clusterId',
    'workloadType',
    'replicas',
    'cpuRequest',
    'memoryRequest',
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
    'readOnlyRootFilesystem',
    'capabilityDrop',
    'nodeSelector',
    'tolerations',
    'affinity',
    'topologySpreadConstraints',
    'priorityClassName',
    'serviceAnnotations',
    'serviceSessionAffinity',
    'autoScalingEnabled',
    'autoScalingMinReplicas',
    'autoScalingMaxReplicas',
    'autoScalingCpuPercent',
    'autoScalingMemoryPercent',
    'autoScalingBehavior',
    'stage',
    'servicePorts',
    'sourceType',
    'runtimeConfigRefs',
    'environmentVariables',
    'configFiles',
    'dataVolumes',
  ]
  if (nextPayload.sourceType === 'image')
    fields.push('imageRef')
  if (String(nextPayload.secretFiles ?? '').trim())
    return true
  if (fields.some(field => normalizedComparable(currentPayload[field]) !== normalizedComparable(nextPayload[field])))
    return true
  return normalizedComparable(deploymentRuntimeHookBindings(currentPayload.buildHookBindings))
    !== normalizedComparable(deploymentRuntimeHookBindings(nextPayload.buildHookBindings))
}

export function deploymentTargetHasRunningInstances(target: DeploymentTarget) {
  return target.desiredReplicas > 0
}

export function repositoryBindingItems(items: RepositoryBinding[] | null | undefined) {
  return Array.isArray(items) ? items : []
}

export function normalizeDeploymentTargetPayload(values: DeploymentTargetPayload): DeploymentTargetPayload {
  const enabled = normalizeBoolean(values.enabled, true)
  const autoDeploy = normalizeBoolean(values.autoDeploy, true)
  const requireApproval = normalizeBoolean(values.requireApproval, false)
  const buildHooksEnabled = normalizeBoolean(values.buildHooksEnabled, true)
  const readOnlyRootFilesystem = normalizeBoolean(values.readOnlyRootFilesystem, false)
  const dataVolumes = parseRuntimeDataVolumes(values.dataVolumes)
  const sourceType = values.sourceType === 'image' ? 'image' : 'repository'
  const buildDefinitionMode = values.buildDefinitionMode === 'template' ? 'template' : 'repository_dockerfile'
  const servicePorts = normalizeDeploymentServicePorts(values.servicePorts)
  const runtimeConfigRefs = normalizeRuntimeConfigRefs(values.runtimeConfigRefs)
  return {
    ...values,
    sourceType,
    clusterId: values.clusterId?.trim() ?? '',
    workloadType: normalizeChoice(values.workloadType, ['Deployment', 'StatefulSet']) || 'Deployment',
    replicas: normalizePositiveInteger(values.replicas, 1),
    cpuRequest: values.cpuRequest || '1',
    memoryRequest: values.memoryRequest || '1Gi',
    imagePullPolicy: normalizeChoice(values.imagePullPolicy, ['IfNotPresent', 'Always', 'Never']),
    containerCommand: values.containerCommand?.trim() ?? '',
    containerArgs: values.containerArgs?.trim() ?? '',
    lifecycle: values.lifecycle?.trim() ?? '',
    initContainers: values.initContainers?.trim() ?? '',
    sidecarContainers: values.sidecarContainers?.trim() ?? '',
    readinessProbe: values.readinessProbe?.trim() ?? '',
    livenessProbe: values.livenessProbe?.trim() ?? '',
    startupProbe: values.startupProbe?.trim() ?? '',
    runAsUser: values.runAsUser?.trim() ?? '',
    runAsGroup: values.runAsGroup?.trim() ?? '',
    fsGroup: values.fsGroup?.trim() ?? '',
    fsGroupChangePolicy: normalizeChoice(values.fsGroupChangePolicy, ['OnRootMismatch', 'Always']),
    readOnlyRootFilesystem,
    capabilityDrop: values.capabilityDrop?.trim() ?? '',
    nodeSelector: values.nodeSelector?.trim() ?? '',
    tolerations: values.tolerations?.trim() ?? '',
    affinity: values.affinity?.trim() ?? '',
    topologySpreadConstraints: values.topologySpreadConstraints?.trim() ?? '',
    priorityClassName: values.priorityClassName?.trim() ?? '',
    serviceAnnotations: values.serviceAnnotations?.trim() ?? '',
    serviceSessionAffinity: normalizeChoice(values.serviceSessionAffinity, ['None', 'ClientIP']),
    autoScalingEnabled: normalizeBoolean(values.autoScalingEnabled, false),
    autoScalingMinReplicas: normalizeNonNegativeInteger(values.autoScalingMinReplicas),
    autoScalingMaxReplicas: Math.max(normalizePositiveInteger(values.autoScalingMaxReplicas, 1), normalizePositiveInteger(values.replicas, 1)),
    autoScalingCpuPercent: normalizeNonNegativeInteger(values.autoScalingCpuPercent),
    autoScalingMemoryPercent: normalizeNonNegativeInteger(values.autoScalingMemoryPercent),
    autoScalingBehavior: values.autoScalingBehavior?.trim() ?? '',
    stage: normalizeDeploymentStage(values.stage),
    servicePorts,
    enabled,
    autoDeploy,
    requireApproval,
    webConsoleEnabled: normalizeWebConsoleOverride(values.webConsoleEnabled),
    buildHooksEnabled,
    dataVolumes: serializeRuntimeDataVolumes(dataVolumes),
    repositoryBindingId: sourceType === 'repository' ? values.repositoryBindingId : '',
    buildDefinitionMode,
    buildTemplateId: sourceType === 'repository' && buildDefinitionMode === 'template' ? values.buildTemplateId?.trim() : '',
    buildTemplateVersion: sourceType === 'repository' && buildDefinitionMode === 'template' ? values.buildTemplateVersion?.trim() : '',
    buildTemplateValues: sourceType === 'repository' && buildDefinitionMode === 'template' ? (values.buildTemplateValues?.trim() || '{}') : '{}',
    targetRegistryId: sourceType === 'repository' ? values.targetRegistryId : '',
    targetImageRef: sourceType === 'repository' ? values.targetImageRef : '',
    imageRef: sourceType === 'image' ? values.imageRef : '',
    buildArgs: sourceType === 'repository' ? (values.buildArgs?.trim() ?? '') : '',
    buildEnvironmentId: values.buildEnvironmentId || '',
    buildCpuRequest: values.buildCpuRequest || defaultBuildCpuRequest,
    buildMemoryRequest: values.buildMemoryRequest || defaultBuildMemoryRequest,
    buildTimeoutSeconds: normalizePositiveInteger(values.buildTimeoutSeconds, defaultBuildTimeoutSeconds),
    targetTag: values.targetTag || 'latest',
    buildVariableSetIds: normalizeStringIds(values.buildVariableSetIds),
    runtimeConfigRefs,
    configFiles: values.configFiles?.trim() ?? '',
    secretFiles: values.secretFiles?.trim() ?? '',
    buildHookBindings: normalizeDeploymentHookBindings(values.buildHookBindings),
  }
}

function normalizeChoice(value: unknown, allowed: string[]) {
  const normalized = String(value ?? '').trim()
  return allowed.includes(normalized) ? normalized : ''
}

const deploymentHookPhases = new Set<HookPhase>([
  'prePull',
  'postPull',
  'preBuild',
  'postBuild',
  'prePush',
  'postPush',
  'preDeployment',
  'postDeployment',
])

export function normalizeDeploymentHookBindings(value: unknown): DeploymentTargetHookBinding[] {
  const rows = Array.isArray(value) ? value : []
  const seen = new Set<string>()
  const output: DeploymentTargetHookBinding[] = []
  for (const item of rows) {
    const binding = item as Partial<DeploymentTargetHookBinding>
    const hookConfigId = String(binding.hookConfigId ?? '').trim()
    const phase = binding.phase
    if (!hookConfigId || !phase || !deploymentHookPhases.has(phase))
      continue
    const key = `${phase}\x00${hookConfigId}`
    if (seen.has(key))
      continue
    seen.add(key)
    output.push({
      ...binding,
      hookConfigId,
      phase,
      runOrder: output.length + 1,
    })
  }
  return output
}

export function normalizeRuntimeConfigRefs(value: unknown): DeploymentRuntimeConfigRef[] {
  const rawRefs = Array.isArray(value) ? value : []
  const refs = rawRefs
    .map((item) => {
      const ref = item as Partial<DeploymentRuntimeConfigRef>
      return {
        mode: ref.mode === 'snapshot' ? 'snapshot' : 'live',
        setId: String(ref.setId ?? '').trim(),
      } satisfies DeploymentRuntimeConfigRef
    })
    .filter(ref => ref.setId)
  const seen = new Set<string>()
  return refs.filter((ref) => {
    if (seen.has(ref.setId))
      return false
    seen.add(ref.setId)
    return true
  })
}

export function runtimeConfigRefIds(refs: DeploymentRuntimeConfigRef[]) {
  return normalizeRuntimeConfigRefs(refs).map(ref => ref.setId)
}

export function runtimeConfigLiveSetIds(refs: DeploymentRuntimeConfigRef[]) {
  return normalizeRuntimeConfigRefs(refs).filter(ref => ref.mode === 'live').map(ref => ref.setId)
}

export function normalizeRuntimeConfigPayload(values: ProjectRuntimeConfigSetPayload): ProjectRuntimeConfigSetPayload {
  return {
    configFiles: values.configFiles?.trim() ?? '',
    enabled: Boolean(values.enabled),
    environmentVariables: publicRuntimeEnvironmentInputs(Object.fromEntries((values.environmentVariables ?? []).map(item => [item.key, item.value]))),
    name: values.name.trim(),
    secretFiles: values.secretFiles?.trim() ?? '',
  }
}

export function applyDockerfileBuildDefaults(form: UseFormReturn<DeploymentTargetPayload>, dockerfilePath: string, directories: string[], exposedPorts: Record<string, number[]> = {}) {
  const normalizedDockerfile = dockerfilePath.trim()
  if (!normalizedDockerfile)
    return
  const buildContext = defaultBuildContextForDockerfile(normalizedDockerfile, directories)
  form.setValue('dockerfilePath', normalizedDockerfile, { shouldDirty: true, shouldValidate: true })
  form.setValue('buildContext', buildContext, { shouldDirty: true, shouldValidate: true })
  form.setValue('buildDirectory', buildContext === '.' ? '' : buildContext, { shouldDirty: true, shouldValidate: true })
  const detectedPort = exposedPorts[normalizedDockerfile]?.find(port => Number.isInteger(port) && port > 0 && port <= 65535)
  if (detectedPort) {
    form.setValue('servicePorts', [{ name: 'http', port: detectedPort }], { shouldDirty: true, shouldValidate: true })
  }
}

export function normalizeDeploymentServicePorts(value: unknown) {
  const input = Array.isArray(value) ? value : []
  const seen = new Set<number>()
  const ports = input
    .map((item, index) => {
      const port = normalizePositiveInteger(Number((item as { port?: unknown })?.port), index === 0 ? 8080 : 0)
      const name = String((item as { name?: unknown })?.name ?? '').trim() || (index === 0 ? 'http' : `port-${port}`)
      const appProtocol = String((item as { appProtocol?: unknown })?.appProtocol ?? '').trim()
      return { appProtocol, name, port }
    })
    .filter((item) => {
      if (item.port <= 0 || item.port > 65535 || seen.has(item.port))
        return false
      seen.add(item.port)
      return true
    })
  return ports.length > 0 ? ports : [{ appProtocol: '', name: 'http', port: 8080 }]
}

function normalizeNonNegativeInteger(value: unknown) {
  const number = Number(value)
  if (!Number.isFinite(number) || number < 0)
    return 0
  return Math.floor(number)
}

export function normalizeBoolean(value: unknown, fallback: boolean) {
  if (typeof value === 'boolean')
    return value
  if (value === 'true')
    return true
  if (value === 'false')
    return false
  return fallback
}

export function normalizeStringIds(value: string[] | undefined): string[] {
  return (value ?? []).map(item => item.trim()).filter(Boolean)
}

export function formatMetricsPercent(value: number, locale: string) {
  if (!Number.isFinite(value) || value <= 0)
    return '0%'
  return `${value.toLocaleString(locale, { maximumFractionDigits: 1 })}%`
}

export function formatMetricsBytes(value: number, locale: string) {
  if (!Number.isFinite(value) || value <= 0)
    return '-'
  const gib = 1024 ** 3
  const mib = 1024 ** 2
  if (value >= gib)
    return `${(value / gib).toLocaleString(locale, { maximumFractionDigits: 1 })}Gi`
  return `${(value / mib).toLocaleString(locale, { maximumFractionDigits: 1 })}Mi`
}

function formatCPU(value: string) {
  const normalized = value?.trim() || '1'
  const cpu = normalized.endsWith('m')
    ? Number(normalized.slice(0, -1)) / 1000
    : Number(normalized)
  return Number.isFinite(cpu) ? formatRuntimeQuantity(cpu) : normalized
}

function formatMemoryGi(value: string) {
  const normalized = value?.trim() || '1Gi'
  const matched = normalized.match(/^(\d+(?:\.\d+)?)\s*([kmgt])i?$/i)
  if (!matched)
    return normalized
  const amount = Number(matched[1])
  const unit = matched[2].toLowerCase()
  const gibibytes = unit === 'k'
    ? amount / (1024 ** 2)
    : unit === 'm' ? amount / 1024 : unit === 't' ? amount * 1024 : amount
  return `${formatRuntimeQuantity(gibibytes)}G`
}

function formatRuntimeQuantity(value: number) {
  return Number(value.toFixed(3)).toString()
}

function normalizedComparable(value: unknown) {
  if (typeof value === 'boolean')
    return value ? 'true' : 'false'
  if (typeof value === 'string')
    return value.trim()
  if (Array.isArray(value))
    return JSON.stringify(value)
  if (value && typeof value === 'object')
    return JSON.stringify(Object.fromEntries(Object.entries(value).sort(([left], [right]) => left.localeCompare(right))))
  return String(value ?? '').trim()
}

function deploymentRuntimeHookBindings(value: unknown) {
  return normalizeDeploymentHookBindings(value)
    .filter(binding => binding.phase === 'preDeployment' || binding.phase === 'postDeployment')
    .map(binding => ({ hookConfigId: binding.hookConfigId, phase: binding.phase, runOrder: binding.runOrder }))
}

function normalizeDeploymentStage(value: string) {
  return value.trim() || 'dev'
}

function normalizePositiveInteger(value: number, fallback: number) {
  if (!Number.isFinite(value) || value <= 0)
    return fallback
  return Math.floor(value)
}

function defaultBuildContextForDockerfile(dockerfilePath: string, directories: string[]) {
  const normalized = dockerfilePath.trim().replace(/^\/+/, '')
  const separatorIndex = normalized.lastIndexOf('/')
  if (separatorIndex < 0)
    return '.'
  const directory = normalized.slice(0, separatorIndex).trim()
  if (!directory)
    return '.'
  if (directories.length === 0 || directories.includes(directory))
    return directory
  const parent = directories
    .filter(option => option !== '.' && directory.startsWith(`${option}/`))
    .sort((left, right) => right.length - left.length)[0]
  return parent ?? directory
}

export { emptyRuntimeDataVolumeRow, parseRuntimeDataVolumes, serializeRuntimeDataVolumes }
