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
