import type { ReactNode } from 'react'
import type { BuildRun } from '@/api'
import { useTranslation } from 'react-i18next'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { EMPTY_SELECT_VALUE, fromSelectValue, toSelectValue } from '@/components/ui/select-value'

const buildRunStatusFilters: Array<BuildRun['status']> = ['queued', 'running', 'succeeded', 'failed', 'canceled', 'timeout']
const buildRunEventFilters: Array<BuildRun['triggerType']> = ['manual', 'push', 'tag', 'webhook', 'api', 'retry']

export function ApplicationBuildRunFilterBar({ actor, actorOptions, branch, branchOptions, event, onActorChange, onBranchChange, onEventChange, onStatusChange, status }: {
  actor: string
  actorOptions: string[]
  branch: string
  branchOptions: string[]
  event: string
  status: string
  onActorChange: (value: string) => void
  onBranchChange: (value: string) => void
  onEventChange: (value: string) => void
  onStatusChange: (value: string) => void
}) {
  const { t } = useTranslation()
  return (
    <div className="flex flex-wrap items-center gap-2">
      <BuildRunFilterSelect
        label={t('buildsPage.eventFilter')}
        value={event}
        onChange={onEventChange}
      >
        <SelectItem value={EMPTY_SELECT_VALUE}>{t('buildsPage.allEvents')}</SelectItem>
        {buildRunEventFilters.map(value => (
          <SelectItem key={value} value={value}>{t(`buildsPage.events.${value}`)}</SelectItem>
        ))}
      </BuildRunFilterSelect>
      <BuildRunFilterSelect
        label={t('buildsPage.statusFilter')}
        value={status}
        onChange={onStatusChange}
      >
        <SelectItem value={EMPTY_SELECT_VALUE}>{t('buildsPage.allStatuses')}</SelectItem>
        {buildRunStatusFilters.map(value => (
          <SelectItem key={value} value={value}>{t(`buildsPage.statuses.${value}`)}</SelectItem>
        ))}
      </BuildRunFilterSelect>
      <BuildRunFilterSelect
        label={t('buildsPage.branchFilter')}
        value={branch}
        onChange={onBranchChange}
      >
        <SelectItem value={EMPTY_SELECT_VALUE}>{t('buildsPage.allBranches')}</SelectItem>
        {branchOptions.map(value => (
          <SelectItem key={value} value={value}>{value}</SelectItem>
        ))}
      </BuildRunFilterSelect>
      <BuildRunFilterSelect
        label={t('buildsPage.actorFilter')}
        value={actor}
        onChange={onActorChange}
      >
        <SelectItem value={EMPTY_SELECT_VALUE}>{t('buildsPage.allActors')}</SelectItem>
        {actorOptions.map(value => (
          <SelectItem key={value} value={value}>{shortActorLabel(value)}</SelectItem>
        ))}
      </BuildRunFilterSelect>
    </div>
  )
}

function BuildRunFilterSelect({ children, label, onChange, value }: {
  children: ReactNode
  label: string
  value: string
  onChange: (value: string) => void
}) {
  return (
    <Select
      value={toSelectValue(value)}
      onValueChange={next => onChange(fromSelectValue(next))}
    >
      <SelectTrigger aria-label={label} className="w-full @lg/data-list:w-36">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {children}
      </SelectContent>
    </Select>
  )
}

function shortActorLabel(value: string) {
  if (!value)
    return '-'
  const index = value.indexOf('_')
  if (index >= 0)
    return value.slice(index + 1, index + 9)
  return value.length > 12 ? value.slice(0, 12) : value
}
