import type { TFunction } from 'i18next'
import type { DeploymentTargetFormValues, RuntimeEnvironmentRow } from '../deployment-target-form'
import type { DeploymentTargetConfigInferenceOutput } from '@/api'
import { maxRuntimeEnvironmentVariables, maxServicePorts } from '../deployment-target-form'

export type AiInferDiffFieldKey
  = | 'buildDefinitionMode'
    | 'dockerfilePath'
    | 'buildTemplate'
    | 'buildTemplateValues'
    | 'buildContext'
    | 'buildDirectory'
    | 'buildArgs'
    | 'servicePorts'
    | 'cpuRequest'
    | 'memoryRequest'
    | 'environmentVariables'
    | 'containerCommand'
    | 'containerArgs'

export interface AiInferDiffItem {
  after: string
  before: string
  field: AiInferDiffFieldKey
}

export interface AiInferDraftPlan {
  diffItems: AiInferDiffItem[]
  needsAdvancedExpand: boolean
  patch: Partial<DeploymentTargetFormValues>
}

function formatPortsSummary(ports: Array<{ appProtocol: string, name: string, port: number }>) {
  return ports.map(item => `${item.name}:${item.port}/${item.appProtocol || 'http'}`).join(', ')
}

function formatBuildArgsText(args: DeploymentTargetConfigInferenceOutput['buildArgs']) {
  return args.map(item => `${item.key}=${item.value}`).join('\n')
}

function formatTemplateValuesText(values: Record<string, string>) {
  const keys = Object.keys(values).sort()
  if (keys.length === 0)
    return '{}'
  const sorted: Record<string, string> = {}
  for (const key of keys)
    sorted[key] = values[key] ?? ''
  return JSON.stringify(sorted, null, 2)
}

function formatModeLabel(mode: 'repository_dockerfile' | 'template', t: TFunction) {
  return mode === 'template'
    ? t('deploymentTargetEditor.aiInfer.modeTemplate')
    : t('deploymentTargetEditor.aiInfer.modeDockerfile')
}

export function aiInferFieldLabel(field: AiInferDiffFieldKey, t: TFunction) {
  switch (field) {
    case 'buildDefinitionMode':
      return t('deploymentTargetEditor.aiInfer.fields.buildDefinitionMode')
    case 'dockerfilePath':
      return t('deploymentTargetEditor.aiInfer.fields.dockerfilePath')
    case 'buildTemplate':
      return t('deploymentTargetEditor.aiInfer.fields.buildTemplate')
    case 'buildTemplateValues':
      return t('deploymentTargetEditor.aiInfer.fields.buildTemplateValues')
    case 'buildContext':
      return t('deploymentTargetEditor.aiInfer.fields.buildContext')
    case 'buildDirectory':
      return t('deploymentTargetEditor.aiInfer.fields.buildDirectory')
    case 'buildArgs':
      return t('deploymentTargetEditor.aiInfer.fields.buildArgs')
    case 'servicePorts':
      return t('deploymentTargetEditor.aiInfer.fields.servicePorts')
    case 'cpuRequest':
      return t('deploymentTargetEditor.aiInfer.fields.cpuRequest')
    case 'memoryRequest':
      return t('deploymentTargetEditor.aiInfer.fields.memoryRequest')
    case 'environmentVariables':
      return t('deploymentTargetEditor.aiInfer.fields.environmentVariables')
    case 'containerCommand':
      return t('deploymentTargetEditor.aiInfer.fields.containerCommand')
    case 'containerArgs':
      return t('deploymentTargetEditor.aiInfer.fields.containerArgs')
  }
}

/** 计算 AI 推断输出回填到当前部署配置草稿时的差异与精确补丁，不覆盖已配置的环境变量或密钥。 */
export function buildAiInferDraftPlan(
  current: DeploymentTargetFormValues,
  inferred: DeploymentTargetConfigInferenceOutput,
  t: TFunction,
): AiInferDraftPlan {
  const emptyText = t('deploymentTargetEditor.aiInfer.emptyValue')
  const displayOrEmpty = (value: string) => value.trim() ? value.trim() : emptyText
  const patch: Partial<DeploymentTargetFormValues> = {}
  const diffItems: AiInferDiffItem[] = []

  if (current.buildDefinitionMode !== inferred.buildDefinitionMode) {
    patch.buildDefinitionMode = inferred.buildDefinitionMode
    diffItems.push({
      field: 'buildDefinitionMode',
      before: formatModeLabel(current.buildDefinitionMode, t),
      after: formatModeLabel(inferred.buildDefinitionMode, t),
    })
  }

  if (inferred.buildDefinitionMode === 'repository_dockerfile') {
    const nextDockerfile = inferred.dockerfilePath.trim() || 'Dockerfile'
    if (current.dockerfilePath.trim() !== nextDockerfile) {
      patch.dockerfilePath = nextDockerfile
      diffItems.push({
        field: 'dockerfilePath',
        before: displayOrEmpty(current.dockerfilePath),
        after: nextDockerfile,
      })
    }
  }
  else {
    const nextTemplateId = inferred.buildTemplateId.trim()
    const nextTemplateVersion = inferred.buildTemplateVersion.trim()
    const beforeTemplate = current.buildTemplateId.trim()
      ? `${current.buildTemplateId.trim()}@${current.buildTemplateVersion || '-'}`
      : emptyText
    const afterTemplate = nextTemplateId
      ? `${nextTemplateId}@${nextTemplateVersion || '-'}`
      : emptyText
    if (current.buildTemplateId.trim() !== nextTemplateId || current.buildTemplateVersion.trim() !== nextTemplateVersion) {
      patch.buildTemplateId = nextTemplateId
      patch.buildTemplateVersion = nextTemplateVersion
      diffItems.push({
        field: 'buildTemplate',
        before: beforeTemplate,
        after: afterTemplate,
      })
    }
    const nextValuesText = formatTemplateValuesText(inferred.buildTemplateValues)
    if (current.buildTemplateValues.trim() !== nextValuesText.trim()) {
      patch.buildTemplateValues = nextValuesText
      diffItems.push({
        field: 'buildTemplateValues',
        before: displayOrEmpty(current.buildTemplateValues),
        after: nextValuesText,
      })
    }
  }

  const nextContext = inferred.buildContext.trim() || '.'
  if (current.buildContext.trim() !== nextContext) {
    patch.buildContext = nextContext
    diffItems.push({
      field: 'buildContext',
      before: displayOrEmpty(current.buildContext),
      after: nextContext,
    })
  }

  const nextBuildDir = inferred.buildDirectory.trim()
  if (current.buildDirectory.trim() !== nextBuildDir && (nextBuildDir !== '' || current.buildDirectory.trim() !== '')) {
    patch.buildDirectory = nextBuildDir
    diffItems.push({
      field: 'buildDirectory',
      before: displayOrEmpty(current.buildDirectory),
      after: displayOrEmpty(nextBuildDir),
    })
  }

  const nextBuildArgs = formatBuildArgsText(inferred.buildArgs)
  if (nextBuildArgs && current.buildArgs.trim() !== nextBuildArgs) {
    patch.buildArgs = nextBuildArgs
    diffItems.push({
      field: 'buildArgs',
      before: displayOrEmpty(current.buildArgs),
      after: nextBuildArgs,
    })
  }

  if (inferred.servicePorts.length > 0) {
    const nextPorts = inferred.servicePorts.slice(0, maxServicePorts).map(item => ({
      name: item.name,
      port: item.port,
      appProtocol: item.appProtocol || 'http',
    }))
    const beforeSummary = formatPortsSummary(current.servicePorts)
    const afterSummary = formatPortsSummary(nextPorts)
    if (beforeSummary !== afterSummary) {
      patch.servicePorts = nextPorts
      diffItems.push({
        field: 'servicePorts',
        before: displayOrEmpty(beforeSummary),
        after: afterSummary,
      })
    }
  }

  if (inferred.cpuRequest.trim() && current.cpuRequest.trim() !== inferred.cpuRequest.trim()) {
    patch.cpuRequest = inferred.cpuRequest.trim()
    diffItems.push({
      field: 'cpuRequest',
      before: displayOrEmpty(current.cpuRequest),
      after: inferred.cpuRequest.trim(),
    })
  }

  if (inferred.memoryRequest.trim() && current.memoryRequest.trim() !== inferred.memoryRequest.trim()) {
    patch.memoryRequest = inferred.memoryRequest.trim()
    diffItems.push({
      field: 'memoryRequest',
      before: displayOrEmpty(current.memoryRequest),
      after: inferred.memoryRequest.trim(),
    })
  }

  const existingKeys = new Set(
    current.environmentVariables
      .filter(row => row.key.trim() !== '' && !(row.mode === 'secret' && row.secretAction === 'clear'))
      .map(row => row.key.trim()),
  )
  const addedEnvRows: RuntimeEnvironmentRow[] = []
  const addedEnvLabels: string[] = []
  for (const item of inferred.environmentVariables) {
    const key = item.key.trim()
    if (!key || existingKeys.has(key) || current.environmentVariables.length + addedEnvRows.length >= maxRuntimeEnvironmentVariables)
      continue
    existingKeys.add(key)
    if (item.secret) {
      addedEnvRows.push({
        key,
        mode: 'secret',
        value: '',
        stored: 'none',
        secretAction: 'set',
      })
      addedEnvLabels.push(`${key}=${t('deploymentTargetEditor.aiInfer.secretPendingPlaceholder')}`)
    }
    else {
      addedEnvRows.push({
        key,
        mode: 'public',
        value: item.placeholder,
        stored: 'none',
        secretAction: 'keep',
      })
      addedEnvLabels.push(`${key}=${item.placeholder || emptyText}`)
    }
  }
  if (addedEnvRows.length > 0) {
    patch.environmentVariables = [...current.environmentVariables, ...addedEnvRows]
    diffItems.push({
      field: 'environmentVariables',
      before: emptyText,
      after: addedEnvLabels.join('\n'),
    })
  }

  if (inferred.containerCommand.length > 0) {
    const nextCommand = JSON.stringify(inferred.containerCommand)
    if (current.containerCommand.trim() !== nextCommand) {
      patch.containerCommand = nextCommand
      diffItems.push({
        field: 'containerCommand',
        before: displayOrEmpty(current.containerCommand),
        after: nextCommand,
      })
    }
  }

  if (inferred.containerArgs.length > 0) {
    const nextArgs = JSON.stringify(inferred.containerArgs)
    if (current.containerArgs.trim() !== nextArgs) {
      patch.containerArgs = nextArgs
      diffItems.push({
        field: 'containerArgs',
        before: displayOrEmpty(current.containerArgs),
        after: nextArgs,
      })
    }
  }

  const needsAdvancedExpand = Boolean(
    patch.buildDirectory !== undefined
    || patch.buildArgs !== undefined
    || patch.containerCommand !== undefined
    || patch.containerArgs !== undefined,
  )

  return { diffItems, needsAdvancedExpand, patch }
}
