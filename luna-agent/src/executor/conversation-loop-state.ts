import type { ConversationHistoryEntry } from "../domain.js"

export const RECENT_EMPTY_READ_WINDOW_MS = 300_000

const explicitLiveRefreshPhrases = ["强制刷新实时状态", "重新观察实时状态", "force refresh live state", "refresh live state"]

export type HistoricalStableResult = {
  operationId: string
  argumentsHash: string
  result: unknown
}

/**
 * 跨轮保护只继承短时间内、成功且没有返回任何业务信息的读取结果。
 * 非空实时状态仍允许用户主动刷新，写操作和失败结果也不会在这里被重放或阻断。
 */
export function recentEmptyReadResults(
  history: ConversationHistoryEntry[],
  now = Date.now(),
  windowMs = RECENT_EMPTY_READ_WINDOW_MS,
): HistoricalStableResult[] {
  const latest = new Map<string, HistoricalStableResult>()
  for (const turn of history) {
    for (const raw of turn.toolInteractions ?? []) {
      const interaction = objectValue(raw)
      if (interaction.type !== "tool_call") continue
      const content = objectValue(interaction.content)
      if (content.status !== "succeeded") continue
      const operationId = stringValue(content.operationId)
      const argumentsHash = stringValue(content.argumentsHash)
      const createdAt = stringValue(interaction.createdAt)
      const observedAt = createdAt ? Date.parse(createdAt) : Number.NaN
      if (!operationId || !argumentsHash || !Number.isFinite(observedAt)) continue
      if (now < observedAt || now - observedAt > windowMs) continue
      if (!isEmptyBusinessResult(content.result)) continue
      latest.set(`${operationId}\u0000${argumentsHash}`, { operationId, argumentsHash, result: content.result })
    }
  }
  return [...latest.values()]
}

export function requestsExplicitLiveRefresh(input: string): boolean {
  const normalized = input.trim().toLocaleLowerCase()
  return explicitLiveRefreshPhrases.some(phrase => normalized.includes(phrase))
}

function isEmptyBusinessResult(input: unknown): boolean {
  const value = delegatedBusinessResult(input)
  if (Array.isArray(value)) return value.length === 0
  const object = objectValue(value)
  if (!Object.hasOwn(object, "items") || !Array.isArray(object.items) || object.items.length !== 0) return false
  return object.total === undefined || object.total === 0
}

function delegatedBusinessResult(input: unknown): unknown {
  const value = objectValue(input)
  return typeof value.operationId === "string" && typeof value.verified === "boolean" && Object.hasOwn(value, "result")
    ? value.result
    : input
}

function objectValue(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {}
}

function stringValue(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined
}
