import type { StatusTone } from '@/components/common/status-tone'

/** Kubernetes 资源状态是自由文本（Running、Programmed、CrashLoopBackOff…），按关键词归入语义 tone。 */
export function toneForResourceStatus(status: string): StatusTone {
  const normalized = status.trim().toLowerCase()
  if (
    normalized.includes('fail')
    || normalized.includes('error')
    || normalized.includes('backoff')
    || normalized.includes('back-off')
    || normalized.includes('crash')
    || normalized.includes('oom')
    || normalized.includes('evict')
    || normalized.includes('unschedulable')
    || normalized.includes('degraded')
  ) {
    return 'danger'
  }
  if (
    normalized.includes('not-ready')
    || normalized.includes('not ready')
    || normalized.includes('terminating')
    || normalized.includes('pending')
    || normalized.includes('progress')
    || normalized.includes('creating')
  ) {
    return 'warning'
  }
  if (
    normalized.includes('ready')
    || normalized.includes('running')
    || normalized.includes('programmed')
    || normalized.includes('bound')
  ) {
    return 'success'
  }
  return 'neutral'
}
