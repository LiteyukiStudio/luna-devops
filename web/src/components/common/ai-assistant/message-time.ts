export function formatMessageTime(value: string, locale: string, now = new Date()) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime()))
    return { label: '', title: '' }

  const isSameYear = date.getFullYear() === now.getFullYear()
  const isSameDay = isSameYear
    && date.getMonth() === now.getMonth()
    && date.getDate() === now.getDate()
  const labelOptions: Intl.DateTimeFormatOptions = isSameDay
    ? { hour: '2-digit', minute: '2-digit' }
    : isSameYear
      ? { day: '2-digit', hour: '2-digit', minute: '2-digit', month: '2-digit' }
      : { day: '2-digit', hour: '2-digit', minute: '2-digit', month: '2-digit', year: 'numeric' }

  return {
    label: new Intl.DateTimeFormat(locale, labelOptions).format(date),
    title: new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(date),
  }
}
