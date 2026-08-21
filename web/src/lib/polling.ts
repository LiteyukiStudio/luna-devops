export const WORKFLOW_STATUS_REFETCH_INTERVAL_MS = 2000
export const IDLE_STATUS_REFETCH_INTERVAL_MS = 30_000

export function statusRefetchInterval(hasActiveWork: boolean) {
  return hasActiveWork ? WORKFLOW_STATUS_REFETCH_INTERVAL_MS : IDLE_STATUS_REFETCH_INTERVAL_MS
}
