export type UsageRingBaseTone = 'info' | 'primary' | 'success'

/**
 * 统一的三级语义色分级：
 * `<70%` 为健康基础色（默认 `text-success`），`70%~89%` 为 `text-warning`，`>=90%` 为 `text-danger`，无效值降级为 `text-muted-foreground`。
 */
export function usageRingToneClass(percent: number | undefined, baseTone: UsageRingBaseTone = 'success') {
  if (percent === undefined || !Number.isFinite(percent) || percent < 0)
    return 'text-muted-foreground'
  if (percent >= 90)
    return 'text-danger'
  if (percent >= 70)
    return 'text-warning'
  if (baseTone === 'primary')
    return 'text-primary'
  if (baseTone === 'info')
    return 'text-info'
  return 'text-success'
}
