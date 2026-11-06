import type { AIStepStatus } from './status-indicator-states'
import { useTranslation } from 'react-i18next'
import { StatusGlyph } from '@/components/common/status-indicator'
import { aiStepIndicatorStates } from './status-indicator-states'

/** 交互卡片状态列表与实时进度步骤共用的状态图形；图形 aria-hidden，状态含义由 sr-only 文本表达。 */
export function AIStepStatusGlyph({ className, status }: { className?: string, status: AIStepStatus }) {
  const { t } = useTranslation()
  return (
    <>
      <StatusGlyph className={className} state={aiStepIndicatorStates[status]} />
      <span className="sr-only">{t(`aiAssistant.cards.stepStatuses.${status}`)}</span>
    </>
  )
}
