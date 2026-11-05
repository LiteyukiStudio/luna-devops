import { Layers } from 'lucide-react'
import { useTranslation } from 'react-i18next'

/**
 * 上下文压缩提示：中性的系统说明，与导航事件同一尺寸，不使用状态色。
 * 非交互元素，仅作信息展示。
 */
export function AIContextCompactedBadge() {
  const { t } = useTranslation()
  return (
    <span
      aria-label={t('aiAssistant.contextCompacted.ariaLabel')}
      className="inline-flex h-7 max-w-full w-fit items-center gap-1.5 rounded-control border border-dashed border-border bg-surface-subtle px-2.5 text-xs font-medium text-muted-foreground"
      data-ai-context-compacted
      title={t('aiAssistant.contextCompacted.ariaLabel')}
    >
      <Layers aria-hidden="true" className="size-3.5" />
      <span className="truncate">{t('aiAssistant.contextCompacted.label')}</span>
    </span>
  )
}
