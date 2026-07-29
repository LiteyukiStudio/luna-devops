export const aiInternalRouteNames = [
  'dashboard',
  'projects',
  'project.workspace',
  'application.detail',
  'events',
  'code-repositories',
  'registries',
  'clusters',
  'app-templates',
  'billing',
  'settings.account',
  'settings.auth-providers',
  'settings.notifications',
  'settings.operations',
  'settings.site',
  'settings.users',
] as const

export type AIInternalRouteName = typeof aiInternalRouteNames[number]

const identifierPattern = /^[\w.:-]{1,160}$/
const dynamicPathPatterns: readonly RegExp[] = [
  /^\/projects\/[\w.:-]{1,160}$/,
  /^\/projects\/[\w.:-]{1,160}\/apps\/[\w.:-]{1,160}$/,
]
const staticPaths = new Set([
  '/dashboard',
  '/projects',
  '/events',
  '/code-repositories',
  '/registries',
  '/clusters',
  '/app-templates',
  '/billing',
  '/settings/account',
  '/settings/auth-providers',
  '/settings/notifications',
  '/settings/operations',
  '/settings/site',
  '/settings/users',
])

const routeBuilders: Record<AIInternalRouteName, (params: Record<string, string>) => string | null> = {
  'dashboard': () => '/dashboard',
  'projects': () => '/projects',
  'project.workspace': params => params.projectId ? `/projects/${encodeURIComponent(params.projectId)}` : null,
  'application.detail': params => params.projectId && params.applicationId ? `/projects/${encodeURIComponent(params.projectId)}/apps/${encodeURIComponent(params.applicationId)}` : null,
  'events': () => '/events',
  'code-repositories': () => '/code-repositories',
  'registries': () => '/registries',
  'clusters': () => '/clusters',
  'app-templates': () => '/app-templates',
  'billing': () => '/billing',
  'settings.account': () => '/settings/account',
  'settings.auth-providers': () => '/settings/auth-providers',
  'settings.notifications': () => '/settings/notifications',
  'settings.operations': () => '/settings/operations',
  'settings.site': () => '/settings/site',
  'settings.users': () => '/settings/users',
}

export function buildAIInternalRoute(routeName: AIInternalRouteName, params: Record<string, string>, query: Record<string, string>) {
  if (!Object.values(params).every(value => identifierPattern.test(value)))
    return null
  const path = routeBuilders[routeName](params)
  if (!path)
    return null
  const search = new URLSearchParams(query)
  return search.size ? `${path}?${search}` : path
}

export function normalizeAIInternalHref(href: string | undefined) {
  if (!href || !href.startsWith('/') || href.startsWith('//') || href.includes('\\'))
    return null
  try {
    const url = new URL(href, 'https://luna.invalid')
    if (url.origin !== 'https://luna.invalid' || !isRegisteredPath(url.pathname))
      return null
    if (!hasSafeParameters(url.searchParams) || !hasSafeHash(url.hash))
      return null
    return `${url.pathname}${url.search}${url.hash}`
  }
  catch {
    return null
  }
}

export function normalizeAIExternalHref(href: string | undefined) {
  if (!href)
    return null
  try {
    const url = new URL(href)
    return url.protocol === 'https:' || url.protocol === 'http:' ? url.href : null
  }
  catch {
    return null
  }
}

function isRegisteredPath(pathname: string) {
  return staticPaths.has(pathname) || dynamicPathPatterns.some(pattern => pattern.test(pathname))
}

function hasSafeParameters(params: URLSearchParams) {
  const entries = [...params.entries()]
  return entries.length <= 20 && entries.every(([key, value]) =>
    /^[a-z][\w.-]{0,63}$/i.test(key)
    && value.length <= 200
    && !hasControlCharacters(value),
  )
}

function hasSafeHash(hash: string) {
  if (!hash)
    return true
  if (!hash.startsWith('#') || hash.length > 1000)
    return false
  return hasSafeParameters(new URLSearchParams(hash.slice(1)))
}

function hasControlCharacters(value: string) {
  return [...value].some((character) => {
    const code = character.codePointAt(0) ?? 0
    return code <= 31 || code === 127
  })
}
