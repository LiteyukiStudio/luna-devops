import type { TFunction } from 'i18next'
import type { BuildEnvironmentConfig, DeploymentRuntimeEnvironmentVariableInput, DeploymentTarget, DeploymentTargetPayload, HookPhase } from '@/api'
import type { RuntimeDataVolumeRow } from '@/lib/runtime-data-volumes'
import { z } from 'zod'
import { buildVariableRecordToRows, buildVariableRowsToRecord, emptyKeyValueRow, secretStateToRows } from '@/lib/build-variables'
import { duplicateRuntimeConfigPathRowIds, parseRuntimeConfigFiles } from '@/lib/runtime-config-files'
import { parseRuntimeDataVolumes, projectVolumeRowsAreValid, serializeRuntimeDataVolumes } from '@/lib/runtime-data-volumes'
import { defaultBuildCpuRequest, defaultBuildMemoryRequest, defaultBuildTimeoutSeconds } from '@/pages/applications/application-build-defaults'
import { deploymentTargetImageRef } from '@/pages/applications/application-config-utils'

export const publicDeploymentStages = ['dev', 'test', 'staging', 'prod'] as const
export function isPublicDeploymentStage(stage: string): stage is typeof publicDeploymentStages[number] {
  return (publicDeploymentStages as readonly string[]).includes(stage)
}

export const deploymentHookPhases = ['prePull', 'postPull', 'preBuild', 'postBuild', 'prePush', 'postPush', 'preDeployment', 'postDeployment'] as const satisfies readonly HookPhase[]
export const buildHookPhases = new Set<HookPhase>(['prePull', 'postPull', 'preBuild', 'postBuild', 'prePush', 'postPush'])
export const maxServicePorts = 16
export const maxRuntimeEnvironmentVariables = 128
export const maxRuntimeEnvironmentValueLength = 8192
/** 与后端 isBuildEnvKey 一致的环境变量名规则。 */
export const runtimeEnvironmentKeyPattern = /^[A-Z_]\w{0,127}$/i

export type DeploymentTargetEditorSection = 'basics' | 'source' | 'runtime' | 'release' | 'hooks' | 'security'

/**
 * 部署配置编辑器的唯一表单模型。
 * 默认值、约束与跨字段规则都集中在这里；提交只做格式转换，不再静默修补用户输入。
 */
export function createDeploymentTargetFormSchema(t: TFunction) {
  const message = {
    required: t('deploymentTargetEditor.validation.required'),
    integer: t('deploymentTargetEditor.validation.integer'),
    digits: t('deploymentTargetEditor.validation.digits'),
    jsonObject: t('deploymentTargetEditor.validation.jsonObject'),
    jsonArray: t('deploymentTargetEditor.validation.jsonArray'),
    stringList: t('deploymentTargetEditor.validation.stringList'),
    keyValueLines: t('deploymentTargetEditor.validation.keyValueLines'),
  }
  const requiredText = z.string().trim().min(1, message.required)
  const integer = (min: number, max?: number) => {
    const base = z.number({ error: message.integer }).int(message.integer).min(min, t('deploymentTargetEditor.validation.min', { min }))
    return max === undefined ? base : base.max(max, t('deploymentTargetEditor.validation.max', { max }))
  }
  const digits = z.string().trim().regex(/^\d*$/, message.digits)
  const jsonObject = z.string().refine(value => jsonTextMatches(value, 'object'), message.jsonObject)
  const jsonArray = z.string().refine(value => jsonTextMatches(value, 'array'), message.jsonArray)
  const stringList = z.string().refine(stringListTextIsValid, message.stringList)
  const keyValueOrJson = z.string().refine(keyValueOrJsonTextIsValid, message.keyValueLines)
  const keyValueRow = z.object({ id: z.string(), key: z.string(), value: z.string(), existing: z.boolean().optional() })

  return z.object({
    // 基本信息
    name: requiredText.max(120, t('deploymentTargetEditor.validation.maxLength', { max: 120 })),
    stage: z.string().regex(/^(?:dev|test|staging|prod|sys-[a-z0-9-]+)$/, message.required),
    clusterId: z.string(),
    enabled: z.boolean(),

    // 来源与构建
    sourceType: z.enum(['repository', 'image']),
    repositoryBindingId: z.string(),
    imageRef: z.string(),
    targetRegistryId: z.string(),
    targetImageRef: z.string(),
    buildDefinitionMode: z.enum(['repository_dockerfile', 'template']),
    buildTemplateId: z.string(),
    buildTemplateVersion: z.string(),
    buildTemplateValues: z.string(),
    dockerfilePath: z.string(),
    buildContext: z.string(),
    buildDirectory: z.string(),
    buildArgs: z.string(),
    buildCpuRequest: requiredText,
    buildMemoryRequest: requiredText,
    buildTimeoutMinutes: integer(1, 1440),
    buildVariables: z.array(keyValueRow),
    buildSecrets: z.array(keyValueRow),

    // 运行配置
    replicas: integer(0),
    cpuRequest: requiredText,
    memoryRequest: requiredText,
    autoScalingEnabled: z.boolean(),
    autoScalingMinReplicas: integer(1),
    autoScalingMaxReplicas: integer(1),
    autoScalingCpuPercent: integer(0, 1000),
    autoScalingMemoryPercent: integer(0, 1000),
    autoScalingBehavior: jsonObject,
    servicePorts: z.array(z.object({
      name: z.string().trim().max(63, t('deploymentTargetEditor.validation.maxLength', { max: 63 })),
      port: integer(1, 65535),
      appProtocol: z.string().trim().max(253, t('deploymentTargetEditor.validation.maxLength', { max: 253 })),
    })).min(1, message.required).max(maxServicePorts, t('deploymentTargetEditor.validation.portsMax', { max: maxServicePorts })),
    serviceSessionAffinity: z.enum(['', 'None', 'ClientIP']),
    serviceAnnotations: keyValueOrJson,
    runtimeConfigRefs: z.array(z.object({ setId: z.string(), mode: z.enum(['live', 'snapshot']) })),
    environmentVariables: z.array(z.object({
      key: z.string().trim(),
      mode: z.enum(['public', 'secret']),
      value: z.string().max(maxRuntimeEnvironmentValueLength, t('deploymentTargetEditor.validation.maxLength', { max: maxRuntimeEnvironmentValueLength })),
      stored: z.enum(['none', 'public', 'secret']),
      secretAction: z.enum(['keep', 'set', 'generate', 'clear']),
    })).max(maxRuntimeEnvironmentVariables, t('deploymentTargetEditor.validation.environmentMax', { max: maxRuntimeEnvironmentVariables })),
    configFiles: z.string(),
    secretFiles: z.string(),
    dataVolumes: z.array(z.custom<RuntimeDataVolumeRow>()),
    workloadType: z.enum(['Deployment', 'StatefulSet']),
    imagePullPolicy: z.enum(['', 'IfNotPresent', 'Always', 'Never']),
    containerCommand: stringList,
    containerArgs: stringList,
    lifecycle: jsonObject,
    readinessProbe: jsonObject,
    livenessProbe: jsonObject,
    startupProbe: jsonObject,

    // 发布与触发
    autoDeploy: z.boolean(),
    branchPattern: z.string(),
    tagPattern: z.string(),
    concurrencyPolicy: z.enum(['queue', 'parallel']),

    // 钩子
    buildHooksEnabled: z.boolean(),
    buildHookBindings: z.array(z.object({ phase: z.enum(deploymentHookPhases), hookConfigId: requiredText })),

    // 安全与调度
    webConsoleEnabled: z.enum(['inherit', 'disabled']),
    runAsUser: digits,
    runAsGroup: digits,
    fsGroup: digits,
    fsGroupChangePolicy: z.enum(['', 'OnRootMismatch', 'Always']),
    readOnlyRootFilesystem: z.boolean(),
    capabilityDrop: stringList,
    nodeSelector: keyValueOrJson,
    tolerations: jsonArray,
    affinity: jsonObject,
    topologySpreadConstraints: jsonArray,
    priorityClassName: z.string(),
    initContainers: jsonArray,
    sidecarContainers: jsonArray,

    // 编辑器不展示、但必须原样回写的字段
    requireApproval: z.boolean(),
    buildEnvironmentId: z.string(),
    buildVariableSetIds: z.array(z.string()),
    targetRepository: z.string(),
    targetTag: z.string(),
  }).superRefine((values, context) => {
    const issue = (path: (string | number)[], text: string) => context.addIssue({ code: 'custom', message: text, path })
    if (values.sourceType === 'repository') {
      if (!values.repositoryBindingId.trim())
        issue(['repositoryBindingId'], message.required)
      if (!values.targetRegistryId.trim())
        issue(['targetRegistryId'], message.required)
      if (!values.targetImageRef.trim())
        issue(['targetImageRef'], message.required)
      if (!values.buildContext.trim())
        issue(['buildContext'], message.required)
      if (values.buildDefinitionMode === 'repository_dockerfile' && !values.dockerfilePath.trim())
        issue(['dockerfilePath'], message.required)
      if (values.buildDefinitionMode === 'template' && !values.buildTemplateId.trim())
        issue(['buildTemplateId'], message.required)
    }
    else if (!values.imageRef.trim()) {
      issue(['imageRef'], message.required)
    }

    if (values.autoScalingEnabled) {
      if (values.autoScalingMaxReplicas < values.autoScalingMinReplicas)
        issue(['autoScalingMaxReplicas'], t('deploymentTargetEditor.validation.autoScalingRange'))
      if (values.autoScalingCpuPercent === 0 && values.autoScalingMemoryPercent === 0)
        issue(['autoScalingCpuPercent'], t('deploymentTargetEditor.validation.autoScalingMetric'))
    }

    const seenPorts = new Set<number>()
    const seenPortNames = new Set<string>()
    values.servicePorts.forEach((port, index) => {
      if (seenPorts.has(port.port))
        issue(['servicePorts', index, 'port'], t('deploymentTargetEditor.validation.portDuplicate'))
      seenPorts.add(port.port)
      const name = port.name.trim().toLowerCase()
      if (name && seenPortNames.has(name))
        issue(['servicePorts', index, 'name'], t('deploymentTargetEditor.validation.portNameDuplicate'))
      if (name)
        seenPortNames.add(name)
    })

    // 普通配置与加密配置共用一个命名空间；待清除的密钥行不再占用名称。
    const seenEnvironmentKeys = new Set<string>()
    values.environmentVariables.forEach((row, index) => {
      if (row.mode === 'secret' && row.secretAction === 'clear')
        return
      if (!runtimeEnvironmentKeyPattern.test(row.key))
        issue(['environmentVariables', index, 'key'], t('deploymentTargetEditor.validation.environmentKey'))
      else if (seenEnvironmentKeys.has(row.key))
        issue(['environmentVariables', index, 'key'], t('deploymentTargetEditor.validation.environmentKeyDuplicate'))
      seenEnvironmentKeys.add(row.key)
      if (row.mode === 'secret' && row.secretAction === 'set' && !row.value)
        issue(['environmentVariables', index, 'value'], t('deploymentTargetEditor.validation.secretValueRequired'))
      if (row.mode === 'public' && row.stored === 'secret' && !row.value)
        issue(['environmentVariables', index, 'value'], t('deploymentTargetEditor.validation.reenterValue'))
    })

    for (const field of ['configFiles', 'secretFiles'] as const) {
      const rows = parseRuntimeConfigFiles(values[field])
      if (rows.some(row => !row.path.trim().startsWith('/')))
        issue([field], t('deploymentTargetEditor.validation.configFilePath'))
      else if (duplicateRuntimeConfigPathRowIds(rows).size > 0)
        issue([field], t('runtimeConfigFilesEditor.duplicatePath'))
    }

    values.dataVolumes.forEach((row, index) => {
      if (!projectVolumeRowsAreValid([row]))
        issue(['dataVolumes', index], t('projectVolumes.deploymentSelectionIncomplete'))
    })

    const seenHooks = new Set<string>()
    values.buildHookBindings.forEach((binding, index) => {
      const key = `${binding.phase}\u0000${binding.hookConfigId}`
      if (seenHooks.has(key))
        issue(['buildHookBindings', index, 'hookConfigId'], t('deploymentTargetEditor.validation.hookDuplicate'))
      seenHooks.add(key)
    })
  })
}

export type DeploymentTargetFormSchema = ReturnType<typeof createDeploymentTargetFormSchema>
export type DeploymentTargetFormValues = z.input<DeploymentTargetFormSchema>
export type DeploymentTargetFieldName = keyof DeploymentTargetFormValues

/** 新建部署配置时的平台默认值；也是“高级项是否已设置”的比较基线。 */
export function emptyDeploymentTargetFormValues(): DeploymentTargetFormValues {
  return {
    name: '',
    stage: 'dev',
    clusterId: '',
    enabled: true,
    sourceType: 'repository',
    repositoryBindingId: '',
    imageRef: '',
    targetRegistryId: '',
    targetImageRef: '',
    buildDefinitionMode: 'repository_dockerfile',
    buildTemplateId: '',
    buildTemplateVersion: '',
    buildTemplateValues: '{}',
    dockerfilePath: 'Dockerfile',
    buildContext: '.',
    buildDirectory: '',
    buildArgs: '',
    buildCpuRequest: defaultBuildCpuRequest,
    buildMemoryRequest: defaultBuildMemoryRequest,
    buildTimeoutMinutes: defaultBuildTimeoutSeconds / 60,
    buildVariables: [emptyKeyValueRow()],
    buildSecrets: [emptyKeyValueRow()],
    replicas: 1,
    cpuRequest: '1',
    memoryRequest: '1Gi',
    autoScalingEnabled: false,
    autoScalingMinReplicas: 1,
    autoScalingMaxReplicas: 1,
    autoScalingCpuPercent: 0,
    autoScalingMemoryPercent: 0,
    autoScalingBehavior: '',
    servicePorts: [{ name: 'http', port: 8080, appProtocol: '' }],
    serviceSessionAffinity: '',
    serviceAnnotations: '',
    runtimeConfigRefs: [],
    environmentVariables: [],
    configFiles: '',
    secretFiles: '',
    dataVolumes: [],
    workloadType: 'Deployment',
    imagePullPolicy: '',
    containerCommand: '',
    containerArgs: '',
    lifecycle: '',
    readinessProbe: '',
    livenessProbe: '',
    startupProbe: '',
    autoDeploy: true,
    branchPattern: '',
    tagPattern: '',
    concurrencyPolicy: 'queue',
    buildHooksEnabled: true,
    buildHookBindings: [],
    webConsoleEnabled: 'inherit',
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
    initContainers: '',
    sidecarContainers: '',
    requireApproval: false,
    buildEnvironmentId: '',
    buildVariableSetIds: [],
    targetRepository: '',
    targetTag: '',
  }
}

/** 把已保存的部署配置转换成表单值；构建变量来自独立接口，加载失败时保持空行且不会提交。 */
export function deploymentTargetToFormValues(target: DeploymentTarget, buildEnvironment?: BuildEnvironmentConfig): DeploymentTargetFormValues {
  const defaults = emptyDeploymentTargetFormValues()
  const buildEnvironmentValues = buildEnvironment
    ? { buildVariables: buildVariableRecordToRows(buildEnvironment.variables), buildSecrets: secretStateToRows(buildEnvironment.secrets) }
    : {}
  // Go 端空切片可能序列化为 null，数组字段在入口处收窄。
  return {
    ...defaults,
    ...buildEnvironmentValues,
    name: target.name,
    stage: target.stage,
    clusterId: target.clusterId,
    enabled: target.enabled,
    sourceType: target.sourceType,
    repositoryBindingId: target.repositoryBindingId,
    imageRef: target.imageRef,
    targetRegistryId: target.targetRegistryId,
    targetImageRef: deploymentTargetImageRef(target),
    buildDefinitionMode: target.buildDefinitionMode,
    buildTemplateId: target.buildTemplateId,
    buildTemplateVersion: target.buildTemplateVersion,
    buildTemplateValues: target.buildTemplateValues || '{}',
    dockerfilePath: target.dockerfilePath,
    buildContext: target.buildContext,
    buildDirectory: target.buildDirectory,
    buildArgs: target.buildArgs,
    buildCpuRequest: target.buildCpuRequest || defaults.buildCpuRequest,
    buildMemoryRequest: target.buildMemoryRequest || defaults.buildMemoryRequest,
    buildTimeoutMinutes: Math.max(1, Math.round((target.buildTimeoutSeconds || defaultBuildTimeoutSeconds) / 60)),
    replicas: target.replicas,
    cpuRequest: target.cpuRequest || defaults.cpuRequest,
    memoryRequest: target.memoryRequest || defaults.memoryRequest,
    autoScalingEnabled: target.autoScalingEnabled,
    autoScalingMinReplicas: target.autoScalingMinReplicas || 1,
    autoScalingMaxReplicas: target.autoScalingMaxReplicas || 1,
    autoScalingCpuPercent: target.autoScalingCpuPercent,
    autoScalingMemoryPercent: target.autoScalingMemoryPercent,
    autoScalingBehavior: target.autoScalingBehavior,
    servicePorts: target.servicePorts?.length
      ? target.servicePorts.map(port => ({ name: port.name, port: port.port, appProtocol: port.appProtocol ?? '' }))
      : defaults.servicePorts,
    serviceSessionAffinity: oneOf(target.serviceSessionAffinity, ['', 'None', 'ClientIP'] as const, ''),
    serviceAnnotations: target.serviceAnnotations,
    runtimeConfigRefs: (target.runtimeConfigRefs ?? []).map(ref => ({ setId: ref.setId, mode: ref.mode })),
    environmentVariables: (target.environmentVariables ?? []).map(variable => ({
      key: variable.key,
      mode: variable.valueMode,
      value: variable.valueMode === 'public' ? variable.value ?? '' : '',
      stored: variable.valueMode,
      secretAction: 'keep' as const,
    })),
    configFiles: target.configFiles,
    dataVolumes: parseRuntimeDataVolumes(target.dataVolumes),
    workloadType: oneOf(target.workloadType, ['Deployment', 'StatefulSet'] as const, 'Deployment'),
    imagePullPolicy: oneOf(target.imagePullPolicy, ['', 'IfNotPresent', 'Always', 'Never'] as const, ''),
    containerCommand: target.containerCommand,
    containerArgs: target.containerArgs,
    lifecycle: target.lifecycle,
    readinessProbe: target.readinessProbe,
    livenessProbe: target.livenessProbe,
    startupProbe: target.startupProbe,
    autoDeploy: target.autoDeploy,
    branchPattern: target.branchPattern,
    tagPattern: target.tagPattern,
    concurrencyPolicy: target.concurrencyPolicy,
    buildHooksEnabled: target.buildHooksEnabled,
    buildHookBindings: [...(target.buildHookBindings ?? [])]
      .sort((left, right) => left.runOrder - right.runOrder)
      .map(binding => ({ phase: binding.phase, hookConfigId: binding.hookConfigId })),
    webConsoleEnabled: target.webConsoleEnabled === false ? 'disabled' : 'inherit',
    runAsUser: target.runAsUser,
    runAsGroup: target.runAsGroup,
    fsGroup: target.fsGroup,
    fsGroupChangePolicy: oneOf(target.fsGroupChangePolicy, ['', 'OnRootMismatch', 'Always'] as const, ''),
    readOnlyRootFilesystem: target.readOnlyRootFilesystem,
    capabilityDrop: target.capabilityDrop,
    nodeSelector: target.nodeSelector,
    tolerations: target.tolerations,
    affinity: target.affinity,
    topologySpreadConstraints: target.topologySpreadConstraints,
    priorityClassName: target.priorityClassName,
    initContainers: target.initContainers,
    sidecarContainers: target.sidecarContainers,
    requireApproval: target.requireApproval,
    buildEnvironmentId: target.buildEnvironmentId,
    buildVariableSetIds: target.buildVariableSetIds ?? [],
    targetRepository: target.targetRepository,
    targetTag: target.targetTag,
  }
}

/** 把已通过 schema 校验的表单值转换成 API 载荷；构建变量只在用户修改过时提交，避免覆盖未加载的服务端值。 */
export function deploymentTargetFormToPayload(values: DeploymentTargetFormValues, options: { includeBuildEnvironment: boolean }): DeploymentTargetPayload {
  const repository = values.sourceType === 'repository'
  const template = repository && values.buildDefinitionMode === 'template'
  const payload: DeploymentTargetPayload = {
    name: values.name.trim(),
    stage: values.stage,
    clusterId: values.clusterId.trim(),
    enabled: values.enabled,
    sourceType: values.sourceType,
    repositoryBindingId: repository ? values.repositoryBindingId : '',
    imageRef: repository ? '' : values.imageRef.trim(),
    targetRegistryId: repository ? values.targetRegistryId : '',
    targetImageRef: repository ? values.targetImageRef.trim() : '',
    targetRepository: values.targetRepository,
    targetTag: values.targetTag,
    buildDefinitionMode: values.buildDefinitionMode,
    buildTemplateId: template ? values.buildTemplateId.trim() : '',
    buildTemplateVersion: template ? values.buildTemplateVersion.trim() : '',
    buildTemplateValues: template ? (values.buildTemplateValues.trim() || '{}') : '{}',
    dockerfilePath: values.dockerfilePath.trim(),
    buildContext: values.buildContext.trim(),
    buildDirectory: values.buildDirectory.trim(),
    buildArgs: repository ? values.buildArgs.trim() : '',
    buildEnvironmentId: values.buildEnvironmentId,
    buildCpuRequest: values.buildCpuRequest,
    buildMemoryRequest: values.buildMemoryRequest,
    buildTimeoutSeconds: values.buildTimeoutMinutes * 60,
    buildVariableSetIds: values.buildVariableSetIds,
    replicas: values.replicas,
    cpuRequest: values.cpuRequest,
    memoryRequest: values.memoryRequest,
    autoScalingEnabled: values.autoScalingEnabled,
    autoScalingMinReplicas: values.autoScalingMinReplicas,
    autoScalingMaxReplicas: values.autoScalingMaxReplicas,
    autoScalingCpuPercent: values.autoScalingCpuPercent,
    autoScalingMemoryPercent: values.autoScalingMemoryPercent,
    autoScalingBehavior: values.autoScalingBehavior.trim(),
    servicePorts: values.servicePorts.map(port => ({ name: port.name.trim(), port: port.port, appProtocol: port.appProtocol.trim() })),
    serviceSessionAffinity: values.serviceSessionAffinity,
    serviceAnnotations: values.serviceAnnotations.trim(),
    runtimeConfigRefs: values.runtimeConfigRefs,
    environmentVariables: environmentRowsToPayload(values.environmentVariables),
    configFiles: values.configFiles.trim(),
    secretFiles: values.secretFiles.trim(),
    dataVolumes: serializeRuntimeDataVolumes(values.dataVolumes),
    workloadType: values.workloadType,
    imagePullPolicy: values.imagePullPolicy,
    containerCommand: values.containerCommand.trim(),
    containerArgs: values.containerArgs.trim(),
    lifecycle: values.lifecycle.trim(),
    readinessProbe: values.readinessProbe.trim(),
    livenessProbe: values.livenessProbe.trim(),
    startupProbe: values.startupProbe.trim(),
    autoDeploy: values.autoDeploy,
    branchPattern: values.branchPattern.trim(),
    tagPattern: values.tagPattern.trim(),
    concurrencyPolicy: values.concurrencyPolicy,
    buildHooksEnabled: values.buildHooksEnabled,
    buildHookBindings: values.buildHookBindings.map((binding, index) => ({ hookConfigId: binding.hookConfigId, phase: binding.phase, runOrder: index + 1 })),
    webConsoleEnabled: values.webConsoleEnabled === 'disabled' ? false : null,
    runAsUser: values.runAsUser.trim(),
    runAsGroup: values.runAsGroup.trim(),
    fsGroup: values.fsGroup.trim(),
    fsGroupChangePolicy: values.fsGroupChangePolicy,
    readOnlyRootFilesystem: values.readOnlyRootFilesystem,
    capabilityDrop: values.capabilityDrop.trim(),
    nodeSelector: values.nodeSelector.trim(),
    tolerations: values.tolerations.trim(),
    affinity: values.affinity.trim(),
    topologySpreadConstraints: values.topologySpreadConstraints.trim(),
    priorityClassName: values.priorityClassName.trim(),
    initContainers: values.initContainers.trim(),
    sidecarContainers: values.sidecarContainers.trim(),
    requireApproval: values.requireApproval,
  }
  if (!options.includeBuildEnvironment)
    return payload
  return {
    ...payload,
    buildVariables: buildVariableRowsToRecord(values.buildVariables),
    buildSecrets: buildVariableRowsToRecord(values.buildSecrets),
  }
}

/** 修改后需要重新发布才能在运行中工作负载生效的字段。副本数由平台直接伸缩，不在其中。 */
const restartRequiredFields = [
  'clusterId',
  'workloadType',
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
] as const satisfies ReadonlyArray<keyof DeploymentTargetPayload>

export function deploymentTargetNeedsRedeploy(baseline: DeploymentTargetPayload, next: DeploymentTargetPayload) {
  if (next.secretFiles?.trim())
    return true
  const fields: Array<keyof DeploymentTargetPayload> = [...restartRequiredFields]
  if (next.sourceType === 'image')
    fields.push('imageRef')
  if (fields.some(field => JSON.stringify(baseline[field]) !== JSON.stringify(next[field])))
    return true
  return JSON.stringify(deploymentPhaseHooks(baseline)) !== JSON.stringify(deploymentPhaseHooks(next))
}

function deploymentPhaseHooks(payload: DeploymentTargetPayload) {
  return payload.buildHookBindings
    .filter(binding => !buildHookPhases.has(binding.phase))
    .map(binding => ({ hookConfigId: binding.hookConfigId, phase: binding.phase, runOrder: binding.runOrder }))
}

/** 各分区字段归属：用于导航上的错误计数和提交失败后的定位。 */
export const deploymentTargetSectionFields: Record<DeploymentTargetEditorSection, readonly DeploymentTargetFieldName[]> = {
  basics: ['name', 'stage', 'clusterId', 'enabled'],
  source: ['sourceType', 'repositoryBindingId', 'imageRef', 'targetRegistryId', 'targetImageRef', 'buildDefinitionMode', 'buildTemplateId', 'buildTemplateVersion', 'buildTemplateValues', 'dockerfilePath', 'buildContext', 'buildDirectory', 'buildArgs', 'buildCpuRequest', 'buildMemoryRequest', 'buildTimeoutMinutes', 'buildVariables', 'buildSecrets'],
  runtime: ['replicas', 'cpuRequest', 'memoryRequest', 'autoScalingEnabled', 'autoScalingMinReplicas', 'autoScalingMaxReplicas', 'autoScalingCpuPercent', 'autoScalingMemoryPercent', 'autoScalingBehavior', 'servicePorts', 'serviceSessionAffinity', 'serviceAnnotations', 'runtimeConfigRefs', 'environmentVariables', 'configFiles', 'secretFiles', 'dataVolumes', 'workloadType', 'imagePullPolicy', 'containerCommand', 'containerArgs', 'lifecycle', 'readinessProbe', 'livenessProbe', 'startupProbe'],
  release: ['autoDeploy', 'branchPattern', 'tagPattern', 'concurrencyPolicy'],
  hooks: ['buildHooksEnabled', 'buildHookBindings'],
  security: ['webConsoleEnabled', 'runAsUser', 'runAsGroup', 'fsGroup', 'fsGroupChangePolicy', 'readOnlyRootFilesystem', 'capabilityDrop', 'nodeSelector', 'tolerations', 'affinity', 'topologySpreadConstraints', 'priorityClassName', 'initContainers', 'sidecarContainers'],
}

export const deploymentTargetSectionOrder: readonly DeploymentTargetEditorSection[] = ['basics', 'source', 'runtime', 'release', 'hooks', 'security']

/** 统计与平台默认值不同的字段数量，用于“高级选项已设置 N 项”。 */
export function changedFieldCount(values: Partial<Record<DeploymentTargetFieldName, unknown>>, fields: readonly DeploymentTargetFieldName[]) {
  const defaults = emptyDeploymentTargetFormValues()
  return fields.filter(field => JSON.stringify(values[field]) !== JSON.stringify(defaults[field])).length
}

/** 后端字段错误码到表单字段的映射；其余错误以 toast 呈现。 */
export const deploymentTargetErrorFields: Record<string, DeploymentTargetFieldName> = {
  'deployment_target.repository_binding_required': 'repositoryBindingId',
  'deployment_target.repository_binding_not_found': 'repositoryBindingId',
  'deployment_target.image_ref_required': 'imageRef',
  'deployment_target.replicas_invalid': 'replicas',
}

export type RuntimeEnvironmentRow = DeploymentTargetFormValues['environmentVariables'][number]

/** 统一键值表转 API 载荷：普通行给出完整期望集合；加密行只提交显式操作，未改动的已有密钥不出现在载荷中。 */
export function environmentRowsToPayload(rows: RuntimeEnvironmentRow[]): DeploymentRuntimeEnvironmentVariableInput[] {
  const items: DeploymentRuntimeEnvironmentVariableInput[] = []
  for (const row of rows) {
    const key = row.key.trim()
    if (row.mode === 'public') {
      items.push({ key, valueMode: 'public', value: row.value })
      // 加密转普通：同一次保存先清除原密钥，再写入重新填写的普通值。
      if (row.stored === 'secret')
        items.push({ key, valueMode: 'secret', secretOperation: 'clear' })
      continue
    }
    if (row.secretAction === 'set')
      items.push({ key, valueMode: 'secret', secretOperation: 'set', value: row.value })
    else if (row.secretAction === 'generate')
      items.push({ key, valueMode: 'secret', secretOperation: 'generate' })
    else if (row.secretAction === 'clear' && row.stored === 'secret')
      items.push({ key, valueMode: 'secret', secretOperation: 'clear' })
  }
  return items
}

/** 统计保存时将执行的加密操作，用于提交前的汇总提示。 */
export function pendingSecretOperations(rows: RuntimeEnvironmentRow[]) {
  const counts = { cleared: 0, generated: 0, set: 0 }
  for (const row of rows) {
    if (row.mode === 'public' && row.stored === 'secret')
      counts.cleared++
    else if (row.mode === 'secret' && row.secretAction === 'clear' && row.stored === 'secret')
      counts.cleared++
    else if (row.mode === 'secret' && row.secretAction === 'generate')
      counts.generated++
    else if (row.mode === 'secret' && row.secretAction === 'set')
      counts.set++
  }
  return counts
}

function oneOf<const T extends string>(value: string, allowed: readonly T[], fallback: T): T {
  return (allowed as readonly string[]).includes(value) ? value as T : fallback
}

function jsonTextMatches(value: string, kind: 'object' | 'array') {
  const trimmed = value.trim()
  if (!trimmed)
    return true
  try {
    const parsed: unknown = JSON.parse(trimmed)
    return kind === 'array' ? Array.isArray(parsed) : Boolean(parsed) && typeof parsed === 'object' && !Array.isArray(parsed)
  }
  catch {
    return false
  }
}

function stringListTextIsValid(value: string) {
  const trimmed = value.trim()
  if (!trimmed.startsWith('['))
    return true
  try {
    const parsed: unknown = JSON.parse(trimmed)
    return Array.isArray(parsed) && parsed.every(item => typeof item === 'string')
  }
  catch {
    return false
  }
}

function keyValueOrJsonTextIsValid(value: string) {
  const trimmed = value.trim()
  if (!trimmed)
    return true
  if (trimmed.startsWith('{'))
    return jsonTextMatches(trimmed, 'object')
  return trimmed.split('\n').every((line) => {
    const text = line.trim()
    if (!text || text.startsWith('#'))
      return true
    const separator = text.indexOf('=')
    return separator > 0
  })
}
