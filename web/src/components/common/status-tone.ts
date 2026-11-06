export type StatusTone = 'danger' | 'info' | 'neutral' | 'success' | 'warning'

export function statusToneFor(value: string): StatusTone {
  switch (value.trim().toLowerCase()) {
    case 'active':
    case 'available':
    case 'connected':
    case 'created':
    case 'enabled':
    case 'healthy':
    case 'issued':
    case 'passed':
    case 'ready':
    case 'succeeded':
    case 'success':
    case 'verified':
      return 'success'
    case 'failed':
    case 'error':
    case 'crash-loop-back-off':
    case 'create-container-config-error':
    case 'create-container-error':
    case 'delete_failed':
    case 'err-image-pull':
    case 'image-pull-back-off':
    case 'lost':
    case 'missing-credential':
    case 'revoked':
    case 'timeout':
    case 'unhealthy':
    case 'unavailable':
      return 'danger'
    case 'expired':
    case 'warning':
    case 'degraded':
    case 'checking':
    case 'container-creating':
    case 'pending':
    case 'preparing':
    case 'progressing':
    case 'queued':
    case 'deleting':
    case 'running':
    case 'in_progress':
    case 'scanning':
    case 'streaming':
    case 'not-ready':
    case 'unregistered':
      return 'warning'
    case 'createdstatus':
      return 'success'
    case 'info':
      return 'info'
    case 'disabled':
    case 'canceled':
    case 'cancelled':
    case 'scaled-to-zero':
    case 'not-configured':
    case 'not-found':
    case 'missing':
    case 'unknown':
      return 'neutral'
    default:
      return 'info'
  }
}

/** 状态指示器的图形状态：决定 StatusIndicator 画哪种图形，颜色由 statusIndicatorTone 统一映射。 */
export type StatusIndicatorState = 'canceled' | 'failure' | 'info' | 'neutral' | 'queued' | 'running' | 'success' | 'warning'

/** 把原始状态值映射为指示器状态：排队/等待与进行中优先识别，其余沿用 statusToneFor 的语义。 */
export function statusIndicatorStateFor(value: string | null | undefined): StatusIndicatorState {
  const normalized = (value ?? '').trim().toLowerCase()
  switch (normalized) {
    case 'pending':
    case 'queued':
      return 'queued'
    case 'checking':
    case 'connecting':
    case 'container-creating':
    case 'deleting':
    case 'in_progress':
    case 'preparing':
    case 'progressing':
    case 'running':
    case 'scanning':
    case 'streaming':
      return 'running'
    case 'canceled':
    case 'cancelled':
      return 'canceled'
  }
  return statusIndicatorStateForTone(statusToneFor(normalized))
}

function statusIndicatorStateForTone(tone: StatusTone): StatusIndicatorState {
  switch (tone) {
    case 'success':
      return 'success'
    case 'danger':
      return 'failure'
    case 'warning':
      return 'warning'
    case 'info':
      return 'info'
    case 'neutral':
      return 'neutral'
  }
}

/** 指示器状态到语义 tone：进行中与排队沿用 StatusBadge 对 running/queued 的琥珀色。 */
export function statusIndicatorTone(state: StatusIndicatorState): StatusTone {
  switch (state) {
    case 'running':
    case 'queued':
    case 'warning':
      return 'warning'
    case 'success':
      return 'success'
    case 'failure':
      return 'danger'
    case 'info':
      return 'info'
    case 'canceled':
    case 'neutral':
      return 'neutral'
  }
}
