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
