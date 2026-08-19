import { Layers } from 'lucide-react'
import { useTranslation } from 'react-i18next'

/**
 * 上下文压缩提示 badge。
 * 样式参考 navigation-event.tsx 的胶囊形态，但使用紫色系表达"记忆已被摘要"的系统提示语义。
 * 这是一次性装饰元素而非状态色，故直接使用 violet 色板；不与 danger/warning/success 等状态 token 冲突。
 * 非交互元素，仅作信息展示。
 */
export function AIContextCompactedBadge() {
  const { t } = useTranslation()
  return (
    <span
      aria-label={t('aiAssistant.contextCompacted.ariaLabel')}
      className="inline-flex h-6 max-w-full w-fit items-center gap-1.5 rounded-full bg-violet-500/10 px-2.5 text-[11px] font-medium text-violet-700 dark:bg-violet-400/10 dark:text-violet-300"
      data-ai-context-compacted
    >
      <Layers aria-hidden="true" className="size-3" />
      <span className="truncate">{t('aiAssistant.contextCompacted.label')}</span>
    </span>
  )
}
