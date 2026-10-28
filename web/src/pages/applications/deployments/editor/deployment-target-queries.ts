import type { DeploymentTargetFormValues } from './deployment-target-form'
import type { RepositoryBinding } from '@/api'
import { queryOptions } from '@tanstack/react-query'
import { api } from '@/api'

/** 仓库结构探测：Dockerfile、目录和 EXPOSE 端口建议。查询键与发布入口共享缓存。 */
export function repositoryBuildOptionsQuery(binding: RepositoryBinding | undefined) {
  return queryOptions({
    queryKey: ['git-repository-build-options', binding?.gitAccountId, binding?.owner, binding?.repo, binding?.defaultBranch],
    queryFn: () => api.getGitRepositoryBuildOptions(binding?.gitAccountId ?? '', binding?.owner ?? '', binding?.repo ?? '', binding?.defaultBranch),
    enabled: Boolean(binding?.gitAccountId && binding.owner && binding.repo),
  })
}

/** 镜像站凭据的镜像模板默认值；只在新建且用户未手动修改目标镜像时使用。 */
export function targetImageTemplateQuery(params: { applicationId: string, projectId: string, registryId: string, stage: string, targetName: string }) {
  return queryOptions({
    queryKey: ['registry-image-template-default', params.registryId, params.projectId, params.applicationId, params.stage, params.targetName],
    queryFn: () => api.getRegistryImageTemplateDefault(params.registryId, {
      applicationId: params.applicationId,
      projectId: params.projectId,
      stage: params.stage,
      targetName: params.targetName,
    }),
    enabled: Boolean(params.registryId && params.projectId && params.applicationId),
  })
}

/** 按选中的 Dockerfile 推导构建上下文、工作目录和服务端口。 */
export function dockerfileBuildDefaults(
  dockerfilePath: string,
  directories: string[],
  exposedPorts: Record<string, number[]>,
): Partial<Pick<DeploymentTargetFormValues, 'buildContext' | 'buildDirectory' | 'dockerfilePath' | 'servicePorts'>> {
  const normalizedDockerfile = dockerfilePath.trim()
  if (!normalizedDockerfile)
    return {}
  const buildContext = defaultBuildContextForDockerfile(normalizedDockerfile, directories)
  const detectedPort = exposedPorts[normalizedDockerfile]?.find(port => Number.isInteger(port) && port > 0 && port <= 65535)
  return {
    dockerfilePath: normalizedDockerfile,
    buildContext,
    buildDirectory: buildContext === '.' ? '' : buildContext,
    ...(detectedPort ? { servicePorts: [{ name: 'http', port: detectedPort, appProtocol: '' }] } : {}),
  }
}

function defaultBuildContextForDockerfile(dockerfilePath: string, directories: string[]) {
  const normalized = dockerfilePath.replace(/^\/+/, '')
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
