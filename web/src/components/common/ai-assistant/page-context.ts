import type { AIInternalRouteName } from './internal-routes'

interface PageContextOptions {
  hash?: string
  timeZone?: string
}

type AIPageRouteName = AIInternalRouteName | 'ai-assistant' | 'inbox' | 'notification-channels' | 'settings' | 'unknown'
type AIPageKind = 'dashboard' | 'collection' | 'project' | 'application' | 'settings' | 'ai-assistant' | 'unknown'

interface RouteContext {
  routeName: AIPageRouteName
  routeTemplate: string
  pageKind: AIPageKind
  projectId?: string
  applicationId?: string
  targetId?: string
}

const identifierPattern = /^[\w.:-]{1,160}$/
const safeViewKeys = new Set(['tab', 'config', 'view', 'buildRunId', 'releaseId', 'deploymentId', 'clusterId', 'envId', 'page', 'sortBy', 'sortOrder'])

export function buildAIPageContext(pathname: string, search: string, locale: string, options: PageContextOptions = {}) {
  const route = resolveRouteContext(pathname)
  const query = safeViewState(new URLSearchParams(search))
  const hash = safeViewState(new URLSearchParams(options.hash?.replace(/^#/, '') ?? ''))
  const activeTab = query.tab ?? hash.tab
  const clusterId = query.clusterId ?? hash.clusterId
  const envId = query.envId ?? hash.envId
  const selectedResourceIds = ['buildRunId', 'releaseId', 'deploymentId']
    .flatMap(key => query[key] ?? hash[key] ?? [])
    .filter(value => identifierPattern.test(value))
  const timeZone = options.timeZone ?? resolvedTimeZone()

  return {
    schemaVersion: 1,
    routeName: route.routeName,
    routeTemplate: route.routeTemplate,
    pathname,
    pageKind: route.pageKind,
    ...(route.projectId && identifierPattern.test(route.projectId) ? { projectId: route.projectId } : {}),
    ...(route.applicationId && identifierPattern.test(route.applicationId) ? { applicationId: route.applicationId } : {}),
    ...(route.targetId && identifierPattern.test(route.targetId) ? { targetId: route.targetId } : {}),
    ...(clusterId && identifierPattern.test(clusterId) ? { clusterId } : {}),
    ...(envId && identifierPattern.test(envId) ? { envId } : {}),
    ...(activeTab ? { activeTab } : {}),
    view: {
      query,
      hash,
      selectedResourceIds,
    },
    locale,
    timeZone,
  }
}

const staticRoutes: Record<string, RouteContext> = {
  '/ai': { routeName: 'ai-assistant', routeTemplate: '/ai', pageKind: 'ai-assistant' },
  '/ai-assistant': { routeName: 'ai-assistant', routeTemplate: '/ai-assistant', pageKind: 'ai-assistant' },
  '/dashboard': route('dashboard', 'dashboard'),
  '/projects': route('projects', 'collection'),
  '/events': route('events', 'collection'),
  '/inbox': { routeName: 'inbox', routeTemplate: '/inbox', pageKind: 'collection' },
  '/code-repositories': route('code-repositories', 'collection'),
  '/registries': route('registries', 'collection'),
  '/clusters': route('clusters', 'collection'),
  '/notification-channels': { routeName: 'notification-channels', routeTemplate: '/notification-channels', pageKind: 'collection' },
  '/app-templates': route('app-templates', 'collection'),
  '/billing': route('billing', 'collection'),
  '/settings': { routeName: 'settings', routeTemplate: '/settings', pageKind: 'settings' },
  '/settings/account': route('settings.account', 'settings'),
  '/settings/auth-providers': route('settings.auth-providers', 'settings'),
  '/settings/notifications': route('settings.notifications', 'settings'),
  '/settings/operations': route('settings.operations', 'settings'),
  '/settings/site': route('settings.site', 'settings'),
  '/settings/users': route('settings.users', 'settings'),
}

function route(routeName: AIInternalRouteName, pageKind: AIPageKind): RouteContext {
  return { routeName, routeTemplate: `/${routeName.replaceAll('.', '/')}`, pageKind }
}

function resolveRouteContext(pathname: string): RouteContext {
  const normalized = pathname.replace(/\/+$/u, '') || '/'
  if (normalized === '/ai' || normalized.startsWith('/ai/') || normalized === '/ai-assistant' || normalized.startsWith('/ai-assistant/')) {
    return {
      routeName: 'ai-assistant',
      routeTemplate: normalized.startsWith('/ai-assistant') ? '/ai-assistant' : '/ai',
      pageKind: 'ai-assistant',
    }
  }
  const deploymentEditMatch = normalized.match(/^\/projects\/([^/]+)\/apps\/([^/]+)\/deployments\/([^/]+)\/edit$/)
  if (
    deploymentEditMatch?.[1] && identifierPattern.test(deploymentEditMatch[1])
    && deploymentEditMatch[2] && identifierPattern.test(deploymentEditMatch[2])
    && deploymentEditMatch[3] && identifierPattern.test(deploymentEditMatch[3])
  ) {
    return {
      routeName: 'application.detail',
      routeTemplate: '/projects/:projectId/apps/:applicationId/deployments/:targetId/edit',
      pageKind: 'application',
      projectId: deploymentEditMatch[1],
      applicationId: deploymentEditMatch[2],
      targetId: deploymentEditMatch[3],
    }
  }
  // 部署配置新建页属于应用详情的子页面，沿用应用上下文。
  const applicationMatch = normalized.match(/^\/projects\/([^/]+)\/apps\/([^/]+)(?:\/deployments\/new)?$/)
  if (
    applicationMatch?.[1] && identifierPattern.test(applicationMatch[1])
    && applicationMatch[2] && identifierPattern.test(applicationMatch[2])
  ) {
    return {
      routeName: 'application.detail',
      routeTemplate: '/projects/:projectId/apps/:applicationId',
      pageKind: 'application',
      projectId: applicationMatch[1],
      applicationId: applicationMatch[2],
    }
  }
  const projectMatch = normalized.match(/^\/projects\/([^/]+)$/)
  if (projectMatch?.[1] && identifierPattern.test(projectMatch[1])) {
    return {
      routeName: 'project.workspace',
      routeTemplate: '/projects/:projectId',
      pageKind: 'project',
      projectId: projectMatch[1],
    }
  }
  const staticRoute = staticRoutes[normalized]
  if (staticRoute)
    return staticRoute
  const settingsMatch = normalized.match(/^\/settings\/([\w-]+)$/)
  if (settingsMatch?.[1]) {
    return {
      routeName: 'settings',
      routeTemplate: '/settings/:section',
      pageKind: 'settings',
    }
  }
  return {
    routeName: 'unknown',
    routeTemplate: '',
    pageKind: 'unknown',
  }
}

function safeViewState(params: URLSearchParams) {
  return Object.fromEntries([...params.entries()]
    .filter(([key, value]) => safeViewKeys.has(key) && value.length <= 200 && !hasControlCharacters(value))
    .slice(0, 12))
}

function resolvedTimeZone() {
  try {
    return new Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
  }
  catch {
    return 'UTC'
  }
}

function hasControlCharacters(value: string) {
  return [...value].some((character) => {
    const code = character.codePointAt(0) ?? 0
    return code <= 31 || code === 127
  })
}
