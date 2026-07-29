import type { AIInternalRouteName } from './internal-routes'

interface PageContextOptions {
  hash?: string
  now?: Date
  timeZone?: string
}

interface RouteContext {
  routeName: AIInternalRouteName | 'unknown'
  routeTemplate: string
  pageKind: 'dashboard' | 'collection' | 'project' | 'application' | 'settings' | 'unknown'
  projectId?: string
  applicationId?: string
  availableTabs?: string[]
  relatedRouteNames: AIInternalRouteName[]
}

const identifierPattern = /^[\w.:-]{1,160}$/
const safeViewKeys = new Set(['tab', 'buildRunId', 'releaseId', 'deploymentId', 'page', 'sortBy', 'sortOrder'])
const projectTabs = ['overview', 'apps', 'members', 'build-variables', 'runtime-configs', 'hooks', 'topology']
const applicationTabs = ['overview', 'repositories', 'builds', 'deployments', 'gateway', 'topology', 'settings']

export function buildAIPageContext(pathname: string, search: string, locale: string, options: PageContextOptions = {}) {
  const route = resolveRouteContext(pathname)
  const query = safeViewState(new URLSearchParams(search))
  const hash = safeViewState(new URLSearchParams(options.hash?.replace(/^#/, '') ?? ''))
  const activeTab = query.tab ?? hash.tab
  const selectedResourceIds = ['buildRunId', 'releaseId', 'deploymentId']
    .flatMap(key => query[key] ?? hash[key] ?? [])
    .filter(value => identifierPattern.test(value))
  const now = options.now ?? new Date()
  const timeZone = options.timeZone ?? resolvedTimeZone()

  return {
    schemaVersion: 1,
    routeName: route.routeName,
    routeTemplate: route.routeTemplate,
    pathname,
    pageKind: route.pageKind,
    ...(route.projectId ? { projectId: route.projectId } : {}),
    ...(route.applicationId ? { applicationId: route.applicationId } : {}),
    ...(activeTab ? { activeTab } : {}),
    view: {
      query,
      hash,
      selectedResourceIds,
      availableTabs: route.availableTabs ?? [],
    },
    navigation: {
      relatedRouteNames: route.relatedRouteNames,
    },
    client: {
      locale,
      timeZone,
      timestamp: now.toISOString(),
      utcOffsetMinutes: -now.getTimezoneOffset(),
    },
    locale,
  }
}

const staticRoutes: Record<string, RouteContext> = {
  '/dashboard': route('dashboard', 'dashboard', ['projects', 'events']),
  '/projects': route('projects', 'collection', ['dashboard', 'events']),
  '/events': route('events', 'collection', ['dashboard', 'projects']),
  '/code-repositories': route('code-repositories', 'collection', ['projects', 'registries']),
  '/registries': route('registries', 'collection', ['code-repositories', 'clusters']),
  '/clusters': route('clusters', 'collection', ['projects', 'events']),
  '/app-templates': route('app-templates', 'collection', ['projects', 'registries']),
  '/billing': route('billing', 'collection', ['dashboard', 'settings.operations']),
  '/settings/account': route('settings.account', 'settings', ['settings.notifications']),
  '/settings/auth-providers': route('settings.auth-providers', 'settings', ['settings.users', 'settings.site']),
  '/settings/notifications': route('settings.notifications', 'settings', ['events', 'settings.account']),
  '/settings/operations': route('settings.operations', 'settings', ['dashboard', 'events']),
  '/settings/site': route('settings.site', 'settings', ['settings.users', 'settings.auth-providers']),
  '/settings/users': route('settings.users', 'settings', ['settings.auth-providers', 'settings.site']),
}

function route(routeName: AIInternalRouteName, pageKind: RouteContext['pageKind'], relatedRouteNames: AIInternalRouteName[]): RouteContext {
  return { routeName, routeTemplate: `/${routeName.replaceAll('.', '/')}`, pageKind, relatedRouteNames }
}

function resolveRouteContext(pathname: string): RouteContext {
  const applicationMatch = pathname.match(/^\/projects\/([^/]+)\/apps\/([^/]+)$/)
  if (applicationMatch?.[1] && applicationMatch[2]) {
    return {
      routeName: 'application.detail',
      routeTemplate: '/projects/:projectId/apps/:applicationId',
      pageKind: 'application',
      projectId: applicationMatch[1],
      applicationId: applicationMatch[2],
      availableTabs: applicationTabs,
      relatedRouteNames: ['project.workspace', 'events'],
    }
  }
  const projectMatch = pathname.match(/^\/projects\/([^/]+)$/)
  if (projectMatch?.[1]) {
    return {
      routeName: 'project.workspace',
      routeTemplate: '/projects/:projectId',
      pageKind: 'project',
      projectId: projectMatch[1],
      availableTabs: projectTabs,
      relatedRouteNames: ['projects', 'events'],
    }
  }
  const staticRoute = staticRoutes[pathname]
  return staticRoute ?? {
    routeName: 'unknown',
    routeTemplate: '',
    pageKind: 'unknown',
    relatedRouteNames: ['dashboard', 'projects'],
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
