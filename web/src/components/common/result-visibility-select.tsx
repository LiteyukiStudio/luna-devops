import type { ResultVisibility } from '@/api'
import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { cn } from '@/lib/utils'

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
    <div className={cn('min-w-0', containerClassName)}>
      <Select value={value} onValueChange={next => onChange(next as ResultVisibility)}>
        <SelectTrigger id={selectId} aria-label={t('common.resultVisibility.label')} className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="related">{t('common.resultVisibility.related')}</SelectItem>
          <SelectItem value="all">{t('common.resultVisibility.all')}</SelectItem>
        </SelectContent>
      </Select>
    </div>
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
