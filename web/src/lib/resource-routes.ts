import type { BuildRun, PlatformEvent } from '@/api'

type ApplicationTab = 'builds' | 'deployments' | 'gateway'

/** 应用详情页路径；tab 与 hash 对应应用页 `?tab=` 和构建列表的 `#buildRunId=` 定位。 */
export function applicationPath(projectId: string, applicationId: string, tab?: ApplicationTab, hash?: Record<string, string>) {
  const path = `/projects/${encodeURIComponent(projectId)}/apps/${encodeURIComponent(applicationId)}`
  const query = tab ? `?tab=${tab}` : ''
  const fragment = hash ? `#${new URLSearchParams(hash).toString()}` : ''
  return `${path}${query}${fragment}`
}

/** 构建运行所在应用的构建列表，并定位到该运行。 */
export function buildRunPath(build: Pick<BuildRun, 'applicationId' | 'id' | 'projectId'>) {
  return applicationPath(build.projectId, build.applicationId, 'builds', { buildRunId: build.id })
}

/** 事件中心中打开指定事件详情，用于无法定位到具体资源页面的事件。 */
export function platformEventDetailPath(event: Pick<PlatformEvent, 'id' | 'projectId'>) {
  const search = new URLSearchParams({ eventId: event.id })
  if (event.projectId)
    search.set('projectId', event.projectId)
  return `/events?${search.toString()}`
}

/** 事件中心按项目空间与事件分类筛选。 */
export function projectEventsPath(projectId: string, category?: string) {
  const search = new URLSearchParams({ projectId })
  if (category)
    search.set('category', category)
  return `/events?${search.toString()}`
}

/**
 * 事件主要资源在控制台中的页面；资源类型来自事件生产方的稳定 resourceType。
 * 无法定位到具体页面时返回 null，由调用方决定是否回退到事件详情。
 */
export function platformEventResourcePath(event: Pick<PlatformEvent, 'applicationId' | 'projectId' | 'resourceId' | 'resourceType'>) {
  const { applicationId, projectId, resourceId, resourceType } = event
  if (!projectId)
    return null
  if (resourceType === 'service_binding')
    return `/projects/${encodeURIComponent(projectId)}?tab=topology`
  if (!applicationId)
    return null
  switch (resourceType) {
    case 'build':
      return resourceId ? buildRunPath({ applicationId, id: resourceId, projectId }) : applicationPath(projectId, applicationId, 'builds')
    case 'repository_binding':
      return applicationPath(projectId, applicationId, 'builds')
    case 'deployment_target':
      return resourceId
        ? applicationPath(projectId, applicationId, 'deployments', { targetId: resourceId })
        : applicationPath(projectId, applicationId, 'deployments')
    case 'release':
      return resourceId
        ? applicationPath(projectId, applicationId, 'deployments', { releaseId: resourceId })
        : applicationPath(projectId, applicationId, 'deployments')
    case 'hook':
      return applicationPath(projectId, applicationId, 'deployments')
    case 'gateway':
    case 'certificate':
      return applicationPath(projectId, applicationId, 'gateway')
    default:
      return applicationPath(projectId, applicationId)
  }
}
