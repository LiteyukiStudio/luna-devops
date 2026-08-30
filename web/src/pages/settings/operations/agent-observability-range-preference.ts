import type { AgentObservabilityRange } from '@/api'
import { safeStorageGet, safeStorageSet } from '@/lib/safe-storage'

export const agentObservabilityRanges: readonly AgentObservabilityRange[] = ['1h', '6h', '24h', '7d', '30d', '1y']
export const defaultAgentObservabilityRange: AgentObservabilityRange = '1h'

const storageKeyPrefix = 'luna.operations.agent-observability.range.v1'

export function readAgentObservabilityRange(userId: string, storage?: Storage): AgentObservabilityRange {
  const value = safeStorageGet(storageKey(userId), storage ? () => storage : undefined)
  return isAgentObservabilityRange(value) ? value : defaultAgentObservabilityRange
}

export function writeAgentObservabilityRange(userId: string, value: AgentObservabilityRange, storage?: Storage) {
  safeStorageSet(storageKey(userId), value, storage ? () => storage : undefined)
}

export function agentObservabilityRangeStorageKey(userId: string) {
  return storageKey(userId)
}

function isAgentObservabilityRange(value: string | null): value is AgentObservabilityRange {
  return agentObservabilityRanges.includes(value as AgentObservabilityRange)
}

function storageKey(userId: string) {
  return `${storageKeyPrefix}.${userId.trim() || 'anonymous'}`
}
