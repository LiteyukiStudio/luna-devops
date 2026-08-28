import type { ResultVisibility } from '@/api'
import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { NativeSelect } from '@/components/ui/native-select'

interface ResultVisibilitySelectProps {
  canViewAll: boolean
  containerClassName?: string
  showLabel?: boolean
  value: ResultVisibility
  onChange: (visibility: ResultVisibility) => void
}

export function ResultVisibilitySelect({ canViewAll, containerClassName = 'min-w-32 sm:w-40', showLabel = false, value, onChange }: ResultVisibilitySelectProps) {
  const { t } = useTranslation()
  const selectId = useId()

  if (!canViewAll)
    return null

  const select = (
    <NativeSelect
      id={selectId}
      aria-label={t('common.resultVisibility.label')}
      containerClassName={containerClassName}
      value={value}
      onChange={event => onChange(event.target.value as ResultVisibility)}
    >
      <option value="related">{t('common.resultVisibility.related')}</option>
      <option value="all">{t('common.resultVisibility.all')}</option>
    </NativeSelect>
  )

  if (!showLabel)
    return select

  return (
    <label className="grid gap-1.5 text-xs text-muted-foreground" htmlFor={selectId}>
      {t('common.resultVisibility.label')}
      {select}
    </label>
  )
}
