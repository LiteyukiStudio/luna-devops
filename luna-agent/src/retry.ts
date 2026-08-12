export type RetryOptions = {
  maxRetries: number
  initialDelayMs?: number
  maxDelayMs?: number
  signal?: AbortSignal
  retryAfterMs?: number
}

const DEFAULT_INITIAL_DELAY_MS = 500
const DEFAULT_MAX_DELAY_MS = 8_000
const MAX_RETRY_AFTER_MS = 60_000

export function isRetryableHTTPStatus(status: number, includeConflict = false): boolean {
  return status === 408
    || status === 429
    || status >= 500
    || (includeConflict && status === 409)
}

export function parseRetryAfter(headers: Headers, now = Date.now()): number | undefined {
  const milliseconds = Number(headers.get("retry-after-ms"))
  if (Number.isFinite(milliseconds) && milliseconds >= 0)
    return milliseconds

  const raw = headers.get("retry-after")?.trim()
  if (!raw) return undefined
  const seconds = Number(raw)
  if (Number.isFinite(seconds) && seconds >= 0)
    return seconds * 1_000
  const timestamp = Date.parse(raw)
  if (Number.isNaN(timestamp)) return undefined
  return Math.max(0, timestamp - now)
}

export function retryDelayMs(attempt: number, options: RetryOptions, random = Math.random): number {
  if (options.retryAfterMs !== undefined)
    return Math.min(options.retryAfterMs, MAX_RETRY_AFTER_MS)
  const base = Math.min(
    (options.initialDelayMs ?? DEFAULT_INITIAL_DELAY_MS) * 2 ** Math.max(0, attempt - 1),
    options.maxDelayMs ?? DEFAULT_MAX_DELAY_MS,
  )
  // 乘法抖动避免多个 Agent 实例在上游恢复时同时重试。
  return Math.round(base * (0.75 + random() * 0.5))
}

export async function waitForRetry(attempt: number, options: RetryOptions): Promise<void> {
  const delay = retryDelayMs(attempt, options)
  if (options.signal?.aborted)
    throw abortReason(options.signal)
  await new Promise<void>((resolve, reject) => {
    const finish = () => {
      options.signal?.removeEventListener("abort", abort)
      resolve()
    }
    const timer = setTimeout(finish, delay)
    const abort = () => {
      clearTimeout(timer)
      reject(abortReason(options.signal))
    }
    options.signal?.addEventListener("abort", abort, { once: true })
    if (delay === 0) {
      clearTimeout(timer)
      options.signal?.removeEventListener("abort", abort)
      resolve()
    }
  })
}

function abortReason(signal?: AbortSignal): Error {
  return signal?.reason instanceof Error ? signal.reason : new Error("ai.run_canceled")
}
