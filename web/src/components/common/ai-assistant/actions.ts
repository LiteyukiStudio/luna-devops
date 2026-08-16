import type { QueryClient } from '@tanstack/react-query'
import type { NavigateFunction } from 'react-router-dom'
import type { AIUIAction } from '@/api'
import { aiOptionIconNames } from '@luna-devops/ai-interaction-card-contract'
import { z } from 'zod'
import { aiInternalRouteNames, buildAIInternalRoute } from './internal-routes'

export interface AIActionContext {
  pathname: string
  search: string
  navigate: NavigateFunction
  queryClient: QueryClient
  sendMessage?: (message: string) => Promise<void>
  requestTool?: (action: Extract<AIUIAction, { type: 'request_tool' }>) => Promise<void>
}

const identifiers = z.record(z.string(), z.string().regex(/^[\w.:-]{1,160}$/)).default({})
const routeSchema = z.object({
  routeName: z.enum(aiInternalRouteNames),
  params: identifiers,
  query: identifiers,
})

const tabSchema = z.object({ tabId: z.enum(['overview', 'apps', 'members', 'builds', 'deployments', 'gateway', 'runtime']) })
const filterSchema = z.object({
  targetId: z.enum(['events']),
  values: identifiers,
})
const refreshSchema = z.object({ queryKeyId: z.enum(['projects', 'events', 'applications', 'build-runs', 'releases', 'runtime']) })
const highlightSchema = z.object({ resourceId: z.string().regex(/^[\w.:-]{1,160}$/) })
const sendMessageSchema = z.object({ message: z.string().trim().min(1).max(2000) })
const requestToolSchema = z.object({
  operationId: z.string().regex(/^[a-z][\w.-]{2,100}$/i),
  arguments: z.record(z.string(), z.unknown()).optional(),
  message: z.string().trim().min(1).max(2000),
})
const optionVisualSchema = z.discriminatedUnion('type', [
  z.object({ type: z.literal('emoji'), value: z.string().trim().min(1).max(16) }),
  z.object({ type: z.literal('icon'), value: z.enum(aiOptionIconNames) }),
  z.object({ type: z.literal('img'), value: z.url().max(2048).refine(value => value.startsWith('https://')) }),
])
const optionPresentationSchema = {
  version: z.literal(1),
  id: z.string().regex(/^[\w-]{1,40}$/).optional(),
  repeatable: z.boolean().optional(),
  activation: z.literal('manual').optional(),
  label: z.string().trim().min(1).max(80).optional(),
  description: z.string().trim().max(180).optional(),
  tone: z.enum(['default', 'primary', 'danger']).optional(),
  visual: optionVisualSchema.optional(),
}
const optionActionSchema = z.discriminatedUnion('type', [
  z.object({ ...optionPresentationSchema, type: z.literal('navigate'), payload: routeSchema }),
  z.object({ ...optionPresentationSchema, type: z.literal('send_message'), payload: sendMessageSchema }),
  z.object({ ...optionPresentationSchema, type: z.literal('request_tool'), payload: requestToolSchema }),
])

export type AIOptionAction = Extract<AIUIAction, { type: 'navigate' | 'send_message' | 'request_tool' }>

export function parseAIOptionAction(value: unknown): AIOptionAction | null {
  const parsed = optionActionSchema.safeParse(value)
  if (!parsed.success)
    return null
  if (parsed.data.repeatable === true && parsed.data.type !== 'navigate')
    return null
  return parsed.data as AIOptionAction
}

export function parseAIOptionActions(values: readonly unknown[]): AIOptionAction[] {
  return values
    .map(parseAIOptionAction)
    .filter((action): action is AIOptionAction => action !== null)
    .slice(0, 5)
}

export function isAIUIActionRepeatable(action: AIUIAction): boolean {
  if (action.type === 'send_message' || action.type === 'request_tool')
    return false
  if (action.type === 'navigate')
    return action.repeatable ?? true
  return true
}

export function getAIUIActionTargetPath(action: AIUIAction): string | null {
  if (action.version !== 1 || action.type !== 'navigate')
    return null
  const parsed = routeSchema.safeParse(action.payload)
  return parsed.success
    ? buildAIInternalRoute(parsed.data.routeName, parsed.data.params, parsed.data.query)
    : null
}

export async function executeAIUIAction(action: AIUIAction, context: AIActionContext) {
  if (action.version !== 1)
    return false
  if (action.type === 'navigate') {
    const path = getAIUIActionTargetPath(action)
    if (!path)
      return false
    context.navigate(path)
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
  if (action.type === 'send_message' || action.type === 'request_tool') {
    if (action.type === 'send_message') {
      if (!context.sendMessage)
        return false
      const parsed = sendMessageSchema.safeParse(action.payload)
      if (!parsed.success)
        return false
      await context.sendMessage(parsed.data.message)
      return true
    }
    if (!context.requestTool)
      return false
    const parsed = requestToolSchema.safeParse(action.payload)
    if (!parsed.success)
      return false
    await context.requestTool({ ...action, payload: { ...parsed.data, arguments: parsed.data.arguments ?? {} } })
    return true
  }
  return false
}
