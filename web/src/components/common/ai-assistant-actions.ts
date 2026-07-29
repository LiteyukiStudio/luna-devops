import type { QueryClient } from '@tanstack/react-query'
import type { NavigateFunction } from 'react-router-dom'
import type { AIUIAction } from '@/api'
import { z } from 'zod'

export interface AIActionContext {
  pathname: string
  search: string
  navigate: NavigateFunction
  queryClient: QueryClient
}

const identifiers = z.record(z.string(), z.string().regex(/^[\w.:-]{1,160}$/)).default({})
const routeSchema = z.object({
  routeName: z.enum(['dashboard', 'projects', 'project.workspace', 'application.detail', 'events', 'clusters', 'registries', 'billing']),
  params: identifiers,
  query: identifiers,
})

const routeBuilders: Record<z.infer<typeof routeSchema>['routeName'], (params: Record<string, string>) => string | null> = {
  'dashboard': () => '/dashboard',
  'projects': () => '/projects',
  'project.workspace': params => params.projectId ? `/projects/${encodeURIComponent(params.projectId)}` : null,
  'application.detail': params => params.projectId && params.applicationId ? `/projects/${encodeURIComponent(params.projectId)}/apps/${encodeURIComponent(params.applicationId)}` : null,
  'events': () => '/events',
  'clusters': () => '/clusters',
  'registries': () => '/registries',
  'billing': () => '/billing',
}

const tabSchema = z.object({ tabId: z.enum(['overview', 'apps', 'members', 'builds', 'deployments', 'gateway', 'runtime']) })
const filterSchema = z.object({
  targetId: z.enum(['events']),
  values: identifiers,
})
const refreshSchema = z.object({ queryKeyId: z.enum(['projects', 'events', 'applications', 'build-runs', 'releases', 'runtime']) })
const highlightSchema = z.object({ resourceId: z.string().regex(/^[\w.:-]{1,160}$/) })

function withQuery(path: string, query: Record<string, string>) {
  const search = new URLSearchParams(query)
  return search.size ? `${path}?${search}` : path
}

export async function executeAIUIAction(action: AIUIAction, context: AIActionContext) {
  if (action.version !== 1)
    return false
  if (action.type === 'navigate') {
    const parsed = routeSchema.safeParse(action.payload)
    if (!parsed.success)
      return false
    const path = routeBuilders[parsed.data.routeName](parsed.data.params)
    if (!path)
      return false
    context.navigate(withQuery(path, parsed.data.query))
    return true
  }
  if (action.type === 'select_tab') {
    const parsed = tabSchema.safeParse(action.payload)
    if (!parsed.success || !context.pathname.startsWith('/projects/'))
      return false
    const search = new URLSearchParams(context.search)
    search.set('tab', parsed.data.tabId)
    context.navigate(`${context.pathname}?${search}`)
    return true
  }
  if (action.type === 'set_filters') {
    const parsed = filterSchema.safeParse(action.payload)
    if (!parsed.success || context.pathname !== '/events')
      return false
    const search = new URLSearchParams(context.search)
    Object.entries(parsed.data.values).forEach(([key, value]) => search.set(key, value))
    context.navigate(`${context.pathname}?${search}`)
    return true
  }
  if (action.type === 'refresh_query') {
    const parsed = refreshSchema.safeParse(action.payload)
    if (!parsed.success)
      return false
    await context.queryClient.invalidateQueries({ queryKey: [parsed.data.queryKeyId] })
    return true
  }
  if (action.type === 'highlight')
    return highlightSchema.safeParse(action.payload).success
  return false
}
