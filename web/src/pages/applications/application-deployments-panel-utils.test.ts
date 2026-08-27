import type { DeploymentTarget, DeploymentTargetPayload } from '@/api'
import { describe, expect, it } from 'vitest'
import { deploymentTargetDefaults, deploymentTargetHasRunningInstances, deploymentTargetRuntimeChanged, formatTargetRuntimeSize, normalizeDeploymentTargetPayload } from './application-deployments-panel-utils'

const currentTarget = {
  ...deploymentTargetDefaults,
  applicationId: 'app_1',
  availableReplicas: 1,
  buildVariableSetIds: [],
  createdAt: '2026-08-17T00:00:00Z',
  createdBy: 'user_1',
  dataVolumes: [],
  deleteMessage: '',
  deleteStatus: 'active',
  desiredReplicas: 1,
  id: 'target_1',
  kubernetesName: 'app-dev',
  lastCheckedAt: '2026-08-17T00:00:00Z',
  observationCode: '',
  projectId: 'prj_1',
  readyReplicas: 1,
  runtimeConfigRefs: [],
  secretFilesSet: false,
  status: 'ready',
  updatedReplicas: 1,
} as unknown as DeploymentTarget

function changedPayload(overrides: Partial<DeploymentTargetPayload>) {
  return normalizeDeploymentTargetPayload({ ...deploymentTargetDefaults, ...overrides })
}

describe('deployment target runtime changes', () => {
  it('formats runtime specs without repeating replicas', () => {
    const format = (_key: string, options?: Record<string, unknown>) => `${options?.cpu} · ${options?.memory}`

    expect(formatTargetRuntimeSize({ ...currentTarget, cpuRequest: '500m', memoryRequest: '1Gi' }, format)).toBe('0.5 · 1G')
    expect(formatTargetRuntimeSize({ ...currentTarget, cpuRequest: '125m', memoryRequest: '512Mi' }, format)).toBe('0.125 · 0.5G')
  })

  it('uses the live desired replica observation to identify running instances', () => {
    expect(deploymentTargetHasRunningInstances(currentTarget)).toBe(true)
    expect(deploymentTargetHasRunningInstances({ ...currentTarget, desiredReplicas: 0 })).toBe(false)
  })

  it.each([
    ['replicas', { replicas: 2 }],
    ['service account', { serviceAccountName: 'runtime-service-account' }],
    ['service account token mount', { automountServiceAccountToken: 'false' }],
    ['runtime config', { configRefs: { LOG_LEVEL: 'debug' } }],
    ['service ports', { servicePorts: [{ name: 'http', port: 9090 }] }],
    ['deployment hook', { buildHookBindings: [{ hookConfigId: 'hook_1', phase: 'preDeployment', runOrder: 1 }] }],
  ] satisfies Array<[string, Partial<DeploymentTargetPayload>]>)('detects %s changes that require a redeploy', (_label, overrides) => {
    expect(deploymentTargetRuntimeChanged(currentTarget, changedPayload(overrides))).toBe(true)
  })

  it.each([
    ['display name', { name: 'renamed target' }],
    ['build args', { buildArgs: 'VERSION=2' }],
    ['automatic deployment policy', { autoDeploy: false }],
    ['approval policy', { requireApproval: true }],
    ['Web Console policy', { webConsoleEnabled: false }],
    ['build-only hook', { buildHookBindings: [{ hookConfigId: 'hook_1', phase: 'postBuild', runOrder: 1 }] }],
  ] satisfies Array<[string, Partial<DeploymentTargetPayload>]>)('ignores %s changes that do not alter running instances', (_label, overrides) => {
    expect(deploymentTargetRuntimeChanged(currentTarget, changedPayload(overrides))).toBe(false)
  })
})
