import type { AutomaticRouteDelivery } from './automatic-actions'
import type { AIUIActionAcknowledgement } from '@/api'
import { getAIUIActionTargetPath } from './actions'

interface AutomaticRouteDeliveryContext {
  delivery: AutomaticRouteDelivery
  execute: (action: AutomaticRouteDelivery['action']) => Promise<boolean>
  acknowledge: (actionId: string, acknowledgement: Omit<AIUIActionAcknowledgement, 'clientInstanceId'>) => Promise<void>
  currentPath: () => string
  now?: () => number
  waitForPath?: (expectedPath: string) => Promise<boolean>
}

export async function executeAutomaticRouteDelivery(context: AutomaticRouteDeliveryContext): Promise<boolean> {
  const now = context.now ?? Date.now
  if (Date.parse(context.delivery.expiresAt) <= now()) {
    await context.acknowledge(context.delivery.actionId, {
      status: 'failed',
      errorCode: 'ai.ui_action_expired',
    })
    return false
  }
  const targetPath = getAIUIActionTargetPath(context.delivery.action)
  if (!targetPath) {
    await context.acknowledge(context.delivery.actionId, {
      status: 'failed',
      errorCode: 'ai.ui_action_invalid',
    })
    return false
  }
  if (!await context.execute(context.delivery.action)) {
    await context.acknowledge(context.delivery.actionId, {
      status: 'failed',
      errorCode: 'ai.ui_action_rejected',
    })
    return false
  }
  const landed = context.waitForPath
    ? await context.waitForPath(targetPath)
    : await waitForRoutePath(context.currentPath, targetPath)
  if (!landed) {
    await context.acknowledge(context.delivery.actionId, {
      status: 'failed',
      actualPath: context.currentPath(),
      errorCode: 'ai.ui_action_navigation_timeout',
    })
    return false
  }
  await context.acknowledge(context.delivery.actionId, {
    status: 'succeeded',
    actualPath: context.currentPath(),
  })
  return true
}

export async function waitForRoutePath(currentPath: () => string, expectedPath: string, timeoutMs = 4_000): Promise<boolean> {
  if (currentPath() === expectedPath)
    return true
  const startedAt = Date.now()
  while (Date.now() - startedAt < timeoutMs) {
    await new Promise(resolve => window.setTimeout(resolve, 25))
    if (currentPath() === expectedPath)
      return true
  }
  return false
}
