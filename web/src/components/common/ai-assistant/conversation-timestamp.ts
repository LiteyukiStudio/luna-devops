export function formatAIConversationTimestamp(value: string, language: string, now = new Date()) {
  const timestamp = new Date(value)
  if (Number.isNaN(timestamp.getTime()))
    return ''
  const sameYear = timestamp.getFullYear() === now.getFullYear()
  const sameDay = sameYear
    && timestamp.getMonth() === now.getMonth()
    && timestamp.getDate() === now.getDate()
  const dateOptions: Intl.DateTimeFormatOptions = sameDay
    ? {}
    : sameYear
      ? { month: 'short', day: 'numeric' }
      : { year: 'numeric', month: 'short', day: 'numeric' }
  return new Intl.DateTimeFormat(language, {
    ...dateOptions,
    hour: '2-digit',
    minute: '2-digit',
  }).format(timestamp)
}

export type AIConversationRecency = 'today' | 'recent' | 'earlier'

const RECENT_DAYS = 7

/** 按本地日历把会话分为今天、7 天内与更早；保持输入顺序，空分组不返回。 */
export function groupAIConversationsByRecency<T extends { updatedAt: string }>(items: readonly T[], now = new Date()) {
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()
  const startOfRecent = new Date(now.getFullYear(), now.getMonth(), now.getDate() - (RECENT_DAYS - 1)).getTime()
  const groups: Record<AIConversationRecency, T[]> = { today: [], recent: [], earlier: [] }
  for (const item of items) {
    const updatedAt = new Date(item.updatedAt).getTime()
    const recency: AIConversationRecency = Number.isNaN(updatedAt) || updatedAt < startOfRecent
      ? 'earlier'
      : updatedAt >= startOfToday ? 'today' : 'recent'
    groups[recency].push(item)
  }
  return (['today', 'recent', 'earlier'] as const)
    .filter(recency => groups[recency].length > 0)
    .map(recency => ({ recency, items: groups[recency] }))
}
