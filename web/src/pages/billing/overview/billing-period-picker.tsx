import type { BillingPeriodPreset, BillingPeriodSelection } from './billing-page-utils'
import { CalendarDays } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { periodSelectionForPreset } from './billing-page-utils'

export function BillingPeriodPicker({ period, onChange }: { period: BillingPeriodSelection, onChange: (period: BillingPeriodSelection) => void }) {
  const { t } = useTranslation()
  const presets: BillingPeriodPreset[] = ['thisWeek', 'last7Days', 'thisMonth', 'last30Days', 'thisYear', 'lastYear']

  const updateDate = (field: 'startDate' | 'endDate', value: string) => {
    if (!value)
      return
    const next = { ...period, [field]: value, preset: 'custom' as const }
    if (next.startDate > next.endDate) {
      if (field === 'startDate')
        next.endDate = value
      else
        next.startDate = value
    }
    onChange(next)
  }

  return (
    <div className="flex w-full min-w-0 flex-col gap-2 md:flex-row md:items-center xl:w-auto">
      <Select
        value={period.preset}
        onValueChange={(value) => {
          if (value === 'custom')
            onChange({ ...period, preset: 'custom' })
          else
            onChange(periodSelectionForPreset(value as BillingPeriodPreset))
        }}
      >
        <SelectTrigger className="!h-11 rounded-2xl md:w-40">
          <CalendarDays className="size-4 shrink-0 text-muted-foreground" />
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {presets.map(preset => (
            <SelectItem key={preset} value={preset}>{t(`billingPage.periodPresets.${preset}`)}</SelectItem>
          ))}
          <SelectItem value="custom">{t('billingPage.periodPresets.custom')}</SelectItem>
        </SelectContent>
      </Select>
      <div className="grid min-w-0 flex-1 grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] items-center gap-2 xl:w-[26rem] xl:flex-none">
        <Input
          aria-label={t('billingPage.periodStartDate')}
          className="h-11 rounded-2xl px-3"
          max={period.endDate}
          type="date"
          value={period.startDate}
          onChange={event => updateDate('startDate', event.target.value)}
        />
        <span className="text-xs text-muted-foreground">{t('billingPage.periodSeparator')}</span>
        <Input
          aria-label={t('billingPage.periodEndDate')}
          className="h-11 rounded-2xl px-3"
          min={period.startDate}
          type="date"
          value={period.endDate}
          onChange={event => updateDate('endDate', event.target.value)}
        />
      </div>
    </div>
  )
}
