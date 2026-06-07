export type StatusTone = 'danger' | 'info' | 'neutral' | 'success' | 'warning'

export function statusToneFor(value: string): StatusTone {
  switch (value.trim().toLowerCase()) {
    case 'active':
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
    case 'missing-credential':
    case 'revoked':
    case 'unhealthy':
      return 'danger'
    case 'expired':
    case 'pending':
    case 'queued':
    case 'running':
    case 'scanning':
      return 'warning'
    case 'createdstatus':
      return 'success'
    case 'disabled':
    case 'canceled':
    case 'unknown':
      return 'neutral'
    default:
      return 'info'
  }
}
