import type { UseFormReturn } from 'react-hook-form'
import type { DeploymentTarget, DeploymentTargetPayload, ProjectRuntimeConfigSetPayload, Release, RepositoryBinding } from '@/api'
import { emptyRuntimeDataVolumeRow, parseRuntimeDataVolumes, serializeRuntimeDataVolumes } from '@/lib/runtime-data-volumes'
import { defaultBuildCpuRequest, defaultBuildMemoryRequest, defaultBuildTimeoutSeconds } from './application-build-defaults'

export type ReleaseForm = Omit<Release, 'id' | 'projectId' | 'createdBy' | 'createdAt' | 'rollbackFromId'>

export const releaseDefaults: ReleaseForm = { applicationId: '', buildRunId: '', deploymentTargetId: '', environmentId: '', forceImagePull: false, imageRef: '', message: '', revision: 1, status: 'pending', type: 'deploy' }

export const deploymentTargetDefaults: DeploymentTargetPayload = {
  name: '',
  environmentId: '',
  stage: 'prod',
  clusterId: '',
  namespace: '',
  replicas: 1,
  cpuRequest: '1',
  memoryRequest: '1Gi',
  servicePort: 8080,
  servicePorts: [{ name: 'http', port: 8080 }],
  sourceType: 'repository',
  repositoryBindingId: '',
  dockerfilePath: 'Dockerfile',
  buildContext: '.',
  buildDirectory: '',
  buildEnvironmentId: '',
  buildCpuRequest: defaultBuildCpuRequest,
  buildMemoryRequest: defaultBuildMemoryRequest,
  buildTimeoutSeconds: defaultBuildTimeoutSeconds,
  targetRegistryId: '',
  targetRepository: '',
  targetTag: 'latest',
  targetImageRef: '',
  imageRef: '',
  buildLabels: '',
  buildVariableSetIds: [],
  buildHooksEnabled: true,
  buildHookBindings: [],
  autoDeploy: true,
  branchPattern: '',
  tagPattern: '',
  concurrencyPolicy: 'queue',
  runtimeConfigSetIds: [],
  envVars: '',
  configRefs: '',
  secretRefs: '',
  configFiles: '',
  secretFiles: '',
  dataRetentionEnabled: false,
  dataCapacity: '1Gi',
  dataMountPath: '/data',
  dataVolumes: JSON.stringify([{ name: 'data', mountPath: '/data', capacity: '1Gi' }]),
  requireApproval: false,
  enabled: true,
}

export const runtimeConfigDefaults: ProjectRuntimeConfigSetPayload = {
  configFiles: '',
  enabled: true,
  envVars: '',
  name: '',
  secretFiles: '',
  secretRefs: '',
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

export function compactReleaseMessage(message?: string) {
  const value = message?.trim()
  if (!value)
    return '-'
  if (value.startsWith('invalid configuration'))
    return 'config invalid'
  if (value.includes('timed out'))
    return 'rollout timeout'
  if (value.includes('Deployment/Service/ConfigMap/Secret'))
    return 'resources applied'
  return value
}

export function formatTargetRuntimeSize(target: DeploymentTarget, t: (key: string, options?: Record<string, unknown>) => string) {
  const replicas = target.replicas > 0 ? target.replicas : 1
  return t('deploymentsPage.runtimeSizeValue', {
    cpu: formatCPU(target.cpuRequest),
    memory: formatMemoryGi(target.memoryRequest),
    replicas,
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
    environmentId: target.environmentId,
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
    secretRefs: '',
  })
  const nextPayload = normalizeDeploymentTargetPayload(next)
  const fields: Array<keyof DeploymentTargetPayload> = [
    'clusterId',
    'namespace',
    'replicas',
    'cpuRequest',
    'memoryRequest',
    'stage',
    'servicePort',
    'servicePorts',
    'sourceType',
    'runtimeConfigSetIds',
    'envVars',
    'configRefs',
    'configFiles',
    'dataRetentionEnabled',
    'dataCapacity',
    'dataMountPath',
    'dataVolumes',
  ]
  if (nextPayload.sourceType === 'image')
    fields.push('imageRef')
  if (String(nextPayload.secretRefs ?? '').trim() || String(nextPayload.secretFiles ?? '').trim())
    return true
  return fields.some(field => normalizedComparable(currentPayload[field]) !== normalizedComparable(nextPayload[field]))
}

export function repositoryBindingItems(items: RepositoryBinding[] | null | undefined) {
  return Array.isArray(items) ? items : []
}

export function normalizeDeploymentTargetPayload(values: DeploymentTargetPayload): DeploymentTargetPayload {
  const enabled = normalizeBoolean(values.enabled, true)
  const autoDeploy = normalizeBoolean(values.autoDeploy, true)
  const requireApproval = normalizeBoolean(values.requireApproval, false)
  const buildHooksEnabled = normalizeBoolean(values.buildHooksEnabled, true)
  const dataRetentionEnabled = normalizeBoolean(values.dataRetentionEnabled, false)
  const dataVolumes = dataRetentionEnabled
    ? parseRuntimeDataVolumes(values.dataVolumes, values.dataMountPath || '/data', values.dataCapacity || '1Gi')
    : []
  const primaryDataVolume = dataVolumes[0]
  const sourceType = values.sourceType === 'image' ? 'image' : 'repository'
  const servicePorts = normalizeDeploymentServicePorts(values.servicePorts, values.servicePort)
  return {
    ...values,
    sourceType,
    clusterId: values.clusterId?.trim() ?? '',
    namespace: values.namespace?.trim() ?? '',
    replicas: normalizePositiveInteger(values.replicas, 1),
    cpuRequest: values.cpuRequest || '1',
    memoryRequest: values.memoryRequest || '1Gi',
    stage: normalizeDeploymentStage(values.stage),
    servicePorts,
    servicePort: servicePorts[0]?.port ?? 8080,
    enabled,
    autoDeploy,
    requireApproval,
    buildHooksEnabled,
    dataRetentionEnabled,
    dataCapacity: dataRetentionEnabled ? (primaryDataVolume?.capacity?.trim() || '1Gi') : '',
    dataMountPath: dataRetentionEnabled ? (primaryDataVolume?.mountPath?.trim() || '/data') : '',
    dataVolumes: dataRetentionEnabled ? serializeRuntimeDataVolumes(dataVolumes) : '',
    repositoryBindingId: sourceType === 'repository' ? values.repositoryBindingId : '',
    targetRegistryId: sourceType === 'repository' ? values.targetRegistryId : '',
    targetImageRef: sourceType === 'repository' ? values.targetImageRef : '',
    imageRef: sourceType === 'image' ? values.imageRef : '',
    buildEnvironmentId: values.buildEnvironmentId || '',
    buildCpuRequest: values.buildCpuRequest || defaultBuildCpuRequest,
    buildMemoryRequest: values.buildMemoryRequest || defaultBuildMemoryRequest,
    buildTimeoutSeconds: normalizePositiveInteger(values.buildTimeoutSeconds, defaultBuildTimeoutSeconds),
    targetTag: values.targetTag || 'latest',
    buildVariableSetIds: normalizeStringIds(values.buildVariableSetIds),
    runtimeConfigSetIds: normalizeStringIds(values.runtimeConfigSetIds),
    configFiles: values.configFiles?.trim() ?? '',
    secretFiles: values.secretFiles?.trim() ?? '',
    buildHookBindings: values.buildHookBindings ?? [],
  }
}

export function normalizeRuntimeConfigPayload(values: ProjectRuntimeConfigSetPayload): ProjectRuntimeConfigSetPayload {
  return {
    configFiles: values.configFiles?.trim() ?? '',
    enabled: Boolean(values.enabled),
    envVars: values.envVars?.trim() ?? '',
    name: values.name.trim(),
    secretFiles: values.secretFiles?.trim() ?? '',
    secretRefs: values.secretRefs?.trim() ?? '',
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
    form.setValue('servicePort', detectedPort, { shouldDirty: true, shouldValidate: true })
    form.setValue('servicePorts', [{ name: 'http', port: detectedPort }], { shouldDirty: true, shouldValidate: true })
  }
}

export function normalizeDeploymentServicePorts(value: unknown, fallbackPort = 8080) {
  const input = Array.isArray(value) ? value : []
  const seen = new Set<number>()
  const ports = input
    .map((item, index) => {
      const port = normalizePositiveInteger(Number((item as { port?: unknown })?.port), index === 0 ? fallbackPort : 0)
      const name = String((item as { name?: unknown })?.name ?? '').trim() || (index === 0 ? 'http' : `port-${port}`)
      return { name, port }
    })
    .filter((item) => {
      if (item.port <= 0 || item.port > 65535 || seen.has(item.port))
        return false
      seen.add(item.port)
      return true
    })
  return ports.length > 0 ? ports : [{ name: 'http', port: normalizePositiveInteger(fallbackPort, 8080) }]
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

export function normalizeStringIds(value: unknown): string[] {
  if (Array.isArray(value))
    return value.map(item => String(item).trim()).filter(Boolean)
  if (typeof value !== 'string')
    return []
  const trimmed = value.trim()
  if (!trimmed)
    return []
  try {
    const parsed = JSON.parse(trimmed)
    if (Array.isArray(parsed))
      return parsed.map(item => String(item).trim()).filter(Boolean)
  }
  catch {
    return trimmed.split(',').map(item => item.trim()).filter(Boolean)
  }
  return []
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
  return normalized.endsWith('m') ? normalized : `${normalized}c`
}

function formatMemoryGi(value: string) {
  const normalized = value?.trim() || '1Gi'
  return normalized.endsWith('Gi') ? normalized.replace('Gi', 'g') : normalized
}

function normalizedComparable(value: unknown) {
  if (typeof value === 'boolean')
    return value ? 'true' : 'false'
  if (typeof value === 'string')
    return value.trim()
  if (Array.isArray(value))
    return value.map(item => String(item).trim()).filter(Boolean).join(',')
  return String(value ?? '').trim()
}

function normalizeDeploymentStage(value: string) {
  if (value === 'dev' || value === 'test' || value === 'staging' || value === 'prod')
    return value
  return 'prod'
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
