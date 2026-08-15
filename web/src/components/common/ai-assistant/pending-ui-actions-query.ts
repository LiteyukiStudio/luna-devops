import type { AIPendingUIActions } from '@/api'

export const PENDING_UI_ACTIONS_POLL_INTERVAL_MS = 5_000
export const PENDING_UI_ACTIONS_RECOVERY_INTERVAL_MS = 30_000
const MIN_RECOVERY_INTERVAL_SECONDS = 5
const MAX_RECOVERY_INTERVAL_SECONDS = 300

export function pendingUIActionsPollInterval(data?: AIPendingUIActions, hasError = false) {
  if (!hasError && data?.agentAvailable !== false)
    return PENDING_UI_ACTIONS_POLL_INTERVAL_MS

  const configuredSeconds = data?.retryAfterSeconds
  if (typeof configuredSeconds !== 'number' || !Number.isFinite(configuredSeconds))
    return PENDING_UI_ACTIONS_RECOVERY_INTERVAL_MS

  return Math.min(
    MAX_RECOVERY_INTERVAL_SECONDS,
    Math.max(MIN_RECOVERY_INTERVAL_SECONDS, configuredSeconds),
  ) * 1_000
}
