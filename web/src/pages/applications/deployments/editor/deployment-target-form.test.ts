import type { DeploymentTargetFormValues, RuntimeEnvironmentRow } from './deployment-target-form'
import type { DeploymentTarget, DeploymentTargetPayload } from '@/api'
import type { components } from '@/api/generated/openapi'
import { beforeAll, describe, expect, expectTypeOf, it } from 'vitest'
import i18next from '@/i18n'
import {
  createDeploymentTargetFormSchema,
  deploymentTargetFormToPayload,
  deploymentTargetNeedsRedeploy,
  deploymentTargetSectionFields,
  deploymentTargetToFormValues,
  emptyDeploymentTargetFormValues,
  environmentRowsToPayload,
  pendingSecretOperations,
} from './deployment-target-form'

const configuredTarget: DeploymentTarget = {
  id: 'target_1',
  projectId: 'prj_1',
  applicationId: 'app_1',
  name: '生产环境',
  stage: 'prod',
  kubernetesName: 'demo-prod',
  clusterId: 'cluster_1',
  workloadType: 'StatefulSet',
  replicas: 3,
  cpuRequest: '500m',
  memoryRequest: '2Gi',
  imagePullPolicy: 'Always',
  containerCommand: '["/bin/sh","-ec"]',
  containerArgs: 'npm run start',
  lifecycle: '{"preStop":{"exec":{"command":["sleep","5"]}}}',
  initContainers: '[{"name":"init","image":"busybox"}]',
  sidecarContainers: '[{"name":"proxy","image":"envoy"}]',
  readinessProbe: '{"httpGet":{"path":"/ready","port":8080}}',
  livenessProbe: '{"httpGet":{"path":"/live","port":8080}}',
  startupProbe: '{"httpGet":{"path":"/start","port":8080}}',
  runAsUser: '1001',
  runAsGroup: '1002',
  fsGroup: '1003',
  fsGroupChangePolicy: 'OnRootMismatch',
  readOnlyRootFilesystem: true,
  capabilityDrop: 'ALL',
  nodeSelector: 'kubernetes.io/os=linux',
  tolerations: '[{"key":"dedicated","operator":"Exists"}]',
  affinity: '{"nodeAffinity":{}}',
  topologySpreadConstraints: '[{"maxSkew":1,"topologyKey":"kubernetes.io/hostname","whenUnsatisfiable":"ScheduleAnyway"}]',
  priorityClassName: 'high-priority',
  serviceAnnotations: 'service.beta.kubernetes.io/aws-load-balancer-type=nlb',
  serviceSessionAffinity: 'ClientIP',
  autoScalingEnabled: true,
  autoScalingMinReplicas: 2,
  autoScalingMaxReplicas: 6,
  autoScalingCpuPercent: 70,
  autoScalingMemoryPercent: 80,
  autoScalingBehavior: '{"scaleDown":{"stabilizationWindowSeconds":300}}',
  servicePorts: [{ name: 'http', port: 3000, appProtocol: 'http' }, { name: 'metrics', port: 9100, appProtocol: '' }],
  sourceType: 'repository',
  repositoryBindingId: 'rb_1',
  buildDefinitionMode: 'template',
  buildTemplateId: 'node-service',
  buildTemplateVersion: '1.2.0',
  buildTemplateValues: '{"nodeVersion":"22"}',
  dockerfilePath: 'apps/web/Dockerfile',
  buildContext: 'apps/web',
  buildDirectory: 'apps/web',
  buildArgs: 'VERSION=1',
  buildEnvironmentId: 'env_1',
  buildCpuRequest: '4',
  buildMemoryRequest: '8Gi',
  buildTimeoutSeconds: 3600,
  targetRegistryId: 'reg_1',
  targetRepository: 'team/demo',
  targetTag: 'v1',
  imageRef: '',
  buildVariableSetIds: ['bvs_1'],
  buildHooksEnabled: false,
  buildHookBindings: [
    { hookConfigId: 'hook_2', phase: 'postDeployment', runOrder: 2 },
    { hookConfigId: 'hook_1', phase: 'preBuild', runOrder: 1 },
  ],
  autoDeploy: false,
  branchPattern: 'main,release-*',
  tagPattern: 'v*',
  concurrencyPolicy: 'parallel',
  runtimeConfigRefs: [{ setId: 'set_1', mode: 'snapshot' }],
  environmentVariables: [
    { configured: true, key: 'LOG_LEVEL', value: 'info', valueMode: 'public' },
    { configured: true, key: 'API_TOKEN', valueMode: 'secret' },
  ],
  configFiles: '[{"content":"a=1","path":"/etc/app.conf"}]',
  secretFilesSet: true,
  dataVolumes: [{ activationState: 'active', bindingId: 'dvb_1', logicalName: 'cache', sourceType: 'emptyDir', mountPath: '/cache', readOnly: false, emptyDir: { medium: 'Memory', sizeLimit: '512Mi' } }],
  requireApproval: true,
  webConsoleEnabled: false,
  enabled: false,
  status: 'ready',
  desiredReplicas: 3,
  updatedReplicas: 3,
  readyReplicas: 3,
  availableReplicas: 3,
  deleteStatus: 'active',
  deleteMessage: '',
  createdBy: 'usr_1',
  createdAt: '2026-10-01T00:00:00Z',
}

const schema = () => createDeploymentTargetFormSchema(i18next.t)

function validationMessages(values: DeploymentTargetFormValues) {
  const result = schema().safeParse(values)
  return result.success ? {} : Object.fromEntries(result.error.issues.map(issue => [issue.path.join('.'), issue.message]))
}

function validNewTarget(overrides: Partial<DeploymentTargetFormValues> = {}): DeploymentTargetFormValues {
  return {
    ...emptyDeploymentTargetFormValues(),
    name: 'dev',
    repositoryBindingId: 'rb_1',
    targetImageRef: 'team/demo:latest',
    targetRegistryId: 'reg_1',
    ...overrides,
  }
}

function publicRow(key: string, value: string): RuntimeEnvironmentRow {
  return { key, mode: 'public', secretAction: 'keep', stored: 'none', value }
}

function storedSecretRow(key: string): RuntimeEnvironmentRow {
  return { key, mode: 'secret', secretAction: 'keep', stored: 'secret', value: '' }
}

describe('unified runtime environment table', () => {
  beforeAll(async () => {
    await i18next.changeLanguage('en-US')
  })

  it('loads public values and masked secrets as rows', () => {
    expect(deploymentTargetToFormValues(configuredTarget).environmentVariables).toEqual([
      { key: 'LOG_LEVEL', mode: 'public', secretAction: 'keep', stored: 'public', value: 'info' },
      { key: 'API_TOKEN', mode: 'secret', secretAction: 'keep', stored: 'secret', value: '' },
    ])
  })

  it('submits public rows in full and only explicit secret operations', () => {
    expect(environmentRowsToPayload([
      publicRow('LOG_LEVEL', 'info'),
      storedSecretRow('KEEP_ME'),
      { ...storedSecretRow('ROTATE'), secretAction: 'generate' },
      { ...storedSecretRow('REMOVE'), secretAction: 'clear' },
      { key: 'NEW_SECRET', mode: 'secret', secretAction: 'set', stored: 'none', value: 's3cret' },
      { key: 'WAS_SECRET', mode: 'public', secretAction: 'keep', stored: 'secret', value: 'now-public' },
      { key: 'WAS_PUBLIC', mode: 'secret', secretAction: 'set', stored: 'public', value: 'promoted' },
    ])).toEqual([
      { key: 'LOG_LEVEL', valueMode: 'public', value: 'info' },
      { key: 'ROTATE', valueMode: 'secret', secretOperation: 'generate' },
      { key: 'REMOVE', valueMode: 'secret', secretOperation: 'clear' },
      { key: 'NEW_SECRET', valueMode: 'secret', secretOperation: 'set', value: 's3cret' },
      { key: 'WAS_SECRET', valueMode: 'public', value: 'now-public' },
      { key: 'WAS_SECRET', valueMode: 'secret', secretOperation: 'clear' },
      { key: 'WAS_PUBLIC', valueMode: 'secret', secretOperation: 'set', value: 'promoted' },
    ])
  })

  it('keeps names unique across public and secret rows', () => {
    const messages = validationMessages(validNewTarget({ environmentVariables: [publicRow('TOKEN', 'a'), storedSecretRow('TOKEN')] }))
    expect(messages['environmentVariables.1.key']).toBe('Name is already used by another variable')
    expect(validationMessages(validNewTarget({
      environmentVariables: [{ ...storedSecretRow('TOKEN'), secretAction: 'clear' }, publicRow('TOKEN', 'a')],
    }))).toEqual({})
  })

  it('requires a new value when a secret becomes public or is replaced', () => {
    expect(validationMessages(validNewTarget({
      environmentVariables: [{ ...storedSecretRow('TOKEN'), mode: 'public' }],
    }))).toHaveProperty('environmentVariables.0.value')
    expect(validationMessages(validNewTarget({
      environmentVariables: [{ ...storedSecretRow('TOKEN'), secretAction: 'set' }],
    }))).toHaveProperty('environmentVariables.0.value')
  })

  it('counts pending secret operations for the save summary', () => {
    expect(pendingSecretOperations([
      { ...storedSecretRow('A'), secretAction: 'clear' },
      { ...storedSecretRow('B'), mode: 'public', value: 'x' },
      { ...storedSecretRow('C'), secretAction: 'generate' },
      { key: 'D', mode: 'secret', secretAction: 'set', stored: 'none', value: 'v' },
      storedSecretRow('E'),
    ])).toEqual({ cleared: 2, generated: 1, set: 1 })
  })
})

describe('deployment target form model', () => {
  beforeAll(async () => {
    await i18next.changeLanguage('en-US')
  })

  it('covers every field accepted by the OpenAPI deployment target input', () => {
    type InputField = keyof components['schemas']['DeploymentTargetInput']
    expectTypeOf<Exclude<InputField, keyof DeploymentTargetPayload>>().toEqualTypeOf<never>()
  })

  it('assigns every editable form field to exactly one navigation section', () => {
    const passthrough = ['requireApproval', 'buildEnvironmentId', 'buildVariableSetIds', 'targetRepository', 'targetTag']
    const assigned = Object.values(deploymentTargetSectionFields).flat()
    expect(new Set(assigned).size).toBe(assigned.length)
    expect([...assigned, ...passthrough].sort()).toEqual(Object.keys(emptyDeploymentTargetFormValues()).sort())
  })

  it('round trips every configured option without dropping or rewriting values', () => {
    const values = deploymentTargetToFormValues(configuredTarget)
    expect(validationMessages(values)).toEqual({})

    const payload = deploymentTargetFormToPayload(values, { includeBuildEnvironment: false })
    expect(payload).toMatchObject({
      name: '生产环境',
      stage: 'prod',
      workloadType: 'StatefulSet',
      replicas: 3,
      cpuRequest: '500m',
      memoryRequest: '2Gi',
      imagePullPolicy: 'Always',
      readOnlyRootFilesystem: true,
      serviceSessionAffinity: 'ClientIP',
      autoScalingEnabled: true,
      autoScalingMinReplicas: 2,
      autoScalingMaxReplicas: 6,
      servicePorts: configuredTarget.servicePorts,
      buildTemplateId: 'node-service',
      buildTemplateVersion: '1.2.0',
      buildTimeoutSeconds: 3600,
      targetImageRef: 'team/demo:v1',
      buildVariableSetIds: ['bvs_1'],
      buildHooksEnabled: false,
      buildHookBindings: [
        { hookConfigId: 'hook_1', phase: 'preBuild', runOrder: 1 },
        { hookConfigId: 'hook_2', phase: 'postDeployment', runOrder: 2 },
      ],
      autoDeploy: false,
      concurrencyPolicy: 'parallel',
      runtimeConfigRefs: [{ setId: 'set_1', mode: 'snapshot' }],
      environmentVariables: [{ key: 'LOG_LEVEL', value: 'info', valueMode: 'public' }],
      configFiles: configuredTarget.configFiles,
      secretFiles: '',
      dataVolumes: [{ logicalName: 'cache', sourceType: 'emptyDir', mountPath: '/cache', emptyDir: { medium: 'Memory', sizeLimit: '512Mi' } }],
      requireApproval: true,
      webConsoleEnabled: false,
      enabled: false,
    })
    for (const field of ['containerCommand', 'containerArgs', 'lifecycle', 'initContainers', 'sidecarContainers', 'readinessProbe', 'livenessProbe', 'startupProbe', 'runAsUser', 'runAsGroup', 'fsGroup', 'fsGroupChangePolicy', 'capabilityDrop', 'nodeSelector', 'tolerations', 'affinity', 'topologySpreadConstraints', 'priorityClassName', 'serviceAnnotations', 'autoScalingCpuPercent', 'autoScalingMemoryPercent', 'autoScalingBehavior', 'buildContext', 'buildDirectory', 'buildArgs', 'buildCpuRequest', 'buildMemoryRequest', 'buildEnvironmentId', 'branchPattern', 'tagPattern', 'repositoryBindingId', 'targetRegistryId', 'clusterId'] as const)
      expect(payload[field]).toEqual(configuredTarget[field])
  })

  it('only submits build variables when requested', () => {
    const values = validNewTarget({ buildVariables: [{ id: 'row', key: 'NODE_ENV', value: 'production' }] })
    expect(deploymentTargetFormToPayload(values, { includeBuildEnvironment: false })).not.toHaveProperty('buildVariables')
    expect(deploymentTargetFormToPayload(values, { includeBuildEnvironment: true }).buildVariables).toEqual({ NODE_ENV: 'production' })
  })

  it('reports field errors instead of silently repairing input', () => {
    expect(validationMessages(validNewTarget({ name: ' ' }))).toHaveProperty('name')
    expect(validationMessages(validNewTarget({ repositoryBindingId: '' }))).toHaveProperty('repositoryBindingId')
    expect(validationMessages(validNewTarget({ sourceType: 'image' }))).toHaveProperty('imageRef')
    expect(validationMessages(validNewTarget({ replicas: Number.NaN }))).toHaveProperty('replicas')
    expect(validationMessages(validNewTarget({ autoScalingEnabled: true, autoScalingMinReplicas: 3, autoScalingMaxReplicas: 2, autoScalingCpuPercent: 70 }))).toHaveProperty('autoScalingMaxReplicas')
    expect(validationMessages(validNewTarget({ autoScalingEnabled: true }))).toHaveProperty('autoScalingCpuPercent')
    expect(validationMessages(validNewTarget({ servicePorts: [{ name: 'http', port: 8080, appProtocol: '' }, { name: 'admin', port: 8080, appProtocol: '' }] }))).toHaveProperty('servicePorts.1.port')
    expect(validationMessages(validNewTarget({ readinessProbe: '{"httpGet":' }))).toHaveProperty('readinessProbe')
    expect(validationMessages(validNewTarget({ tolerations: '{}' }))).toHaveProperty('tolerations')
    expect(validationMessages(validNewTarget({ runAsUser: 'root' }))).toHaveProperty('runAsUser')
    expect(validationMessages(validNewTarget({ environmentVariables: [publicRow('1BAD', 'x')] }))).toHaveProperty('environmentVariables.0.key')
    expect(validationMessages(validNewTarget({ configFiles: '[{"content":"x","path":"relative.conf"}]' }))).toHaveProperty('configFiles')
    expect(validationMessages(validNewTarget({
      buildHookBindings: [{ hookConfigId: 'hook_1', phase: 'preDeployment' }, { hookConfigId: 'hook_1', phase: 'preDeployment' }],
    }))).toHaveProperty('buildHookBindings.1.hookConfigId')
  })

  it('accepts a freshly created target with platform defaults', () => {
    expect(validationMessages(validNewTarget())).toEqual({})
  })

  describe('redeploy detection', () => {
    const baselineValues = deploymentTargetToFormValues({ ...configuredTarget, autoScalingEnabled: false, buildHooksEnabled: true, buildHookBindings: [] })
    const baseline = deploymentTargetFormToPayload(baselineValues, { includeBuildEnvironment: false })
    const changed = (overrides: Partial<DeploymentTargetFormValues>) => deploymentTargetNeedsRedeploy(
      baseline,
      deploymentTargetFormToPayload({ ...baselineValues, ...overrides }, { includeBuildEnvironment: false }),
    )

    it.each([
      ['runtime config', { environmentVariables: [publicRow('LOG_LEVEL', 'debug'), storedSecretRow('API_TOKEN')] }],
      ['secret replacement', { environmentVariables: [publicRow('LOG_LEVEL', 'info'), { ...storedSecretRow('API_TOKEN'), secretAction: 'set' as const, value: 'new' }] }],
      ['service ports', { servicePorts: [{ name: 'http', port: 9090, appProtocol: '' }] }],
      ['deployment hook', { buildHookBindings: [{ hookConfigId: 'hook_1', phase: 'preDeployment' as const }] }],
      ['secret files', { secretFiles: '[{"content":"x","path":"/run/secret"}]' }],
    ])('requires a redeploy for %s changes', (_label, overrides) => {
      expect(changed(overrides)).toBe(true)
    })

    it.each([
      ['replica scale', { replicas: 0 }],
      ['display name', { name: 'renamed target' }],
      ['build args', { buildArgs: 'VERSION=2' }],
      ['automatic deployment policy', { autoDeploy: true }],
      ['Web Console policy', { webConsoleEnabled: 'inherit' as const }],
      ['build-only hook', { buildHookBindings: [{ hookConfigId: 'hook_1', phase: 'postBuild' as const }] }],
    ])('does not require a redeploy for %s changes', (_label, overrides) => {
      expect(changed(overrides)).toBe(false)
    })
  })
})
