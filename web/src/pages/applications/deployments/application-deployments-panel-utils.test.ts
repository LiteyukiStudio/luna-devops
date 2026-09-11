import type { DeploymentTarget, DeploymentTargetPayload } from '@/api'
import { describe, expect, it } from 'vitest'
import { deploymentTargetDefaults, deploymentTargetRuntimeChanged, formatTargetRuntimeSize, normalizeDeploymentTargetPayload } from './application-deployments-panel-utils'

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

  it.each([
    ['runtime config', { environmentVariables: [{ key: 'LOG_LEVEL', value: 'debug', valueMode: 'public' }] }],
    ['service ports', { servicePorts: [{ name: 'http', port: 9090 }] }],
    ['deployment hook', { buildHookBindings: [{ hookConfigId: 'hook_1', phase: 'preDeployment', runOrder: 1 }] }],
  ] satisfies Array<[string, Partial<DeploymentTargetPayload>]>)('detects %s changes that require a redeploy', (_label, overrides) => {
    expect(deploymentTargetRuntimeChanged(currentTarget, changedPayload(overrides))).toBe(true)
  })

  it.each([
    ['replica scale up', { replicas: 2 }],
    ['replica scale to zero', { replicas: 0 }],
    ['display name', { name: 'renamed target' }],
    ['build args', { buildArgs: 'VERSION=2' }],
    ['automatic deployment policy', { autoDeploy: false }],
    ['approval policy', { requireApproval: true }],
    ['Web Console policy', { webConsoleEnabled: false }],
    ['build-only hook', { buildHookBindings: [{ hookConfigId: 'hook_1', phase: 'postBuild', runOrder: 1 }] }],
  ] satisfies Array<[string, Partial<DeploymentTargetPayload>]>)('ignores %s changes that do not alter running instances', (_label, overrides) => {
    expect(deploymentTargetRuntimeChanged(currentTarget, changedPayload(overrides))).toBe(false)
  })

  it('preserves zero replicas while keeping HPA replica bounds positive', () => {
    expect(changedPayload({
      autoScalingMaxReplicas: 0,
      autoScalingMinReplicas: 0,
      replicas: 0,
    })).toMatchObject({
      autoScalingMaxReplicas: 1,
      autoScalingMinReplicas: 1,
      replicas: 0,
    })
  })

  it('does not turn an empty replica field into a stop request', () => {
    expect(changedPayload({ replicas: Number.NaN })).toMatchObject({ replicas: 1 })
  })
})
