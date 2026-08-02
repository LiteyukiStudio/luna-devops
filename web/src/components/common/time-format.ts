import type { TFunction } from 'i18next'

const RELATIVE_TIME_THRESHOLD_MS = 10 * 60 * 60 * 1000

/**
 * 统一的“智能时间”显示：10 小时内显示相对时间，超过后显示带年份绝对时间。
 * 用于构建运行时间、任务更新时间等需要快速判断新鲜度的场景。
 */
export function formatSmartDateTime(value: string | undefined, t: TFunction, fallback = '-') {
  const date = parseDateTime(value)
  if (!date)
    return fallback
  const elapsedMs = Date.now() - date.getTime()
  if (elapsedMs >= 0 && elapsedMs < RELATIVE_TIME_THRESHOLD_MS)
    return formatRelativeElapsed(elapsedMs, t)
  return formatAbsoluteDateTimeParts(date)
}

/**
 * 统一绝对时间显示，按浏览器本地时区输出 `YYYY/MM/DD HH:mm`。
 * 用于列表中的创建时间、过期时间等不需要“刚刚/几分钟前”的稳定时间列。
 */
export function formatAbsoluteDateTime(value: string | undefined, fallback = '-') {
  const date = parseDateTime(value)
  if (!date)
    return fallback
  return formatAbsoluteDateTimeParts(date)
}

/**
 * 统一紧凑时间显示，按浏览器本地时区输出 `MM/DD HH:mm`。
 * 用于看板、卡片角标等空间有限但仍需要具体时间的位置。
 */
export function formatCompactDateTime(value: string | undefined, fallback = '-') {
  const date = parseDateTime(value)
  if (!date)
    return fallback
  return `${padTimePart(date.getMonth() + 1)}/${padTimePart(date.getDate())} ${padTimePart(date.getHours())}:${padTimePart(date.getMinutes())}`
}

/**
 * 统一耗时格式化，按当前语言显示时/分/秒或 h/m/s。
 * 用于构建、部署、网关任务等有 startedAt/finishedAt 的执行记录。
 */
export function formatElapsedDuration(startValue: string | undefined, endValue: string | undefined, running: boolean, t: TFunction) {
  const startedAt = parseDateTime(startValue)
  if (!startedAt)
    return ''
  const finishedAt = parseDateTime(endValue) ?? (running ? new Date() : null)
  if (!finishedAt)
    return ''
  return formatDurationSeconds(Math.max(0, Math.floor((finishedAt.getTime() - startedAt.getTime()) / 1000)), t)
}

/**
 * 格式化毫秒级短耗时；工具调用等紧凑状态使用毫秒，超过一秒后切换为本地化秒数。
 */
export function formatMillisecondsDuration(durationMs: number, locale: string): string {
  if (durationMs < 1000)
    return `${Math.round(durationMs)} ms`
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: 2 }).format(durationMs / 1000)} s`
}

function formatRelativeElapsed(elapsedMs: number, t: TFunction) {
  const totalMinutes = Math.max(0, Math.floor(elapsedMs / 60000))
  const hours = Math.floor(totalMinutes / 60)
  const minutes = totalMinutes % 60
  if (hours > 0)
    return t('time.relativeHoursMinutesAgo', { hours, minutes })
  if (minutes > 0)
    return t('time.relativeMinutesAgo', { minutes })
  return t('time.relativeJustNow')
}

function formatDurationSeconds(elapsedSeconds: number, t: TFunction) {
  const hours = Math.floor(elapsedSeconds / 3600)
  const minutes = Math.floor((elapsedSeconds % 3600) / 60)
  const seconds = elapsedSeconds % 60
  if (hours > 0)
    return t('time.durationHoursMinutesSeconds', { hours, minutes, seconds })
  return t('time.durationMinutesSeconds', { minutes, seconds })
}

function formatAbsoluteDateTimeParts(date: Date) {
  const year = date.getFullYear()
  const month = padTimePart(date.getMonth() + 1)
  const day = padTimePart(date.getDate())
  const hour = padTimePart(date.getHours())
  const minute = padTimePart(date.getMinutes())
  return `${year}/${month}/${day} ${hour}:${minute}`
}

function parseDateTime(value?: string) {
  if (!value)
    return null
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? null : date
}

function padTimePart(value: number) {
  return value.toString().padStart(2, '0')
}
