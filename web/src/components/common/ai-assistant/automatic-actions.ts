import type { AIEvent, AIUIAction } from '@/api'

export function automaticRouteActionFromEvent(event: AIEvent): AIUIAction | null {
  if (event.type !== 'tool.completed' || event.payload.operationId !== 'navigate_to_route')
    return null
  const actions = Array.isArray(event.payload.uiActions) ? event.payload.uiActions : []
  const action = actions.find((candidate): candidate is AIUIAction =>
    Boolean(candidate)
    && typeof candidate === 'object'
    && (candidate as Partial<AIUIAction>).version === 1
    && (candidate as Partial<AIUIAction>).type === 'navigate'
    && 'activation' in candidate
    && candidate.activation === 'automatic',
  )
  return action ?? null
}
