import type { DeploymentRuntimeConfigRef, DeploymentTarget, ProjectRuntimeConfigSetPayload, Release, RepositoryBinding } from '@/api'
import { publicRuntimeEnvironmentInputs } from '@/lib/runtime-environment'
import { deploymentReleaseKey } from '@/pages/applications/application-config-utils'

export type ReleaseForm = Omit<Release, 'id' | 'projectId' | 'createdBy' | 'createdAt' | 'rollbackFromId'>

export const releaseDefaults: ReleaseForm = { applicationId: '', buildRunId: '', deploymentTargetId: '', forceImagePull: false, imageRef: '', message: '', revision: 1, status: 'pending', type: 'deploy' }

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

/** 每个部署配置最近一次创建的发布记录。 */
export function latestReleaseByTarget(releases: Release[]) {
  const output: Record<string, Release> = {}
  for (const release of releases) {
    const key = deploymentReleaseKey(release.deploymentTargetId)
    const existing = output[key]
    if (!existing || new Date(release.createdAt).getTime() > new Date(existing.createdAt).getTime())
      output[key] = release
  }
  return output
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

export function repositoryBindingItems(items: RepositoryBinding[] | null | undefined) {
  return Array.isArray(items) ? items : []
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
