import type { AIEvent, AIPendingUIAction, AIUIAction } from '@/api'
import { z } from 'zod'

export interface AutomaticRouteDelivery {
  actionId: string
  action: AIUIAction
  expiresAt: string
}

const deliverySchema = z.object({
  actionId: z.string().min(5).max(64),
  expiresAt: z.string().datetime(),
})

function automaticNavigateAction(value: unknown): AIUIAction | null {
  if (!value || typeof value !== 'object')
    return null
  const candidate = value as Partial<AIUIAction>
  return candidate.version === 1
    && candidate.type === 'navigate'
    && 'activation' in candidate
    && candidate.activation === 'automatic'
    ? candidate as AIUIAction
    : null
}

export function automaticRouteDeliveryFromEvent(event: AIEvent): AutomaticRouteDelivery | null {
  if (event.type !== 'tool.completed' || event.payload.operationId !== 'navigate_to_route')
    return null
  const actions = Array.isArray(event.payload.uiActions) ? event.payload.uiActions : []
  const action = actions.map(automaticNavigateAction).find(Boolean)
  const delivery = deliverySchema.safeParse(event.payload.uiActionDelivery)
  return action && delivery.success ? { ...delivery.data, action } : null
}

export function automaticRouteDeliveryFromPending(value: AIPendingUIAction): AutomaticRouteDelivery | null {
  const action = automaticNavigateAction(value.action)
  const delivery = deliverySchema.safeParse({ actionId: value.actionId, expiresAt: value.expiresAt })
  return action && delivery.success ? { ...delivery.data, action } : null
}
