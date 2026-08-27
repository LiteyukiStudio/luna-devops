export const AI_ASSISTANT_ROUTE_PATH = '/ai-assistant'
export const AI_ASSISTANT_DIRECT_ACCESS_FALLBACK_PATH = '/dashboard'
export const AI_ASSISTANT_ROUTE_VIEWS = ['chat', 'conversations'] as const

export type AIAssistantView = typeof AI_ASSISTANT_ROUTE_VIEWS[number]

export interface AIAssistantReturnLocation {
  pathname: string
  search: string
  hash: string
}

export interface AIAssistantRouteState {
  returnTo: AIAssistantReturnLocation
  aiView: AIAssistantView
  hasChatHistoryEntry: boolean
}

export interface AIAssistantRouteLocationLike {
  pathname: string
  search?: string
  hash?: string
}

const DEFAULT_AI_VIEW: AIAssistantView = 'chat'
const FALLBACK_RETURN_LOCATION: AIAssistantReturnLocation = {
  pathname: AI_ASSISTANT_DIRECT_ACCESS_FALLBACK_PATH,
  search: '',
  hash: '',
}
function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function isAIAssistantView(value: unknown): value is AIAssistantView {
  return value === 'chat' || value === 'conversations'
}

export function isAIAssistantRoutePath(pathname: unknown): boolean {
  if (typeof pathname !== 'string')
    return false

  const pathOnly = pathname.split(/[?#]/u, 1)[0]
  const normalized = pathOnly.replace(/\/+$/u, '') || '/'
  const lowerPath = normalized.toLowerCase()
  return lowerPath === AI_ASSISTANT_ROUTE_PATH || lowerPath.startsWith(`${AI_ASSISTANT_ROUTE_PATH}/`)
}

function safePathname(pathname: unknown): pathname is string {
  return typeof pathname === 'string'
    && pathname.startsWith('/')
    && !pathname.startsWith('//')
    && !isAIAssistantRoutePath(pathname)
}

function fallbackReturnLocation(): AIAssistantReturnLocation {
  return { ...FALLBACK_RETURN_LOCATION }
}

function readReturnLocation(value: unknown): AIAssistantReturnLocation | undefined {
  if (!isRecord(value))
    return undefined

  if (!safePathname(value.pathname))
    return undefined

  return {
    pathname: value.pathname,
    search: typeof value.search === 'string' ? value.search : '',
    hash: typeof value.hash === 'string' ? value.hash : '',
  }
}

export function createAIAssistantRouteState(
  location: AIAssistantRouteLocationLike,
  aiView: AIAssistantView = DEFAULT_AI_VIEW,
): AIAssistantRouteState {
  return {
    returnTo: readReturnLocation(location) ?? fallbackReturnLocation(),
    aiView,
    hasChatHistoryEntry: false,
  }
}

export function readAIAssistantRouteState(state: unknown): AIAssistantRouteState {
  if (!isRecord(state)) {
    return {
      returnTo: fallbackReturnLocation(),
      aiView: DEFAULT_AI_VIEW,
      hasChatHistoryEntry: false,
    }
  }

  const aiView = isAIAssistantView(state.aiView) ? state.aiView : DEFAULT_AI_VIEW
  return {
    returnTo: readReturnLocation(state.returnTo) ?? fallbackReturnLocation(),
    aiView,
    hasChatHistoryEntry: aiView === 'conversations' && state.hasChatHistoryEntry === true,
  }
}

export function withAIAssistantRouteView(state: unknown, aiView: AIAssistantView): AIAssistantRouteState {
  const current = readAIAssistantRouteState(state)
  return {
    ...current,
    aiView,
    hasChatHistoryEntry: aiView === 'conversations'
      && (current.aiView === 'chat' || current.hasChatHistoryEntry),
  }
}

export function resolveAIAssistantReturnPath(state: unknown): string {
  const { pathname, search, hash } = readAIAssistantRouteState(state).returnTo
  return `${pathname}${search}${hash}`
}
