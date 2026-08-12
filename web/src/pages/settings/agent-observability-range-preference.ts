import type { AgentObservabilityRange } from '@/api'

export const agentObservabilityRanges: readonly AgentObservabilityRange[] = ['1h', '6h', '24h', '7d', '30d', '1y']
export const defaultAgentObservabilityRange: AgentObservabilityRange = '1h'

const storageKeyPrefix = 'luna.operations.agent-observability.range.v1'

export function readAgentObservabilityRange(userId: string, storage = browserLocalStorage()): AgentObservabilityRange {
  if (!storage)
    return defaultAgentObservabilityRange
  try {
    const value = storage.getItem(storageKey(userId))
    return isAgentObservabilityRange(value) ? value : defaultAgentObservabilityRange
  }
  catch {
    return defaultAgentObservabilityRange
  }
}

export function writeAgentObservabilityRange(userId: string, value: AgentObservabilityRange, storage = browserLocalStorage()) {
  if (!storage)
    return
  try {
    storage.setItem(storageKey(userId), value)
  }
  catch {
    // Browser storage may be disabled; the in-memory selection still remains usable.
  }
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

function browserLocalStorage() {
  if (typeof window === 'undefined')
    return null
  try {
    return window.localStorage
  }
  catch {
    return null
  }
}
